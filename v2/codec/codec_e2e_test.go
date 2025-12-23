package codec

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/rpc"
	"testing"
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
		Seq:           0,
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
		Seq:           0,
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
	assert.NoErrorf(t, err, "read error: %v", err)
	assert.Equalf(t, 4, n, "expected 4 bytes read, got %d", n)
	assert.Equalf(t, "test", string(p), "expected 'test', got '%s'", string(p))

}
func TestOpenCodec(t *testing.T) {
	conn := newTestConnection()
	testClient := initClientCodec(&conn, nil)
	testServer := initServerCodec(&conn, nil)
	testCodecE2E(t, &testClient, &testServer)
}
func TestCryptoCodec(t *testing.T) {
	var key []byte
	key = []byte("0123456789ABCDEF")
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

func testCodecE2E(t *testing.T, c *testClientCodec, s *testServerCodec) {

	assert.NotNilf(t, c.cdc, "client codec should not be nil")
	assert.NotNilf(t, s.cdc, "server codec should not be nil")

	// client: write client request
	err := c.cdc.WriteRequest(&c.rq, c.body)
	assert.NoError(t, err)
	fmt.Printf("%v\n", chars(c.conn.request.Bytes()))

	// server: parse client request header
	var rqHeader rpc.Request
	err = s.cdc.ReadRequestHeader(&rqHeader)
	assert.NoError(t, err)
	assert.Equal(t, c.rq.ServiceMethod, rqHeader.ServiceMethod)

	// server: parse client request body
	var rqBody testRq
	err = s.cdc.ReadRequestBody(&rqBody)
	assert.NoError(t, err)
	assert.Equal(t, c.body.Key, rqBody.Key)
	assert.Equal(t, c.body.Val, rqBody.Val)

	// server: write response
	err = s.cdc.WriteResponse(&s.rs, s.body)
	assert.NoError(t, err)

	// client: parse server response header
	var rsHeader rpc.Response
	err = c.cdc.ReadResponseHeader(&rsHeader)
	assert.NoError(t, err)
	assert.Equal(t, s.rs.ServiceMethod, rsHeader.ServiceMethod)

	// client: parse server response body
	var rsBody testRs
	err = c.cdc.ReadResponseBody(&rsBody)
	assert.NoError(t, err)
	assert.Equal(t, s.body.Key, rsBody.Key)
	assert.Equal(t, s.body.Val, rsBody.Val)

}
func testCodecErr(t *testing.T, c *testClientCodec, s *testServerCodec) {

	assert.NotNilf(t, c.cdc, "client codec should not be nil")
	assert.NotNilf(t, s.cdc, "server codec should not be nil")

	// client: write client request
	err := c.cdc.WriteRequest(&c.rq, c.body)
	assert.NoError(t, err)

	// server: parse client request header
	var rqHeader rpc.Request
	err = s.cdc.ReadRequestHeader(&rqHeader)
	assert.NoError(t, err)
	assert.Equal(t, c.rq.ServiceMethod, rqHeader.ServiceMethod)

	// server: parse client request body
	var rqBody testRq
	err = s.cdc.ReadRequestBody(&rqBody)
	assert.Error(t, err)
	fmt.Printf("error: %v\n", errors.Unwrap(err))

}
