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
python3 website/tests/check_seo.py docs
node website/tests/check-doc-examples.cjs
go test -count=1 ./website/tests/docscheck
python3 website/tests/check_docs.py
python3 website/tests/check_quick_start.py --toolchain go1.26.10
python3 website/tests/check_quick_start.py --toolchain go1.27.3
website/playground-engine/verify-hugo.sh
# Run the full container gate on the merge candidate and after publication.
# A branch push has a parallel PR run; duplicating kind here occupies both
# runner processes while one waits for the same Docker-host lock.
if [[ "${GITHUB_ACTIONS:-}" == true && "${GITHUB_EVENT_NAME:-}" == push && "${GITHUB_REF:-}" == refs/heads/* && "${GITHUB_REF:-}" != refs/heads/master ]]; then
  echo 'Deployment acceptance is covered by the PR run; branch push keeps the build, native docs and website checks.'
else
  python3 deployment/tests/verify.py
fi
git diff --exit-code -- docs/playground docs/hugo/static/playground docs/css docs/hugo/static/css
test -z "$(git ls-files --others --exclude-standard -- docs/playground docs/hugo/static/playground docs/css docs/hugo/static/css)"
