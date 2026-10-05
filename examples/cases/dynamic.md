# Dynamic argument

The output path is supplied through a variable.

```sh
go build -o "$OUTPUT"
```

The variable is not expanded. The scanner reports `dynamic-argument` for the unchecked value.
Expected: no errors or warnings, partial coverage, and exit code `0`.
Neither the variable's value nor runtime success is verified.
