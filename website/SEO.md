# Search and AI discovery

The existing design and homepage copy remain unchanged. Public concept guides
use the current documentation layout and navigation. English and Chinese pages
contain the same tested policies and explain the same integration boundaries.

| English queries | Chinese queries | English landing page (Chinese adds `/zh`) |
| --- | --- | --- |
| authorization, access control | 授权、访问控制、权限控制 | `/docs/access-control/` |
| RBAC, role-based access control | RBAC、基于角色的访问控制、角色权限 | `/docs/access-control/rbac/` |
| ABAC, attribute-based access control | ABAC、基于属性的访问控制、属性权限 | `/docs/access-control/abac/` |
| PBAC, policy-based access control | PBAC、基于策略的访问控制、策略权限 | `/docs/access-control/pbac/` |
| identity, IAM, identity and access management | 身份、身份与访问管理、IAM | `/docs/access-control/identity-iam/` |
| OES, Oracle Entitlements Server | OES、Oracle 权限管理服务器 | `/docs/access-control/oes/` |
| XACML, eXtensible Access Control Markup Language | XACML、可扩展访问控制标记语言 | `/docs/access-control/xacml/` |

The documentation index and architecture pages link to the guides; the existing
sidebar includes the new group. They are in both language sitemaps and linked
from `llms.txt`. The latter is a navigation aid, not a ranking guarantee.

The shared Hugo metadata partial adds descriptions, canonical URLs, reciprocal
English/Chinese `hreflang`, Open Graph/Twitter summaries and WebPage/TechArticle
JSON-LD referencing the homepage's existing website and software entities.
Existing noindex taxonomy rules and pagination canonicals remain in place.
Static homepage and Playground metadata are maintained in their existing sources.
Archives retain their original content and metadata.

## Release checks

```sh
HUGO_BIN=/path/to/hugo-0.48 website/build-site.sh /tmp/speedle-site
python3 website/tests/check_seo.py docs
node website/tests/check-doc-examples.cjs
```

Both checks also run in the existing CI release gate. The rendered-site check
verifies visible English/Chinese terminology, matching translations, descriptions,
canonical URLs, parseable JSON-LD and sitemap coverage. The example check reads
the actual Markdown policies and JSON requests, then evaluates positive and
negative cases using the published Go/WASM engine. Browser acceptance additionally
checks desktop/mobile rendering and actual language navigation.

## Measure after deployment

In Google Search Console, submit `https://speedle.io/sitemap.xml`, inspect the
new English/Chinese URLs and check their selected canonicals. Track impressions,
clicks, click-through rate and average position for each query group and landing
page. Compare a baseline with subsequent 28-day periods, separating branded
queries (Speedle/Speedle+) from non-branded queries and languages. No search
performance data is stored by this repository or inferred from a successful build.

Record dated AI-search checks for representative questions about these models,
including whether an answer cites the official page and describes its scope
correctly. Distinguish a citation from a mere name mention. Published pages and
valid metadata do not prove indexing, rankings, or inclusion in an AI answer.

References: [Google SEO starter guide](https://developers.google.com/search/docs/fundamentals/seo-starter-guide),
[localized versions](https://developers.google.com/search/docs/specialty/international/localized-versions),
[generative AI guidance](https://developers.google.com/search/docs/fundamentals/ai-optimization-guide).
