package codec

import (
	"bufio"
	"go.slink.ws/logging"
	"go.slink.ws/rpc"
	"io"
)

// NewServerCodec возвращает серверный кодек (crypto — при непустом ключе, иначе открытый gob).
// В отличие от GetServerCodec не паникует при ошибке создания.
func NewServerCodec(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte) (rpc.ServerCodec, error) {
	if len(key) > 0 {
		cdc, err := newCryptoServerCodec(buf, conn, key)
		if err != nil {
			return nil, err
		}
		logging.GetLogger("codec").Trace("use crypto server codec")
		return cdc, nil
	}
	logging.GetLogger("codec").Trace("use open server codec")
	return newGobServerCodec(buf, conn), nil
}

// NewClientCodec возвращает клиентский кодек (crypto — при непустом ключе, иначе открытый gob).
// В отличие от GetClientCodec не паникует при ошибке создания.
func NewClientCodec(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte) (rpc.ClientCodec, error) {
	if len(key) > 0 {
		cdc, err := newCryptoClientCodec(buf, conn, key)
		if err != nil {
			return nil, err
		}
		logging.GetLogger("codec").Trace("use crypto client codec")
		return cdc, nil
	}
	logging.GetLogger("codec").Trace("use open client codec")
	return newGobClientCodec(buf, conn), nil
}

// Deprecated: используйте NewServerCodec. Сохранён для обратной совместимости.
func GetServerCodec(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte) rpc.ServerCodec {
	cdc, err := NewServerCodec(buf, conn, key)
	if err != nil {
		panic(err)
	}
	return cdc
}

// Deprecated: используйте NewClientCodec. Сохранён для обратной совместимости.
func GetClientCodec(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte) rpc.ClientCodec {
	cdc, err := NewClientCodec(buf, conn, key)
	if err != nil {
		panic(err)
	}
	return cdc
}
