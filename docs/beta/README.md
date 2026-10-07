# Speedle SEO/GEO beta

This directory contains the isolated technical SEO/GEO candidate for review. It does not change the current site root.

## Review endpoints

- `/beta/` — homepage metadata plus `SoftwareSourceCode` and `WebSite` JSON-LD; visible page content and styling are unchanged.
- `/beta/robots.txt` — production robots policy candidate.
- `/beta/sitemap.xml` — production sitemap candidate with absolute canonical URLs; it includes only currently indexable pages and deliberately omits hand-maintained freshness dates.
- `/beta/llms.txt` — machine-readable project guide for answer engines.

## Promotion notes

1. Copy the reviewed files to the site root.
2. Replace the beta-only `noindex,nofollow` meta tag in `index.html` with `index,follow,max-image-preview:large,max-snippet:-1,max-video-preview:-1`.
3. Update Hugo's `baseURL` to `https://speedle.io/` and its site description so future builds retain absolute canonical URLs and the reviewed metadata.
4. Validate the deployed files, then submit `https://speedle.io/sitemap.xml` in Google Search Console.

## Indexing alert follow-up (2026-10-07)

`indexing-fixes.patch` records the reviewed changes against master commit `4222765`. The patch was approved on 2026-10-07 and applied to the production source in this release. It corrects relative links in the developer guide and the English/Chinese Docker integration guides, including their Hugo sources. It also adds immediate HTML redirect aliases for the two historical 404 URLs reported by Search Console:

- `/developer/docs/api/asserter_api` to `/docs/api/asserter_api/`
- `/integrations/quick-start` to `/quick-start/`

The developer guide's Policy Discovery link now targets the existing `/docs/pms/discover/` page. Visible copy and CSS are identical; only link destinations change. Redirect aliases are also supplied in Hugo's static source tree. On GitHub Pages these aliases use an immediate meta refresh, because this hosting setup does not support custom server-side redirect rules.

The patch is retained as a review record and should not be applied again. After deployment, start validation for the 404 category in Search Console. Existing host redirects and noindex rules are unchanged. Google may list the restored historical URLs as redirect pages, which is expected because their canonical destinations hold the content.
