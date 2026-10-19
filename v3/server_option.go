// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package rpc2

import (
	"go.slink.ws/logging"
	"go.slink.ws/rpc"
)

// ServerOption — опция сервера rpc2/v3.
type ServerOption func(*CustomRpcServer)

// ServerWithCryptoKey включает crypto-режим (AES-GCM): legacy-путь — gob с
// шифрованием тела, JSON-путь — clear-заголовок + CRYPT-фрейминг.
func ServerWithCryptoKey(value []byte) ServerOption {
	return func(s *CustomRpcServer) {
		s.cryptoKey = value
	}
}

// ServerWithAddress задаёт bind-адрес.
func ServerWithAddress(value string) ServerOption {
	return func(s *CustomRpcServer) {
		s.address = value
	}
}

// ServerWithPort задаёт порт.
func ServerWithPort(value int) ServerOption {
	return func(s *CustomRpcServer) {
		s.port = value
	}
}

// ServerWithHandler задаёт обработчик соединений.
func ServerWithHandler(value ServerHandler) ServerOption {
	return func(s *CustomRpcServer) {
		s.handler = value
	}
}

// ServerWithLogger задаёт логгер.
func ServerWithLogger(value logging.Logger) ServerOption {
	return func(s *CustomRpcServer) {
		s.logger = value
	}
}

// ServerWithMiddleware добавляет middleware на rpc.Server.
func ServerWithMiddleware(value rpc.MiddlewareFunc) ServerOption {
	return func(s *CustomRpcServer) {
		s.svr.Use(value)
	}
}

// ServerWithMaxJSONBody ограничивает размер одного JSON-сообщения
// (заголовок/тело); ≤0 — DefaultMaxJSONBody (4MB).
func ServerWithMaxJSONBody(value int64) ServerOption {
	return func(s *CustomRpcServer) {
		s.maxJSONBody = value
	}
}
