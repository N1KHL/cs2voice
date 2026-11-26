//go:build js && wasm

package codec

import (
	"encoding/binary"
	"errors"
	"math"
	"syscall/js"
)

// OpusDecoder decodes mono 48 kHz Opus packets via synchronous JS/WASM callbacks.
type OpusDecoder struct {
	handle js.Value
	raw    []byte
}

// NewOpusDecoder makes a mono 48 kHz decoder. Each player needs their own,
// fed with that player's packets in order.
func NewOpusDecoder() (*OpusDecoder, error) {
	fn := js.Global().Get("cs2opusCreate")
	if fn.Type() != js.TypeFunction {
		return nil, errors.New("opus decoder not available in browser yet")
	}
	h := fn.Invoke()
	if !h.Truthy() {
		return nil, errors.New("cs2opusCreate failed")
	}
	return &OpusDecoder{handle: h}, nil
}

// Decode decodes one packet and appends the float32 samples to dst.
func (o *OpusDecoder) Decode(dst []float32, pkt []byte) ([]float32, error) {
	if len(pkt) == 0 {
		return dst, errors.New("empty opus packet")
	}
	fn := js.Global().Get("cs2opusDecode")
	if fn.Type() != js.TypeFunction {
		return dst, errors.New("opus decoder not available in browser yet")
	}
	jsPkt := js.Global().Get("Uint8Array").New(len(pkt))
	js.CopyBytesToJS(jsPkt, pkt)

	res := fn.Invoke(o.handle, jsPkt)
	if !res.Truthy() {
		return dst, errors.New("cs2opusDecode failed")
	}
	byteLen := res.Get("byteLength").Int()
	if byteLen == 0 {
		return dst, nil
	}
	u8 := js.Global().Get("Uint8Array").New(res.Get("buffer"), res.Get("byteOffset"), byteLen)
	if cap(o.raw) < byteLen {
		o.raw = make([]byte, byteLen)
	}
	raw := o.raw[:byteLen]
	js.CopyBytesToGo(raw, u8)

	for i := 0; i+4 <= byteLen; i += 4 {
		dst = append(dst, math.Float32frombits(binary.LittleEndian.Uint32(raw[i:i+4])))
	}
	return dst, nil
}

func (o *OpusDecoder) Close() {
	if o.handle.Truthy() {
		if fn := js.Global().Get("cs2opusDestroy"); fn.Type() == js.TypeFunction {
			fn.Invoke(o.handle)
		}
		o.handle = js.Undefined()
	}
}

func OpusVersion() string {
	if fn := js.Global().Get("cs2opusVersion"); fn.Type() == js.TypeFunction {
		return fn.Invoke().String()
	}
	return "opus-wasm"
}
