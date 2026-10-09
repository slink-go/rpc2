package codec

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"go.slink.ws/logging"
	"go.slink.ws/rpc"
)

// magicPrefix — внешний CRYPT-фрейминг, байт-в-байт как в v2:
// после clear-заголовка идёт 'C','R','Y','P','T', затем gob-сообщение
// с шифр-блобом. Внутри шифр-блоба — JSON-байты тела (вместо gob).
var magicPrefix = []byte{'C', 'R', 'Y', 'P', 'T'}

// Crypto — AES-GCM поверх произвольного «секрета» (зеркало v2/codec/crypto.go:
// тот же normalizeKey, поэтому RPC_SECRET-совместимость сохраняется).
type Crypto struct {
	gcm       cipher.AEAD
	nonceSize int
}

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
	return &Crypto{gcm: gcm, nonceSize: gcm.NonceSize()}, nil
}

func (c *Crypto) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("rpc: nonce generate: %w", err)
	}
	sealed := c.gcm.Seal(nil, nonce, plaintext, nil)
	return append(nonce, sealed...), nil
}

func (c *Crypto) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < c.nonceSize {
		return nil, fmt.Errorf("rpc: ciphertext is too short")
	}
	nonce := ciphertext[:c.nonceSize]
	return c.gcm.Open(nil, nonce, ciphertext[c.nonceSize:], nil)
}

// --- запись: заголовок (JSON) + CRYPT + gob(encrypt(json(body))) ---

// writeCryptoBody пишет тело в формате json-plaintext → AES-GCM → gob-блоб.
func writeCryptoBody(encBuf *bufio.Writer, crypto *Crypto, body any) error {
	plaintext, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("rpc: json body marshal: %w", err)
	}
	encrypted, err := crypto.Encrypt(plaintext)
	if err != nil {
		return err
	}
	if _, err = encBuf.Write(magicPrefix); err != nil {
		return fmt.Errorf("rpc: crypto prefix write: %w", err)
	}
	if err = gob.NewEncoder(encBuf).Encode(encrypted); err != nil {
		return fmt.Errorf("rpc: crypto blob encode: %w", err)
	}
	return nil
}

// readCryptoBodySource читает CRYPT-префикс и gob-блоб из src, расшифровывает
// и возвращает JSON-байты тела. Читается напрямую (без буферизации поверх),
// чтобы не потерять байты следующего сообщения, уже попавшие в буфер.
func readCryptoBodySource(src io.Reader, crypto *Crypto, maxBody int64) ([]byte, error) {
	budget := &budgetReader{r: src, left: maxBody}
	// json.Encoder разделяет сообщения '\n' — пропускаем пробельные разделители
	// перед префиксом
	magic := make([]byte, 0, len(magicPrefix))
	var one [1]byte
	for len(magic) < len(magicPrefix) {
		if _, err := io.ReadFull(budget, one[:]); err != nil {
			return nil, fmt.Errorf("rpc: crypto prefix read: %w", err)
		}
		b := one[0]
		if len(magic) == 0 {
			switch b {
			case ' ', '\t', '\n', '\r':
				continue
			}
		}
		if b != magicPrefix[len(magic)] {
			return nil, fmt.Errorf("rpc: invalid crypto prefix: %w", ErrInvalidMagic)
		}
		magic = append(magic, b)
	}
	var blob []byte
	if err := gob.NewDecoder(budget).Decode(&blob); err != nil {
		return nil, fmt.Errorf("rpc: crypto blob decode: %w", err)
	}
	plaintext, err := crypto.Decrypt(blob)
	if err != nil {
		return nil, fmt.Errorf("rpc: body decrypt: %w", err)
	}
	return plaintext, nil
}

// ErrInvalidMagic — на месте CRYPT-префикса оказалось что-то другое.
var ErrInvalidMagic = errors.New("rpc: invalid magic prefix")

// decodeCryptoBodySource — readCryptoBodySource + json.Unmarshal в body
// (body == nil → discard).
func decodeCryptoBodySource(src io.Reader, crypto *Crypto, maxBody int64, body any) error {
	plaintext, err := readCryptoBodySource(src, crypto, maxBody)
	if err != nil {
		return err
	}
	if body == nil {
		var raw json.RawMessage
		if err := json.Unmarshal(plaintext, &raw); err != nil {
			return fmt.Errorf("rpc: json body discard: %w", err)
		}
		return nil
	}
	if err := json.Unmarshal(plaintext, body); err != nil {
		return fmt.Errorf("rpc: json body unmarshal: %w", err)
	}
	return nil
}

// jsonCryptoServerCodec — серверный кодек crypto+JSON: clear JSON-заголовок
// (паритет с v2: заголовок не шифруется), тело под CRYPT-фреймингом с
// JSON-plaintext внутри (вместо gob из v2).
type jsonCryptoServerCodec struct {
	logger  logging.Logger
	crypto  *Crypto
	rwc     io.ReadWriteCloser
	budget  *budgetReader
	dec     *json.Decoder
	encBuf  *bufio.Writer
	enc     *json.Encoder
	maxBody int64
	closed  bool
}

func newJSONCryptoServerCodec(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte, maxBody int64) (rpc.ServerCodec, error) {
	crypto, err := newCrypto(key)
	if err != nil {
		return nil, err
	}
	if maxBody <= 0 {
		maxBody = DefaultMaxJSONBody
	}
	budget := &budgetReader{r: conn, left: maxBody}
	return &jsonCryptoServerCodec{
		logger:  logging.GetLogger("json-crypto-server-codec"),
		crypto:  crypto,
		rwc:     conn,
		budget:  budget,
		dec:     json.NewDecoder(budget),
		encBuf:  buf,
		enc:     json.NewEncoder(buf),
		maxBody: maxBody,
	}, nil
}

func (c *jsonCryptoServerCodec) ReadRequestHeader(r *rpc.Request) (err error) {
	c.budget.resetBudget(c.maxBody)
	if err = c.dec.Decode(r); err != nil && err != io.EOF {
		c.logger.Warning("request header decoding error: %s %T", err.Error(), err)
	}
	return err
}

func (c *jsonCryptoServerCodec) ReadRequestBody(body any) (err error) {
	c.budget.resetBudget(c.maxBody)
	// json.Decoder мог буферизовать начало (или всё) тела — читаем остаток
	// буфера первым и переустанавливаем декодер для следующего заголовка,
	// иначе следующий Decode повторно распарсит уже прочитанные байты.
	bufBytes, err := io.ReadAll(c.dec.Buffered())
	if err != nil {
		return err
	}
	bufSrc := bytes.NewReader(bufBytes)
	src := io.MultiReader(bufSrc, c.budget)
	decErr := decodeCryptoBodySource(src, c.crypto, c.maxBody, body)
	rest := bufBytes[len(bufBytes)-bufSrc.Len():]
	c.dec = json.NewDecoder(io.MultiReader(bytes.NewReader(rest), c.budget))
	if decErr != nil {
		c.logger.Warning("request body decoding error: %s", decErr)
	}
	return decErr
}

func (c *jsonCryptoServerCodec) WriteResponse(r *rpc.Response, body any) (err error) {
	if err = c.enc.Encode(r); err != nil {
		if c.encBuf.Flush() == nil {
			c.logger.Warning("rpc: json error encoding response: %s", err)
			_ = c.Close()
		}
		return err
	}
	if err = writeCryptoBody(c.encBuf, c.crypto, body); err != nil {
		if c.encBuf.Flush() == nil {
			c.logger.Warning("rpc: json crypto error encoding body: %s", err)
			_ = c.Close()
		}
		return err
	}
	return c.encBuf.Flush()
}

func (c *jsonCryptoServerCodec) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.rwc.Close()
}

var _ rpc.ServerCodec = (*jsonCryptoServerCodec)(nil)

// jsonCryptoClientCodec — клиентский кодек crypto+JSON, зеркало серверного.
type jsonCryptoClientCodec struct {
	logger  logging.Logger
	crypto  *Crypto
	rwc     io.ReadWriteCloser
	budget  *budgetReader
	dec     *json.Decoder
	encBuf  *bufio.Writer
	enc     *json.Encoder
	maxBody int64
	closed  bool
}

func newJSONCryptoClientCodec(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte, maxBody int64) (rpc.ClientCodec, error) {
	crypto, err := newCrypto(key)
	if err != nil {
		return nil, err
	}
	if maxBody <= 0 {
		maxBody = DefaultMaxJSONBody
	}
	budget := &budgetReader{r: conn, left: maxBody}
	return &jsonCryptoClientCodec{
		logger:  logging.GetLogger("json-crypto-client-codec"),
		crypto:  crypto,
		rwc:     conn,
		budget:  budget,
		dec:     json.NewDecoder(budget),
		encBuf:  buf,
		enc:     json.NewEncoder(buf),
		maxBody: maxBody,
	}, nil
}

func (c *jsonCryptoClientCodec) WriteRequest(r *rpc.Request, body any) (err error) {
	if err = c.enc.Encode(r); err != nil {
		return err
	}
	if err = writeCryptoBody(c.encBuf, c.crypto, body); err != nil {
		return err
	}
	return c.encBuf.Flush()
}

func (c *jsonCryptoClientCodec) ReadResponseHeader(r *rpc.Response) (err error) {
	c.budget.resetBudget(c.maxBody)
	if err = c.dec.Decode(r); err != nil && err != io.EOF {
		c.logger.Warning("response header decoding error: %s %T", err.Error(), err)
	}
	return err
}

func (c *jsonCryptoClientCodec) ReadResponseBody(body any) (err error) {
	c.budget.resetBudget(c.maxBody)
	// зеркало серверного ReadRequestBody: остаток буфера декодера первым,
	// затем переустановка декодера для следующего заголовка
	bufBytes, err := io.ReadAll(c.dec.Buffered())
	if err != nil {
		return err
	}
	bufSrc := bytes.NewReader(bufBytes)
	src := io.MultiReader(bufSrc, c.budget)
	decErr := decodeCryptoBodySource(src, c.crypto, c.maxBody, body)
	rest := bufBytes[len(bufBytes)-bufSrc.Len():]
	c.dec = json.NewDecoder(io.MultiReader(bytes.NewReader(rest), c.budget))
	if decErr != nil {
		c.logger.Warning("response body decoding error: %s", decErr)
	}
	return decErr
}

func (c *jsonCryptoClientCodec) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.rwc.Close()
}

var _ rpc.ClientCodec = (*jsonCryptoClientCodec)(nil)
