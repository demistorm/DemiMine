package net.demimine.dynamic.handlers;

import com.velocitypowered.api.event.EventTask;
import com.velocitypowered.api.event.player.ServerConnectedEvent;
import com.velocitypowered.api.proxy.Player;
import com.velocitypowered.api.proxy.ProxyServer;
import com.velocitypowered.api.proxy.server.RegisteredServer;
import net.demimine.dynamic.ApiClient;
import net.demimine.dynamic.AutoStopManager;
import net.demimine.dynamic.Config;
import org.slf4j.Logger;

import java.util.Optional;

public class ServerConnectedHandler {
    private final ProxyServer server;
    private final Config config;
    private final Logger logger;
    private final AutoStopManager autoStopManager;
    private final ApiClient apiClient;

    public ServerConnectedHandler(ProxyServer server, Config config, Logger logger, AutoStopManager autoStopManager, ApiClient apiClient) {
        this.server = server;
        this.config = config;
        this.logger = logger;
        this.autoStopManager = autoStopManager;
        this.apiClient = apiClient;
    }

    @com.velocitypowered.api.event.Subscribe
    public EventTask onServerConnected(ServerConnectedEvent event) {
        return EventTask.async(() -> {
            Player player = event.getPlayer();
            String serverName = event.getServer().getServerInfo().getName();

            logger.info(player.getUsername() + " connected to " + serverName);

            Optional<RegisteredServer> previousServer = event.getPreviousServer();
            if (previousServer.isPresent()) {
                String previousServerName = previousServer.get().getServerInfo().getName();
                logger.info(player.getUsername() + " transferred from " + previousServerName);
                autoStopManager.removePlayerFromServer(player, previousServerName);
                apiClient.reportPlayerLeave(player.getUniqueId().toString(), previousServerName);

                ApiClient.ServerStatus status = apiClient.getServerStatus(previousServerName);
                if (status != null && autoStopManager.isServerEmpty(previousServerName) && !config.configVar.excludedServers.contains(previousServerName) && !previousServerName.equals(config.configVar.loginServer)) {
                    autoStopManager.scheduleStopTimer(previousServerName);
                }
            }

            autoStopManager.addPlayerToServer(player, serverName);
        });
    }
}
