// Copyright 2019 NetApp, Inc. All Rights Reserved.

package rest

import "time"

const (
	HTTPTimeout = 90 * time.Second

	CACertName     = "trident-ca"
	ServerCertName = "trident-csi" // Must match CSI service name
	ClientCertName = "trident-node"

	CAKeyFile      = "caKey"
	CACertFile     = "caCert"
	ServerKeyFile  = "serverKey"
	ServerCertFile = "serverCert"
	ClientKeyFile  = "clientKey"
	ClientCertFile = "clientCert"

	certsPath = "/certs/"

	CAKeyPath      = certsPath + CAKeyFile
	CACertPath     = certsPath + CACertFile
	ServerKeyPath  = certsPath + ServerKeyFile
	ServerCertPath = certsPath + ServerCertFile
	ClientKeyPath  = certsPath + ClientKeyFile
	ClientCertPath = certsPath + ClientCertFile
)
