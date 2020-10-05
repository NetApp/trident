#!/bin/sh

# find staged files that end in .go
STAGED_GO_FILES=$(git diff --cached --name-only | grep ".go$")

# if nothing, don't lint. hook successful.
if [ "$STAGED_GO_FILES" = "" ]; then
  exit 0
fi

PASS=true
for FILE in $STAGED_GO_FILES
do
    # This is where we will be testing each of our files.
    golangci-lint run -E goimports --fix $FILE
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
