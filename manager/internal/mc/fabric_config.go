package mc

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type FabricProxyConfig struct {
	Secret            string `toml:"secret"`
	HackOnlineMode    bool   `toml:"hackOnlineMode"`
	HackEarlySend     bool   `toml:"hackEarlySend"`
	HackMessageChain  bool   `toml:"hackMessageChain"`
	DisconnectMessage string `toml:"disconnectMessage"`
}

func DefaultFabricProxyConfig(secret string) *FabricProxyConfig {
	return &FabricProxyConfig{
		Secret:            secret,
		HackOnlineMode:    true,
		HackEarlySend:     true,
		HackMessageChain:  true,
		DisconnectMessage: "This server requires you to connect with Velocity.",
	}
}

func ReadFabricProxyConfig(path string) (*FabricProxyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &FabricProxyConfig{}
	if err := toml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse FabricProxy-Lite config: %w", err)
	}

	return config, nil
}

func (c *FabricProxyConfig) WriteToFile(path string) error {
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal FabricProxy-Lite config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

func ConfigureFabricProxy(serverPath, mcVersion, forwardingSecret string) error {
	configDir := filepath.Join(serverPath, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "FabricProxy-Lite.toml")
	config := DefaultFabricProxyConfig(forwardingSecret)

	if err := config.WriteToFile(configPath); err != nil {
		return err
	}

	modsDir := filepath.Join(serverPath, "mods")
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		return fmt.Errorf("failed to create mods directory: %w", err)
	}

	fabricAPIFilename, err := GetModrinthModFilename("fabric-api", mcVersion, "fabric")
	if err != nil {
		return fmt.Errorf("failed to get Fabric API filename: %w", err)
	}

	fabricAPIModPath := filepath.Join(modsDir, fabricAPIFilename)
	if _, err := os.Stat(fabricAPIModPath); os.IsNotExist(err) {
		if err := DownloadModrinthMod("fabric-api", mcVersion, "fabric", fabricAPIModPath); err != nil {
			return fmt.Errorf("failed to download Fabric API: %w", err)
		}
	}

	filename, err := GetModrinthModFilename("fabricproxy-lite", mcVersion, "fabric")
	if err != nil {
		return fmt.Errorf("failed to get FabricProxy-Lite filename: %w", err)
	}

	modPath := filepath.Join(modsDir, filename)
	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		if err := DownloadModrinthMod("fabricproxy-lite", mcVersion, "fabric", modPath); err != nil {
			return fmt.Errorf("failed to download FabricProxy-Lite: %w", err)
		}
	}

	return nil
}

func RevertFabricProxyConfig(serverPath string) error {
	configPath := filepath.Join(serverPath, "config", "FabricProxy-Lite.toml")
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove FabricProxy-Lite config: %w", err)
	}

	modsDir := filepath.Join(serverPath, "mods")
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read mods directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			name := entry.Name()
			if containsIgnoreCase(name, "fabricproxy-lite") || containsIgnoreCase(name, "FabricProxy-Lite") {
				modPath := filepath.Join(modsDir, name)
				if err := os.Remove(modPath); err != nil {
					return fmt.Errorf("failed to remove FabricProxy-Lite mod: %w", err)
				}
			}
			if containsIgnoreCase(name, "fabric-api") || containsIgnoreCase(name, "Fabric API") {
				modPath := filepath.Join(modsDir, name)
				if err := os.Remove(modPath); err != nil {
					return fmt.Errorf("failed to remove Fabric API mod: %w", err)
				}
			}
		}
	}

	return nil
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && containsSubstringIgnoreCase(s, substr)))
}

func containsSubstringIgnoreCase(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			subc := substr[j]
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if subc >= 'A' && subc <= 'Z' {
				subc += 32
			}
			if sc != subc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
