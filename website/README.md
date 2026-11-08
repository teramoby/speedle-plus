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
  Documentation uses `css/docs.css` and `js/docs.js`: the Engine container,
  colours, type scale and spacing, a fully expanded left navigation in normal
  page flow with depth-indented links, bold section headings and an accent pill
  for the current page (a fully expanded, scrollable `Menu` drawer on phones),
  and heading-based TOCs without Kube, jQuery, tocbot or webfont requests.
  `partials/hreflang.html` adds the same EN/ZH/x-default alternates as the
  homepage, including docs redirect aliases. Swagger CSS and its local bundle
  load only on API reference pages; existing heading IDs and legacy TOC anchor
  hooks remain unchanged. Documentation-only legacy assets (`theme.min.css`,
  `spdl-title.css`, `functions.min.js`, `jquery.backtothetop`, `kube.css` and
  `kube.min.js`) are removed; the Kube stack still used by `_default` pages and
  archived snapshots keeps its remaining files.

Speedle Engine, Cloud and AI share one header and footer standard (#203): the
`Speedle | Engine` lock-up, a `Products` menu and three footer product cards
(header standard v16) linking the three sites. Those cross-site links keep the
reader's language (#217): Chinese pages link Engine `/zh/`, Cloud `/zh-CN/` and
AI `/zh/`, and English pages link the English roots. `localize-links.py`
rewrites the Products menu and footer family on every page, including archived
snapshots, and `site.test.cjs` fails if a Chinese page links another site's
English root.
The footer lives in the static homepages; `partials/footer.html` reuses it for
Hugo pages and `localize-links.py` gives archived snapshots the same footer.
All footer rules live only in
`css/navigation.css`, which every page loads, under `html body footer.site-footer`
so the docs theme styles cannot override them. The sphere marks
`static/img/speedle/speedle-mark-{engine,cloud,ai}.svg` and `static/favicon.svg`
are the sphere group of `img/speedle/Speedle_logo_home_bold.svg` without path
edits (Engine original `#ff9a45`, Cloud `#4da3ff`, AI `#8b5cf6`). `favicon.svg`
uses the shared v13 favicon treatment on a transparent background in the
115-unit sphere box (`viewBox="21.8 22 115 115"`): no dark outline, and the
sphere's own strokes scaled up (`outer polygon ×2.0`, `inner lines ×2.2`) so the
16 px render stays legible. `favicon.ico` (16, 32, 48 px PNG entries) and
`apple-touch-icon.png` (180 px, sphere on `#2f2350` with 20 px padding) are
rendered from those SVGs; they replace `images/speedle_purple.png` and
`images/speedle_black.png`.

`static/img/social/engine-en.png` and `engine-zh.png` are the 1200 × 630 Open
Graph/Twitter share images: the `Speedle | Engine` lock-up and the home-page
headline on the Engine hero gradient. Every rendered content page declares the
image for its language, with `twitter:card=summary_large_image`; the docs pages
also carry `TechArticle`/`WebPage` and `BreadcrumbList` JSON-LD. `site.test.cjs`
checks the image files, the metadata and the title/lang pattern on every page.

Cloudflare Web Analytics uses manual JS installation. The public beacon snippet
is in `website/cloudflare-web-analytics.html`; `localize-links.py` adds it once to
every rendered page, including archived snapshots, replaces an older copy of the
snippet (for example after a token change), and skips redirect pages. The token
is the engine.speedle.io site token.
The four static homepage/Playground HTML sources carry the same snippet.
The site continues to publish through GitHub Pages with the existing DNS setup.
Its canonical host is `https://engine.speedle.io` (Hugo `baseURL`, canonical,
hreflang, Open Graph, sitemap, `robots.txt`, generated `llms.txt` and
`llms-full.txt`, and JSON-LD URLs);
`speedle.io` redirects there, so `localize-links.py` treats links to either host
as same-site. Products menu and footer family links stay absolute.

The merge-gating test job executes the English and Chinese Quick Start and
embedded deployment guides directly from `docs/hugo/content/` on every run.
`tests/check_quick_start.py` extracts the actual Bash blocks and exercises native
PMS/ADS; `tests/check_embedded.py` extracts the Bash, Go and SPDL blocks, builds
an external application against the documented public release and the current
candidate library, and checks Allow/Deny output for both. Both checkers use the
document blocks as executable inputs. The same global gate runs
`go test ./website/tests/docscheck` for the documented API contracts and
`tests/check_docs.py` for the remaining bilingual executable API/deployment guides.
The gate uses its current Go toolchain; nightly Quick Start checks additionally
cover Go 1.26.10 and 1.27.3.
The Quick Start check runs the documented `go install` route once against the
pinned release tag, checks the documented module path offline with `go list -m`,
requires the highest full semantic tag returned by public `git ls-remote --tags`
to be that same tag (so a new release, including a new major, fails the check
until both guides are updated), checks that the installed binaries accept the
flags the guide uses, runs one
documented walkthrough with those installed binaries, performs a public shallow
clone, checks out the captured CI revision, builds the three executables, creates
and lists policies, and checks direct, role and conditional (ABAC) grants and
denials. On Linux the documented service and client commands run in
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
python3 website/generate-llms.py --check docs
python3 website/tests/check_llms.py docs
```

`website/generate-llms.py` builds `llms.txt` (an llmstxt.org index of annotated
links) and `llms-full.txt` (the complete English documentation as Markdown) from
`docs/hugo/content`. Page titles, descriptions, routes and bodies come from the
content, so they cannot drift; the blockquote definition is the first paragraph
of `about.md`, and every published page must be listed in `FULL_PAGES` or
`EXCLUDED_FROM_FULL`. `build-site.sh` generates both files into the Hugo output
after the postprocessor, and `verify.sh` fails when the committed `docs/` copies
are stale. `tests/check_llms.py --external` additionally requires every curated
external link in `llms.txt` to answer 200 without a redirect
(`--external-full` extends that to the code references inside `llms-full.txt`);
the offline check runs in CI. The
production `robots.txt` explicitly allows the major AI crawlers as well as `*`.
There are no test-stage host rules in this repository.

To regenerate the committed copies:

```sh
python3 website/generate-llms.py --output docs
```

The existing protected Go CI build job runs the Playground release gate, both
language form suites, route tests, a complete published-link audit, and a temporary
build with the checksum-pinned official Hugo 0.48 binary. Existing homepage/CSS
and source/output invariants remain checked. The release query on the Playground
entry module refreshes previously cached browser code when that module changes.
