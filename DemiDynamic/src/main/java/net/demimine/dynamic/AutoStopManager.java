package net.demimine.dynamic;

import com.velocitypowered.api.proxy.Player;
import com.velocitypowered.api.proxy.ProxyServer;
import org.slf4j.Logger;

import java.util.*;
import java.util.concurrent.*;

public class AutoStopManager {
    private final ProxyServer server;
    private final Config config;
    private final Logger logger;
    private final QueueManager queueManager;
    private final ScheduledExecutorService scheduler;
    private final DemiDynamic plugin;
    private final ApiClient apiClient;

    private final Map<String, Set<UUID>> serverPlayers = new ConcurrentHashMap<>();
    private final Map<String, ScheduledFuture<?>> stopTimers = new ConcurrentHashMap<>();

    public AutoStopManager(ProxyServer server, Config config, Logger logger, QueueManager queueManager, DemiDynamic plugin, ApiClient apiClient) {
        this.server = server;
        this.config = config;
        this.logger = logger;
        this.queueManager = queueManager;
        this.plugin = plugin;
        this.apiClient = apiClient;
        this.scheduler = Executors.newScheduledThreadPool(4);
    }

    public void addPlayerToServer(UUID playerId, String serverName) {
        Set<UUID> players = serverPlayers.computeIfAbsent(serverName, k -> ConcurrentHashMap.newKeySet());
        players.add(playerId);
        cancelStopTimer(serverName);
        logger.debug("Added " + playerId + " to " + serverName + " (now " + players.size() + " players)");
    }

    public void removePlayerFromServer(UUID playerId, String serverName) {
        Set<UUID> players = serverPlayers.get(serverName);
        if (players != null) {
            if (players.remove(playerId)) {
                logger.debug("Removed " + playerId + " from " + serverName + " (now " + players.size() + " players)");
            } else {
                logger.warn("Failed to remove " + playerId + " from " + serverName + " (not found in set, current players: " + players + ")");
            }
        } else {
            logger.warn("No player set for server " + serverName + " when trying to remove " + playerId);
        }
    }

    public boolean isServerEmpty(String serverName) {
        Set<UUID> players = serverPlayers.get(serverName);
        return players == null || players.isEmpty();
    }

    public void scheduleStopTimer(String serverName) {
        cancelStopTimer(serverName);

        logger.info("Scheduling auto-stop for " + serverName + " in " + config.configVar.autoStopTimeoutMinutes + " minutes");

        int warningDelay = Math.max(0, config.configVar.autoStopTimeoutMinutes - 1);
        scheduler.schedule(() -> {
            if (stopTimers.containsKey(serverName) && isServerEmpty(serverName)) {
                logger.info("Auto-stop warning: " + serverName + " will shut down in 1 minute");
            }
        }, warningDelay, TimeUnit.MINUTES);

        ScheduledFuture<?> task = scheduler.schedule(() -> {
            if (isServerEmpty(serverName)) {
                logger.info("Auto-stopping " + serverName);
                apiClient.stopServerByName(serverName);
            } else {
                logger.info("Auto-stop canceled for " + serverName + " - players present");
            }
            stopTimers.remove(serverName);
        }, config.configVar.autoStopTimeoutMinutes, TimeUnit.MINUTES);

        stopTimers.put(serverName, task);
    }

    public void cancelStopTimer(String serverName) {
        ScheduledFuture<?> task = stopTimers.remove(serverName);
        if (task != null) {
            task.cancel(false);
            logger.info("Canceled stop timer for " + serverName);
        }
    }

    public boolean hasPendingStopTimer(String serverName) {
        return stopTimers.containsKey(serverName);
    }

    public List<String> getServersWithPendingStopTimers() {
        List<String> servers = new ArrayList<>(stopTimers.keySet());
        return servers;
    }

    public void cancelStopTimerForStart(String serverName, ApiClient apiClient) {
        ScheduledFuture<?> task = stopTimers.remove(serverName);
        if (task != null) {
            task.cancel(false);
            logger.info("Canceled stop timer for " + serverName + " - server starting");
        }
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
}
