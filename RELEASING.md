# Releasing

This project uses tag-based releases via GitHub Actions.

## Prerequisites

- Push access to `main`
- GitHub Actions enabled for this repo
- CI green on `main`

## Create a Release

1. Ensure local `main` is up to date:

```bash
git checkout main
git pull --ff-only
```

2. Create and push a SemVer tag (`vMAJOR.MINOR.PATCH`):

```bash
git tag -a v1.2.3 -m "v1.2.3"
git push origin v1.2.3
```

3. Wait for the `Release` workflow to complete.

## What the Release Workflow Does

For tag pushes matching `v*`, the workflow:

- builds `rbv` for:
  - linux/amd64
  - linux/arm64
  - darwin/amd64
  - darwin/arm64
  - windows/amd64
- embeds version metadata with `-ldflags`:
  - `main.version` (tag)
  - `main.commit` (SHA)
  - `main.date` (UTC timestamp)
  - `main.builtBy` (`github-actions`)
- packages artifacts
- generates `checksums.txt`
- publishes a GitHub Release with all artifacts attached

## Verify a Release

1. Download an artifact and run:

```bash
./rbv version
```

2. Confirm output contains the expected tag and commit.

3. Verify checksum (example):

```bash
sha256sum -c checksums.txt
```

## Hotfix Release

1. Create/fix branch from `main`
2. Merge fix to `main`
3. Tag next patch version (for example `v1.2.4`)
4. Push tag to trigger release
