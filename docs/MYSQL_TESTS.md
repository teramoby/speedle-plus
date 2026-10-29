# MySQL regression tests

The MySQL backend implements `pms.PolicyStoreManager`. MySQL does not implement
the optional Discover interface. Tests for that capability verify the existing
unsupported response; the supported backends retain their Discover tests.

## Nightly and manual runs

`.github/workflows/nightly.yml` schedules the full nightly suite at 17:00 UTC
(01:00 Asia/Shanghai). GitHub schedules execute the workflow on the default
branch, so a workflow change in an unmerged PR does not activate the schedule.

Trusted nightly jobs use the `speedle-linux` self-hosted runner pool on charming:
`charming-speedle` and `charming-speedle-2`. The first runner's registered name
does not have a `-1` suffix. The worker holds a host-wide `flock` before checking
legacy fixed test ports and running tests, to serialize their use across the
two runner processes. It refuses occupied ports without stopping host services.
The protected reusable worker is loaded from `master`; candidate worker changes
take effect after merge.

To try the MySQL suite on a branch before merge:

```sh
gh workflow run nightly.yml --repo teramoby/speedle-plus \
  --ref codex/mysql-policy-store -f mysql_only=true
```

This manual mode runs MySQL and CI script checks. It skips the other nightly
backends and the notification/issue-report job. A normal scheduled run includes
file, etcd and MySQL, supported-toolchain Quick Start checks, govulncheck and
the CI script tests.

## What runs against MySQL

`scripts/ci/run-mysql-tests.py` uses Docker to start an owned MySQL 8.4 container
with a pinned image digest, a random password and a random loopback-only host
port. It builds native PMS, ADS and spctl from the candidate with the race
detector, and configures PMS and ADS to use the same database. The ADS poll
interval is 20ms for the existing 500ms cache-update waits.

The suites are:

- `mysql-go`: the ordinary product packages, plus test utilities, with
  `STORE_TYPE=mysql`. Existing evaluator, CLI, PMS REST/gRPC and ADS REST/gRPC
  fixtures use their original policy data in isolated MySQL databases.
- `mysql-ads-prepare`: the existing `runtime_test_prepare` fixture setup via
  spctl and native PMS. Legacy PDL fragments receive the complete current SPDL
  service/section headers. The originally disabled role-principal AND fixture
  is checked as unsupported instead of preventing all subsequent setup.
- `mysql-ads-runtime`: every test enabled by `runtime_test,runtime_cache_test`,
  including both `TestMats` and `TestLrg`, using native PMS/ADS. Preparation
  failure blocks this stage and fails the run. Authorization assertions are
  retained.
- `mysql-docs`: both step-by-step MySQL guides, executed against owned MySQL
  containers with native PMS/ADS, including read-only ADS, automatic schema
  creation, live synchronization, restart, and provisioning without runtime DDL.

The ordinary package selection follows the product CI gate: generated protobuf
packages, samples, the logging demo and the independently provisioned MongoDB
adapter are excluded. Store-specific file/etcd tests still exercise those
stores; only their applicable shared fixtures switch to MySQL.

MySQL unavailability, nonzero suite exits, native service exits or detected races
fail the run. Failed tests are not filtered out or converted into successful
results. JSON event counts include both parent tests and subtests.

## Fixture ownership and artifacts

The fixture owns its container, databases, temporary configuration, binaries and
native service processes. Cleanup stops these processes and removes only the
container with the current run/attempt label. The worker also retries owned
container cleanup in an `always()` step. No host MySQL installation is needed.

The runner account must already have access to Docker. If the account acquired
Docker group membership after its runner process started, the fixture uses `sg`
to refresh that existing membership for its child process. It does not edit
groups, change socket permissions or restart the runner.

The `nightly-results` artifact contains Go JSON events, stderr, native PMS/ADS
logs, Docker setup output, `mysql-summary.txt` and `checks.txt`. The generated
database password is redacted before upload. The test helper requires a DSN
whose database starts with `speedle_test`, and creates/drops only fresh owned
databases. Database creation privileges are needed for tests, independently of
the permissions of a deployed PMS/ADS account.

## Regression repairs and verification

At commit `8d4a7f8aed8b40bad9a707792579415036771ba3`, the
[charming runner trial](https://github.com/teramoby/speedle-plus/actions/runs/40841295197)
completed all three stages:

- Ordinary MySQL suites: 690 pass events, 0 failures, 1 optional Discover skip.
- Runtime preparation: passed.
- Tagged authorization/cache suites: 307 pass events and 27 fail events across
  7 top-level tests (20 failing leaf cases). Cache mutation/refresh cases passed.

A local file-store control run of the same tagged tests produced the same 27
failing test names and the same 307 pass events, with no MySQL-only failure.
The failures involve permission-list order, obsolete denial reason expectations,
invalid attribute responses, Boolean fixtures and built-in date expectations.
The failing cases were subsequently repaired without filtering tests or relaxing
Allow/Deny checks: permission responses are compared without imposing order
while still detecting missing/extra/duplicate permissions; explicit deny
expects DENY_POLICY_FOUND; invalid typed attributes assert HTTP 400 and the
actual error response schema; Boolean inputs use explicit types; month/day
fixtures use the numeric month and correct built-in day attribute.

The [charming verification after these repairs](https://github.com/teramoby/speedle-plus/actions/runs/40845891556)
passed all suites at commit `9332c3b1`: 1,032 pass events, no failures and one
optional Discover skip. The CI script checks passed 52 Python and 134 Node
tests. Event counts include parent tests and subtests. Later runs also execute
the bilingual MySQL setup instructions directly from the documentation.

## Shared regression and four-backend configuration

`scripts/ci/store_regression.py` now owns the common native PMS/ADS, CLI,
preparation and tagged regression stages. The MySQL launcher retains Docker
provisioning and delegates these stages to the same engine used by MongoDB.
`scripts/ci/run-store-tests.py` runs that engine against an explicitly
provisioned backend selected by `STORE_TYPE=file|etcd|mongodb|mysql`.

Integration fixtures share `storetest.PolicyStore` / `storetest.NewFixture`.
`SPEEDLE_TEST_STORE_PROPS` accepts a native JSON connection-property map;
`SPEEDLE_MONGO_URI`, `SPEEDLE_MYSQL_TEST_DSN` and `SPEEDLE_ETCD_ENDPOINT` are
connection fallbacks. Fixtures generate isolated databases or key prefixes.
For `STORE_TYPE=etcd`, `run-store-tests.py` requires an endpoint on a port other
than 2379/2380: the engine refuses to start while those ports are in use, because
the etcd implementation tests start their own server there.
MongoDB/MySQL database prefixes must start with `speedle_test`. Invalid or unknown
backends fail instead of falling back to file. Optional Discover is checked
through the store's interface, independently of the selected backend name.

The ordinary stage executes `go test -race -json -p 1 -count=1 -timeout 600s ./...`
without package exclusions. Implementation-specific backend tests still exercise
their own adapters; independently provisioned database integration tests may skip
when their database is absent. Skips and failures are included in the summary.
The engine then builds/starts native PMS and ADS over one database, runs fixture
preparation, and executes every tagged ADS runtime/cache test.

MySQL remains on charming. MongoDB validation uses `runs-on: fiona-speedle` and
an owned macOS ARM64 replica set on fiona. This does not change the runner-group
workflow allowlist or merge/deploy either backend automatically.
