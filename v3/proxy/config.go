package proxy

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// Переменные окружения generic-proxy (конвенция проекта: структура + env):
//
//	PROXY_TARGET_<SERVICE>=addr[:port]  — обязательная цель (port по умолчанию 2233)
//	PROXY_TARGET_<SERVICE>_TRANSPORT    — auto|json|legacy (по умолчанию auto)
//	PROXY_TARGET_<SERVICE>_SECRET       — ключ AES-GCM в hex (по умолчанию без крипты)
//
// <SERVICE> — имя сервиса в нижнем регистре (совпадает с RegisterName).
const (
	envTargetPrefix    = "PROXY_TARGET_"
	envTransportSuffix = "_TRANSPORT"
	envSecretSuffix    = "_SECRET"
	defaultTargetPort  = 2233
)

// Config — конфигурация generic-proxy. Секции добавляются по мере развития
// плана: allowlist (T2.3), лимиты (T2.5).
type Config struct {
	// Targets — таблица маршрутов «имя сервиса → цель».
	Targets map[string]Target
}

// ConfigFromEnv собирает Config из окружения. Ошибки парсинга возвращаются
// с указанием переменной.
func ConfigFromEnv() (Config, error) {
	type rawTarget struct {
		addr      string
		transport string
		secret    string
		hasAddr   bool
	}
	raw := map[string]*rawTarget{}
	get := func(service string) *rawTarget {
		t := raw[service]
		if t == nil {
			t = &rawTarget{}
			raw[service] = t
		}
		return t
	}

	for _, kv := range os.Environ() {
		key, value, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(key, envTargetPrefix) {
			continue
		}
		rest := strings.TrimPrefix(key, envTargetPrefix)
		if rest == "" {
			return Config{}, fmt.Errorf("proxy: %s without service name", envTargetPrefix)
		}
		switch {
		case strings.HasSuffix(rest, envTransportSuffix):
			service := strings.ToLower(strings.TrimSuffix(rest, envTransportSuffix))
			if service == "" {
				return Config{}, fmt.Errorf("proxy: %s without service name", envTargetPrefix)
			}
			get(service).transport = value
		case strings.HasSuffix(rest, envSecretSuffix):
			service := strings.ToLower(strings.TrimSuffix(rest, envSecretSuffix))
			if service == "" {
				return Config{}, fmt.Errorf("proxy: %s without service name", envTargetPrefix)
			}
			get(service).secret = value
		default:
			service := strings.ToLower(rest)
			t := get(service)
			t.addr = value
			t.hasAddr = true
		}
	}

	cfg := Config{Targets: make(map[string]Target, len(raw))}
	for service, t := range raw {
		if !t.hasAddr {
			return Config{}, fmt.Errorf("proxy: target address for service %q is missing (set %s%s)",
				service, envTargetPrefix, strings.ToUpper(service))
		}
		addr, port, err := splitHostPortDefault(t.addr)
		if err != nil {
			return Config{}, fmt.Errorf("proxy: %s%s: %w", envTargetPrefix, strings.ToUpper(service), err)
		}
		var secret []byte
		if t.secret != "" {
			secret, err = hex.DecodeString(t.secret)
			if err != nil {
				return Config{}, fmt.Errorf("proxy: %s%s_SECRET: invalid hex: %w",
					envTargetPrefix, strings.ToUpper(service), err)
			}
		}
		cfg.Targets[service] = Target{
			Addr:      addr,
			Port:      port,
			Secret:    secret,
			Transport: t.transport,
		}
	}
	return cfg, nil
}

// splitHostPortDefault разбирает "addr[:port]"; без порта — defaultTargetPort.
func splitHostPortDefault(v string) (string, int, error) {
	if v == "" {
		return "", 0, fmt.Errorf("empty target address")
	}
	host, portStr, err := net.SplitHostPort(v)
	if err != nil {
		if strings.Contains(err.Error(), "missing port") {
			return v, defaultTargetPort, nil
		}
		return "", 0, fmt.Errorf("invalid target %q: %w", v, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port in %q: %w", v, err)
	}
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("port %d out of range in %q", port, v)
	}
	return host, port, nil
}
