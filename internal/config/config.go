package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Auth  AuthConfig  `toml:"auth"`
	Paths PathsConfig `toml:"paths"`
}

type AuthConfig struct {
	AppKey       string `toml:"app_key"`
	AppSecret    string `toml:"app_secret"`
	RefreshToken string `toml:"refresh_token"`
}

type PathsConfig struct {
	PreprocessLive        string   `toml:"preprocess_live"`
	PreprocessPrerecord   string   `toml:"preprocess_prerecord"`
	PostprocessSoundcloud string   `toml:"postprocess_soundcloud"`
	PostprocessArchive    string   `toml:"postprocess_archive"`
	Jingles               []string `toml:"jingles"`
	JinglesDir            string   `toml:"jingles_dir"`
}

func Load(path string) (Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Auth.AppKey == "" || cfg.Auth.AppSecret == "" || cfg.Auth.RefreshToken == "" {
		return Config{}, fmt.Errorf("auth config is missing required fields")
	}
	if cfg.Paths.PreprocessLive == "" && cfg.Paths.PreprocessPrerecord == "" {
		return Config{}, fmt.Errorf("paths.preprocess_live or paths.preprocess_prerecord must be set")
	}
	if cfg.Paths.PostprocessSoundcloud == "" || cfg.Paths.PostprocessArchive == "" {
		return Config{}, fmt.Errorf("paths.postprocess_soundcloud and paths.postprocess_archive are required")
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := toml.NewEncoder(file)
	return encoder.Encode(cfg)
}
