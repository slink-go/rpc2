package rpc2

import "strings"

type Meta struct {
	Records map[string]interface{} `json:"fields"`
}

func NewMeta() *Meta {
	return &Meta{make(map[string]interface{})}
}
func (m *Meta) Set(key string, value any) {
	m.Records[strings.ToUpper(key)] = value
}
func (m *Meta) Get(key string) interface{} {
	v, ok := m.Records[strings.ToUpper(key)]
	if !ok {
		return nil
	}
	return v
}

type Request[T any] struct {
	Meta Meta
	Data T
}
