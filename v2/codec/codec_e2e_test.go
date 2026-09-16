package codec

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"go.slink.ws/rpc"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// region - test connection

type testConnection struct {
	request  *BytesReadWriteCloser
	response *BytesReadWriteCloser
}

func newTestConnection() testConnection {
	return testConnection{
		request:  NewBytesReadWriteCloser(),
		response: NewBytesReadWriteCloser(),
	}
}

// endregion
// region - test client

type testRq struct {
	Key string
	Val string
}

type testClientCodec struct {
	rq   rpc.Request
	body testRq
	conn *testConnection
	cdc  rpc.ClientCodec
}

func initClientCodec(conn *testConnection, key []byte) testClientCodec {
	rq := rpc.Request{
		ServiceMethod: "test.Method",
		ID:            "test-request-id",
	}
	body := testRq{
		Key: "rq_test_key",
		Val: "rq_test_val",
	}
	cc := GetClientCodec(bufio.NewWriter(conn.request), conn.response, key)
	return testClientCodec{
		rq:   rq,
		body: body,
		conn: conn,
		cdc:  cc,
	}
}

// endregion
// region - test server

type testRs struct {
	Key string
	Val string
}

type testServerCodec struct {
	rs   rpc.Response
	body testRs
	conn *testConnection
	cdc  rpc.ServerCodec
}

func initServerCodec(conn *testConnection, key []byte) testServerCodec {
	rs := rpc.Response{
		ServiceMethod: "test.Method",
		ID:            "test-request-id",
	}
	body := testRs{
		Key: "rs_test_key",
		Val: "rs_test_val",
	}
	sc := GetServerCodec(bufio.NewWriter(conn.response), conn.request, key)
	return testServerCodec{
		rs:   rs,
		body: body,
		conn: conn,
		cdc:  sc,
	}
}

// endregion

func TestConnectionIo(t *testing.T) {

	conn := newTestConnection()
	assert.Equal(t, 0, conn.request.Len())

	n, err := conn.request.Write([]byte("test"))
	assert.NoErrorf(t, err, "Write failed: %v", err)
	assert.Equalf(t, 4, n, "Write failed: expected 4 bytes written, got %d", n)

	p := conn.request.ReadAll()
	assert.Equalf(t, 4, len(p), "expected 4 bytes read, got %d", len(p))
	assert.Equalf(t, "test", string(p), "expected 'test', got '%s'", string(p))

}

func TestOpenCodec(t *testing.T) {
	conn := newTestConnection()
	testClient := initClientCodec(&conn, nil)
	testServer := initServerCodec(&conn, nil)
	testCodecE2E(t, &testClient, &testServer)
}

func TestCryptoCodec(t *testing.T) {
	key := []byte("0123456789ABCDEF")
	conn := newTestConnection()
	testClient := initClientCodec(&conn, key)
	testServer := initServerCodec(&conn, key)
	testCodecE2E(t, &testClient, &testServer)
}

func TestCryptoCodecNonMatchedKeys(t *testing.T) {
	conn := newTestConnection()
	testClient := initClientCodec(&conn, []byte("0123456789ABCDEF"))
	testServer := initServerCodec(&conn, []byte("FEDCBA9876543210"))
	testCodecErr(t, &testClient, &testServer)
}

func TestCryptoCodecTamperedCiphertext(t *testing.T) {
	conn := newTestConnection()
	testClient := initClientCodec(&conn, []byte("0123456789ABCDEF"))
	testServer := initServerCodec(&conn, []byte("0123456789ABCDEF"))

	require.NoError(t, testClient.cdc.WriteRequest(&testClient.rq, testClient.body))

	// фрейм ещё в conn.request (сервер ничего не читал): gob(header) + magic + gob([]byte ciphertext).
	// Последний байт фрейма — последний байт GCM-тега. Инверсия обязана сломать аутентификацию.
	frame := conn.request.Bytes()
	require.Greater(t, len(frame), 0)
	frame[len(frame)-1] ^= 0xFF

	var rqHeader rpc.Request
	require.NoError(t, testServer.cdc.ReadRequestHeader(&rqHeader))

	var rqBody testRq
	err := testServer.cdc.ReadRequestBody(&rqBody)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt")
}

func TestCryptoCodecInvalidMagic(t *testing.T) {
	conn := newTestConnection()
	testClient := initClientCodec(&conn, []byte("0123456789ABCDEF"))
	testServer := initServerCodec(&conn, []byte("0123456789ABCDEF"))

	require.NoError(t, testClient.cdc.WriteRequest(&testClient.rq, testClient.body))

	frame := conn.request.Bytes()
	idx := bytes.Index(frame, magicPrefix)
	require.GreaterOrEqual(t, idx, 0, "magic prefix must be present in crypto frame")
	for i := idx; i < idx+len(magicPrefix); i++ {
		frame[i] = 'X'
	}

	var rqHeader rpc.Request
	require.NoError(t, testServer.cdc.ReadRequestHeader(&rqHeader))

	var rqBody testRq
	err := testServer.cdc.ReadRequestBody(&rqBody)
	require.Error(t, err)
	assert.ErrorIs(t, err, InvalidPrefixErr)
}

func TestCryptoCodecShortCiphertext(t *testing.T) {
	conn := newTestConnection()
	testServer := initServerCodec(&conn, []byte("0123456789ABCDEF"))

	// собираем фрейм вручную: корректный gob-заголовок, magic и gob-массив, короче nonce
	var header bytes.Buffer
	require.NoError(t, gob.NewEncoder(&header).Encode(testServer.rs))

	var blob bytes.Buffer
	require.NoError(t, gob.NewEncoder(&blob).Encode([]byte{1, 2, 3}))

	frame := append(header.Bytes(), magicPrefix...)
	frame = append(frame, blob.Bytes()...)
	_, err := conn.request.Write(frame)
	require.NoError(t, err)

	var rqHeader rpc.Request
	err = testServer.cdc.ReadRequestHeader(&rqHeader)
	if err != nil {
		// gob может дочитать накопленные байты и споткнуться о "неправильный" magic-хвост;
		// в этом случае мы не добираемся до тела — приёмлемо для негативного теста.
		return
	}

	var rqBody testRq
	err = testServer.cdc.ReadRequestBody(&rqBody)
	require.Error(t, err)
}

func testCodecE2E(t *testing.T, c *testClientCodec, s *testServerCodec) {

	require.NotNil(t, c.cdc, "client codec should not be nil")
	require.NotNil(t, s.cdc, "server codec should not be nil")

	// client: write client request
	require.NoError(t, c.cdc.WriteRequest(&c.rq, c.body))

	// server: parse client request header
	var rqHeader rpc.Request
	require.NoError(t, s.cdc.ReadRequestHeader(&rqHeader))
	assert.Equal(t, c.rq.ServiceMethod, rqHeader.ServiceMethod)
	assert.Equal(t, c.rq.ID, rqHeader.ID)

	// server: parse client request body
	var rqBody testRq
	require.NoError(t, s.cdc.ReadRequestBody(&rqBody))
	assert.Equal(t, c.body.Key, rqBody.Key)
	assert.Equal(t, c.body.Val, rqBody.Val)

	// server: write response
	require.NoError(t, s.cdc.WriteResponse(&s.rs, s.body))

	// client: parse server response header
	var rsHeader rpc.Response
	require.NoError(t, c.cdc.ReadResponseHeader(&rsHeader))
	assert.Equal(t, s.rs.ServiceMethod, rsHeader.ServiceMethod)
	assert.Equal(t, s.rs.ID, rsHeader.ID)

	// client: parse server response body
	var rsBody testRs
	require.NoError(t, c.cdc.ReadResponseBody(&rsBody))
	assert.Equal(t, s.body.Key, rsBody.Key)
	assert.Equal(t, s.body.Val, rsBody.Val)

}

func testCodecErr(t *testing.T, c *testClientCodec, s *testServerCodec) {

	require.NotNil(t, c.cdc, "client codec should not be nil")
	require.NotNil(t, s.cdc, "server codec should not be nil")

	// client: write client request
	require.NoError(t, c.cdc.WriteRequest(&c.rq, c.body))

	// server: parse client request header
	var rqHeader rpc.Request
	require.NoError(t, s.cdc.ReadRequestHeader(&rqHeader))
	assert.Equal(t, c.rq.ServiceMethod, rqHeader.ServiceMethod)

	// server: parse client request body
	var rqBody testRq
	err := s.cdc.ReadRequestBody(&rqBody)
	assert.Error(t, err)

}
