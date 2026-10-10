package proxy

import (
	"fmt"
	"sync"
)

// StaticRouter — статическая таблица маршрутов «имя сервиса → Target».
// Runtime-перезагрузка на первом шаге не требуется (см. план T2.2).
type StaticRouter struct {
	mu      sync.RWMutex
	targets map[string]Target
}

// NewStaticRouter собирает роутер из таблицы целей и валидирует её:
// addr обязателен, порт — 0 (дефолт) или 1..65535, transport ∈ {"",auto,json,legacy}.
func NewStaticRouter(targets map[string]Target) (*StaticRouter, error) {
	validated := make(map[string]Target, len(targets))
	for name, tgt := range targets {
		if name == "" {
			return nil, fmt.Errorf("proxy: empty service name in routing table")
		}
		if tgt.Addr == "" {
			return nil, fmt.Errorf("proxy: target for %q has empty addr", name)
		}
		if tgt.Port < 0 || tgt.Port > 65535 {
			return nil, fmt.Errorf("proxy: target %q has invalid port %d", name, tgt.Port)
		}
		switch tgt.Transport {
		case "", "auto", "json", "legacy":
		default:
			return nil, fmt.Errorf("proxy: target %q has invalid transport %q", name, tgt.Transport)
		}
		validated[name] = tgt
	}
	return &StaticRouter{targets: validated}, nil
}

// Resolve возвращает целевой адрес сервиса или ошибку маршрута (CodeRoute).
func (r *StaticRouter) Resolve(service string) (Target, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tgt, ok := r.targets[service]
	if !ok {
		return Target{}, fmt.Errorf("proxy: no route for service %q", service)
	}
	return tgt, nil
}

// Targets возвращает копию таблицы маршрутов (для диагностики/логов).
func (r *StaticRouter) Targets() map[string]Target {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]Target, len(r.targets))
	for k, v := range r.targets {
		out[k] = v
	}
	return out
}
