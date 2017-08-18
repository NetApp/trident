#!/bin/bash

if [ -e ca.csr ] && [ -e ca.pem ] && [ -e ca-key.pem ]; then
	exit 0
fi

cfssl gencert -initca ca-csr.json | cfssljson -bare ca -
