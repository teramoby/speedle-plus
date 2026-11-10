# Speedle Homebrew tap

These files are the source of truth for [teramoby/homebrew-tap](https://github.com/teramoby/homebrew-tap). They are maintained in [speedle-plus `deployment/homebrew`](https://github.com/teramoby/speedle-plus/tree/master/deployment/homebrew).

Install the Speedle command-line tool:

```sh
brew tap teramoby/tap
brew trust teramoby/tap
brew install teramoby/tap/spctl
```

Homebrew does not install or start formulas from a tap outside the official Homebrew organization until that tap is trusted.

The formula is named `spctl`. On macOS the command is `speedle-spctl`, so the install does not replace Apple Gatekeeper at `/usr/sbin/spctl`. On Linux the same program is also available as `spctl`.

Install the command-line tool and both services together:

```sh
brew install teramoby/tap/speedle
```

`brew services start speedle-pms` and `brew services start speedle-ads` run the services on `127.0.0.1:6733` and `127.0.0.1:6734`. The [Homebrew](https://engine.speedle.io/docs/deployment/homebrew/) page shows the demo.

Each of `spctl`, `speedle-pms` and `speedle-ads` installs the macOS or Linux release binary from [speedle-plus releases](https://github.com/teramoby/speedle-plus/releases) and checks it against that release's `checksums.txt`.
