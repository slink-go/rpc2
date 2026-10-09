package codec

import (
	"bufio"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"go.slink.ws/rpc"
	v2codec "go.slink.ws/rpc2/v2/codec"
)

// Бенчмарк T1.8: gob (текущий путь v2) vs encoding/json (новый путь v3) на
// репликах реальных DTO проекта. Импорт самих сервисов создал бы цикл модулей,
// поэтому DTO скопированы 1:1 по полям и тегам.
//
// Размеры — примерный размер payload-строки (100B/1K/10K/100KB); фактический
// wire size меряется по проводу и репортится метрикой wireB/op (стэйт после
// прогрева: gob-энкодер шлёт type descriptors только в первом сообщении).

// BenchSendTemplateRequest — копия svc/misc/mailer/internal/controller/jrpc/dto.go.
type BenchSendTemplateRequest struct {
	To       string         `json:"to"`
	Template string         `json:"template"`
	Params   map[string]any `json:"params"`
}

// benchActionRequest — копия kansi model.ActionRequest.
type benchActionRequest struct {
	ObjectId string `json:"object_id"`
	UserId   string `json:"user_id"`
	TenantId string `json:"tenant_id"`
}

// BenchRpcActionRequest — копия kansi dto.RpcActionRequest.
type BenchRpcActionRequest struct {
	Value benchActionRequest `json:"rq"`
}

// benchErrorMsg — в оригинале ErrorResponse.Error это интерфейс error; для
// бенчмарка используется конкретный тип (JSON не умеет unmarshal в interface).
type benchErrorMsg struct {
	Msg string
}

func (e *benchErrorMsg) Error() string { return e.Msg }

// BenchErrorResponse — копия kansi dto.ErrorResponse.
type BenchErrorResponse struct {
	Error *benchErrorMsg `json:"error,omitempty"`
}

var benchKey = []byte("0123456789ABCDEF")

type benchDTO struct {
	name string
	val  any
}

type benchSize struct {
	label string
	n     int
}

func benchSizes() []benchSize {
	return []benchSize{
		{"100B", 100},
		{"1K", 1 << 10},
		{"10K", 10 << 10},
		{"100K", 100 << 10},
	}
}

func newBenchDTOs(size int) []benchDTO {
	pad := strings.Repeat("x", size)
	return []benchDTO{
		{"SendTemplateRequest", BenchSendTemplateRequest{
			To:       "ops@example.com",
			Template: "welcome",
			Params:   map[string]any{"name": "Ada", "payload": pad},
		}},
		{"RpcActionRequest", BenchRpcActionRequest{
			Value: benchActionRequest{
				ObjectId: "obj-" + pad,
				UserId:   "user-0001",
				TenantId: "tenant-0001",
			},
		}},
		{"ErrorResponse", BenchErrorResponse{
			Error: &benchErrorMsg{Msg: pad},
		}},
	}
}

// newBenchPair собирает пару кодеков поверх memConn в обе стороны.
// Варианты: json | jsonCrypto (v3) и gob | gobCrypto (legacy v2).
func newBenchPair(variant string) (rpc.ServerCodec, rpc.ClientCodec, *memConn, *memConn) {
	srvConn, cliConn := newMemPair()
	switch variant {
	case "json":
		srv, cli, s, c := newJSONPair(0)
		return srv, cli, s, c
	case "jsonCrypto":
		srv, err := newJSONCryptoServerCodec(bufio.NewWriter(srvConn), srvConn, benchKey, 0)
		if err != nil {
			panic(err)
		}
		cli, err := newJSONCryptoClientCodec(bufio.NewWriter(cliConn), cliConn, benchKey, 0)
		if err != nil {
			panic(err)
		}
		return srv, cli, srvConn, cliConn
	case "gob":
		srv, err := v2codec.NewServerCodec(bufio.NewWriter(srvConn), srvConn, nil)
		if err != nil {
			panic(err)
		}
		cli, err := v2codec.NewClientCodec(bufio.NewWriter(cliConn), cliConn, nil)
		if err != nil {
			panic(err)
		}
		return srv, cli, srvConn, cliConn
	case "gobCrypto":
		srv, err := v2codec.NewServerCodec(bufio.NewWriter(srvConn), srvConn, benchKey)
		if err != nil {
			panic(err)
		}
		cli, err := v2codec.NewClientCodec(bufio.NewWriter(cliConn), cliConn, benchKey)
		if err != nil {
			panic(err)
		}
		return srv, cli, srvConn, cliConn
	default:
		panic("unknown variant: " + variant)
	}
}

// BenchmarkRoundTrip — полный цикл вызова: WriteRequest → Read(header+body) →
// WriteResponse → Read(header+body). Метрики: wireB/op (размер запроса на
// проводе, последняя итерация), B/op и allocs/op из ReportAllocs.
func BenchmarkRoundTrip(b *testing.B) {
	for _, variant := range []string{"json", "jsonCrypto", "gob", "gobCrypto"} {
		for _, size := range benchSizes() {
			for _, d := range newBenchDTOs(size.n) {
				b.Run(fmt.Sprintf("%s/%s/%s", variant, d.name, size.label), func(b *testing.B) {
					srv, cli, _, cliConn := newBenchPair(variant)
					req := &rpc.Request{ServiceMethod: "Bench.Call", ID: "req-1"}
					outPtr := reflect.New(reflect.TypeOf(d.val))
					var wire int

					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := cli.WriteRequest(req, d.val); err != nil {
							b.Fatal(err)
						}
						wire = cliConn.wr.Len()

						var hdr rpc.Request
						if err := srv.ReadRequestHeader(&hdr); err != nil {
							b.Fatal(err)
						}
						if err := srv.ReadRequestBody(outPtr.Interface()); err != nil {
							b.Fatal(err)
						}
						if err := srv.WriteResponse(&rpc.Response{ID: hdr.ID}, d.val); err != nil {
							b.Fatal(err)
						}
						var rhdr rpc.Response
						if err := cli.ReadResponseHeader(&rhdr); err != nil {
							b.Fatal(err)
						}
						if err := cli.ReadResponseBody(outPtr.Interface()); err != nil {
							b.Fatal(err)
						}
					}
					b.StopTimer()
					b.ReportMetric(float64(wire), "wireB/op")
				})
			}
		}
	}
}
