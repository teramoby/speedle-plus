+++
title = "Docker image"
description = "Run speedle-pms, speedle-ads and spctl from the published Speedle image."
weight = 20
aliases = ["/docs/sdks/docker/"]
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "One image on Docker Hub and GHCR runs the policy manager, the decision service and spctl, with a shared file store."
lastmod = 2026-11-10T14:45:43+08:00
+++

Speedle publishes one image that contains `speedle-pms`, `speedle-ads` and `spctl`. The image is built `FROM scratch`, runs as UID 65532, and has no shell. The first argument selects the program.

The same image index is on both registries:

- Docker Hub: `docker.io/speedle/speedle`
- GHCR: `ghcr.io/teramoby/speedle`

Release 1.7.0 publishes the tags `1.7.0`, `1.7` and `latest` on both registries, for `linux/amd64` and `linux/arm64`.

## Pull the image

```bash
docker pull docker.io/speedle/speedle:1.7.0
docker pull ghcr.io/teramoby/speedle:1.7.0
```

`docker pull` prints the index digest. For 1.7.0 that digest is `sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0` on both registries.

## Choose a tag

`1.7.0` names this release. `1.7` moves when a higher 1.7 patch is published. `latest` moves when a higher release is published from the newest commit on `master`. A release retry can publish the same version tag again, so a tag is not an immutable build.

To read the index digest:

```bash
docker buildx imagetools inspect docker.io/speedle/speedle:1.7.0 --format '{{.Manifest.Digest}}'
```

For the 1.7.0 release, `1.7.0`, `1.7` and `latest` on both registries all print:

```text
sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0
```

Check the digest again before you deploy `1.7` or `latest`. To stay on one build, use the image name with `@` and that digest, for example `docker.io/speedle/speedle@sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0`.

## Commands in the image

With no arguments the image prints the three programs:

```bash
docker run --rm docker.io/speedle/speedle:1.7.0
```

```text
speedle-pms
speedle-ads
spctl
```

`docker run --rm ghcr.io/teramoby/speedle:1.7.0` prints the same lines. The commands below use the Docker Hub name. Substitute the GHCR name to use that copy. There is no shell in the image, so `docker exec` must call `/spctl`, `/speedle-pms` or `/speedle-ads` by absolute path.

## Run PMS and ADS

Create a named volume and start the policy manager. The server listens on `0.0.0.0:6733` inside the container. Publishing `127.0.0.1:6733` keeps the unauthenticated management API on this machine.

```bash
docker volume create speedle-policies
docker run -d --name speedle-pms \
  -p 127.0.0.1:6733:6733 \
  -v speedle-policies:/data \
  docker.io/speedle/speedle:1.7.0 \
  speedle-pms --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6733 --insecure=true
```

A new named volume is filled from the image's `/data` directory, which UID 65532 can write. PMS creates `/data/policies.json` when it is missing. The image has no CA certificates. `--insecure=true` is for this local HTTP example. See [Security](/docs/security/) for TLS.

Docker can accept the published port and reset the connection before the process listens. Retry every error, including that reset:

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6733/policy-mgmt/v1/service
```

The first attempt may print `curl: (56) Recv failure: Connection reset by peer`. A later attempt then returns the body. An empty store answers:

```text
[]
```

Start ADS on the same volume. It reads the file PMS writes, including later updates.

```bash
docker run -d --name speedle-ads \
  -p 127.0.0.1:6734:6734 \
  -v speedle-policies:/data \
  docker.io/speedle/speedle:1.7.0 \
  speedle-ads --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6734 --insecure=true
```

## Check health and readiness

PMS does not serve `/healthz`. A successful `GET /policy-mgmt/v1/service`, shown above, means the management API is accepting requests.

ADS serves liveness and readiness:

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/healthz
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/readyz
```

```text
{"status":"ok"}
{"ready":true,"servicesTotal":0,"servicesUsable":0}
```

`/healthz` answers `200 {"status":"ok"}` while the process is running. `/readyz` answers `200` when ADS can evaluate a request. A file store has no distribution state, so the service counts stay zero; `ready` is the field to check. The image has no shell and no `curl`, so these probes run on the host, or in the orchestrator, against the published ports. `http://localhost:6734/healthz` reaches the same published port.

## Create a policy with spctl

`/spctl` in the running PMS container talks to PMS on that container's own port:

```bash
docker exec speedle-pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
docker exec speedle-pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

```text
service created
{"name":"docs","type":"application","metadata":{"createtime":"2026-11-10T03:30:03Z"}}
policy created
{"id":"zhtbsgm54iiuwky4zgmq","name":"","effect":"grant","permissions":[{"resource":"/docs/public","actions":["get","list"]}],"principals":[["user:alice"]],"metadata":{"createtime":"2026-11-10T03:30:04Z"}}
```

PMS assigns `createtime` and the policy `id`, so those values differ on each run. This is the same service and policy as the [SDKs overview](/docs/sdks/#example-policy).

The same `spctl` binary can run in a second container that shares the PMS network namespace:

```bash
docker run --rm --network container:speedle-pms \
  docker.io/speedle/speedle:1.7.0 \
  spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
```

## Ask ADS for a decision

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"docs","resource":"/docs/public","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

```text
{"allowed":true,"reason":0}
```

`reason` `0` is `GRANT_POLICY_FOUND`. The same policy does not grant `delete`:

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"docs","resource":"/docs/public","action":"delete"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

```text
{"allowed":false,"reason":3}
```

`reason` `3` is `NO_APPLICABLE_POLICIES`. A policy written through PMS while ADS is already running is visible on the next request. See [Authorization Decisions](/docs/decisions/) for the request fields.

## Point an SDK at ADS

The SDK pages call `http://localhost:6734`. With the port published above, that address is this ADS. The Node.js client below uses `http://127.0.0.1:6734`, which is the address the container publishes. Create the example directory first, then save the client in that directory as `main.mjs`:

```bash
mkdir speedle-example
cd speedle-example
```

```javascript
import { Auth, SpeedleClient } from "@speedle/sdk";

const speedle = new SpeedleClient({
  backend: "speedle",
  endpoint: "http://127.0.0.1:6734",
  auth: Auth.none(),
  serviceName: "docs",
});
try {
  const allowed = await speedle.isAllowed("alice", "get", "/docs/public");
  console.log(`alice get /docs/public: allowed=${allowed}`);
} finally {
  await speedle.close();
}
```

Install the published package in `speedle-example` and run `main.mjs` there. The install is the one on the [Node.js SDK](/docs/sdks/node/) page, pinned to the same version:

```bash
npm init -y
npm pkg set type=module
npm install @speedle/sdk@0.2.1
node main.mjs
```

```text
alice get /docs/public: allowed=true
```

The other language pages take the same endpoint and service name. The SDKs accept plain `http://` only for a loopback host.

## Run both services with Compose

Save this file as `compose.yaml`. It is another way to run the same two processes and the same volume. If the containers from the earlier section are still running, stop them first so ports 6733 and 6734 are free. Removing that volume also removes its policy file.

```yaml
services:
  pms:
    image: docker.io/speedle/speedle:1.7.0
    command:
      - speedle-pms
      - --store-type
      - file
      - --filestore-loc
      - /data/policies.json
      - --endpoint
      - 0.0.0.0:6733
      - --insecure=true
    ports:
      - 127.0.0.1:6733:6733
    volumes:
      - speedle-policies:/data
  ads:
    image: docker.io/speedle/speedle:1.7.0
    command:
      - speedle-ads
      - --store-type
      - file
      - --filestore-loc
      - /data/policies.json
      - --endpoint
      - 0.0.0.0:6734
      - --insecure=true
    ports:
      - 127.0.0.1:6734:6734
    volumes:
      - speedle-policies:/data
volumes:
  speedle-policies:
```

```bash
docker compose -f compose.yaml up -d
```

`up -d` returns before PMS is listening. Wait for the PMS service list and for ADS `/healthz` and `/readyz` before creating anything. The first PMS attempt may reset the connection, as described above.

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6733/policy-mgmt/v1/service
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/healthz
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/readyz
docker compose -f compose.yaml exec -T pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
docker compose -f compose.yaml exec -T pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

Send the same `is-allowed` request. ADS sees the new policy without a restart.

```bash
docker compose -f compose.yaml down -v
```

`down -v` removes the Compose volume and the policies stored in it.

## Verify provenance

The publish build attaches provenance and an SBOM (`docker buildx build --provenance=mode=max --sbom=true`). Inspect the index:

```bash
docker buildx imagetools inspect docker.io/speedle/speedle:1.7.0
```

The index lists `linux/amd64`, `linux/arm64`, and two `unknown/unknown` manifests. Those unknown platforms are the attestation manifests. Each one sets `vnd.docker.reference.type` to `attestation-manifest` and `vnd.docker.reference.digest` to the platform image it describes. `docker buildx imagetools inspect ghcr.io/teramoby/speedle:1.7.0` prints the same platform and attestation digests.

## Stop the containers

```bash
docker rm -f speedle-pms speedle-ads
docker volume rm speedle-policies
```

`docker volume rm` deletes the policy file. Use `docker compose -f compose.yaml down -v` instead when the processes were started with Compose.
