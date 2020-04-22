#!/usr/bin/env bash

# process debug
MY_K8S=${K8S}
case $(echo ${MY_K8S}) in
    "")
        K8S_SWITCH=""
        ;;
     *)
        K8S_SWITCH="--k8s-api-server ${MY_K8S}"
        ;;
esac
export K8S_SWITCH

echo Running: /usr/local/bin/trident-operator ${K8S_SWITCH} "${@:1}"
/usr/local/bin/trident-operator ${K8S_SWITCH} "${@:1}"
