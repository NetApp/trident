#!/bin/bash

#Usage:
# -n <namespace>      Specifies the namespace for the Trident deployment; defaults to the current namespace.
# -t <trident_image>  Specifies the new image for the "trident-main" container in the Trident deployment.
# -e <etcd_image>     Specifies the new image for the "etcd" container in the Trident deployment.
# -h                  Prints this usage guide.
#
#Example:
#  ./update_trident.sh -n trident -t netapp/trident:17.04.1		Updates the Trident deployment in namespace "trident" to use image "netapp/trident:17.04.1".

usage() {
	printf "\nUsage:\n"
	printf " %-20s%s\n" "-n <namespace>" "Specifies the namespace for the Trident deployment; defaults to the current namespace."
	printf " %-20s%s\n" "-t <trident_image>" "Specifies the new image for the \"trident-main\" container in the Trident deployment."
	printf " %-20s%s\n" "-e <etcd_image>" "Specifies the new image for the \"etcd\" container in the Trident deployment."
	printf " %-20s%s\n" "-d <deployment>" "Specifies the name of the deployment; defaults to \"trident\"."
	printf " %-20s%s\n" "-h" "Prints this usage guide."
	printf "\nExample:\n"
	printf " %s\t\t%s\n\n" " ./update_trident.sh -n trident -t netapp/trident:17.04.1" "Updates the Trident deployment in namespace \"trident\" to use image \"netapp/trident:17.04.1\"."
}

die() {
	usage
	exit 1
}

get_namespace() {
	TMP=$(kubectl get serviceaccount default -o json | grep "namespace\":" | awk '{print $2}' | sed 's/,//g; s/"//g')
	echo $TMP
}

# Process arguments
TMP=`getopt -o n:t:e:d:h:: -- "$@"`
if [ $? -ne 0 ]; then
	die
fi
eval set -- "$TMP"
while true ; do
	case "$1" in
		-n)
			NAMESPACE=$2
			shift 2 ;;
		-t)
			TRIDENT_IMAGE=$2
			shift 2 ;;
		-e)
			ETCD_IMAGE=$2
			shift 2 ;;
		-d)
			DEPLOYMENT=$2
			shift 2 ;;
		-h)
			usage
			exit 0 ;;
		--) shift ; break ;;
		*)	die ;;
	esac
done

# Check for the requirements
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
command -v curl > /dev/null 2>&1 || \
	{ echo >&2 "$0 requires curl present in \$PATH."; exit 1; }
command -v kubectl > /dev/null || \
	{ echo >&2 "$0 requires kubectl present in \$PATH."; exit 1; }
if [ -z "$TRIDENT_IMAGE" ] && [ -z "$ETCD_IMAGE" ]; then
	echo >&2 "Requires to use -t, -e, or both options!"
	die
fi

# Check if the namespace exists already
if [ -z "$NAMESPACE" ]; then
	NAMESPACE=$(get_namespace)
	if [ -z "$NAMESPACE" ]; then
		echo >&2 "Failed to determine the current namespace!"; exit 1 
	fi
	echo "You are running in namespace $NAMESPACE."
	echo "Update will take place in namespace ${NAMESPACE}."
fi
TMP=$(kubectl get namespace $NAMESPACE 2>&1)
if [ "$?" -ne "0" ]; then
	exit 1
fi

# Set the deployment if not set
if [ -z "$DEPLOYMENT" ]; then
	DEPLOYMENT="trident"
fi

if [ -n "$TRIDENT_IMAGE" ]; then
	kubectl --namespace=$NAMESPACE set image deployment/$DEPLOYMENT trident-main=$TRIDENT_IMAGE
	if [ "$?" -ne "0" ]; then
		exit 1
	fi
fi

if [ -n "$ETCD_IMAGE" ]; then
	kubectl --namespace=$NAMESPACE set image deployment/$DEPLOYMENT etcd=$ETCD_IMAGE
	if [ "$?" -ne "0" ]; then
		exit 1
	fi
fi
