package mc

import (
	"os"
	"path/filepath"
)

func WriteEntrypointScript(serverPath string) error {
	scriptPath := filepath.Join(serverPath, "start.sh")
	return os.WriteFile(scriptPath, []byte(startShScript), 0755)
}

func GetEntrypointScriptContent() string {
	return startShScript
}

const startShScript = `#!/bin/sh
set -e

cd /server

RAM_MB="${RAM_MB:-2048}"
JAVA_ARGS="-Xmx${RAM_MB}M -Xms$((RAM_MB/2))M"

log_info() {
    echo "[DemiMine] $1"
}

run_installer_if_needed() {
    log_info "Checking if server installation is needed..."
    
    case "$SERVER_TYPE" in
        fabric)
            if [ ! -f "fabric-server-launch.jar" ] && [ -f "fabric-installer.jar" ]; then
                log_info "Running Fabric installer for MC $MC_VERSION..."
                java -jar fabric-installer.jar server -mcversion "$MC_VERSION" -downloadMinecraft -dir .
                if [ $? -eq 0 ]; then
                    log_info "Fabric installer completed successfully"
                else
                    echo "[DemiMine] ERROR: Fabric installer failed!"
                    exit 1
                fi
            else
                log_info "Fabric server already installed or installer not found"
            fi
            ;;
        neoforge)
            if [ ! -f "run.sh" ] && [ -f "neoforge-installer.jar" ]; then
                log_info "Running NeoForge installer for MC $MC_VERSION..."
                java -jar neoforge-installer.jar --installServer
                if [ $? -eq 0 ]; then
                    log_info "NeoForge installer completed successfully"
                else
                    echo "[DemiMine] ERROR: NeoForge installer failed!"
                    exit 1
                fi
            else
                log_info "NeoForge server already installed or installer not found"
            fi
            ;;
        forge)
            if [ ! -f "run.sh" ] && [ -f "forge-installer.jar" ]; then
                log_info "Running Forge installer for MC $MC_VERSION..."
                java -jar forge-installer.jar --installServer
                if [ $? -eq 0 ]; then
                    log_info "Forge installer completed successfully"
                else
                    echo "[DemiMine] ERROR: Forge installer failed!"
                    exit 1
                fi
            else
                log_info "Forge server already installed or installer not found"
            fi
            ;;
        *)
            log_info "No installer needed for server type: $SERVER_TYPE"
            ;;
    esac
}

start_server() {
    log_info "Starting server..."
    
    case "$SERVER_TYPE" in
        fabric)
            exec java $JAVA_ARGS -jar fabric-server-launch.jar nogui
            ;;
        neoforge|forge)
            exec sh run.sh nogui
            ;;
        paper|purpur|*)
            exec java $JAVA_ARGS -jar server.jar nogui
            ;;
        *)
            echo "[DemiMine] ERROR: Unknown server type: $SERVER_TYPE"
            exit 1
            ;;
    esac
}

run_installer_if_needed
start_server
`
