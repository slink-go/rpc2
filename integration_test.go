package rpc2

import (
	"context"
	"fmt"
	"github.com/renevo/rpc"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

// region - test server implementation

type Server int

func (Server) Hello(ctx context.Context, name string, msg *string) error {
	fmt.Printf("    > Hello Request ID: %q\n", rpc.ContextID(ctx))
	fmt.Printf("    > Hello Request Injected Header: %v\n", ctx.Value("received-x-test-header"))
	*msg = fmt.Sprintf("Hello, %s!", name)
	return nil
}

// endregion
// region - test client implementation

type Client struct {
	*CustomRpcClient
}

func (c *Client) Hello(ctx context.Context, name string) (string, error) {
	var msg string
	err := c.Call(
		rpc.ContextWithHeaders(ctx, rpc.Header{}.Set("x-test-header", "some value")),
		"TestServer.Hello",
		name,
		&msg,
	)
	return msg, err
}

// endregion
// region - test middlewares

func outerLoggingMiddleware(next rpc.MiddlewareHandler) rpc.MiddlewareHandler {
	return func(ctx context.Context, writer rpc.ResponseWriter, request *rpc.Request) {
		fmt.Println("outer logger: pre-action")
		next(ctx, writer, request)
		fmt.Println("outer logger: post-action")
	}
}

func innerLoggingMiddleware(next rpc.MiddlewareHandler) rpc.MiddlewareHandler {
	return func(ctx context.Context, writer rpc.ResponseWriter, request *rpc.Request) {
		fmt.Println("  inner logger: pre-action")
		next(ctx, writer, request)
		fmt.Println("  inner logger: post-action")
	}
}

func checkerMiddleware(next rpc.MiddlewareHandler) rpc.MiddlewareHandler {
	return func(ctx context.Context, writer rpc.ResponseWriter, request *rpc.Request) {
		fmt.Println("    request header: x-test-header =", request.Header.Get("x-test-header"))
		ctx = context.WithValue(ctx, "received-x-test-header", request.Header.Get("x-test-header"))
		next(ctx, writer, request)
	}
}

// endregion
// region - start test server

func startTestServer(ctx context.Context) error {

	svr := NewRpcServer(
		ServerWithAddress("0.0.0.0"),
		ServerWithPort(2345),
		ServerWithCryptoKey([]byte("0123456789ABCDEF")),
		ServerWithMiddleware(outerLoggingMiddleware),
		ServerWithMiddleware(innerLoggingMiddleware),
		ServerWithMiddleware(checkerMiddleware),
	)

	if err := svr.RegisterName("TestServer", new(Server)); err != nil {
		panic(err)
	}
	go func() { _ = svr.Accept(ctx) }()

	time.Sleep(time.Millisecond * 50)
	return nil

}

// endregion

func TestClientServerIntegration(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	_ = startTestServer(ctx)
	defer cancel()

	c := NewRpcClient(
		ClientWithAddress("127.0.0.1"),
		ClientWithPort(2345),
		ClientWithCryptoKey([]byte("0123456789ABCDEF")),
	)
	client := Client{
		CustomRpcClient: c,
	}

	rsp, err := client.Hello(ctx, "Test")
	assert.NoError(t, err)
	assert.NotEmpty(t, rsp)
	assert.Equal(t, "Hello, Test!", rsp)
	fmt.Println("response:", rsp)

}
