# Speedle Homebrew tap

These files are the source of truth for [teramoby/homebrew-tap](https://github.com/teramoby/homebrew-tap). They are maintained in [speedle-plus `deployment/homebrew`](https://github.com/teramoby/speedle-plus/tree/master/deployment/homebrew).

Install the Speedle command-line tool:

```sh
brew install teramoby/tap/spctl
```

The formula is named `spctl`. On macOS the command is `speedle-spctl`, so the install does not replace Apple Gatekeeper at `/usr/sbin/spctl`. On Linux the same program is also available as `spctl`.

The policy management service and the authorization decision service use the same tap:

```sh
brew install teramoby/tap/speedle-pms
brew install teramoby/tap/speedle-ads
```

Install the command-line tool and both services together:

```sh
brew install teramoby/tap/speedle
```

Each of `spctl`, `speedle-pms` and `speedle-ads` installs the macOS or Linux release binary from [speedle-plus releases](https://github.com/teramoby/speedle-plus/releases) and checks it against that release's `checksums.txt`.

## Demo with Homebrew

Start both services. They listen on `127.0.0.1` and share one file policy store.

```sh
brew services start speedle-pms
brew services start speedle-ads
```

Create a service and a policy. On Linux you can use `spctl` in place of `speedle-spctl`.

```sh
speedle-spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service mysvc
speedle-spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get,del res1" --service-name=mysvc
```

Ask ADS whether `user1` may get `res1`:

```sh
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

The response contains `"allowed":true`. The services read policy changes from the shared file. Stop them with:

```sh
brew services stop speedle-ads
brew services stop speedle-pms
```

The policy file is `$(brew --prefix)/var/speedle/policies.json`. Logs are under `$(brew --prefix)/var/log/`.
