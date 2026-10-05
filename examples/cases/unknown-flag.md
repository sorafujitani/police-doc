# Flag absent from help

This flag name is absent from the collected help.

```sh
go build --policedoc-unknown-flag
```

Expected: one `unverified-flag` review warning, no errors, and exit code `0`.
Absence from help is not proof that the flag is invalid.
With `--fail-on warning`, the exit code is `1`.
