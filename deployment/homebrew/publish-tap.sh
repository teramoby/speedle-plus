#!/bin/bash
# Publish deployment/homebrew to teramoby/homebrew-tap and open a pull request
# so this repository keeps the same formulas.
#
# HOMEBREW_TAP_TOKEN must be able to push a branch and open a pull request on
# teramoby/homebrew-tap. The script pushes that branch and opens a pull request
# against main. It never pushes main or master. When the token is unset, this
# script prints a skip message and exits 0. The release is unchanged.
# The token is passed to gh through GH_TOKEN and is never printed. It is unset
# before this repository is updated. The pull request on this repository uses
# GITHUB_TOKEN.
#
# teramoby/homebrew-tap must already exist and its default branch must be main.
# brew install reads that default branch after the tap pull request is merged.

set -euo pipefail

if [[ -z "${HOMEBREW_TAP_TOKEN:-}" ]]; then
  echo "HOMEBREW_TAP_TOKEN is not set. Skipping the Homebrew tap update. The release is unchanged."
  exit 0
fi

if [[ ! "${VERSION:-}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "VERSION must look like v1.2.3." >&2
  exit 1
fi

script_dir=$(dirname -- "$0")
script_dir=$(cd -- "${script_dir}" >/dev/null && pwd)
checksums="${CHECKSUMS:?CHECKSUMS is required}"
if [[ "${checksums}" != /* ]]; then
  checksums="${PWD}/${checksums}"
fi

python3 "${script_dir}/update-formulas.py" "${VERSION}" "${checksums}" "${script_dir}/Formula"

tap_repo="teramoby/homebrew-tap"
source_repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
workspace="${GITHUB_WORKSPACE:?GITHUB_WORKSPACE is required}"
tap_branch="homebrew-${VERSION}"
if [[ "${tap_branch}" == "main" || "${tap_branch}" == "master" ]]; then
  echo "Refusing to push the tap default branch ${tap_branch}." >&2
  exit 1
fi

workdir=$(mktemp -d)
cleanup() {
  rm -rf "${workdir}"
}
trap cleanup EXIT

export GH_TOKEN="${HOMEBREW_TAP_TOKEN}"
gh auth setup-git --hostname github.com
gh repo clone "${tap_repo}" "${workdir}/tap" -- --depth 1

default_branch=$(git -C "${workdir}/tap" symbolic-ref --short refs/remotes/origin/HEAD)
default_branch="${default_branch#origin/}"
if [[ "${default_branch}" != "main" ]]; then
  echo "teramoby/homebrew-tap default branch is ${default_branch}. The tap update opens a pull request against main." >&2
  exit 1
fi

remote_branch=$(git -C "${workdir}/tap" ls-remote --heads origin "refs/heads/${tap_branch}")
if [[ -n "${remote_branch}" ]]; then
  # A depth-1 clone is single-branch. Name the remote-tracking ref here;
  # a bare fetch of the branch name only updates FETCH_HEAD.
  git -C "${workdir}/tap" fetch --depth 1 origin \
    "refs/heads/${tap_branch}:refs/remotes/origin/${tap_branch}"
  git -C "${workdir}/tap" checkout --quiet -B "${tap_branch}" "origin/${tap_branch}"
else
  git -C "${workdir}/tap" checkout --quiet -B "${tap_branch}"
fi

mkdir -p "${workdir}/tap/Formula"
cp "${script_dir}/Formula/"*.rb "${workdir}/tap/Formula/"
cp "${script_dir}/README.md" "${workdir}/tap/README.md"
git -C "${workdir}/tap" config user.name "github-actions[bot]"
git -C "${workdir}/tap" config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git -C "${workdir}/tap" add -- README.md Formula
if ! git -C "${workdir}/tap" diff --cached --quiet; then
  git -C "${workdir}/tap" commit -s -m "Update Speedle formulas to ${VERSION}"
fi

current_branch=$(git -C "${workdir}/tap" rev-parse --abbrev-ref HEAD)
if [[ "${current_branch}" == "main" || "${current_branch}" == "master" ]]; then
  echo "Refusing to push the tap default branch ${current_branch}." >&2
  exit 1
fi

if [[ -n "${remote_branch}" ]]; then
  local_head=$(git -C "${workdir}/tap" rev-parse HEAD)
  remote_head=$(git -C "${workdir}/tap" rev-parse "origin/${tap_branch}")
  if [[ "${local_head}" != "${remote_head}" ]]; then
    git -C "${workdir}/tap" push origin "HEAD:${tap_branch}"
    echo "Pushed ${tap_branch} to ${tap_repo}."
  else
    echo "Homebrew tap branch ${tap_branch} already matches ${VERSION}."
  fi
else
  base_head=$(git -C "${workdir}/tap" rev-parse origin/main)
  local_head=$(git -C "${workdir}/tap" rev-parse HEAD)
  if [[ "${local_head}" != "${base_head}" ]]; then
    git -C "${workdir}/tap" push origin "HEAD:${tap_branch}"
    echo "Pushed ${tap_branch} to ${tap_repo}."
  else
    echo "Homebrew tap already matches ${VERSION}."
  fi
fi

if ! git -C "${workdir}/tap" diff --quiet origin/main -- README.md Formula; then
  existing=$(gh pr list --repo "${tap_repo}" --head "${tap_branch}" --base main --state open --json number --jq ".[0].number")
  if [[ -z "${existing}" || "${existing}" == "null" ]]; then
    cat > "${workdir}/tap-pr.md" <<EOF
Update the Speedle formulas to ${VERSION}.

The source of truth is https://github.com/${source_repo}/tree/master/deployment/homebrew.
EOF
    gh pr create --repo "${tap_repo}" --base main --head "${tap_branch}" \
      --title "chore(release): update Homebrew formulas to ${VERSION}" \
      --body-file "${workdir}/tap-pr.md"
  else
    echo "Pull request #${existing} already tracks ${tap_branch} on ${tap_repo}."
  fi
fi

unset GH_TOKEN
if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  echo "GITHUB_TOKEN is not set. Skipping the deployment/homebrew pull request."
  exit 0
fi
gh auth setup-git --hostname github.com

cd "${workspace}"
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git fetch --quiet origin master
source_branch="homebrew-${VERSION}"
if [[ "${source_branch}" == "main" || "${source_branch}" == "master" ]]; then
  echo "Refusing to push the default branch ${source_branch}." >&2
  exit 1
fi
# Keep a branch only while its pull request is still open. A retained branch
# from a merged pull request is not an ancestor of master after a squash, so
# counting commits on it would push the old history again. Delete that branch
# and start from master. Never force-push and never delete main or master.
open_pr=$(gh pr list --repo "${source_repo}" --head "${source_branch}" --base master --state open --json number --jq ".[0].number")
if [[ -n "${open_pr}" && "${open_pr}" != "null" ]]; then
  git fetch --quiet origin "${source_branch}"
  git checkout --quiet -B "${source_branch}" "origin/${source_branch}"
  git merge --quiet --no-edit origin/master
else
  if git ls-remote --exit-code --heads origin "${source_branch}" >/dev/null; then
    git push origin --delete "${source_branch}"
  fi
  git checkout --quiet -B "${source_branch}" origin/master
fi

mkdir -p deployment/homebrew/Formula
cp "${script_dir}/Formula/"*.rb deployment/homebrew/Formula/
cp "${script_dir}/README.md" deployment/homebrew/README.md
git add -- deployment/homebrew/Formula deployment/homebrew/README.md
if ! git diff --cached --quiet; then
  git commit -s -m "chore(release): update Homebrew formulas to ${VERSION}"
fi

ahead=$(git rev-list --count "origin/master..HEAD")
if [[ "${ahead}" == "0" ]]; then
  echo "deployment/homebrew already matches ${VERSION}."
  exit 0
fi

git push origin "HEAD:${source_branch}"
existing=$(gh pr list --repo "${source_repo}" --head "${source_branch}" --state open --json number --jq ".[0].number")
if [[ -z "${existing}" || "${existing}" == "null" ]]; then
  cat > "${workdir}/pr.md" <<EOF
Update the Homebrew formulas in \`deployment/homebrew/\` to ${VERSION}.

These files are the source of truth for \`teramoby/homebrew-tap\`. The Release workflow opened a pull request with the same files on that tap.
EOF
  gh pr create --repo "${source_repo}" --base master --head "${source_branch}" \
    --title "chore(release): update Homebrew formulas to ${VERSION}" \
    --body-file "${workdir}/pr.md"
else
  echo "Pull request #${existing} already tracks ${source_branch}."
fi
