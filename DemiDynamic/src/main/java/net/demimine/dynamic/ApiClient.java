package net.demimine.dynamic;

import com.google.gson.Gson;
import com.google.gson.JsonObject;
import com.google.gson.reflect.TypeToken;
import okhttp3.*;

import org.slf4j.Logger;

import java.io.IOException;
import java.lang.reflect.Type;
import java.util.List;
import java.util.concurrent.TimeUnit;

public class ApiClient {
    private final OkHttpClient httpClient;
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
        public int auto_shutdown_minutes;
        public int player_count;
    }

    public ApiClient(Config config, Logger logger) {
        this.config = config;
        this.logger = logger;
        this.gson = new Gson();
        this.httpClient = new OkHttpClient.Builder()
            .connectTimeout(30, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .writeTimeout(30, TimeUnit.SECONDS)
            .build();
    }

    public void updateConfig(Config config) {
        this.config = config;
    }

    public boolean startServer(String serverName) {
        String url = config.configVar.managerUrl + "/api/servers/" + serverName + "/start-by-name";
        RequestBody body = RequestBody.create(new byte[0]);
        Request request = new Request.Builder()
            .url(url)
            .post(body)
            .addHeader("Authorization", "Bearer " + config.configVar.apiKey)
            .build();

        try (Response response = httpClient.newCall(request).execute()) {
            if (response.isSuccessful()) {
                logger.info("Started server: " + serverName);
                return true;
            } else {
                logger.error("Failed to start server " + serverName + ": " + response.code());
                return false;
            }
        } catch (IOException e) {
            logger.error("Error starting server " + serverName, e);
            return false;
        }
    }

    public boolean stopServer(String serverId) {
        String url = config.configVar.managerUrl + "/api/servers/" + serverId + "/stop";
        RequestBody body = RequestBody.create(new byte[0]);
        Request request = new Request.Builder()
            .url(url)
            .post(body)
            .addHeader("Authorization", "Bearer " + config.configVar.apiKey)
            .build();

        try (Response response = httpClient.newCall(request).execute()) {
            if (response.isSuccessful()) {
                logger.info("Stopped server: " + serverId);
                return true;
            } else {
                logger.error("Failed to stop server " + serverId + ": " + response.code());
                return false;
            }
        } catch (IOException e) {
            logger.error("Error stopping server " + serverId, e);
            return false;
        }
    }

    public boolean stopServerByName(String serverName) {
        String url = config.configVar.managerUrl + "/api/servers/" + serverName + "/stop-by-name";
        RequestBody body = RequestBody.create(new byte[0]);
        Request request = new Request.Builder()
            .url(url)
            .post(body)
            .addHeader("Authorization", "Bearer " + config.configVar.apiKey)
            .build();

        try (Response response = httpClient.newCall(request).execute()) {
            if (response.isSuccessful()) {
                logger.info("Stopped server by name: " + serverName);
                return true;
            } else {
                logger.error("Failed to stop server " + serverName + ": " + response.code());
                return false;
            }
        } catch (IOException e) {
            logger.error("Error stopping server " + serverName, e);
            return false;
        }
    }

    public boolean reportPlayerJoin(String uuid, String name, String serverName) {
        String url = config.configVar.managerUrl + "/api/players/join";
        JsonObject requestBody = new JsonObject();
        requestBody.addProperty("uuid", uuid);
        requestBody.addProperty("name", name);
        requestBody.addProperty("server_name", serverName);

        RequestBody body = RequestBody.create(requestBody.toString(), MediaType.parse("application/json"));
        Request request = new Request.Builder()
            .url(url)
            .post(body)
            .addHeader("Authorization", "Bearer " + config.configVar.apiKey)
            .addHeader("Content-Type", "application/json")
            .build();

        try (Response response = httpClient.newCall(request).execute()) {
            if (!response.isSuccessful()) {
                logger.warn("Failed to report player join: " + response.code());
                return false;
            }
            return true;
        } catch (IOException e) {
            logger.warn("Error reporting player join", e);
            return false;
        }
    }

    public boolean reportPlayerLeave(String uuid, String serverName) {
        String url = config.configVar.managerUrl + "/api/players/leave";
        JsonObject requestBody = new JsonObject();
        requestBody.addProperty("uuid", uuid);
        requestBody.addProperty("server_name", serverName);

        RequestBody body = RequestBody.create(requestBody.toString(), MediaType.parse("application/json"));
        Request request = new Request.Builder()
            .url(url)
            .post(body)
            .addHeader("Authorization", "Bearer " + config.configVar.apiKey)
            .addHeader("Content-Type", "application/json")
            .build();

        try (Response response = httpClient.newCall(request).execute()) {
            if (!response.isSuccessful()) {
                logger.warn("Failed to report player leave: " + response.code());
                return false;
            }
            return true;
        } catch (IOException e) {
            logger.warn("Error reporting player leave", e);
            return false;
        }
    }

    public List<AutoShutdownServer> getAutoShutdownServers() {
        String url = config.configVar.managerUrl + "/api/servers/auto-shutdown";
        Request request = new Request.Builder()
            .url(url)
            .get()
            .addHeader("Authorization", "Bearer " + config.configVar.apiKey)
            .build();

        try (Response response = httpClient.newCall(request).execute()) {
            if (response.isSuccessful()) {
                String body = response.body().string();
                Type listType = new TypeToken<List<AutoShutdownServer>>(){}.getType();
                List<AutoShutdownServer> servers = gson.fromJson(body, listType);
                return servers;
            }
        } catch (IOException e) {
            logger.warn("Error getting auto-shutdown servers", e);
        }
        return null;
    }

    public ServerStatus getServerStatus(String serverName) {
        String url = config.configVar.managerUrl + "/api/servers/" + serverName + "/status";
        Request request = new Request.Builder()
            .url(url)
            .get()
            .addHeader("Authorization", "Bearer " + config.configVar.apiKey)
            .build();

        try (Response response = httpClient.newCall(request).execute()) {
            if (response.isSuccessful()) {
                String body = response.body().string();
                return gson.fromJson(body, ServerStatus.class);
            }
        } catch (IOException e) {
            logger.warn("Error getting server status for " + serverName, e);
        }
        return null;
    }
}
