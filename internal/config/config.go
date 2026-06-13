package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Listen            string `toml:"listen"`
	DBPath            string `toml:"db_path"`
	AssetsDir         string `toml:"assets_dir"`
	LogDir            string `toml:"log_dir"`
	Debug             bool   `toml:"debug"`
	OwnerSteam64ID    string `toml:"owner_steam64id"`
	SessionKey        string `toml:"session_key"`
	SessionTTLSeconds int    `toml:"session_ttl_seconds"`
}

// Load 读取 toml；不存在时使用全部默认值并把 owner 留空（启动会拒绝）。
func Load(path string) (*Config, error) {
	cfg := &Config{
		Listen:            "127.0.0.1:8090",
		DBPath:            "data/stock.db",
		AssetsDir:         "assets",
		LogDir:            "logs",
		SessionTTLSeconds: 30 * 24 * 3600,
	}
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, cfg); err != nil {
				return nil, fmt.Errorf("decode %s: %w", path, err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if cfg.OwnerSteam64ID == "" {
		// 允许空 owner 启动，但首次访问后台会提示需要修复 config.toml。
		// 这里给一个 dev 默认，方便本机测试。
		if env := os.Getenv("FOXHOLE_OWNER_STEAM64ID"); env != "" {
			cfg.OwnerSteam64ID = env
		} else {
			cfg.OwnerSteam64ID = "76561198000000000"
		}
	}
	if cfg.SessionKey == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		cfg.SessionKey = hex.EncodeToString(buf)
	}
	if cfg.SessionTTLSeconds <= 0 {
		cfg.SessionTTLSeconds = 30 * 24 * 3600
	}
	return cfg, nil
}