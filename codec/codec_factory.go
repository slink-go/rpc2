package codec

import (
	"bufio"
	"github.com/renevo/rpc"
	"go.slink.ws/logging"
	"io"
)

func GetServerCodec(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte) rpc.ServerCodec {
	if len(key) > 0 {
		codec, err := newCryptoServerCodec(buf, conn, key)
		if err != nil {
			panic(err)
		}
		logging.GetLogger("codec").Trace("use crypto server codec")
		return codec
	}
	logging.GetLogger("codec").Trace("use open server codec")
	return newGobServerCodec(buf, conn)
}

func GetClientCodec(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte) rpc.ClientCodec {
	if len(key) > 0 {
		codec, err := newCryptoClientCodec(buf, conn, key)
		if err != nil {
			panic(err)
		}
		logging.GetLogger("codec").Trace("use crypto client codec")
		return codec
	}
	logging.GetLogger("codec").Trace("use open client codec")
	return newGobClientCodec(buf, conn)
}
