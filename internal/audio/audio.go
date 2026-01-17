package audio

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	rng   = rand.New(rand.NewSource(time.Now().UnixNano()))
	rngMu sync.Mutex
)

type ffprobeOutput struct {
	Streams []struct {
		BitRate string `json:"bit_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

func GetArtist(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func ProcessMetadataAndBitrate(path, artist string, jingles []string) error {
	duration, bitrate, err := probeAudio(path)
	if err != nil {
		return err
	}
	bitrateK := bitrateToK(bitrate)
	if duration > 90*60 {
		bitrateK = "210k"
	}

	if len(jingles) > 0 {
		rngMu.Lock()
		jingle := jingles[rng.Intn(len(jingles))]
		rngMu.Unlock()
		return exportWithJingle(path, jingle, artist, bitrateK)
	}
	return exportWithMetadata(path, artist, bitrateK)
}

func probeAudio(path string) (float64, int, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-of", "json", "-show_entries", "format=duration,bit_rate:stream=bit_rate", "-select_streams", "a:0", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("ffprobe failed: %w; output: %s", err, strings.TrimSpace(string(out)))
	}
	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return 0, 0, err
	}
	duration := 0.0
	if parsed.Format.Duration != "" {
		if v, err := parseFloat(parsed.Format.Duration); err == nil {
			duration = v
		}
	}
	bitrate := 192000
	if len(parsed.Streams) > 0 && parsed.Streams[0].BitRate != "" {
		if v, err := parseInt(parsed.Streams[0].BitRate); err == nil && v > 0 {
			bitrate = v
		}
	} else if parsed.Format.BitRate != "" {
		if v, err := parseInt(parsed.Format.BitRate); err == nil && v > 0 {
			bitrate = v
		}
	}
	return duration, bitrate, nil
}

func exportWithMetadata(path, artist, bitrate string) error {
	tmpPath, err := tempOutput(path)
	if err != nil {
		return err
	}
	cmd := exec.Command("ffmpeg", "-y", "-i", path, "-vn", "-metadata", fmt.Sprintf("artist=%s", artist), "-codec:a", "libmp3lame", "-b:a", bitrate, tmpPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg export failed: %w; output: %s", err, strings.TrimSpace(string(out)))
	}
	return replaceFile(tmpPath, path)
}

func exportWithJingle(path, jingle, artist, bitrate string) error {
	tmpPath, err := tempOutput(path)
	if err != nil {
		return err
	}
	// Concatenate jingle audio (input 0) followed by the track (input 1).
	filter := "[0:a][1:a]concat=n=2:v=0:a=1[a]"
	cmd := exec.Command(
		"ffmpeg",
		"-y",
		"-i", jingle,
		"-i", path,
		"-filter_complex", filter,
		"-map", "[a]",
		"-metadata", fmt.Sprintf("artist=%s", artist),
		"-codec:a", "libmp3lame",
		"-b:a", bitrate,
		tmpPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg export with jingle failed: %w; output: %s", err, strings.TrimSpace(string(out)))
	}
	return replaceFile(tmpPath, path)
}

func tempOutput(path string) (string, error) {
	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return filepath.Join(dir, fmt.Sprintf("%s.tmp.mp3", base)), nil
}

func replaceFile(tmpPath, targetPath string) error {
	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmpPath, targetPath)
}

func bitrateToK(bitrate int) string {
	if bitrate <= 0 {
		return "192k"
	}
	return fmt.Sprintf("%dk", int(math.Round(float64(bitrate)/1000.0)))
}

func parseFloat(val string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(val), 64)
}

func parseInt(val string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(val))
	return parsed, err
}
