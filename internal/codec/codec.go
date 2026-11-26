// Package codec wraps voice decoding (Opus) and audio encoding (MP3, WAV).
// Opus and MP3 call the JS/WASM bridges of the browser build; native builds
// only have stubs. WAV is plain Go everywhere.
package codec

const SampleRate = 48000
