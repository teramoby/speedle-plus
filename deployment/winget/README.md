# Speedle winget Manifests (`Teramoby.spctl`)

`deployment/winget/update-windows-manifests.py` renders multi-file WinGet `1.6.0` manifests (`Teramoby.spctl.yaml`, `Teramoby.spctl.installer.yaml`, `Teramoby.spctl.locale.en-US.yaml`) and packs them into `spctl-winget-manifests.zip` from `checksums.txt` during `.github/workflows/release.yml`.

Starting with the first release published after `#320` merges (`v1.9.0` and earlier releases do not include Windows binaries), each GitHub Release attaches `spctl_windows_amd64.exe`, `spctl_windows_arm64.exe`, and `spctl-winget-manifests.zip`.

## Validate and install locally from a release bundle

On Windows 10/11 with `winget`, verify `spctl-winget-manifests.zip` against `checksums.txt`, extract it, and run:

```powershell
Expand-Archive .\spctl-winget-manifests.zip -DestinationPath .\spctl-winget
winget validate --manifest .\spctl-winget\manifests\t\Teramoby\spctl\<X.Y.Z>
winget settings --enable LocalManifestFiles
winget install --manifest .\spctl-winget\manifests\t\Teramoby\spctl\<X.Y.Z>
spctl --help
```

## External `microsoft/winget-pkgs` catalog status

`Teramoby.spctl` is not yet listed in the upstream [`microsoft/winget-pkgs`](https://github.com/microsoft/winget-pkgs) community catalog. Submitting to `microsoft/winget-pkgs` is an external maintainer step requiring a fork of `microsoft/winget-pkgs` and Microsoft Contributor License Agreement (CLA) acceptance on the submitting GitHub account; extract `spctl-winget-manifests.zip` into the fork root (`manifests/t/Teramoby/spctl/<X.Y.Z>/`) and open a pull request upstream.
