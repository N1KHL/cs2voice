package audio

import (
	"bufio"
	"fmt"
	"io"

	"cs2voice/internal/codec"
)

type Format string

const (
	MP3 Format = "mp3"
	WAV Format = "wav"
)

const mp3Kbps = 96 // mono voice: clear and small (~0.7 MB per minute)

// WriteTo renders the plan in the given format into an io.WriteSeeker.
func (p *Plan) WriteTo(out io.WriteSeeker, f Format, waveRate int) (*Waveform, error) {
	switch f {
	case WAV:
		w, err := codec.NewWAVWriter(out)
		if err != nil {
			return nil, err
		}
		wave, err := p.Render(w.Write, waveRate)
		if err != nil {
			return nil, err
		}
		return wave, w.Close()

	case MP3:
		bw := bufio.NewWriterSize(out, 1<<16)
		e, err := codec.NewMP3Encoder(bw, mp3Kbps)
		if err != nil {
			return nil, err
		}
		wave, err := p.Render(e.Write, waveRate)
		if err != nil {
			e.Close()
			return nil, err
		}
		if err := e.Close(); err != nil {
			return nil, err
		}
		return wave, bw.Flush()
	}
	return nil, fmt.Errorf("unknown format %q", f)
}
