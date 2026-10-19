# Reference specifications

Use a reference when the target CLI is unavailable, runs on another OS, or needs argument rules that help cannot establish.
A reference is a JSON file containing a target version, sources, and declared syntax rules.

```sh
policedoc scan docs/ --spec specs/ --require-complete
```

`--spec` accepts a file or a directory of `*.json` files and can be repeated.
Select one version per target identity; conflicting selections are rejected.
Bash builtins use `"shell": "bash"` as well as `tool`. They are separate from external executables with the same name.
A reference describes the selected target, not the locally installed version.

## Execution boundary

With `--spec`, no executable is started by default, including for forwarded commands.
Missing references produce `missing-reference` diagnostics.
Use `--allow-live-help` only to permit metadata collection for executables absent from the references:

```sh
policedoc scan docs/ --spec specs/ --allow-live-help --require-complete
```

A supplied reference is never silently combined with another installed version.
Sources are recorded as evidence; scanning does not download or execute them.
Documents, wrappers, substitutions, and package-install commands are never executed.
Live help collection is not a sandbox, so use trusted executables.

## Start from collected help

Run this where the required CLIs are installed:

```sh
policedoc scan docs/ --export-spec specs/collected.json
```

The output contains successfully collected specifications and any supplied references.
Export overwrites its output file. Use a new path to preserve manually maintained rules.
Missing executables are not filled in, and unvisited command details remain absent.

**Exported help preserves what is known; it does not make unknown rules complete.**
Simple synopses can supply positional forms and documented modes, marked `derived_forms: true`.
A mismatch with those forms still needs review because help can omit modes.
Review version-pinned manuals or CLI source before declaring additional rules.
Do not use unrestricted argument counts or `closed: true` merely to silence diagnostics.

## File format

This illustrative contract describes `acme copy`: exactly two operands and an optional output flag.
Its source label stands for a project-owned contract, not a verified public CLI.

```json
{
  "schema_version": 1,
  "tools": [
    {
      "tool": "acme",
      "version": "1.0",
      "sources": [
        {"kind": "reference", "reference": "Project-owned acme command contract, revision 1"}
      ],
      "root": {
        "subcommand_first": true,
        "commands": [
          {
            "name": "copy",
            "aliases": ["cp"],
            "closed": true,
            "flags_after_positionals": true,
            "forms": [{"min_args": 2, "max_args": 2}],
            "flags": [
              {"name": "--output", "aliases": ["-o"], "value": "required"}
            ]
          }
        ]
      }
    }
  ]
}
```

`tool` matches the executable name after supported wrapper resolution.
`version` identifies the selected release or source revision; empty and `latest` are rejected.
Optional `os` identifies the target OS independently of the scanner's host.
Sources use `kind` (`reference`, `manual`, or `help`), `reference`, and optional `detail`.
A command inherits its parent's sources unless it supplies its own.
The loader rejects unknown fields, conflicting names or aliases, invalid bounds, and contradictory flag requirements.
Each file must be a regular file of at most 8 MiB; command nesting is limited to 64 levels.

### Argument forms

`forms` lists alternative combinations of positional counts and flags.
Omitting it leaves positional grammar unknown, even when an example has no operands.

| Field | Meaning |
| --- | --- |
| `min_args` | Minimum positional count |
| `max_args` | Maximum count; omission explicitly allows an unlimited count |
| `required_flags` | Flags that must occur in this form |
| `forbidden_flags` | Flags that must not occur in this form |

Counts exclude option values and selected subcommand names.
Required and forbidden flags recognize their declared aliases.
Parent requirements are checked before entering a child command.
Parent flags apply before the subcommand; declare flags on the child when they also apply there.

`flags_after_positionals: true` permits options after operands.
Otherwise, options must precede operands unless `stop_flags_after_operand: true` says that every remaining word is an operand.
The two settings are mutually exclusive.
Collected help uses `flag_order_unknown: true` when the synopsis does not establish ordering.
That is a gap if an option follows an operand, not a guessed ordering error.
`--` ends option parsing unless parsing has already stopped, in which case it is another operand.
`short_flag_clusters: true` permits groups such as `-abc` when all component boundaries are known.
`literal_arguments: true` treats every word, including `--` and dash-prefixed words, as an operand.
It cannot coexist with flags or subcommands; use it only for a documented argument language that accepts those words, such as Bash `echo`.

### Flag values

| `value` | Rule |
| --- | --- |
| `none` | No separate value; attached values still need review in an open specification |
| `required` | Consume one following word, or an attached value |
| `optional-attached` | An optional value must be attached; the next word is not consumed |
| `optional-separated` | May also consume a following static word that does not begin with `-` |
| `optional` or `unknown` | Insufficient attachment or value information; leaves a checking gap |

`value_count` declares multiple separate required values.
`choices` declares known permitted values. Dynamic choices remain unchecked.
A simple quoted variable can supply one unrestricted string argument without being evaluated.
Unquoted expansions, quoted `$@`, complex parameter expansion, and embedded commands can still leave gaps.
Commands inside argument, assignment, or redirection substitutions are not treated as validated outer arguments.

By default, an unlisted option, command, or value needs review.
Use `closed: true` only when the option catalog, subcommand catalog, and choice lists are exhaustive for that command.
Their violations then become confirmed errors **against that contract**.

### Forwarded commands

A flag can declare that its value starts another command's argv:

```json
{"name": "--exec", "value": "required", "forward": true, "terminator": ";"}
```

The remaining words belong to the child until the literal terminator or the end of input.
Omit `terminator` to forward the entire tail.
This models argv forwarding, not a shell program passed as a string.
The child is checked using the same reference/live-help policy, with the same wrapper safeguards.
Its gaps and findings become part of the parent result. Nested forwarding is limited to eight levels.
A dynamic word that might be the terminator leaves the boundary unchecked.

## What completion means

Report schema `3` includes:

- `summary.checked`, `partial`, and `uncheckable` counts;
- `coverage_complete`, true only when there is at least one result and every result is `checked`;
- each result's target version, OS, shell, `spec_source`, and source `evidence` when available;
- `requirements` on every incomplete result, naming the evidence or input needed for a new check.

`checked` means the supplied argument rules were applied without unresolved portions.
It does **not** mean that the example is error-free, that the reference is accurate,
or that runtime behavior, embedded languages, package identities, or external resources were verified.
The reference author owns rules not established by the collector.

`--require-complete` exits `1` on partial or uncheckable results, or on a scan with no examples.
It also shows the gaps in text output without requiring `--verbose`.
Finding thresholds are separate: `--fail-on none` does not disable coverage failures.

## Pinned DNF5 example

[examples/specs/dnf5.json](../examples/specs/dnf5.json) models a small DNF5 subset selected as `dnf`:
`install`, `upgrade` (including `update`), and the `config-manager addrepo --from-repofile` form.
It is not a complete DNF catalog and does not apply to DNF4.
Unlisted modes and options remain gaps, not invented facts.

The rules come from these pinned upstream manuals and alias definitions:

- https://raw.githubusercontent.com/rpm-software-management/dnf5/dc7b57ceaffbb9a73ccfd62ba92d51221dd6d8a4/doc/commands/install.8.rst
- https://raw.githubusercontent.com/rpm-software-management/dnf5/dc7b57ceaffbb9a73ccfd62ba92d51221dd6d8a4/doc/commands/upgrade.8.rst
- https://raw.githubusercontent.com/rpm-software-management/dnf5/dc7b57ceaffbb9a73ccfd62ba92d51221dd6d8a4/dnf5/config/usr/share/dnf5/aliases.d/compatibility.conf
- https://raw.githubusercontent.com/rpm-software-management/dnf5/dc7b57ceaffbb9a73ccfd62ba92d51221dd6d8a4/doc/dnf5_plugins/config-manager.8.rst

The offline regression checks unmodified ripgrep, fd, and GitHub CLI installation examples.
Negative controls remove required arguments, add extra arguments, omit flag values, and introduce unlisted flags.
No DNF executable or package installation is used by those tests.
