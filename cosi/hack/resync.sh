#!/usr/bin/env bash
#
# Diff the vendored COSI source against upstream.
#
# Without arguments, checks that every file in cosi/upstream.manifest still
# matches upstream at the pinned commit. This is the guard: it fails if anyone
# has edited vendored code without recording the divergence.
#
# Given a commit, additionally reports what upstream changed between the pinned
# commit and that one. Run this before bumping the pin, to see what is coming.
#
# Import blocks are excluded from every comparison. Rewriting them from
# upstream's internal/ layout to Trident package paths is the one divergence
# that applies to all files by design.
#
# A file whose manifest line ends in "diverged" is one we edit deliberately. Its
# diff is still printed on every run, because that diff is what a reviewer needs
# when moving the pin, but it does not fail the check. A file marked diverged
# that in fact matches upstream fails, so the marks cannot go stale and start
# hiding real drift.

set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly COSI_DIR="$(dirname "${SCRIPT_DIR}")"
readonly REPO_ROOT="$(dirname "${COSI_DIR}")"
readonly MANIFEST="${COSI_DIR}/upstream.manifest"

usage() {
    cat >&2 <<'EOF'
Usage: cosi/hack/resync.sh [upstream-commit]

  (no argument)      Verify vendored files still match the pinned upstream commit.
  <upstream-commit>  Also report what upstream changed since the pinned commit.

Exits non-zero if vendored files have drifted from the pinned commit.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage
    exit 0
fi

command -v curl >/dev/null || { echo "resync: curl is required" >&2; exit 1; }
[[ -f "${MANIFEST}" ]] || { echo "resync: manifest not found: ${MANIFEST}" >&2; exit 1; }

# Strip the import block so comparisons see only real code. Handles both the
# parenthesised block form and single-line imports.
strip_imports() {
    awk '
        /^import \(/     { in_block = 1; next }
        in_block && /^\)/ { in_block = 0; next }
        in_block          { next }
        /^import "/       { next }
                          { print }
    ' "$1"
}

# Fetch a file from upstream at a given commit. Returns 1 if it does not exist.
fetch() {
    local commit="$1" path="$2" dest="$3"
    curl -sfL "https://raw.githubusercontent.com/${REPO}/${commit}/${path}" -o "${dest}"
}

REPO=""
PINNED=""
declare -a UPSTREAM_PATHS=() LOCAL_PATHS=() EXPECT_DIVERGED=()

while read -r first second third _rest; do
    [[ -z "${first}" || "${first}" == \#* ]] && continue
    case "${first}" in
        repo)   REPO="${second}" ;;
        commit) PINNED="${second}" ;;
        *)
            UPSTREAM_PATHS+=("${first}")
            LOCAL_PATHS+=("${second}")
            # A third column of "diverged" marks a file we edit on purpose. The
            # diff is still printed, because a re-sync reviewer needs to see it.
            if [[ "${third}" == "diverged" ]]; then
                EXPECT_DIVERGED+=("yes")
            else
                EXPECT_DIVERGED+=("no")
            fi
            ;;
    esac
done < "${MANIFEST}"

[[ -n "${REPO}"   ]] || { echo "resync: manifest has no 'repo' line" >&2; exit 1; }
[[ -n "${PINNED}" ]] || { echo "resync: manifest has no 'commit' line" >&2; exit 1; }

readonly TARGET="${1:-}"
readonly FILE_COUNT="${#UPSTREAM_PATHS[@]}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

echo "repo:   ${REPO}"
echo "pinned: ${PINNED}"
echo "files:  ${FILE_COUNT}"
echo

# Pass one: has anything drifted locally from the pinned commit?
echo "== local tree vs pinned upstream =="
drifted=0
missing=0
expected=0
for (( i = 0; i < FILE_COUNT; i++ )); do
    upstream_path="${UPSTREAM_PATHS[i]}"
    local_path="${REPO_ROOT}/${LOCAL_PATHS[i]}"

    if [[ ! -f "${local_path}" ]]; then
        echo "MISSING LOCALLY  ${LOCAL_PATHS[i]}"
        missing=$(( missing + 1 ))
        continue
    fi

    if ! fetch "${PINNED}" "${upstream_path}" "${tmpdir}/pinned"; then
        echo "MISSING UPSTREAM ${upstream_path}"
        missing=$(( missing + 1 ))
        continue
    fi

    strip_imports "${tmpdir}/pinned" > "${tmpdir}/a"
    strip_imports "${local_path}"    > "${tmpdir}/b"

    if ! diff -q "${tmpdir}/a" "${tmpdir}/b" >/dev/null; then
        if [[ "${EXPECT_DIVERGED[i]}" == "yes" ]]; then
            echo "DIVERGED (expected)  ${LOCAL_PATHS[i]}"
            diff -u "${tmpdir}/a" "${tmpdir}/b" | sed 's/^/    /' || true
            expected=$(( expected + 1 ))
        else
            echo "DIVERGED         ${LOCAL_PATHS[i]}"
            diff -u "${tmpdir}/a" "${tmpdir}/b" | sed 's/^/    /' || true
            drifted=$(( drifted + 1 ))
        fi
    elif [[ "${EXPECT_DIVERGED[i]}" == "yes" ]]; then
        # The marker outlived the edit. Left alone it would hide a real drift
        # later, so make removing it someone's problem now.
        echo "STALE MARK       ${LOCAL_PATHS[i]} matches upstream; drop its 'diverged' mark"
        drifted=$(( drifted + 1 ))
    fi
done

if (( drifted == 0 && missing == 0 )); then
    echo "all ${FILE_COUNT} files match the pinned commit (imports excluded, ${expected} expected divergence(s))"
fi
echo

# Pass two: what has upstream done since the pin?
if [[ -n "${TARGET}" ]]; then
    echo "== upstream ${PINNED:0:12} -> ${TARGET:0:12} =="
    changed=0
    for (( i = 0; i < FILE_COUNT; i++ )); do
        upstream_path="${UPSTREAM_PATHS[i]}"

        fetch "${PINNED}" "${upstream_path}" "${tmpdir}/old" || true
        if ! fetch "${TARGET}" "${upstream_path}" "${tmpdir}/new"; then
            echo "DELETED UPSTREAM ${upstream_path}"
            changed=$(( changed + 1 ))
            continue
        fi

        if ! diff -q "${tmpdir}/old" "${tmpdir}/new" >/dev/null; then
            echo "CHANGED          ${upstream_path}"
            diff -u "${tmpdir}/old" "${tmpdir}/new" | sed 's/^/    /' || true
            changed=$(( changed + 1 ))
        fi
    done
    if (( changed == 0 )); then
        echo "upstream is unchanged for every vendored file"
    else
        echo
        echo "${changed} file(s) changed upstream. Review before moving the pin,"
        echo "then update commit in cosi/upstream.manifest and go.mod together."
    fi
    echo
fi

if (( drifted > 0 || missing > 0 )); then
    echo "resync: ${drifted} diverged, ${missing} missing" >&2
    echo "If a divergence is deliberate, record it before suppressing it here." >&2
    exit 1
fi
