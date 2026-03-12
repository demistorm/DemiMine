package mc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type SpigotConfig struct {
	Settings struct {
		BungeeCord bool `yaml:"bungeecord"`
	} `yaml:"settings"`
}

type PaperGlobalConfig struct {
	Proxies struct {
		Velocity struct {
			Enabled    bool   `yaml:"enabled"`
			Secret     string `yaml:"secret"`
			OnlineMode bool   `yaml:"online-mode"`
		} `yaml:"velocity"`
	} `yaml:"proxies"`
}

func ReadSpigotConfig(path string) (*SpigotConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &SpigotConfig{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse spigot.yml: %w", err)
	}

	return config, nil
}

func (c *SpigotConfig) WriteToFile(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal spigot.yml: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

func ReadPaperGlobalConfig(path string) (*PaperGlobalConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &PaperGlobalConfig{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse paper-global.yml: %w", err)
	}

	return config, nil
}

func (c *PaperGlobalConfig) WriteToFile(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal paper-global.yml: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

func ConfigurePaperProxy(serverPath, forwardingSecret string) error {
	configDir := filepath.Join(serverPath, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	paperGlobalPath := filepath.Join(configDir, "paper-global.yml")
	config := &PaperGlobalConfig{}

	if existingData, err := os.ReadFile(paperGlobalPath); err == nil {
		yaml.Unmarshal(existingData, config)
	}

	config.Proxies.Velocity.Enabled = true
	config.Proxies.Velocity.Secret = forwardingSecret
	config.Proxies.Velocity.OnlineMode = true

	if err := config.WriteToFile(paperGlobalPath); err != nil {
		return err
	}

	spigotPath := filepath.Join(serverPath, "spigot.yml")
	spigotConfig := &SpigotConfig{}

	if existingData, err := os.ReadFile(spigotPath); err == nil {
		yaml.Unmarshal(existingData, spigotConfig)
	}

	spigotConfig.Settings.BungeeCord = false

	if err := spigotConfig.WriteToFile(spigotPath); err != nil {
		return err
	}

	return nil
}

func RevertPaperProxyConfig(serverPath string) error {
	paperGlobalPath := filepath.Join(serverPath, "config", "paper-global.yml")
	if _, err := os.Stat(paperGlobalPath); err == nil {
		config := &PaperGlobalConfig{}
		if existingData, err := os.ReadFile(paperGlobalPath); err == nil {
			yaml.Unmarshal(existingData, config)
		}

		config.Proxies.Velocity.Enabled = false
		config.Proxies.Velocity.Secret = ""

		if err := config.WriteToFile(paperGlobalPath); err != nil {
			return err
		}
	}

	spigotPath := filepath.Join(serverPath, "spigot.yml")
	if _, err := os.Stat(spigotPath); err == nil {
		spigotConfig := &SpigotConfig{}
		if existingData, err := os.ReadFile(spigotPath); err == nil {
			yaml.Unmarshal(existingData, spigotConfig)
		}

		spigotConfig.Settings.BungeeCord = false

		if err := spigotConfig.WriteToFile(spigotPath); err != nil {
			return err
		}
	}

	return nil
}

func mergeYAML(existing, newConfig map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range existing {
		result[k] = v
	}
	for k, v := range newConfig {
		if existingVal, ok := existing[k]; ok {
			if existingMap, ok := existingVal.(map[string]interface{}); ok {
				if newMap, ok := v.(map[string]interface{}); ok {
					result[k] = mergeYAML(existingMap, newMap)
					continue
				}
			}
		}
		result[k] = v
	}
	return result
}

func writeYAMLWithComments(path string, config interface{}, headerComments []string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	var sb strings.Builder
	for _, comment := range headerComments {
		sb.WriteString("# " + comment + "\n")
	}
	sb.Write(data)

	return os.WriteFile(path, []byte(sb.String()), 0644)
}
