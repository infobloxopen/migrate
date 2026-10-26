#!/usr/bin/env bash
# Merge golang-migrate/migrate into this fork's ib branch and re-apply the fork's
# removals (scripts/fork-removed.txt). Run from the repo root on a clean branch
# cut from ib; open the result as a PR. Nothing is pushed.
#
#   UPSTREAM_REF=upstream/master scripts/sync-upstream.sh
set -euo pipefail

UPSTREAM_REF=${UPSTREAM_REF:-upstream/master}
REMOVED_LIST=scripts/fork-removed.txt

git diff --quiet && git diff --cached --quiet || { echo "working tree not clean" >&2; exit 1; }

if [[ $UPSTREAM_REF == upstream/* ]]; then
  git remote get-url upstream >/dev/null 2>&1 ||
    git remote add upstream https://github.com/golang-migrate/migrate.git
  git fetch upstream --tags
fi

removed() { grep -vE '^\s*(#|$)' "$REMOVED_LIST"; }

# Deleted-here paths that upstream touched show up as modify/delete conflicts,
# so --no-commit lets us finish the resolution before the merge commit exists.
git merge --no-commit --no-ff "$UPSTREAM_REF" || true

while read -r path; do
  git rm -r -q --ignore-unmatch -- "$path"
done < <(removed)

# go.mod/go.sum conflicts are regenerated rather than hand-merged: take ours,
# then let tidy pull in whatever the merged sources import.
for f in go.mod go.sum; do
  if git ls-files -u -- "$f" | grep -q .; then git checkout --ours -- "$f"; git add "$f"; fi
done

if git ls-files -u | grep -q .; then
  echo "Unresolved conflicts need a human:" >&2
  git diff --name-only --diff-filter=U >&2
  exit 2
fi

go mod tidy
git add go.mod go.sum
go build ./... && go vet ./...
git commit -q -m "Merge ${UPSTREAM_REF} into ib"
echo "Merged ${UPSTREAM_REF}. Run govulncheck and the tests, then open a PR."
