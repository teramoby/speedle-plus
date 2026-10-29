# Website languages and builds

The public English site uses `/`; Chinese pages use `/zh/` with the same logical
path. Page and taxonomy counterparts must exist in both languages. Language
switches link to the current page's counterpart, including pagination; ordinary
internal links stay in the current language. URLs carry the language, so no
cookie, browser storage or language-detection redirect is needed.
The header shows a single direct link to the other language (`English` on Chinese
pages, `中文` on English pages), including inside the mobile navigation menu.

## Sources

- `docs/hugo/content/`: paired `name.md` and `name.zh.md` content.
- `docs/hugo/static/index.html` and `zh/index.html`: approved modern homepages.
- `docs/hugo/static/playground/`: shared Playground modules, styling, engine and
  English HTML; `static/zh/playground/index.html`: Chinese HTML using the same
  engine. `i18n.js` translates display text only, retaining request values and
  original engine diagnostics.
- `docs/hugo/layouts/partials/header.html`: reuses the approved English/Chinese
  homepage headers for current Hugo pages. `css/navigation.css` and
  `js/navigation.js` provide the same header layout and mobile interaction on
  homepages, Playground and documentation. Home-section links return to the
  localized homepage; the language router keeps language switches on the same
  page. Mobile links stay visible until their controller initializes, so primary
  navigation also works when JavaScript is disabled or the script fails to load.
  `localize-links.py` gives archived snapshots the same header: its links stay in
  the snapshot where it has the page (and anchor), and otherwise lead to the
  current site.

The merge-gating test job executes the English and Chinese Quick Start and
embedded deployment guides directly from `docs/hugo/content/` on every run.
`tests/check_quick_start.py` extracts the actual Bash blocks and exercises native
PMS/ADS; `tests/check_embedded.py` extracts the Bash, Go and SPDL blocks, builds
an external application against the documented public release and the current
candidate library, and checks Allow/Deny output for both. Both checkers use the
document blocks as executable inputs.
The gate uses its current Go toolchain; nightly Quick Start checks additionally
cover Go 1.26.10 and 1.27.3.
The Quick Start check performs a public shallow clone, checks out the captured CI revision, builds
the three executables, creates and lists policies, and checks direct/role grants
and denials. On Linux the documented service and client commands run in
unprivileged user/network/PID namespaces with a private loopback interface, so
existing host services are untouched; this needs util-linux and iproute2. The
checks run through `scripts/ci/run-go-tests.sh gate` on the test runner for every
PR and every master push, regardless of changed paths. Other jobs retain path
classification. The site/build checks also verify that generated HTML matches
the Hugo sources.

Both quick-start guides enable `copycommands` with the page-specific
`command-copy.css` and `command-copy.js` assets. Executable Bash blocks gain a
keyboard-accessible copy control and localized feedback; example output is left
alone. Code blocks use the dark `dracula` highlight style to match their dark
background; archived snapshots were recoloured to the same palette.

## Generate and publish

Use the existing Hugo 0.48 toolchain, and always render into a separate directory:

```sh
HUGO_BIN=/path/to/hugo-0.48 website/build-site.sh /tmp/speedle-site
```

The build renders Hugo then runs `localize-links.py`. This postprocessor handles
legacy relative URLs, pagination, and same-origin links written in Markdown or
HTML. It fails on missing counterparts or broken local anchor destinations and
does not rewrite external links or translate source code.

Copy the generated output into `docs/` without deleting existing legacy aliases,
then run the postprocessor on `docs/` as well. Navigation and language switches in
archived snapshots retain their version prefix, including pagination redirects.
When an archived translation is absent, a localized navigation shell explicitly
labels the original-language content; it never substitutes current-version text.
Commit source and generated output together. Pages continues to publish
`master:/docs`.

```sh
python3 website/localize-links.py docs
python3 website/localize-links.py docs --check
python3 website/tests/test_language_routes.py
```

The existing protected Go CI build job runs the Playground release gate, both
language form suites, route tests, a complete published-link audit, and a temporary
build with the checksum-pinned official Hugo 0.48 binary. Existing homepage/CSS
and source/output invariants remain checked. The language-navigation release
query on the legacy script and Playground entry module refreshes previously
cached browser code when this feature is deployed.
