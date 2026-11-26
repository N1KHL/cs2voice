//go:build !js

// Opus and MP3 only exist in the browser build (js/wasm bridges). Native
// builds (go test, cmd/demoinfo) get these stubs, so the pure-Go packages
// compile and test without a C toolchain.
package codec

import (
	"errors"
	"io"
)

var errUnavailable = errors.New("codec unavailable outside the browser")

type OpusDecoder struct{}

func NewOpusDecoder() (*OpusDecoder, error)                            { return nil, errUnavailable }
func (*OpusDecoder) Decode(dst []float32, _ []byte) ([]float32, error) { return dst, errUnavailable }
func (*OpusDecoder) Close()                                            {}
func OpusVersion() string                                              { return "unavailable outside the browser" }

type MP3Encoder struct{}

func NewMP3Encoder(io.Writer, int) (*MP3Encoder, error) { return nil, errUnavailable }
func (*MP3Encoder) Write([]float32) error               { return errUnavailable }
func (*MP3Encoder) Close() error                        { return errUnavailable }
func LameVersion() string                               { return "unavailable outside the browser" }
