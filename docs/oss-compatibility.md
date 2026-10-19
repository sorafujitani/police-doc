# OSS documentation compatibility check

## Current result

The same pinned corpus was rescanned with the DNF5 reference in
[examples/specs/dnf5.json](../examples/specs/dnf5.json), live help fallback, and `--require-complete`.

| Measure | Result |
| --- | ---: |
| Documents / extracted results | 13 / 545 |
| Checked against documented argument rules | 131 |
| Partial / uncheckable | 251 / 163 |
| Errors / warnings | 0 / 202 |
| Exit code / complete coverage | 1 / false |

**The whole corpus is not verified.** There are still 414 incomplete results.
Every incomplete result has explicit `requirements`; those requirements are not successful checks or suppressions.
The checked results use recognized help synopses, the installed Bash's builtin rules, and the selected DNF5 reference.
They do not establish behavior for every distribution or version mentioned by the documents.

The preceding reference-only improvement checked 19 of 528 results.
The new extractor also inspects compound bodies and declarations and recognizes `❯` transcripts, so result counts are not a one-to-one accuracy comparison.
Long fzf signatures retain their value boundaries, and advertised ripgrep long help supplies modes omitted from its short synopsis.

The GitHub CLI document's explicitly labeled DNF5 section was also tested separately, without editing its four command examples.
All four were checked against the pinned reference with no errors or warnings and exit code `0`.
Separate mutation tests detect missing operands, missing flag values, extra operands, and unlisted flags.
DNF was neither installed nor started.

Fresh and cached corpus reports matched byte for byte.
Exporting the available specifications and rescanning with an empty `PATH` also retained 131 checked results and incomplete overall coverage.
That offline run had 252 partial and 162 uncheckable results; inheriting available catalog evidence changes one incomplete classification, not the number of checked results.
Export preserved both known synopsis rules and uncertainty.

### What still needs input or implementation

- 116 results could not collect target metadata. Missing package-manager CLIs, local scripts, nonstandard help protocols, and output mistakenly labeled as shell are distinct causes; the report retains each target and reason.
- Some installed help omits option ordering, argument boundaries, aliases, or value rules. The scanner does not manufacture those rules from an option's name.
- Unbound variables, substitutions, functions, aliases, interactive keystrokes, templates, and configuration fragments need concrete inputs or an explicit contract for their role.

No arbitrary CLI or unconstrained shell expansion can be certified from a name alone.
These cases stay incomplete and make `--require-complete` fail until the necessary rules or context are supplied.

## Earlier help-only result

Checked on 2026-10-07 on macOS arm64, using installed CLIs and unmodified Markdown from nine OSS projects (ten repositories).

| Metric | Before | After |
| --- | ---: | ---: |
| Documents | 13 | 13 |
| Extracted results | 444 | 527 |
| Partially checked | 164 | 278 |
| Uncheckable | 280 | 249 |
| Errors | 0 | 0 |
| Warnings | 245 | 215 |

"Before" is the working tree after the earlier ccsession help fixes, not a released version.
"After" includes the fixes below. Extraction changed, so these totals are not an accuracy or false-positive-rate comparison.
Partially checked means help-backed checks were possible; it does not mean a command was executed or proved valid.
Uncheckable results include unsupported shell blocks and builtins as well as unavailable executables.

## Fixes backed by regressions

| Finding | Change |
| --- | --- |
| GitHub CLI's `shell` fences and ripgrep's unlabeled `$ ` transcripts were skipped | Recognize both; continue ignoring other unlabeled data |
| fd's leading `> ` was parsed as a redirection, and output filenames became executable names | Mark the ambiguous block uncheckable; do not guess whether it is a prompt |
| Docker's diagnostic-only console output became a command named `WARNING:` | Mark diagnostic-only console blocks uncheckable |
| Library, interpreter, or repeated version strings stopped collection for curl, pip, fzf, and Nix tools | Recognize a leading CLI version, repeated equivalent versions, and Nix distribution banners; retain ambiguity checks otherwise |
| GNU Usage lines absorbed following prose; rustup's `[EXE]` placeholder broke child help | Stop at unindented prose and normalize the optional executable suffix |
| Abbreviated help omitted available options | Follow the explicit longer-help hint within the call budget; use curl's full option catalog |
| ripgrep's "OTHER BEHAVIORS" options were dropped | Recognize that option group |

Regression tests cover these formats without network access.
The opt-in real-CLI matrix was extended to curl, fd, bat, fzf, rustup, and ripgrep's extra option group.
It checks known-good commands for erroneous review findings, an intentionally unknown flag, required-value omissions where established by help, and repeat scans with cached help.
Honest incomplete-grammar notices are still allowed.

## Corpus

All source URLs below were fetched successfully and pin a commit.
Each count is **extracted / partially checked / warnings**.

| Project / document | Before | After |
| --- | --- | --- |
| BurntSushi/ripgrep: `README.md` | 0 / 0 / 0 | 41 / 10 / 34 |
| BurntSushi/ripgrep: `GUIDE.md` | 0 / 0 / 0 | 51 / 43 / 20 |
| sharkdp/fd: `README.md` | 90 / 18 / 75 | 60 / 17 / 28 |
| sharkdp/bat: `README.md` | 89 / 34 / 45 | 89 / 41 / 38 |
| junegunn/fzf: `README.md` | 83 / 9 / 49 | 83 / 44 / 19 |
| astral-sh/uv: `README.md` | 25 / 19 / 5 | 25 / 21 / 3 |
| astral-sh/uv: `docs/getting-started/features.md` | 0 / 0 / 0 | 0 / 0 / 0 |
| cli/cli: `README.md` | 0 / 0 / 0 | 4 / 3 / 1 |
| cli/cli: `docs/install_linux.md` | 58 / 3 / 49 | 75 / 14 / 55 |
| docker/cli: `docs/reference/commandline/run.md` | 0 / 0 / 0 | 0 / 0 / 0 |
| npm/cli: `docs/lib/content/commands/npm-install.md` | 44 / 38 / 12 | 44 / 38 / 12 |
| nodejs/node: `doc/api/cli.md` | 51 / 40 / 8 | 51 / 44 / 4 |
| docker/docs: `content/manuals/engine/containers/resource_constraints.md` | 4 / 3 / 2 | 4 / 3 / 1 |

The uv feature index contains inline commands rather than supported fenced examples.
The Docker CLI reference is an option table without fenced examples. Their zero counts are not validation successes.
The fd count decreases because output lines are no longer reported as commands.

### Pinned sources

- BurntSushi/ripgrep, `README.md`: <https://raw.githubusercontent.com/BurntSushi/ripgrep/3fce3b5bb0236da2df6d99672afb8a719642eca7/README.md>
- BurntSushi/ripgrep, `GUIDE.md`: <https://raw.githubusercontent.com/BurntSushi/ripgrep/3fce3b5bb0236da2df6d99672afb8a719642eca7/GUIDE.md>
- sharkdp/fd, `README.md`: <https://raw.githubusercontent.com/sharkdp/fd/14dcd92fb76ca0ebc2e82671a275f67c790d25fc/README.md>
- sharkdp/bat, `README.md`: <https://raw.githubusercontent.com/sharkdp/bat/4608fc959aa8abf80d32198836511a570b7ae9ea/README.md>
- junegunn/fzf, `README.md`: <https://raw.githubusercontent.com/junegunn/fzf/b1be3a8be1b833ce5b92fbbac11637643d60a046/README.md>
- astral-sh/uv, `README.md`: <https://raw.githubusercontent.com/astral-sh/uv/27b9edf780130acfb366e1976ce7414c96e3c2e6/README.md>
- astral-sh/uv, `docs/getting-started/features.md`: <https://raw.githubusercontent.com/astral-sh/uv/27b9edf780130acfb366e1976ce7414c96e3c2e6/docs/getting-started/features.md>
- cli/cli, `README.md`: <https://raw.githubusercontent.com/cli/cli/996be976cad960a726aabb94be67dfc7e31e317b/README.md>
- cli/cli, `docs/install_linux.md`: <https://raw.githubusercontent.com/cli/cli/996be976cad960a726aabb94be67dfc7e31e317b/docs/install_linux.md>
- docker/cli, `docs/reference/commandline/run.md`: <https://raw.githubusercontent.com/docker/cli/3d2650ce76053393872e8389c2b77033146cb356/docs/reference/commandline/run.md>
- npm/cli, `docs/lib/content/commands/npm-install.md`: <https://raw.githubusercontent.com/npm/cli/b317f16c80df02ea3628cfa77170d5ae9b59720c/docs/lib/content/commands/npm-install.md>
- nodejs/node, `doc/api/cli.md`: <https://raw.githubusercontent.com/nodejs/node/e36633a53108a0fff71b2236a3426c607f30bd6e/doc/api/cli.md>
- docker/docs, `content/manuals/engine/containers/resource_constraints.md`: <https://raw.githubusercontent.com/docker/docs/17062882e67c2c0ac4d1909d38c91c81889cfbf0/content/manuals/engine/containers/resource_constraints.md>

## Help-only gaps

The counts below describe the earlier help-only run. A reference supplies additional rules only for its explicitly modeled target; all other gaps remain.

Of the 215 warnings:

| Diagnostic | Count | Interpretation |
| --- | ---: | --- |
| `help-unavailable` | 145 | 128 missing executables, 2 missing local paths, 15 unsupported version/help requests or missing manuals |
| `unchecked-arguments` | 41 | Positional tails, expansions, or forwarded flags cannot be checked safely |
| `unknown-flag-arity` | 12 | Includes fd's variadic command arguments, fzf's height syntax, and Docker's custom ulimit value type |
| `unverified-flag` | 9 | Node/V8 options and bat/fd options absent from the collected help; not proof that those flags are invalid |
| `invalid-shell` | 7 | Documentation templates such as `<TAB>` and `<shell>`, a URL grammar, and a dependency tree labeled as Bash |
| `unverified-command` | 1 | `git ls-files` is absent from the advertised root help catalog |

Installed-tool collection gaps remain for macOS utilities with nonstandard version/help protocols (`unzip`, `find`, `xargs`, `ps`, `iconv`), `json_pp`, `man`, and Vim's usage format.
The installed `nix-shell` and `nix-env` cannot display their help because their manual entries are unavailable.
These are unresolved coverage gaps, not passed checks.
No packages or manuals were installed to hide environment-dependent failures.

## Verification and reproduction

The corpus was downloaded outside the repository. The scanner invoked only metadata requests; it did not run installation, container, network, or other commands copied from the documents.

Selected installed versions: rg 15.1.0, fd 10.4.2, bat 0.26.1, fzf 0.71.0, uv 0.11.7, gh 2.91.0, docker 29.8.0, npm 11.19.0, node 24.21.0, curl 8.19.0, git 2.53.0, rustup 1.29.1, and GNU coreutils 9.10.
Results depend on these executables and the selected PATH.

Download the pinned sources into a directory, then run:

```sh
go build -o ./policedoc ./cmd/policedoc
./policedoc scan /path/to/corpus --refresh --format json --fail-on none
POLICEDOC_REAL_CLI_TESTS=1 go test -race ./...
go vet ./...
```

To reproduce the reference-backed scan:

```sh
./policedoc scan /path/to/corpus --spec examples/specs/dnf5.json \
  --allow-live-help --require-complete --format json
```

This scan must exit `1` until its remaining gaps are resolved.
See [reference specifications](references.md) for export, offline use, and the boundary of a `checked` result.

The real-CLI suite, reference tests, race detector, `go vet`, formatting checks, and `git diff --check` passed.
The real-CLI suite remains opt-in so normal tests do not require installed tools.
