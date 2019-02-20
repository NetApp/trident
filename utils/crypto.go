// Copyright 2019 NetApp, Inc. All Rights Reserved.

package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"time"
)

type CertInfo struct {
	CAKey      string
	CACert     string
	ServerKey  string
	ServerCert string
	ClientKey  string
	ClientCert string
}

// makeHTTPCertInfo generates a CA key and cert, then uses that key to sign two
// other keys and certs, one for a TLS server and one for a TLS client. None of
// the parameters are configurable...the serial numbers and principal names are
// hardcoded, the validity period is hardcoded to 1970-2070, and the algorithm
// and key size are hardcoded to 521-bit elliptic curve.
func MakeHTTPCertInfo() (*CertInfo, error) {

	certInfo := &CertInfo{}

	notBefore := time.Unix(0, 0)                      // The Epoch (1970 Jan 1)
	notAfter := notBefore.Add(time.Hour * 24 * 36525) // 100 years (365.25 days per year)

	caKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, err
	}
	caKeyBase64, err := keyToBase64String(caKey)
	if err != nil {
		return nil, err
	}
	certInfo.CAKey = caKeyBase64

	caCert := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(1),
		Subject: pkix.Name{
			Country:      []string{"US"},
			Locality:     []string{"RTP"},
			Organization: []string{"NetApp"},
			Province:     []string{"NC"},
			CommonName:   "CA",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		SubjectKeyId:          bigIntHash(caKey.D),
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &caCert, &caCert, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	certInfo.CACert = certToBase64String(derBytes)

	serverKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serverKeyBase64, err := keyToBase64String(serverKey)
	if err != nil {
		return nil, err
	}
	certInfo.ServerKey = serverKeyBase64

	serverCert := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(2),
		Subject: pkix.Name{
			Country:      []string{"US"},
			Locality:     []string{"RTP"},
			Organization: []string{"NetApp"},
			Province:     []string{"NC"},
			CommonName:   "csi-controller",
		},
		NotBefore:      notBefore,
		NotAfter:       notAfter,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		AuthorityKeyId: caCert.SubjectKeyId,
		SubjectKeyId:   bigIntHash(serverKey.D),
	}

	derBytes, err = x509.CreateCertificate(rand.Reader, &serverCert, &caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	certInfo.ServerCert = certToBase64String(derBytes)

	clientKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, err
	}
	clientKeyBase64, err := keyToBase64String(clientKey)
	if err != nil {
		return nil, err
	}
	certInfo.ClientKey = clientKeyBase64

	clientCert := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(3),
		Subject: pkix.Name{
			Country:      []string{"US"},
			Locality:     []string{"RTP"},
			Organization: []string{"NetApp"},
			Province:     []string{"NC"},
			CommonName:   "csi-node",
		},
		NotBefore:      notBefore,
		NotAfter:       notAfter,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		AuthorityKeyId: caCert.SubjectKeyId,
		SubjectKeyId:   bigIntHash(clientKey.D),
	}

	derBytes, err = x509.CreateCertificate(rand.Reader, &clientCert, &caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	certInfo.ClientCert = certToBase64String(derBytes)

	return certInfo, nil
}

func keyToBase64String(key *ecdsa.PrivateKey) (string, error) {
	b, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", err
	}
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	return base64.StdEncoding.EncodeToString(keyBytes), nil
}

func certToBase64String(derBytes []byte) string {
	certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	return base64.StdEncoding.EncodeToString(certBytes)
}

func bigIntHash(n *big.Int) []byte {
	hash := sha1.New()
	hash.Write(n.Bytes())
	return hash.Sum(nil)
}
