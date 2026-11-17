# radiobuenavia

CLI for processing Radio Buena Vida recordings from Dropbox. It downloads new
MP3s, processes them with FFmpeg, optionally prepends a jingle, then uploads the
results to the configured Dropbox output and archive folders.

## Requirements

- Native `ffmpeg` and `ffprobe` on `PATH`, with `libmp3lame` and the required audio filters.
- Dropbox app credentials and a refresh token.

## Build and run

```bash
go build -o rbv ./cmd/rbv
./rbv init -config ./config.toml
./rbv doctor -config ./config.toml
./rbv -config ./config.toml
```

`init` sets up Dropbox access and writes the config. `doctor` checks Dropbox paths
and FFmpeg capabilities. A normal run lists new files and asks for confirmation.
Only MP3s are processed; archived output names are skipped.

Processing defaults to CPU count minus one, capped at four workers with a minimum
of one. Override with `-jobs N`:

```bash
./rbv -config ./config.toml -jobs 2
```

Downloads and uploads overlap processing. Live recordings finish before prerecords
start. Failures cancel active work and clean up temporary files; completed Dropbox
uploads remain in place.

For a Windows build without cgo, run in PowerShell:

```powershell
$env:CGO_ENABLED = "0"
go build -o rbv.exe ./cmd/rbv
```

FFmpeg and ffprobe must still be installed separately. Use `rbv version` to show
build information.

## Processing

| Recording | Processing |
| --- | --- |
| Live | Remove DC offset and normalize to −0.3 dBFS with shared stereo gain. |
| Prerecord | Compress at −12 dB, 2:1, with 2 s attack/9 s release; normalize to 0 dBFS, then limit to −4 dBFS without makeup gain. |

Prerecord processing approximates Audacity's legacy effects. Its original 3 s/10 s
compressor timing, noise-floor control and soft limiter do not map directly to
FFmpeg. The replacement limiter uses 6.2 ms lookahead and 50 ms release.

Output is 44.1 kHz stereo MP3, encoded once using lossless temporary files. Allow
space for these intermediates on long recordings. The bitrate follows the source,
rounded to a supported MP3 rate, with a 192 kb/s fallback. Recordings over 90 minutes
use 224 kb/s instead of the previous 210 kb/s request.

One random jingle is prepended after programme processing. Jingles and MP3 encoding
can exceed the programme's peak target. The artist tag comes from the output filename.

## Configuration

Use `init`, or copy [config.example.toml](config.example.toml) and fill in the values:

```toml
[auth]
app_key = ""
app_secret = ""
refresh_token = ""

[paths]
preprocess_live = "/automation/preprocessed/live"
preprocess_prerecord = "/automation/preprocessed/prerecord"
postprocess_soundcloud = "/automation/postprocessed"
postprocess_archive = "/automation/archive"
jingles = []
jingles_dir = ""
```

`jingles` accepts local file paths. `jingles_dir` adds MP3s from a local directory.
The SoundCloud path is a Dropbox staging folder; this tool does not publish directly
to SoundCloud.

## Development

```bash
go test ./...
prek install
prek run --all-files
```

CI runs hooks, tests and Linux/Windows builds. See [RELEASING.md](RELEASING.md)
for tagged releases.
