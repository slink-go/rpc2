package codec

import (
	"bufio"
	"bytes"
	"io"
)

type BytesReadWriteCloser struct {
	buffer *bytes.Buffer
}

func NewBytesReadWriteCloser() *BytesReadWriteCloser {
	var buffer []byte
	return &BytesReadWriteCloser{
		buffer: bytes.NewBuffer(buffer),
	}
}
func NewBytesReadWriteCloserWithBuffer(buffer []byte) *BytesReadWriteCloser {
	return &BytesReadWriteCloser{
		buffer: bytes.NewBuffer(buffer),
	}
}

func (b *BytesReadWriteCloser) Len() int {
	return b.buffer.Len()
}
func (b *BytesReadWriteCloser) Bytes() []byte {
	return b.buffer.Bytes()
}
func (b *BytesReadWriteCloser) ReadAll() []byte {
	var result []byte
	for {
		b, err := b.buffer.ReadByte()
		if err != nil {
			break
		}
		result = append(result, b)
	}
	return result
}
func (b *BytesReadWriteCloser) Read(p []byte) (n int, err error) {
	return b.buffer.Read(p)
}
func (b *BytesReadWriteCloser) Write(p []byte) (n int, err error) {
	return b.buffer.Write(p)
}
func (b *BytesReadWriteCloser) Close() error {
	return nil
}

type BufferedReadWriteCloser struct {
	*bufio.Reader
	io.ReadWriteCloser
}

func newBufferedReadWriteCloser(r io.ReadWriteCloser) *BufferedReadWriteCloser {
	return &BufferedReadWriteCloser{
		Reader:          bufio.NewReader(r),
		ReadWriteCloser: r,
	}
}
func (rw *BufferedReadWriteCloser) ReadAll() []byte {
	var result []byte
	for {
		b, err := rw.Reader.ReadByte()
		if err != nil {
			break
		}
		result = append(result, b)
	}
	return result
}
func (rw *BufferedReadWriteCloser) Read(p []byte) (int, error) {
	return rw.Reader.Read(p)
}
func (rw *BufferedReadWriteCloser) Write(p []byte) (n int, err error) {
	return rw.ReadWriteCloser.Write(p)
}
func (rw *BufferedReadWriteCloser) Close() (err error) {
	return rw.ReadWriteCloser.Close()
}
func (rw *BufferedReadWriteCloser) Peek(n int) (p []byte, err error) {
	return rw.Reader.Peek(n)
}
func (rw *BufferedReadWriteCloser) Discard(n int) (discarded int, err error) {
	return rw.Reader.Discard(n)
}
func (rw *BufferedReadWriteCloser) Size() int {
	return rw.Reader.Buffered()
}
