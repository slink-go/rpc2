package codec

// https://stackoverflow.com/questions/56714284/golang-encrypting-data-using-aes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

type Crypto struct {
	block     cipher.Block
	gcm       cipher.AEAD
	nonceSize int
}

func newCrypto(key []byte) *Crypto {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}

	return &Crypto{
		block:     block,
		gcm:       gcm,
		nonceSize: gcm.NonceSize(),
	}
}

func (c *Crypto) Encrypt(plaintext []byte) (ciphertext []byte) {
	nonce := randBytes(c.nonceSize)
	cc := c.gcm.Seal(nil, nonce, plaintext, nil)
	return append(nonce, cc...)
}

func (c *Crypto) Decrypt(ciphertext []byte) (plaintext []byte, err error) {
	if len(ciphertext) < c.nonceSize {
		return nil, fmt.Errorf("ciphertext is too short")
	}
	nonce := ciphertext[0:c.nonceSize]
	msg := ciphertext[c.nonceSize:]
	return c.gcm.Open(nil, nonce, msg, nil)
}

func randBytes(length int) []byte {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return b
}
