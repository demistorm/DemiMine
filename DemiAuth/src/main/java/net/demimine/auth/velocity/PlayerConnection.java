package net.demimine.auth.velocity;

import com.velocitypowered.api.event.Subscribe;
import com.velocitypowered.api.event.player.KickedFromServerEvent;
import com.velocitypowered.api.event.player.PlayerChooseInitialServerEvent;
import com.velocitypowered.api.event.player.PlayerChatEvent;
import com.velocitypowered.api.event.player.ServerConnectedEvent;
import com.velocitypowered.api.event.player.ServerPreConnectEvent;
import com.velocitypowered.api.proxy.Player;
import com.velocitypowered.api.proxy.ProxyServer;
import com.velocitypowered.api.proxy.server.RegisteredServer;
import com.velocitypowered.api.scheduler.ScheduledTask;
import net.kyori.adventure.text.Component;
import net.kyori.adventure.text.format.NamedTextColor;
import net.kyori.adventure.text.serializer.plain.PlainTextComponentSerializer;
import net.demimine.auth.common.Config.configVar;
import net.demimine.auth.common.BypassList;
import net.luckperms.api.LuckPerms;
import net.luckperms.api.LuckPermsProvider;
import net.luckperms.api.node.Node;
import org.slf4j.Logger;

import java.util.Collection;
import java.util.HashMap;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.TimeUnit;

public class PlayerConnection {
    private final ProxyServer server;
    private final Object plugin;
    private final Logger logger;
    static Map<Integer, String> hashScheduledPlayerTask = new HashMap<>();
    static Map<UUID, Integer> playerAttempts = new HashMap<>();

    public PlayerConnection(ProxyServer server, Object plugin, Logger logger) {
        this.server = server;
        this.plugin = plugin;
        this.logger = logger;
    }

    @Subscribe
    public void onPlayerChat(PlayerChatEvent event) {
        Player player = event.getPlayer();
        
        if (!player.getCurrentServer().isPresent()) {
            return;
        }
        
        boolean isLoginServer = player.getCurrentServer().get().getServerInfo().getName().equals(configVar.loginServer);
        if (!isLoginServer || !configVar.pluginEnabled) {
            return;
        }
        
        if (player.hasPermission(configVar.bypassNode)) {
            return;
        }

        String password = event.getMessage();
        if (isValidPassword(password)) {
            if (configVar.oneTimeLogin && configVar.pluginGrantsBypass && server.getPluginManager().isLoaded("luckperms")) {
                grantBypassPermission(player);
            }
            server.getScheduler().buildTask(plugin, () -> transferToHub(player))
                .delay(500, TimeUnit.MILLISECONDS)
                .schedule();
            playerAttempts.remove(player.getUniqueId());
        } else {
            int attempts = playerAttempts.getOrDefault(player.getUniqueId(), 0) + 1;
            playerAttempts.put(player.getUniqueId(), attempts);
            
            player.sendMessage(Component.text(configVar.wrongPassword, NamedTextColor.RED));
            
            if (attempts >= 3) {
                player.disconnect(Component.text(configVar.kickMessage));
                playerAttempts.remove(player.getUniqueId());
            }
        }
    }
    
    private boolean isValidPassword(String password) {
        for (String validPassword : configVar.serverPassword) {
            if (password.equals(validPassword)) {
                return true;
            }
        }
        return false;
    }
    
    private void grantBypassPermission(Player player) {
        LuckPerms lpApi = LuckPermsProvider.get();
        
        if (configVar.bypassMethod.equalsIgnoreCase("user")) {
            lpApi.getUserManager().modifyUser(player.getUniqueId(), user -> {
                user.data().add(Node.builder(configVar.bypassNode).build());
            });
        } else if (configVar.bypassMethod.equalsIgnoreCase("group")) {
            lpApi.getUserManager().modifyUser(player.getUniqueId(), user -> {
                user.data().add(Node.builder("group." + configVar.bypassGroup).build());
            });
        }
    }
    
    private void transferToHub(Player player) {
        Optional<RegisteredServer> connectToServer = server.getServer(configVar.hubServer);
        if (connectToServer.isEmpty()) {
            player.sendMessage(Component.text("Hub server not found. Please contact an admin.", NamedTextColor.RED));
            logger.error("Hub server '{}' not found in proxy configuration", configVar.hubServer);
            return;
        }
        player.createConnectionRequest(connectToServer.get()).connectWithIndication();
        logger.info("Player {} has authenticated", player.getUsername());
    }

    @Subscribe
    public void onPlayerJoin(PlayerChooseInitialServerEvent event) {
        Player player = event.getPlayer();
 
        if ((!configVar.oneTimeLogin || !(player.hasPermission(configVar.bypassNode) || BypassList.inBypassList(player.getUniqueId())) ) && configVar.pluginEnabled) {
            Optional<RegisteredServer> connectToServer = server.getServer(configVar.loginServer);
            try {
                connectToServer.get().ping().get();
                event.setInitialServer(connectToServer.get());
                player.sendMessage(Component.text(configVar.welcomeMessage, NamedTextColor.GREEN));
            } catch (InterruptedException | ExecutionException e) {
                event.setInitialServer(null);
                logger.error("Error pinging login server: " + e.getMessage());
                logger.error("Make sure the login server is online");
            }
        }
    }
 
    @Subscribe
    public void onBypassPlayerConnectToLoginServer(ServerPreConnectEvent event) {
        Player player = event.getPlayer();
        
        if (event.getResult().getServer().isEmpty()) {
            return;
        }
        
        boolean isLoginServer = event.getResult().getServer().get().getServerInfo().getName().equals(configVar.loginServer);
 
        if (isLoginServer && player.hasPermission(configVar.bypassNode) && configVar.bypasserLoginExitMethod.equals("auto")) {
            Optional<RegisteredServer> connectToServer = server.getServer(configVar.hubServer);
            
            if (connectToServer.isEmpty()) {
                player.sendMessage(Component.text("Hub server not found. Please contact an admin.", NamedTextColor.RED));
                logger.error("Hub server '{}' not found in proxy configuration", configVar.hubServer);
                return;
            }
            
            event.setResult(ServerPreConnectEvent.ServerResult.allowed(connectToServer.get()));
            player.sendMessage(Component.text("Transferring you to the hub server...", NamedTextColor.GREEN));
        } else if (isLoginServer && player.hasPermission(configVar.bypassNode) && configVar.bypasserLoginExitMethod.equals("deny-entry")) {
            event.setResult(ServerPreConnectEvent.ServerResult.denied());
            player.sendMessage(Component.text("Cannot connect to the login server, try connecting to a different server", NamedTextColor.RED));
        }
    }
 
    @Subscribe
    public void onPlayerJoinLoginServer(ServerConnectedEvent event) {
        Player player = event.getPlayer();
        RegisteredServer connectedServer = event.getServer();
 
        if (connectedServer.getServerInfo().getName().equals(configVar.loginServer) && !player.hasPermission(configVar.bypassNode)) {
            ScheduledTask task = server.getScheduler().buildTask(plugin, () -> player.disconnect(Component.text(configVar.kickMessage))).delay(configVar.kickTimeout, TimeUnit.SECONDS).schedule();
            hashScheduledPlayerTask.put(player.getUniqueId().hashCode(), String.valueOf(task.toString().hashCode()));
        }
        if (connectedServer.getServerInfo().getName().equals(configVar.loginServer) && player.hasPermission(configVar.bypassNode)) {
            player.sendMessage(Component.text("Type the password in chat to continue", NamedTextColor.YELLOW));
        }
    }
 
    @Subscribe
    public void onPlayerLeaveLoginServer(ServerConnectedEvent event) {
        Player player = event.getPlayer();
        Optional<RegisteredServer> transferServer = event.getPreviousServer();
        RegisteredServer connectedServer = event.getServer();
        if (transferServer.isEmpty()) {
            return;
        }
        if (connectedServer.getServerInfo().getName().equals(configVar.hubServer) && Objects.equals(transferServer.get().getServerInfo().getName(), configVar.loginServer)) {
            server.getScheduler().buildTask(plugin, () -> {
                Collection<ScheduledTask> tasks = server.getScheduler().tasksByPlugin(plugin);
                for (ScheduledTask cancelTask : tasks) {
                    if (hashScheduledPlayerTask.containsKey(player.getUniqueId().hashCode())) {
                        if (!cancelTask.status().toString().equals("FINISHED")) {
                            cancelTask.cancel();
                        }
                    }
                }
            }).delay(1, TimeUnit.SECONDS).schedule();
        }
    }
 
    @Subscribe
    public void onPlayerKick(KickedFromServerEvent event) {
        String serverPlayerKicked = event.getServer().getServerInfo().getName();
        String kickReason = PlainTextComponentSerializer.plainText().serialize(event.getServerKickReason().orElse(Component.text("No reason provided")));
 
        if (serverPlayerKicked.equals(configVar.loginServer) && kickReason.equals("Unsupported client version")) {
            event.setResult(KickedFromServerEvent.DisconnectPlayer.create(Component.text("Unsupported client version. Please contact an admin.", NamedTextColor.RED)));
            logger.error("A player attempted to connect to the login server with version {} and failed. Please check if the login server is updated.", event.getPlayer().getProtocolVersion());
        }
    }
 
    @Subscribe
    public void onPlayerRedirect(KickedFromServerEvent event) {
        String serverPlayerKicked = event.getServer().getServerInfo().getName();
 
        if (serverPlayerKicked.equals(configVar.loginServer) && event.getResult().toString().contains("RedirectPlayer")) {
            event.setResult(KickedFromServerEvent.DisconnectPlayer.create(Component.text("There was a problem connecting to the server. Please try again later or contact an admin.", NamedTextColor.RED)));
            logger.warn("Player {} was redirected to another server, this usually means they were unable to connect to the login server for some reason. Check if the login server is updated or online.", event.getPlayer().getUsername());
        }
    }
}
