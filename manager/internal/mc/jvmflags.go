package mc

import (
	"github.com/demimine/manager/internal/java"
	"strings"
)

func GetDefaultJVMFlags(serverType, mcVersion string) string {
	switch strings.ToLower(serverType) {
	case "paper", "purpur", "fabric", "forge", "neoforge":
		flags := []string{
			"-XX:+UseG1GC",
			"-XX:+ParallelRefProcEnabled",
			"-XX:MaxGCPauseMillis=200",
			"-XX:+UnlockExperimentalVMOptions",
			"-XX:+DisableExplicitGC",
			"-XX:+AlwaysPreTouch",
			"-XX:G1HeapWastePercent=5",
			"-XX:G1MixedGCCountTarget=4",
			"-XX:InitiatingHeapOccupancyPercent=15",
			"-XX:G1MixedGCLiveThresholdPercent=90",
			"-XX:G1RSetUpdatingPauseTimePercent=5",
			"-XX:SurvivorRatio=32",
			"-XX:+PerfDisableSharedMem",
			"-XX:MaxTenuringThreshold=1",
			"-Dusing.aikars.flags=https://mcflags.emc.gs",
			"-Daikars.new.flags=true",
			"-XX:G1NewSizePercent=30",
			"-XX:G1MaxNewSizePercent=40",
			"-XX:G1HeapRegionSize=8M",
			"-XX:G1ReservePercent=20",
		}

		javaVersion := java.GetRequiredJavaVersionForServerType(serverType, mcVersion)
		javaMajor := 0
		for _, c := range javaVersion {
			if c >= '0' && c <= '9' {
				javaMajor = javaMajor*10 + int(c-'0')
			} else {
				break
			}
		}
		if javaMajor >= 17 {
			flags = append([]string{"--add-modules=jdk.incubator.vector"}, flags...)
		}

		return strings.Join(flags, " ")
	case "nanolimbo":
		return ""
	default:
		return ""
	}
}

func GetDefaultProxyJVMFlags() string {
	return strings.Join([]string{
		"-XX:+UseG1GC",
		"-XX:G1HeapRegionSize=4M",
		"-XX:+UnlockExperimentalVMOptions",
		"-XX:+ParallelRefProcEnabled",
		"-XX:+AlwaysPreTouch",
		"-XX:MaxInlineLevel=15",
	}, " ")
}
