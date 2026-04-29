package net.demimine.dynamic.handlers;

import com.velocitypowered.api.event.EventTask;
import com.velocitypowered.api.event.connection.DisconnectEvent;
import com.velocitypowered.api.proxy.Player;
import com.velocitypowered.api.proxy.ProxyServer;
import com.velocitypowered.api.proxy.ServerConnection;
import net.demimine.dynamic.ApiClient;
import net.demimine.dynamic.AutoStopManager;
import net.demimine.dynamic.Config;
import org.slf4j.Logger;

import java.util.Optional;

public class DisconnectHandler {
    private final ProxyServer server;
    private final Config config;
    private final Logger logger;
    private final AutoStopManager autoStopManager;
    private final ApiClient apiClient;

    public DisconnectHandler(ProxyServer server, Config config, Logger logger, AutoStopManager autoStopManager, ApiClient apiClient) {
        this.server = server;
        this.config = config;
        this.logger = logger;
        this.autoStopManager = autoStopManager;
        this.apiClient = apiClient;
    }

    @com.velocitypowered.api.event.Subscribe
    public EventTask onPlayerDisconnect(DisconnectEvent event) {
        Player player = event.getPlayer();
        Optional<ServerConnection> lastServer = player.getCurrentServer();
        String serverName = lastServer.map(sc -> sc.getServerInfo().getName()).orElse(null);

        return EventTask.async(() -> {
            logger.info(player.getUsername() + " disconnected");

            if (serverName != null) {
                autoStopManager.removePlayerFromServer(player, serverName);
                apiClient.reportPlayerLeave(player.getUniqueId().toString(), serverName);

                ApiClient.ServerStatus status = apiClient.getServerStatus(serverName);
                if (status == null) {
                    logger.warn("Could not get status for " + serverName + ", skipping auto-stop check");
                } else if (!autoStopManager.isServerEmpty(serverName)) {
                    logger.info("Skipping auto-stop for " + serverName + " (players still present)");
                } else if (config.configVar.excludedServers.contains(serverName)) {
                    logger.info("Skipping auto-stop for " + serverName + " (excluded)");
                } else if (serverName.equals(config.configVar.loginServer)) {
                    logger.info("Skipping auto-stop for " + serverName + " (login server)");
                } else {
                    autoStopManager.scheduleStopTimer(serverName);
                }
            } else {
                logger.warn(player.getUsername() + " had no server connection at disconnect, skipping auto-stop check");
            }
        });
    }
}
