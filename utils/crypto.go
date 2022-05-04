// Copyright 2019 NetApp, Inc. All Rights Reserved.

package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math"
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
func MakeHTTPCertInfo(caCertName, serverCertName, clientCertName string) (*CertInfo, error) {
	certInfo := &CertInfo{}

	notBefore := time.Unix(0, 0)                      // The Epoch (1970 Jan 1)
	notAfter := notBefore.Add(time.Hour * 24 * 36525) // 100 years (365.25 days per year)

	// Create CA key
	caKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, err
	}
	caKeyBase64, err := keyToBase64String(caKey)
	if err != nil {
		return nil, err
	}
	caSerial, err := makeSerial()
	if err != nil {
		return nil, err
	}
	certInfo.CAKey = caKeyBase64
	caKeyId, err := bigIntHash(caKey.D)
	if err != nil {
		return nil, err
	}
	// Create CA cert
	caCert := x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               makeSubject(caCertName),
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		SubjectKeyId:          caKeyId,
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &caCert, &caCert, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	certInfo.CACert = certToBase64String(derBytes)

	// Create HTTPS server key
	serverKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serverKeyBase64, err := keyToBase64String(serverKey)
	if err != nil {
		return nil, err
	}
	certInfo.ServerKey = serverKeyBase64

	serverSerial, err := makeSerial()
	if err != nil {
		return nil, err
	}
	serverKeyId, err := bigIntHash(serverKey.D)
	if err != nil {
		return nil, err
	}
	// Create HTTPS server cert
	serverCert := x509.Certificate{
		SerialNumber:   serverSerial,
		Subject:        makeSubject(serverCertName),
		NotBefore:      notBefore,
		NotAfter:       notAfter,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		AuthorityKeyId: caCert.SubjectKeyId,
		SubjectKeyId:   serverKeyId,
		DNSNames:       []string{serverCertName},
	}

	derBytes, err = x509.CreateCertificate(rand.Reader, &serverCert, &caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	certInfo.ServerCert = certToBase64String(derBytes)

	// Create HTTPS client key
	clientKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, err
	}
	clientKeyBase64, err := keyToBase64String(clientKey)
	if err != nil {
		return nil, err
	}
	certInfo.ClientKey = clientKeyBase64

	clientSerial, err := makeSerial()
	if err != nil {
		return nil, err
	}
	clientKeyId, err := bigIntHash(clientKey.D)
	if err != nil {
		return nil, err
	}
	// Create HTTPS client cert
	clientCert := x509.Certificate{
		SerialNumber:   clientSerial,
		Subject:        makeSubject(clientCertName),
		NotBefore:      notBefore,
		NotAfter:       notAfter,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		AuthorityKeyId: caCert.SubjectKeyId,
		SubjectKeyId:   clientKeyId,
	}

	derBytes, err = x509.CreateCertificate(rand.Reader, &clientCert, &caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	certInfo.ClientCert = certToBase64String(derBytes)

	return certInfo, nil
}

func makeSubject(cn string) pkix.Name {
	return pkix.Name{
		Country:      []string{"US"},
		Locality:     []string{"RTP"},
		Organization: []string{"NetApp"},
		Province:     []string{"NC"},
		CommonName:   cn,
	}
}

func makeSerial() (*big.Int, error) {
	maxSerial := big.NewInt(math.MaxInt64)
	serial, err := rand.Int(rand.Reader, maxSerial)
	if nil != err {
		return nil, err
	}
	return serial, nil
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

// bigIntHash is used to hash subject key ids. The standard says we should derive the id from the public key, but it
// only suggests using sha-1. Since that's risky, we're using sha-256.
func bigIntHash(n *big.Int) ([]byte, error) {
	hash := sha256.New()
	if _, err := hash.Write(n.Bytes()); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

// GenerateAESKey generates a cryptographically random 32-byte key, returned as a base64-encoded string
func GenerateAESKey() (string, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	return base64.StdEncoding.EncodeToString(key), err
}

// EncryptStringWithAES takes a string and a key and returns the encrypted,
// base64-encoded form of the string
func EncryptStringWithAES(plainText string, key []byte) (string, error) {
	// Create the cipher
	block, err2 := aes.NewCipher(key)
	if err2 != nil {
		return "", err2
	}

	// Pad the input
	paddedBytes := PKCS7Pad([]byte(plainText), aes.BlockSize)

	// Create the encryptor
	encryptedBytes := make([]byte, aes.BlockSize+len(paddedBytes))
	iv := encryptedBytes[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	cfb := cipher.NewCFBEncrypter(block, iv)

	// Encrypt
	cfb.XORKeyStream(encryptedBytes[aes.BlockSize:], paddedBytes)

	// Return in string format
	return base64.StdEncoding.EncodeToString(encryptedBytes), nil
}

// DecryptStringWithAES takes an encrypted,
// base64-encoded string and a key and returns the plaintext form of the string
func DecryptStringWithAES(encryptedText string, key []byte) (string, error) {
	// Decode the string into byte array
	encryptedBytes, err2 := base64.StdEncoding.DecodeString(encryptedText)
	if err2 != nil {
		return "", err2
	}
	if len(encryptedBytes) < aes.BlockSize {
		return "", fmt.Errorf("encrypted text too short")
	}

	// Create the cipher
	block, err3 := aes.NewCipher(key)
	if err3 != nil {
		return "", err3
	}

	// Create the decrypter
	iv := encryptedBytes[:aes.BlockSize]
	encryptedBytes = encryptedBytes[aes.BlockSize:]
	cfb := cipher.NewCFBDecrypter(block, iv)

	// Decrypt
	paddedBytes := make([]byte, len(encryptedBytes))
	cfb.XORKeyStream(paddedBytes, encryptedBytes)

	// Unpad
	plainText, err4 := PKCS7Unpad(paddedBytes)
	if err4 != nil {
		return "", err4
	}

	// Return in string format
	return string(plainText), nil
}

// PKCS7Pad will pad the input to a multiple of blocksize according to PKCS#7 standard
func PKCS7Pad(input []byte, blockSize int) []byte {
	padLength := blockSize - len(input)%blockSize
	padding := bytes.Repeat([]byte{byte(padLength)}, padLength)
	return append(input, padding...)
}

// PKCS7Pad will remove the padding from input according to PKCS#7 standard
func PKCS7Unpad(input []byte) ([]byte, error) {
	inputLength := len(input)
	paddingLength := int(input[inputLength-1])

	if paddingLength > inputLength {
		return nil, errors.New("error unpadding")
	}

	return input[:(inputLength - paddingLength)], nil
}
