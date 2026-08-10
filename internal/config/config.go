package config

import (
	"fmt"
	"log"

	"github.com/spf13/viper"
)

// Config holds all configuration parameters for the application.
type Config struct {
	Port             string `mapstructure:"PORT"`
	AppEnv           string `mapstructure:"APP_ENV"`
	DBPath           string `mapstructure:"DB_PATH"`
	TelegramBotToken string `mapstructure:"TELEGRAM_BOT_TOKEN"`
	TelegramChatID   string `mapstructure:"TELEGRAM_CHAT_ID"`
	BasicAuthUser    string `mapstructure:"BASIC_AUTH_USER"`
	BasicAuthPass    string `mapstructure:"BASIC_AUTH_PASS"`
}

// LoadConfig reads configuration from .env file or environment variables.
func LoadConfig() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")

	// Set sane default values
	viper.SetDefault("PORT", "8080")
	viper.SetDefault("APP_ENV", "development")
	viper.SetDefault("DB_PATH", "recon.db")

	// Allow environment variables to override .env settings
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Println("Notice: .env file not found or couldn't be loaded, relying on environment variables and defaults.")
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Validate checks that required configuration fields contain valid values.
func (c *Config) Validate() error {
	if c.Port == "" {
		return fmt.Errorf("PORT configuration cannot be empty")
	}
	if c.DBPath == "" {
		return fmt.Errorf("DB_PATH configuration cannot be empty")
	}
	return nil
}
