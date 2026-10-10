package proxy

import (
	"context"
	"fmt"
	"strings"
)

// ServiceName — имя сервиса для RegisterName: вызывается как "proxy.Call".
const ServiceName = "proxy"

// Router резолвит имя сервиса (часть Method до точки) в целевой адрес.
type Router interface {
	Resolve(service string) (Target, error)
}

// Allowlister разрешает или запрещает вызов метода (nil = default-allow,
// см. spec/mgmt/rpc2_proxy_decisions.md).
type Allowlister interface {
	// Allowed возвращает nil, если вызов разрешён, иначе — ошибку
	// с описанием отказа (её текст уходит в CallError.Message).
	Allowed(method string) error
}

// Invoker выполняет вызов целевого сервиса: возвращает payload ответа
// либо структурированную ошибку (никогда оба сразу).
type Invoker interface {
	Invoke(ctx context.Context, target Target, method string, payload []byte, md map[string]string) ([]byte, *CallError)
}

// Service — generic-jrpc-proxy. Регистрируется как обычный rpc2-сервис:
//
//	srv.RegisterName(proxy.ServiceName, svc) → метод "proxy.Call".
type Service struct {
	router  Router
	allow   Allowlister
	invoker Invoker
}

// New собирает Service с явным внедрением зависимостей.
// allow может быть nil — в этом случае действует default-allow.
func New(router Router, allow Allowlister, invoker Invoker) *Service {
	return &Service{router: router, allow: allow, invoker: invoker}
}

// Call — контракт generic-proxy.
//
// Контракт ошибок: все сбои вызова (валидация, роутинг, allowlist, бэкенд,
// транспорт) попадают в структурированный res.Error, а возвращаемое error —
// только неустранимая ошибка программирования (nil res / неполная сборка),
// чтобы вызывающая сторона обрабатывала ровно один канал ошибки.
func (s *Service) Call(ctx context.Context, req GenericRequest, res *GenericResponse) error {
	if res == nil {
		return fmt.Errorf("proxy: nil response argument")
	}
	if s.router == nil || s.invoker == nil {
		return fmt.Errorf("proxy: service is not fully configured")
	}

	service, ok := SplitMethod(req.Method)
	if !ok {
		res.Error = &CallError{
			Code:    CodeBadRequest,
			Message: fmt.Sprintf("method must be \"Service.Method\", got %q", req.Method),
		}
		return nil
	}

	if s.allow != nil {
		if err := s.allow.Allowed(req.Method); err != nil {
			res.Error = &CallError{Code: CodeAllowlist, Message: err.Error()}
			return nil
		}
	}

	target, err := s.router.Resolve(service)
	if err != nil {
		res.Error = &CallError{Code: CodeRoute, Message: err.Error()}
		return nil
	}

	payload, callErr := s.invoker.Invoke(ctx, target, req.Method, req.Payload, req.Metadata)
	if callErr != nil {
		res.Error = callErr
		return nil
	}

	res.Payload = payload
	return nil
}

// SplitMethod разделяет "Service.Method" на имя сервиса.
// ok=false, если метод пуст, не содержит точки или точка крайняя.
func SplitMethod(method string) (service string, ok bool) {
	i := strings.IndexByte(method, '.')
	if i <= 0 || i == len(method)-1 {
		return "", false
	}
	return method[:i], true
}
