+++
title = "Homebrew"
description = "Install speedle-spctl, speedle-pms and speedle-ads with Homebrew and run a local demo."
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Homebrew installs the command-line tool and both services, then runs them on 127.0.0.1 with one shared policy file."
+++

Install the Speedle command-line tool with [Homebrew](https://brew.sh/) on macOS or Linux. Homebrew does not install or start formulas from a tap outside the official Homebrew organization until that tap is trusted.

```bash
brew tap teramoby/tap
brew trust teramoby/tap
brew install teramoby/tap/spctl
```

The formula is named `spctl`. On macOS the command is `speedle-spctl`, so the install does not replace Apple Gatekeeper at `/usr/sbin/spctl`. On Linux the same program is also available as `spctl`.

The policy management service and the authorization decision service are separate formulas:

```bash
brew install teramoby/tap/speedle-pms
brew install teramoby/tap/speedle-ads
```

Install the command-line tool and both services together:

```bash
brew install teramoby/tap/speedle
```

Each of `spctl`, `speedle-pms` and `speedle-ads` installs the macOS or Linux release binary and checks it against that release's `checksums.txt`.

## Demo with Homebrew

Start both services. They listen on `127.0.0.1` and share one file policy store.

```bash
brew services start speedle-pms
brew services start speedle-ads
```

`speedle-pms` listens on `127.0.0.1:6733`. `speedle-ads` listens on `127.0.0.1:6734`. Both use `--store-type file` and the policy file `$(brew --prefix)/var/speedle/policies.json`. Logs are under `$(brew --prefix)/var/log/`. `--insecure=true` is for this loopback demo. See [Security](../../security/) for TLS.

Create a service and a policy. On Linux you can use `spctl` in place of `speedle-spctl`.

```bash
speedle-spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service mysvc
speedle-spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get,del res1" --service-name=mysvc
```

Ask ADS whether `user1` may get `res1`:

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

The response contains `"allowed":true`. The services read policy changes from the shared file. Stop them with:

```bash
brew services stop speedle-ads
brew services stop speedle-pms
```
