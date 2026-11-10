# Releasing Speedle

The Release workflow (`.github/workflows/release.yml`) is started by hand. It
tags one commit already on `master`, builds the release binaries and one
container image, publishes a GitHub release, and pushes that image. Dispatch it
from `master`. The `release` environment used by the workflow accepts
deployments from `master`.

```sh
gh workflow run release.yml --repo teramoby/speedle-plus \
  --ref master -f version=vX.Y.Z -f commit=<40-character lowercase SHA>
```

`version` is the git tag (`vX.Y.Z`). `commit` is the full SHA to tag. The
workflow rejects a version that does not match `vX.Y.Z`, a SHA that is not on
`master`, and a tag that already points somewhere else. Go accepts a `v2` or
later tag only when `go.mod` at that commit ends in the major version.

Re-dispatch the same version and commit to resume a failed run. An existing tag
is left in place when it already points at that commit. Image tags are
published again. Release assets are replaced. The workflow does not move a tag
or create a second GitHub release.

## Order

1. Check the version, the commit and the Docker Hub credentials.
2. Build the binaries and the image, then start PMS and ADS from that image
   against one shared volume.
3. Create the git tag and the GitHub release.
4. Log in and push the image.
5. Package the Helm chart and push it to the chart registry.
6. Update the Homebrew tap when `HOMEBREW_TAP_TOKEN` is set.

A failed image build or smoke test stops the run before a tag exists. A failed
push leaves the tag and the GitHub release; the next dispatch of the same
version finishes the push. A failed chart push is the same: dispatch the same
version again and the chart is published from that commit. A commit that has
no `deployment/helm/speedle` chart skips the chart push. A failed Homebrew tap
update leaves the tag, the GitHub release, the image and the chart; the next
dispatch of the same version finishes the tap update.

## Smoke test

`scripts/release-smoke.sh` starts `speedle-pms` and then `speedle-ads` from
the local image `speedle:smoke`, using one shared volume. It retries each
request until the service answers or 30 seconds pass. Each attempt stops
after at most 5 seconds, and sooner when less time remains, so an accepted
connection that never answers cannot wait past the deadline. A connection
reset is retried: the published port accepts connections before the process
listens.
On failure the script prints `docker ps -a` and the logs of the PMS and ADS
containers.

The Release workflow builds `linux/amd64` and `linux/arm64`, loads the image
for the job's architecture, and runs that script before it creates a tag.

A pull request that changes `deployment/container/`, `cmd/`,
`.github/workflows/release.yml`, `.github/workflows/image-smoke.yml` or
`scripts/release-smoke.sh` builds the image for the job's architecture and
runs the same script.

## Container image

One image contains `speedle-pms`, `speedle-ads` and `spctl`. It is built from
`deployment/container/Dockerfile` for `linux/amd64` and `linux/arm64`. The base
is scratch. The process runs as UID 65532. Ports 6733 and 6734 are exposed.
`/data` is the writable file-store directory.

The image is pushed to both registries:

- `docker.io/speedle/speedle`
- `ghcr.io/teramoby/speedle`

The git tag stays `vX.Y.Z`. Image tags have no `v`:

| Git tag | Image tags |
| --- | --- |
| `vX.Y.Z` | `X.Y.Z` on both registries |
| highest `vX.Y.*` patch | also `X.Y` on both registries |
| highest `vX.Y.Z` on the newest `master` commit | also `latest` on both registries |

`X.Y` moves only when this version is the highest patch for that minor.
Retrying an older patch publishes `X.Y.Z` again and leaves `X.Y` where it is.
`latest` moves only when this version is the highest `vX.Y.Z` tag and the
requested commit is still the newest commit on `master`. A maintenance release
or a release from an older master commit leaves `latest` unchanged.

The command selects the program. With no command, the image prints:

```text
speedle-pms
speedle-ads
spctl
```

```sh
docker run --rm docker.io/speedle/speedle:X.Y.Z
docker run --rm docker.io/speedle/speedle:X.Y.Z speedle-pms --help
docker run --rm docker.io/speedle/speedle:X.Y.Z speedle-ads --help
docker run --rm docker.io/speedle/speedle:X.Y.Z spctl --help
```

`ghcr.io/teramoby/speedle` is the same image. The push build attaches maximum
provenance and an SBOM (`docker buildx build --provenance=mode=max --sbom=true`).

The image has no CA certificates. Pass a CA file when a service calls a TLS
endpoint.

## Helm chart

The chart in `deployment/helm/speedle` runs PMS and ADS from
`speedle/speedle`. After the image push, the workflow packages that chart
with `helm package --version X.Y.Z --app-version X.Y.Z` and pushes it to
`oci://ghcr.io/teramoby/charts/speedle`. `X.Y.Z` is the image version. Helm
3.22.1 publishes the package. Install it with Helm 3.8 or newer:

```sh
helm install speedle oci://ghcr.io/teramoby/charts/speedle --version X.Y.Z
```

The first release whose commit contains the chart is the first chart
package. Re-running an older commit does not publish a chart.

Pull requests that change the chart, the chart workflow or this release
workflow run `helm lint`, `helm template` for the file, etcd and MongoDB
stores, and an install on a throwaway kind cluster. The install waits until
the PMS and ADS pods are ready, creates a service and a policy with `spctl`
from the image, and checks that ADS returns `allowed: true`.

### Artifact Hub

Listing is a separate step from the chart push. To list
`oci://ghcr.io/teramoby/charts/speedle`:

1. Open the new `charts/speedle` package on GHCR and set its visibility to
   public. A new GHCR package is private until that change. Helm can install
   a private package only after `helm registry login`.
2. In the Artifact Hub control panel, add a Helm charts repository with the
   URL `oci://ghcr.io/teramoby/charts/speedle`.
3. Copy the repository ID shown on that repository.
4. Edit `deployment/helm/artifacthub-repo.yml`. Set `repositoryID` to that
   ID. Set the owner email to the Artifact Hub account that will claim the
   repository. The `owners` section alone is enough to claim ownership
   before the ID is known.
5. Push the metadata file with [oras](https://oras.land) to the tag
   `artifacthub.io`:

```sh
oras push ghcr.io/teramoby/charts/speedle:artifacthub.io \
  --config /dev/null:application/vnd.cncf.artifacthub.config.v1+yaml \
  artifacthub-repo.yml:application/vnd.cncf.artifacthub.repository-metadata.layer.v1.yaml
```

Artifact Hub reads that tag when it next processes the repository. The
account named in `owners` claims the repository from the control panel.
This repository does not register the chart.


## Docker Hub credentials

The release job uses the `release` environment. That environment must define
both of these secrets:

- `DOCKERHUB_USERNAME`
- `DOCKERHUB_TOKEN`

The workflow stops with an error before it creates a tag when either secret is
missing. The token is passed to `docker login docker.io --password-stdin` in
the login step. No other step reads the username or the token. GHCR login uses
the workflow's `GITHUB_TOKEN`.

## Homebrew

`deployment/homebrew/` is the source of truth for the
[`teramoby/homebrew-tap`](https://github.com/teramoby/homebrew-tap) repository.
It contains formulas for `spctl`, `speedle-pms`, `speedle-ads` and `speedle`.
`spctl`, `speedle-pms` and `speedle-ads` each install the release binary for
the host system and check the SHA-256 sum from that release's `checksums.txt`.
`speedle` depends on those three formulas.

```sh
brew tap teramoby/tap
brew trust teramoby/tap
brew install teramoby/tap/spctl
brew install teramoby/tap/speedle
```

Homebrew does not install or start formulas from a tap outside the official Homebrew organization until that tap is trusted.

The `spctl` formula installs the `speedle-spctl` command. On Linux it also
installs `spctl`. On macOS it does not install a command named `spctl`,
because `/usr/sbin/spctl` is Apple Gatekeeper.

`brew services start speedle-pms` listens on `127.0.0.1:6733`.
`brew services start speedle-ads` listens on `127.0.0.1:6734`. Both use the
file store `$(brew --prefix)/var/speedle/policies.json` and write logs under
`$(brew --prefix)/var/log/`. The demo is on the
[Homebrew](https://engine.speedle.io/docs/deployment/homebrew/) page.

After the image is published, the workflow rewrites the formula URLs and
checksums from the binaries it just built. When the `release` environment
defines `HOMEBREW_TAP_TOKEN`, the workflow pushes a `homebrew-vX.Y.Z` branch
to `teramoby/homebrew-tap` and opens a pull request against `main`. It does
not push `main` or `master`. It also opens a pull request on this repository
so `deployment/homebrew/` matches that update. That pull request also points
both Quick Start guides at this release and refreshes the committed guide
pages, including `llms-full.txt`. The token needs permission to
push a branch and open a pull request on `teramoby/homebrew-tap`. The pull
request on this repository uses the workflow's `GITHUB_TOKEN`. The tap token
is read only by that publish step.

When `HOMEBREW_TAP_TOKEN` is unset, the workflow logs that the Homebrew tap
update was skipped and the release still succeeds. `brew install` reads the
tap's `main` branch, so `teramoby/homebrew-tap` must already exist and its
default branch must be `main` before the token is set.

## Binaries

Each GitHub release attaches `spctl`, `speedle-spctl` (the same bytes as
`spctl`), `speedle-pms` and `speedle-ads` for macOS and Linux on amd64 and
arm64, named `<command>_<os>_<arch>`, plus `checksums.txt`.

## Local images

`make image` builds `deployment/docker/speedle-pms` and
`deployment/docker/speedle-ads`. Those Dockerfiles are local development
images. The Release workflow does not publish them.
