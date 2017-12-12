#!/bin/bash

#$ ./uninstall_trident.sh -h
#Usage:
# -n <namespace>      Specifies the namespace for the Trident deployment; defaults to the current namespace.
# -a                  Deletes almost all artifacts of Trident, including the PVC and PV used by Trident; however, it doesn't delete the volume used by Trident from the storage backend. Use with caution!
# -h                  Prints this usage guide.
#
#Example:
#  ./uninstall_trident.sh -n trident		Deletes artifacts of Trident from namespace "trident".

usage() {
	printf "\nUsage:\n"
	printf " %-20s%s\n" "-n <namespace>" "Specifies the namespace for the Trident deployment; defaults to the current namespace."
	printf " %-20s%s\n" "-a" "Deletes almost all artifacts of Trident, including the PVC and PV used by Trident; however, it doesn't delete the volume used by Trident from the storage backend. Use with caution!"
	printf " %-20s%s\n" "-h" "Prints this usage guide."
	printf "\nExample:\n"
	printf " %s\t\t%s\n\n" " ./uninstall_trident.sh -n trident" "Deletes artifacts of Trident from namespace \"trident\"."
}

die() {
	usage
	exit 1
}

get_namespace() {
	TMP=$($CMD get serviceaccount default -o json | grep "namespace\":" | awk '{print $2}' | sed 's/,//g; s/"//g')
	echo $TMP
}

get_environment() {
	TMP=$(command -v oc > /dev/null 2>&1)
	if [ $? -ne 0 ]; then
		echo k8s
	else
		echo openshift
	fi
}

get_environment_version() {
	TMP=$($CMD version | grep "Server Version" | grep -oP '(?<=GitVersion:")[^"]+')
	echo $TMP
}

version_gt() {
    # Returns true if $1 > $2
    test "$(printf '%s\n' "$@" | sort -V | head -n 1)" != "$1";
}

# Process arguments
TMP=`getopt -o n:a,h -- "$@"`
if [ $? -ne 0 ]; then
	die
fi
eval set -- "$TMP"
while true ; do
	case "$1" in
		-n)
			NAMESPACE=$2
			shift 2 ;;
		-a)
			DELETEALL="true"
			shift ;;
		-h)
			usage
			exit 0 ;;
		--) shift ; break ;;
		*)	die ;;
	esac
done

# Check for the requirements
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ENV=$(get_environment)
if [ "$ENV" == "k8s" ]; then
	CMD="kubectl"
	echo "Installer assumes you have deployed Kubernetes. If this is an OpenShift deployment, make sure 'oc' is in the \$PATH."
else
	CMD="oc"
	echo "Installer assumes you have deployed OpenShift."
fi
command -v curl > /dev/null 2>&1 || \
	{ echo >&2 "$0 requires curl present in \$PATH."; exit 1; }
command -v $CMD > /dev/null || \
	{ echo >&2 "$0 requires $CMD present in \$PATH."; exit 1; }

# Determine YAML files based on environment and version
VERSION=$(get_environment_version)
if version_gt "v1.8.0" $VERSION; then
    CLUSTER_ROLE_BINDINGS_YAML=$DIR/trident-clusterrolebindings-${ENV}-v1alpha1.yaml
    CLUSTER_ROLES_YAML=$DIR/trident-clusterroles-${ENV}-v1alpha1.yaml
else
    CLUSTER_ROLE_BINDINGS_YAML=$DIR/trident-clusterrolebindings-${ENV}.yaml
    CLUSTER_ROLES_YAML=$DIR/trident-clusterroles-${ENV}.yaml
fi

# Determine the namespace
if [ -z $NAMESPACE ]; then
	NAMESPACE=$(get_namespace)
	if [ -z "$NAMESPACE" ]; then
		echo >&2 "Failed to determine the current namespace!"; exit 1
	fi
	echo "You are running in namespace $NAMESPACE."
	echo "Cleanup will take place in namespace ${NAMESPACE}."
fi
TMP=$($CMD get namespace $NAMESPACE 2>&1)
if [ "$?" -ne "0" ]; then
	exit 1
fi

# Determine the PVC and PV used by the Trident deployment
PVC=$(grep -oP '(?<=claimName: ).+' $DIR/setup/trident-deployment.yaml)
if [ -n "$PVC" ]; then
	PV=$($CMD --namespace=$NAMESPACE get pvc $PVC 2>&1 | grep -vE 'NAME|Error' | awk '{ if ($2 == "Bound") print $3 }')
fi

# Clean up Trident artifacts
$CMD --namespace=$NAMESPACE delete -f $DIR/setup/trident-deployment.yaml --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi

$CMD --namespace=$NAMESPACE delete -f $DIR/launcher-pod.yaml --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi

$CMD --namespace=$NAMESPACE delete pod trident-ephemeral --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi

$CMD --namespace=$NAMESPACE delete -f $CLUSTER_ROLE_BINDINGS_YAML --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi

$CMD --namespace=$NAMESPACE delete -f $CLUSTER_ROLES_YAML --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi

if [ "$ENV" == "openshift" ]; then
	$CMD --namespace=$NAMESPACE describe scc anyuid | grep trident > /dev/null
	if [ $? -eq 0 ]; then
		$CMD --namespace=$NAMESPACE adm policy remove-scc-from-user anyuid -z trident
		if [ $? -ne 0 ]; then
			exit 1;
		fi
	fi
fi

$CMD --namespace=$NAMESPACE delete -f $DIR/trident-serviceaccounts.yaml --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi

$CMD --namespace=$NAMESPACE delete configmap trident-launcher-config --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi

if [ -n "$DELETEALL" ]; then
	if [ -n "$PV" ]; then
		$CMD delete pv $PV --ignore-not-found=true
	fi
	if [ -n "$PVC" ]; then
		$CMD --namespace=$NAMESPACE delete pvc $PVC --ignore-not-found=true
	fi
	echo "If desired, the volume on the storage backend needs to be manually deleted. Deleting the volume would result in losing all the state that Trident maintains to manage storage backends, storage classes, and provisioned volumes!"
else
	echo "This script didn't delete namespace \"$NAMESPACE\" and PVC and PV \"trident\" in case they're going to be reused. Please use the -a option if you need PVC and PV \"trident\" deleted."
fi

echo "Successfully deleted Trident deployment, service accounts, and cluster roles and role bindings."
echo "Successfully deleted Trident launcher pod and configuration."
