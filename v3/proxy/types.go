// Package proxy — generic-jrpc-proxy: вызов произвольной rpc-функции системы
// по строке "Service.Method" с JSON-payload, без знания типов метода на стороне
// вызывающего. Целевой jRPC-сервис сам восстанавливает аргументы из JSON
// (reflection-диспетчер go.slink.ws/rpc + json-кодек rpc2/v3) — серверы
// переписывать не требуется.
package proxy

// Коды ошибок CallError.Code — контракт для вызывающих сторон.
const (
	// CodeBadRequest — некорректный запрос (нет метода, не тот формат).
	CodeBadRequest = "bad_request"
	// CodeRoute — сервис не найден в роутинге (неизвестный префикс).
	CodeRoute = "route_error"
	// CodeAllowlist — метод запрещён allowlist'ом прокси.
	CodeAllowlist = "allowlist_denied"
	// CodeBackend — целевой метод вернул ошибку (rpc.ServerError).
	CodeBackend = "backend_error"
	// CodeTransport — сетевая/транспортная ошибка или таймаут.
	CodeTransport = "transport_error"
	// CodePayload — превышен допустимый размер payload.
	CodePayload = "payload_too_large"
	// CodeNoJSON — бэкенд не поддерживает JSON-транспорт (gob-only и т.п.).
	CodeNoJSON = "backend_not_json"
)

// GenericRequest — входной аргумент метода proxy.Call.
type GenericRequest struct {
	// Method — полное имя вызова в формате "Service.Method".
	Method string `json:"method"`
	// Payload — тело вызова: JSON-аргументы целевого метода, байт-в-байт.
	Payload []byte `json:"payload,omitempty"`
	// Metadata — входные метаданные; наружу уходит только whitelist-набор
	// заголовков (см. invoke.go).
	Metadata map[string]string `json:"metadata,omitempty"`
}

// GenericResponse — ответ proxy.Call.
type GenericResponse struct {
	// Payload — JSON-ответ целевого метода, байт-в-байт.
	Payload []byte `json:"payload,omitempty"`
	// Error — структурированная ошибка; nil при успехе.
	Error *CallError `json:"error,omitempty"`
}

// CallError — структурированная ошибка вызова (коды — Code* выше).
type CallError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Target  string `json:"target,omitempty"`
}

// Target — целевой rpc-сервис для вызова (секция роутинга).
type Target struct {
	// Addr — хост целевого сервиса.
	Addr string
	// Port — порт целевого сервиса.
	Port int
	// Secret — ключ шифрования AES-GCM; пусто — без крипты.
	Secret []byte
	// Transport — транспорт клиента: "" (auto) | "json" | "legacy".
	Transport string
}
