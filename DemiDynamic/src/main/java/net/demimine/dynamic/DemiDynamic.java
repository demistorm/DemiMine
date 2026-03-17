package net.demimine.dynamic;

import com.google.inject.Inject;
import com.velocitypowered.api.command.CommandManager;
import com.velocitypowered.api.command.CommandMeta;
import com.velocitypowered.api.command.SimpleCommand;
import com.velocitypowered.api.event.Subscribe;
import com.velocitypowered.api.event.proxy.ProxyInitializeEvent;
import com.velocitypowered.api.event.proxy.ProxyReloadEvent;
import com.velocitypowered.api.plugin.Plugin;
import com.velocitypowered.api.plugin.annotation.DataDirectory;
import com.velocitypowered.api.proxy.ProxyServer;
import net.demimine.dynamic.commands.CancelCommand;
import net.demimine.dynamic.handlers.DisconnectHandler;
import net.demimine.dynamic.handlers.ServerConnectedHandler;
import net.demimine.dynamic.handlers.ServerPreConnectHandler;
import org.slf4j.Logger;

import java.io.IOException;
import java.nio.file.Path;

@Plugin(
    id = "demidynamic",
    name = "DemiDynamic",
    version = BuildConstants.VERSION,
    authors = {"DemiMine Team"},
    dependencies = {
        @com.velocitypowered.api.plugin.Dependency(id = "luckperms", optional = true)
    }
)
public class DemiDynamic {
    private final ProxyServer server;
    private final Logger logger;
    private final Path dataDirectory;
    private final Config config;
    private final QueueManager queueManager;
    private final AutoStopManager autoStopManager;
    private final ApiClient apiClient;

    @Inject
    public DemiDynamic(ProxyServer server, Logger logger, @DataDirectory Path dataDirectory) {
        this.server = server;
        this.logger = logger;
        this.dataDirectory = dataDirectory;
        this.config = new Config(logger, dataDirectory);
        this.queueManager = new QueueManager(server, config, logger);
        this.autoStopManager = new AutoStopManager(server, config, logger, queueManager, this);
        this.apiClient = new ApiClient(config, logger);
    }

    @Subscribe
    public void onProxyInitialization(ProxyInitializeEvent event) {
        try {
            logger.info("Initializing DemiDynamic plugin...");
            config.initConfig();
        } catch (Exception e) {
            logger.error("Failed to initialize DemiDynamic", e);
            throw new RuntimeException(e);
        }

        server.getEventManager().register(this, new ServerPreConnectHandler(server, config, logger, queueManager, autoStopManager, apiClient, this));
        server.getEventManager().register(this, new DisconnectHandler(server, config, logger, autoStopManager, apiClient));
        server.getEventManager().register(this, new ServerConnectedHandler(server, config, logger, autoStopManager));

        CommandManager commandManager = server.getCommandManager();
        CommandMeta commandMeta = commandManager.metaBuilder("cancel").plugin(this).build();
        SimpleCommand cancelCommand = new CancelCommand(server, config, logger, queueManager, autoStopManager, apiClient, this);
        commandManager.register(commandMeta, cancelCommand);

        logger.info("DemiDynamic plugin ready!");
    }

    @Subscribe
    public void onProxyReload(ProxyReloadEvent event) {
        try {
            config.reloadConfig();
            apiClient.updateConfig(config);
        } catch (Exception e) {
            logger.error("Failed to reload config", e);
        }
        logger.info("Config reloaded!");
    }

    public ApiClient getApiClient() {
        return apiClient;
    }
}
