package rpc2

import (
	"bufio"
	"context"
	"fmt"
	"github.com/renevo/rpc"
	"go.slink.ws/logging"
	"go.slink.ws/rpc2/codec"
	"net"
)

type CustomRpcClient struct {
	address   string
	port      int
	cryptoKey []byte
	logger    logging.Logger
}

func NewRpcClient(opts ...ClientOption) *CustomRpcClient {
	client := &CustomRpcClient{
		logger:  logging.GetLogger("rpc-client"),
		address: "127.0.0.1",
		port:    2233,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}
func (c *CustomRpcClient) Call(ctx context.Context, method string, args interface{}, reply interface{}) error {

	// Client With Codec
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", c.address, c.port))
	if err != nil {
		return err
	}
	cl := rpc.NewClientWithCodec(codec.GetClientCodec(bufio.NewWriter(conn), conn, c.cryptoKey))
	defer func() { _ = cl.Close(); _ = conn.Close() }()

	return cl.Call(ctx, method, args, reply)

}
