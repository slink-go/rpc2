package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeMetrics записывает последний CallStats.
type fakeMetrics struct {
	last    []CallStats
	calls   int
	gotCall func(CallStats)
}

func (m *fakeMetrics) ObserveCall(stats CallStats) {
	m.calls++
	m.last = append(m.last, stats)
	if m.gotCall != nil {
		m.gotCall(stats)
	}
}

func TestServiceMetricsOnSuccess(t *testing.T) {
	t.Parallel()
	m := &fakeMetrics{}
	svc := New(&fakeRouter{tgt: testTarget}, nil, &fakeInvoker{payload: []byte(`{"ok":1}`)}, WithMetrics(m))

	res := &GenericResponse{}
	require.NoError(t, svc.Call(context.Background(), GenericRequest{Method: "miki.Get", Payload: []byte(`{}`)}, res))

	require.Equal(t, 1, m.calls)
	st := m.last[0]
	assert.Equal(t, "miki.Get", st.Method)
	assert.Equal(t, OutcomeOK, st.Outcome)
	assert.Empty(t, st.Code)
	assert.NotEmpty(t, st.Target)
	assert.Equal(t, 2, st.ReqBytes)
	assert.Equal(t, 8, st.ResBytes)
	assert.GreaterOrEqual(t, st.Latency, time.Duration(0))
}

func TestServiceMetricsOnAllowlistDeny(t *testing.T) {
	t.Parallel()
	m := &fakeMetrics{}
	allow := &fakeAllow{err: context.Canceled} // любая не-nil ошибка
	svc := New(&fakeRouter{tgt: testTarget}, allow, &fakeInvoker{}, WithMetrics(m))

	res := &GenericResponse{}
	require.NoError(t, svc.Call(context.Background(), GenericRequest{Method: "miki.Get", Payload: []byte(`{}`)}, res))

	require.Equal(t, 1, m.calls)
	st := m.last[0]
	assert.Equal(t, OutcomeError, st.Outcome)
	assert.Equal(t, CodeAllowlist, st.Code)
	assert.Empty(t, st.ResBytes)
}

func TestServiceMetricsOnInvokerError(t *testing.T) {
	t.Parallel()
	m := &fakeMetrics{}
	inv := &fakeInvoker{cerr: &CallError{Code: CodeBackend, Message: "boom", Target: "svc:1234"}}
	svc := New(&fakeRouter{tgt: testTarget}, nil, inv, WithMetrics(m))

	res := &GenericResponse{}
	require.NoError(t, svc.Call(context.Background(), GenericRequest{Method: "miki.Get", Payload: []byte(`{}`)}, res))

	require.Equal(t, 1, m.calls)
	st := m.last[0]
	assert.Equal(t, OutcomeError, st.Outcome)
	assert.Equal(t, CodeBackend, st.Code)
	assert.Equal(t, "svc:1234", st.Target)
}

func TestServiceNoopMetricsDefault(t *testing.T) {
	t.Parallel()
	svc := New(&fakeRouter{tgt: testTarget}, nil, &fakeInvoker{payload: []byte(`{}`)})
	res := &GenericResponse{}
	require.NoError(t, svc.Call(context.Background(), GenericRequest{Method: "miki.Get"}, res))
	assert.Equal(t, noopMetrics{}, svc.metrics)
}

func TestWithNilOptionsIgnored(t *testing.T) {
	t.Parallel()
	svc := New(&fakeRouter{tgt: testTarget}, nil, &fakeInvoker{}, WithLogger(nil), WithMetrics(nil))
	assert.NotNil(t, svc.logger)
	assert.Equal(t, noopMetrics{}, svc.metrics)
}
