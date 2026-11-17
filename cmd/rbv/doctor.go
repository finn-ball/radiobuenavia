package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"radiobuenavia/internal/audio"
	"radiobuenavia/internal/config"
	"radiobuenavia/internal/dropbox"
)

func runDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	configPath := fs.String("config", "./config.toml", "path to config file")
	_ = fs.Parse(args)

	log.Print("Running doctor checks...")
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	dbx, err := dropbox.NewClient(cfg.Auth.AppKey, cfg.Auth.AppSecret, cfg.Auth.RefreshToken)
	if err != nil {
		log.Fatalf("dropbox client error: %v", err)
	}

	acct, err := dbx.GetCurrentAccount()
	if err != nil {
		log.Printf("dropbox account check failed: %v", err)
	} else {
		log.Printf("dropbox account: %s (%s)", acct.Name, acct.Email)
	}

	checkPath(dbx, "preprocess_live", cfg.Paths.PreprocessLive)
	checkPath(dbx, "preprocess_prerecord", cfg.Paths.PreprocessPrerecord)
	checkPath(dbx, "postprocess_soundcloud", cfg.Paths.PostprocessSoundcloud)
	checkPath(dbx, "postprocess_archive", cfg.Paths.PostprocessArchive)

	checkJingles(cfg.Paths.Jingles, cfg.Paths.JinglesDir)
	if err := audio.CheckTools(); err != nil {
		log.Fatalf("doctor audio tools check failed: %v", err)
	}
	log.Print("audio tools: FFmpeg filters, MP3 encoder and ffprobe ok")
}

func checkPath(dbx *dropbox.Client, label, path string) {
	if strings.TrimSpace(path) == "" {
		log.Printf("%s: not set", label)
		return
	}
	files, err := dbx.ListFiles(path)
	if err != nil {
		log.Printf("%s: error (%s): %v", label, path, err)
		return
	}
	log.Printf("%s: ok (%s) files=%d", label, path, len(files))
}

func checkJingles(jingles []string, jinglesDir string) {
	if len(jingles) == 0 && strings.TrimSpace(jinglesDir) == "" {
		log.Print("jingles: none configured")
		return
	}
	for _, path := range jingles {
		if _, err := os.Stat(path); err != nil {
			log.Printf("jingle file missing: %s (%v)", path, err)
		} else {
			log.Printf("jingle file ok: %s", path)
		}
	}
	if strings.TrimSpace(jinglesDir) != "" {
		entries, err := os.ReadDir(jinglesDir)
		if err != nil {
			log.Printf("jingles_dir error: %s (%v)", jinglesDir, err)
			return
		}
		count := 0
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.ToLower(filepath.Ext(entry.Name())) == ".mp3" {
				count++
			}
		}
		log.Printf("jingles_dir ok: %s (mp3 files=%d)", jinglesDir, count)
	}
}
