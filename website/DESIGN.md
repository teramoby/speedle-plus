# Speedle website design standard

Canonical design standard for the three Speedle websites. Every change to any of
them follows this file. The sites share one design; **colours are the only
per-site difference**.

| Site | Product name | URL |
| --- | --- | --- |
| Speedle Engine | Speedle Engine | <https://engine.speedle.io/> (`speedle.io` redirects there) |
| Speedle Cloud | Speedle Cloud | <https://cloud.speedle.io/> |
| Speedle AI | Speedle AI | <https://ai.speedle.io/> |

The values below are measured from the live CSS, not invented:

- **Engine**: `docs/css/site.css` and `docs/css/navigation.css`
  (`header-20261027`) in `teramoby/speedle-plus`.
- **AI**: `https://ai.speedle.io/styles.css`, the reference implementation of
  the shared header and footer.
- **Cloud**: `apps/console/src/styles/tokens.css` at `main` in
  `teramoby/cloud.speedle.io`, merged in `cloud.speedle.io#244`.
- **Header and footer additions**: the owner-approved header standard v2–v16.
  This file is the merged form; where an older header standard differs, this
  file wins.

Where the three sites disagree today, the standard follows speedle.io (Engine)
except for colours.

## 1. Sites and names

- The product name is **Speedle**, never "Speedle+". This is legal advice, not a
  style preference.
- When the self-hosted engine must be told apart from Cloud, write
  **self-hosted Speedle**.
- The header lock-up is `Speedle | Engine`, `Speedle | Cloud` or
  `Speedle | AI`: the sphere mark plus the "Speedle" wordmark, a divider, then
  the product word.
- Use the canonical URLs everywhere: <https://engine.speedle.io/>,
  <https://cloud.speedle.io/>, <https://ai.speedle.io/>. Chinese pages use the
  site's Chinese route and keep the same host.
- Mark the current site in the Products menu and the footer with
  `aria-current="page"`.
- **Page titles** use the same pattern on all three sites, in English and
  Chinese. Here `<Product>` is only the product word `Engine`, `Cloud` or `AI`,
  not the full product name:
  - Home page: `Speedle <Product> — <one-line description>`, giving
    `Speedle Engine — <one-line description>`,
    `Speedle Cloud — <one-line description>` or
    `Speedle AI — <one-line description>`.
  - Every other page: `<Page name> · Speedle <Product>`.
  - Chinese pages localize the one-line description and page name; the product
    names remain **Speedle Engine**, **Speedle Cloud** and **Speedle AI**.
  - `og:title` is identical to `<title>`. Neither ends with a full stop.

## 2. Colour

### 2.1 Token roles

The same token roles exist on all three sites. Only the values differ. Never
add a colour outside the site's tokens.

| Role | Engine (speedle.io) | AI (ai.speedle.io) | Cloud (cloud.speedle.io) |
| --- | --- | --- | --- |
| Page background | `--white` `#ffffff` | `--white` `#ffffff` | `--surface` `#fff` |
| Hero background | `linear-gradient(135deg, #25134e 0%, #6636c8 62%, #814ce4 100%)` | `linear-gradient(135deg, #ffffff 0%, #f4f0fb 55%, #e9e1f8 118%)` | `--hero-background` `linear-gradient(135deg, #0b2149 0%, #14469c 62%, #1f66d1 100%)`; `--hero-ring` `rgba(1, 231, 203, 0.12)`; `--hero-disc` `rgba(1, 231, 203, 0.09)` |
| Header background | `rgba(31, 17, 66, .92)` + `backdrop-filter: blur(18px)`; composite `#312451` over white | `rgba(255, 255, 255, .96)` + blur 18 | `--surface-inverse` `#0b2149` |
| Footer background | `#171022` | `#f4f0fb` | `--surface-inverse` `#0b2149` |
| Surface | `#ffffff` | `--white` `#ffffff` | `--surface` `#fff` |
| Surface soft / tint | `--soft` `#f6f7fb` | `--soft` `#f7f6fa`, `--tint` `#f0ebfa`, `--tint-light` `#f4f0fb`, `--tint-strong` `#e9e1f8` | `--surface-soft` `#f6f7fb`, `--surface-tint` `#e6eefa` |
| Ink | `--ink` `#172033` | `--ink` `#182034` | `--text` `#172033` |
| Muted text | `--muted` `#5d667a` | `--muted` `#596276` | `--muted` `#5d667a` |
| Primary | `--purple` `#814ce4` | `--primary` `#7546d8` | `--primary` `#1f66d1` |
| Primary strong / hover | `--purple-dark` `#5c2fc0` | `--primary-dark` `#5229ad` | `--primary-hover` `#1550a8` |
| Accent | `--cyan` `#01e7cb` | `--accent` `#8b5cf6`, `--accent-light` `#b9a2f0` | `--accent-decorative` `#01e7cb`; inverse text accent `--text-accent-inverse` `#7cc4ff` |
| Border | `--line` `#e5e8f1`; on dark `rgba(255, 255, 255, .1)` | `--line` `#e4e5ee`, `--line-purple` `#d8cbf0`; on ink `rgba(24, 32, 52, .1)` | `--border` `#e5e8f1`, `--border-control` `#777f91`; header `--header-border` `rgba(255, 255, 255, 0.1)`; footer `--footer-border` `rgba(255, 255, 255, 0.18)` |
| Focus ring (light surface) | header `#01e7cb`; page controls `#5c2fc0` | `--focus` `#5229ad` | `--focus` `#1550a8` |
| Focus ring (dark surface) | `--cyan` `#01e7cb` | `--focus` `#5229ad` (the AI header is light) | `--focus-inverse` `#9cc3ff` |
| Inverse text | `#ffffff` | `#ffffff` | `--text-inverse` `#fff` |
| Logo sphere | `#ff9a45` (original) | `#8b5cf6` | `#4da3ff` |

Engine's page CSS leaves the browser's default focus ring for page controls;
this standard sets it to `--purple-dark` `#5c2fc0` (8.01:1 on white), while the
header keeps its measured `#01e7cb` ring.

Status colours are part of the token set where the site has status UI: AI uses
`--success` `#176342` / `--success-soft` `#edf8f1` and `--danger` `#a3243c` /
`--danger-soft` `#fff0f2`; Cloud uses `--success` `#176342` /
`--success-surface` `#edf8f1` and `--danger` `#a3243c` / `--danger-surface`
`#fff0f2`. Use them; do not invent new status colours.

### 2.2 WCAG AA contrast pairs

Normal text is at least **4.5:1**; large text (≥ 24 px, or ≥ 18.66 px bold) and
UI boundaries and focus indicators are at least **3:1**. These pairs are
measured:

| Site | Foreground | Background | Ratio | Use |
| --- | --- | --- | --- | --- |
| Engine | `#172033` | `#ffffff` | 16.27:1 | body text |
| Engine | `#5d667a` | `#ffffff` | 5.76:1 | muted text |
| Engine | `#5d667a` | `#f6f7fb` | 5.38:1 | muted text on soft |
| Engine | `#5c2fc0` | `#ffffff` | 8.01:1 | text links and light-ground eyebrows |
| Engine | `#5c2fc0` | `#f6f7fb` | 7.48:1 | eyebrows on soft |
| Engine | `#01e7cb` | `#25134e` | 10.37:1 | accent text on dark |
| Engine | `#8f85a6` | `#100b1e` | 5.58:1 | SPDL comments |
| Engine | `#d7d4dd` (white at .82) | `#312451` | 10.23:1 | header nav links |
| Engine | `#ffffff` | `#6636c8` | 7.18:1 | hero text at the 62% stop |
| Engine | `#ffffff` | `#814ce4` | 5.14:1 | hero text at the light end |
| Engine | `#1b1c2c` | `#01e7cb` | 10.64:1 | primary button |
| Engine | `#b9b7bd` (white at .7) | `#171022` | 8.99:1 | footer text |
| AI | `#182034` | `#ffffff` | 16.22:1 | body text |
| AI | `#596276` | `#ffffff` | 6.12:1 | muted text |
| AI | `#596276` | `#f4f0fb` | 5.45:1 | muted text on tint |
| AI | `#7546d8` | `#ffffff` | 5.79:1 | links and primary labels |
| AI | `#5229ad` | `#ffffff` | 9.21:1 | strong links |
| AI | `#ffffff` | `#7546d8` | 5.79:1 | text on primary |
| AI | `#ffffff` | `#5229ad` | 9.21:1 | text on primary-dark |
| AI | `#3f4656` (ink at .82) | `#ffffff` | 9.45:1 | header nav links |
| AI | `#555b6b` (ink at .72) | `#f4f0fb` | 6.04:1 | footer text |
| AI | `#5229ad` | `#f4f0fb` | 8.21:1 | badge text |
| Cloud | `#ffffff` | `#0b2149` | 15.81:1 | text on darkest blue |
| Cloud | `#4da3ff` | `#0b2149` | 6.02:1 | sphere/accent on dark |

Colour rules:

- Engine cyan is for dark grounds. `#01e7cb` on `#814ce4` is 3.25:1 (large text
  only) and on white it fails. Never use cyan as body text or as an eyebrow on
  a light surface; light-ground eyebrows use `--purple-dark` `#5c2fc0`.
- Engine hero body copy at `rgba(255, 255, 255, .78)` is 4.62:1 over `#6636c8`
  but 3.78:1 over `#814ce4`. Keep body copy in the dark part of the gradient
  (0–62%) or use solid white.
- AI `#8b5cf6` on white is 4.23:1: decorative graphics, icons and large text
  only. Normal text uses `#7546d8` or `#5229ad`.
- Cloud `#4da3ff` on white is 2.63:1: never use it as text or as a control
  boundary on white. It needs a dark ground.

## 3. Header

The header is identical on all three sites, every page, English and Chinese.
Copy speedle.io's `navigation.css` geometry verbatim; only colour values may
differ. Do not invent other sizes.

### 3.1 Geometry

| Element | Desktop (above 1160 px) | Phone (≤ 700 px) |
| --- | --- | --- |
| Header bar | height 76 px (`min-height: 76px`); bottom border 1 px | same 76 px |
| Inner container | `width: min(1160px, calc(100% - 40px))`, centred; flex, `justify-content: space-between`, `align-items: center` | `width: min(100% - 28px, 1160px)` |
| Lock-up `<a class="brand">` | `display: inline-flex; align-items: center; gap: 14px` | same |
| Logo image (sphere + wordmark SVG, viewBox 304×104) | width **152 px**, height auto (≈ 52 px) | width **128 px** |
| Divider | 1 px × **26 px**, `background: currentColor; opacity: .4` | 1 px × **22 px** |
| Product word (`Engine` / `Cloud` / `AI`) | Inter 600, **22 px**, line-height 1, letter-spacing 0 | **19 px** |
| Nav links | Inter **15.04 px**, weight 600, line-height 1.65, gap **30 px**, `white-space: nowrap` | same font; collapsed list gap 18 px |
| GitHub pill | padding **10 px × 18 px**, 1 px border, radius 999 px, same font | same |
| Menu collapse | **≤ 1160 px** | — |
| Collapsed menu | — | button; dropdown `padding: 24px`, `gap: 18px`, absolutely positioned below the 76 px bar, `max-height: calc(100dvh - 76px)`, scrollable |

The header height stays 76 px at every width once the navigation controller is
ready. The Products menu does not change the header height.

### 3.2 Lock-up and navigation

- The lock-up is the original sphere mark plus the unmodified "Speedle"
  wordmark, then the divider, then the product word. The wordmark asset is the
  same on all three sites.
- The logo links to that site's home page.
- The first nav item is the **Products** menu (section 3.4). The site's own
  links follow. It is the header's only route to the other Speedle sites
  (v10): no stand-alone product link (`Speedle Engine`, `Speedle Cloud`,
  `Speedle AI` or `Speedle+`) may appear in the desktop or collapsed menu, in
  English or Chinese.
- The **language switch** is one text link, last before the GitHub pill on
  Engine and last overall on Cloud and AI (section 9).
- Only Engine ends with the **GitHub pill**. Cloud and AI have **no GitHub
  pill** in the header or the collapsed menu (v7); for them the language switch
  is the last nav item. The footer keeps the GitHub icon on all three sites.
- Focus is always visible: `outline: 2px solid <site header accent>;
  outline-offset: 4px`.

### 3.3 Header colours

| Part | Engine | Cloud | AI |
| --- | --- | --- | --- |
| Background | `rgba(31, 17, 66, .92)` | `--surface-inverse` `#0b2149` | `rgba(255, 255, 255, .96)` |
| Links | `rgba(255, 255, 255, .82)` | `--text-inverse` `#fff` | `rgba(24, 32, 52, .82)` |
| Hover / current | `#01e7cb` | `--text-accent-inverse` `#7cc4ff` | `#7546d8` |
| GitHub pill border | `rgba(255, 255, 255, .28)` | none (no pill) | none (no pill) |
| Product word | `#01e7cb` | `--text-accent-inverse` `#7cc4ff` | `#7546d8` |
| Sphere | `#ff9a45` (original) | `#4da3ff` | `#8b5cf6` |
| Bottom border | `rgba(255, 255, 255, .1)` | `--header-border` `rgba(255, 255, 255, 0.1)` | `rgba(24, 32, 52, .1)` |
| Focus ring | `#01e7cb` | `--focus-inverse` `#9cc3ff` | `#7546d8` |

### 3.4 Products menu (primary cross-link)

- First nav item: a `<button>` labelled **Products** (zh: **产品**) with a
  10 px chevron (inline SVG), same font, size, weight and colour as the nav
  links. `aria-haspopup="true"`, `aria-expanded`, `aria-controls`.
- Panel: opens 12 px below the button, left-aligned to it; width 340 px;
  radius 14 px; padding 10 px; 1 px border and a soft shadow; background = the
  site's surface colour.
- Three rows, each a link: a 20 px sphere in that product's colour, the name
  (Inter 600, 15 px), and a one-line description (Inter 400, 13 px, muted).
  Row padding 10 px × 12 px, radius 10 px, hover = the site's tint.
  - **Speedle Engine** — "Open-source authorization engine" / "开源授权引擎" —
    <https://engine.speedle.io/>
  - **Speedle Cloud** — "Hosted authorization for developers" / "面向开发者的托管授权服务" —
    <https://cloud.speedle.io/> — badge **"Coming soon" / "即将上线"**
    (small pill, 11 px, weight 600)
  - **Speedle AI** — "AI for access control, access control for AI" /
    "用 AI 做权限，为 AI 管权限" — <https://ai.speedle.io/>
- Sphere colours in the menu: Engine `#ff9a45` (original orange), Cloud
  `#4da3ff`, AI `#8b5cf6`.
- The current site's row shows a check mark and `aria-current="page"`.
- Behaviour: click, Enter or Space toggles; Esc closes and returns focus to the
  button; click outside closes; arrow keys move between rows; the focus ring is
  visible; hover never opens the panel.
- At ≤ 1160 px the nav is collapsed: a **Products** group at the top of the open
  menu lists the three rows inline (no nested dropdown), with the heading
  visible.
- Chinese pages use the Chinese labels and link to the Chinese home page where
  one exists.

### 3.5 Parity check

With Playwright at **1440 px and 390 px**, measure the same values on the site
and on <https://speedle.io> (for the divider and product word, compare against
this section, since speedle.io may not have them yet):

header height; logo image width and height; divider width and height; product
word font-size and weight; first nav link font-size, weight and letter-spacing;
nav gap; GitHub pill padding and height; footer height; footer lock-up logo
width; footer text size; social box size.

Every value must match within **± 1 px**. Put the table in the PR body.

## 4. Typography

- **Family**: `Inter, ui-sans-serif, -apple-system, BlinkMacSystemFont,
  "Segoe UI", sans-serif`.
- **Chinese**: `Inter, ui-sans-serif, -apple-system, BlinkMacSystemFont,
  "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif`.
- **Weights**: body 400; navigation and strong UI 600; bold 700; buttons and key
  links 750; eyebrows 800. The header lock-up uses 600. The stack is identical
  on all three sites, so fallback behaviour is identical; if a site loads Inter
  as a webfont, load 400/600/700 and let the variable face supply 750/800.
- **Base**: 16 px / 1.65, weight 400.

| Role | Size | Line height | Letter spacing | Weight |
| --- | --- | --- | --- | --- |
| h1 (hero) | `clamp(3.1rem, 6vw, 5.6rem)` = 49.6–89.6 px | .98 | -.055em | 700 |
| h2 (section) | `clamp(2.15rem, 4vw, 3.3rem)` = 34.4–52.8 px | 1.08 | -.04em | 700 |
| h3 (card) | 1.3rem = 20.8 px | 1.65 (inherited) | 0 | 700 |
| h3 (compact card) | 1.16rem = 18.56 px | 1.35 | 0 | 700 |
| Eyebrow | .78rem = 12.48 px, uppercase | 1.65 (inherited) | .14em | 800 |
| Body | 1rem = 16 px | 1.65 | 0 | 400 |
| Lead (hero copy) | 1.18rem = 18.88 px | 1.65 | 0 | 400 |
| Small (captions) | .88rem = 14.08 px | 1.65 | 0 | 400 |
| Footer tagline / card name | 15 px | 24 px | 0 | 400 / 600 |
| Footer card description | 14 px | 20 px | 0 | 400 |
| Footer legal bar | 13 px | 20 px | 0 | 400 |
| Code | .9rem = 14.4 px, `ui-monospace, SFMono-Regular, Menlo, monospace` | 1.8 | 0 | 500 |
| Nav link | 15.04 px | 1.65 | 0 | 600 |
| Button | 1rem | 1.65 | 0 | 750 |

At ≤ 700 px the hero h1 is fixed at 3.25rem (52 px). Do not add a different
weight or letter-spacing per site.

## 5. Layout and spacing

### 5.1 Layout

| Element | Value |
| --- | --- |
| Container | max 1160 px; side gutters 20 px (`calc(100% - 40px)`); ≤ 700 px gutters 14 px (`calc(100% - 28px)`) |
| Section vertical padding | 104 px; ≤ 700 px 76 px |
| Hero | min-height 600 px; padding 96 px 0 100 px; grid `1.15fr .85fr`; gap 72 px; ≤ 900 px one column, padding 76 px 0, gap 30 px; hero art min-height 390 px (≤ 900 px 260 px) |
| Status bar | min-height 78 px; ≤ 700 px padding 18 px 0, column, gap 9 px |
| Section head | flex, `align-items: end`, `justify-content: space-between`, gap 40 px, margin-bottom 44 px; ≤ 900 px column, gap 16 px |
| Footer | see section 7 |
| Card grids | about 2 columns gap 26 px; features 12 columns gap 18 px (cards span 4, then 3); integrations 3 columns gap 20 px; policy `.8fr 1.2fr` gap 68 px; contribute `minmax(0, 1fr) auto` gap 48 px, padding 62 px |

Radii: 22 px default (`--radius`); 18 px feature and integration cards; 16 px
code blocks; 14 px Products panel; 10 px social boxes and menu rows; 9 px
controls (`--radius-control`); 999 px pills.

### 5.2 Spacing values

These are the values measured in the Engine CSS. Reuse them; do not introduce a
new one-off value.

| Range | Values | Used for |
| --- | --- | --- |
| 4–10 px | 4, 7, 8, 9, 10 | icon gaps, code dots, eyebrow rule gap |
| 12–16 px | 12, 13, 14, 16 | inline gaps, lock-up, list gaps |
| 18–24 px | 18, 20, 22, 24 | card and integration grids, mobile menu |
| 26–34 px | 26, 28, 30, 34 | two-column grids, card padding, action margins |
| 40–50 px | 40, 42, 44, 48, 50 | section head, card padding |
| 60–76 px | 60, 62, 68, 72, 76 | large card padding, hero gap, header height |
| 96–104 px | 96, 100, 104 | hero and section vertical rhythm |

### 5.3 Breakpoints

| Width | Behaviour |
| --- | --- |
| ≤ 1160 px | header nav collapses to the menu; Products becomes an inline group; the 30 px nav gap applies only while the desktop nav shows (v8) |
| ≤ 1024 px | footer brand block and cards each span all 12 columns; the cards stay in one row below the brand |
| ≤ 900 px | hero, policy, about and contribute grids become one column; section head stacks; feature cards span 6 |
| ≤ 720 px (AI/Playground) / ≤ 700 px (Engine home) | mobile container gutters; brand 128 / 22 / 19; h1 3.25rem; section padding 76 px; menu button and dropdown; status stacks; feature cards full width; footer cards stack in one column and the main tier padding is 48 px 0 32 px. Keep the site's existing value; do not mix 720 and 700 on one page |
| ≤ 620 / 560 px | component-level stacking measured in the Playground CSS |
| QA widths | 1440 px desktop, 390 px and 320 px phones |

## 6. Components

Values follow the Engine CSS. Colours come from the site's token row.

### 6.1 Buttons

| Property | Value |
| --- | --- |
| Box | `inline-flex`, centred, min-height 50 px, padding 0 23 px, radius 9 px |
| Type | 1rem, weight 750 |
| Transition | `transform .2s, background .2s, border-color .2s`; hover `translateY(-2px)` |
| Primary | Engine: `#1b1c2c` on `#01e7cb`. AI: white on `#7546d8`, hover `#5229ad`. Cloud: `--text-inverse` `#fff` on `--primary` `#1f66d1`, hover `--primary-hover` `#1550a8` |
| Secondary | Engine: white text, 1 px `rgba(255, 255, 255, .3)` border. AI: `#5229ad` text, 1 px `#d8cbf0` border, white background, hover `#5229ad` border and tint. Cloud: `--primary-hover` `#1550a8` text, 1 px `--border-control` `#777f91` border, `--surface` `#fff` background, hover `--primary-hover` border and `--surface-tint` `#e6eefa` |
| Focus | 3 px focus ring, offset 4 px |

### 6.2 Cards

| Property | Value |
| --- | --- |
| Default | radius 22 px; padding 42 px; surface background; 1 px `--line` border |
| Feature and integration cards | radius 18 px; padding 30 px; min-height 300 px (feature), 220 px (integration) |
| Hover | `translateY(-5px)`; border = the site's primary at .35 (Engine `rgba(129, 76, 228, .35)`); shadow = `var(--shadow)` (Engine `0 20px 60px rgba(40, 27, 83, .12)`); transition .25 s |
| Dark card | `--ink` background, white text, body `rgba(255, 255, 255, .72)`, strong text in the accent colour |
| Contribute banner | `--soft` background, 1 px `--line` border, radius 22 px, padding 62 px; copy on the left (eyebrow, h2, muted paragraph) and the primary action on the right; no illustration; one column at ≤ 900 px |

### 6.3 Eyebrows and badges

| Element | Value |
| --- | --- |
| Eyebrow | `inline-flex`, centred, gap 9 px; uppercase; .78rem; weight 800; letter-spacing .14em; colour = the site's AA-safe accent: Engine `#5c2fc0` on light surfaces and `#01e7cb` on dark grounds, AI `#5229ad`; 28 px × 2 px rule before it in `currentColor` |
| "Coming soon" badge | 11 px, weight 600, padding 2 px × 7 px, radius 999 px; colours = primary-strong on the site's tint (AI measured: `#5229ad` text on `#f4f0fb`, `#d8cbf0` border); Cloud: `--primary-hover` `#1550a8` on `--surface-tint` `#e6eefa`, `--border` `#e5e8f1` border |
| Status dot | 10 px, accent colour, 6 px ring at .14 opacity |

### 6.4 Code blocks and SPDL

| Element | Value |
| --- | --- |
| Container | `#100b1e` background; 1 px `rgba(255, 255, 255, .13)` border; radius 16 px; shadow `0 30px 80px rgba(0, 0, 0, .25)`; overflow hidden |
| Bar | padding 13 px × 18 px; bottom border 1 px `rgba(255, 255, 255, .1)`; colour `rgba(255, 255, 255, .55)`; font 600 .78rem/1 `ui-monospace`; dots 9 px, gap 7 px, `#684e8e` |
| Body | padding 26 px; colour `#ece8f8`; font 500 .9rem/1.8 `ui-monospace, SFMono-Regular, Menlo, monospace`; `overflow: auto` |
| SPDL classes | `.kw` `#01e7cb`; `.str` `#d3afff`; `.cm` `#8f85a6` (5.58:1 on `#100b1e`) |
| SPDL comments | `#` only; no `//` or `/* */` |

### 6.5 Status bar

| Element | Value |
| --- | --- |
| Bar | white background, 1 px bottom border in `--line` |
| Inner | min-height 78 px, flex, space-between, gap 24 px |
| Copy | flex, gap 13 px, muted |
| Dot | 10 px, accent, 6 px ring at .14 opacity |
| Link | `--purple-dark` (or the site's primary strong colour), weight 750 |

### 6.6 Icons

| Icon | Size |
| --- | --- |
| Footer social icon | 20 px inside a 36 px box |
| Product sphere in menus | 20 px |
| Product sphere in footer cards | 28 px |
| Check in a menu row or footer card | 15 px |
| Check list | 22 px |
| Feature icon | 78 px |
| Hero art | `min(330px, 78%)` |

Use inline SVG or the shared sprite; icons inherit `currentColor`; no emoji.

### 6.7 Focus and motion

- Every interactive element has a visible focus ring. Header and navigation:
  2 px solid accent, offset 4 px. Page controls: 3 px solid the site's focus
  colour (`--focus` on AI and Cloud, `--purple-dark` on Engine), offset 4 px.
  Never remove an outline without an equally visible replacement.
- Motion is short and one-shot: .2 s on buttons, .25 s on cards; hover lifts
  2 px (buttons) or 5 px (cards). No looping, parallax or scroll-jacking.
- When a site has a `prefers-reduced-motion` block, keep it; add one when
  touching motion in a site that lacks it.

## 7. Footer

The footer is identical on all three sites, English and Chinese; only colours
differ. It is the v16 standard: a 12-column main tier with the brand block on
the left and three product cards on the right, then a hairline legal bar.

### 7.1 Layout

- `<footer>`: `margin-top: 104px`; the main tier and the legal bar share the
  header container width (1160 px; side gutters 20 px, ≤ 700 px 14 px).
- **Main tier** `.footer-main`: `display: grid; grid-template-columns:
  repeat(12, minmax(0, 1fr)); gap: 32px 24px; align-items: start;
  padding: 56px 0 40px`.
  - Columns 1–4 `.footer-brand`: the footer lock-up, then a one-line tagline in
    Inter 400 **15 px / 24 px** muted 12 px below it, then the social row 24 px
    below the tagline.
  - Columns 6–12 `.footer-family`: three equal cards in a
    `repeat(3, minmax(0, 1fr))` grid with a 16 px gap, top-aligned with the
    lock-up.
- **Footer lock-up** = the header lock-up at footer scale: logo image width
  **132 px**, divider 1 × 22 px, product word Inter 600 **19 px**; it is a link
  to that language's home page.
- **Cards** `.footer-card`: 12 px radius, 16 px padding, 1 px border in the
  site's border token, a 28 px sphere with a 12 px bottom margin, the name
  Inter 600 **15 px / 24 px**, the description Inter 400 **14 px / 20 px**
  muted. Cards in a row are the same height; the tagline and the card names
  share the y = 113 px baseline at 1440 px.
  - The current site's card: accent tint background, accent border, the accent
    check mark (15 px) in its top-right corner and `aria-current="page"`; its
    name uses the accent colour.
  - Cloud carries the "Coming soon" / "即将上线" badge (11 px / 600), placed in
    the card's top-right corner so the name and description stay on the shared
    baseline.
  - The other cards take the accent border on hover.
- **No heading** above the products.
- **Bottom bar** `.footer-bar`: 1 px hairline, `padding: 20px 0`, Inter 400
  **13 px / 20 px** muted. Left: the license, then the copyright on the same
  line separated by `" · "`; the wording is byte-identical to the approved
  text (section 7.3). Right: the existing legal links where the site has them
  (Cloud: privacy · terms).
- ≤ 1024 px: the brand block and the cards each span all 12 columns; the cards
  stay in one row below the brand (32 px row gap).
- ≤ 720 px: the cards stack in one full-width column, the bottom bar stacks,
  and the license and copyright each get their own line (the " · " separator
  is hidden). The main tier padding is 48 px 0 32 px.

### 7.2 Social row

- Exactly speedle.io's four, in this order:
  1. Email — `mailto:contact@speedle.io`
  2. Google Groups — <https://groups.google.com/forum/#!forum/speedle-users>
  3. GitHub — <https://github.com/teramoby/speedle-plus>
  4. Discord — <https://discord.gg/uaY2xVsbbT>
- Each is a 36 px × 36 px box, 1 px border, radius 10 px, 20 px icon, gap 8 px;
  the same SVG icons, recoloured only; `aria-label`s as on speedle.io.

### 7.3 Colours

| Site | Footer | Text | Border | Accent / current card |
| --- | --- | --- | --- | --- |
| Engine | `#171022` | `rgba(255, 255, 255, .7)` | `rgba(255, 255, 255, .18)` | `--cyan` `#01e7cb`; tint `rgba(1, 231, 203, .12)` |
| AI | `#f4f0fb` | `#182034` at .72 | `rgba(24, 32, 52, .14)` | `--primary` `#7546d8`; tint `--tint-strong` `#e9e1f8`, name `--primary-dark` `#5229ad` |
| Cloud | `--surface-inverse` `#0b2149` | `--footer-text` `rgba(255, 255, 255, 0.7)` | `--footer-border` `rgba(255, 255, 255, 0.18)` | `--text-accent-inverse` `#7cc4ff`; tint `rgba(124, 196, 255, .12)` |

- AA contrast in every case: Engine footer text white at .7 on `#171022` is
  8.99:1 and the cyan accent is 11.8:1; AI footer text is 6.04:1 on `#f4f0fb`.
- The legal text is byte-identical to the approved wording:
  1. Engine and AI: "Licensed under the Universal Permissive License v1.0." /
     "基于 Universal Permissive License v1.0 授权。"
  2. All: "Copyright © 2019–2026. All rights reserved." /
     "版权所有 © 2019–2026。"

Add the footer to the parity table: footer height, lock-up logo width, text
size, social box size.

## 8. Logo and favicons

### 8.1 Logo

- The sphere mark's shape never changes. Colours may differ per site: Engine
  original `#ff9a45`, Cloud `#4da3ff`, AI `#8b5cf6`.
- Use the same mark plus "Speedle" wordmark asset everywhere; do not redraw,
  stretch or re-space the wordmark.

### 8.2 Favicons

- Every site's favicon is the **sphere mark only** (no wordmark), transparent
  background, same shape as the logo, ~1 px padding at 16 px.
- Colours: Engine `#ff9a45` (original), Cloud `#4da3ff`, AI `#8b5cf6`.
- **No dark outline** (v13; v9 is withdrawn). Remove the underlay polygon or
  outline from `favicon.svg` and every frame inside the `.ico`. The logos in the
  header and footer are unchanged.
- In the favicon copies only, strengthen the sphere's own strokes in the site's
  sphere colour: for example, outer polygon **×1.6** and inner lines **×1.8**,
  tuned so the 16 px render has clear lines without filling in. Keep the same
  shape and transparent background.
- Files at the site root, generated from the same recoloured SVG (colour and
  favicon-only stroke-width scaling, no path edits):
  - `favicon.svg`
  - `favicon.ico` (16, 32 and 48 px inside one `.ico`)
  - `apple-touch-icon.png` (180 × 180, the sphere on the site's header
    background colour with 20 px padding)
- Link them in every page `<head>`, including Hugo-generated pages:
  `<link rel="icon" href="/favicon.svg" type="image/svg+xml">`,
  `<link rel="icon" href="/favicon.ico" sizes="any">`,
  `<link rel="apple-touch-icon" href="/apple-touch-icon.png">`.
- Remove old icon links and old icon files that are no longer referenced; keep a
  provenance note in the site README.
- Check the 16, 32 and 48 px renders on light and dark tab bars. **Rollout is
  pending**: the v13 favicon fix merges last, after the other site PRs.

## 9. Language and bilingual pages

- Every page has a counterpart in the other language. URLs carry the language:
  `/` and `/zh/` on Engine and AI; Cloud keeps its existing `/zh-CN/…` routes.
  No cookie, browser storage or language-detection redirect.
- The header has **one** text link to the other language, the last nav item
  before the GitHub pill (or the last item on Cloud and AI, which have no
  pill). Same
  font, size, weight and colour as the other nav links; no flag, icon,
  dropdown or two-link toggle.
- The link shows the *other* language, written in that language:
  - English pages: `中文`, `lang="zh-CN" hreflang="zh-CN"`,
    `aria-label="切换到中文"`.
  - Chinese pages: `English`, `lang="en" hreflang="en"`,
    `aria-label="Switch to English"`.
- It links to the **same page** in the other language (for example
  `/docs/` ↔ `/zh/docs/`), falling back to that language's home page only when
  no counterpart exists.
- Every page carries `<link rel="alternate">` entries with `hreflang="en"`,
  `hreflang="zh-CN"` and `hreflang="x-default"` for its counterparts. No
  automatic redirects by browser language.
- In the collapsed menu (≤ 1160 px) the link keeps the same position in the
  list.
- Chinese pages use the Chinese labels throughout the header and footer
  (Products → 产品) and the Chinese descriptions in the Products panel and
  footer cards.
- Cloud replaces its two-link "中文 / English" toggle with this single link.
  AI gets the link when its Chinese pages exist.

## 10. Analytics

Every public Speedle site loads the Cloudflare Web Analytics beacon on every
public page. It is cookieless.

```html
<script defer src="https://static.cloudflareinsights.com/beacon.min.js" data-cf-beacon='{"token": "<site token>"}'></script>
```

- **One site token per hostname.** The coordinator provides the token; site
  tokens are public. Never reuse one host's token on another host.
- Public pages only: landing pages, docs, legal pages, blog and archive
  snapshots, including Hugo-generated pages. **Never** on signed-in app pages
  or API responses.
- Engine adds the snippet once to every rendered page through
  `website/localize-links.py`, using `website/cloudflare-web-analytics.html`,
  and skips redirect pages. AI and Cloud add the same snippet in the shared
  page head or layout.
- Cloud's CSP on public pages allows `https://static.cloudflareinsights.com`
  in `script-src` and `https://cloudflareinsights.com` in `connect-src`. The
  signed-in workspace keeps `script-src 'self'`.
- A site whose privacy policy lists data processors must disclose Web
  Analytics. That legal text needs the owner's verbatim approval before the
  beacon goes live there.

## 11. Content rules

- **Titles never end with a full stop.** In English and Chinese, no title-like
  element ends with `.` or `。`: headings h1–h6, the hero title, card and
  section titles, eyebrow and kicker labels, button and nav labels, `<title>`
  and `og:title`. Punctuation inside a title stays ("Policy first. Risk signals
  second. Veto intact"). Body text, captions, list items that are full
  sentences and meta descriptions keep their punctuation.
- **Only claim what has shipped.** Use "Coming soon" or "available at launch"
  for anything that is not live. Do not state partner, availability or
  performance claims without evidence.
- **SPDL examples must be real.** Test them with the Speedle parser and
  evaluator; use `#` comments only; never present pseudo-code as runnable.
- **Names and cross-links** follow sections 1 and 3.4. The only permitted
  contextual cross-links are:
  - Engine home: **none** (v12). Remove "Prefer a hosted service? Speedle Cloud
    (coming soon) →" and "Authorization for AI agents → Speedle AI", including
    their Chinese equivalents. Engine reaches the other sites only through the
    header Products menu and the footer product cards.
  - Cloud landing: "Built on the open-source Speedle Engine →".
  - AI home: the existing Engine and Cloud links use exactly these names,
    URLs and the Cloud badge.
- **English and Chinese stay in sync.** Both languages get the same structure,
  facts and links; see section 9.
- **Legal text** (privacy, terms, licence statements) changes only with the
  owner's verbatim approval.

## 12. Quality bar: checklist for every site change

- [ ] Read this file and the site's colour row before editing.
- [ ] Header parity at 1440 px and 390 px, every value within ± 1 px: header
      76; logo 152 / 128; divider 26 / 22; product word 22 / 19; nav
      15.04 px / 600; gap 30; GitHub pill 10 × 18; collapse at ≤ 1160.
- [ ] Footer parity (v16): 12 columns with 24 px gutters, padding 56 px 0
      40 px; lock-up logo 132; product word 19; tagline 15 / 24; social box
      36 with a 20 px icon and an 8 px gap; card gap 16, radius 12, padding
      16, sphere 28; names 15 / 24 600, descriptions 14 / 20 400 muted;
      legal bar 13 / 20 with a 20 px padding; breakpoints at 1024 and 720 px.
- [ ] No stand-alone product links in the desktop or collapsed header; Engine
      home has no contextual cross-links (v10/v12).
- [ ] Products menu, footer cards and permitted contextual links use the exact
      names, URLs, descriptions and Cloud "Coming soon" badge.
- [ ] Only Engine has a GitHub pill in the header or collapsed menu; Cloud and
      AI have none.
- [ ] Widths 1440, 390 and 320 px: no horizontal scroll; everything usable at
      320 px.
- [ ] WCAG AA (axe): 0 violations; text 4.5:1; large text 3:1; UI and focus
      3:1; every menu and link reachable by keyboard with a visible focus ring.
- [ ] No console errors and no 404 assets.
- [ ] English and Chinese pages match; the language link points to the same
      page; `hreflang` alternates are present.
- [ ] Favicons: `favicon.svg`, `favicon.ico` (16/32/48), `apple-touch-icon.png`
      (180), sphere only in the site colour, no underlay polygon or dark
      outline; strengthened sphere strokes keep the 16 px render legible. The
      v13 rollout stays pending until the favicon fix can merge last.
- [ ] Analytics beacon on every public page with that host's token; never on
      signed-in app pages or API responses; Cloud CSP split as in section 10.
- [ ] Claims checked; SPDL examples tested; no unsupported partner,
      availability or performance claims.
- [ ] Screenshots and other evidence are not committed or uploaded.
- [ ] Local checks from the repository root:
      `python3 website/tests/test_language_routes.py`,
      `python3 website/localize-links.py docs --check`,
      `HUGO_BIN=<hugo-0.48> website/build-site.sh /tmp/speedle-site`, and
      `website/playground-engine/verify.sh` where the change touches the site
      tooling.
- [ ] PR body: the header/footer parity table plus the exact commands and their
      results; CI green; coordinator review.

## 13. Process and change control

- Website PRs merge on green CI after coordinator review (owner rule).
- Legal text changes need the owner's verbatim approval before merge.
- Do not add CI steps or artifacts that capture or upload screenshots or other
  evidence; put the commands and results in the PR description.
- **To change this standard**: open a PR to this file first, describing the
  rule and the measured values behind it. After it merges, apply it to the
  three sites in separate PRs. Until it merges, the owner-approved header
  standard (v2–v13) remains the header reference.
- Each site repository's `AGENTS.md` points here; keep the pointer current.
