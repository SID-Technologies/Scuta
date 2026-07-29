// Package sigverify implements detached-signature creation and verification
// for Scuta's supply-chain trust chain. It is the single home for signature
// primitives: release assets, the remote registry, remote policy, and remote
// config all verify through this package.
//
// Keys are PEM-encoded: public keys in PKIX form ("PUBLIC KEY"), private keys
// in PKCS #8 form ("PRIVATE KEY"). RSA, ECDSA, and Ed25519 are supported.
package sigverify

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"

	"github.com/sid-technologies/scuta/lib/errors"
)

// Verify checks a detached signature over data using a PEM-encoded public key.
// RSA and ECDSA signatures are over the SHA-256 digest of data; Ed25519 signs
// the raw data (per the Ed25519 construction).
func Verify(data []byte, sig []byte, publicKeyPEM []byte) error {
	block, _ := pem.Decode(publicKeyPEM)
	if block == nil {
		return errors.New("failed to decode PEM public key")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return errors.Wrap(err, "parsing public key")
	}

	hash := sha256.Sum256(data)

	switch key := pubKey.(type) {
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], sig); err != nil {
			return errors.New("RSA signature verification failed")
		}
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(key, hash[:], sig) {
			return errors.New("ECDSA signature verification failed")
		}
	case ed25519.PublicKey:
		if !ed25519.Verify(key, data, sig) {
			return errors.New("Ed25519 signature verification failed")
		}
	default:
		return errors.New("unsupported public key type: %T", pubKey)
	}

	return nil
}

// Sign produces a detached signature over data using a PEM-encoded PKCS #8
// private key. The output pairs with Verify: RSA and ECDSA sign the SHA-256
// digest, Ed25519 signs the raw data.
func Sign(data []byte, privateKeyPEM []byte) ([]byte, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, errors.New("failed to decode PEM private key")
	}

	privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "parsing private key")
	}

	hash := sha256.Sum256(data)

	switch key := privKey.(type) {
	case *rsa.PrivateKey:
		sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
		if err != nil {
			return nil, errors.Wrap(err, "RSA signing failed")
		}
		return sig, nil
	case *ecdsa.PrivateKey:
		sig, err := ecdsa.SignASN1(rand.Reader, key, hash[:])
		if err != nil {
			return nil, errors.Wrap(err, "ECDSA signing failed")
		}
		return sig, nil
	case ed25519.PrivateKey:
		return ed25519.Sign(key, data), nil
	default:
		return nil, errors.New("unsupported private key type: %T", privKey)
	}
}

// GenerateEd25519Keys creates a new Ed25519 keypair and returns it PEM-encoded
// (PKIX public key, PKCS #8 private key). Ed25519 is the recommended default:
// small keys, fast, no parameter or hash-choice foot-guns.
func GenerateEd25519Keys() (publicPEM []byte, privatePEM []byte, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, errors.Wrap(err, "generating Ed25519 key")
	}

	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, nil, errors.Wrap(err, "encoding public key")
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, errors.Wrap(err, "encoding private key")
	}

	publicPEM = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	privatePEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

	return publicPEM, privatePEM, nil
}
