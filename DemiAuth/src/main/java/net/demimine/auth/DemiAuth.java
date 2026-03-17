package net.demimine.auth;

import com.google.inject.Inject;
import com.velocitypowered.api.command.CommandManager;
import com.velocitypowered.api.command.CommandMeta;
import com.velocitypowered.api.command.SimpleCommand;
import com.velocitypowered.api.event.Subscribe;
import com.velocitypowered.api.event.proxy.ProxyInitializeEvent;
import com.velocitypowered.api.event.proxy.ProxyReloadEvent;
import com.velocitypowered.api.plugin.Dependency;
import com.velocitypowered.api.plugin.Plugin;
import com.velocitypowered.api.plugin.annotation.DataDirectory;
import com.velocitypowered.api.proxy.ProxyServer;
import net.demimine.auth.common.Config;
import net.demimine.auth.common.BypassList;
import net.demimine.auth.velocity.AdminCommand;
import net.demimine.auth.velocity.PlayerConnection;
import org.slf4j.Logger;

import java.io.IOException;
import java.nio.file.Path;

@Plugin(
    id = "demiauth",
    name = "DemiAuth",
    version = BuildConstants.VERSION,
    authors = {"DemiMine Team"},
    dependencies = {
        @Dependency(id = "luckperms", optional = true)
    }
)
public class DemiAuth {
    private final ProxyServer server;
    private final Logger logger;
    private final Path dataDirectory;
    private final Config config;

    @Inject
    public DemiAuth(ProxyServer server, Logger logger, @DataDirectory Path dataDirectory) {
        this.server = server;
        this.logger = logger;
        this.dataDirectory = dataDirectory;
        this.config = new Config(logger, dataDirectory);
    }

    @Subscribe
    public void onProxyInitialization(ProxyInitializeEvent event) {
        try {
            logger.info("Initializing DemiAuth plugin...");
            config.initConfig();
            new BypassList(logger, dataDirectory);
            BypassList.loadBypassList();
        } catch (Exception e) {
            logger.error("Failed to initialize DemiAuth", e);
            throw new RuntimeException(e);
        }

        server.getEventManager().register(this, new PlayerConnection(server, this, logger));

        CommandManager commandManager = server.getCommandManager();
        CommandMeta commandMetaAdmin = commandManager.metaBuilder("demiauth").plugin(this).build();

        SimpleCommand adminCommand = new AdminCommand(this, server, logger, config);
        commandManager.register(commandMetaAdmin, adminCommand);

        if (server.getPluginManager().isLoaded("luckperms")) {
            logger.info("LuckPerms found! Bypass permissions will be granted automatically.");
        } else {
            if (Config.configVar.pluginGrantsBypass && Config.configVar.oneTimeLogin) {
                logger.warn("pluginGrantsBypass is enabled but LuckPerms is not found. Bypass permissions must be granted manually.");
            }
        }

        logger.info("DemiAuth plugin ready!");
    }

    @Subscribe
    public void onProxyReload(ProxyReloadEvent event) {
        try {
           config.reloadConfig();
           BypassList.loadBypassList();
        } catch (Exception e) {
            logger.error("Failed to reload config", e);
        }
        logger.info("Config reloaded!");
    }
}
