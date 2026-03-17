package net.demimine.dynamic.commands;

import com.velocitypowered.api.command.CommandSource;
import com.velocitypowered.api.command.SimpleCommand;
import com.velocitypowered.api.proxy.Player;
import com.velocitypowered.api.proxy.ProxyServer;
import com.velocitypowered.api.proxy.ServerConnection;
import net.demimine.dynamic.ApiClient;
import net.demimine.dynamic.AutoStopManager;
import net.demimine.dynamic.Config;
import net.demimine.dynamic.DemiDynamic;
import net.demimine.dynamic.QueueManager;
import net.kyori.adventure.text.Component;
import net.kyori.adventure.text.minimessage.MiniMessage;
import net.kyori.adventure.text.minimessage.tag.resolver.Placeholder;
import org.slf4j.Logger;

import java.util.Optional;

public class CancelCommand implements SimpleCommand {
    private final ProxyServer server;
    private final Config config;
    private final Logger logger;
    private final QueueManager queueManager;
    private final AutoStopManager autoStopManager;
    private final ApiClient apiClient;
    private final DemiDynamic plugin;

    public CancelCommand(ProxyServer server, Config config, Logger logger, QueueManager queueManager, AutoStopManager autoStopManager, ApiClient apiClient, DemiDynamic plugin) {
        this.server = server;
        this.config = config;
        this.logger = logger;
        this.queueManager = queueManager;
        this.autoStopManager = autoStopManager;
        this.apiClient = apiClient;
        this.plugin = plugin;
    }

    @Override
    public void execute(Invocation invocation) {
        CommandSource source = invocation.source();

        if (!(source instanceof Player)) {
            source.sendMessage(Component.text("This command can only be used by players."));
            return;
        }

        Player player = (Player) source;

        if (!queueManager.isInAnyQueue(player)) {
            MiniMessage mm = MiniMessage.miniMessage();
            player.sendMessage(mm.deserialize("<yellow>You are not queued for any server."));
            return;
        }

        Optional<ServerConnection> currentServer = player.getCurrentServer();
        String currentServerName = currentServer.map(sc -> sc.getServerInfo().getName()).orElse("");

        if (currentServerName.equals(config.configVar.queueServer)) {
            handleQueueServerCancel(player);
        } else {
            handleHubServerCancel(player);
        }
    }

    private void handleQueueServerCancel(Player player) {
        queueManager.cancelCountdown(player);
        String targetServer = queueManager.isInAnyQueue(player) ? getQueuedServer(player) : "unknown";
        queueManager.removeFromQueue(player);

        if (queueManager.isQueueEmpty(targetServer)) {
            queueManager.clearQueue(targetServer);
        }

        MiniMessage mm = MiniMessage.miniMessage();
        player.sendMessage(mm.deserialize(config.configVar.messages.cancelDisconnect));
        player.disconnect(Component.text("Queue canceled"));
    }

    private void handleHubServerCancel(Player player) {
        String targetServer = getQueuedServer(player);
        queueManager.cancelCountdown(player);
        queueManager.removeFromQueue(player);

        if (queueManager.isQueueEmpty(targetServer)) {
            queueManager.clearQueue(targetServer);
            cancelServerStart(targetServer);
        }

        MiniMessage mm = MiniMessage.miniMessage();
        Component message = mm.deserialize(config.configVar.messages.cancelQueue,
            Placeholder.parsed("server", targetServer));
        player.sendMessage(message);
    }

    private String getQueuedServer(Player player) {
        return queueManager.getQueuedServer(player);
    }

    private void cancelServerStart(String serverName) {
        server.getScheduler().buildTask(plugin, () -> {
            apiClient.stopServerByName(serverName);
        }).schedule();
    }

    @Override
    public boolean hasPermission(Invocation invocation) {
        return invocation.source().hasPermission("demimine.authenticated");
    }
}
