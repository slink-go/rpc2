package codec

import (
	"bufio"
	"encoding/gob"
	"go.slink.ws/logging"
	"go.slink.ws/rpc"
	"io"
)

// копипаста из net/rpc

type gobClientCodec struct {
	logger logging.Logger
	rwc    io.ReadWriteCloser
	dec    *gob.Decoder
	enc    *gob.Encoder
	encBuf *bufio.Writer
}

func newGobClientCodec(buf *bufio.Writer, conn io.ReadWriteCloser) rpc.ClientCodec {
	return &gobClientCodec{
		logger: logging.GetLogger("gob-client-codec"),
		rwc:    conn,
		dec:    gob.NewDecoder(conn),
		enc:    gob.NewEncoder(buf),
		encBuf: buf,
	}
}

func (c *gobClientCodec) WriteRequest(r *rpc.Request, body any) (err error) {
	if err = c.enc.Encode(r); err != nil {
		return
	}
	if err = c.enc.Encode(body); err != nil {
		return
	}
	return c.encBuf.Flush()
}
func (c *gobClientCodec) ReadResponseHeader(r *rpc.Response) error {
	return c.dec.Decode(r)
}
func (c *gobClientCodec) ReadResponseBody(body any) error {
	return c.dec.Decode(body)
}
func (c *gobClientCodec) Close() error {
	return c.rwc.Close()
}
