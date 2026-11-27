//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"cs2voice/internal/demo"
)

// registerSession exposes the parsed-match cache: the worker stores the
// snapshot in the browser's private storage so a reload, or opening the same
// demo again later, skips parsing.
func registerSession() {
	js.Global().Set("cs2snapshot", js.FuncOf(func(_ js.Value, args []js.Value) any {
		key := ""
		if len(args) > 0 && args[0].Type() == js.TypeString {
			key = args[0].String()
		}
		mu.Lock()
		m, curKey := currentMatch, currentKey
		mu.Unlock()
		if m == nil {
			return jsError("no demo open")
		}
		if key != "" && curKey != key {
			return jsError("snapshot: another demo is open")
		}
		data, err := m.MarshalBinary()
		if err != nil {
			return jsError(err.Error())
		}
		out := js.Global().Get("Object").New()
		out.Set("bytes", bytesToJS(data))
		return out
	}))

	// cs2restore returns the same JSON as cs2openDemo. Any decode or version
	// error drops the cached snapshot in the worker so the demo is parsed again.
	js.Global().Set("cs2restore", js.FuncOf(func(_ js.Value, args []js.Value) any {
		var res openResult
		if len(args) == 0 || args[0].Type() != js.TypeObject {
			res.Error = "restore: missing snapshot bytes"
		} else {
			key := ""
			if len(args) > 1 && args[1].Type() == js.TypeString {
				key = args[1].String()
			}
			fileName := ""
			if len(args) > 2 && args[2].Type() == js.TypeString {
				fileName = args[2].String()
			}
			seq := nextOpenSeq()
			data := make([]byte, args[0].Get("byteLength").Int())
			js.CopyBytesToGo(data, args[0])
			m, err := demo.UnmarshalMatch(data)
			if err != nil {
				res.Error = err.Error()
			} else {
				m.SetFile(fileName)
				mv, err := installMatch(m, key, seq)
				if err != nil {
					res.Error = err.Error()
				} else {
					res.Data = mv
				}
			}
		}
		out, _ := json.Marshal(res)
		return string(out)
	}))
}
