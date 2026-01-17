# radiobuenavia

CLI tool for Radio Buena Vida that automates:
- listing new Dropbox audio uploads
- running Audacity processing via scripting pipes
- re-encoding metadata/bitrate and optional jingles
- uploading to Dropbox post-process folders and archiving

## Requirements
- Audacity with scripting enabled
- `ffmpeg` and `ffprobe` available in `PATH`
- Dropbox app credentials with a refresh token
  
Enable the Audacity module `mod-script-pipe` in Preferences > Modules.
On Windows, scripting pipes must be available at `\\\\.\\pipe\\ToSrvPipe` and `\\\\.\\pipe\\FromSrvPipe`.

## Dependencies

```bash
go mod tidy
```

## Build

```bash
go build -o rbv ./cmd/rbv
```

## Build (static Windows)

```powershell
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o rbv.exe ./cmd/rbv
```

## Build (static Linux)

```bash
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o rbv ./cmd/rbv
```

## Install

```bash
go build -o rbv ./cmd/rbv
install -m 755 rbv /usr/local/bin/rbv
```

```powershell
go build -o rbv.exe ./cmd/rbv
Copy-Item .\rbv.exe $env:USERPROFILE\bin\
```

## Run

```bash
./rbv -config ./config.toml
```

## Init

Generate a config file interactively:

```bash
./rbv init -config ./config.toml
```

## Doctor

Check Dropbox access and configured paths:

```bash
./rbv doctor -config ./config.toml
```
`rbv doctor` exits non-zero if `ffmpeg` or `ffprobe` are missing.

## Config

Create `config.toml` in the working directory (or point to it with `-config`).
`jingles_dir` will be scanned for `.mp3` files and combined with any explicit `jingles` entries.
Only `.mp3` files are processed.

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
jingles_dir = "/path/to/jingles"
```
