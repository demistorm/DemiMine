package java

import (
	"strconv"
	"strings"
)

func GetRequiredJavaVersion(mcVersion string) string {
	baseVersion := strings.Split(mcVersion, "-")[0]

	if !strings.HasPrefix(baseVersion, "1.") {
		parts := strings.Split(baseVersion, ".")
		if len(parts) >= 1 {
			major, err := strconv.Atoi(parts[0])
			if err == nil && major >= 26 {
				return "25"
			}
		}
		return "21"
	}

	parts := strings.Split(baseVersion, ".")
	if len(parts) < 2 {
		return "21"
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "21"
	}

	patch := 0
	if len(parts) > 2 {
		patch, _ = strconv.Atoi(parts[2])
	}

	switch {
	case minor >= 26:
		return "25"
	case minor > 20 || (minor == 20 && patch >= 5):
		return "21"
	case minor >= 17:
		return "17"
	default:
		return "8"
	}
}

func GetRequiredJavaVersionForServerType(serverType, version string) string {
	if strings.ToLower(serverType) == "nanolimbo" {
		return "21"
	}
	return GetRequiredJavaVersion(version)
}
