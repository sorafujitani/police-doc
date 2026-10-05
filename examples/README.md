# Try policedoc

Scan examples for Go, Git, and gofmt to see diagnostics and exit codes.
The Markdown files under `cases/` are the scan inputs.

## Build and scan

Put Go 1.27.1 (including gofmt) and Git on `PATH`.
The expected output below uses Go 1.27.1 and Git 2.53.0.
Other versions may add collection warnings.
Run these commands from the repository root.

```sh
go build -o policedoc ./cmd/policedoc
./policedoc scan examples/cases
echo $?
```

Exit code `1` is expected. The final summary is:

```text
6 files, 7 examples: 2 errors, 2 warnings
Coverage: 6 partially checked, 1 uncheckable. Use --verbose for details.
```

Text shows errors and warnings, grouped by source command.
Six examples are partially checked; the gofmt example is uncheckable.
Help alone cannot provide complete validation.

Use `--verbose` to include INFO messages, target versions, and help evidence.
INFO describes incomplete checking, not a successful validation.

```sh
./policedoc scan examples/cases --verbose
```

## Inspect individual cases

| File | CLI | What it demonstrates | Default exit code |
| --- | --- | --- | --- |
| [valid.md](cases/valid.md) | Go | No errors or warnings for a supplied flag value | `0` |
| [missing-value.md](cases/missing-value.md) | Go | A missing flag value produces an error | `1` |
| [unknown-flag.md](cases/unknown-flag.md) | Go | A flag absent from help produces a review warning | `0` |
| [dynamic.md](cases/dynamic.md) | Go | A variable value is not expanded or checked | `0` |
| [git.md](cases/git.md) | Git | Supplied and missing search patterns | `1` |
| [gofmt.md](cases/gofmt.md) | gofmt | Missing version support makes it uncheckable | `0` |

Run one file to inspect an individual case.

```sh
./policedoc scan examples/cases/missing-value.md
```

Treat warnings as failures with `--fail-on warning`.
The following command returns exit code `1`:

```sh
./policedoc scan examples/cases/unknown-flag.md --fail-on warning
echo $?
```

Use JSON to inspect the same findings and summary.

```sh
./policedoc scan examples/cases --format json
```

## Interpreting results

Exit code `0` does not guarantee that an example works or that all arguments were checked.
By default, warnings alone do not fail the scan. Check severity and coverage.

`gofmt` does not support `--version`, so its example is uncheckable rather than invalid.
Use `--fail-on warning` to make that warning fail the scan too.

Scanning starts installed executables for version/help requests.
Successful results are cached.
The commands inside `cases/` are not executed; no `policedoc-example` binary is created.

For other options and limits, see the [project README](../README.md).
