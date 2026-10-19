# Checking specification

policedoc checks Markdown CLI examples against installed help or explicitly supplied reference rules.
It never executes document commands, wrappers, substitutions, or package installations.

## Inputs and metadata

Input is a Markdown file or directory.
Shell fences labeled `sh`, `bash`, `shell`, and `console` are supported.
A leading `$ ` or `❯ ` can also identify a transcript in an unlabeled fence.
Pipes, continuation lines, `&&`, and `||` preserve each command's source location.
Ambiguous terminal output and unsupported shell constructs remain checking gaps.
Bash builtins use shell-qualified references or Bash’s own builtin help, with startup files and imported functions disabled.
Compound bodies are inspected statically. Document functions and aliases do not select unrelated installed executables.

| Source | Selection and limits |
| --- | --- |
| Installed help | Default; discovers trusted executables and their versions, then collects root and advertised child help |
| `--spec` | Uses selected target versions from reference files; does not start executables |
| `--spec --allow-live-help` | Uses references first; permits collection only for CLIs absent from them |
| `--export-spec` | Writes available rules as a portable starting point; does not infer missing grammar |

References never silently merge with a different installed version.
Duplicate target selections and invalid reference rules are operational errors.
The [reference format](references.md) defines its bounds and provenance requirements.

Live collection uses only version/help protocols and advertised command paths, not document flag values or operands.
Explicit argv forwarding may select a child executable for its own metadata requests, under the same source policy.
Supported `sudo` and `npx` prefixes are resolved without running wrappers.
For local `npx` packages, executable lookup checks ancestor `node_modules/.bin` directories before `PATH`.

Help requests have bounded depth, count, duration, and output size; see [collection details](checking.md#collection-and-caching).
Only commands reached by examples need detailed help.
Failures do not imply that a CLI example is invalid.

## Cache behavior

Successful live help is cached by executable identity, version, and executable metadata.
New command paths extend the cache; changed executables or invalid caches trigger recollection.
Failures are retried on later scans.
`--refresh` forces collection after plugin or environment changes.
An installed executable is still required for this cache; portable references are a separate input format.

## Checking scope

Help can establish required option values and suggest suspicious flags or commands.
Its omissions do not prove that an option was removed or is invalid.
Unambiguous help synopses also supply positional counts and documented modes.
Unknown ordering, mixed flag/operand dependencies, and ambiguous alternatives stay incomplete.
A mismatch with inferred usage remains a review finding, not proof of invalid CLI syntax.

Reference rules can additionally establish positional counts, flag requirements and exclusions,
option ordering, and explicitly declared argv forwarding.
An incomplete reference still leaves gaps.
Unresolved expansions and nested commands are not treated as successful validation.

`checked` means the declared argument rules were applied without an unresolved portion.
It is not a statement about runtime effects, resource existence, embedded-language semantics,
or the accuracy and completeness of a supplied contract.
A checked example may contain confirmed violations of that contract.

## Reports and exit codes

JSON report schema `3` includes target versions and OS values, source evidence,
checked/partial/uncheckable counts, and `coverage_complete`.
Each incomplete result includes `requirements` for missing specifications, grammar, inputs, or document context.
Requirements do not count as successful checks or suppress findings.
A scan with no extracted examples is not complete.
Text reports show errors, warnings, and coverage. `--verbose` adds all gaps and evidence.
`--require-complete` also makes gaps visible without verbose output.

| Exit code | Meaning |
| --- | --- |
| `0` | No finding reaches the selected threshold, and any requested coverage requirement is satisfied |
| `1` | Findings reach the threshold, or `--require-complete` rejects incomplete coverage |
| `2` | Operational failure |

The default finding threshold is `error`.
`--fail-on warning` also fails on warnings; `--fail-on none` disables only finding-based failures.
It cannot bypass `--require-complete`.

Live help is not a sandbox and may have side effects inside the selected executable.
Use trusted documents and binaries, or reference-only mode.
Documents are not rewritten.
