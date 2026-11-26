package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

type memSeeker struct {
	buf []byte
	pos int64
}

func (m *memSeeker) Write(p []byte) (int, error) {
	end := m.pos + int64(len(p))
	if end > int64(len(m.buf)) {
		grown := make([]byte, end)
		copy(grown, m.buf)
		m.buf = grown
	}
	copy(m.buf[m.pos:], p)
	m.pos = end
	return len(p), nil
}

func (m *memSeeker) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = m.pos + offset
	case io.SeekEnd:
		next = int64(len(m.buf)) + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if next < 0 {
		return 0, errors.New("negative position")
	}
	m.pos = next
	return next, nil
}

func TestWAVWriterAndClamp(t *testing.T) {
	var ms memSeeker
	w, err := NewWAVWriter(&ms)
	if err != nil {
		t.Fatalf("NewWAVWriter: %v", err)
	}
	if err := w.Write([]float32{0, 1.5, -1.5}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(ms.buf) != 44+6 {
		t.Fatalf("len = %d, want 50", len(ms.buf))
	}
	if !bytes.Equal(ms.buf[0:4], []byte("RIFF")) || !bytes.Equal(ms.buf[8:12], []byte("WAVE")) {
		t.Fatalf("bad RIFF/WAVE header: %q", ms.buf[:12])
	}
	if got := binary.LittleEndian.Uint32(ms.buf[4:8]); got != 42 {
		t.Fatalf("RIFF size = %d, want 42", got)
	}
	if got := binary.LittleEndian.Uint32(ms.buf[40:44]); got != 6 {
		t.Fatalf("data size = %d, want 6", got)
	}
	if s0 := int16(binary.LittleEndian.Uint16(ms.buf[44:46])); s0 != 0 {
		t.Fatalf("sample 0 = %d, want 0", s0)
	}
	if s1 := int16(binary.LittleEndian.Uint16(ms.buf[46:48])); s1 != 32767 {
		t.Fatalf("sample 1 = %d, want 32767", s1)
	}
	if s2 := int16(binary.LittleEndian.Uint16(ms.buf[48:50])); s2 != -32768 {
		t.Fatalf("sample 2 = %d, want -32768", s2)
	}
}
