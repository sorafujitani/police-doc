# No errors or warnings

The `-o` flag has an output path.

```sh
go build -o policedoc-example ./cmd/policedoc
```

Expected: no errors, no warnings, and exit code `0`.
Coverage is still partial because only help is used for checking.
