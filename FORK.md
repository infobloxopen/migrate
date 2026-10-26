# Infoblox fork

`ib` is the only maintained branch. It tracks golang-migrate/migrate by merging
`upstream/master` (never rebasing, so history stays shared).

## Removed from upstream

| Path | Why |
| --- | --- |
| `database/yugabytedb` | Not used by Infoblox; drops its dependencies and CVE surface. |

The list lives in `scripts/fork-removed.txt`. To remove another upstream
component, delete it, add its path to that file, and drop its name from the
`DATABASE` list in the `Makefile` and its line in `README.md`.

## Syncing upstream

```
git switch -c sync-upstream origin/ib
scripts/sync-upstream.sh
```

The script merges upstream, re-deletes everything in `fork-removed.txt`,
regenerates `go.mod`/`go.sum`, builds and vets. It stops with exit 2 and lists
the files if anything else conflicts. Then run `govulncheck ./...` and the
tests and open a PR into `ib`.
