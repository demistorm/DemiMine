package net.demimine.auth.common;

import com.electronwill.nightconfig.core.ConfigSpec;
import com.electronwill.nightconfig.core.file.CommentedFileConfig;
import org.slf4j.Logger;

import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Collections;

public class Config {
    protected final Logger logger;
    protected final Path dataDirectory;
    protected final ConfigSpec defaultSpec;

    public Config(Logger logger, Path dataDirectory) {
        this.logger = logger;
        this.dataDirectory = dataDirectory;
        this.defaultSpec = defaultConfig();
        com.electronwill.nightconfig.core.Config.setInsertionOrderPreserved(true);
    }

    void debugMessage(String message) {
        if (configVar.debugMode) {
            logger.info("[Debug] {}", message);
        }
    }

    public static class configVar {
        public static String loginServer;
        public static String hubServer;
        public static ArrayList<String> serverPassword;
        public static Boolean oneTimeLogin;
        public static String bypassNode;
        public static Boolean pluginGrantsBypass;
        public static String bypassMethod;
        public static String bypassGroup;
        public static String kickMessage;
        public static Integer kickTimeout;
        public static String wrongPassword;
        public static String welcomeMessage;
        public static Integer configVersion;
        public static Boolean pluginEnabled;
        public static String bypasserLoginExitMethod;
        public static Boolean debugMode;
    }

    public ConfigSpec defaultConfig() {
        ConfigSpec spec = new ConfigSpec();

        spec.define("core.loginServer", "login");
        spec.define("core.hubServer", "survival");
        spec.define("core.serverPassword", new ArrayList<>(Collections.singleton("changeme")));
        spec.define("core.oneTimeLogin", true);
        spec.define("core.bypass.pluginGrantsBypass", true);
        spec.define("core.bypass.bypassNode", "demimine.authenticated");
        spec.define("core.bypass.bypasserLoginExitMethod", "auto");
        spec.define("core.bypass.methods.bypassMethod", "user");
        spec.define("core.bypass.methods.bypassGroup", "default");
        spec.define("core.kick.kickMessage", "Too many failed attempts");
        spec.define("core.kick.kickTimeout", 120);
        spec.define("messages.wrongPassword", "Wrong password. Please try again.");
        spec.define("messages.welcomeMessage", "Type the password in chat to continue");
        spec.define("misc.configVersion", 1);
        spec.define("misc.pluginEnabled", true);
        spec.define("misc.debugMode", false);

        return spec;
    }

    public Object isConfigCorrect(String key, Object value) {
        return defaultSpec.correct(key, value);
    }

    public boolean validateConfig(Path configFile) {
        CommentedFileConfig config = CommentedFileConfig.of(configFile);
        config.load();
        if (config.get("misc.configVersion") instanceof String) {
            config.set("misc.configVersion", 1);
            logger.warn("Config version was a string. Set to 1 for migration.");
            config.save();
            config.close();
            return true;
        }
        switch ((Integer) config.get("misc.configVersion")) {
            case null -> {
                logger.warn("Config version is missing. Pre-migration required.");
                return true;
            }
            default -> {
            }
        }
        defaultSpec.correct(config);
        config.save();
        config.close();
        return false;
    }

    public void initConfig() throws IOException {
        if (Files.notExists(dataDirectory)) {
            logger.info("Data directory does not exist. Creating it.");
            try {
                Files.createDirectory(dataDirectory);
            } catch (IOException e) {
                throw new RuntimeException(e);
            }
        }
        final Path configFile = dataDirectory.resolve("config.toml");
        if (Files.notExists(configFile)) {
            try (InputStream stream = this.getClass().getClassLoader().getResourceAsStream("config.toml")) {
                Files.copy(stream, configFile);
            } catch (IOException e) {
                throw new RuntimeException(e);
            }
        }

        if (Files.size(configFile) == 0) {
            try (InputStream stream = this.getClass().getClassLoader().getResourceAsStream("config.toml")) {
                Files.copy(stream, configFile);
            } catch (IOException e) {
                throw new RuntimeException(e);
            }
        }
        boolean preMigrationRequired = validateConfig(configFile);
        if (preMigrationRequired) {
            initConfig();
            return;
        }
        getTomlConfig(configFile);
        if (configVar.configVersion != 1) {
            initConfig();
        }
    }

    public void getTomlConfig(Path configFile) {
        if (Files.notExists(dataDirectory)) {
            try {
                Files.createDirectory(dataDirectory);
            } catch (IOException e) {
                throw new RuntimeException(e);
            }
        }
        CommentedFileConfig config = CommentedFileConfig.of(configFile);
        config.load();
        configVar.loginServer = config.get("core.loginServer");
        configVar.hubServer = config.get("core.hubServer");
        configVar.serverPassword = config.get("core.serverPassword");
        configVar.oneTimeLogin = config.get("core.oneTimeLogin");
        configVar.pluginGrantsBypass = config.get("core.bypass.pluginGrantsBypass");
        configVar.bypassNode = config.get("core.bypass.bypassNode");
        configVar.bypassMethod = config.get("core.bypass.methods.bypassMethod");
        configVar.bypassGroup = config.get("core.bypass.methods.bypassGroup");
        configVar.kickMessage = config.get("core.kick.kickMessage");
        configVar.kickTimeout = config.get("core.kick.kickTimeout");
        configVar.wrongPassword = config.get("messages.wrongPassword");
        configVar.welcomeMessage = config.get("messages.welcomeMessage");
        configVar.configVersion = config.get("misc.configVersion");
        configVar.pluginEnabled = config.get("misc.pluginEnabled");
        configVar.bypasserLoginExitMethod = config.get("core.bypass.bypasserLoginExitMethod");
        configVar.debugMode = config.get("misc.debugMode");
        config.close();
    }

    public void migrateConfigVersion() {
        Path templateConfigFile = null;
        try {
            templateConfigFile = Files.createTempFile(dataDirectory, "configTemp", ".toml");
        } catch (IOException e) {
            throw new RuntimeException(e);
        }
        InputStream templateStream = this.getClass().getClassLoader().getResourceAsStream("config.toml");
        try {
            Files.copy(templateStream, templateConfigFile, java.nio.file.StandardCopyOption.REPLACE_EXISTING);
        } catch (IOException e) {
            throw new RuntimeException(e);
        }
        Path configFile = dataDirectory.resolve("config.toml");

        CommentedFileConfig templateConfig = CommentedFileConfig.of(templateConfigFile);
        CommentedFileConfig config = CommentedFileConfig.of(configFile);
        templateConfig.load();
        config.load();

        templateConfig.set("core.loginServer", isConfigCorrect("core.loginServer", config.get("core.loginServer")));
        templateConfig.set("core.hubServer", isConfigCorrect("core.hubServer", config.get("core.hubServer")));
        templateConfig.set("core.serverPassword", isConfigCorrect("core.serverPassword", config.get("core.serverPassword")));
        templateConfig.set("core.oneTimeLogin", isConfigCorrect("core.oneTimeLogin", config.get("core.oneTimeLogin")));
        templateConfig.set("core.bypass.pluginGrantsBypass", isConfigCorrect("core.bypass.pluginGrantsBypass", config.get("core.bypass.pluginGrantsBypass")));
        templateConfig.set("core.bypass.bypassNode", isConfigCorrect("core.bypass.bypassNode", config.get("core.bypass.bypassNode")));
        templateConfig.set("core.bypass.methods.bypassMethod", isConfigCorrect("core.bypass.methods.bypassMethod", config.get("core.bypass.methods.bypassMethod")));
        templateConfig.set("core.bypass.methods.bypassGroup", isConfigCorrect("core.bypass.methods.bypassGroup", config.get("core.bypass.methods.bypassGroup")));
        templateConfig.set("core.kick.kickMessage", isConfigCorrect("core.kick.kickMessage", config.get("core.kick.kickMessage")));
        templateConfig.set("core.kick.kickTimeout", isConfigCorrect("core.kick.kickTimeout", config.get("core.kick.kickTimeout")));
        templateConfig.set("messages.wrongPassword", isConfigCorrect("messages.wrongPassword", config.get("messages.wrongPassword")));
        templateConfig.set("messages.welcomeMessage", isConfigCorrect("messages.welcomeMessage", config.get("messages.welcomeMessage")));
        templateConfig.set("misc.pluginEnabled", isConfigCorrect("misc.pluginEnabled", config.get("misc.pluginEnabled")));
        templateConfig.set("core.bypass.bypasserLoginExitMethod", isConfigCorrect("core.bypass.bypasserLoginExitMethod", config.get("core.bypass.bypasserLoginExitMethod")));
        templateConfig.set("misc.debugMode", isConfigCorrect("misc.debugMode", config.get("misc.debugMode")));

        templateConfig.save();
        templateConfig.close();
        config.close();
        try {
            Files.copy(templateConfigFile, configFile, java.nio.file.StandardCopyOption.REPLACE_EXISTING);
            Files.delete(templateConfigFile);
        } catch (IOException e) {
            throw new RuntimeException(e);
        }
    }

    public void setConfigValue(String key, Object value) {
        CommentedFileConfig config = CommentedFileConfig.of(dataDirectory.resolve("config.toml"));
        config.load();
        config.set(key, isConfigCorrect(key, value));
        config.save();
        config.close();
        try {
            initConfig();
        } catch (IOException e) {
            throw new RuntimeException(e);
        }
    }

    public void reloadConfig() {
        try {
            initConfig();
        } catch (IOException e) {
            throw new RuntimeException(e);
        }
    }

    public void togglePlugin() {
        if (configVar.pluginEnabled) {
            setConfigValue("misc.pluginEnabled", false);
        } else {
            setConfigValue("misc.pluginEnabled", true);
        }
    }
}
