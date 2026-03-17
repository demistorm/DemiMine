package net.demimine.auth.velocity;
import java.util.Optional;

import java.util.Optional;
import com.velocitypowered.api.command.CommandSource;
import java.util.Optional;
import com.velocitypowered.api.command.SimpleCommand;
import java.util.Optional;
import com.velocitypowered.api.proxy.ConsoleCommandSource;
import java.util.Optional;
import com.velocitypowered.api.proxy.Player;
import java.util.Optional;
import com.velocitypowered.api.proxy.ProxyServer;
import java.util.Optional;
import net.demimine.auth.common.Config;
import java.util.Optional;
import net.demimine.auth.common.BypassList;
import java.util.Optional;
import net.kyori.adventure.text.Component;
import java.util.Optional;
import net.kyori.adventure.text.format.NamedTextColor;
import java.util.Optional;
import org.slf4j.Logger;

import java.util.UUID;

public class AdminCommand implements SimpleCommand {
    private final Object plugin;
    private final ProxyServer server;
    private final Logger logger;
    private final Config config;

    public AdminCommand(Object plugin, ProxyServer server, Logger logger, Config config) {
        this.plugin = plugin;
        this.server = server;
        this.logger = logger;
        this.config = config;
    }

    @Override
    public void execute(Invocation invocation) {
        CommandSource source = invocation.source();
        String[] args = invocation.arguments();

        if (args.length == 0) {
            source.sendMessage(Component.text("Usage: /demiauth <reload|toggle|add|remove|list>", NamedTextColor.RED));
            return;
        }

        switch (args[0].toLowerCase()) {
            case "reload":
                if (!hasPermission(source)) return;
                config.reloadConfig();
                BypassList.loadBypassList();
                source.sendMessage(Component.text("Config reloaded!", NamedTextColor.GREEN));
                break;

            case "toggle":
                if (!hasPermission(source)) return;
                config.togglePlugin();
                source.sendMessage(Component.text("Plugin " + (Config.configVar.pluginEnabled ? "enabled" : "disabled"), NamedTextColor.GREEN));
                break;

            case "add":
                if (!hasPermission(source)) return;
                if (args.length < 2) {
                    source.sendMessage(Component.text("Usage: /demiauth add <uuid|player>", NamedTextColor.RED));
                    return;
                }
                addPlayerToBypassList(args[1], source);
                break;

            case "remove":
                if (!hasPermission(source)) return;
                if (args.length < 2) {
                    source.sendMessage(Component.text("Usage: /demiauth remove <uuid|player>", NamedTextColor.RED));
                    return;
                }
                removePlayerFromBypassList(args[1], source);
                break;

            case "list":
                if (!hasPermission(source)) return;
                listBypassPlayers(source);
                break;

            default:
                source.sendMessage(Component.text("Unknown subcommand. Usage: /demiauth <reload|toggle|add|remove|list>", NamedTextColor.RED));
        }
    }

    private boolean hasPermission(CommandSource source) {
        if (source instanceof ConsoleCommandSource) {
            return true;
        }
        if (source instanceof Player player && player.hasPermission("demiauth.admin")) {
            return true;
        }
        source.sendMessage(Component.text("You don't have permission to use this command.", NamedTextColor.RED));
        return false;
    }

    private void addPlayerToBypassList(String identifier, CommandSource source) {
        UUID uuid;
        if (BypassList.validUUID(identifier)) {
            uuid = UUID.fromString(identifier);
        } else {
            if (source instanceof Player player) {
                Optional<Player> targetPlayer = server.getPlayer(identifier);
                if (targetPlayer.isPresent()) {
                    uuid = targetPlayer.get().getUniqueId();
                } else {
                    source.sendMessage(Component.text("Player not found.", NamedTextColor.RED));
                    return;
                }
            } else {
                source.sendMessage(Component.text("Please use a valid UUID.", NamedTextColor.RED));
                return;
            }
        }
        BypassList.addBypassEntry(uuid.toString());
        source.sendMessage(Component.text("Added " + identifier + " to bypass list.", NamedTextColor.GREEN));
    }

    private void removePlayerFromBypassList(String identifier, CommandSource source) {
        UUID uuid;
        if (BypassList.validUUID(identifier)) {
            uuid = UUID.fromString(identifier);
        } else {
            if (source instanceof Player player) {
                Optional<Player> targetPlayer = server.getPlayer(identifier);
                if (targetPlayer.isPresent()) {
                    uuid = targetPlayer.get().getUniqueId();
                } else {
                    source.sendMessage(Component.text("Player not found.", NamedTextColor.RED));
                    return;
                }
            } else {
                source.sendMessage(Component.text("Please use a valid UUID.", NamedTextColor.RED));
                return;
            }
        }
        BypassList.removeBypassEntry(uuid.toString());
        source.sendMessage(Component.text("Removed " + identifier + " from bypass list.", NamedTextColor.GREEN));
    }

    private void listBypassPlayers(CommandSource source) {
        if (BypassList.bypassList.isEmpty()) {
            source.sendMessage(Component.text("No players in bypass list.", NamedTextColor.YELLOW));
            return;
        }
        source.sendMessage(Component.text("Bypass List:", NamedTextColor.GOLD));
        for (int i = 0; i < BypassList.bypassList.size(); i++) {
            source.sendMessage(Component.text("  " + (i + 1) + ". " + BypassList.bypassList.get(i), NamedTextColor.WHITE));
        }
    }
}
