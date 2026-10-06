# Releasing policedoc

Releases are published from https://github.com/sorafujitani/police-doc.
Stable releases also update https://github.com/sorafujitani/homebrew-tap.

## Version and prerequisites

The Git tag is the release version. GoReleaser injects it into `main.version`.
Keep the source default `dev` and the Nix checkout version `unstable-*` unchanged.
For a patch release, increment the latest stable tag's patch component. Check both
remote tags and GitHub releases; do not reuse an existing or partially published tag.

```sh
git status --short
git remote -v
git ls-remote --tags --refs origin
gh release list --repo sorafujitani/police-doc --limit 5
```

Use the existing Git identity and authenticated GitHub account. Confirm the intended
branch, changes and publication target before committing. Do not include unrelated
work or publish another repository as part of this release.

The `Release` workflow requires `HOMEBREW_TAP_GITHUB_TOKEN`, with **Contents: read
and write** access to `sorafujitani/homebrew-tap`. The workflow's `GITHUB_TOKEN`
publishes this repository's assets but cannot update the separate tap. Check the
secret's presence without displaying its value.

## Validate the release contents

Use passing results for the exact code being released, or run the missing checks:

```sh
go test -race ./...
go vet ./...
goreleaser check
nix build "path:$PWD#policedoc" --no-link
goreleaser release --snapshot --clean
```

The explicit Nix `path:` source includes new files before they are staged. A plain
Git-backed `nix build .#policedoc` excludes untracked files. Nix runs the tests and
checks the installed binary. Update `vendorHash` only when dependencies change;
use the hash Nix reports after temporarily setting it to `pkgs.lib.fakeHash`.

Snapshot builds test packaging without publishing. GoReleaser builds macOS and
Linux archives for amd64 and arm64. Keep generated `dist/` output out of commits.
When help collection changes, also run the opt-in installed-CLI tests documented
in `README.md`; skip unavailable CLIs rather than installing them implicitly.

## Publish

Commit only the approved release contents and push the intended branch. Wait for
the `CI` workflow on that exact commit, including its Linux and macOS Nix jobs.
Set `TAG` to the chosen unused `vMAJOR.MINOR.PATCH` and `SHA` to the verified commit.
Create an annotated tag at that commit, then push that tag to `origin`.
Follow the active harness's Git-delegation rules for these mutations.

A `v*` tag push starts `.github/workflows/release.yml`. That workflow tests the
code, runs GoReleaser, publishes archives and checksums, and updates the tap for
stable versions. Do not create a duplicate release manually or run a second
publisher while the workflow is active.

Find the `Release` run for the tag and exact commit, then wait for it to finish:

```sh
gh run list --repo sorafujitani/police-doc --workflow release.yml --branch "$TAG" --json databaseId,headSha,status,conclusion
gh run watch "$RUN_ID" --repo sorafujitani/police-doc --exit-status
```

If publication fails, inspect the failed job and already-published state before
retrying. Never move or replace a published tag, delete a release, or force-push
as an automatic recovery step.

## Verify publication

A successful tag push is not completion. Confirm:

- The tag resolves to `SHA`, and the `Release` run for that SHA succeeded.
- The GitHub release is published with the intended stable/prerelease status.
- All four platform archives and `checksums.txt` are present.
- The downloaded host archive matches `checksums.txt`; its `policedoc version`
  reports the tag without its leading `v`. Extract into a temporary directory,
  without replacing the user's installed binary.
- For a stable release, `Formula/policedoc.rb` in the tap has the intended version,
  release asset URLs and matching archive checksums. Verify the remote formula;
  do not update the user's local Homebrew installation unless requested.

Report the verified release URL and any unresolved publishing or verification
failure. Keep repository-specific procedure changes in this document and the
owning workflow/configuration, not in a separate copy of the release procedure.
