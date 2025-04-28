package rpc2

import (
	"bufio"
	"fmt"
	"go.slink.ws/logging"
	"go.slink.ws/rpc2/codec"
	"net"
	"net/rpc"
)

type CustomRpcClient struct {
	address   string
	port      int
	cryptoKey []byte
	handler   ServerHandler
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
func (c *CustomRpcClient) Call(method string, args interface{}, reply interface{}) error {

	// Client With Codec
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", c.address, c.port))
	if err != nil {
		return err
	}
	cl := rpc.NewClientWithCodec(codec.GetClientCodec(bufio.NewWriter(conn), conn, c.cryptoKey))
	defer func() { _ = cl.Close(); _ = conn.Close() }()

	return cl.Call(method, args, reply)
}
