# Git flag values

## Pattern supplied

The `-e` flag has its required pattern.

```sh
git grep -e policedoc
```

The scanner checks the flag value without running the search.

## Missing pattern

The same flag has no value.

```sh
git grep -e
```

Expected: a `missing-flag-value` error for the second example and exit code `1`.
Both examples have partial coverage.
