package config

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/spf13/viper"
)

type Config struct {
	Port           string
	DataDir        string
	ServersDir     string
	HostServersDir string
	BackupsDir     string
	JavaDir        string
	NetworkName    string
	MaxRAMMB       int
	JWTSecret      string
	SessionKey     string
	DatabaseURL    string
}

var (
	cfg           *Config
	once          sync.Once
	viperInstance *viper.Viper
)

func Load() (*Config, error) {
	var err error
	once.Do(func() {
		v := viper.New()
		viperInstance = v

		v.SetEnvPrefix("DEMIMINE")
		v.AutomaticEnv()

		v.SetDefault("PORT", "8080")
		v.SetDefault("DATA_DIR", "/data")
		v.SetDefault("SERVERS_DIR", "/servers")
		v.SetDefault("HOST_SERVERS_DIR", "/servers")
		v.SetDefault("BACKUPS_DIR", "/backups")
		v.SetDefault("JAVA_DIR", "/java")
		v.SetDefault("NETWORK", "demimine_internal")
		v.SetDefault("MAX_RAM_MB", 16384)
		v.SetDefault("JWT_SECRET", "")
		v.SetDefault("SESSION_KEY", "")
		v.SetDefault("DATABASE_URL", "")

		if configFile := os.Getenv("DEMIMINE_CONFIG"); configFile != "" {
			v.SetConfigFile(configFile)
			if err = v.ReadInConfig(); err != nil {
				return
			}
		}

		jwtSecret := v.GetString("JWT_SECRET")
		if jwtSecret == "" {
			jwtSecret = generateRandomString(32)
		}

		sessionKey := v.GetString("SESSION_KEY")
		if sessionKey == "" {
			sessionKey = generateRandomString(32)
		}

		cfg = &Config{
			Port:           v.GetString("PORT"),
			DataDir:        v.GetString("DATA_DIR"),
			ServersDir:     v.GetString("SERVERS_DIR"),
			HostServersDir: v.GetString("HOST_SERVERS_DIR"),
			BackupsDir:     v.GetString("BACKUPS_DIR"),
			JavaDir:        v.GetString("JAVA_DIR"),
			NetworkName:    v.GetString("NETWORK"),
			MaxRAMMB:       v.GetInt("MAX_RAM_MB"),
			JWTSecret:      jwtSecret,
			SessionKey:     sessionKey,
			DatabaseURL:    v.GetString("DATABASE_URL"),
		}
	})
	return cfg, err
}

func Get() *Config {
	if cfg == nil {
		panic("config not loaded")
	}
	return cfg
}

func (c *Config) Validate() error {
	if _, err := strconv.Atoi(c.Port); err != nil {
		return fmt.Errorf("DEMIMINE_PORT must be a valid number, got %q", c.Port)
	}
	if c.HostServersDir == "" {
		return fmt.Errorf("DEMIMINE_HOST_SERVERS_DIR is required")
	}
	if c.NetworkName == "" {
		return fmt.Errorf("DEMIMINE_NETWORK is required")
	}
	if c.MaxRAMMB < 0 {
		return fmt.Errorf("DEMIMINE_MAX_RAM_MB must be >= 0, got %d", c.MaxRAMMB)
	}
	return nil
}

func generateRandomString(length int) string {
	b := make([]byte, (length+1)/2)
	if _, err := crand.Read(b); err != nil {
		for i := range b {
			b[i] = byte(i)
		}
	}
	return hex.EncodeToString(b)[:length]
}
