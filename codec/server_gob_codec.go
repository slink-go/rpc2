package codec

import (
	"bufio"
	"encoding/gob"
	"go.slink.ws/logging"
	"io"
	"net/rpc"
)

/*

type ServerCodec interface {
	ReadRequestHeader(*Request) error
	ReadRequestBody(any) error
	WriteResponse(*Response, any) error
	// Close can be called multiple times and must be idempotent.
	Close() error
}
*/

// копипаста из net/rpc

type gobServerCodec struct {
	logger logging.Logger
	rwc    io.ReadWriteCloser
	dec    *gob.Decoder
	enc    *gob.Encoder
	encBuf *bufio.Writer
	closed bool
}

func newGobServerCodec(buf *bufio.Writer, conn io.ReadWriteCloser) rpc.ServerCodec {
	return &gobServerCodec{
		logger: logging.GetLogger("gob-server-codec"),
		rwc:    conn,
		dec:    gob.NewDecoder(conn),
		enc:    gob.NewEncoder(buf),
		encBuf: buf,
	}
}
func (c *gobServerCodec) ReadRequestHeader(r *rpc.Request) (err error) {
	err = c.dec.Decode(r)
	if err != nil && err != io.EOF {
		c.logger.Warning("request header decoding error: %s %T", err.Error(), err)
	}
	return
}
func (c *gobServerCodec) ReadRequestBody(body any) (err error) {
	err = c.dec.Decode(body)
	if err != nil && err != io.EOF {
		c.logger.Warning("request body decoding error: %s", err.Error())
	}
	return
}
func (c *gobServerCodec) WriteResponse(r *rpc.Response, body any) (err error) {
	if err = c.enc.Encode(r); err != nil {
		if c.encBuf.Flush() == nil {
			// Gob couldn't encode the header. Should not happen, so if it does,
			// shut down the connection to signal that the connection is broken.
			c.logger.Warning("rpc: gob error encoding response: %s", err)
			_ = c.Close()
		}
		return
	}
	if err = c.enc.Encode(body); err != nil {
		if c.encBuf.Flush() == nil {
			// Was a gob problem encoding the body but the header has been written.
			// Shut down the connection to signal that the connection is broken.
			c.logger.Warning("rpc: gob error encoding body: %s", err)
			_ = c.Close()
		}
		return
	}
	return c.encBuf.Flush()
}
func (c *gobServerCodec) Close() error {
	if c.closed {
		// Only call c.rwc.Close once; otherwise the semantics are undefined.
		return nil
	}
	c.closed = true
	return c.rwc.Close()
}
