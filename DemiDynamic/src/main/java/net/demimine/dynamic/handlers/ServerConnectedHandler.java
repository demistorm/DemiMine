package net.demimine.dynamic.handlers;

import com.velocitypowered.api.event.EventTask;
import com.velocitypowered.api.event.player.ServerConnectedEvent;
import com.velocitypowered.api.proxy.Player;
import com.velocitypowered.api.proxy.ProxyServer;
import net.demimine.dynamic.AutoStopManager;
import net.demimine.dynamic.Config;
import org.slf4j.Logger;

public class ServerConnectedHandler {
    private final ProxyServer server;
    private final Config config;
    private final Logger logger;
    private final AutoStopManager autoStopManager;

    public ServerConnectedHandler(ProxyServer server, Config config, Logger logger, AutoStopManager autoStopManager) {
        this.server = server;
        this.config = config;
        this.logger = logger;
        this.autoStopManager = autoStopManager;
    }

    @com.velocitypowered.api.event.Subscribe
    public EventTask onServerConnected(ServerConnectedEvent event) {
        return EventTask.async(() -> {
            Player player = event.getPlayer();
            String serverName = event.getServer().getServerInfo().getName();

            logger.info(player.getUsername() + " connected to " + serverName);
            autoStopManager.addPlayerToServer(player, serverName);
        });
    }
}
