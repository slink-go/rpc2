package proxy

import "time"

// Итоги вызова для логов и метрик.
const (
	OutcomeOK    = "ok"
	OutcomeError = "error"
)

// CallStats — параметры завершённого вызова для наблюдаемости.
type CallStats struct {
	Method   string
	Target   string
	Outcome  string // OutcomeOK | OutcomeError
	Code     string // Code* при ошибке, иначе ""
	Latency  time.Duration
	ReqBytes int
	ResBytes int
}

// Metrics — точка наблюдаемости generic-proxy. Ядро rpc2 не зависит от
// prometheus: реализация с метриками внедряется сборкой (T2.7/Etap-2
// обвязка), здесь — только контракт. nil-метрики не поддерживаются —
// по умолчанию используется noopMetrics.
type Metrics interface {
	ObserveCall(stats CallStats)
}

// noopMetrics — no-op реализация по умолчанию.
type noopMetrics struct{}

// ObserveCall игнорирует статистику.
func (noopMetrics) ObserveCall(CallStats) {}
