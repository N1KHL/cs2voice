//go:build js && wasm

package main

import (
	"encoding/json"
	"errors"
	"syscall/js"

	"cs2voice/internal/demo"
)

// registerSession exposes the parsed-match cache: the worker stores the
// snapshot in the browser's private storage so a reload, or opening the same
// demo again later, skips parsing.
func registerSession() {
	js.Global().Set("cs2snapshot", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		m, err := currentMatchOrErr()
		if err != nil {
			return jsError(err.Error())
		}
		data, err := m.MarshalBinary()
		if err != nil {
			return jsError(err.Error())
		}
		out := js.Global().Get("Object").New()
		out.Set("bytes", bytesToJS(data))
		return out
	}))

	// cs2restore returns the same JSON as cs2openDemo. "stale" marks a
	// snapshot from another cache version: the worker drops it and the demo
	// has to be opened from its file again.
	js.Global().Set("cs2restore", js.FuncOf(func(_ js.Value, args []js.Value) any {
		var res struct {
			openResult
			Stale bool `json:"stale,omitempty"`
		}
		if len(args) == 0 || args[0].Type() != js.TypeObject {
			res.Error = "restore: missing snapshot bytes"
		} else {
			data := make([]byte, args[0].Get("byteLength").Int())
			js.CopyBytesToGo(data, args[0])
			m, err := demo.UnmarshalMatch(data)
			if err != nil {
				res.Error = err.Error()
				res.Stale = errors.Is(err, demo.ErrCacheVersion)
			} else {
				res.Data = installMatch(m)
			}
		}
		out, _ := json.Marshal(res)
		return string(out)
	}))
}
