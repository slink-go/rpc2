// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package proxy

import (
	"context"
	"errors"
	"io"

	"go.slink.ws/rpc"
)

// mapCallError классифицирует ошибку вызова целевого сервиса:
//   - rpc.ServerError — структурированная ошибка бэкенда → CodeBackend;
//   - EOF-семейство (бэкенд закрыл соединение на JSON-запросе) → CodeNoJSON —
//     типичный признак сервиса, не поддерживающего JSON-транспорт (gob-only);
//   - дедлайн/отмена контекста → CodeTransport (таймаут);
//   - прочие транспортные сбои → CodeTransport.
func mapCallError(err error, target string) *CallError {
	var srvErr rpc.ServerError
	if errors.As(err, &srvErr) {
		return &CallError{Code: CodeBackend, Message: srvErr.Error(), Target: target}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe) {
		return &CallError{
			Code:    CodeNoJSON,
			Message: "proxy: target closed connection; backend likely does not support JSON transport (gob-only?): " + err.Error(),
			Target:  target,
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &CallError{Code: CodeTransport, Message: "proxy: call timeout: " + err.Error(), Target: target}
	}
	if errors.Is(err, context.Canceled) {
		return &CallError{Code: CodeTransport, Message: "proxy: call canceled: " + err.Error(), Target: target}
	}
	return &CallError{Code: CodeTransport, Message: err.Error(), Target: target}
}
