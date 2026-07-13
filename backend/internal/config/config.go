package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	DB     DBConfig
	Server ServerConfig
	Jira   JiraConfig
	Sync   SyncConfig
	Log    LogConfig
	Auth   AuthConfig
	Gemini GeminiConfig
}

type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
}

func (d DBConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode)
}

type ServerConfig struct {
	Port string
	Host string
}

type JiraConfig struct {
	BaseURL   string
	AuthType  string
	UserEmail string
	APIToken  string
}

type SyncConfig struct {
	IntervalMinutes  int
	RateLimitPerSec  int
}

type LogConfig struct {
	Level string
}

type AuthConfig struct {
	JWTSecret          string
	JWTExpirationHours int
	AdminEmail         string
}

type GeminiConfig struct {
	APIKey string
	Model  string
}

func Load() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()

	viper.SetDefault("DB_HOST", "localhost")
	viper.SetDefault("DB_PORT", 5432)
	viper.SetDefault("DB_USER", "tcloud")
	viper.SetDefault("DB_PASSWORD", "tcloud_dev")
	viper.SetDefault("DB_NAME", "tcloud_planner")
	viper.SetDefault("DB_SSLMODE", "disable")
	viper.SetDefault("SERVER_PORT", "8080")
	viper.SetDefault("SERVER_HOST", "0.0.0.0")
	viper.SetDefault("SYNC_INTERVAL_MINUTES", 30)
	viper.SetDefault("SYNC_RATE_LIMIT_PER_SEC", 5)
	viper.SetDefault("LOG_LEVEL", "debug")
	viper.SetDefault("JWT_EXPIRATION_HOURS", 24)
	viper.SetDefault("GEMINI_MODEL", "gemini-2.0-flash")
	viper.SetDefault("ADMIN_EMAIL", "admin@tcloud.local")

	_ = viper.ReadInConfig()

	return &Config{
		DB: DBConfig{
			Host:     viper.GetString("DB_HOST"),
			Port:     viper.GetInt("DB_PORT"),
			User:     viper.GetString("DB_USER"),
			Password: viper.GetString("DB_PASSWORD"),
			Name:     viper.GetString("DB_NAME"),
			SSLMode:  viper.GetString("DB_SSLMODE"),
		},
		Server: ServerConfig{
			Port: viper.GetString("SERVER_PORT"),
			Host: viper.GetString("SERVER_HOST"),
		},
		Jira: JiraConfig{
			BaseURL:   viper.GetString("JIRA_BASE_URL"),
			AuthType:  viper.GetString("JIRA_AUTH_TYPE"),
			UserEmail: viper.GetString("JIRA_USER_EMAIL"),
			APIToken:  viper.GetString("JIRA_API_TOKEN"),
		},
		Sync: SyncConfig{
			IntervalMinutes:  viper.GetInt("SYNC_INTERVAL_MINUTES"),
			RateLimitPerSec:  viper.GetInt("SYNC_RATE_LIMIT_PER_SEC"),
		},
		Log: LogConfig{
			Level: viper.GetString("LOG_LEVEL"),
		},
		Auth: AuthConfig{
			JWTSecret:          viper.GetString("JWT_SECRET"),
			JWTExpirationHours: viper.GetInt("JWT_EXPIRATION_HOURS"),
			AdminEmail:         viper.GetString("ADMIN_EMAIL"),
		},
		Gemini: GeminiConfig{
			APIKey: viper.GetString("GEMINI_API_KEY"),
			Model:  viper.GetString("GEMINI_MODEL"),
		},
	}, nil
}
