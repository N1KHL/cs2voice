package audio

import (
	"errors"
	"io"
)

// memWriteSeeker is an in-memory io.WriteSeeker used to build WAV files
// without touching the filesystem (e.g. inside the browser WASM worker).
type memWriteSeeker struct {
	buf []byte
	pos int64
}

func (m *memWriteSeeker) Write(p []byte) (int, error) {
	if m.pos == int64(len(m.buf)) {
		m.buf = append(m.buf, p...)
		m.pos = int64(len(m.buf))
		return len(p), nil
	}
	end := m.pos + int64(len(p))
	if end > int64(len(m.buf)) {
		if end <= int64(cap(m.buf)) {
			m.buf = m.buf[:end]
		} else {
			grown := make([]byte, end, max(end, int64(cap(m.buf))*2))
			copy(grown, m.buf)
			m.buf = grown
		}
	}
	copy(m.buf[m.pos:], p)
	m.pos = end
	return len(p), nil
}

func (m *memWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = m.pos + offset
	case io.SeekEnd:
		next = int64(len(m.buf)) + offset
	default:
		return 0, errors.New("memWriteSeeker: invalid whence")
	}
	if next < 0 {
		return 0, errors.New("memWriteSeeker: negative position")
	}
	m.pos = next
	return next, nil
}

// RenderWAV renders the plan into an in-memory WAV byte slice and optional waveform.
func (p *Plan) RenderWAV(waveRate int) ([]byte, *Waveform, error) {
	var mem memWriteSeeker
	if p.Length > 0 {
		mem.buf = make([]byte, 0, 44+p.Length*2)
	}
	wave, err := p.WriteTo(&mem, WAV, waveRate)
	if err != nil {
		return nil, nil, err
	}
	return mem.buf, wave, nil
}
