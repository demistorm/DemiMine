package net.demimine.dynamic;

import com.electronwill.nightconfig.core.file.FileConfig;
import org.slf4j.Logger;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;

public class Config {
    public static ConfigVar configVar;
    private final Logger logger;
    private final Path dataDirectory;
    private FileConfig fileConfig;

    public Config(Logger logger, Path dataDirectory) {
        this.logger = logger;
        this.dataDirectory = dataDirectory;
    }

    public void initConfig() throws IOException {
        Path configPath = dataDirectory.resolve("config.toml");

        if (!Files.exists(configPath)) {
            Files.createDirectories(dataDirectory);
            Files.writeString(configPath, getDefaultConfig());
            logger.info("Created default config.toml");
        }

        fileConfig = FileConfig.of(configPath);
        fileConfig.load();
        loadConfigVars();
        logger.info("Loaded config.toml");
    }

    public void reloadConfig() {
        fileConfig.load();
        loadConfigVars();
    }

    private void loadConfigVars() {
        configVar = new ConfigVar();
        configVar.managerUrl = fileConfig.getOrElse("manager_url", "http://demimine-manager:8080");
        configVar.apiKey = fileConfig.getOrElse("api_key", "");
        configVar.authPermission = fileConfig.getOrElse("auth_permission", "demimine.authenticated");
        configVar.queueServer = fileConfig.getOrElse("queue_server", "nexus");
        configVar.hubServer = fileConfig.getOrElse("hub_server", "nexus");
        configVar.useHubQueue = fileConfig.getOrElse("use_hub_queue", true);
        configVar.checkIntervalSeconds = fileConfig.getOrElse("check_interval_seconds", 1);
        configVar.startTimeoutSeconds = fileConfig.getOrElse("start_timeout_seconds", 120);
        configVar.autoStopTimeoutMinutes = fileConfig.getOrElse("auto_stop_timeout_minutes", 15);
        configVar.messageIntervalSeconds = fileConfig.getOrElse("message_interval_seconds", 5);

        if (fileConfig.contains("messages")) {
            configVar.messages = new Messages();
            configVar.messages.starting = fileConfig.getOrElse("messages.starting", "<yellow>Server is starting...");
            configVar.messages.loading = fileConfig.getOrElse("messages.loading", "<yellow>Loading server...");
            configVar.messages.teleporting = fileConfig.getOrElse("messages.teleporting", "<green>Teleporting in 5 seconds!");
            configVar.messages.countdown = fileConfig.getOrElse("messages.countdown", "<yellow>Teleporting in <seconds>...");
            configVar.messages.cancelQueue = fileConfig.getOrElse("messages.cancel_queue", "<red>Canceled queue for <server>!");
            configVar.messages.cancelDisconnect = fileConfig.getOrElse("messages.cancel_disconnect", "<red>Player canceled the queue!");
        } else {
            configVar.messages = new Messages();
        }
    }

    private String getDefaultConfig() {
        return "# DemiDynamic Configuration for Velocity Proxy\n" +
               "# Automatically starts/stops servers based on player activity\n\n" +
               "manager_url = \"http://demimine-manager:8080\"\n" +
               "api_key = \"\"\n\n" +
               "auth_permission = \"demimine.authenticated\"\n\n" +
               "queue_server = \"nexus\"\n" +
               "hub_server = \"nexus\"\n" +
               "use_hub_queue = true\n\n" +
               "check_interval_seconds = 1\n" +
               "start_timeout_seconds = 120\n" +
               "auto_stop_timeout_minutes = 15\n" +
               "message_interval_seconds = 5\n\n" +
                "[messages]\n" +
                "starting = \"<yellow>Server is starting...\"\n" +
                "loading = \"<yellow>Loading server...\"\n" +
                "teleporting = \"<green>Teleporting in 5 seconds!\"\n" +
                "countdown = \"<yellow>Teleporting in <seconds>...\"\n" +
                "cancel_queue = \"<red>Canceled queue for <server>!\"\n" +
                "cancel_disconnect = \"<red>Player canceled the queue!\"\n\n" +
               "configVersion = 1\n";
    }

    public static class ConfigVar {
        public String managerUrl;
        public String apiKey;
        public String authPermission;
        public String queueServer;
        public String hubServer;
        public boolean useHubQueue;
        public int checkIntervalSeconds;
        public int startTimeoutSeconds;
        public int autoStopTimeoutMinutes;
        public int messageIntervalSeconds;
        public Messages messages;
    }

    public static class Messages {
        public String starting;
        public String loading;
        public String teleporting;
        public String countdown;
        public String cancelQueue;
        public String cancelDisconnect;
    }
}
