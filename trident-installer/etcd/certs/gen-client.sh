#!/bin/bash

if [ -e client.csr ] && [ -e client.pem ] && [ -e client-key.pem ]; then
	exit 0
fi

cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=client client-csr.json | cfssljson -bare client
