#!/bin/bash

TRIDENT_PORT=8000

if [ -z "$TRIDENT_IP" ]
then
	export TRIDENT_IP=`kubectl describe pod --selector=app=trident.netapp.io 2>/dev/null | grep ^IP | awk -F' '  '{print $NF}'`
fi
if [ -z "$TRIDENT_IP" ]
then 
	TRIDENT_IP=$(docker ps -a 2>/dev/null | awk '{print $1,$2}' | grep trident | awk '{print $1}' | xargs -I {} docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' {})
fi
if [ -z "$TRIDENT_IP" ]
then
	>&2 echo "Unable to discover Trident IP.  Either Trident is not running or its IP address must be manually set at \$TRIDENT_IP."
	exit 1
fi

if [ $# -eq 1 ]
then
	curl -s -S ${TRIDENT_IP}:${TRIDENT_PORT}/trident/v1/${1} | aeson-pretty
elif [ $# -eq 2 ]
then
	curl -s -S ${TRIDENT_IP}:${TRIDENT_PORT}/trident/v1/${1}/${2} | aeson-pretty
else
	>&2 echo "Usage: $0 <resource-type> <resource-name>"
	>&2 echo "resource-type:  either 'volume' or 'backend'.  Required."
	>&2 echo "resource-name:  name of specific resource.  Optional."
	exit 1
fi
