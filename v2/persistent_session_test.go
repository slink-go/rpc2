package rpc2

import (
	"context"
	"fmt"
	"io"
	"net"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCryptoKey = "0123456789ABCDEF"

// echoService — простая эхо-методика для сессионных тестов.
type echoService struct{}

func (echoService) Say(ctx context.Context, in string, out *string) error {
	*out = in
	return nil
}

// cancelProbe — метод, который ждёт отмены контекста на стороне сервера.
type cancelProbe struct {
	started chan struct{}
	stopped chan struct{}
	once    sync.Once
}

func newCancelProbe() *cancelProbe {
	return &cancelProbe{started: make(chan struct{}), stopped: make(chan struct{})}
}

func (p *cancelProbe) Wait(ctx context.Context, in string, out *string) error {
	close(p.started)
	defer p.once.Do(func() { close(p.stopped) })
	<-ctx.Done()
	return ctx.Err()
}

// tcpProxy — прозрачный TCP-прокси, считает входящие клиентские соединения
// и умеет резать живые соединения (эмуляция обрыва сети).
type tcpProxy struct {
	ln       net.Listener
	upstream string

	mu    sync.Mutex
	conns map[net.Conn]struct{}
	total atomic.Int64
}

func newTCPProxy(t *testing.T, upstream string) *tcpProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := &tcpProxy{ln: ln, upstream: upstream, conns: make(map[net.Conn]struct{})}
	t.Cleanup(func() { _ = ln.Close() })
	go p.acceptLoop()
	return p
}

func (p *tcpProxy) addr() net.Addr { return p.ln.Addr() }

func (p *tcpProxy) totalConns() int64 { return p.total.Load() }

func (p *tcpProxy) acceptLoop() {
	for {
		conn, err := p.ln.Accept()
		if err != nil {
			return
		}
		up, err := net.Dial("tcp", p.upstream)
		if err != nil {
			_ = conn.Close()
			continue
		}
		p.total.Add(1)
		p.mu.Lock()
		p.conns[conn] = struct{}{}
		p.conns[up] = struct{}{}
		p.mu.Unlock()
		go func() { _, _ = io.Copy(up, conn) }()
		go func() { _, _ = io.Copy(conn, up) }()
	}
}

func (p *tcpProxy) cut() {
	p.mu.Lock()
	for conn := range p.conns {
		delete(p.conns, conn)
		_ = conn.Close()
	}
	p.mu.Unlock()
}

// testEnv — сервер (127.0.0.1:free) + прокси перед ним + персистентный клиент.
type testEnv struct {
	proxy  *tcpProxy
	client *CustomRpcClient
	probe  *cancelProbe

	cancelSvr context.CancelFunc
}

func startEnv(t *testing.T, key []byte, withProbe bool) *testEnv {
	t.Helper()
	ctx, cancelSvr := context.WithCancel(context.Background())
	t.Cleanup(cancelSvr)

	port := freePort(t)
	svr := NewRpcServer(
		ServerWithAddress("127.0.0.1"),
		ServerWithPort(port),
		ServerWithCryptoKey(key),
	)
	require.NoError(t, svr.RegisterName("Echo", echoService{}))

	var probe *cancelProbe
	if withProbe {
		probe = newCancelProbe()
		require.NoError(t, svr.RegisterName("Probe", probe))
	}
	go func() { _ = svr.Accept(ctx) }()

	serverAddr := "127.0.0.1:" + strconv.Itoa(port)
	waitDial(t, serverAddr)

	proxy := newTCPProxy(t, serverAddr)
	client := NewRpcClient(
		ClientWithAddress("127.0.0.1"),
		ClientWithPort(proxy.addr().(*net.TCPAddr).Port),
		ClientWithCryptoKey(key),
	)
	t.Cleanup(func() { _ = client.Close() })

	return &testEnv{proxy: proxy, client: client, probe: probe, cancelSvr: cancelSvr}
}

func (e *testEnv) say(t *testing.T, ctx context.Context, in string) (string, error) {
	t.Helper()
	var out string
	err := e.client.Call(ctx, "Echo.Say", in, &out)
	return out, err
}

func (e *testEnv) say2(ctx context.Context, in string) (string, error) {
	var out string
	err := e.client.Call(ctx, "Echo.Say", in, &out)
	return out, err
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

func waitDial(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server %s did not start", addr)
}

func eventually(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s: %s", timeout, msg)
}

// один клиент, 1000 последовательных вызовов -> одно соединение через прокси
func TestSessionReusesConnection(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  []byte
	}{
		{"open", nil},
		{"crypto", []byte(testCryptoKey)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := startEnv(t, tc.key, false)
			for i := 0; i < 1000; i++ {
				out, err := env.say(t, context.Background(), "hello")
				require.NoError(t, err)
				assert.Equal(t, "hello", out)
			}
			assert.LessOrEqual(t, env.proxy.totalConns(), int64(2), "persistent session must reuse the connection")
		})
	}
}

// обрыв соединения -> ошибка, затем следующий вызов автоматически переподнимает сессию
func TestSessionReconnectsAfterDrop(t *testing.T) {
	env := startEnv(t, []byte(testCryptoKey), false)

	out, err := env.say(t, context.Background(), "before")
	require.NoError(t, err)
	assert.Equal(t, "before", out)
	assert.EqualValues(t, 1, env.proxy.totalConns())

	env.proxy.cut()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = env.say(t, ctx, "after-cut")
	require.Error(t, err, "call over a severed connection must fail")

	out, err = env.say(t, context.Background(), "after-reconn")
	require.NoError(t, err, "client must transparently reconnect")
	assert.Equal(t, "after-reconn", out)
	assert.EqualValues(t, 2, env.proxy.totalConns(), "exactly one new connection after the drop")
}

// параллельные вызовы на общей сессии (мультиплексирование одного соединения)
func TestSessionParallelCalls(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  []byte
	}{
		{"open", nil},
		{"crypto", []byte(testCryptoKey)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := startEnv(t, tc.key, false)

			const workers, perGoroutine = 20, 20
			var wg sync.WaitGroup
			errs := make(chan error, workers*perGoroutine)

			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					expect := strconv.Itoa(w) + "/"
					for i := 0; i < perGoroutine; i++ {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						out, err := env.say2(ctx, expect+strconv.Itoa(i))
						cancel()
						if err != nil {
							errs <- fmt.Errorf("worker=%d i=%d conns=%d: %w", w, i, env.proxy.totalConns(), err)
							return
						}
						if out != expect+strconv.Itoa(i) {
							errs <- fmt.Errorf("worker=%d i=%d: echo mismatch %q", w, i, out)
							return
						}
					}
				}(w)
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				require.NoError(t, err)
			}
			assert.LessOrEqual(t, env.proxy.totalConns(), int64(3), "parallel calls must share a single session")
		})
	}
}

// отмена клиентского контекста -> отмена задачи на сервере, сессия остаётся живой
func TestSessionContextCancelPropagatesToServer(t *testing.T) {
	env := startEnv(t, []byte(testCryptoKey), true)
	require.NotNil(t, env.probe)

	callCtx, cancel := context.WithCancel(context.Background())
	var out string
	var callErr error
	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		callErr = env.client.Call(callCtx, "Probe.Wait", "", &out)
	}()

	select {
	case <-env.probe.started:
	case <-time.After(2 * time.Second):
		t.Fatal("server handler did not start")
	}

	cancel()

	select {
	case <-callDone:
	case <-time.After(3 * time.Second):
		t.Fatal("client call did not return after context cancel")
	}
	require.Error(t, callErr, "cancelled client call must report an error")

	select {
	case <-env.probe.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("server-side task was not cancelled after client context cancel")
	}

	out, err := env.say(t, context.Background(), "after-cancel")
	require.NoError(t, err, "session must stay usable after cancellation")
	assert.Equal(t, "after-cancel", out)
}

// мёртвый сервер: клиент не зависает и быстро возвращает ошибку
func TestSessionDeadServerNoHang(t *testing.T) {
	port := freePort(t)
	client := NewRpcClient(
		ClientWithAddress("127.0.0.1"),
		ClientWithPort(port),
		ClientWithCryptoKey([]byte(testCryptoKey)),
	)
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := client.Call(ctx, "Echo.Say", "ping", new(string))
	elapsed := time.Since(start)
	require.Error(t, err)
	assert.Less(t, elapsed, 2*time.Second, "call to a dead server must fail fast")
}

// утечки горутин на цикле переподключений
func TestSessionNoGoroutineLeak(t *testing.T) {
	env := startEnv(t, []byte(testCryptoKey), false)

	start := runtime.NumGoroutine()

	for i := 0; i < 20; i++ {
		out, err := env.say(t, context.Background(), "tick")
		require.NoError(t, err)
		assert.Equal(t, "tick", out)
		env.proxy.cut()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, _ = env.say(t, ctx, "fail")
		cancel()
	}

	env.client.Close()
	env.cancelSvr()
	_ = env.proxy.ln.Close()

	eventually(t, 5*time.Second, "goroutine count must return to baseline",
		func() bool { runtime.GC(); return runtime.NumGoroutine() <= start+5 })
}
