# policedoc

Check CLI examples in Markdown using help from installed executables.
No configuration or manually maintained specification is needed.

`scan` never executes document examples. It does start executables for version and help requests.
Use only trusted documents and binaries.

## Install

### Go

```sh
go install github.com/sorafujitani/police-doc/cmd/policedoc@latest
```

Go 1.27.1 or later is required. Add `$(go env GOPATH)/bin` to `PATH`, or use your
custom `GOBIN` directory.

### Nix

With flakes enabled, install on Linux or macOS (x86-64 or ARM64):

```sh
nix profile install github:sorafujitani/police-doc#policedoc
```

Or run without installing:

```sh
nix run github:sorafujitani/police-doc -- scan README.md docs/
```

### Homebrew

Install a tagged release from https://github.com/sorafujitani/homebrew-tap:

```sh
brew install sorafujitani/tap/policedoc
```

If Homebrew requests trust, run `brew trust --formula sorafujitani/tap/policedoc`
and retry the installation.

Homebrew packages are available after the first stable release is published.
They include prebuilt binaries for Linux and macOS (x86-64 and ARM64).

### Build from source

Use Go 1.27.1 or later to build the single binary:

```sh
go build -o policedoc ./cmd/policedoc
./policedoc scan README.md docs/
```

Go is not needed to run policedoc. The inspected CLI still needs its own runtime.

Try the [examples](examples/README.md) to see findings and exit codes.

## Options

```sh
./policedoc scan docs/ --format json --fail-on warning
./policedoc scan docs/ --refresh
```

| Option | Purpose |
| --- | --- |
| `--refresh` | Recollect help for CLIs used in the documents |
| `--cache-dir PATH` | Cache location; default: `.policedoc/cache` |
| `--format text\|json` | Report format; default: `text` |
| `--verbose` | Include INFO messages, target versions, and collection details in text output |
| `--fail-on error\|warning\|none` | Failure threshold; default: `error` |

Options may appear before or after input paths.
Use `policedoc --help` for usage or `policedoc version` for the scanner version.

## What gets checked

Markdown fences labeled `sh`, `bash`, or `console` are supported.
The scanner handles continuation lines, pipes, `&&`, and `||`.
In any supported fence, a first nonempty, non-comment line starting with `$ ` marks a terminal transcript.
Only prompted commands and their continuation lines are checked; terminal output is ignored.
Known POSIX and Bash builtins, such as `cd`, `echo`, and `source`, receive an INFO notice instead of external help collection.

- A flag with an explicit value requirement in help produces an error when its value is missing.
- A flag absent from collected help produces a review warning, not a claim that it is invalid.
- Flags documented without a value do not stop checking later flags. Short flag groups and attached values are checked for getopt-style short/long aliases when each component's value rule is known. Single-dash long-flag formats are not split.
- For `--flag[=VALUE]`, a separate following word is not consumed as its value, so checking continues.
- Values outside an explicit choice list in help produce review warnings. Reordering the same choices does not change the constraint; examples and defaults do not establish permitted choices.
- Known subcommand names and documented aliases select their collected help.
- An unlisted subcommand produces a review warning when usage identifies a command position and help lists available commands. It is not proof that the name is invalid.
- Flags after positional arguments are checked when usage explicitly documents that order. Positional values and bounds remain unchecked.
- Ambiguous value rules or positional tails containing unchecked flags produce warnings. Dynamic words and arguments after `--` are not expanded or checked.

All help-based checks are partial. Help cannot establish complete grammar or hidden entries.
Required flags, positional bounds, deprecation, removal, and replacements are not inferred.
Undocumented aliases, inherited flags absent from help, and argument forwarding are not guessed.
No examples, substitutions, or variable expansions are executed; documents are never rewritten.

Text reports show errors, warnings, and concrete gaps such as skipped arguments, grouped by source command, followed by counts and coverage.
Use `--verbose` for all INFO messages, target CLI/version/OS, evidence, and diagnostic certainty.
INFO describes incomplete checking, not a successful validation.
Coverage distinguishes partial checks from uncheckable examples; no result claims full validation.
JSON always includes all diagnostics and collection details, regardless of `--verbose`, using schema version `2`.

| Exit code | Meaning |
| --- | --- |
| `0` | No findings reach the threshold |
| `1` | Findings reach the threshold |
| `2` | Operational failure |

`warning` fails on warnings or errors. `none` disables finding-based failures.
Exit code `0` does not mean that every example was checked. Always inspect coverage.

## Collection and caching

The scanner queries each executable's version once per run.
It reuses successful help when the version and executable metadata are unchanged,
and extends the cache when new examples need additional commands.
Examples that resolve to the same executable share its cache, including `npx`
examples in different directories. Different local executables remain separate.
Failed requests are retried on the next scan; they are not stored as permanent failures.
Every run still checks all input examples. Use `--refresh` after plugin or environment changes.

Collection starts with root help, then follows advertised subcommands reached
while checking the examples. An unlisted command name triggers a bounded search
of advertised sibling help for aliases. Alias resolution requires the sibling search
to finish without conflicts; incomplete searches leave a review warning.
The unlisted name itself, flag values,
positional arguments, dynamic words, and forwarded arguments are never used as help requests.
Most CLIs use `-h`, then `--help` as a fallback.
Limits are three levels and 100 help calls per executable per scan, including failed calls.
Alias discovery has a separate 100-call budget so it cannot exhaust normal collection.
Each call allows ten seconds and 1 MiB of combined output, with no interactive input.
For Homebrew, `brew commands --quiet` lists commands and `brew help <command>`
collects their help within the same limits.
For Go, `version`, `help`, and `help <command>` are used instead;
automatic toolchain downloads are disabled.

Common static `sudo` and bare-executable `npx` forms resolve to the inner CLI.
Neither wrapper runs, and no packages are installed.
For `npx`, lookup starts in ancestor `node_modules/.bin` directories, then checks `PATH`.

Missing executables or failed collection produce warnings and uncheckable results.
A failed child-help request leaves a placeholder and warns only on examples that need it.
Unrelated failures and unvisited commands do not produce warnings on other examples.
Usage-only help is accepted for commands without listed flags or subcommands.
Help for a parent or a different command is not used to validate a child.
An ambiguous synopsis, such as an unmarked lowercase word that could name a
further subcommand, remains uncheckable rather than proving a flag error.
Cache-write failures warn but retain the collected help for the current scan.
Caches are internal snapshots, not a supported specification-input format.
Old or corrupt caches are rebuilt automatically.
An installed executable is required even when its help is cached.

Help collection is not a sandbox: even help requests can have side effects.
Directory scans skip `.git`, `node_modules`, `vendor`, and symbolic links.
Limits are 16 MiB per Markdown file and 8 MiB per cache entry.

## Development

```sh
go test -race ./...
go vet ./...
go build ./cmd/policedoc
```

Optional integration tests use installed Go, Git, Node.js, Python, uv, ripgrep,
Homebrew, and npm.

```sh
POLICEDOC_REAL_CLI_TESTS=1 go test ./internal/app -run TestRealCLIHelpAndScan -v
```

### Packaging and releases

```sh
nix build .#policedoc
goreleaser check
goreleaser release --snapshot --clean
```

The Nix build runs the tests and checks the installed binary. Use an explicit
`path:` source before staging new files; Git-backed flakes omit untracked files.
Snapshot builds do not publish anything.

Follow the [release guide](docs/releasing.md) to choose a version, validate the
commit, publish its tag, and verify the release assets and Homebrew formula.

See the [checking specification](docs/init.md). Licensed under [MIT](LICENSE).
