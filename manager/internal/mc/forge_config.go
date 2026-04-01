package mc

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type ForgeProxyConfig struct {
	Forwarding struct {
		Enabled bool   `toml:"enabled"`
		Mode    string `toml:"mode"`
		Secret  string `toml:"secret"`
	} `toml:"forwarding"`
}

func DefaultForgeProxyConfig(secret string) *ForgeProxyConfig {
	config := &ForgeProxyConfig{}
	config.Forwarding.Enabled = true
	config.Forwarding.Mode = "MODERN"
	config.Forwarding.Secret = secret
	return config
}

func ReadForgeProxyConfig(path string) (*ForgeProxyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &ForgeProxyConfig{}
	if err := toml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse ProxyCompatibleForge config: %w", err)
	}

	return config, nil
}

func (c *ForgeProxyConfig) WriteToFile(path string) error {
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal ProxyCompatibleForge config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

func ConfigureForgeProxy(serverPath, mcVersion, loader, forwardingSecret string) error {
	configDir := filepath.Join(serverPath, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "proxy-compatible-forge.toml")
	config := DefaultForgeProxyConfig(forwardingSecret)

	if err := config.WriteToFile(configPath); err != nil {
		return err
	}

	modsDir := filepath.Join(serverPath, "mods")
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		return fmt.Errorf("failed to create mods directory: %w", err)
	}

	filename, err := GetModrinthModFilename("proxy-compatible-forge", mcVersion, loader)
	if err != nil {
		log.Printf("Warning: ProxyCompatibleForge not available for %s/%s (%v), skipping auto-install", mcVersion, loader, err)
	} else {
		modPath := filepath.Join(modsDir, filename)
		if _, err := os.Stat(modPath); os.IsNotExist(err) {
			if err := DownloadModrinthMod("proxy-compatible-forge", mcVersion, loader, modPath); err != nil {
				log.Printf("Warning: Failed to download ProxyCompatibleForge: %v", err)
			}
		}
	}

	return nil
}

func RevertForgeProxyConfig(serverPath, mcVersion, loader string) error {
	configPath := filepath.Join(serverPath, "config", "proxy-compatible-forge.toml")
	if _, err := os.Stat(configPath); err == nil {
		config := &ForgeProxyConfig{}
		if existingData, err := os.ReadFile(configPath); err == nil {
			toml.Unmarshal(existingData, config)
		}
		config.Forwarding.Enabled = false
		config.Forwarding.Secret = ""
		if err := config.WriteToFile(configPath); err != nil {
			return err
		}
	}

	filename, err := GetModrinthModFilename("proxy-compatible-forge", mcVersion, loader)
	if err != nil {
		return nil
	}

	modsDir := filepath.Join(serverPath, "mods")
	modPath := filepath.Join(modsDir, filename)
	if _, err := os.Stat(modPath); err == nil {
		if err := os.Remove(modPath); err != nil {
			return fmt.Errorf("failed to remove ProxyCompatibleForge mod: %w", err)
		}
	}

	return nil
}
