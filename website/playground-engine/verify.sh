#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
website/playground-engine/build.sh
node website/playground-engine/tests/form-model.test.cjs
node website/playground-engine/tests/form-dom.test.cjs
PLAYGROUND_LOCALE=zh node website/playground-engine/tests/form-dom.test.cjs
node website/playground-engine/tests/site.test.cjs
python3 website/tests/test_language_routes.py
python3 website/localize-links.py docs --check
python3 website/tests/check_seo.py docs
node website/tests/check-doc-examples.cjs
website/playground-engine/verify-hugo.sh
git diff --exit-code -- docs/playground docs/hugo/static/playground docs/css docs/hugo/static/css
test -z "$(git ls-files --others --exclude-standard -- docs/playground docs/hugo/static/playground docs/css docs/hugo/static/css)"
