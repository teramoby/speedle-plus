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
- `docs/hugo/layouts/partials/header.html`: language-aware legacy documentation
  navigation. `master.js` also handles its mobile menu directly.

## Generate and publish

Use the existing Hugo 0.48 toolchain, and always render into a separate directory:

```sh
HUGO_BIN=/path/to/hugo-0.48 website/build-site.sh /tmp/speedle-site
```

The build renders Hugo, runs `localize-links.py`, then validates rendered SEO
contracts with `tests/check_seo.py`. The postprocessor handles
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

See [SEO.md](SEO.md) for the bilingual topic map, metadata contracts and
post-deployment measurement. Public guides are source content, not hidden
search-only pages. The shared sidebar uses local template variables so concurrent
Hugo renders do not overwrite another page's menu entries.
