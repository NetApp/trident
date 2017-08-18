#!/bin/bash

if [ -e peer.csr ] && [ -e peer.pem ] && [ -e peer-key.pem ]; then
	exit 0
fi

cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=peer peer-csr.json | cfssljson -bare peer
