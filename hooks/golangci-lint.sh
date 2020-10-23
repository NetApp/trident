#!/bin/sh

# find staged files that end in .go
STAGED_GO_DIRS=$(git diff --cached --name-only | grep ".go$" | xargs dirname)
GO_PACKAGE_DIRS=($(echo "${STAGED_GO_DIRS}" | tr ' ' '\n' | sort -u | tr '\n' ' '))

# if nothing, don't lint. hook successful.
if [ "$STAGED_GO_DIRS" = "" ]; then
  exit 0
fi

PASS=true
for DIR in $GO_PACKAGE_DIRS
do
    # This is where we will be testing each of our files.
    golangci-lint run -E goimports $DIR
	if [ $? != 0 ]; then
		PASS=false
	fi
done

if ! $PASS; then
    printf "COMMIT FAILED\n"
    exit 1
else
    printf "COMMIT SUCCEEDED\n"
fi

exit 0
