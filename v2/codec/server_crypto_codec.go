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

type cryptoServerCodec struct {
	logger logging.Logger
	crypto *Crypto
	rwc    *BufferedReadWriteCloser
	w      *bufio.Writer
	closed bool
}

func newCryptoServerCodec(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte) (rpc.ServerCodec, error) {
	crypto, err := newCrypto(key)
	if err != nil {
		return nil, err
	}
	rwc := newBufferedReadWriteCloser(conn)
	return &cryptoServerCodec{
		logger: logging.GetLogger("crypto-server-codec"),
		crypto: crypto,
		rwc:    rwc,
		w:      buf,
	}, nil
}

func (c *cryptoServerCodec) ReadRequestHeader(r *rpc.Request) (err error) {
	err = gob.NewDecoder(c.rwc).Decode(r)
	if err != nil && err != io.EOF {
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
	err = gob.NewDecoder(c.rwc).Decode(&decodedBody)
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
	if err = c.writeResponse(r, body); err != nil {
		c.logger.Warning("rpc: response encoding error: %s", err.Error())
		_ = c.Close()
	}
	return err
}

func (c *cryptoServerCodec) writeResponse(r *rpc.Response, body any) (err error) {
	// кодируем body с помощью gob
	var bodyBuffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&bodyBuffer)
	if err = gobEncoder.Encode(body); err != nil {
		return
	}

	// шифруем body
	encryptedBody := c.crypto.Encrypt(bodyBuffer.Bytes())

	// кодируем заголовок с помощью gob
	var header bytes.Buffer
	gobEncoder = gob.NewEncoder(&header)
	if err = gobEncoder.Encode(r); err != nil {
		return
	}

	// кодируем сообщение с помощью gob
	var buffer bytes.Buffer
	gobEncoder = gob.NewEncoder(&buffer)
	if err = gobEncoder.Encode(encryptedBody); err != nil {
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
