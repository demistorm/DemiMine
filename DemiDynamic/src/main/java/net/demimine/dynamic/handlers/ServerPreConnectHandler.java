package net.demimine.dynamic.handlers;

import com.velocitypowered.api.event.EventTask;
import com.velocitypowered.api.event.connection.PreLoginEvent;
import com.velocitypowered.api.event.player.ServerPreConnectEvent;
import com.velocitypowered.api.proxy.Player;
import com.velocitypowered.api.proxy.ProxyServer;
import com.velocitypowered.api.proxy.ServerConnection;
import com.velocitypowered.api.proxy.server.RegisteredServer;
import net.demimine.dynamic.ApiClient;
import net.demimine.dynamic.AutoStopManager;
import net.demimine.dynamic.Config;
import net.demimine.dynamic.DemiDynamic;
import net.demimine.dynamic.QueueManager;
import net.kyori.adventure.text.Component;
import net.kyori.adventure.text.minimessage.MiniMessage;
import org.slf4j.Logger;

import java.util.List;
import java.util.Optional;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

public class ServerPreConnectHandler {
    private final ProxyServer server;
    private final Config config;
    private final Logger logger;
    private final QueueManager queueManager;
    private final AutoStopManager autoStopManager;
    private final ApiClient apiClient;
    private final DemiDynamic plugin;

    public ServerPreConnectHandler(ProxyServer server, Config config, Logger logger, QueueManager queueManager, AutoStopManager autoStopManager, ApiClient apiClient, DemiDynamic plugin) {
        this.server = server;
        this.config = config;
        this.logger = logger;
        this.queueManager = queueManager;
        this.autoStopManager = autoStopManager;
        this.apiClient = apiClient;
        this.plugin = plugin;
    }

    @com.velocitypowered.api.event.Subscribe
    public EventTask onServerPreConnect(ServerPreConnectEvent event) {
        return EventTask.withContinuation(continuation -> {
            Player player = event.getPlayer();
            RegisteredServer targetServer = event.getOriginalServer();
            String serverName = targetServer.getServerInfo().getName();

            logger.info(player.getUsername() + " attempting to connect to " + serverName);

            if (!hasPermission(player)) {
                MiniMessage mm = MiniMessage.miniMessage();
                player.sendMessage(mm.deserialize("<red>You must authenticate first! Please reconnect and complete authentication."));
                event.setResult(ServerPreConnectEvent.ServerResult.denied());
                continuation.resume();
                return;
            }

            if (!queueManager.isInAnyQueue(player)) {
                Optional<ServerConnection> currentServer = player.getCurrentServer();
                String currentServerName = currentServer.map(sc -> sc.getServerInfo().getName()).orElse(null);

                if (config.configVar.hubServer.equals(currentServerName) && config.configVar.useHubQueue) {
                    handleHubQueuePlayer(event, player, serverName, currentServerName);
                } else {
                    handleDirectConnectionPlayer(event, player, serverName, currentServerName);
                }
            } else {
                String targetName = targetServer.getServerInfo().getName();
                if (queueManager.isInQueue(player, targetName) && isServerRunning(targetName)) {
                    event.setResult(ServerPreConnectEvent.ServerResult.allowed(targetServer));
                } else {
                    event.setResult(ServerPreConnectEvent.ServerResult.denied());
                }
            }
            continuation.resume();
        });
    }

    private boolean hasPermission(Player player) {
        return player.hasPermission(config.configVar.authPermission);
    }

    private boolean isExcluded(String serverName) {
        return config.configVar.excludedServers.contains(serverName);
    }

    private void handleHubQueuePlayer(ServerPreConnectEvent event, Player player, String serverName, String currentServerName) {
        if (serverName.equals(currentServerName)) {
            event.setResult(ServerPreConnectEvent.ServerResult.allowed(event.getOriginalServer()));
            return;
        }

        if (isServerRunning(serverName)) {
            event.setResult(ServerPreConnectEvent.ServerResult.allowed(event.getOriginalServer()));
            apiClient.reportPlayerJoin(player.getUniqueId().toString(), player.getUsername(), serverName);
            return;
        }

        if (isExcluded(serverName)) {
            MiniMessage mm = MiniMessage.miniMessage();
            player.sendMessage(mm.deserialize("<red>This server is currently offline and not automatically started."));
            event.setResult(ServerPreConnectEvent.ServerResult.denied());
            return;
        }

        if (queueManager.addToQueue(player, serverName)) {
            event.setResult(ServerPreConnectEvent.ServerResult.denied());
            
            sendStartingMessage(player);

            server.getScheduler().buildTask(plugin, () -> {
                startServerAndQueue(player, serverName, currentServerName, true);
            }).schedule();
        }
    }

    private void handleDirectConnectionPlayer(ServerPreConnectEvent event, Player player, String serverName, String currentServerName) {
        if (isServerRunning(serverName)) {
            event.setResult(ServerPreConnectEvent.ServerResult.allowed(event.getOriginalServer()));
            apiClient.reportPlayerJoin(player.getUniqueId().toString(), player.getUsername(), serverName);
            return;
        }

        if (isExcluded(serverName)) {
            MiniMessage mm = MiniMessage.miniMessage();
            player.sendMessage(mm.deserialize("<red>This server is currently offline and not automatically started."));
            event.setResult(ServerPreConnectEvent.ServerResult.denied());
            return;
        }

        Optional<RegisteredServer> queueServer = server.getServer(config.configVar.queueServer);
        if (queueServer.isEmpty()) {
            MiniMessage mm = MiniMessage.miniMessage();
            player.sendMessage(mm.deserialize("<red>Server is offline and queue is unavailable. Please try again later."));
            event.setResult(ServerPreConnectEvent.ServerResult.denied());
            return;
        }

        if (queueManager.addToQueue(player, serverName)) {
            event.setResult(ServerPreConnectEvent.ServerResult.allowed(queueServer.get()));

            sendStartingMessage(player);

            server.getScheduler().buildTask(plugin, () -> {
                startServerAndQueue(player, serverName, config.configVar.queueServer, false);
            }).schedule();
        }
    }

    private void startServerAndQueue(Player player, String serverName, String currentServerName, boolean inHub) {
        ApiClient.CanStartResponse canStart = apiClient.canStartServer(serverName);

        if (!canStart.can_start) {
            List<String> toStop = autoStopManager.getServersWithPendingStopTimers();
            if (!toStop.isEmpty()) {
                logger.info("Insufficient RAM, stopping " + toStop.size() + " idle servers");

                for (String server : toStop) {
                    logger.info("Stopping idle server: " + server);
                    apiClient.stopServerByName(server);
                }

                for (String server : toStop) {
                    logger.info("Waiting for container removal: " + server);
                    apiClient.waitForContainerRemoval(server);
                }

                canStart = apiClient.canStartServer(serverName);
            }
        }

        if (canStart.can_start) {
            boolean started = apiClient.startServer(serverName);
            if (started) {
                waitForServerReady(player, serverName, currentServerName, inHub);
            } else {
                sendResourceExceededMessage(player, currentServerName, inHub);
            }
        } else {
            sendResourceExceededMessage(player, currentServerName, inHub);
        }
    }

    private void waitForServerReady(Player player, String serverName, String currentServerName, boolean inHub) {
        server.getScheduler().buildTask(plugin, () -> {
            int attempts = 0;
            int maxAttempts = config.configVar.startTimeoutSeconds / config.configVar.checkIntervalSeconds;

            while (attempts < maxAttempts) {
                try {
                    TimeUnit.SECONDS.sleep(config.configVar.checkIntervalSeconds);
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    break;
                }
                attempts++;

                Optional<RegisteredServer> targetServer = server.getServer(serverName);
                if (targetServer.isPresent()) {
                    try {
                        targetServer.get().ping().get(2, TimeUnit.SECONDS);
                        logger.info("Server " + serverName + " is responsive (ping success)");
                        if (queueManager.isInQueue(player, serverName)) {
                            if (inHub) {
                                queueManager.startCountdown(player, serverName, () -> {
                                    player.createConnectionRequest(targetServer.get()).fireAndForget();
                                    apiClient.reportPlayerJoin(player.getUniqueId().toString(), player.getUsername(), serverName);
                                    queueManager.removeFromQueue(player);
                                });
                            } else {
                                player.createConnectionRequest(targetServer.get()).fireAndForget();
                                apiClient.reportPlayerJoin(player.getUniqueId().toString(), player.getUsername(), serverName);
                                queueManager.removeFromQueue(player);
                            }
                        }
                        return;
                    } catch (Exception e) {
                        logger.debug("Ping attempt " + attempts + " failed for " + serverName + ": " + e.getMessage());
                    }
                }

                if (attempts % 10 == 0) {
                    MiniMessage mm = MiniMessage.miniMessage();
                    player.sendMessage(mm.deserialize(config.configVar.messages.loading));
                }
            }

            logger.warn("Server " + serverName + " did not start in time");
            queueManager.removeFromQueue(player);
            MiniMessage mm = MiniMessage.miniMessage();
            player.sendMessage(mm.deserialize("<red>Server failed to start. Please try again later."));
        }).schedule();
    }

    private boolean isServerRunning(String serverName) {
        Optional<RegisteredServer> registeredServer = server.getServer(serverName);
        if (registeredServer.isEmpty()) {
            return false;
        }

        try {
            registeredServer.get().ping().get(2, TimeUnit.SECONDS);
            return true;
        } catch (Exception e) {
            return false;
        }
    }

    private void sendStartingMessage(Player player) {
        MiniMessage mm = MiniMessage.miniMessage();
        player.sendMessage(mm.deserialize(config.configVar.messages.starting));
    }

    private void sendResourceExceededMessage(Player player, String currentServerName, boolean inHub) {
        MiniMessage mm = MiniMessage.miniMessage();
        String message = config.configVar.messages.resourceExceeded;

        if (inHub) {
            player.sendMessage(mm.deserialize(message));
        } else {
            player.disconnect(mm.deserialize(message));
        }
        queueManager.removeFromQueue(player);
    }
}
