package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Goff     GoffConfig     `mapstructure:"goff"`
	Keycloak KeycloakConfig `mapstructure:"keycloak"`
}

type AppConfig struct {
	Port        string   `mapstructure:"port"`
	LogLevel    string   `mapstructure:"log_level"`
	CorsOrigins []string `mapstructure:"cors_origins"`
	RateLimit   int      `mapstructure:"rate_limit"`
}

type GoffConfig struct {
	Endpoint string `mapstructure:"endpoint"`
}

type KeycloakConfig struct {
	URL          string `mapstructure:"url"`
	Realm        string `mapstructure:"realm"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
}

func Load() (*Config, error) {
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "local"
	}

	viper.SetConfigName(appEnv)
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("../config")
	viper.AddConfigPath("../../config")

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("app.rate_limit", 100)

	// Load .env file if present (for local development secrets)
	if data, err := os.ReadFile(".env"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if k, v, ok := strings.Cut(line, "="); ok {
				k = strings.TrimSpace(k)
				v = strings.TrimSpace(v)
				if os.Getenv(k) == "" {
					os.Setenv(k, v)
				}
			}
		}
	}

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &Config{}
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) Validate() error {
	if c.App.Port == "" {
		return fmt.Errorf("app.port cannot be empty")
	}

	if c.Goff.Endpoint == "" {
		return fmt.Errorf("goff.endpoint cannot be empty")
	}

	if c.Keycloak.URL == "" {
		return fmt.Errorf("keycloak.url is required")
	}

	if c.Keycloak.Realm == "" {
		return fmt.Errorf("keycloak.realm is required")
	}

	if c.Keycloak.ClientID == "" {
		return fmt.Errorf("keycloak.client_id is required")
	}

	if c.Keycloak.ClientSecret == "" {
		return fmt.Errorf("keycloak.client_secret is required")
	}

	return nil
}
