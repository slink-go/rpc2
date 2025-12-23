package rpc2

import "go.slink.ws/logging"

type ClientOption func(*CustomRpcClient)

func ClientWithCryptoKey(value []byte) ClientOption {
	return func(s *CustomRpcClient) {
		s.cryptoKey = value
	}
}

func ClientWithAddress(value string) ClientOption {
	return func(s *CustomRpcClient) {
		s.address = value
	}
}

func ClientWithPort(value int) ClientOption {
	return func(s *CustomRpcClient) {
		s.port = value
	}
}

func ClientWithLogger(value logging.Logger) ClientOption {
	return func(s *CustomRpcClient) {
		s.logger = value
	}
}
