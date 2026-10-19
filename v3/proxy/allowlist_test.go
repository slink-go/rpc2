// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllowlistDefaultAllow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		entries []string
		policy  AllowlistPolicy
		method  string
		allowed bool
	}{
		{"nil-allowlist: любой метод", nil, PolicyDefault, "ghost.Do", true},
		{"пустой список: любой метод", []string{}, PolicyDefault, "ghost.Do", true},
		{"список из пробелов: любой метод", []string{"", "  "}, PolicyDefault, "ghost.Do", true},
		{"точное совпадение", []string{"miki.GetProgram"}, PolicyDefault, "miki.GetProgram", true},
		{"метод вне списка", []string{"miki.GetProgram"}, PolicyDefault, "miki.DeleteProgram", false},
		{"другой сервис", []string{"miki.GetProgram"}, PolicyDefault, "mailer.Send", false},
		{"wildcard-сервис", []string{"miki.*"}, PolicyDefault, "miki.Anything", true},
		{"wildcard-сервис: другой метод с точкой", []string{"miki.*"}, PolicyDefault, "miki.A.B", true},
		{"wildcard не пускает другой сервис", []string{"miki.*"}, PolicyDefault, "mailer.Send", false},
		{"wildcard не пускает без метода", []string{"miki.*"}, PolicyDefault, "miki.", false},
		{"смешанный список", []string{"mailer.Send", "kata.*"}, PolicyDefault, "kata.Create", true},
		{"PolicyDenyEmpty: пустой список — deny", []string{}, PolicyDenyEmpty, "ghost.Do", false},
		{"PolicyDenyEmpty: непустой — фильтр", []string{"miki.GetProgram"}, PolicyDenyEmpty, "miki.GetProgram", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, err := NewAllowlist(tc.entries, tc.policy)
			require.NoError(t, err)
			err = a.Allowed(tc.method)
			if tc.allowed {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestNewAllowlistValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		entries []string
	}{
		{"без точки", []string{"miki"}},
		{"точка в начале", []string{".miki"}},
		{"точка в конце", []string{"miki."}},
		{"две точки", []string{"a.b.c"}},
		{"wildcard не по сервису", []string{"a.b.*"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewAllowlist(tc.entries, PolicyDefault)
			require.Error(t, err)
		})
	}
}

func TestAllowlistEntries(t *testing.T) {
	t.Parallel()
	a, err := NewAllowlist([]string{"miki.GetProgram", "miki.GetProgram", "mailer.*"}, PolicyDefault)
	require.NoError(t, err)
	entries := a.Entries()
	assert.ElementsMatch(t, []string{"miki.GetProgram", "mailer.*"}, entries)
}
