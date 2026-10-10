package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("PROXY_TARGET_MAILER", "mailer.local:2443")
	t.Setenv("PROXY_TARGET_MAILER_TRANSPORT", "json")
	t.Setenv("PROXY_TARGET_MAILER_SECRET", "0011aabb")
	t.Setenv("PROXY_TARGET_MIKI", "miki.local")
	t.Setenv("UNRELATED_PROXY_TARGET_X", "skip")

	cfg, err := ConfigFromEnv()
	require.NoError(t, err)

	mailer, ok := cfg.Targets["mailer"]
	require.True(t, ok)
	assert.Equal(t, "mailer.local", mailer.Addr)
	assert.Equal(t, 2443, mailer.Port)
	assert.Equal(t, "json", mailer.Transport)
	assert.Equal(t, []byte{0x00, 0x11, 0xaa, 0xbb}, mailer.Secret)

	miki, ok := cfg.Targets["miki"]
	require.True(t, ok)
	assert.Equal(t, "miki.local", miki.Addr)
	assert.Equal(t, defaultTargetPort, miki.Port)
	assert.Equal(t, "", miki.Transport)
	assert.Nil(t, miki.Secret)

	assert.Len(t, cfg.Targets, 2, "переменные без префикса игнорируются")
}

func TestConfigFromEnvErrors(t *testing.T) {
	cases := []struct {
		name  string
		env   map[string]string
		match string
	}{
		{
			"пустое имя сервиса",
			map[string]string{"PROXY_TARGET_": "h:1"},
			"without service name",
		},
		{
			"нет адреса, задан только transport",
			map[string]string{"PROXY_TARGET_MAILER_TRANSPORT": "json"},
			"address for service",
		},
		{
			"плохой hex-секрет",
			map[string]string{"PROXY_TARGET_MAILER": "h:1", "PROXY_TARGET_MAILER_SECRET": "zz"},
			"invalid hex",
		},
		{
			"пустой адрес",
			map[string]string{"PROXY_TARGET_MAILER": ""},
			"empty target address",
		},
		{
			"несуществующий порт",
			map[string]string{"PROXY_TARGET_MAILER": "h:abc"},
			"invalid port",
		},
		{
			"порт вне диапазона",
			map[string]string{"PROXY_TARGET_MAILER": "h:99999"},
			"out of range",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, err := ConfigFromEnv()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.match)
		})
	}
}

func TestSplitHostPortDefault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		host string
		port int
		ok   bool
	}{
		{"h:1234", "h", 1234, true},
		{"h", "h", defaultTargetPort, true},
		{"[::1]:2233", "::1", 2233, true},
		{"10.0.0.1:1", "10.0.0.1", 1, true},
		{"", "", 0, false},
		{"h:0", "", 0, false},
		{"h:65536", "", 0, false},
	}
	for _, tc := range cases {
		host, port, err := splitHostPortDefault(tc.in)
		if !tc.ok {
			require.Error(t, err, "in=%q", tc.in)
			continue
		}
		require.NoError(t, err, "in=%q", tc.in)
		assert.Equal(t, tc.host, host)
		assert.Equal(t, tc.port, port)
	}
}

func TestConfigFromEnvAllowlist(t *testing.T) {
	t.Setenv("PROXY_TARGET_MAILER", "h:1")
	t.Setenv("PROXY_ALLOWLIST", "miki.GetProgram, kata.* ,")

	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, []string{"miki.GetProgram", "kata.*"}, cfg.Allowlist)

	t.Setenv("PROXY_ALLOWLIST", "")
	cfg, err = ConfigFromEnv()
	require.NoError(t, err)
	assert.Nil(t, cfg.Allowlist)
}
