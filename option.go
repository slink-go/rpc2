package rpc2

type RpcConfig struct {
	Key     []byte
	Address string
	Port    int
}

func DefaultRpcConfig() *RpcConfig {
	return &RpcConfig{
		Key:     nil,
		Address: "0.0.0.0",
		Port:    2233,
	}
}

type RpcOption interface {
	Apply(*RpcConfig)
}

// region - crypto key

type cryptoKeyOpt struct {
	value []byte
}

func (c *cryptoKeyOpt) Apply(s *RpcConfig) {
	if len(c.value) > 0 {
		s.Key = c.value
	}
}
func WithCryptoKey(v []byte) RpcOption {
	return &cryptoKeyOpt{v}
}

// endregion
// region - RPC address

type addressOpt struct {
	value string
}

func (c *addressOpt) Apply(s *RpcConfig) {
	s.Address = c.value
}
func WithAddress(v string) RpcOption {
	return &addressOpt{v}
}

// endregion
// region - RPC port

type portOpt struct {
	value int
}

func (c *portOpt) Apply(s *RpcConfig) {
	s.Port = c.value
}
func WithPort(v int) RpcOption {
	return &portOpt{v}
}

// endregion
