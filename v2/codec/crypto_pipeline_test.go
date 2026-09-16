package codec

import (
	"bufio"
	"testing"

	"go.slink.ws/rpc"

	"github.com/stretchr/testify/require"
)

// Пайплайн нескольких крипто-фреймов в одном буфере: клиент шлёт burst,
// сервер последовательно читает. Проверяет, не десинхронизируется ли серверный
// кодек при конвейеризации (магический префикс между gob-сообщениями).
func TestCryptoServerPipelinedBurst(t *testing.T) {
	conn := newTestConnection()
	client, err := NewClientCodec(bufio.NewWriter(conn.request), conn.response, []byte("0123456789ABCDEF"))
	require.NoError(t, err)
	server, err := NewServerCodec(bufio.NewWriter(conn.response), conn.request, []byte("0123456789ABCDEF"))
	require.NoError(t, err)

	const n = 200
	var args []testRq
	for i := 0; i < n; i++ {
		args = append(args, testRq{Key: "k", Val: "v"})
	}

	// весь burst пишем синхронно до чтения (bytes.Buffer не конкурентен)
	rq := rpc.Request{ServiceMethod: "test.Method", ID: "burst-id"}
	for i := 0; i < n; i++ {
		require.NoError(t, client.WriteRequest(&rq, &args[i]))
	}

	for i := 0; i < n; i++ {
		var req rpc.Request
		if err := server.ReadRequestHeader(&req); err != nil {
			t.Fatalf("read header %d: %v", i, err)
		}
		var arg testRq
		if err := server.ReadRequestBody(&arg); err != nil {
			t.Fatalf("read body %d: %v", i, err)
		}
		if arg != args[i] {
			t.Fatalf("body mismatch on %d", i)
		}
	}

}
