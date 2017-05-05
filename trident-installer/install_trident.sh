#!/bin/bash

#$ ./install_trident.sh -h
#Usage:
# -n <namespace>:     Specifies the namespace for the Trident deployment; defaults to the current namespace.
# -h:                 Prints this usage guide.
#
#Example:
#  ./install_trident.sh -n trident		Installs the Trident deployment in namespace "trident".

usage() {
	printf "\nUsage:\n"
	printf " %-20s%s\n" "-n <namespace>" "Specifies the namespace for the Trident deployment; defaults to the current namespace."
	printf " %-20s%s\n" "-h" "Prints this usage guide."
	printf "\nExample:\n"
	printf " %s\t\t%s\n\n" " ./install_trident.sh -n trident" "Installs the Trident deployment in namespace \"trident\"."
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
if [ ! -e $DIR/setup/backend.json ]; then
	>&2 echo "${DIR}/setup must contain backend definition in 'backend.json'."
	exit 1
fi

# Determine the namespace
if [ -z "$NAMESPACE" ]; then
	NAMESPACE=$(get_namespace)
	if [ -z "$NAMESPACE" ]; then
		echo >&2 "Failed to determine the current namespace!"; exit 1
	fi
	echo "You are running in namespace $NAMESPACE."
	echo "Trident will be deployed in namespace ${NAMESPACE}."
	if [ "$NAMESPACE" != "trident" ]
	then
		echo "For maximum security, we recommend running Trident in its own namespace: ./install_trident.sh -n trident"
	fi
fi

# Check if the namespace exists already
TMP=$(kubectl get namespace $NAMESPACE 2>&1)
if [ "$?" -ne "0" ]; then
	TMP=$(kubectl create -f $DIR/trident-namespace.yaml 2>&1)
	if [ "$?" -ne "0" ]; then
		echo >&2 "Installer failed to create namespace ${NAMESPACE}: ${TMP}"; exit 1
	fi
fi

# Delete any previous state
kubectl --namespace=$NAMESPACE delete -f $DIR/launcher-pod.yaml --ignore-not-found=true
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

# Create service accounts
kubectl --namespace=$NAMESPACE create -f $DIR/trident-serviceaccounts.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi

# Create cluster roles
kubectl --namespace=$NAMESPACE create -f $DIR/trident-clusterroles.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi

# Create cluster role bindings
sed -i "s/namespace: [A-Za-z-]\+/namespace: $NAMESPACE/g" $DIR/trident-clusterrolebindings.yaml
kubectl --namespace=$NAMESPACE create -f $DIR/trident-clusterrolebindings.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi

# Create configmap
kubectl --namespace=$NAMESPACE create configmap trident-launcher-config --from-file=$DIR/setup
if [ $? -ne 0 ]; then
	exit 1;
fi

# Create launcher pod
kubectl --namespace=$NAMESPACE create -f $DIR/launcher-pod.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi

echo "Trident deployment definition is available in $DIR/setup/trident-deployment.yaml."
echo "Started launcher in namespace \"$NAMESPACE\"."
