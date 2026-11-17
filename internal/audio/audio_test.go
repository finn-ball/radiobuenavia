package audio

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseStats(t *testing.T) {
	block := func(c int, mean, min, max string) string {
		return fmt.Sprintf("[Parsed_astats_1 @ 0] Channel: %d\n[Parsed_astats_1 @ 0] DC offset: %s\n[Parsed_astats_1 @ 0] Min level: %s\n[Parsed_astats_1 @ 0] Max level: %s\n", c, mean, min, max)
	}
	good := block(1, "0.1", "-0.2", "0.4") + block(2, "0", "0", "0") + "[Parsed_astats_1 @ 0] Overall\n[Parsed_astats_1 @ 0] Number of samples: 100\n"
	stats, err := parseStats(block(1, "0", "-1", "1") + good)
	if err != nil || stats[0].mean != 0.1 || stats[1].max != 0 {
		t.Fatalf("final stereo/silence stats: %+v, %v", stats, err)
	}
	for _, bad := range []string{"", block(1, "0", "0", "0"), strings.Replace(good, "0.1", "NaN", 1), strings.Replace(good, "100", "0", 1), strings.Replace(good, "0.1", "broken", 1)} {
		if _, err := parseStats(bad); err == nil {
			t.Errorf("accepted malformed/missing statistics %q", bad)
		}
	}
}

func TestFFmpegProcessing(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not installed")
		}
	}
	if err := CheckTools(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "input.wav")
	// Unequal stereo amplitudes and offsets, including a nonzero final sample.
	if _, err := command(context.Background(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "aevalsrc=0.2+0.4*sin(2*PI*440*t)|-0.1+0.1*sin(2*PI*440*t):s=44100:d=1", "-c:a", "pcm_f32le", input); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(dir, "live.wav")
	if err := processProgramme(context.Background(), input, live, dir, true); err != nil {
		t.Fatal(err)
	}
	s, err := analyze(context.Background(), live)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range s {
		if math.Abs(c.mean) > 2e-6 {
			t.Errorf("DC not removed: %+v", s)
		}
	}
	if math.Abs(s[0].max-math.Pow(10, -0.3/20)) > 2e-5 || math.Abs(s[1].max/s[0].max-0.25) > 2e-5 {
		t.Errorf("live peak/stereo gain: %+v", s)
	}
	prerecord := filepath.Join(dir, "prerecord.wav")
	if err := processProgramme(context.Background(), input, prerecord, dir, false); err != nil {
		t.Fatal(err)
	}
	s, err = analyze(context.Background(), prerecord)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range s {
		if math.Max(math.Abs(c.min), math.Abs(c.max)) > 0.63096 || c.samples != 44100 {
			t.Errorf("prerecord ceiling/duration: %+v", s)
		}
	}
	// A DC jingle makes prepend order distinguishable from the DC-corrected programme.
	jingle := filepath.Join(dir, "jingle.wav")
	if _, err := command(context.Background(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "aevalsrc=0.1:s=32000:d=0.25", "-c:a", "pcm_f32le", jingle); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "out.mp3")
	if err := Process(context.Background(), input, output, true, "Test Artist", []string{jingle}); err != nil {
		t.Fatal(err)
	}
	out, err := command(context.Background(), "ffmpeg", "-v", "info", "-i", output, "-af", "atrim=start=0.05:end=0.15,"+stereo+","+statistics, "-f", "null", "-")
	if err != nil {
		t.Fatal(err)
	}
	first, err := parseStats(out)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(first[0].mean) < 0.05 {
		t.Errorf("jingle not first: %+v", first)
	}
	duration, _, err := probeAudio(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(duration-1.25) > 0.06 {
		t.Errorf("combined duration: %g", duration)
	}
}
