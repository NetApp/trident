#!/bin/bash

#Usage:
# -n <namespace>      Specifies the namespace for the Trident deployment; defaults to the current namespace.
# -h                  Prints this usage guide.
#
#Example:
#  ./delete_trident.sh -n trident		Deletes artifacts of Trident from namespace "trident".

usage() {
	printf "\nUsage:\n"
	printf " %-20s%s\n" "-n <namespace>" "Specifies the namespace for the Trident deployment; defaults to the current namespace."
	printf " %-20s%s\n" "-h" "Prints this usage guide."
	printf "\nExample:\n"
	printf " %s\t\t%s\n\n" " ./delete_trident.sh -n trident" "Deletes artifacts of Trident from namespace \"trident\"."
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
TMP=`getopt -o n:h:: -- "$@"`
if [ $? -ne 0 ]; then
	die
fi
eval set -- "$TMP"
while true ; do
	case "$1" in
		-n)
			NAMESPACE=$2
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

# Determine the namespace
if [ -z $NAMESPACE ]; then
	NAMESPACE=$(get_namespace)
	if [ -z "$NAMESPACE" ]; then
		echo >&2 "Failed to determine the current namespace!"; exit 1
	fi
	echo "You are running in namespace $NAMESPACE."
	echo "Cleanup will take place in namespace ${NAMESPACE}."
fi
TMP=$(kubectl get namespace $NAMESPACE 2>&1)
if [ "$?" -ne "0" ]; then
	exit 1
fi

# Clean up Trident artifacts
kubectl --namespace=$NAMESPACE delete -f $DIR/setup/trident-deployment.yaml --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi
kubectl --namespace=$NAMESPACE delete -f $DIR/launcher-pod.yaml --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi
kubectl --namespace=$NAMESPACE delete pod trident-ephemeral --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi
kubectl --namespace=$NAMESPACE delete -f $DIR/trident-clusterrolebindings.yaml --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi
kubectl --namespace=$NAMESPACE delete -f $DIR/trident-clusterroles.yaml --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi
kubectl --namespace=$NAMESPACE delete -f $DIR/trident-serviceaccounts.yaml --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi
kubectl --namespace=$NAMESPACE delete configmap trident-launcher-config --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi

echo "Successfully deleted Trident deployment, service accounts, and cluster roles and role bindings."
echo "Successfully deleted Trident launcher pod and configuration."
echo "This script didn't delete namespace \"$NAMESPACE\" and PVC and PV \"trident\" in case they're going to be reused."
