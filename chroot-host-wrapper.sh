#!/usr/bin/env bash

ME=`basename "$0"`
#echo "I am ${ME}"

DIR="/host"
if [ ! -d "${DIR}" ]; then
    echo "Could not find docker engine host's filesystem at expected location: ${DIR}"
    exit 1
fi

#echo chroot /host /usr/bin/env -i PATH="/sbin:/bin:/usr/bin" ${ME} "${@:1}"
exec chroot /host /usr/bin/env -i PATH="/sbin:/bin:/usr/bin" ${ME} "${@:1}"
