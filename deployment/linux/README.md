# Speedle Linux Packages (`.deb` and `.rpm`)

`deployment/linux/` builds deterministic `.deb` and `.rpm` packages for `spctl`, `speedle-pms`, and `speedle-ads` on `amd64`/`x86_64` and `arm64`/`aarch64` from checksum-verified release binaries.

Package assets are attached to GitHub Releases starting with the first release created after this directory was added (`v1.9.0` and earlier releases contain raw macOS/Linux binaries only).

## Packages

| Package | `.deb` (`amd64`, `arm64`) | `.rpm` (`x86_64`, `aarch64`) | Contents |
| --- | --- | --- | --- |
| `spctl` | `spctl_<X.Y.Z>-1_<arch>.deb` | `spctl-<X.Y.Z>-1.<arch>.rpm` | `/usr/bin/spctl`, `/usr/bin/speedle-spctl -> spctl`, `/usr/share/doc/spctl/LICENSE` |
| `speedle-pms` | `speedle-pms_<X.Y.Z>-1_<arch>.deb` | `speedle-pms-<X.Y.Z>-1.<arch>.rpm` | `/usr/bin/speedle-pms`, `/usr/lib/systemd/system/speedle-pms.service`, `/etc/speedle/pms.json`, `/etc/default/speedle-pms`, `/usr/share/doc/speedle-pms/LICENSE` |
| `speedle-ads` | `speedle-ads_<X.Y.Z>-1_<arch>.deb` | `speedle-ads-<X.Y.Z>-1.<arch>.rpm` | `/usr/bin/speedle-ads`, `/usr/lib/systemd/system/speedle-ads.service`, `/etc/speedle/ads.json`, `/etc/default/speedle-ads`, `/usr/share/doc/speedle-ads/LICENSE` |

## Verify and install from a GitHub Release

Always verify downloaded packages against the release's `checksums.txt` before installing (`gh release upload --clobber` updates release files sequentially during workflow retries rather than atomically):

```sh
# Debian / Ubuntu (replace <X.Y.Z> and amd64/arm64)
curl -fsSLO "https://github.com/teramoby/speedle-plus/releases/download/v<X.Y.Z>/checksums.txt"
curl -fsSLO "https://github.com/teramoby/speedle-plus/releases/download/v<X.Y.Z>/spctl_<X.Y.Z>-1_amd64.deb"
curl -fsSLO "https://github.com/teramoby/speedle-plus/releases/download/v<X.Y.Z>/speedle-pms_<X.Y.Z>-1_amd64.deb"
curl -fsSLO "https://github.com/teramoby/speedle-plus/releases/download/v<X.Y.Z>/speedle-ads_<X.Y.Z>-1_amd64.deb"
grep -E ' (spctl|speedle-pms|speedle-ads)_<X.Y.Z>-1_amd64\.deb$' checksums.txt | sha256sum -c -
sudo apt-get install ./spctl_<X.Y.Z>-1_amd64.deb ./speedle-pms_<X.Y.Z>-1_amd64.deb ./speedle-ads_<X.Y.Z>-1_amd64.deb
```

```sh
# Fedora / RHEL / Rocky / AlmaLinux (replace <X.Y.Z> and x86_64/aarch64)
curl -fsSLO "https://github.com/teramoby/speedle-plus/releases/download/v<X.Y.Z>/checksums.txt"
curl -fsSLO "https://github.com/teramoby/speedle-plus/releases/download/v<X.Y.Z>/spctl-<X.Y.Z>-1.x86_64.rpm"
curl -fsSLO "https://github.com/teramoby/speedle-plus/releases/download/v<X.Y.Z>/speedle-pms-<X.Y.Z>-1.x86_64.rpm"
curl -fsSLO "https://github.com/teramoby/speedle-plus/releases/download/v<X.Y.Z>/speedle-ads-<X.Y.Z>-1.x86_64.rpm"
grep -E ' (spctl|speedle-pms|speedle-ads)-<X.Y.Z>-1\.x86_64\.rpm$' checksums.txt | sha256sum -c -
sudo dnf install ./spctl-<X.Y.Z>-1.x86_64.rpm ./speedle-pms-<X.Y.Z>-1.x86_64.rpm ./speedle-ads-<X.Y.Z>-1.x86_64.rpm
```

## Security and systemd behavior

- **Unprivileged service account**: `preinstall.sh` creates the system group and user `speedle` (`--home-dir /var/lib/speedle --no-create-home`, non-interactive `nologin`/`false` shell). If a `speedle` account or group already exists with UID/GID `0`, mismatched primary group, interactive shell, or a different home directory, installation fails rather than silently adopting it.
- **Disabled and stopped on initial install**: Installing `speedle-pms` or `speedle-ads` runs `systemctl daemon-reload` (when `/run/systemd/system` is present) and **never** enables or starts either service automatically. Review `/etc/speedle/pms.json` and `/etc/speedle/ads.json`, then start explicitly:
  ```sh
  sudo systemctl enable --now speedle-pms.service speedle-ads.service
  ```
- **Loopback-only defaults**: `/etc/speedle/pms.json` listens on `127.0.0.1:6733` and `/etc/speedle/ads.json` listens on `127.0.0.1:6734` using the shared file store `/var/lib/speedle/policies.json` and JSON logs on `stderr` (`journalctl -u speedle-pms -u speedle-ads`). Plain HTTP (`"insecure": "true"`) is solely for single-host loopback use; before binding a non-loopback address or running on an untrusted multi-user host, set `"insecure": "false"` and configure `certPath`, `keyPath`, `clientCertPath`, and `forceClientCert` (see `docs/hugo/content/docs/security.md`).
- **Read-only configuration**: `/etc/speedle` (`0750`) and `/etc/speedle/{pms,ads}.json` + `/etc/default/speedle-{pms,ads}` (`0640`) are owned by `root:speedle` and marked `config|noreplace`. The systemd units set `ProtectSystem=strict` and intentionally omit `ConfigurationDirectory=` so the `speedle` service process never gains write access to `/etc/speedle`.
- **Graceful stop**: Both units set `KillSignal=SIGINT` so `systemctl stop` triggers graceful HTTP, gRPC, and store shutdown.

## Upgrades, removal, purge, and manual data cleanup

- **Upgrades**: User edits to `/etc/speedle/*.json` and `/etc/default/speedle-*` and data under `/var/lib/speedle` are preserved. Active services are restarted via `systemctl try-restart`; stopped services remain stopped.
- **Removal and purge**: `apt remove`, `apt purge`, and `dnf remove` stop and disable the removed unit, remove package-owned files through standard `dpkg`/`rpm` semantics, and **always preserve `/var/lib/speedle`, `/var/lib/speedle/policies.json`, and the `speedle` system user/group**.
- **Explicit administrator cleanup** (only after removing both `speedle-pms` and `speedle-ads` and backing up any needed policies):
  ```sh
  sudo rm -rf /var/lib/speedle /var/log/speedle /etc/speedle
  sudo userdel speedle
  sudo groupdel speedle
  ```

## Local reproducible build

`build-packages.py` packages existing release binaries verified against `checksums.txt` using SHA-256-pinned `nFPM v2.49.0` (`install-nfpm.sh`):

```sh
NFPM_INSTALL_DIR=/tmp/nfpm-bin bash deployment/linux/install-nfpm.sh
python3 -I deployment/linux/build-packages.py \
  --version vX.Y.Z \
  --dist-dir dist \
  --checksums dist/checksums.txt \
  --license LICENSE \
  --output-dir dist \
  --nfpm-bin /tmp/nfpm-bin/nfpm \
  --source-date-epoch 1763368554
```
