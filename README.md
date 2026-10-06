# policedoc

Check CLI examples in Markdown using help from installed executables.
No configuration or manually maintained specification is needed.

`scan` never executes document examples. It does start executables for version and help requests.
Use only trusted documents and binaries.

## How it works

```sh
policedoc scan README.md docs/
```

1. Find CLI examples in `sh`, `bash`, and `console` code blocks.
2. Read version and help output from the installed CLIs, reusing cached help when possible.
3. Compare example flags, values, and subcommands with the collected help.
4. Report errors, review warnings, and unchecked parts, grouped by source command.

For example, [this Markdown file](examples/cases/git.md) contains both
`git grep -e policedoc` and `git grep -e`, which is missing its search pattern:

```console
$ policedoc scan examples/cases/git.md
examples/cases/git.md:18:1
  git grep -e
  ERROR missing-flag-value: -e requires a value.

1 files, 2 examples: 1 errors, 0 warnings
Coverage: 2 partially checked, 0 uncheckable. Use --verbose for details.
```

The scan exits with code `1` because Git's help says `-e` requires a value.
Neither search is executed.
A flag absent from help is instead a warning to review, not proof that it is invalid.
Coverage shows which examples were only partly checked or could not be checked.
By default, errors cause a nonzero exit code.

See [what gets checked](#what-gets-checked) for details and
[examples](examples/README.md) for sample reports.

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

policedoc reads `sh`, `bash`, and `console` code blocks.
It supports multiline commands, pipes, `&&`, and `||`. In terminal transcripts
marked with `$ ` prompts, it checks commands and ignores output.

| Check | Result |
| --- | --- |
| A flag is missing a value required by help | Error |
| A flag or subcommand is absent from help | Review warning, not proof it is invalid |
| A value is outside a choice list stated in help | Review warning |
| An executable or its help is unavailable | Warning; affected examples cannot be checked |

**Help-based checks are partial.** They do not prove that a command works.
Variable expansions, forwarded arguments after `--`, required flags, and
positional argument limits are not checked. Shell builtins such as `cd` and
`echo` are skipped. Documents are never executed or rewritten.

### Reading results

Reports group findings by source command and show file locations.
Always check **Coverage**: examples may be only partly checked or uncheckable,
even when there are no errors. Use `--verbose` for details or `--format json`
for machine-readable output.

| Exit code | Meaning |
| --- | --- |
| `0` | No errors by default; inspect warnings and coverage |
| `1` | Findings reach the `--fail-on` threshold |
| `2` | Operational failure |

Use `--fail-on warning` to fail on warnings too, or `--fail-on none` to report
findings without failing.

## Collection and caching

- **Install the CLIs first.** They must be available even when help is cached.
- **Only version and help requests run**, including help for needed subcommands.
  Document examples, `sudo`, and `npx` wrappers are not executed; no packages are installed.
- **Successful help is cached** in `.policedoc/cache`. It is recollected when
  the executable's version or metadata changes. Every scan still checks the examples again.
- **Use `--refresh` after plugin or environment changes.** Failed requests are
  retried on the next scan; outdated or corrupt caches are rebuilt automatically.

Only scan trusted documents and binaries: help requests are not sandboxed.
Directory scans skip `.git`, `node_modules`, `vendor`, and symbolic links.

See [checking details](docs/checking.md) for argument rules, help collection limits,
and cache behavior.

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
