// Package secretbox provides authenticated encryption for secrets stored at rest
// (currently Jira OAuth tokens) using AES-256-GCM with a key derived from the
// application secret.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// ErrMalformed is returned when the ciphertext is too short or fails authentication.
var ErrMalformed = errors.New("secretbox: malformed or tampered ciphertext")

// Box encrypts and decrypts small byte slices.
type Box struct {
	aead cipher.AEAD
}

// New derives a 256-bit key from the application secret.
func New(secret string) (*Box, error) {
	key := sha256.Sum256([]byte("estimeet/secretbox/v1|" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext and prefixes the random nonce.
func (b *Box) Seal(plaintext string) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Open authenticates and decrypts a value produced by Seal.
func (b *Box) Open(ciphertext []byte) (string, error) {
	n := b.aead.NonceSize()
	if len(ciphertext) < n {
		return "", ErrMalformed
	}
	plaintext, err := b.aead.Open(nil, ciphertext[:n], ciphertext[n:], nil)
	if err != nil {
		return "", ErrMalformed
	}
	return string(plaintext), nil
}
