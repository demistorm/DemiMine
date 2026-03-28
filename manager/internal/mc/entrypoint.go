package mc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteEntrypointScript(serverPath string) error {
	scriptPath := filepath.Join(serverPath, "start.sh")
	return os.WriteFile(scriptPath, []byte(startShScript), 0755)
}

func GetEntrypointScriptContent() string {
	return startShScript
}

func WriteUserJVMArgs(serverPath string, ramMB int, jvmFlags string) error {
	argsPath := filepath.Join(serverPath, "user_jvm_args.txt")
	var lines []string
	lines = append(lines, "# Xmx and Xms set by DemiMine")
	lines = append(lines, fmt.Sprintf("-Xmx%dM", ramMB))
	lines = append(lines, fmt.Sprintf("-Xms%dM", ramMB))
	if strings.TrimSpace(jvmFlags) != "" {
		for _, arg := range strings.Fields(jvmFlags) {
			lines = append(lines, arg)
		}
	}
	return os.WriteFile(argsPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

const startShScript = `#!/bin/sh
set -e

cd /server

RAM_MB="${RAM_MB:-2048}"
JAVA_ARGS="-Xmx${RAM_MB}M -Xms${RAM_MB}M"
JVM_FLAGS="${JVM_FLAGS:-}"

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

write_user_jvm_args() {
    log_info "Writing user_jvm_args.txt with JVM flags..."
    echo "# Xmx and Xms set by DemiMine" > user_jvm_args.txt
    echo "-Xmx${RAM_MB}M" >> user_jvm_args.txt
    echo "-Xms${RAM_MB}M" >> user_jvm_args.txt
    if [ -n "$JVM_FLAGS" ]; then
        for arg in $JVM_FLAGS; do
            echo "$arg" >> user_jvm_args.txt
        done
    fi
}

start_server() {
    log_info "Starting server..."
    
    case "$SERVER_TYPE" in
        fabric)
            exec java $JAVA_ARGS $JVM_FLAGS -jar fabric-server-launch.jar nogui
            ;;
        neoforge|forge)
            write_user_jvm_args
            exec sh run.sh nogui
            ;;
        nanolimbo)
            if [ -n "$JVM_FLAGS" ]; then
                exec java $JVM_FLAGS -jar server.jar
            else
                exec java -jar server.jar
            fi
            ;;
        paper|purpur|*)
            exec java $JAVA_ARGS $JVM_FLAGS -jar server.jar nogui
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
