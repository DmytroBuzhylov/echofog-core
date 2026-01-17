package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	CurrentProtocolVersion uint32 = 100
	DefaultConfigName             = "echofog.json"
	AppName                       = "EchoFog"
)

type AppConfig struct {
	Identity struct {
		Callsign string `json:"callsign"`
		KeyPath  string `json:"key_path"`
	} `json:"identity"`

	Network struct {
		ListenAddr      string   `json:"listen_addr"`
		BootstrapNodes  []string `json:"bootstrap_nodes"`
		MaxConnections  int      `json:"max_connections"`
		ProtocolVersion uint32   `json:"protocol_version"`
		EnableMDNS      bool     `json:"enable_mdns"`
	} `json:"network"`

	Storage struct {
		DatabasePath string `json:"database_path"`
		DownloadsDir string `json:"downloads_dir"`
	} `json:"storage"`

	Security struct {
		AnonymousMode bool `json:"anonymous_mode"`
		HideTraffic   bool `json:"hide_traffic"`
	} `json:"security"`
}

func LoadConfig(path string) (*AppConfig, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &AppConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *AppConfig) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func DefaultConfig() *AppConfig {
	cfg := &AppConfig{}

	cfg.Network.ListenAddr = ":0"
	cfg.Network.MaxConnections = 100
	cfg.Network.ProtocolVersion = CurrentProtocolVersion
	cfg.Network.EnableMDNS = true

	home, _ := os.UserHomeDir()
	appDir := filepath.Join(home, ".echofog")

	cfg.Storage.DatabasePath = filepath.Join(appDir, "db")
	cfg.Storage.DownloadsDir = filepath.Join(home, "Downloads", "EchoFog")
	cfg.Identity.KeyPath = filepath.Join(appDir, "identity.key")

	_ = os.MkdirAll(cfg.Storage.DatabasePath, 0755)
	_ = os.MkdirAll(cfg.Storage.DownloadsDir, 0755)

	return cfg
}
