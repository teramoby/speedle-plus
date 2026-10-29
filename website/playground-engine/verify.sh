#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
website/playground-engine/build.sh
node website/playground-engine/tests/form-model.test.cjs
node website/playground-engine/tests/form-dom.test.cjs
PLAYGROUND_LOCALE=zh node website/playground-engine/tests/form-dom.test.cjs
node website/playground-engine/tests/site.test.cjs
node --check docs/hugo/static/js/command-copy.js
python3 website/tests/test_language_routes.py
python3 website/localize-links.py docs --check
# Merge gating checks the quick start with the CI Go release; the nightly run
# (scripts/ci/run-go-tests.sh) covers every supported release.
for toolchain in ${QUICK_START_TOOLCHAINS:-go1.26.10}; do
  python3 website/tests/check_quick_start.py --toolchain "${toolchain}"
done
website/playground-engine/verify-hugo.sh
git diff --exit-code -- docs/playground docs/hugo/static/playground docs/css docs/hugo/static/css
test -z "$(git ls-files --others --exclude-standard -- docs/playground docs/hugo/static/playground docs/css docs/hugo/static/css)"
