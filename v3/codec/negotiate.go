package codec

import (
	"bufio"
	"errors"
	"fmt"
	"io"
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
