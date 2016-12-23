#!/bin/bash

cd "$( dirname "${0}" )"

function die() {
		 cat<<-EOF 
			Usage:
			-n<namespace>, --namespace <namespace>:  Namespace in which to deploy Trident; defaults to "default"
			-s<service-account>, --serviceaccount <service-account>:  service account to use for Trident; defaults to "default"
			EOF
			exit 1
		}
TEMP=`getopt -o n:s: --long namespace:,serviceaccount: -- "$@"`
if [ $? -ne 0 ]; then
	die
fi

eval set -- "$TEMP"


while true ; do
	case "$1" in
		-n|--namespace)
			NAMESPACE=$2
			shift 2 ;;
		-s|--serviceaccount)
			SERVICE_ACCOUNT=$2
			shift 2;;
		--) shift ; break ;;
		*)	die ;;
	esac
done

if [ ! -f ./setup/backend.json ]
then
	>&2 echo "Setup directory must contain backend definition in 'backend.json'."
	exit 1
fi

# Check for necessary utilities
command -v curl > /dev/null 2>&1 || \
	{ echo >&2 "$0 requires curl present in \$PATH."; exit 1; }
command -v kubectl > /dev/null || \
	{ echo >&2 "$0 requires kubectl present in \$PATH."; exit 1; }

kubectl delete configmap --ignore-not-found=true trident-launcher-config
kubectl delete pod --ignore-not-found=true trident-launcher

if [ ! -z $NAMESPACE ]
then
	sed -i "s/namespace: [A-Za-z-]\+/namespace: $NAMESPACE/g" setup/trident-deployment.yaml
	sed -i "s/namespace: [A-Za-z-]\+/namespace: $NAMESPACE/g" launcher-pod.yaml
fi

if [ ! -z $SERVICE_ACCOUNT ]
then
	sed -i "s/serviceAccount: [A-Za-z-]\+/serviceAccount: $SERVICE_ACCOUNT/g" setup/trident-deployment.yaml
	sed -i "s/serviceAccount: [A-Za-z-]\+/serviceAccount: $SERVICE_ACCOUNT/g" launcher-pod.yaml
fi

kubectl create configmap trident-launcher-config --from-file=./setup
if [ $? -ne 0 ]; then
	exit 1;
fi
kubectl create -f ./launcher-pod.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi

echo "Started launcher.  Trident deployment definition is available in $1/trident-deployment.yaml"
