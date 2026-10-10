package proxy

import (
	"fmt"
	"strings"
)

// AllowlistPolicy — политика allowlist (решение: spec/mgmt/rpc2_proxy_decisions.md).
// Смена дефолтного поведения прокси — правка константы/кейса здесь,
// без изменений остального кода.
type AllowlistPolicy int

const (
	// PolicyDefault — принятая политика: allowlist отсутствует **или пуст** →
	// пропускается любой метод; задан непустой список → разрешены только его
	// записи (wildcard "Service.*"), остальное — отказ.
	PolicyDefault AllowlistPolicy = iota
	// PolicyDenyEmpty — переключатель для будущего пересмотра: ключевание
	// задано (в т.ч. пустым списком) → включается фильтрация, пустой список
	// → отказ всего. Сейчас не применяется (см. док решений).
	PolicyDenyEmpty
)

// Allowlist — allowlist методов generic-proxy.
// nil-значение и пустой список при PolicyDefault = default-allow.
type Allowlist struct {
	policy   AllowlistPolicy
	exact    map[string]struct{}
	services map[string]struct{}
}

// NewAllowlist собирает allowlist. Записи: "Service.Method" или "Service.*"
// (wildcard по всему сервису). Дубликаты схлопываются; невалидные записи —
// ошибка.
func NewAllowlist(entries []string, policy AllowlistPolicy) (*Allowlist, error) {
	a := &Allowlist{
		policy:   policy,
		exact:    map[string]struct{}{},
		services: map[string]struct{}{},
	}
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		service, rest, ok := strings.Cut(entry, ".")
		if !ok || service == "" || rest == "" {
			return nil, fmt.Errorf("proxy: allowlist entry %q must be \"Service.Method\" or \"Service.*\"", entry)
		}
		if strings.Contains(rest, ".") {
			return nil, fmt.Errorf("proxy: allowlist entry %q must contain a single dot", entry)
		}
		if rest == "*" {
			a.services[service] = struct{}{}
			continue
		}
		a.exact[entry] = struct{}{}
	}
	return a, nil
}

// Allowed реализует Allowlister: nil — разрешено, иначе — отказ.
// Возвращает nil для nil-приёмника и пустого списка при PolicyDefault
// (default-allow).
func (a *Allowlist) Allowed(method string) error {
	if a == nil {
		return nil
	}
	filter := true
	switch a.policy {
	case PolicyDefault:
		filter = len(a.exact)+len(a.services) > 0
	case PolicyDenyEmpty:
		// фильтрация всегда, включая пустой список (deny all)
	default:
		return fmt.Errorf("proxy: unknown allowlist policy %d", a.policy)
	}
	if !filter {
		return nil
	}
	if a.allows(method) {
		return nil
	}
	return fmt.Errorf("proxy: method %q is not allowed by the proxy allowlist", method)
}

// allows — единственная точка сопоставления метода со списком.
func (a *Allowlist) allows(method string) bool {
	if _, ok := a.exact[method]; ok {
		return true
	}
	service, ok := SplitMethod(method)
	if !ok {
		return false
	}
	_, ok = a.services[service]
	return ok
}

// Entries возвращает нормализованный список записей (для логов/диагностики).
func (a *Allowlist) Entries() []string {
	if a == nil {
		return nil
	}
	out := make([]string, 0, len(a.exact)+len(a.services))
	for e := range a.exact {
		out = append(out, e)
	}
	for s := range a.services {
		out = append(out, s+".*")
	}
	return out
}
