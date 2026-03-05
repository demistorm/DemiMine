package config

import (
	crand "crypto/rand"
	 "encoding/hex"
    "os"
    "sync"

    "github.com/spf13/viper"
)

type Config struct {
	Port              string
	DataDir           string
	ServersDir        string
	ProxiesDir        string
	BackupsDir        string
	JavaDir           string
	NetworkName       string
	MaxRAMMB          int
	JWTSecret         string
	SessionKey        string
	DatabaseURL       string
}

var (
	cfg     *Config
	once    sync.Once
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
		v.SetDefault("PROXIES_DIR", "/proxies")
		v.SetDefault("BACKUPS_DIR", "/backups")
		v.SetDefault("JAVA_DIR", "/java")
		v.SetDefault("NETWORK", "demimine_internal")
		v.SetDefault("MAX_RAM_MB", 0)
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
			Port:        v.GetString("PORT"),
			DataDir:     v.GetString("DATA_DIR"),
			ServersDir:  v.GetString("SERVERS_DIR"),
			ProxiesDir:  v.GetString("PROXIES_DIR"),
			BackupsDir:  v.GetString("BACKUPS_DIR"),
			JavaDir:     v.GetString("JAVA_DIR"),
			NetworkName: v.GetString("NETWORK"),
			MaxRAMMB:    v.GetInt("MAX_RAM_MB"),
			JWTSecret:   jwtSecret,
			SessionKey:  sessionKey,
			DatabaseURL: v.GetString("DATABASE_URL"),
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

func generateRandomString(length int) string {
	b := make([]byte, (length+1)/2)
	if _, err := crand.Read(b); err != nil {
		for i := range b {
			b[i] = byte(i)
		}
	}
	return hex.EncodeToString(b)[:length]
}
