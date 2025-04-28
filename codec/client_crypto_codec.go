package codec

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"github.com/pkg/errors"
	"github.com/renevo/rpc"
	"go.slink.ws/logging"
	"io"
)

type cryptoClientCodec struct {
	logger logging.Logger
	crypto *Crypto
	rwc    *BufferedReadWriteCloser
	dec    *gob.Decoder
	w      *bufio.Writer
	closed bool
}

func newCryptoClientCodec(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte) (rpc.ClientCodec, error) {
	rwc := newBufferedReadWriteCloser(conn)
	return &cryptoClientCodec{
		logger: logging.GetLogger("crypto-client-codec"),
		crypto: newCrypto(key),
		rwc:    rwc,
		dec:    gob.NewDecoder(rwc),
		w:      buf,
	}, nil
}

func (c *cryptoClientCodec) WriteRequest(r *rpc.Request, body any) (err error) {

	// кодируем body с помощью gob
	var bodyBuffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&bodyBuffer)
	if err = gobEncoder.Encode(body); err != nil {
		c.logger.Warning("rpc: gob body encoding error: %s", err.Error())
		return
	}

	// шифруем body
	encryptedBody := c.crypto.Encrypt(bodyBuffer.Bytes())

	// кодируем заголовок с помощью gob
	var header bytes.Buffer
	gobEncoder = gob.NewEncoder(&header)
	if err = gobEncoder.Encode(r); err != nil {
		c.logger.Warning("rpc: gob header encoding error: %s", err.Error())
		return
	}

	// кодируем сообщение с помощью gob
	var buffer bytes.Buffer
	gobEncoder = gob.NewEncoder(&buffer)
	if err = gobEncoder.Encode(encryptedBody); err != nil {
		c.logger.Warning("rpc: gob body encoding error: %s", err.Error())
		return
	}

	if _, err = c.w.Write(header.Bytes()); err != nil {
		return err
	}
	if _, err = c.w.Write(magicPrefix); err != nil {
		return err
	}
	if _, err = c.w.Write(buffer.Bytes()); err != nil {
		return err
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
	return c.rwc.Close()
}
