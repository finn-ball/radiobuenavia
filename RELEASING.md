# Releasing

Start from an up-to-date `main` with passing CI, then push a version tag:

```bash
git checkout main
git pull --ff-only
git tag -a v1.2.3 -m "v1.2.3"
git push origin v1.2.3
```

The [release workflow](.github/workflows/release.yml) builds Linux and Windows amd64
binaries, embeds version metadata, and publishes archives and `checksums.txt` to a
GitHub Release.

After downloading a release, verify its version and checksum:

```bash
./rbv version
sha256sum -c checksums.txt
```

Use the same steps with a new patch version for a hotfix.
