// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package codec

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"

	"go.slink.ws/logging"
	"go.slink.ws/rpc"
)

// DefaultMaxJSONBody — лимит размера одного JSON-сообщения (заголовок или тело).
// JSON-декодер не имеет встроенного лимита — без него возможен OOM на враждебном потоке.
const DefaultMaxJSONBody int64 = 4 << 20

// ErrBodyTooLarge возвращается, когда размер JSON-сообщения превышает лимит.
var ErrBodyTooLarge = errors.New("rpc: json message exceeds size limit")

// budgetReader ограничивает число байтов, которое декодер может прочитать
// из источника после последнего resetBudget. Лимит сбрасывается перед каждым
// Decode, поэтому лимит действует на одно сообщение, а не на поток целиком.
type budgetReader struct {
	r    io.Reader
	left int64
}

func (b *budgetReader) resetBudget(n int64) {
	b.left = n
}

func (b *budgetReader) Read(p []byte) (int, error) {
	if b.left <= 0 {
		return 0, ErrBodyTooLarge
	}
	if int64(len(p)) > b.left {
		p = p[:b.left]
	}
	n, err := b.r.Read(p)
	b.left -= int64(n)
	return n, err
}

// jsonServerCodec — серверный JSON-кодек rpc2/v3.
//
// На каждый вызов в потоке лежат два JSON-сообщения подряд: заголовок
// (rpc.Request) и тело (аргумент метода). Контракт go.slink.ws/rpc требует
// читать тело даже при ошибке (ReadRequestBody(nil) — discard), поэтому
// discard декодирует и отбрасывает следующее значение, сохраняя поток
// синхронизированным.
type jsonServerCodec struct {
	logger  logging.Logger
	rwc     io.ReadWriteCloser
	budget  *budgetReader
	dec     *json.Decoder
	encBuf  *bufio.Writer
	enc     *json.Encoder
	maxBody int64
	closed  bool
}

func newJSONServerCodec(buf *bufio.Writer, conn io.ReadWriteCloser, maxBody int64) rpc.ServerCodec {
	if maxBody <= 0 {
		maxBody = DefaultMaxJSONBody
	}
	budget := &budgetReader{r: conn, left: maxBody}
	return &jsonServerCodec{
		logger:  logging.GetLogger("json-server-codec"),
		rwc:     conn,
		budget:  budget,
		dec:     json.NewDecoder(budget),
		encBuf:  buf,
		enc:     json.NewEncoder(buf),
		maxBody: maxBody,
	}
}

func (c *jsonServerCodec) ReadRequestHeader(r *rpc.Request) (err error) {
	c.budget.resetBudget(c.maxBody)
	if err = c.dec.Decode(r); err != nil && !errors.Is(err, io.EOF) {
		c.logger.Warning("request header decoding error: %s %T", err.Error(), err)
	}
	return err
}

func (c *jsonServerCodec) ReadRequestBody(body any) (err error) {
	c.budget.resetBudget(c.maxBody)
	if body == nil {
		// discard: тело читается даже после ошибки заголовка (контракт rpc)
		var raw json.RawMessage
		if err = c.dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
			c.logger.Warning("request body discard error: %s", err)
		}
		return err
	}
	if err = c.dec.Decode(body); err != nil && !errors.Is(err, io.EOF) {
		c.logger.Warning("request body decoding error: %s", err)
	}
	return err
}

func (c *jsonServerCodec) WriteResponse(r *rpc.Response, body any) (err error) {
	if err = c.enc.Encode(r); err != nil {
		if c.encBuf.Flush() == nil {
			// заголовок закодировать не удалось — поток непригоден, закрываем
			c.logger.Warning("rpc: json error encoding response: %s", err)
			_ = c.Close()
		}
		return err
	}
	if err = c.enc.Encode(body); err != nil {
		if c.encBuf.Flush() == nil {
			// заголовок уже записан в буфер, тело закодировать не удалось
			c.logger.Warning("rpc: json error encoding body: %s", err)
			_ = c.Close()
		}
		return err
	}
	return c.encBuf.Flush()
}

func (c *jsonServerCodec) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.rwc.Close()
}

var _ rpc.ServerCodec = (*jsonServerCodec)(nil)
