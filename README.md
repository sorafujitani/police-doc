# policedoc

Check CLI examples in Markdown using help from installed executables.
No configuration or manually maintained specification is needed.

`scan` never executes document examples. It does start executables for version and help requests.
Use only trusted documents and binaries.

## Build and run

Use Go 1.27.1 to build the single binary:

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
It ignores terminal output in prompt-based console blocks.

- A flag with an explicit value requirement in help produces an error when its value is missing.
- A flag absent from collected help produces a review warning, not a claim that it is invalid.
- Known subcommand names select their collected help.
- Unknown value rules, positional arguments, `--`, and dynamic words stop further argument checking.

All help-based checks are partial. Help cannot establish complete grammar or hidden entries.
Required flags, positional bounds, deprecation, removal, and replacements are not inferred.
Short flag groups, aliases, inheritance, and argument forwarding are not guessed.
No examples, substitutions, or variable expansions are executed; documents are never rewritten.

Text reports show errors and warnings, grouped by source command, followed by counts and coverage.
Use `--verbose` for INFO messages, target CLI/version/OS, evidence, and diagnostic certainty.
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
It reuses help when the version and executable metadata are unchanged.
Every run still checks all input examples. Use `--refresh` after plugin or environment changes.

Collection tries `-h`, then `--help`, and explores advertised subcommands.
Limits are three levels and 100 help calls per executable, including failed calls.
Each call allows ten seconds and 1 MiB of combined output, with no interactive input.
For Go, fixed `version`, `help`, and `help build` calls are used instead.
Only Go's root command list and build flags are collected; toolchain downloads are disabled.

Common static `sudo` and bare-executable `npx` forms resolve to the inner CLI.
Neither wrapper runs, and no packages are installed.
For `npx`, lookup starts in ancestor `node_modules/.bin` directories, then checks `PATH`.

Missing executables or failed collection produce warnings and uncheckable results.
A failed child-help request leaves a placeholder and a warning.
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

Optional integration tests use installed Go, Git, Node.js, Python, uv, and ripgrep.

```sh
POLICEDOC_REAL_CLI_TESTS=1 go test ./internal/app -run TestRealCLIHelpAndScan -v
```

See the [checking specification](docs/init.md). Licensed under [MIT](LICENSE).
