package codec

// https://stackoverflow.com/questions/56714284/golang-encrypting-data-using-aes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

type Crypto struct {
	block     cipher.Block
	gcm       cipher.AEAD
	nonceSize int
}

// normalizeKey приводит произвольный "секрет" к валидному ключу AES.
// Ключи ровно 16/24/32 байта используются как есть (обратная совместимость),
// всё остальное деривируется SHA-256 в 32 байта.
func normalizeKey(key []byte) []byte {
	switch len(key) {
	case 16, 24, 32:
		return key
	default:
		sum := sha256.Sum256(key)
		return sum[:]
	}
}

func newCrypto(key []byte) (*Crypto, error) {
	block, err := aes.NewCipher(normalizeKey(key))
	if err != nil {
		return nil, fmt.Errorf("rpc: create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("rpc: create gcm: %w", err)
	}

	return &Crypto{
		block:     block,
		gcm:       gcm,
		nonceSize: gcm.NonceSize(),
	}, nil
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
