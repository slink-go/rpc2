package rpc2

import "net/rpc"

type ServerHandler interface {
	Handle(codec rpc.ServerCodec)
}
