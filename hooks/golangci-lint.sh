#!/bin/sh

STAGED_GO_FILES=$(git diff --cached --name-only | grep '\.go$' || true)

if [ -z "$STAGED_GO_FILES" ]; then
  exit 0
fi

GO_PACKAGE_DIRS=$(
  printf '%s\n' "$STAGED_GO_FILES" |
    while read -r file; do
      dirname "$file"
    done |
    sort -u
)

PASS=true
for DIR in $GO_PACKAGE_DIRS; do
  if [ ! -d "$DIR" ]; then
    continue
  fi
  if ! golangci-lint run "./$DIR"; then
    PASS=false
  fi
done

if ! $PASS; then
  printf "COMMIT FAILED\n"
  exit 1
fi

printf "COMMIT SUCCEEDED\n"
exit 0
