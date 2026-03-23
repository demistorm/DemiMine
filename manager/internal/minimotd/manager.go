package minimotd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	pluginSettingsFile = "plugin_settings.conf"
	mainConfigFile     = "main.conf"
	extraConfigsDir    = "extra-configs"
	iconsDir           = "icons"
)

type Manager struct {
	ProxiesDir string
}

func NewManager(proxiesDir string) *Manager {
	return &Manager{
		ProxiesDir: proxiesDir,
	}
}

func (m *Manager) getProxyPath(proxyName string) string {
	return filepath.Join(m.ProxiesDir, proxyName)
}

func (m *Manager) getMinimotdPluginsPath(proxyName string) string {
	proxyPath := m.getProxyPath(proxyName)
	minimotdDirs := []string{
		"plugins/minimotd-velocity",
		"plugins/minimotd",
	}

	for _, dir := range minimotdDirs {
		fullPath := filepath.Join(proxyPath, dir)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}

	return filepath.Join(proxyPath, "plugins/minimotd-velocity")
}

func (m *Manager) getExtraConfigPath(proxyName, serverName string) string {
	minimotdPath := m.getMinimotdPluginsPath(proxyName)
	return filepath.Join(minimotdPath, extraConfigsDir, serverName+".conf")
}

func (m *Manager) getIconPath(proxyName, serverName string) string {
	minimotdPath := m.getMinimotdPluginsPath(proxyName)
	return filepath.Join(minimotdPath, iconsDir, serverName+"-server.png")
}

func (m *Manager) getPluginSettingsPath(proxyName string) string {
	minimotdPath := m.getMinimotdPluginsPath(proxyName)
	return filepath.Join(minimotdPath, pluginSettingsFile)
}

func (m *Manager) getMainConfigPath(proxyName string) string {
	minimotdPath := m.getMinimotdPluginsPath(proxyName)
	return filepath.Join(minimotdPath, mainConfigFile)
}

func (m *Manager) GetMainConfigIconPath(proxyName string) string {
	minimotdPath := m.getMinimotdPluginsPath(proxyName)
	return filepath.Join(minimotdPath, iconsDir, proxyName+".png")
}

func (m *Manager) CreateExtraConfig(proxyName, serverName, line1, line2 string, hasIcon bool) error {
	minimotdPath := m.getMinimotdPluginsPath(proxyName)
	extraConfigDir := filepath.Join(minimotdPath, extraConfigsDir)

	if err := os.MkdirAll(extraConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create extra-configs directory: %w", err)
	}

	configPath := m.getExtraConfigPath(proxyName, serverName)
	config := m.generateExtraConfig(serverName, line1, line2, hasIcon)

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write extra config: %w", err)
	}

	return nil
}

func (m *Manager) UpdateExtraConfig(proxyName, serverName, line1, line2 string, hasIcon bool) error {
	configPath := m.getExtraConfigPath(proxyName, serverName)

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return m.CreateExtraConfig(proxyName, serverName, line1, line2, hasIcon)
	}

	config := m.generateExtraConfig(serverName, line1, line2, hasIcon)

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to update extra config: %w", err)
	}

	return nil
}

func (m *Manager) DeleteExtraConfig(proxyName, serverName string) error {
	configPath := m.getExtraConfigPath(proxyName, serverName)

	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete extra config: %w", err)
	}

	return nil
}

func (m *Manager) CopyIcon(proxyName, serverName string, iconData []byte) error {
	minimotdPath := m.getMinimotdPluginsPath(proxyName)
	iconsPath := filepath.Join(minimotdPath, iconsDir)

	if err := os.MkdirAll(iconsPath, 0755); err != nil {
		return fmt.Errorf("failed to create icons directory: %w", err)
	}

	iconPath := m.getIconPath(proxyName, serverName)

	if err := os.WriteFile(iconPath, iconData, 0644); err != nil {
		return fmt.Errorf("failed to write icon: %w", err)
	}

	return nil
}

func (m *Manager) DeleteIcon(proxyName, serverName string) error {
	iconPath := m.getIconPath(proxyName, serverName)

	if err := os.Remove(iconPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete icon: %w", err)
	}

	return nil
}

func (m *Manager) RenameServer(proxyName, oldName, newName, domain, proxyPort string) error {
	oldConfigPath := m.getExtraConfigPath(proxyName, oldName)
	newConfigPath := m.getExtraConfigPath(proxyName, newName)

	oldIconPath := m.getIconPath(proxyName, oldName)
	newIconPath := m.getIconPath(proxyName, newName)

	if _, err := os.Stat(oldConfigPath); err == nil {
		if err := os.Rename(oldConfigPath, newConfigPath); err != nil {
			return fmt.Errorf("failed to rename config: %w", err)
		}

		config := m.generateExtraConfig(newName, "", "", false)
		if err := os.WriteFile(newConfigPath, []byte(config), 0644); err != nil {
			return fmt.Errorf("failed to update config with new name: %w", err)
		}
	}

	if _, err := os.Stat(oldIconPath); err == nil {
		if err := os.Rename(oldIconPath, newIconPath); err != nil {
			return fmt.Errorf("failed to rename icon: %w", err)
		}
	}

	if domain != "" && proxyPort != "" {
		if err := m.updateVirtualHostConfig(proxyName, "", proxyPort, domain, newName); err != nil {
			return fmt.Errorf("failed to update virtual host config: %w", err)
		}
	}

	return nil
}

func (m *Manager) AddVirtualHost(proxyName, domain, proxyPort, serverName string) error {
	return m.updateVirtualHostConfig(proxyName, "", proxyPort, domain, serverName)
}

func (m *Manager) RemoveVirtualHost(proxyName, domain, proxyPort string) error {
	return m.updateVirtualHostConfig(proxyName, "", proxyPort, domain, "")
}

func (m *Manager) createDefaultPluginSettings(proxyName string) error {
	settingsPath := m.getPluginSettingsPath(proxyName)
	minimotdPath := m.getMinimotdPluginsPath(proxyName)

	if err := os.MkdirAll(minimotdPath, 0755); err != nil {
		return fmt.Errorf("failed to create minimotd directory: %w", err)
	}

	defaultContent := `# MiniMOTD Plugin Configuration

# Settings only applicable when running on a proxy (Velocity or Waterfall/Bungeecord)
proxy-settings {
    # Here you can assign configs in 'extra-configs' folder to specific virtual hosts
    # Either use name of config in 'extra-configs', or use "default" to use configuration in main.conf
    # 
    # Format is "hostname:port"="configName|default"
    # Parts of domains can be substituted for wildcards, i.e. "*.mydomain.com:25565". Wildcard-containing configs are
    # checked in order they are declared if there are no exact matches.
    virtual-host-configs {
    }
    # Set whether to enable virtual host testing mode.
    # When enabled, MiniMOTD will print virtual host debug info to console on each server ping.
    virtual-host-test-mode=false
}
# Do you want plugin to check for updates on GitHub at launch?
# https://github.com/jpenilla/MiniMOTD
update-checker=true
`

	if err := os.WriteFile(settingsPath, []byte(defaultContent), 0644); err != nil {
		return fmt.Errorf("failed to create plugin settings: %w", err)
	}

	return nil
}

func (m *Manager) updateVirtualHostConfig(proxyName, oldDomain, proxyPort, newDomain, serverName string) error {
	settingsPath := m.getPluginSettingsPath(proxyName)

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := m.createDefaultPluginSettings(proxyName); err != nil {
				return err
			}
			content, err = os.ReadFile(settingsPath)
			if err != nil {
				return fmt.Errorf("failed to read plugin settings after creating: %w", err)
			}
		} else {
			return fmt.Errorf("failed to read plugin settings: %w", err)
		}
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	var inVirtualHosts bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == `proxy-settings {` {
			inVirtualHosts = true
			newLines = append(newLines, line)
			continue
		}

		if inVirtualHosts && trimmed == `}` {
			if newDomain != "" && serverName != "" {
				newLines = append(newLines, fmt.Sprintf(`        "%s:%s"=%s`, newDomain, proxyPort, serverName))
			}
			inVirtualHosts = false
			newLines = append(newLines, line)
			continue
		}

		if inVirtualHosts && trimmed != "" && !strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, "=") {
			if oldDomain != "" {
				entryKey := strings.Split(trimmed, "=")[0]
				expectedKey := fmt.Sprintf(`"%s:%s"`, oldDomain, proxyPort)
				if strings.TrimSpace(entryKey) == expectedKey {
					continue
				}
			}

			if newDomain != "" && serverName != "" {
				newEntryKey := fmt.Sprintf(`"%s:%s"`, newDomain, proxyPort)
				entryKey := strings.Split(trimmed, "=")[0]
				if strings.TrimSpace(entryKey) == newEntryKey {
					continue
				}
			}
		}

		newLines = append(newLines, line)
	}

	newContent := strings.Join(newLines, "\n")

	if err := os.WriteFile(settingsPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write plugin settings: %w", err)
	}

	return nil
}

func (m *Manager) generateExtraConfig(serverName, line1, line2 string, hasIcon bool) string {
	iconSetting := ""
	if hasIcon {
		iconSetting = fmt.Sprintf("        icon=%s-server\n", serverName)
	}

	return fmt.Sprintf(`# Extra MiniMOTD config '%s'

icon-enabled=true
motd-enabled=true
motds=[
    {
%s        line1="%s"
        line2="%s"
    }
]
player-count-settings {
    allow-exceeding-maximum=false
    disable-player-list-hover=false
    fake-players {
        fake-players="0"
        fake-players-enabled=false
    }
    hide-player-count=false
    just-x-more-settings {
        just-x-more-enabled=false
        x-value=0
    }
    max-players=20
    max-players-enabled=false
    servers=[]
}
`, serverName, iconSetting, line1, line2)
}

func (m *Manager) CreateMainConfig(proxyName, line1, line2 string, hasIcon bool) error {
	minimotdPath := m.getMinimotdPluginsPath(proxyName)

	if err := os.MkdirAll(minimotdPath, 0755); err != nil {
		return fmt.Errorf("failed to create minimotd directory: %w", err)
	}

	configPath := m.getMainConfigPath(proxyName)
	config := m.generateMainConfig(proxyName, line1, line2, hasIcon)

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write main config: %w", err)
	}

	return nil
}

func (m *Manager) CopyMainConfigIcon(proxyName string, iconData []byte) error {
	minimotdPath := m.getMinimotdPluginsPath(proxyName)
	iconsPath := filepath.Join(minimotdPath, iconsDir)

	if err := os.MkdirAll(iconsPath, 0755); err != nil {
		return fmt.Errorf("failed to create icons directory: %w", err)
	}

	iconPath := m.GetMainConfigIconPath(proxyName)

	if err := os.WriteFile(iconPath, iconData, 0644); err != nil {
		return fmt.Errorf("failed to write main config icon: %w", err)
	}

	return nil
}

func (m *Manager) generateMainConfig(proxyName, line1, line2 string, hasIcon bool) string {
	iconSetting := "        icon=random"
	if hasIcon {
		iconSetting = fmt.Sprintf("        icon=%s", proxyName)
	}

	defaultLine1 := "<rainbow>Welcome to the Server"
	defaultLine2 := "MiniMessage <gradient:blue:red>Gradients"

	if line1 != "" {
		defaultLine1 = line1
	}
	if line2 != "" {
		defaultLine2 = line2
	}

	return fmt.Sprintf(`# MiniMOTD Main Configuration

# The list of MOTDs to display
# 
#  - Supported placeholders: <online_players>, <max_players>
#  - Putting more than one will cause one to be randomly chosen each refresh
motds=[
    {
        line1="%s"
        line2="%s"
        # Set the icon to use with this MOTD
        #   Either use 'random' to randomly choose an icon, or use the name
        #   of a file in the icons folder (excluding the '.png' extension)
        #     ex: icon="myIconFile"
%s    }
]
# Enable MOTD-related features
motd-enabled=true
# Enable server list icon related features
icon-enabled=true
player-count-settings {
    # Enable modification of the max player count
    max-players-enabled=true
    # Changes the Max Players value
    max-players=69
    # Setting this to true will disable the hover text showing online player usernames
    disable-player-list-hover=false
    # Setting this to true will disable the player list hover (same as 'disable-player-list-hover'),
    # but will also cause the player count to appear as '???'
    hide-player-count=false
    # Settings for the fake player count feature
    fake-players {
        # Enable fake player count feature
        fake-players-enabled=false
        # Modes: add, constant, minimum, random, percent
        # 
        #  - add: This many fake players will be added
        #      ex: fake-players="3"
        #  - constant: A constant value for the player count
        #      ex: fake-players="=42"
        #  - minimum: The minimum bound of the player count
        #      ex: fake-players="7+"
        #  - random: A random number of fake players in this range will be added
        #      ex: fake-players="3:6"
        #  - percent: The player count will be inflated by this much, rounding up
        #      ex: fake-players="25%"
        fake-players="25%%"
    }
    # Changes the Max Players to be X more than the online players
    # ex: x=3 -> 16/19 players online.
    just-x-more-settings {
        # Enable this feature
        just-x-more-enabled=false
        x-value=3
    }
    # Should the displayed online player count be allowed to exceed the displayed maximum player count?
    # If false, the online player count will be capped at the maximum player count
    allow-exceeding-maximum=false
    # The list of server names that affect player counts/listing.
    # Only applicable when running the plugin on a proxy (Velocity or Waterfall/Bungeecord).
    # When set to an empty list, the default count & list as determined by the proxy will be used.
    servers=[]
}
`, defaultLine1, defaultLine2, iconSetting)
}
