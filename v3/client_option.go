package rpc2

import (
	"context"

	"go.slink.ws/logging"
)

// ClientOption — опция клиента rpc2/v3.
type ClientOption func(*CustomRpcClient)

// ClientWithCryptoKey задаёт ключ шифрования (AES-GCM).
func ClientWithCryptoKey(value []byte) ClientOption {
	return func(s *CustomRpcClient) {
		s.cryptoKey = value
	}
}

// ClientWithAddress задаёт адрес сервера.
func ClientWithAddress(value string) ClientOption {
	return func(s *CustomRpcClient) {
		s.address = value
	}
}

// ClientWithPort задаёт порт сервера.
func ClientWithPort(value int) ClientOption {
	return func(s *CustomRpcClient) {
		s.port = value
	}
}

// ClientWithLogger задаёт логгер.
func ClientWithLogger(value logging.Logger) ClientOption {
	return func(s *CustomRpcClient) {
		s.logger = value
	}
}

// ClientWithTransport фиксирует транспорт: "json" | "legacy" | "" (auto).
// auto — JSON с однократным sticky-фолбэком в gob при conn-ошибке.
func ClientWithTransport(value string) ClientOption {
	return func(c *CustomRpcClient) {
		switch value {
		case "json":
			c.transport = transportJSON
		case "legacy":
			c.transport = transportLegacy
		default:
			c.transport = transportAuto
		}
	}
}

// ClientWithMaxJSONBody ограничивает размер JSON-сообщений (≤0 — 4MB).
func ClientWithMaxJSONBody(value int64) ClientOption {
	return func(c *CustomRpcClient) {
		c.maxJSONBody = value
	}
}

// ClientContextMiddleware — контекстная middleware перед каждым вызовом.
type ClientContextMiddleware func(context.Context) context.Context

// ClientWithMiddleware добавляет контекстную middleware.
func ClientWithMiddleware(function ClientContextMiddleware) ClientOption {
	return func(c *CustomRpcClient) {
		c.middlewares = append(c.middlewares, function)
	}
}
