//go:build js && wasm

package main

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"syscall/js"

	"cs2voice/internal/audio"
	"cs2voice/internal/demo"
)

func currentMatchOrErr() (*demo.Match, error) {
	mu.Lock()
	m := currentMatch
	mu.Unlock()
	if m == nil {
		return nil, errors.New("no demo open")
	}
	return m, nil
}

func parseIDs(ss []string) []uint64 {
	var out []uint64
	for _, s := range ss {
		if v, err := strconv.ParseUint(s, 10, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

func parseAudioMode(s string) audio.Mode {
	if s == "full" {
		return audio.Full
	}
	return audio.Compact
}

func parseFormat(s string) audio.Format {
	if s == "wav" {
		return audio.WAV
	}
	return audio.MP3
}

func matchPrefix(m *demo.Match, enabled bool) string {
	if enabled {
		return m.FaceitMatchID
	}
	return ""
}

func sanitizeName(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			return -1
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
}

func cutFilesMetaJS(files []audio.CutFile, includeBytes bool) js.Value {
	arr := js.Global().Get("Array").New(len(files))
	for i, f := range files {
		item := js.Global().Get("Object").New()
		item.Set("name", f.Name)
		item.Set("duration", f.Duration)
		if includeBytes {
			item.Set("bytes", bytesToJS(f.Data))
		}
		arr.SetIndex(i, item)
	}
	return arr
}

func bytesToJS(b []byte) js.Value {
	u8 := js.Global().Get("Uint8Array").New(len(b))
	if len(b) > 0 {
		js.CopyBytesToJS(u8, b)
	}
	return u8
}

func marshalToJS(v any) (js.Value, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return js.Undefined(), err
	}
	return js.Global().Get("JSON").Call("parse", string(b)), nil
}

func jsError(msg string) js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("error", msg)
	return obj
}
