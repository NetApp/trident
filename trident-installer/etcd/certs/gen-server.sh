#!/bin/bash

if [ -e server.csr ] && [ -e server.pem ] && [ -e server-key.pem ]; then
	exit 0
fi

cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=server server-csr.json | cfssljson -bare server

