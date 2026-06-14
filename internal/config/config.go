package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Ameb8/chime/internal/paths"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig        `mapstructure:"server" yaml:"server"`
	Auth          AuthConfig          `mapstructure:"auth" yaml:"auth"`
	Client        ClientConfig        `mapstructure:"client" yaml:"client"`
	Notifications NotificationsConfig `mapstructure:"notifications" yaml:"notifications"`
	Log           LogConfig           `mapstructure:"log" yaml:"log"`
}

type ServerConfig struct {
	Bind string `mapstructure:"bind" yaml:"bind"`
}

type AuthConfig struct {
	Key string `mapstructure:"key" yaml:"key"`
}

type ClientConfig struct {
	Server string `mapstructure:"server" yaml:"server"`
}

type NotificationsConfig struct {
	Toast ToastConfig `mapstructure:"toast" yaml:"toast"`
	Sound SoundConfig `mapstructure:"sound" yaml:"sound"`
}

type ToastConfig struct {
	Enabled bool     `mapstructure:"enabled" yaml:"enabled"`
	Events  []string `mapstructure:"events" yaml:"events"`
}

type SoundConfig struct {
	Enabled       bool     `mapstructure:"enabled" yaml:"enabled"`
	Events        []string `mapstructure:"events" yaml:"events"`
	CompleteSound string   `mapstructure:"complete_sound" yaml:"complete_sound"`
	WaitingSound  string   `mapstructure:"waiting_sound" yaml:"waiting_sound"`
}

type LogConfig struct {
	Level string `mapstructure:"level" yaml:"level"`
	File  string `mapstructure:"file" yaml:"file"`
}

func Load() (*Config, error) {
	viper.Reset()
	registerDefaults()
	if err := bindEnv(); err != nil {
		return nil, err
	}

	viper.SetConfigFile(paths.ConfigFile())
	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) || os.IsNotExist(err) {
			cfg := defaultConfig()
			if saveErr := Save(&cfg); saveErr != nil {
				return nil, fmt.Errorf("create default config: %w", saveErr)
			}
			if readErr := viper.ReadInConfig(); readErr != nil {
				return nil, fmt.Errorf("read created config: %w", readErr)
			}
		} else {
			return nil, err
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	path := paths.ConfigFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	b, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("set config permissions: %w", err)
	}
	return nil
}

func Resolve(flagVal, envVar, configVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return configVal
}

func registerDefaults() {
	viper.SetDefault("server.bind", "0.0.0.0:7777")
	viper.SetDefault("auth.key", "")
	viper.SetDefault("client.server", "")
	viper.SetDefault("notifications.toast.enabled", true)
	viper.SetDefault("notifications.toast.events", []string{"complete", "waiting"})
	viper.SetDefault("notifications.sound.enabled", true)
	viper.SetDefault("notifications.sound.events", []string{"complete", "waiting"})
	viper.SetDefault("notifications.sound.complete_sound", "")
	viper.SetDefault("notifications.sound.waiting_sound", "")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.file", "")
}

func bindEnv() error {
	bindings := map[string]string{
		"auth.key":      "CHIME_KEY",
		"client.server": "CHIME_SERVER",
		"server.bind":   "CHIME_BIND",
		"log.level":     "CHIME_LOG_LEVEL",
	}
	for key, env := range bindings {
		if err := viper.BindEnv(key, env); err != nil {
			return fmt.Errorf("bind %s: %w", env, err)
		}
	}
	return nil
}

func defaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Bind: "0.0.0.0:7777",
		},
		Auth: AuthConfig{
			Key: "",
		},
		Client: ClientConfig{
			Server: "",
		},
		Notifications: NotificationsConfig{
			Toast: ToastConfig{
				Enabled: true,
				Events:  []string{"complete", "waiting"},
			},
			Sound: SoundConfig{
				Enabled:       true,
				Events:        []string{"complete", "waiting"},
				CompleteSound: "",
				WaitingSound:  "",
			},
		},
		Log: LogConfig{
			Level: "info",
			File:  "",
		},
	}
}
