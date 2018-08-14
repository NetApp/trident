#!/usr/bin/env bash

export DOCKER_PLUGIN_MODE=1

# process debug 
MY_DEBUG=${debug:-false}
case $(echo ${MY_DEBUG} | tr '[:upper:]' '[:lower:]') in
    true)
        DEBUG_SWITCH="--debug=true"
        ;;
    *)
        DEBUG_SWITCH="--debug=false"
        ;;
esac
export DEBUG_SWITCH

# process rest api server
MY_REST=${rest:-false}
case $(echo ${MY_REST} | tr '[:upper:]' '[:lower:]') in
    true)
        REST_SWITCH="--rest=true"
        ;; 
    *)  
        REST_SWITCH="--rest=false"
        ;;  
esac
export REST_SWITCH

# process config
MY_CONFIG=${config:-/etc/netappdvp/config.json}
basefile=$(basename ${MY_CONFIG})
MY_CONFIG="/etc/netappdvp/${basefile}"
export CONFIG_SWITCH="--config=${MY_CONFIG}"

echo Running: /netapp/trident ${REST_SWITCH} --address=0.0.0.0 --port=8000 ${DEBUG_SWITCH} ${CONFIG_SWITCH} "${@:1}"
/netapp/trident ${REST_SWITCH} --address=0.0.0.0 --port=8000 ${DEBUG_SWITCH} ${CONFIG_SWITCH} "${@:1}"

