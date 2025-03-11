package codec

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestBufferedIo(t *testing.T) {

	buffer := bytes.Buffer{}
	assert.Equal(t, 0, buffer.Len())
	n, e := buffer.Write([]byte("test"))
	assert.NoErrorf(t, e, "Write error: %v", e)
	assert.Equalf(t, 4, n, "Expected 4 bytes written, got: %v", n)
	assert.Equal(t, 4, buffer.Len())

	p := buffer.Bytes()
	assert.Equalf(t, "test", string(p), "Expected 'test', got: '%v'", string(p))

	p = make([]byte, 2, 2)
	n, e = buffer.Read(p)
	assert.Equalf(t, 2, n, "Expected 4 bytes read, got: %v", n)
	assert.Equalf(t, 2, len(p), "Expected 4 bytes target array len, got: %v", len(p))
	assert.Equalf(t, "te", string(p), "Expected 'te', got: '%v'", string(p))

}

func TestReadWrite(t *testing.T) {

	rwc := newBufferedReadWriteCloser(NewBytesReadWriteCloser())

	n, e := rwc.Write([]byte("test"))
	assert.NoErrorf(t, e, "Write error: %v", e)
	assert.Equalf(t, 4, n, "Expected 4 bytes written, got: %v", n)

	var p = make([]byte, 16)
	n, e = rwc.Read(p)
	assert.NoErrorf(t, e, "Read error: %v", e)
	assert.Equalf(t, 4, n, "Expected 4 bytes read, got: %v", n)
	assert.Equalf(t, "test", string(p[:n]), "Expected 'test', got: '%v'", string(p[:n]))

	_, _ = rwc.Write([]byte("test 2"))
	p = rwc.ReadAll()
	assert.Equalf(t, "test 2", string(p), "Expected 'test 2', got: '%v'", string(p))

}
