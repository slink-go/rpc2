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
//
// Эмпирика (Go 1.27, gob): первый байт свежего gob-потока типовых заголовков
// — 0x39 (Request), 0x45 (Response), 0x16 (прочие struct), т.е. < 0x80;
// формальный инвариант «коллизии нет» — типовой gob-поток не начинается с
// «{». Защита от экзотической коллизии (type-id даёт байт 0x7B): проверяется
// и второй байт — у JSON это кавычка/пробельный, gob там валидным JSON не
// бывает.
const jsonFirstByte = '{'

// isJSONStream — начинается ли поток с валидного JSON-объекта.
func isJSONStream(b []byte) bool {
	if len(b) == 0 || b[0] != jsonFirstByte {
		return false
	}
	if len(b) < 2 {
		// обрезанный поток: решит JSON-декодер
		return true
	}
	switch b[1] {
	case '"', ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

// prefixConn — соединение с replay'ем: чтение идёт через bufio.Reader
// (прочитанный sniff'ом байт остаётся в буфере и возвращается первому
// декодеру), запись — напрямую в conn.
type prefixConn struct {
	*bufio.Reader
	io.WriteCloser
}

// sniffServerConn определяет транспорт соединения по первым двум байтам:
// JSON-объект → JSON-кодек, иначе — legacy-путь (gob/crypto), байт-в-байт
// как в v2. Всегда возвращает соединение с replay'ем прочитанных байтов.
func sniffServerConn(conn io.ReadWriteCloser) (io.ReadWriteCloser, bool, error) {
	br := bufio.NewReader(conn)
	first, err := br.Peek(2)
	if len(first) == 0 {
		if errors.Is(err, io.EOF) {
			// пустое соединение: отдаём legacy-пути, там EOF завершит цикл
			return &prefixConn{Reader: br, WriteCloser: conn}, false, nil
		}
		return nil, false, fmt.Errorf("rpc: sniff peek: %w", err)
	}
	return &prefixConn{Reader: br, WriteCloser: conn}, isJSONStream(first), nil
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
