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
        return EventTask.async(() -> {
            Player player = event.getPlayer();
            logger.info(player.getUsername() + " disconnected");

            Optional<ServerConnection> lastServer = player.getCurrentServer();
            if (lastServer.isPresent()) {
                String serverName = lastServer.get().getServerInfo().getName();
                autoStopManager.removePlayerFromServer(player, serverName);
                apiClient.reportPlayerLeave(player.getUniqueId().toString(), serverName);

                ApiClient.ServerStatus status = apiClient.getServerStatus(serverName);
                if (status != null && status.auto_shutdown_minutes > 0 && autoStopManager.isServerEmpty(serverName) && !config.configVar.excludedServers.contains(serverName)) {
                    autoStopManager.scheduleStopTimer(serverName);
                }
            }
        });
    }
}
