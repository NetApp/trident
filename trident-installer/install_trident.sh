#!/bin/bash

#$ ./install_trident.sh -h
#Usage:
# -n <namespace>:     Specifies the namespace for the Trident deployment; defaults to the current namespace.
# -i                  Enables the insecure mode to disable RBAC configuration for Trident and Trident launcher.
# -h:                 Prints this usage guide.
#
#Example:
#  ./install_trident.sh -n trident		Installs the Trident deployment in namespace "trident".

usage() {
	printf "\nUsage:\n"
	printf " %-20s%s\n" "-n <namespace>" "Specifies the namespace for the Trident deployment; defaults to the current namespace."
	printf " %-20s%s\n" "-i" "Enables the insecure mode to disable RBAC configuration for Trident and Trident launcher."
	printf " %-20s%s\n" "-h" "Prints this usage guide."
	printf "\nExample:\n"
	printf " %s\t\t%s\n\n" " ./install_trident.sh -n trident" "Installs the Trident deployment in namespace \"trident\"."
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
	VERSION=$($CMD version | grep "Server Version" | grep -oP '(?<=GitVersion:")[^"]+')
	echo $VERSION
}

# Process arguments
TMP=`getopt -o n:i,h -- "$@"`
if [ $? -ne 0 ]; then
	die
fi
eval set -- "$TMP"
while true ; do
	case "$1" in
		-n)
			NAMESPACE=$2
			shift 2 ;;
		-i)
			INSECURE="true"
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
TMP=$($CMD get namespace $NAMESPACE 2>&1)
if [ "$?" -ne "0" ]; then
	TMP=$($CMD create -f $DIR/trident-namespace.yaml 2>&1)
	if [ "$?" -ne "0" ]; then
		echo >&2 "Installer failed to create namespace ${NAMESPACE}: ${TMP}"; exit 1
	fi
fi

# Delete any previous state
$CMD --namespace=$NAMESPACE delete -f $DIR/launcher-pod.yaml --ignore-not-found=true
if [ $? -ne 0 ]; then
	exit 1;
fi
if [ -z "$INSECURE" ]; then
	$CMD --namespace=$NAMESPACE delete -f $DIR/trident-clusterrolebindings-${ENV}.yaml --ignore-not-found=true
	if [ $? -ne 0 ]; then
		exit 1;
	fi
	$CMD --namespace=$NAMESPACE delete -f $DIR/trident-clusterroles-${ENV}.yaml --ignore-not-found=true
	if [ $? -ne 0 ]; then
		exit 1;
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

# Create service accounts
$CMD --namespace=$NAMESPACE create -f $DIR/trident-serviceaccounts.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi

if [ -z "$INSECURE" ]; then
	# Create cluster roles
	$CMD --namespace=$NAMESPACE create -f $DIR/trident-clusterroles-${ENV}.yaml
	if [ $? -ne 0 ]; then
		exit 1;
	fi
	# Create cluster role bindings
	sed -i "s/namespace: [A-Za-z-]\+/namespace: $NAMESPACE/g" $DIR/trident-clusterrolebindings-${ENV}.yaml
	if [ $? -ne 0 ]; then
		exit 1;
	fi
	$CMD --namespace=$NAMESPACE create -f $DIR/trident-clusterrolebindings-${ENV}.yaml
	if [ $? -ne 0 ]; then
		exit 1;
	fi
fi

# Create configmap
TMP=$(grep -E -A1 '^[[:space:]]+\- "-pvc_name"' $DIR/launcher-pod.yaml | grep -v pvc_name)
if [ -n "$TMP" ]; then
	PVC=$(echo $TMP | grep -oP '(?<=")[^"]*')
	sed -i "s/claimName: .*$/claimName: $PVC/g" $DIR/setup/trident-deployment.yaml
else
	sed -i "s/claimName: .*$/claimName: trident/g" $DIR/setup/trident-deployment.yaml
fi
$CMD --namespace=$NAMESPACE create configmap trident-launcher-config --from-file=$DIR/setup
if [ $? -ne 0 ]; then
	exit 1;
fi

# Associate security context constraint 'anyuid' with service account 'trident' for OpenShift
if [ "$ENV" == "openshift" ]; then
	$CMD describe scc anyuid | grep trident > /dev/null
	if [ $? -ne 0 ]; then
		$CMD adm policy add-scc-to-user anyuid -z trident
		if [ $? -ne 0 ]; then
			exit 1;
		fi
	fi
fi

# Create launcher pod
$CMD --namespace=$NAMESPACE create -f $DIR/launcher-pod.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi

echo "Trident deployment definition is available in $DIR/setup/trident-deployment.yaml."
echo "Started launcher in namespace \"$NAMESPACE\"."
