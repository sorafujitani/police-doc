# policedoc

Find missing flag values and suspicious options in Markdown CLI examples.
policedoc compares the commands in your docs with help from the installed CLIs.

## How it works

Save this as `example.md`:

````markdown
```sh
git grep -e policedoc
git grep -e
```
````

Scan it:

```console
$ policedoc scan example.md
example.md:3:1
  git grep -e
  ERROR missing-flag-value: -e requires a value.

1 files, 2 examples: 1 errors, 0 warnings
Coverage: 2 partially checked, 0 uncheckable. Use --verbose for details.
```

The second command is missing a search pattern after `-e`.
The scan reports an error and exits with code `1`.
Neither search is executed.

**Install the CLIs used in your docs and put them on `PATH`.**
policedoc runs them for version and help requests, so use only trusted documents and binaries.

## Install

### Homebrew

```sh
brew install sorafujitani/tap/policedoc
```

Homebrew packages support Linux and macOS (x86-64 and ARM64).

If Homebrew requests trust, run `brew trust --formula sorafujitani/tap/policedoc` and retry.

### Go

```sh
go install github.com/sorafujitani/police-doc/cmd/policedoc@latest
```

Requires Go 1.27.1 or later.
Add `$(go env GOPATH)/bin` to `PATH`, or use your custom `GOBIN` directory.

### Nix

With flakes enabled:

```sh
nix profile install github:sorafujitani/police-doc#policedoc
```

Or run without installing:

```sh
nix run github:sorafujitani/police-doc -- scan README.md docs/
```

### Build from source

With Go 1.27.1 or later, run from the repository root:

```sh
go build -o policedoc ./cmd/policedoc
```

## Usage

Scan Markdown files or directories:

```sh
policedoc scan README.md docs/
```

Use warnings to fail a CI check, or get JSON output:

```sh
policedoc scan docs/ --fail-on warning
policedoc scan docs/ --format json
```

| Option | Purpose |
| --- | --- |
| `--fail-on error\|warning\|none` | When findings fail the scan; default: `error` |
| `--format text\|json` | Output format; default: `text` |
| `--verbose` | Show skipped arguments, CLI versions, and help details |
| `--refresh` | Recollect help after plugin or environment changes |
| `--cache-dir PATH` | Cache location; default: `.policedoc/cache` |

Help is cached automatically and updated when the CLI's version or executable metadata changes.

Use `policedoc --help` for command help.

## What gets checked

Commands in `sh`, `bash`, and `console` code blocks are supported, including multiline commands, pipes, `&&`, and `||`.
In transcripts marked with `$ ` prompts, command output is ignored.

Directory scans skip `.git`, `node_modules`, `vendor`, and symbolic links.

| Finding | Meaning |
| --- | --- |
| Error | A flag is missing a value required by help |
| Review warning | A flag or subcommand is not listed, or a value is outside a stated choice list |
| Cannot check | The executable or its help is unavailable |

A review warning is a reason to inspect the example, not proof that it is wrong.

**Help alone cannot prove a command works.**
Runtime behavior, variable expansions, arguments after `--`, required flags, and positional argument limits are not checked.
Shell builtins such as `cd` and `echo` are skipped.

### Reading results

Findings include the file, line, and command.

**Coverage** tells you how many examples were partly checked or could not be checked.
Use `--verbose` to see gaps.

| Exit code | Meaning |
| --- | --- |
| `0` | No findings reached the failure threshold; check warnings and coverage |
| `1` | Findings reached the failure threshold (errors by default) |
| `2` | Operational failure |

For detailed rules and limits, see [checking details](docs/checking.md).

## Development

```sh
go test -race ./...
go vet ./...
go build ./cmd/policedoc
```

See the [release guide](docs/releasing.md) for packaging and publishing.

Licensed under [MIT](LICENSE).
