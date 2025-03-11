package rpc2

import (
	"bufio"
	"fmt"
	"github.com/slink-go/logging"
	"github.com/slink-go/rpc2/codec"
	"net"
	"net/rpc"
)

type CustomRpcClient struct {
	logger    logging.Logger
	addr      string
	cryptoKey []byte
}

func NewRpcClient(opts ...RpcOption) *CustomRpcClient {
	cf := &RpcConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt.Apply(cf)
		}
	}
	server := &CustomRpcClient{
		logger:    logging.GetLogger("rpc-client"),
		cryptoKey: cf.Key,
		addr:      fmt.Sprintf("%v:%v", cf.Address, cf.Port),
	}
	return server
}
func (c *CustomRpcClient) Call(method string, args interface{}, reply interface{}) error {

	// Client With Codec
	conn, err := net.Dial("tcp", c.addr)
	if err != nil {
		return err
	}
	cl := rpc.NewClientWithCodec(codec.GetClientCodec(bufio.NewWriter(conn), conn, c.cryptoKey))
	defer func() { _ = cl.Close(); _ = conn.Close() }()

	return cl.Call(method, args, reply)
}
