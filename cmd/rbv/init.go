package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"radiobuenavia/internal/config"
	"radiobuenavia/internal/dropbox"
)

func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	configPath := fs.String("config", "./config.toml", "path to config file")
	_ = fs.Parse(args)

	log.Print("This software is distributed under the GNU GENERAL PUBLIC LICENSE agreement.")
	log.Print("This software comes with absolutely no warranty or liability.")
	log.Print("More information can be found in the LICENSE file.")
	log.Print(art)

	reader := bufio.NewReader(os.Stdin)
	appKey := prompt(reader, "Dropbox app key")
	appSecret := prompt(reader, "Dropbox app secret")

	fmt.Println("\nOpen this URL in your browser, authorize the app, and copy the code:")
	fmt.Printf("https://www.dropbox.com/oauth2/authorize?client_id=%s&token_access_type=offline&response_type=code\n\n", appKey)
	code := prompt(reader, "Authorization code")

	refreshToken, err := dropbox.ExchangeAuthCode(appKey, appSecret, code)
	if err != nil {
		log.Fatalf("auth code exchange failed: %v", err)
	}

	preprocessLive := promptDefault(reader, "Preprocess live path", "/automation/preprocessed/live")
	preprocessPrerecord := promptDefault(reader, "Preprocess prerecord path", "/automation/preprocessed/prerecord")
	postprocessSoundcloud := promptDefault(reader, "Postprocess soundcloud path", "/automation/postprocessed")
	postprocessArchive := promptDefault(reader, "Postprocess archive path", "/automation/archive")
	jinglesDir := promptDefault(reader, "Jingles folder (optional)", "")
	jinglesRaw := promptDefault(reader, "Extra jingle paths (comma-separated, optional)", "")
	jingles := parseList(jinglesRaw)

	cfg := config.Config{
		Auth: config.AuthConfig{
			AppKey:       appKey,
			AppSecret:    appSecret,
			RefreshToken: refreshToken,
		},
		Paths: config.PathsConfig{
			PreprocessLive:        preprocessLive,
			PreprocessPrerecord:   preprocessPrerecord,
			PostprocessSoundcloud: postprocessSoundcloud,
			PostprocessArchive:    postprocessArchive,
			Jingles:               jingles,
			JinglesDir:            jinglesDir,
		},
	}

	if err := ensureDir(filepath.Dir(*configPath)); err != nil {
		log.Fatalf("config directory error: %v", err)
	}
	if fileExists(*configPath) {
		overwrite := promptDefault(reader, fmt.Sprintf("%s exists. Overwrite? (y/N)", *configPath), "n")
		if !strings.EqualFold(overwrite, "y") {
			log.Print("Aborted.")
			return
		}
	}

	if err := config.Save(*configPath, cfg); err != nil {
		log.Fatalf("failed to write config: %v", err)
	}
	log.Printf("Wrote %s", *configPath)
}

func prompt(reader *bufio.Reader, label string) string {
	for {
		fmt.Printf("%s: ", label)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
}

func promptDefault(reader *bufio.Reader, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}

func parseList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		val := strings.TrimSpace(part)
		if val != "" {
			out = append(out, val)
		}
	}
	return out
}

func ensureDir(path string) error {
	if path == "." || path == "" {
		return nil
	}
	return os.MkdirAll(path, 0o755)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
