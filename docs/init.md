# Checking specification

policedoc checks CLI examples in Markdown against help from installed executables.
No configuration or manually maintained JSON specification is required.
It does not execute document commands and only checks what the collected help supports.

## Input and collection

Input is a Markdown file or directory.
Shell fences labeled `sh`, `bash`, and `console` are supported.
Continuation lines, pipes, `&&`, and `||` are parsed while preserving source locations.

The scanner locates executables used by the examples and collects their versions and help.
It follows advertised subcommands to a depth of three, with at most 100 help calls per executable.
Each call is limited to ten seconds and 1 MiB of combined stdout and stderr.
For Go, fixed requests collect root command names and `go build` flags.
Details of other Go subcommands are not collected.

Supported static `sudo` and `npx` forms resolve to the inner CLI.
Wrappers are not executed and packages are not installed.

## Caching

Collected results are cached automatically.
Help is recollected when the executable's version or metadata changes.
Use `--refresh` after changes such as plugin updates.
Every run checks the document examples again.

The installed CLI's version is checked even when reusing cached help.
A missing executable produces a warning and uncheckable results, even if cached help exists.
Outdated cache formats and corrupt entries are recollected.

## Checks and limits

| Situation | Result |
| --- | --- |
| A flag explicitly requiring a value has none | Confirmed error |
| A flag is absent from help | Review warning |
| The CLI's help cannot be collected | Warning and uncheckable results |
| A subcommand's detailed help is unavailable | That command is uncheckable |

Help is not a complete specification.
Hidden flags, required flags, and positional argument bounds are not inferred.
Neither are deprecation, removal, or replacements.
The scanner does not verify runtime results, side effects, or whether a CLI is the latest version.

Only the statically interpretable prefix is checked.
Unknown flag arity, positional arguments, `--`, and dynamic words stop further argument checking.
This avoids mistaking arguments forwarded to another program for the launcher's flags.
Short flag groups, aliases, and inherited flags are not guessed.

## Results and safety

Text reports show errors and warnings grouped by source command, with source locations and a coverage summary.
`--verbose` adds INFO messages, target CLI versions, diagnostic certainty, and collection details.
INFO describes incomplete checking, not a successful validation.
Coverage distinguishes partial checks from uncheckable examples; full validation is never claimed.
JSON always includes all diagnostics and collection details, using schema version `2`.

Exit codes are `0` below the failure threshold, `1` at or above it, and `2` for operational failures.
The default threshold is error.
Use `--fail-on warning` to fail on warnings too.
`--fail-on none` disables only finding-based failures.
Exit code `0` does not imply that all arguments were checked.

Normal scans start executables to obtain version and help information.
Use only trusted documents and executables.
Help collection is not a sandbox and cannot guarantee freedom from side effects.
See the [README](../README.md) for usage.
