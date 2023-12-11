package syndicat

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/go-fed/httpsig"
)

func MakeRSAKey() (*rsa.PrivateKey, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	return privKey, nil
}

func SaveRSAKey(keyPath string, privKey *rsa.PrivateKey) error {

	privKeyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	})

	err := os.WriteFile(keyPath, privKeyPem, 0644)
	if err != nil {
		return err
	}

	return nil
}

func LoadRSAKey(keyPath string) (*rsa.PrivateKey, error) {
	privKeyPemBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(privKeyPemBytes)
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the key")
	}

	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return privKey, nil
}

func GetPublicKeyPem(privKey *rsa.PrivateKey) (string, error) {
	pubKey := &privKey.PublicKey

	var pubKeyBuf strings.Builder

	pubKeyPEM := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(pubKey),
	}
	if err := pem.Encode(&pubKeyBuf, pubKeyPEM); err != nil {
		return "", err
	}

	return pubKeyBuf.String(), nil
}

func sign(privateKey crypto.PrivateKey, pubKeyId string, r *http.Request) error {
	//prefs := []httpsig.Algorithm{httpsig.RSA_SHA512, httpsig.RSA_SHA256}
	prefs := []httpsig.Algorithm{httpsig.RSA_SHA256}
	digestAlgorithm := httpsig.DigestSha256
	// The "Date" and "Digest" headers must already be set on r, as well as r.URL.
	//headersToSign := []string{httpsig.RequestTarget, "date", "digest"}
	headersToSign := []string{httpsig.RequestTarget, "date", "host"}
	var sigExpirationSeconds int64 = 3600
	signer, _, err := httpsig.NewSigner(prefs, digestAlgorithm, headersToSign, httpsig.Signature, sigExpirationSeconds)
	if err != nil {
		return err
	}
	// To sign the digest, we need to give the signer a copy of the body...
	// ...but it is optional, no digest will be signed if given "nil"
	//body := []byte("Hi there")
	// If r were a http.ResponseWriter, call SignResponse instead.
	return signer.SignRequest(privateKey, pubKeyId, r, nil)
}
