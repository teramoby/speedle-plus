#!/usr/bin/env bash
# Classify changed paths for the quick/full CI routes.
set -euo pipefail
case "${EVENT_NAME}" in
  pull_request) base="${PR_BASE}" ;;
  push) base="${PUSH_BEFORE}" ;;
  *) base="" ;;
esac
go=true website=true docs=true ci=true
if [[ -n "${base}" && "${base}" != 0000000000000000000000000000000000000000 ]] && git cat-file -e "${base}^{commit}" 2>/dev/null; then
  changed=$(mktemp "${TMPDIR:-/tmp}/speedle-ci-paths.XXXXXX")
  trap 'rm -f "${changed}"' EXIT
  # Write the diff first so Git failures propagate instead of being hidden in
  # process substitution. NUL separators preserve quoted and multiline paths.
  git diff --name-only -z "${base}" "${GITHUB_SHA}" > "${changed}"
  go=false website=false docs=false ci=false
  while IFS= read -r -d '' file; do
    case "${file}" in
      '') ;;
      docs/*) docs=true ;;
      website/*) website=true ;;
      .github/workflows/*|scripts/*) go=true ci=true ;;
      */*) go=true ;;
      # Root Markdown includes README.md, whose toolchain requirement the CI
      # script tests assert against go.mod; run those tests for it.
      *.md) ci=true ;;
      *) go=true ;;
    esac
  done < "${changed}"
fi
{
  echo "go=${go}"
  echo "website=${website}"
  echo "docs=${docs}"
  echo "ci=${ci}"
} >> "${GITHUB_OUTPUT}"
echo "Go: ${go}; website tooling: ${website}; docs: ${docs}"
