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
| authorization engine, policy engine, authorization policy engine | 授权引擎、策略引擎、授权策略引擎 | `/docs/access-control/`, `/docs/architecture/` |
| fine grained access control, fine-grained access control, FGA | 细粒度访问控制 | `/docs/access-control/`, RBAC/ABAC/PBAC guides |
| PAP, policy administration point | 策略管理点 | `/docs/pms/`, `/docs/api/management_api/`, XACML guide |
| PDP, policy decision point | 策略决策点 | `/docs/decisions/`, `/docs/api/decision_api/`, XACML guide |
| PEP, policy enforcement point; PIP, policy information point | 策略执行点、策略信息点 | `/docs/architecture/`, XACML guide |
| zero trust | 零信任 | IAM guide; application authorization role and deployment boundaries in `llms.txt` |
| externalized authorization, centralized authorization | 外置授权、授权外部化、集中式授权 | `/docs/architecture/`, `/docs/usecases/`, PBAC guide |
| authorization policy management | 授权策略管理 | `/docs/pms/`, `/docs/pms/policy-mgmt/`, `/docs/api/management_api/` |
| authorization decision service, API authorization | 授权决策服务、API 授权、API 权限控制 | `/docs/decisions/`, `/docs/api/decision_api/` |
| microservices authorization, cloud-native authorization | 微服务授权、微服务权限控制、云原生授权 | `/docs/usecases/`, `/docs/architecture/` |

The documentation index and architecture pages link to the guides; the existing
sidebar includes the new group. They are in both language sitemaps and linked
from `llms.txt`. The latter is a navigation aid, not a ranking guarantee.

The shared Hugo metadata partial adds descriptions, canonical URLs, reciprocal
English/Chinese `hreflang`, Open Graph/Twitter summaries and WebPage/TechArticle
JSON-LD referencing the homepage's existing website and software entities.
Existing noindex taxonomy rules and pagination canonicals remain in place.
Static homepage and Playground metadata are maintained in their existing sources.
Archives retain their original content and metadata.

The English/Chinese integration overviews explicitly set `noindex = false`.
Front matter overrides the default kind-based indexing policy, including false;
other section and taxonomy defaults remain in place. Release checks verify the
two indexable overviews and representative unchanged section/taxonomy exclusions.

The expanded terminology is expressed in the homepage software entity's JSON-LD
and the keyword arrays of relevant bilingual documentation pages. The visible
HTML bodies, navigation, styles and existing descriptions are preserved. The
bilingual `llms.txt` terminology section explains the terms and maps them to
Speedle's actual components. PAP/PDP/PEP/PIP describe XACML-style responsibilities;
NIST's policy administrator (PA) is a distinct concept. Zero trust is described
as a deployment design to which Speedle contributes application authorization,
with identity assurance, trusted device/session context and enforcement supplied
by the integrating system.

These annotations describe topics, not certifications or search ranking gains.
Google does not use the `meta keywords` tag for indexing or ranking; this release
does not add that tag. Reference: [supported and unsupported meta tags](https://developers.google.com/search/docs/crawling-indexing/special-tags).

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
