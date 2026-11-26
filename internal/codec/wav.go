package codec

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

// WAVWriter writes mono 16-bit 48 kHz PCM. The header sizes are fixed on Close,
// so it needs a WriteSeeker (a file).
type WAVWriter struct {
	w     io.WriteSeeker
	bytes int64
	buf   []byte
}

func NewWAVWriter(w io.WriteSeeker) (*WAVWriter, error) {
	ww := &WAVWriter{w: w}
	return ww, ww.header()
}

func (ww *WAVWriter) header() error {
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(min(36+ww.bytes, math.MaxUint32)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], 1) // mono
	binary.LittleEndian.PutUint32(h[24:], SampleRate)
	binary.LittleEndian.PutUint32(h[28:], SampleRate*2)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(min(ww.bytes, math.MaxUint32-36)))
	_, err := ww.w.Write(h)
	return err
}

func (ww *WAVWriter) Write(samples []float32) error {
	if cap(ww.buf) < len(samples)*2 {
		ww.buf = make([]byte, len(samples)*2)
	}
	b := ww.buf[:len(samples)*2]
	for i, s := range samples {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(toInt16(s)))
	}
	n, err := ww.w.Write(b)
	ww.bytes += int64(n)
	return err
}

func (ww *WAVWriter) Close() error {
	if _, err := ww.w.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := ww.header(); err != nil {
		return err
	}
	if ww.bytes > math.MaxUint32-36 {
		return errors.New("wav file too large (over 4 GB)")
	}
	_, err := ww.w.Seek(0, io.SeekEnd)
	return err
}

func toInt16(s float32) int16 {
	v := s * 32767
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return int16(v)
}
