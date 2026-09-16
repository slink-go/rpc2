package codec

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"github.com/pkg/errors"
	"go.slink.ws/logging"
	"go.slink.ws/rpc"
	"io"
)

type cryptoClientCodec struct {
	logger logging.Logger
	crypto *Crypto
	rwc    *BufferedReadWriteCloser
	w      *bufio.Writer
	enc    *gob.Encoder
	dec    *gob.Decoder
	closed bool
}

func newCryptoClientCodec(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte) (rpc.ClientCodec, error) {
	crypto, err := newCrypto(key)
	if err != nil {
		return nil, err
	}
	rwc := newBufferedReadWriteCloser(conn)
	return &cryptoClientCodec{
		logger: logging.GetLogger("crypto-client-codec"),
		crypto: crypto,
		rwc:    rwc,
		w:      buf,
		enc:    gob.NewEncoder(buf),
		dec:    gob.NewDecoder(rwc),
		closed: false,
	}, nil
}

func (c *cryptoClientCodec) WriteRequest(r *rpc.Request, body any) (err error) {

	// кодируем body с помощью gob (внутренний слой — свой поток)
	var bodyBuffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&bodyBuffer)
	if err = gobEncoder.Encode(body); err != nil {
		c.logger.Warning("rpc: gob body encoding error: %s", err.Error())
		return
	}

	// шифруем body
	encryptedBody := c.crypto.Encrypt(bodyBuffer.Bytes())

	// внешняя пара кодеков: заголовок и шифр-тело пишутся постоянным энкодером,
	// между сообщениями — magic-префикс (энкодер пишет ровно одно сообщение на Encode)
	if err = c.enc.Encode(r); err != nil {
		c.logger.Warning("rpc: gob header encoding error: %s", err.Error())
		return
	}
	if _, err = c.w.Write(magicPrefix); err != nil {
		return err
	}
	if err = c.enc.Encode(encryptedBody); err != nil {
		c.logger.Warning("rpc: gob body encoding error: %s", err.Error())
		return
	}
	return c.w.Flush()

}
func (c *cryptoClientCodec) ReadResponseHeader(r *rpc.Response) (err error) {
	err = c.dec.Decode(r)
	if err != nil && err != io.EOF {
		c.logger.Warning("response header decoding error: %s [%#v]", err.Error(), r)
	}
	return err
}
func (c *cryptoClientCodec) ReadResponseBody(body any) (err error) {

	// check magic number prefix (for encrypted data it should be ['C', 'R', 'Y', 'P', 'T'])
	var magicRead []byte
	magicRead, err = c.rwc.Peek(len(magicPrefix))
	if err != nil {
		return errors.Wrap(err, "prefix peeking error")
	}
	if !slicesEqual(magicRead, magicPrefix) {
		return errors.Wrap(InvalidPrefixErr, "invalid magic number")
	}

	// skip magic number prefix
	_, err = c.rwc.Discard(len(magicPrefix))
	if err != nil {
		return errors.Wrap(err, "prefix discarding error")
	}

	// decode body to encrypted byte array
	var decodedBody []byte
	err = c.dec.Decode(&decodedBody)
	if err != nil {
		return errors.Wrap(err, "response body decoding error")
	}

	// decrypt body bytes
	decryptedBody, err := c.crypto.Decrypt(decodedBody)
	if err != nil {
		return errors.Wrap(err, "response body decrypt error")
	}

	// decode decrypted bytes to real data
	err = gob.NewDecoder(bytes.NewBuffer(decryptedBody)).Decode(body)
	if err != nil {
		return errors.Wrap(err, "decrypted body decoding error")
	}

	return

}
func (c *cryptoClientCodec) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.rwc.Close()
}
