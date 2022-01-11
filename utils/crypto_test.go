package utils

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"math/big"
	"testing"
)

func TestGenerateAESKey(t *testing.T) {
	var (
		key1, key2 string
		err        error
	)

	// Test key creation
	if key1, err = GenerateAESKey(); err != nil {
		t.Fatalf("AES key generation failed; %v", err)
	}

	// Test key encoding
	keyBytes, err2 := base64.StdEncoding.DecodeString(key1)
	if err2 != nil {
		t.Fatal("AES key is not a base64-encoded string")
	}

	// Test key length
	if len(keyBytes) != 32 {
		t.Error("AES key is not 32 bytes long.")
	}

	// Test key uniqueness
	if key2, err = GenerateAESKey(); err != nil {
		t.Fatalf("AES key generation failed; %v", err)
	}
	if key1 == key2 {
		t.Error("AES keys are not random.")
	}
}

func TestEncryptStringWithAES(t *testing.T) {
	var (
		encodedKey, encryptedText, decryptedText string
		key, encryptedBytes                      []byte
		err                                      error
	)

	random512bytes := make([]byte, 512) // Max CHAP secret length
	_, err = rand.Read(random512bytes)
	if err != nil {
		t.Fatalf("could not generate 512 byte string; %v", err)
	}
	plainTexts := []string{
		"Foobar",
		"",
		" ",
		"1234",
		"ಠ_ಠ",
		"!@#$%^&*?",
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit, " +
			"sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, " +
			"quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. " +
			"Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. " +
			"Excepteur sint occaecat cupidatat non proident, " +
			"sunt in culpa qui officia deserunt mollit anim id est laborum.",
		string(random512bytes),
	}

	if encodedKey, err = GenerateAESKey(); err != nil {
		t.Fatalf("AES key generation failed; %v", err)
	}
	if key, err = base64.StdEncoding.DecodeString(encodedKey); err != nil {
		t.Fatalf("Could not decode AES key; %v", err)
	}

	for _, plainText := range plainTexts {
		// Test string encryption
		if encryptedText, err = EncryptStringWithAES(plainText, key); err != nil {
			t.Fatalf("Encryption of text failed; %v", err)
		}

		// Test that the string was modified
		if encryptedText == plainText {
			t.Fatal("The string was not modified.")
		}

		// Test that the result is base64-encoded
		if encryptedBytes, err = base64.StdEncoding.DecodeString(encryptedText); err != nil {
			t.Error("Returned string is not base64-encoded.")
		}

		// Test that the result is encrypted
		if bytes.Equal([]byte(plainText), encryptedBytes) {
			t.Fatal("The string was not encrypted.")
		}

		// Test decryption
		if decryptedText, err = DecryptStringWithAES(encryptedText, key); err != nil {
			t.Fatalf("String could not be decrypted back; %v", err)
		}

		// Test the decrypted result matches the original string
		if plainText != decryptedText {
			t.Error("Decrypted string does not match original string.")
		}
	}
}

func TestPKCS7Pad(t *testing.T) {
	blockSize := 16 // aes block size is 16
	inputSizes := []int{
		0,
		1,
		blockSize,
		123,
		512,
	}
	for _, inputSize := range inputSizes {
		input := make([]byte, inputSize)
		paddedInput := PKCS7Pad(input, blockSize)

		// Test that padding was added, no matter the input size
		if len(paddedInput) <= inputSize {
			t.Errorf("padding was not added when input was %d bytes long", inputSize)
		}
		// Test that padded result is a multiple of block size
		if len(paddedInput)%blockSize != 0 {
			t.Error("padding was not added to a multiple of block size")
		}
		// Test that there is a proper amount of padding
		padLength := int(paddedInput[len(paddedInput)-1])
		if padLength <= 0 || padLength > blockSize {
			t.Error("padding must be at least 1 byte and no more than the block size")
		}
		// Test that the value of the pad bytes is correct
		if len(paddedInput)-padLength != inputSize {
			t.Error("padded values are incorrect")
		}
		// Test that we can unpad
		output, err := PKCS7Unpad(paddedInput)
		if err != nil {
			t.Error(err.Error())
		}
		if !bytes.Equal(output, input) {
			t.Error("padding was properly removed")
		}
	}
}

func TestBigIntHash(t *testing.T) {
	var n big.Int
	h, err := bigIntHash(&n)
	if err != nil {
		t.Error(err.Error())
	}

	if bytes.Compare(h, n.Bytes()) == 0 {
		t.Error("bigIntHash returned big int unchanged")
	}

	n.SetInt64(1)
	h1, err := bigIntHash(&n)
	if err != nil {
		t.Error(err.Error())
	}

	if bytes.Compare(h, h1) == 0 {
		t.Error("bigIntHash returned same result for different inputs")
	}
}
