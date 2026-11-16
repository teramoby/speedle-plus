#!/usr/bin/env bash
# Classify changed paths for the quick/full CI routes.
set -euo pipefail
case "${EVENT_NAME}" in
  pull_request) base="${PR_BASE}" ;;
  push) base="${PUSH_BEFORE}" ;;
  *) base="" ;;
esac
go=true website=true docs=true ci=true
changed=""
if [[ -n "${CI_CHANGED_FILES_FILE:-}" ]]; then
  # The route gate supplies a NUL-delimited base...head diff obtained from the
  # API. This path does not require a checkout or a local Git repository.
  if [[ ! -f "${CI_CHANGED_FILES_FILE}" || ! -r "${CI_CHANGED_FILES_FILE}" ]]; then
    echo "CI_CHANGED_FILES_FILE is not a readable regular file" >&2
    exit 1
  fi
  changed="${CI_CHANGED_FILES_FILE}"
elif [[ -n "${base}" && "${base}" != 0000000000000000000000000000000000000000 ]] && git cat-file -e "${base}^{commit}" 2>/dev/null; then
  changed=$(mktemp "${TMPDIR:-/tmp}/speedle-ci-paths.XXXXXX")
  trap 'rm -f "${changed}"' EXIT
  # Write the diff first so Git failures propagate instead of being hidden in
  # process substitution. NUL separators preserve quoted and multiline paths.
  # A pull request counts only its own side (three dots: from the merge base),
  # so base-branch commits missing from a stale merge ref are not its changes.
  range=("${base}" "${GITHUB_SHA}")
  [[ "${EVENT_NAME}" == pull_request ]] && range=("${base}...${GITHUB_SHA}")
  git diff --name-only -z "${range[@]}" > "${changed}"
fi
if [[ -n "${changed}" ]]; then
  go=false website=false docs=false ci=false
  while IFS= read -r -d '' file; do
    case "${file}" in
      '') ;;
      docs/*) docs=true ;;
      website/*) website=true ;;
      # Every CI script change enables every route by design: the route gate
      # consumes these files, so neither a pull-request run nor a push may
      # narrow the matrix from a classification it controls.
      scripts/ci/*) go=true website=true docs=true ci=true ;;
      .github/workflows/*|scripts/*) go=true ci=true ;;
      # Module files and root Markdown carry the toolchain contract that the
      # CI script tests assert against README.md; run those tests for them.
      # `*` in a case pattern matches `/`, so the nested-path arm must come
      # before the root-only `*.md` arm.
      go.mod|go.sum) go=true ci=true ;;
      */*) go=true ;;
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
