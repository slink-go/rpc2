// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

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
