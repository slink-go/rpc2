package codec

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"github.com/pkg/errors"
	"github.com/slink-go/logging"
	"io"
	"net/rpc"
)

type cryptoServerCodec struct {
	logger logging.Logger
	crypto *Crypto
	rwc    *BufferedReadWriteCloser
	w      *bufio.Writer
	dec    *gob.Decoder
	closed bool
}

func newCryptoServerCodec(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte) (rpc.ServerCodec, error) {
	rwc := newBufferedReadWriteCloser(conn)
	return &cryptoServerCodec{
		logger: logging.GetLogger("crypto-server-codec"),
		crypto: newCrypto(key),
		dec:    gob.NewDecoder(rwc),
		rwc:    rwc,
		w:      buf,
	}, nil
}

func (c *cryptoServerCodec) ReadRequestHeader(r *rpc.Request) (err error) {
	err = c.dec.Decode(r)
	if err != nil {
		c.logger.Debug("request header decoding error: %s [%#v]", err.Error(), r)
	}
	return err
}
func (c *cryptoServerCodec) ReadRequestBody(body any) (err error) {

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

	// decode body (to []byte)
	var decodedBody []byte
	err = c.dec.Decode(&decodedBody)
	if err != nil {
		return errors.Wrap(err, "request body decoding error")
	}

	// decrypt body bytes
	decryptedBody, err := c.crypto.Decrypt(decodedBody)
	if err != nil {
		return errors.Wrap(err, "request body decrypt error")
	}

	// decode decrypted bytes to real data
	err = gob.NewDecoder(bytes.NewBuffer(decryptedBody)).Decode(body)
	if err != nil {
		return errors.Wrap(err, "decrypted body decoding error")
	}

	return

}
func (c *cryptoServerCodec) WriteResponse(r *rpc.Response, body any) (err error) {

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
func (c *cryptoServerCodec) Close() error {
	if c.closed {
		// Only call c.rwc.Close once; otherwise the semantics are undefined.
		return nil
	}
	c.closed = true
	return c.rwc.Close()
}
