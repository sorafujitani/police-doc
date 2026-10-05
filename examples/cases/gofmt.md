# CLI without a version flag

This command would display formatting differences without rewriting the file.

```sh
gofmt -d cmd/policedoc/main.go
```

Go's `gofmt` does not support `--version`.
policedoc cannot determine its version, so this example is uncheckable.
The command itself is not executed.

Expected: one `help-unavailable` warning, no errors, and exit code `0`.
With `--fail-on warning`, the exit code is `1`.
This demonstrates a collection limitation, not an invalid gofmt command.
