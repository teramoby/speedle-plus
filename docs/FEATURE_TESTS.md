# Feature regression contracts

The full four-store regression (`scripts/ci/store_regression.py`) runs the same
assertions against file, etcd, MySQL 8.4 and MongoDB. A backend launcher provisions
only its own disposable database. Failures are retained across every stage.

* Ordinary `go test -race ./...` runs the shared parser/file/CLI grammar corpus,
  six built-in function contracts through embedded/REST/gRPC evaluators, typed
  custom-function cache identity, a Go-regexp differential oracle for grant and
  deny Policy and RolePolicy, 512 principal/IDD/condition combinations in two
  policy orders, generated role graphs with independent fixed-point expectations,
  500-deep role chains, 1,000-term conditions, permission/decision consistency and
  concurrent full-snapshot refresh.
* `feature-stress` adds 1,000 and 10,000 authorization plus role policies, checking
  first/middle/last users on both allow and deny paths with cold/warm evaluation.
* Fuzz targets `FuzzParseStatements`, `FuzzConditionCheck`, `FuzzCheckSPDL` run for
  five seconds each on every backend. Seed corpora run in normal unit tests too.
  Keep any failing input as a committed regression seed and fix the cause.
* Native PMS/ADS runtime tests consume the SPDL function fixtures as actual
  authorization requests. Fixtures alone are not function-execution coverage.
* Weekly adds the 100,000-policy/role capacity case with file storage. A single
  MongoDB document has a separate size limit, so this is not a claim about
  100,000 policies within a single MongoDB service document.

Normal nightly runs at 01:00 Asia/Shanghai. To validate a review branch on the
charming runners without sending the scheduled report:

```sh
gh workflow run nightly.yml --repo teramoby/speedle-plus \
  --ref codex/feature-coverage-audit -f stores_only=true
```

Inspect every `<backend>-summary.json`, not only the final suite. The report must
contain all four backends, feature stress, three fuzz stages and native runtime
results. The infrastructure tests require database availability in their owning
backend run; ordinary file runs can skip database-specific tests.

Behavioral contracts fixed by this regression: a parser never drops a condition
because its keyword is misspelled; quoted `#` is not a comment; regex indexing
cannot discard a Go-regexp match; typed function arguments cannot share a cache
entry accidentally; invalid sets return an error rather than panic; empty deny
actions cover every action in permission listings as well as IsAllowed. Names
keep literal backslashes, ordinary Policy AND principals remain supported, and
Role Policy retains its separate subject grammar.

Compatibility decisions: Sum()/Avg() still return zero; empty-set IsSubSet still
returns false; runtime condition errors still make that condition nonmatching.
Numeric built-ins now reject NaN/infinity, negative square roots and overflowing
results. Duplicate set elements do not change subset membership. JSON diagnostics
are checked by error type/location rather than Go-version-specific full messages.
