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
