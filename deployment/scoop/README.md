# Speedle Scoop Manifest (`spctl`)

`deployment/scoop/spctl.json.tmpl` is the template rendered by `deployment/winget/update-windows-manifests.py` during the Release workflow (`.github/workflows/release.yml`).

Starting with the first release published after `#320` merges (`v1.9.0` and earlier releases do not include Windows binaries), each GitHub Release attaches `spctl_windows_amd64.exe`, `spctl_windows_arm64.exe`, and a rendered `spctl.json` manifest whose SHA-256 values come from `checksums.txt`.

## Direct installation from a published release

Once a release `v<X.Y.Z>` includes `spctl.json`, install `spctl` on Windows (`x64` or `arm64`) directly from that release's manifest URL:

```powershell
scoop install https://github.com/teramoby/speedle-plus/releases/download/v<X.Y.Z>/spctl.json
spctl --help
```

The manifest installs `spctl.exe` and exposes both `spctl` and `speedle-spctl` shims.

## External bucket status

No separate `teramoby/scoop-bucket` repository or `ScoopInstaller/Main` catalog entry is published by this repository today. Publishing a dedicated Scoop bucket is an external repository-administration step: create `teramoby/scoop-bucket` (`main` branch) and copy the release's `spctl.json` to `bucket/spctl.json`.
