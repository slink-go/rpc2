package codec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.slink.ws/rpc"
)

// memConn — in-memory duplex-соединение для последовательных codec-тестов
// (без блокировки: записи сразу видны читателю, чтение на пустом буфере — EOF).
type memConn struct {
	rd, wr *bytes.Buffer
}

func newMemPair() (a, b *memConn) {
	ab := &bytes.Buffer{}
	ba := &bytes.Buffer{}
	return &memConn{rd: ba, wr: ab}, &memConn{rd: ab, wr: ba}
}

func (m *memConn) Read(p []byte) (int, error)  { return m.rd.Read(p) }
func (m *memConn) Write(p []byte) (int, error) { return m.wr.Write(p) }
func (m *memConn) Close() error                { return nil }

func newJSONPair(maxBody int64) (rpc.ServerCodec, rpc.ClientCodec, *memConn, *memConn) {
	srvConn, cliConn := newMemPair()
	srv := newJSONServerCodec(bufio.NewWriter(srvConn), srvConn, maxBody)
	cli := newJSONClientCodec(bufio.NewWriter(cliConn), cliConn, maxBody)
	return srv, cli, srvConn, cliConn
}

func newCryptoPair(key []byte, maxBody int64) (rpc.ServerCodec, rpc.ClientCodec) {
	srvConn, cliConn := newMemPair()
	srv, err := newJSONCryptoServerCodec(bufio.NewWriter(srvConn), srvConn, key, maxBody)
	if err != nil {
		panic(err)
	}
	cli, err := newJSONCryptoClientCodec(bufio.NewWriter(cliConn), cliConn, key, maxBody)
	if err != nil {
		panic(err)
	}
	return srv, cli
}

type unitDTO struct {
	ID      string            `json:"id"`
	Count   int               `json:"count"`
	Tags    []string          `json:"tags"`
	Attrs   map[string]string `json:"attrs"`
	Payload []byte            `json:"payload"`
}

func sampleDTO() unitDTO {
	return unitDTO{
		ID:      "юникод-id",
		Count:   42,
		Tags:    []string{"a", "b", "c"},
		Attrs:   map[string]string{"k": "v"},
		Payload: []byte{0x00, 0x01, 0xFF},
	}
}

func request() *rpc.Request {
	return &rpc.Request{ServiceMethod: "Echo.Svc", ID: "req-1"}
}

// TestJSONCodecRoundTrip — полный цикл: запрос (заголовок+тело), ответ
// (заголовок+тело), затем второй вызов на том же потоке.
func TestJSONCodecRoundTrip(t *testing.T) {
	t.Parallel()
	srv, cli, _, _ := newJSONPair(0)

	in := sampleDTO()
	require.NoError(t, cli.WriteRequest(request(), &in))

	var hdr rpc.Request
	require.NoError(t, srv.ReadRequestHeader(&hdr))
	assert.Equal(t, "Echo.Svc", hdr.ServiceMethod)
	var got unitDTO
	require.NoError(t, srv.ReadRequestBody(&got))
	assert.Equal(t, in, got)

	resp := &rpc.Response{ServiceMethod: hdr.ServiceMethod, ID: hdr.ID}
	require.NoError(t, srv.WriteResponse(resp, &got))

	var rhdr rpc.Response
	require.NoError(t, cli.ReadResponseHeader(&rhdr))
	assert.Equal(t, hdr.ID, rhdr.ID)
	var out unitDTO
	require.NoError(t, cli.ReadResponseBody(&out))
	assert.Equal(t, in, out)

	// второй вызов на том же потоке — поток синхронизирован
	in2 := sampleDTO()
	in2.ID = "second"
	require.NoError(t, cli.WriteRequest(request(), &in2))
	require.NoError(t, srv.ReadRequestHeader(&hdr))
	var got2 unitDTO
	require.NoError(t, srv.ReadRequestBody(&got2))
	assert.Equal(t, in2, got2)
}

// TestJSONNilBodyContract — контракт discard: тело читается даже при ошибке
// (ReadRequestBody(nil)/ReadResponseBody(nil)), поток остаётся синхронным.
func TestJSONNilBodyContract(t *testing.T) {
	t.Parallel()
	srv, cli, _, _ := newJSONPair(0)

	// клиент пишет nil-тело (эквивалент error-ответа без payload)
	require.NoError(t, cli.WriteRequest(request(), nil))
	var hdr rpc.Request
	require.NoError(t, srv.ReadRequestHeader(&hdr))
	require.NoError(t, srv.ReadRequestBody(nil))

	// сервер отвечает ошибкой с nil-телом
	require.NoError(t, srv.WriteResponse(&rpc.Response{ID: hdr.ID, Error: "boom"}, nil))
	var rhdr rpc.Response
	require.NoError(t, cli.ReadResponseHeader(&rhdr))
	assert.Equal(t, "boom", rhdr.Error)
	require.NoError(t, cli.ReadResponseBody(nil))

	// следующий вызов работает — синхронность потока сохранена
	in := sampleDTO()
	require.NoError(t, cli.WriteRequest(request(), &in))
	require.NoError(t, srv.ReadRequestHeader(&hdr))
	var got unitDTO
	require.NoError(t, srv.ReadRequestBody(&got))
	assert.Equal(t, in, got)
}

// TestJSONSizeLimitHeader — лимит на размер заголовка.
func TestJSONSizeLimitHeader(t *testing.T) {
	t.Parallel()
	srvConn, peer := newMemPair()
	srv := newJSONServerCodec(bufio.NewWriter(srvConn), srvConn, 64)

	huge := fmt.Sprintf(`{"ServiceMethod":"%s","ID":"1"}`, strings.Repeat("x", 512))
	_, err := peer.Write([]byte(huge))
	require.NoError(t, err)

	var hdr rpc.Request
	err = srv.ReadRequestHeader(&hdr)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBodyTooLarge)
}

// TestJSONSizeLimitBody — лимит на размер тела (защита от OOM).
func TestJSONSizeLimitBody(t *testing.T) {
	t.Parallel()
	srv, cli, _, _ := newJSONPair(128)

	big := unitDTO{ID: strings.Repeat("y", 100<<10)}
	require.NoError(t, cli.WriteRequest(request(), &big))

	var hdr rpc.Request
	require.NoError(t, srv.ReadRequestHeader(&hdr))
	var got unitDTO
	err := srv.ReadRequestBody(&got)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBodyTooLarge)
}

// TestCryptoCodecRoundTrip — симметрия jsonCrypto: заголовок clear, тело под
// CRYPT-фреймингом; враждебный ключ — ошибка расшифровки.
func TestCryptoCodecRoundTrip(t *testing.T) {
	t.Parallel()
	key := []byte("0123456789ABCDEF")
	srv, cli := newCryptoPair(key, 0)

	in := sampleDTO()
	require.NoError(t, cli.WriteRequest(request(), &in))

	var hdr rpc.Request
	require.NoError(t, srv.ReadRequestHeader(&hdr))
	var got unitDTO
	require.NoError(t, srv.ReadRequestBody(&got))
	assert.Equal(t, in, got)

	require.NoError(t, srv.WriteResponse(&rpc.Response{ID: hdr.ID}, &got))
	var rhdr rpc.Response
	require.NoError(t, cli.ReadResponseHeader(&rhdr))
	var out unitDTO
	require.NoError(t, cli.ReadResponseBody(&out))
	assert.Equal(t, in, out)

	// второй вызов: декодер переустановлен после тела — поток синхронизирован
	in2 := sampleDTO()
	in2.ID = "second"
	require.NoError(t, cli.WriteRequest(request(), &in2))
	require.NoError(t, srv.ReadRequestHeader(&hdr))
	var got2 unitDTO
	require.NoError(t, srv.ReadRequestBody(&got2))
	assert.Equal(t, in2, got2)
	require.NoError(t, srv.WriteResponse(&rpc.Response{ID: hdr.ID}, &got2))
	require.NoError(t, cli.ReadResponseHeader(&rhdr))
	var out2 unitDTO
	require.NoError(t, cli.ReadResponseBody(&out2))
	assert.Equal(t, in2, out2)
}

// TestCryptoHeaderIsClearJSON — на проводе после ключа: clear-JSON заголовок,
// затем CRYPT-префикс и gob-блоб тела.
func TestCryptoHeaderIsClearJSON(t *testing.T) {
	t.Parallel()
	key := []byte("0123456789ABCDEF")
	cliConn, _ := newMemPair()
	cli, err := newJSONCryptoClientCodec(bufio.NewWriter(cliConn), cliConn, key, 0)
	require.NoError(t, err)
	dto := sampleDTO()
	require.NoError(t, cli.WriteRequest(request(), &dto))

	raw := cliConn.wr.Bytes()
	require.True(t, bytes.HasPrefix(raw, []byte(`{"ServiceMethod"`)),
		"header must be clear JSON, got %.40q", raw)
	idx := bytes.Index(raw, magicPrefix)
	require.Greater(t, idx, 0, "CRYPT prefix not found after header")
	require.Equal(t, byte('{'), raw[0], "header must start with JSON brace")
}

// TestCryptoWrongKey — тело под чужим ключом не расшифровывается (ошибка, не panic).
func TestCryptoWrongKey(t *testing.T) {
	t.Parallel()
	cliConn, srvConn := newMemPair()
	cli, err := newJSONCryptoClientCodec(bufio.NewWriter(cliConn), cliConn, []byte("0123456789ABCDEF"), 0)
	require.NoError(t, err)
	srv, err := newJSONCryptoServerCodec(bufio.NewWriter(srvConn), srvConn, []byte("FEDCBA9876543210"), 0)
	require.NoError(t, err)

	wrongDTO := sampleDTO()
	require.NoError(t, cli.WriteRequest(request(), &wrongDTO))
	var hdr rpc.Request
	require.NoError(t, srv.ReadRequestHeader(&hdr))
	var got unitDTO
	err = srv.ReadRequestBody(&got)
	require.Error(t, err)
	assert.ErrorContains(t, err, "decrypt")
}

// TestCryptoSizeLimit — лимит размера действует и на шифрованный путь.
func TestCryptoSizeLimit(t *testing.T) {
	t.Parallel()
	key := []byte("0123456789ABCDEF")
	cliConn, srvConn := newMemPair()
	cli, err := newJSONCryptoClientCodec(bufio.NewWriter(cliConn), cliConn, key, 0)
	require.NoError(t, err)
	srv, err := newJSONCryptoServerCodec(bufio.NewWriter(srvConn), srvConn, key, 512)
	require.NoError(t, err)

	big := unitDTO{ID: strings.Repeat("z", 64<<10)}
	require.NoError(t, cli.WriteRequest(request(), &big))
	var hdr rpc.Request
	require.NoError(t, srv.ReadRequestHeader(&hdr))
	var got unitDTO
	err = srv.ReadRequestBody(&got)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBodyTooLarge)
}

// TestJSONServerCodecInterfaces — compile-time гарантии.
func TestJSONServerCodecInterfaces(t *testing.T) {
	t.Parallel()
	assert.Implements(t, (*rpc.ServerCodec)(nil), &jsonServerCodec{})
	assert.Implements(t, (*rpc.ServerCodec)(nil), &jsonCryptoServerCodec{})
	assert.Implements(t, (*rpc.ClientCodec)(nil), &jsonClientCodec{})
	assert.Implements(t, (*rpc.ClientCodec)(nil), &jsonCryptoClientCodec{})
}

// region - fuzz: враждебный JSON не должен ронять процесс

func FuzzRequestHeaderDecode(f *testing.F) {
	seeds := []string{
		`{"ServiceMethod":"A.B","ID":"1"}`,
		`{}`,
		`{"ServiceMethod":`,
		`null`,
		`[1,2,3]`,
		`{"ID":1e9999,"Header":{"a":[1]}}`,
		strings.Repeat(`{"a":`, 100),
		"\x00\x01\x7f{",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var r rpc.Request
		_ = json.Unmarshal(data, &r)
		dec := json.NewDecoder(&budgetReader{r: bytes.NewReader(data), left: 64 << 10})
		var r2 rpc.Request
		_ = dec.Decode(&r2)
	})
}

func FuzzBodyDecode(f *testing.F) {
	f.Add([]byte(`{"id":"1"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"str"`))
	f.Add([]byte(`{"count":1e308}`))
	f.Add([]byte(`[[[[[[[[`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var dto unitDTO
		_ = json.Unmarshal(data, &dto)
		dec := json.NewDecoder(&budgetReader{r: bytes.NewReader(data), left: 4 << 10})
		var dto2 unitDTO
		_ = dec.Decode(&dto2)
		// discard-путь
		var raw json.RawMessage
		dec2 := json.NewDecoder(&budgetReader{r: bytes.NewReader(data), left: 4 << 10})
		_ = dec2.Decode(&raw)
	})
}

// endregion
