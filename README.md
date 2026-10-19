# policedoc

Check CLI argument syntax in Markdown against installed help or versioned reference specifications.
Report missing values, unsupported options, and gaps that prevent a complete check.

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

By default, put the CLIs used in your docs on `PATH`.
policedoc starts them for version and help requests, so use only trusted documents and binaries.
For checks without starting executables, use [reference specifications](docs/references.md).

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
| `--spec PATH` | Use a reference file or directory; repeatable; disables live collection |
| `--allow-live-help` | Allow collection for CLIs absent from supplied references |
| `--export-spec FILE.json` | Export available rules as a portable reference starting point |
| `--require-complete` | Fail if any example lacks a complete check against supplied rules |

Help is cached automatically and updated when the CLI's version or executable metadata changes.

To require every extracted example to be checked against declared argument rules:

```sh
policedoc scan docs/ --spec specs/ --require-complete
```

This needs maintained reference rules. Exporting help alone does not supply missing argument grammar.
See [reference specifications](docs/references.md) for the format and an offline DNF5 example.

Use `policedoc --help` for command help.

## What gets checked

Commands in `sh`, `bash`, `shell`, and `console` code blocks are supported, including multiline commands, pipes, `&&`, and `||`.
Transcripts marked with `$ ` prompts are also recognized in unlabeled code blocks; command output is ignored.

Directory scans skip `.git`, `node_modules`, `vendor`, and symbolic links.

| Finding | Meaning |
| --- | --- |
| Error | A required value is missing, or the command violates supplied reference rules |
| Review warning | A flag or subcommand is not listed, or a value is outside a stated choice list |
| Cannot check | Required metadata, argument rules, or static shell structure are unavailable |

A review warning is a reason to inspect the example, not proof that it is wrong.

**A syntax check does not prove a command works.**
Unambiguous help synopses can establish positional counts and documented modes.
Reference rules can supply missing counts, flag requirements, argument ordering,
and explicitly declared command forwarding. Ambiguous rules remain incomplete.
Runtime behavior, file/package existence, and undeclared value semantics are outside the check.
Substitutions and variable expansions are never executed. Unresolved shell input remains a gap.
Bash builtins use Bash’s own help or shell-qualified references, never an external namesake.
Loop, conditional, subshell, and function bodies are inspected without executing them.

### Reading results

Findings include the file, line, and command.

**Coverage** distinguishes `checked` against declared argument rules, `partial`, and `uncheckable`.
A `checked` result may still contain errors; it is not proof of runtime correctness or reference accuracy.
Use `--require-complete` to reject gaps, including a scan with no examples.
Incomplete JSON results include `requirements`: the missing metadata, argument rules,
concrete inputs, or document context needed to check them. These are not waivers.
Use `--verbose` for target versions and sources.

| Exit code | Meaning |
| --- | --- |
| `0` | No findings reached the failure threshold; check warnings and coverage |
| `1` | Findings reached the threshold, or `--require-complete` found incomplete coverage |
| `2` | Operational failure |

For detailed rules and limits, see [checking details](docs/checking.md).

## Development

```sh
go test -race ./...
go vet ./...
go build ./cmd/policedoc
```

See the [release guide](docs/releasing.md) for packaging and publishing.
The [OSS compatibility check](docs/oss-compatibility.md) records real-document coverage and remaining gaps.

Licensed under [MIT](LICENSE).
