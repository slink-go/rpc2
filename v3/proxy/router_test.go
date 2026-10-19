// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticRouterResolve(t *testing.T) {
	t.Parallel()
	r, err := NewStaticRouter(map[string]Target{
		"mailer": {Addr: "mailer.local", Port: 2443},
		"miki":   {Addr: "10.0.0.5", Port: 0, Transport: "json"},
	})
	require.NoError(t, err)

	tgt, err := r.Resolve("mailer")
	require.NoError(t, err)
	assert.Equal(t, "mailer.local", tgt.Addr)
	assert.Equal(t, 2443, tgt.Port)

	tgt, err = r.Resolve("miki")
	require.NoError(t, err)
	assert.Equal(t, "json", tgt.Transport)

	_, err = r.Resolve("ghost")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestNewStaticRouterValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		targets map[string]Target
	}{
		{"пустое имя сервиса", map[string]Target{"": {Addr: "h"}}},
		{"пустой addr", map[string]Target{"mailer": {Port: 1}}},
		{"порт за диапазоном", map[string]Target{"mailer": {Addr: "h", Port: 70000}}},
		{"отрицательный порт", map[string]Target{"mailer": {Addr: "h", Port: -1}}},
		{"плохой transport", map[string]Target{"mailer": {Addr: "h", Transport: "http"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewStaticRouter(tc.targets)
			require.Error(t, err)
		})
	}
}

func TestStaticRouterAcceptsValidTransports(t *testing.T) {
	t.Parallel()
	for _, tr := range []string{"", "auto", "json", "legacy"} {
		_, err := NewStaticRouter(map[string]Target{"s": {Addr: "h", Transport: tr}})
		require.NoError(t, err, "transport %q", tr)
	}
}

func TestStaticRouterTargetsCopy(t *testing.T) {
	t.Parallel()
	r, err := NewStaticRouter(map[string]Target{"mailer": {Addr: "h", Port: 1}})
	require.NoError(t, err)
	out := r.Targets()
	out["mailer"] = Target{Addr: "mutated"}
	tgt, err := r.Resolve("mailer")
	require.NoError(t, err)
	assert.Equal(t, "h", tgt.Addr, "Targets() must return a copy")
}
