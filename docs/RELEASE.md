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

A failed image build or smoke test stops the run before a tag exists. A failed
push leaves the tag and the GitHub release; the next dispatch of the same
version finishes the push.

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

## Docker Hub credentials

The release job uses the `release` environment. That environment must define
both of these secrets:

- `DOCKERHUB_USERNAME`
- `DOCKERHUB_TOKEN`

The workflow stops with an error before it creates a tag when either secret is
missing. The token is passed to `docker login docker.io --password-stdin` in
the login step. No other step reads the username or the token. GHCR login uses
the workflow's `GITHUB_TOKEN`.

## Binaries

Each GitHub release attaches `spctl`, `speedle-spctl` (the same bytes as
`spctl`), `speedle-pms` and `speedle-ads` for macOS and Linux on amd64 and
arm64, named `<command>_<os>_<arch>`, plus `checksums.txt`.

## Local images

`make image` builds `deployment/docker/speedle-pms` and
`deployment/docker/speedle-ads`. Those Dockerfiles are local development
images. The Release workflow does not publish them.
