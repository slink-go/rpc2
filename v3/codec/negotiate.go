package codec

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"go.slink.ws/rpc"
	v2codec "go.slink.ws/rpc2/v2/codec"
)

// jsonFirstByte — первый байт JSON-заголовка ('{', 0x7B).
// gob-поток всегда начинается с определения типа: отрицательный type-id
// кодируется байтами ≥ 0x80, поэтому коллизия sniff'а невозможна.
const jsonFirstByte = '{'

// prefixConn — соединение с replay'ем: чтение идёт через bufio.Reader
// (прочитанный sniff'ом байт остаётся в буфере и возвращается первому
// декодеру), запись — напрямую в conn.
type prefixConn struct {
	*bufio.Reader
	io.WriteCloser
}

// sniffServerConn определяет транспорт соединения по первому байту:
// '{' → JSON-кодек, иначе — legacy-путь (gob/crypto), байт-в-байт как в v2.
// Всегда возвращает соединение с replay'ем уже прочитанных байтов.
func sniffServerConn(conn io.ReadWriteCloser) (io.ReadWriteCloser, bool, error) {
	br := bufio.NewReader(conn)
	first, err := br.Peek(1)
	if err != nil {
		if errors.Is(err, io.EOF) {
			// пустое соединение: отдаём legacy-пути, там EOF завершит цикл
			return &prefixConn{Reader: br, WriteCloser: conn}, false, nil
		}
		return nil, false, fmt.Errorf("rpc: sniff peek: %w", err)
	}
	return &prefixConn{Reader: br, WriteCloser: conn}, first[0] == jsonFirstByte, nil
}

// NewServerCodecV3 — серверная фабрика кодеков rpc2/v3: sniff первого байта,
// затем выбор JSON (crypto при непустом ключе) или legacy-путь v2
// (gob/crypto) байт-в-байт, без изменений.
func NewServerCodecV3(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte, maxBody int64) (rpc.ServerCodec, error) {
	sniffed, isJSON, err := sniffServerConn(conn)
	if err != nil {
		return nil, err
	}
	if !isJSON {
		return v2codec.NewServerCodec(buf, sniffed, key)
	}
	if len(key) > 0 {
		return newJSONCryptoServerCodec(buf, sniffed, key, maxBody)
	}
	return newJSONServerCodec(buf, sniffed, maxBody), nil
}

// NewClientCodecV3 — клиентская фабрика кодеков rpc2/v3: транспорт выбирает
// вызывающий (json — этот кодек, legacy — gob/crypto как в v2).
func NewClientCodecV3(buf *bufio.Writer, conn io.ReadWriteCloser, key []byte, maxBody int64, jsonTransport bool) (rpc.ClientCodec, error) {
	if !jsonTransport {
		return v2codec.NewClientCodec(buf, conn, key)
	}
	if len(key) > 0 {
		return newJSONCryptoClientCodec(buf, conn, key, maxBody)
	}
	return newJSONClientCodec(buf, conn, maxBody), nil
}
