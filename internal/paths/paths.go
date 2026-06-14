package paths

import (
	"os"
	"path/filepath"
	"strings"
)

const appName = "chime"

var configFileOverride string

// SetConfigFile sets the config file path selected by the CLI --config flag.
func SetConfigFile(path string) {
	configFileOverride = expand(path)
}

func ConfigFile() string {
	if configFileOverride != "" {
		return configFileOverride
	}
	if path := os.Getenv("CHIME_CONFIG"); path != "" {
		return expand(path)
	}
	return filepath.Join(configHome(), appName, "config.yaml")
}

func ConfigDir() string {
	return filepath.Dir(ConfigFile())
}

func DataDir() string {
	return filepath.Join(dataHome(), appName)
}

func LogFile() string {
	return filepath.Join(DataDir(), "chime.log")
}

func PIDFile() string {
	return filepath.Join(DataDir(), "chime.pid")
}

func configHome() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return expand(dir)
	}
	return filepath.Join(homeDir(), ".config")
}

func dataHome() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return expand(dir)
	}
	return filepath.Join(homeDir(), ".local", "share")
}

func expand(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir(), strings.TrimPrefix(path, "~/"))
	}
	return path
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}
