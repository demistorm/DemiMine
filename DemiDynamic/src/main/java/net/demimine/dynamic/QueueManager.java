package net.demimine.dynamic;

import com.velocitypowered.api.proxy.Player;
import com.velocitypowered.api.proxy.ProxyServer;
import com.velocitypowered.api.proxy.server.RegisteredServer;
import net.kyori.adventure.text.Component;
import net.kyori.adventure.text.minimessage.MiniMessage;
import net.kyori.adventure.text.minimessage.tag.resolver.Placeholder;
import org.slf4j.Logger;

import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicBoolean;

public class QueueManager {
    private final ProxyServer server;
    private final Config config;
    private final Logger logger;
    private final ScheduledExecutorService scheduler;

    private final Map<String, Set<Player>> waitingPlayers = new ConcurrentHashMap<>();
    private final Map<Player, QueueEntry> playerQueues = new ConcurrentHashMap<>();

    public QueueManager(ProxyServer server, Config config, Logger logger) {
        this.server = server;
        this.config = config;
        this.logger = logger;
        this.scheduler = Executors.newScheduledThreadPool(4);
    }

    public boolean addToQueue(Player player, String targetServer) {
        QueueEntry newEntry = new QueueEntry(targetServer);
        QueueEntry existing = playerQueues.putIfAbsent(player, newEntry);
        if (existing != null) {
            return false;
        }
        
        Set<Player> queue = waitingPlayers.computeIfAbsent(targetServer, k -> ConcurrentHashMap.newKeySet());
        queue.add(player);
        logger.info("Added " + player.getUsername() + " to queue for " + targetServer);
        return true;
    }

    public boolean removeFromQueue(Player player) {
        QueueEntry entry = playerQueues.remove(player);
        if (entry != null) {
            Set<Player> queue = waitingPlayers.get(entry.targetServer);
            if (queue != null) {
                queue.remove(player);
                if (queue.isEmpty()) {
                    waitingPlayers.remove(entry.targetServer);
                }
            }
            logger.info("Removed " + player.getUsername() + " from queue for " + entry.targetServer);
            return true;
        }
        return false;
    }

    public Set<Player> getQueue(String serverName) {
        return waitingPlayers.getOrDefault(serverName, Collections.emptySet());
    }

    public void clearQueue(String serverName) {
        Set<Player> queue = waitingPlayers.remove(serverName);
        if (queue != null) {
            for (Player player : queue) {
                playerQueues.remove(player);
            }
        }
    }

    public boolean isInQueue(Player player, String serverName) {
        QueueEntry entry = playerQueues.get(player);
        return entry != null && entry.targetServer.equals(serverName);
    }

    public boolean isInAnyQueue(Player player) {
        return playerQueues.containsKey(player);
    }

    public String getQueuedServer(Player player) {
        QueueEntry entry = playerQueues.get(player);
        return entry != null ? entry.targetServer : null;
    }

    public void startCountdown(Player player, String targetServer, Runnable onComplete) {
        QueueEntry entry = playerQueues.get(player);
        if (entry == null) {
            return;
        }

        if (entry.countdownTask != null) {
            entry.countdownTask.cancel(false);
        }

        MiniMessage mm = MiniMessage.miniMessage();
        Component message = mm.deserialize(config.configVar.messages.teleporting,
            Placeholder.parsed("seconds", "5"));
        player.sendMessage(message);

        final AtomicBoolean completed = new AtomicBoolean(false);
        final int[] secondsLeft = {4};
        entry.countdownTask = scheduler.scheduleAtFixedRate(() -> {
            if (secondsLeft[0] <= 0) {
                if (completed.compareAndSet(false, true)) {
                    entry.countdownTask.cancel(false);
                    scheduler.execute(onComplete);
                }
            } else {
                Component countdownMsg = mm.deserialize(config.configVar.messages.countdown,
                    Placeholder.parsed("seconds", String.valueOf(secondsLeft[0])));
                player.sendMessage(countdownMsg);
                secondsLeft[0]--;
            }
        }, 1, 1, TimeUnit.SECONDS);
    }

    public void cancelCountdown(Player player) {
        QueueEntry entry = playerQueues.get(player);
        if (entry != null && entry.countdownTask != null) {
            entry.countdownTask.cancel(false);
            entry.countdownTask = null;
        }
    }

    public boolean isQueueEmpty(String serverName) {
        Set<Player> queue = waitingPlayers.get(serverName);
        return queue == null || queue.isEmpty();
    }

    public void shutdown() {
        scheduler.shutdown();
        try {
            if (!scheduler.awaitTermination(5, TimeUnit.SECONDS)) {
                scheduler.shutdownNow();
            }
        } catch (InterruptedException e) {
            scheduler.shutdownNow();
            Thread.currentThread().interrupt();
        }
    }

    private static class QueueEntry {
        final String targetServer;
        ScheduledFuture<?> countdownTask;

        QueueEntry(String targetServer) {
            this.targetServer = targetServer;
        }
    }
}
