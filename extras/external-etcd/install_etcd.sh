#!/bin/bash

#$ ./install_etcd.sh -h
#Usage:
# -n <namespace>:     Specifies the namespace for the etcd cluster; defaults to the current namespace.
# -h:                 Prints this usage guide.
#
#Example:
#  ./install_etcd.sh -n trident		Installs the etcd cluster in namespace "trident".

usage() {
	printf "\nUsage:\n"
	printf " %-20s%s\n" "-n <namespace>" "Specifies the namespace for the etcd cluster; defaults to the current namespace."
	printf " %-20s%s\n" "-h" "Prints this usage guide."
	printf "\nExample:\n"
	printf " %s\t\t%s\n\n" " ./install_etcd.sh -n trident" "Installs the etcd cluster in namespace \"trident\"."
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

download_cfssl() {
	if [ -e $DIR/bin/cfssl ] && [ -e $DIR/bin/cfssljson ]; then
		echo "cfssl and cfssljson have already been downloaded."
	else
		curl -s -L -o $DIR/bin/cfssl https://pkg.cfssl.org/R1.2/cfssl_linux-amd64
		if [ $? -ne 0 ]; then
			die
		fi
		curl -s -L -o $DIR/bin/cfssljson https://pkg.cfssl.org/R1.2/cfssljson_linux-amd64
		if [ $? -ne 0 ]; then
			die
		fi
		chmod +x $DIR/bin/{cfssl,cfssljson}
	fi
}

# Process arguments
TMP=`getopt -o n:h -- "$@"`
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
export PATH=$PATH:$DIR/bin
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
download_cfssl

# Determine the namespace
if [ -z "$NAMESPACE" ]; then
	NAMESPACE=$(get_namespace)
	if [ -z "$NAMESPACE" ]; then
		echo >&2 "Failed to determine the current namespace!"; exit 1
	fi
	echo "You are running in namespace $NAMESPACE."
	echo "etcd cluster will be deployed in namespace ${NAMESPACE}."
	if [ "$NAMESPACE" != "trident" ]
	then
		echo "For maximum security, we recommend deploying the etcd cluster in the \"trident\" namespace: ./install_etcd.sh -n trident"
	fi
fi

# Check if the namespace exists already
TMP=$($CMD get namespace $NAMESPACE 2>&1)
if [ "$?" -ne "0" ]; then
	sed -i -r "s/name: [a-z0-9]([-a-z0-9]*[a-z0-9])?/name: $NAMESPACE/g" kubernetes-yaml/etcd-namespace.yaml
	if [ $? -ne 0 ]; then
		exit 1;
	fi
	TMP=$($CMD create -f $DIR/kubernetes-yaml/etcd-namespace.yaml 2>&1)
	if [ "$?" -ne "0" ]; then
		echo >&2 "Installer failed to create namespace ${NAMESPACE}: ${TMP}"; exit 1
	fi
fi

# Create service accounts
$CMD --namespace=$NAMESPACE create -f kubernetes-yaml/etcd-serviceaccount.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi

# Create cluster role
#TODO: OpenShift support
$CMD --namespace=$NAMESPACE create -f kubernetes-yaml/etcd-clusterrole-${ENV}.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi

# Create cluster role binding
sed -i -r "s/namespace: [a-z0-9]([-a-z0-9]*[a-z0-9])?/namespace: $NAMESPACE/g" kubernetes-yaml/etcd-clusterrolebinding-${ENV}.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi
$CMD --namespace=$NAMESPACE create -f kubernetes-yaml/etcd-clusterrolebinding-${ENV}.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi

# Create etcd operator
sed -i -r "s/namespace: [a-z0-9]([-a-z0-9]*[a-z0-9])?/namespace: $NAMESPACE/g" kubernetes-yaml/etcd-operator-deployment.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi
$CMD --namespace=$NAMESPACE create -f kubernetes-yaml/etcd-operator-deployment.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi

# Create certificates and keys
sed -i -r "s/trident-etcd.[a-z0-9]([-a-z0-9]*[a-z0-9])?.svc/trident-etcd.$NAMESPACE.svc/g" certs/server-csr.json
if [ $? -ne 0 ]; then
	exit 1;
fi
sed -i -r "s/trident-etcd-client.[a-z0-9]([-a-z0-9]*[a-z0-9])?.svc/trident-etcd-client.$NAMESPACE.svc/g" certs/server-csr.json
if [ $? -ne 0 ]; then
	exit 1;
fi
sed -i -r "s/trident-etcd.[a-z0-9]([-a-z0-9]*[a-z0-9])?.svc/trident-etcd.$NAMESPACE.svc/g" certs/peer-csr.json
if [ $? -ne 0 ]; then
	exit 1;
fi
(cd $DIR/certs && ./gen-ca.sh)
if [ $? -ne 0 ]; then
	exit 1;
fi
(cd $DIR/certs && ./gen-client.sh)
if [ $? -ne 0 ]; then
	exit 1;
fi
(cd $DIR/certs && ./gen-server.sh)
if [ $? -ne 0 ]; then
	exit 1;
fi
(cd $DIR/certs && ./gen-peer.sh)
if [ $? -ne 0 ]; then
	exit 1;
fi

# Create secrets
$CMD --namespace=$NAMESPACE create secret generic etcd-client-tls --from-file=etcd-client-ca.crt=$DIR/certs/ca.pem --from-file=etcd-client.crt=$DIR/certs/client.pem --from-file=etcd-client.key=$DIR/certs/client-key.pem
if [ $? -ne 0 ]; then
	exit 1;
fi
$CMD --namespace=$NAMESPACE create secret generic etcd-server-tls --from-file=server-ca.crt=$DIR/certs/ca.pem --from-file=server.crt=$DIR/certs/server.pem --from-file=server.key=$DIR/certs/server-key.pem
if [ $? -ne 0 ]; then
	exit 1;
fi
$CMD --namespace=$NAMESPACE create secret generic etcd-peer-tls --from-file=peer-ca.crt=$DIR/certs/ca.pem --from-file=peer.crt=$DIR/certs/peer.pem --from-file=peer.key=$DIR/certs/peer-key.pem
if [ $? -ne 0 ]; then
	exit 1;
fi

# Create etcd cluster
$CMD --namespace=$NAMESPACE create -f kubernetes-yaml/etcd-cluster.yaml
if [ $? -ne 0 ]; then
	exit 1;
fi


