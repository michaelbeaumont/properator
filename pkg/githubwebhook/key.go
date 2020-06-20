package githubwebhook

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"golang.org/x/crypto/ssh"
)

// RawKeyPair can be written to disk.
type RawKeyPair struct {
	Public  []byte
	Private []byte
}

// GenerateKey creates an RSA keypair ready to write to disk.
func GenerateKey() (RawKeyPair, error) {
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return RawKeyPair{}, err
	}

	pub, err := ssh.NewPublicKey(&rsaPriv.PublicKey)
	if err != nil {
		return RawKeyPair{}, err
	}

	pksc1PrivateKey := x509.MarshalPKCS1PrivateKey(rsaPriv)
	priv := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   pksc1PrivateKey,
	}

	return RawKeyPair{
		Public:  ssh.MarshalAuthorizedKey(pub),
		Private: pem.EncodeToMemory(&priv),
	}, nil
}
