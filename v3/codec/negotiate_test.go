package codec

import (
	"bytes"
	"encoding/gob"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.slink.ws/rpc"
)

// TestGobTypicalHeadersNotJSON — ключевой инвариант sniff: свежий gob-поток
// типовых заголовков не начинается с '{' (0x7B), т.е. не может быть принят
// за JSON. Эмпирика Go 1.27: 0x39 (Request), 0x45 (Response), 0x16 (прочие).
func TestGobTypicalHeadersNotJSON(t *testing.T) {
	t.Parallel()
	cases := map[string]any{
		"request":       &rpc.Request{ServiceMethod: "TestServer.Hello", ID: "1", Header: rpc.Header{"x": {"y"}}},
		"request_empty": &rpc.Request{ServiceMethod: "A.B"},
		"response":      &rpc.Response{ServiceMethod: "TestServer.Hello", ID: "1"},
		"response_err":  &rpc.Response{Error: "some error message"},
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, gob.NewEncoder(&buf).Encode(v))
			b := buf.Bytes()
			require.NotEmpty(t, b)
			assert.NotEqual(t, byte(jsonFirstByte), b[0],
				"gob stream starts with '{' (0x7B): sniff collision")
			assert.False(t, isJSONStream(b), "gob stream misdetected as JSON")
		})
	}
}

func TestIsJSONStream(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"compact_object", []byte(`{"ServiceMethod":"A.B"}`), true},
		{"space_after_brace", []byte(` {"a":1}`), false}, // первый байт не '{'
		{"newline_after_brace", []byte("{\n\"a\":1}"), true},
		{"tab_after_brace", []byte("{\t\"a\":1}"), true},
		{"space_after_brace_only", []byte("{ "), true},
		{"brace_close", []byte("{}"), false},
		{"brace_letter", []byte("{x"), false}, // gob-подобный второй байт
		{"brace_7f", []byte{0x7B, 0x7F}, false},
		{"single_brace", []byte{'{'}, true}, // обрезанный поток решит декодер
		{"empty", nil, false},
		{"array", []byte("[1,2]"), false},
		{"gob_request", mustGob(t, &rpc.Request{ServiceMethod: "A.B"}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, isJSONStream(tc.in))
		})
	}
}

func mustGob(t *testing.T, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, gob.NewEncoder(&buf).Encode(v))
	return buf.Bytes()
}

// TestSniffReplay — sniff не теряет прочитанные байты: JSON- и gob-потоки
// восстанавливаются байт-в-байт для первого декодера.
func TestSniffReplay(t *testing.T) {
	t.Parallel()
	t.Run("json", func(t *testing.T) {
		t.Parallel()
		payload := []byte(`{"ServiceMethod":"A.B","ID":"1"}`)
		conn, peer := net.Pipe()
		go func() {
			_, _ = peer.Write(payload)
			_ = peer.Close()
		}()
		sniffed, isJSON, err := sniffServerConn(conn)
		require.NoError(t, err)
		assert.True(t, isJSON)
		got, err := io.ReadAll(sniffed)
		require.NoError(t, err)
		assert.Equal(t, payload, got)
	})
	t.Run("gob", func(t *testing.T) {
		t.Parallel()
		payload := mustGob(t, &rpc.Request{ServiceMethod: "A.B", ID: "1"})
		conn, peer := net.Pipe()
		go func() {
			_, _ = peer.Write(payload)
			_ = peer.Close()
		}()
		sniffed, isJSON, err := sniffServerConn(conn)
		require.NoError(t, err)
		assert.False(t, isJSON)
		got, err := io.ReadAll(sniffed)
		require.NoError(t, err)
		assert.Equal(t, payload, got)
	})
	t.Run("empty_conn_is_legacy", func(t *testing.T) {
		t.Parallel()
		conn, peer := net.Pipe()
		_ = peer.Close()
		sniffed, isJSON, err := sniffServerConn(conn)
		require.NoError(t, err)
		assert.False(t, isJSON)
		n, err := sniffed.Read(make([]byte, 1))
		assert.Zero(t, n)
		assert.ErrorIs(t, err, io.EOF)
	})
}
