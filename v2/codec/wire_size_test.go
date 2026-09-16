package codec

import (
	"bufio"
	"testing"

	"go.slink.ws/rpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Проверка оптимизации v0.0.15: внешняя пара кодеков пишет определения типов
// один раз на соединение — второй и последующие кадры меньше первого.
func TestCryptoClientFrameShrinksAfterFirst(t *testing.T) {
	conn := newTestConnection()
	key := []byte("0123456789ABCDEF")

	cl, err := NewClientCodec(bufio.NewWriter(conn.request), conn.response, key)
	require.NoError(t, err)

	makeReq := func(id string) *rpc.Request {
		return &rpc.Request{ServiceMethod: "test.Method", ID: id}
	}

	require.NoError(t, cl.WriteRequest(makeReq("f1"), &testRq{Key: "k1", Val: "v1"}))
	frame1 := conn.request.Len()
	require.NoError(t, cl.WriteRequest(makeReq("f2"), &testRq{Key: "k2", Val: "v2"}))
	frame2 := conn.request.Len() - frame1

	t.Logf("frame1=%d frame2=%d", frame1, frame2)
	assert.LessOrEqual(t, frame2, frame1, "second crypto frame must not be larger than the first")
	if frame2 >= frame1 {
		t.Fatalf("expected defs to be sent once: frame1=%d frame2=%d", frame1, frame2)
	}
}

func TestCryptoServerFrameShrinksAfterFirst(t *testing.T) {
	conn := newTestConnection()
	key := []byte("0123456789ABCDEF")

	svr, err := NewServerCodec(bufio.NewWriter(conn.response), conn.request, key)
	require.NoError(t, err)

	makeRes := func(id string) *rpc.Response {
		return &rpc.Response{ServiceMethod: "test.Method", ID: id}
	}

	require.NoError(t, svr.WriteResponse(makeRes("f1"), &testRs{Key: "k1", Val: "v1"}))
	frame1 := conn.response.Len()
	require.NoError(t, svr.WriteResponse(makeRes("f2"), &testRs{Key: "k2", Val: "v2"}))
	frame2 := conn.response.Len() - frame1

	t.Logf("frame1=%d frame2=%d", frame1, frame2)
	if frame2 >= frame1 {
		t.Fatalf("expected defs to be sent once: frame1=%d frame2=%d", frame1, frame2)
	}
}
