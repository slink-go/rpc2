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

// jsonClientCodec — клиентский JSON-кодек rpc2/v3, симметричен jsonServerCodec:
// два JSON-сообщения на вызов (заголовок rpc.Request + тело), лимит размера
// на каждое сообщение, discard тела при ReadResponseBody(nil).
type jsonClientCodec struct {
	logger  logging.Logger
	rwc     io.ReadWriteCloser
	budget  *budgetReader
	dec     *json.Decoder
	encBuf  *bufio.Writer
	enc     *json.Encoder
	maxBody int64
	closed  bool
}

func newJSONClientCodec(buf *bufio.Writer, conn io.ReadWriteCloser, maxBody int64) rpc.ClientCodec {
	if maxBody <= 0 {
		maxBody = DefaultMaxJSONBody
	}
	budget := &budgetReader{r: conn, left: maxBody}
	return &jsonClientCodec{
		logger:  logging.GetLogger("json-client-codec"),
		rwc:     conn,
		budget:  budget,
		dec:     json.NewDecoder(budget),
		encBuf:  buf,
		enc:     json.NewEncoder(buf),
		maxBody: maxBody,
	}
}

func (c *jsonClientCodec) WriteRequest(r *rpc.Request, body any) (err error) {
	if err = c.enc.Encode(r); err != nil {
		return err
	}
	if err = c.enc.Encode(body); err != nil {
		return err
	}
	return c.encBuf.Flush()
}

func (c *jsonClientCodec) ReadResponseHeader(r *rpc.Response) (err error) {
	c.budget.resetBudget(c.maxBody)
	if err = c.dec.Decode(r); err != nil && !errors.Is(err, io.EOF) {
		c.logger.Warning("response header decoding error: %s %T", err.Error(), err)
	}
	return err
}

func (c *jsonClientCodec) ReadResponseBody(body any) (err error) {
	c.budget.resetBudget(c.maxBody)
	if body == nil {
		// discard: тело читается и отбрасывается, чтобы поток остался синхронным
		var raw json.RawMessage
		if err = c.dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
			c.logger.Warning("response body discard error: %s", err)
		}
		return err
	}
	if err = c.dec.Decode(body); err != nil && !errors.Is(err, io.EOF) {
		c.logger.Warning("response body decoding error: %s", err)
	}
	return err
}

func (c *jsonClientCodec) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.rwc.Close()
}

var _ rpc.ClientCodec = (*jsonClientCodec)(nil)
