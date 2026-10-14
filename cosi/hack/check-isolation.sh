#!/usr/bin/env bash
#
# Guard the isolation of the COSI subtree.
#
# The vendoring strategy assumes cosi/ is self-contained: it can be re-synced
# wholesale against upstream, and removed or split out later, without surgery
# elsewhere. It also keeps controller-runtime, which nothing else in Trident
# uses, from spreading.
#
# That property decays silently. A single import added during some unrelated
# change turns "delete the directory" into a refactor, and nobody notices until
# it is expensive. This script fails the build instead.

set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

readonly COSI_PKG="github.com/netapp/trident/cosi"

# Files outside cosi/ permitted to import it. The frontend is wired up in main.go
# behind --enable_cosi; nothing else should ever need to know COSI exists.
readonly ALLOWED_IMPORTERS=(
    "main.go"
)

# Packages inside cosi/ that anything may import. These carry no behaviour and pull in neither
# controller-runtime nor the COSI Go modules, so importing them does not spread the dependency
# the rest of this script exists to contain. cosi/crd is embedded YAML the installer needs.
# cosi/provisioner is the Provisioner interface plus its plain-Go-struct types (imports only
# "context" and "errors"); frontend/objectprovisioner imports it to implement the interface.
readonly DATA_ONLY_PACKAGES=(
    "github.com/netapp/trident/cosi/crd"
    "github.com/netapp/trident/cosi/provisioner"
)

# Trident packages cosi/ is permitted to import. This is the seam the design
# defines: the orchestrator interface, the frontend plugin contract, and the
# persistent store for the bucket identity record. Adding to this list widens
# the seam, so do it deliberately rather than to make a build pass.
readonly ALLOWED_SEAM=(
    "github.com/netapp/trident/config"
    "github.com/netapp/trident/core"
    "github.com/netapp/trident/frontend"
    "github.com/netapp/trident/logging"
    "github.com/netapp/trident/persistent_store"
    "github.com/netapp/trident/utils"
)

cd "${REPO_ROOT}"

failures=0

fail() {
    echo "FAIL: $*" >&2
    failures=$(( failures + 1 ))
}

# 1. Nothing outside cosi/ may import cosi/, except the wiring point.
echo "== who imports cosi/ =="
importers="$(grep -rlE "\"${COSI_PKG}" --include='*.go' . 2>/dev/null | sed 's|^\./||' | grep -v '^cosi/' || true)"
if [[ -z "${importers}" ]]; then
    echo "  nothing outside cosi/ imports it"
else
    while read -r file; do
        [[ -z "${file}" ]] && continue

        # Which cosi packages does this file actually import? A file that only reaches the
        # data-only packages is fine anywhere.
        behavioural=""
        while read -r pkg; do
            [[ -z "${pkg}" ]] && continue
            is_data=false
            for data_pkg in "${DATA_ONLY_PACKAGES[@]}"; do
                [[ "${pkg}" == "${data_pkg}" ]] && is_data=true && break
            done
            [[ "${is_data}" == false ]] && behavioural="${behavioural} ${pkg}"
        done <<< "$(grep -oE "\"${COSI_PKG}[^\"]*\"" "${file}" | tr -d '"' | sort -u)"

        if [[ -z "${behavioural// /}" ]]; then
            echo "  ok (data only): ${file}"
            continue
        fi

        allowed=false
        for permitted in "${ALLOWED_IMPORTERS[@]}"; do
            [[ "${file}" == "${permitted}" ]] && allowed=true && break
        done
        if [[ "${allowed}" == true ]]; then
            echo "  ok (allowed): ${file}"
        else
            fail "${file} imports${behavioural}; only ${ALLOWED_IMPORTERS[*]} may import cosi/ behaviour"
        fi
    done <<< "${importers}"
fi
echo

# 2. cosi/ may only reach into Trident through the defined seam.
echo "== what cosi/ imports from Trident =="
reaches="$(grep -rhoE "\"github\.com/netapp/trident/[^\"]+\"" cosi --include='*.go' 2>/dev/null \
    | tr -d '"' | grep -v "^${COSI_PKG}" | sort -u || true)"
if [[ -z "${reaches}" ]]; then
    echo "  cosi/ imports no other Trident package"
else
    while read -r pkg; do
        [[ -z "${pkg}" ]] && continue
        allowed=false
        for permitted in "${ALLOWED_SEAM[@]}"; do
            if [[ "${pkg}" == "${permitted}" || "${pkg}" == "${permitted}/"* ]]; then
                allowed=true
                break
            fi
        done
        if [[ "${allowed}" == true ]]; then
            echo "  ok (seam): ${pkg}"
        else
            fail "cosi/ imports ${pkg}, which is outside the agreed seam"
        fi
    done <<< "${reaches}"
fi
echo

# 3. controller-runtime arrives with the COSI API types and is used nowhere else
#    in Trident. Keep it that way, so the dependency stays removable with cosi/.
echo "== controller-runtime containment =="
leaked="$(grep -rlE '"sigs\.k8s\.io/controller-runtime' --include='*.go' . 2>/dev/null \
    | sed 's|^\./||' | grep -v '^cosi/' || true)"
if [[ -z "${leaked}" ]]; then
    echo "  controller-runtime is confined to cosi/"
else
    while read -r file; do
        [[ -z "${file}" ]] && continue
        fail "${file} imports controller-runtime outside cosi/"
    done <<< "${leaked}"
fi
echo

if (( failures > 0 )); then
    echo "check-isolation: ${failures} violation(s)" >&2
    exit 1
fi

echo "check-isolation: ok"
