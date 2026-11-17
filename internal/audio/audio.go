package audio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	stereo     = "aresample=44100,aformat=sample_fmts=fltp:channel_layouts=stereo"
	compressor = "acompressor=mode=downward:threshold=0.2511886432:ratio=2:attack=2000:release=9000:knee=1:makeup=1:detection=peak:link=maximum"
	limiter    = "alimiter=limit=0.6309573445:level_in=1:level_out=1:attack=6.2:release=50:level=false:latency=true"
	statistics = "astats=reset=0:measure_perchannel=DC_offset+Min_level+Max_level+Number_of_samples:measure_overall=Number_of_samples"
)

// CheckTools verifies the installed executables and processing options, once per run.
var CheckTools = sync.OnceValue(func() error {
	ctx := context.Background()
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("%s is required on PATH: %w", name, err)
		}
	}
	if _, err := command(ctx, "ffprobe", "-version"); err != nil {
		return err
	}
	// Exercise the actual filters and MP3 encoder; filter help exit codes are unreliable.
	filter := stereo + ",aeval=exprs='val(0)|val(1)':channel_layout=stereo," + compressor + "," + limiter + "," + statistics + ",asplit[a][b];[a][b]concat=n=2:v=0:a=1[out]"
	if _, err := ffmpeg(ctx, "error", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo:d=0.05", "-filter_complex", filter, "-map", "[out]", "-c:a", "libmp3lame", "-f", "mp3", "-"); err != nil {
		return fmt.Errorf("FFmpeg processing capability check failed (required filters and libmp3lame): %w", err)
	}
	return nil
})

func GetArtist(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// Process processes only the programme, optionally prepends one jingle, and encodes once.
// The output is published only after FFmpeg completes successfully.
// Cancellation stops active FFmpeg and ffprobe subprocesses.
func Process(ctx context.Context, input, output string, live bool, artist string, jingles []string) error {
	if err := CheckTools(); err != nil {
		return err
	}
	duration, bitrate, err := probeAudio(ctx, input)
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "rbv-audio-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(work)
	}()
	programme := filepath.Join(work, "programme.wav")
	if err := processProgramme(ctx, input, programme, work, live); err != nil {
		return err
	}

	return encodeMP3(ctx, programme, output, artist, bitrateToK(bitrate, duration), jingles)
}

func encodeMP3(ctx context.Context, programme, output, artist, bitrate string, jingles []string) error {
	tmp, err := os.CreateTemp(filepath.Dir(output), ".rbv-*.mp3")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	args := []string{"-y"}
	if len(jingles) > 0 {
		jingle := jingles[rand.Intn(len(jingles))]
		args = append(args, "-i", jingle, "-i", programme, "-filter_complex", "[0:a:0]"+stereo+",asetpts=PTS-STARTPTS[j];[1:a:0]"+stereo+",asetpts=PTS-STARTPTS[p];[j][p]concat=n=2:v=0:a=1[out]", "-map", "[out]")
	} else {
		args = append(args, "-i", programme, "-map", "0:a:0")
	}
	args = append(args, "-vn", "-map_metadata", "-1", "-metadata", "artist="+artist, "-ar", "44100", "-ac", "2", "-c:a", "libmp3lame", "-b:a", bitrate, tmpPath)
	if _, err := ffmpeg(ctx, "error", args...); err != nil {
		return err
	}
	// Windows cannot rename over an existing destination. Only remove it after encoding succeeds.
	if err := os.Remove(output); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmpPath, output); err != nil {
		return fmt.Errorf("publish processed audio: %w", err)
	}
	return nil
}

func processProgramme(ctx context.Context, input, output, work string, live bool) error {
	if live {
		stats, err := analyze(ctx, input)
		if err != nil {
			return err
		}
		return renderPCM(ctx, input, output, stereo+","+liveFilter(stats))
	}
	compressed := filepath.Join(work, "compressed.wav")
	if err := renderPCM(ctx, input, compressed, stereo+","+compressor); err != nil {
		return err
	}
	stats, err := analyze(ctx, compressed)
	if err != nil {
		return err
	}
	peak := 0.0
	for _, s := range stats {
		peak = math.Max(peak, math.Max(math.Abs(s.min), math.Abs(s.max)))
	}
	gain := 1.0
	if peak > 0 {
		gain = 1 / peak
	}
	return renderPCM(ctx, compressed, output, stereo+",volume="+number(gain)+":precision=double,"+limiter)
}

func renderPCM(ctx context.Context, input, output, filter string) error {
	_, err := ffmpeg(ctx, "error", "-y", "-i", input, "-map", "0:a:0", "-vn", "-af", filter, "-c:a", "pcm_f32le", "-rf64", "auto", output)
	return err
}

type channelStats struct{ mean, min, max, samples float64 }

func analyze(ctx context.Context, path string) ([2]channelStats, error) {
	out, err := ffmpeg(ctx, "info", "-i", path, "-map", "0:a:0", "-vn", "-af", stereo+","+statistics, "-f", "null", "-")
	if err != nil {
		return [2]channelStats{}, err
	}
	return parseStats(out)
}

const (
	statMean uint8 = 1 << iota
	statMin
	statMax
	statSamples
	allStats = statMean | statMin | statMax | statSamples
)

func parseStats(out string) ([2]channelStats, error) {
	var stats [2]channelStats
	var seen [2]uint8
	channel := -1
	for _, line := range strings.Split(out, "\n") {
		end := strings.Index(line, "] ")
		if end < 0 {
			continue
		}
		line = strings.TrimSpace(line[end+2:])
		if strings.HasPrefix(line, "Channel: ") {
			n, _ := strconv.Atoi(strings.TrimPrefix(line, "Channel: "))
			channel = n - 1
			if channel >= 0 && channel < 2 {
				seen[channel] = 0
			}
			continue
		}
		if line == "Overall" {
			channel = -2
			continue
		}
		// astats logs sample count only in Overall, even when requested per channel.
		// Both channels have the same frame count after the explicit stereo conversion.
		if channel == -2 && strings.HasPrefix(line, "Number of samples: ") {
			n, err := parseFinite(strings.TrimPrefix(line, "Number of samples: "))
			if err != nil || n <= 0 {
				return stats, fmt.Errorf("invalid FFmpeg sample count: %q", line)
			}
			for c := range stats {
				stats[c].samples = n
				seen[c] |= statSamples
			}
			continue
		}
		if channel < 0 || channel >= 2 {
			continue
		}
		key, val, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		bit := uint8(0)
		var dest *float64
		switch key {
		case "DC offset":
			dest, bit = &stats[channel].mean, statMean
		case "Min level":
			dest, bit = &stats[channel].min, statMin
		case "Max level":
			dest, bit = &stats[channel].max, statMax
		case "Number of samples":
			dest, bit = &stats[channel].samples, statSamples
		default:
			continue
		}
		v, err := parseFinite(val)
		if err != nil {
			return stats, fmt.Errorf("invalid FFmpeg statistic %s: %q", key, val)
		}
		*dest = v
		seen[channel] |= bit
	}
	for c, s := range stats {
		if seen[c] != allStats || s.samples <= 0 || s.min > s.max || s.mean < s.min-1e-6 || s.mean > s.max+1e-6 {
			return stats, fmt.Errorf("missing or invalid FFmpeg statistics for channel %d", c+1)
		}
	}
	return stats, nil
}

func liveFilter(stats [2]channelStats) string {
	peak := 0.0
	for _, s := range stats {
		peak = math.Max(peak, math.Max(math.Abs(s.min-s.mean), math.Abs(s.max-s.mean)))
	}
	gain := 1.0
	if peak > 0 {
		gain = math.Pow(10, -0.3/20) / peak
	}
	return "aeval=exprs='(val(0)-(" + number(stats[0].mean) + "))*" + number(gain) + "|(val(1)-(" + number(stats[1].mean) + "))*" + number(gain) + "':channel_layout=stereo"
}

func number(v float64) string { return strconv.FormatFloat(v, 'g', 17, 64) }

func probeAudio(ctx context.Context, path string) (float64, int, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-of", "json", "-show_entries", "format=duration,bit_rate:stream=bit_rate", "-select_streams", "a:0", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if _, err := runCommand(cmd); err != nil {
		return 0, 0, err
	}
	var parsed struct {
		Streams []struct {
			BitRate string `json:"bit_rate"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			BitRate  string `json:"bit_rate"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		return 0, 0, fmt.Errorf("parse ffprobe output: %w", err)
	}
	if len(parsed.Streams) == 0 {
		return 0, 0, fmt.Errorf("no audio stream in %q", path)
	}
	duration, err := parseFinite(parsed.Format.Duration)
	if err != nil || duration <= 0 {
		return 0, 0, fmt.Errorf("missing or invalid programme duration in %q", path)
	}
	bitrate := 192000
	for _, candidate := range []string{parsed.Streams[0].BitRate, parsed.Format.BitRate} {
		if n, err := strconv.Atoi(candidate); err == nil && n > 0 {
			bitrate = n
			break
		}
	}
	return duration, bitrate, nil
}

func bitrateToK(bitrate int, duration float64) string {
	if duration > 90*60 {
		return "224k"
	}
	if bitrate <= 0 {
		bitrate = 192000
	}
	rates := []int{32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	best := rates[0]
	for _, rate := range rates[1:] {
		if math.Abs(float64(bitrate)-float64(rate)*1000) < math.Abs(float64(bitrate)-float64(best)*1000) {
			best = rate
		}
	}
	return strconv.Itoa(best) + "k"
}

// Keep diagnostics bounded even for long programmes; astats emits final results at EOF.
type tailBuffer struct{ data []byte }

func (b *tailBuffer) Write(p []byte) (int, error) {
	const limit = 32768
	n := len(p)
	if len(p) >= limit {
		b.data = append(b.data[:0], p[len(p)-limit:]...)
		return n, nil
	}
	if len(b.data)+len(p) > limit {
		b.data = b.data[len(b.data)+len(p)-limit:]
	}
	b.data = append(b.data, p...)
	return n, nil
}
func (b *tailBuffer) String() string { return strings.TrimSpace(string(b.data)) }

func parseFinite(value string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("nonfinite number %q", value)
	}
	return v, nil
}

func ffmpeg(ctx context.Context, logLevel string, args ...string) (string, error) {
	return command(ctx, "ffmpeg", append([]string{"-hide_banner", "-nostdin", "-v", logLevel}, args...)...)
}

func command(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = io.Discard
	return runCommand(cmd)
}

func runCommand(cmd *exec.Cmd) (string, error) {
	var diagnostic tailBuffer
	cmd.Stderr = &diagnostic
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s failed: %w; %s", filepath.Base(cmd.Path), err, diagnostic.String())
	}
	return diagnostic.String(), nil
}
