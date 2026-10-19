// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.slink.ws/rpc"
)

func TestMapCallError(t *testing.T) {
	t.Parallel()
	const target = "svc:1234"
	cases := []struct {
		name    string
		err     error
		code    string
		contain string
	}{
		{"ServerError → backend", rpc.ServerError("method not found"), CodeBackend, "method not found"},
		{"EOF → nojson + hint", io.EOF, CodeNoJSON, "gob-only"},
		{"ErrUnexpectedEOF → nojson", io.ErrUnexpectedEOF, CodeNoJSON, "does not support JSON"},
		{"ErrClosedPipe → nojson", io.ErrClosedPipe, CodeNoJSON, "does not support JSON"},
		{"дедлайн → transport timeout", context.DeadlineExceeded, CodeTransport, "call timeout"},
		{"отмена → transport canceled", context.Canceled, CodeTransport, "call canceled"},
		{"net error → transport", &net.OpError{Op: "dial", Err: errors.New("refused")}, CodeTransport, "refused"},
		{"прочее → transport", errors.New("weird"), CodeTransport, "weird"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ce := mapCallError(tc.err, target)
			assert.NotNil(t, ce)
			assert.Equal(t, tc.code, ce.Code)
			assert.Equal(t, target, ce.Target)
			assert.Contains(t, ce.Message, tc.contain)
		})
	}
}
