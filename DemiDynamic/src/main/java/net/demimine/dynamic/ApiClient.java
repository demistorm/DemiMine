package net.demimine.dynamic;

import com.google.gson.Gson;
import com.google.gson.JsonObject;
import com.google.gson.reflect.TypeToken;

import org.slf4j.Logger;

import java.io.IOException;
import java.lang.reflect.Type;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpRequest.BodyPublishers;
import java.net.http.HttpResponse;
import java.net.http.HttpResponse.BodyHandlers;
import java.time.Duration;
import java.util.List;

public class ApiClient {
    private final HttpClient httpClient;
    private final Gson gson;
    private final Logger logger;
    private Config config;

    public static class AutoShutdownServer {
        public long id;
        public String name;
        public int ram_mb;
    }

    public static class ServerStatus {
        public String name;
        public String status;
        public int player_count;
    }

    public static class CanStartResponse {
        public boolean can_start;
        public long current_usage_mb;
        public long projected_usage_mb;
        public int max_mb;
        public String reason;
    }

    public ApiClient(Config config, Logger logger) {
        this.config = config;
        this.logger = logger;
        this.gson = new Gson();
        this.httpClient = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(30))
            .build();
    }

    public void updateConfig(Config config) {
        this.config = config;
    }

    public boolean startServer(String serverName) {
        String url = config.configVar.managerUrl + "/api/servers/" + serverName + "/start-by-name";
        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create(url))
            .POST(BodyPublishers.ofString(""))
            .header("Authorization", "Bearer " + config.configVar.apiKey)
            .timeout(Duration.ofSeconds(30))
            .build();

        try {
            HttpResponse<String> response = httpClient.send(request, BodyHandlers.ofString());
            if (response.statusCode() >= 200 && response.statusCode() < 300) {
                logger.info("Started server: " + serverName);
                return true;
            } else {
                logger.error("Failed to start server " + serverName + ": " + response.statusCode());
                return false;
            }
        } catch (IOException | InterruptedException e) {
            logger.error("Error starting server " + serverName, e);
            Thread.currentThread().interrupt();
            return false;
        }
    }

    public boolean stopServer(String serverId) {
        String url = config.configVar.managerUrl + "/api/servers/" + serverId + "/stop";
        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create(url))
            .POST(BodyPublishers.ofString(""))
            .header("Authorization", "Bearer " + config.configVar.apiKey)
            .timeout(Duration.ofSeconds(30))
            .build();

        try {
            HttpResponse<String> response = httpClient.send(request, BodyHandlers.ofString());
            if (response.statusCode() >= 200 && response.statusCode() < 300) {
                logger.info("Stopped server: " + serverId);
                return true;
            } else {
                logger.error("Failed to stop server " + serverId + ": " + response.statusCode());
                return false;
            }
        } catch (IOException | InterruptedException e) {
            logger.error("Error stopping server " + serverId, e);
            Thread.currentThread().interrupt();
            return false;
        }
    }

    public boolean stopServerByName(String serverName) {
        String url = config.configVar.managerUrl + "/api/servers/" + serverName + "/stop-by-name";
        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create(url))
            .POST(BodyPublishers.ofString(""))
            .header("Authorization", "Bearer " + config.configVar.apiKey)
            .timeout(Duration.ofSeconds(30))
            .build();

        try {
            HttpResponse<String> response = httpClient.send(request, BodyHandlers.ofString());
            if (response.statusCode() >= 200 && response.statusCode() < 300) {
                logger.info("Stopped server by name: " + serverName);
                return true;
            } else {
                logger.error("Failed to stop server " + serverName + ": " + response.statusCode());
                return false;
            }
        } catch (IOException | InterruptedException e) {
            logger.error("Error stopping server " + serverName, e);
            Thread.currentThread().interrupt();
            return false;
        }
    }

    public boolean reportPlayerJoin(String uuid, String name, String serverName) {
        String url = config.configVar.managerUrl + "/api/players/join";
        JsonObject requestBody = new JsonObject();
        requestBody.addProperty("uuid", uuid);
        requestBody.addProperty("name", name);
        requestBody.addProperty("server_name", serverName);

        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create(url))
            .POST(BodyPublishers.ofString(requestBody.toString()))
            .header("Authorization", "Bearer " + config.configVar.apiKey)
            .header("Content-Type", "application/json")
            .timeout(Duration.ofSeconds(30))
            .build();

        try {
            HttpResponse<String> response = httpClient.send(request, BodyHandlers.ofString());
            if (response.statusCode() < 200 || response.statusCode() >= 300) {
                logger.warn("Failed to report player join: " + response.statusCode());
                return false;
            }
            return true;
        } catch (IOException | InterruptedException e) {
            logger.warn("Error reporting player join", e);
            Thread.currentThread().interrupt();
            return false;
        }
    }

    public boolean reportPlayerLeave(String uuid, String serverName) {
        String url = config.configVar.managerUrl + "/api/players/leave";
        JsonObject requestBody = new JsonObject();
        requestBody.addProperty("uuid", uuid);
        if (serverName != null) {
            requestBody.addProperty("server_name", serverName);
        }

        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create(url))
            .POST(BodyPublishers.ofString(requestBody.toString()))
            .header("Authorization", "Bearer " + config.configVar.apiKey)
            .header("Content-Type", "application/json")
            .timeout(Duration.ofSeconds(30))
            .build();

        try {
            HttpResponse<String> response = httpClient.send(request, BodyHandlers.ofString());
            if (response.statusCode() < 200 || response.statusCode() >= 300) {
                logger.warn("Failed to report player leave: " + response.statusCode());
                return false;
            }
            return true;
        } catch (IOException | InterruptedException e) {
            logger.warn("Error reporting player leave", e);
            Thread.currentThread().interrupt();
            return false;
        }
    }

    public List<AutoShutdownServer> getAutoShutdownServers() {
        String url = config.configVar.managerUrl + "/api/servers/auto-shutdown";
        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create(url))
            .GET()
            .header("Authorization", "Bearer " + config.configVar.apiKey)
            .timeout(Duration.ofSeconds(30))
            .build();

        try {
            HttpResponse<String> response = httpClient.send(request, BodyHandlers.ofString());
            if (response.statusCode() >= 200 && response.statusCode() < 300) {
                String body = response.body();
                Type listType = new TypeToken<List<AutoShutdownServer>>(){}.getType();
                List<AutoShutdownServer> servers = gson.fromJson(body, listType);
                return servers;
            }
            logger.warn("Failed to get auto-shutdown servers: HTTP " + response.statusCode() + " - " + response.body());
        } catch (IOException | InterruptedException e) {
            logger.warn("Error getting auto-shutdown servers", e);
            Thread.currentThread().interrupt();
        }
        return null;
    }

    public ServerStatus getServerStatus(String serverName) {
        String url = config.configVar.managerUrl + "/api/servers/" + serverName + "/status";
        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create(url))
            .GET()
            .header("Authorization", "Bearer " + config.configVar.apiKey)
            .timeout(Duration.ofSeconds(30))
            .build();

        try {
            HttpResponse<String> response = httpClient.send(request, BodyHandlers.ofString());
            if (response.statusCode() >= 200 && response.statusCode() < 300) {
                String body = response.body();
                return gson.fromJson(body, ServerStatus.class);
            }
            logger.warn("Failed to get server status for " + serverName + ": HTTP " + response.statusCode() + " - " + response.body());
        } catch (IOException | InterruptedException e) {
            logger.warn("Error getting server status for " + serverName, e);
            Thread.currentThread().interrupt();
        }
        return null;
    }

    public CanStartResponse canStartServer(String serverName) {
        String url = config.configVar.managerUrl + "/api/servers/" + serverName + "/can-start";
        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create(url))
            .POST(BodyPublishers.ofString(""))
            .header("Authorization", "Bearer " + config.configVar.apiKey)
            .timeout(Duration.ofSeconds(30))
            .build();

        try {
            HttpResponse<String> response = httpClient.send(request, BodyHandlers.ofString());
            if (response.statusCode() >= 200 && response.statusCode() < 300) {
                String body = response.body();
                return gson.fromJson(body, CanStartResponse.class);
            }
            logger.error("Failed to check can-start for " + serverName + ": HTTP " + response.statusCode() + " - " + response.body());
        } catch (IOException | InterruptedException e) {
            logger.error("Error checking if server can start: " + serverName, e);
            Thread.currentThread().interrupt();
        }
        return null;
    }

    public boolean waitForContainerRemoval(String serverName) {
        String url = config.configVar.managerUrl + "/api/servers/" + serverName + "/wait-for-removal";
        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create(url))
            .POST(BodyPublishers.ofString(""))
            .header("Authorization", "Bearer " + config.configVar.apiKey)
            .timeout(Duration.ofSeconds(65))
            .build();

        try {
            HttpResponse<String> response = httpClient.send(request, BodyHandlers.ofString());
            if (response.statusCode() >= 200 && response.statusCode() < 300) {
                logger.info("Container removal completed for: " + serverName);
                return true;
            } else {
                logger.error("Failed to wait for container removal " + serverName + ": " + response.statusCode());
                return false;
            }
        } catch (IOException | InterruptedException e) {
            logger.error("Error waiting for container removal " + serverName, e);
            Thread.currentThread().interrupt();
            return false;
        }
    }
}
