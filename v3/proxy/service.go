// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package proxy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.slink.ws/logging"
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
	logger  logging.Logger
	metrics Metrics
}

// Option — опция Service (логгер, метрики).
type Option func(*Service)

// WithLogger задаёт логгер (nil игнорируется — остаётся дефолтный).
func WithLogger(value logging.Logger) Option {
	return func(s *Service) {
		if value != nil {
			s.logger = value
		}
	}
}

// WithMetrics задаёт реализацию Metrics (nil игнорируется — остаётся
// no-op).
func WithMetrics(value Metrics) Option {
	return func(s *Service) {
		if value != nil {
			s.metrics = value
		}
	}
}

// New собирает Service с явным внедрением зависимостей.
// allow может быть nil — в этом случае действует default-allow.
func New(router Router, allow Allowlister, invoker Invoker, opts ...Option) *Service {
	s := &Service{
		router:  router,
		allow:   allow,
		invoker: invoker,
		logger:  logging.GetLogger("proxy-service"),
		metrics: noopMetrics{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
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

	start := time.Now()
	targetStr := ""
	reqBytes := len(req.Payload)
	defer func() {
		s.observe(req, res, targetStr, reqBytes, time.Since(start))
	}()

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
	targetStr = targetString(target)

	payload, callErr := s.invoker.Invoke(ctx, target, req.Method, req.Payload, req.Metadata)
	if callErr != nil {
		res.Error = callErr
		return nil
	}

	res.Payload = payload
	return nil
}

// observe фиксирует итог вызова: лог (Warning при ошибке, Debug при
// успехе) и метрика. Параметры идут из уже заполненного res.
func (s *Service) observe(req GenericRequest, res *GenericResponse, targetStr string, reqBytes int, latency time.Duration) {
	stats := CallStats{
		Method:   req.Method,
		Target:   targetStr,
		Latency:  latency,
		ReqBytes: reqBytes,
		ResBytes: len(res.Payload),
	}
	if res.Error != nil {
		stats.Outcome = OutcomeError
		stats.Code = res.Error.Code
		if stats.Target == "" {
			stats.Target = res.Error.Target
		}
		s.logger.Warning(
			"proxy call failed: method=%s target=%s code=%s latency_ms=%d req_bytes=%d err=%s",
			req.Method, stats.Target, stats.Code, latency.Milliseconds(), reqBytes, res.Error.Message,
		)
	} else {
		stats.Outcome = OutcomeOK
		s.logger.Debug(
			"proxy call ok: method=%s target=%s latency_ms=%d req_bytes=%d res_bytes=%d",
			req.Method, stats.Target, latency.Milliseconds(), reqBytes, stats.ResBytes,
		)
	}
	s.metrics.ObserveCall(stats)
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
