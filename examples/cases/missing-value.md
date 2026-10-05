# Missing flag value

The output path required by `-o` is missing.

```sh
go build -o
```

Expected: one `missing-flag-value` error, no warnings, and exit code `1`.
