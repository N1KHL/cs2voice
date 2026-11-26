//go:build js && wasm

package codec

import (
	"encoding/binary"
	"errors"
	"io"
	"syscall/js"
)

// MP3Encoder encodes mono 48 kHz audio to MP3 via synchronous JS/WASM callbacks.
type MP3Encoder struct {
	handle js.Value
	w      io.Writer
	pcmBuf []byte
	outBuf []byte
}

func NewMP3Encoder(w io.Writer, kbps int) (*MP3Encoder, error) {
	fn := js.Global().Get("cs2mp3Create")
	if fn.Type() != js.TypeFunction {
		return nil, errors.New("mp3 encoder not available in browser yet")
	}
	h := fn.Invoke(SampleRate, kbps)
	if !h.Truthy() {
		return nil, errors.New("cs2mp3Create failed")
	}
	return &MP3Encoder{handle: h, w: w}, nil
}

// Write encodes float samples in [-1, 1].
func (e *MP3Encoder) Write(samples []float32) error {
	if len(samples) == 0 {
		return nil
	}
	fn := js.Global().Get("cs2mp3Encode")
	if fn.Type() != js.TypeFunction {
		return errors.New("mp3 encoder not available in browser yet")
	}
	need := len(samples) * 2
	if cap(e.pcmBuf) < need {
		e.pcmBuf = make([]byte, need)
	}
	pcm := e.pcmBuf[:need]
	for i, s := range samples {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(toInt16(s)))
	}
	jsPCM := js.Global().Get("Uint8Array").New(need)
	js.CopyBytesToJS(jsPCM, pcm)

	out := fn.Invoke(e.handle, jsPCM)
	return e.writeJSBytes(out)
}

func (e *MP3Encoder) Close() error {
	if !e.handle.Truthy() {
		return nil
	}
	h := e.handle
	e.handle = js.Undefined()
	fn := js.Global().Get("cs2mp3Flush")
	if fn.Type() != js.TypeFunction {
		return nil
	}
	return e.writeJSBytes(fn.Invoke(h))
}

func (e *MP3Encoder) writeJSBytes(val js.Value) error {
	if !val.Truthy() {
		return nil
	}
	n := val.Get("byteLength").Int()
	if n == 0 {
		return nil
	}
	if cap(e.outBuf) < n {
		e.outBuf = make([]byte, n)
	}
	buf := e.outBuf[:n]
	u8 := js.Global().Get("Uint8Array").New(val.Get("buffer"), val.Get("byteOffset"), n)
	js.CopyBytesToGo(buf, u8)
	_, err := e.w.Write(buf)
	return err
}

func LameVersion() string {
	if fn := js.Global().Get("cs2mp3Version"); fn.Type() == js.TypeFunction {
		return fn.Invoke().String()
	}
	return "lame-wasm"
}
