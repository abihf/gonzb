package downloader

import (
	"io"
	"os"
	"sync"
)

type TempBuff struct {
	MaxSize int
	used    int
	mutex   sync.Mutex
}

type ReadWriteSeekCloser interface {
	io.ReadWriteSeeker
	io.Closer
}

func (t *TempBuff) Allocate(name string, size int) (ReadWriteSeekCloser, error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.used+size < t.MaxSize {
		t.used += size
		return t.newMemBuff(size), nil
	}
	f, err := os.CreateTemp(".", name+"-*")
	return &tempFileWrapper{f}, err
}

type tempFileWrapper struct {
	*os.File
}

func (t *tempFileWrapper) Close() error {
	t.File.Close()
	return os.Remove(t.File.Name())
}

type memBuff struct {
	t *TempBuff

	buf    []byte
	offset int64
	size   int64
}

var _ io.ReaderFrom = &memBuff{}
var _ io.WriterTo = &memBuff{}

func (t *TempBuff) newMemBuff(size int) *memBuff {
	return &memBuff{
		t:    t,
		buf:  make([]byte, size),
		size: int64(size),
	}
}

func (m *memBuff) Read(out []byte) (int, error) {
	if m.offset >= m.size {
		return 0, io.EOF
	}
	n := copy(out, m.buf[m.offset:])
	m.offset += int64(n)
	return n, nil
}

func (m *memBuff) ReadFrom(r io.Reader) (int64, error) {
	n, err := r.Read(m.buf[m.offset:])
	return int64(n), err
}

func (m *memBuff) Write(p []byte) (int, error) {
	n := copy(m.buf[m.offset:], p[:])
	m.offset += int64(n)
	return n, nil
}

func (m *memBuff) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(m.buf[m.offset:])
	return int64(n), err
}

func (m *memBuff) Close() error {
	m.t.mutex.Lock()
	defer m.t.mutex.Unlock()
	m.t.used -= int(m.size)
	return nil
}

func (m *memBuff) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekCurrent:
		m.offset += offset
	case io.SeekStart:
		m.offset = offset
	case io.SeekEnd:
		m.offset = offset + m.size
	}
	if m.offset < 0 {
		m.offset = 0
	} else if m.offset >= m.size {
		m.offset = m.size - 1
	}
	return m.offset, nil
}
