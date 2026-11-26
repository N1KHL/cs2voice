//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"cs2voice/internal/audio"
)

type PreviewRequest struct {
	Slot      string   `json:"slot"` // "left", "right", or "cut"
	Periods   []int    `json:"periods"`
	Players   []string `json:"players"`
	Team      string   `json:"team"`
	Mode      string   `json:"mode"` // "compact" or "full"
	Normalize bool     `json:"normalize"`
}

type SegmentView struct {
	Out    float64 `json:"out"` // seconds in the preview
	Len    float64 `json:"len"`
	Demo   float64 `json:"demo"` // demo time at Out
	Period int     `json:"period"`
}

type PreviewView struct {
	URL         string        `json:"url"`
	Duration    float64       `json:"duration"`
	WaveRate    int           `json:"waveRate"`
	WavePlayers []string      `json:"wavePlayers"`
	Peaks       [][]byte      `json:"peaks"` // base64 in JSON, one per player
	Segments    []SegmentView `json:"segments"`
}

func registerPreview() {
	js.Global().Set("cs2preview", js.FuncOf(func(_ js.Value, args []js.Value) any {
		reqJSON := "{}"
		if len(args) > 0 && args[0].Type() == js.TypeString {
			reqJSON = args[0].String()
		}
		var req PreviewRequest
		if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
			return jsError(err.Error())
		}
		wavBytes, pv, err := renderPreview(req)
		if err != nil {
			return jsError(err.Error())
		}
		viewJS, err := marshalToJS(pv)
		if err != nil {
			return jsError(err.Error())
		}

		out := js.Global().Get("Object").New()
		out.Set("wav", bytesToJS(wavBytes))
		out.Set("view", viewJS)
		return out
	}))
}

func renderPreview(req PreviewRequest) ([]byte, *PreviewView, error) {
	m, err := currentMatchOrErr()
	if err != nil {
		return nil, nil, err
	}

	sel := audio.Selection{
		Periods: req.Periods,
		Players: parseIDs(req.Players),
		Team:    req.Team,
		Mode:    parseAudioMode(req.Mode),
	}
	p, err := audio.NewPlan(m, sel)
	if err != nil {
		return nil, nil, err
	}
	p.Normalize = req.Normalize

	const waveRate = 50
	wavBytes, wave, err := p.RenderWAV(waveRate)
	if err != nil {
		return nil, nil, err
	}

	out := &PreviewView{
		Duration: p.Duration(),
		WaveRate: waveRate,
	}
	if wave != nil {
		for i, id := range wave.Players {
			out.WavePlayers = append(out.WavePlayers, idStr(id))
			out.Peaks = append(out.Peaks, wave.Peaks[i])
		}
	}
	for _, s := range p.Segments {
		out.Segments = append(out.Segments, SegmentView{
			Out:    float64(s.Out) / audio.SR,
			Len:    float64(s.Len) / audio.SR,
			Demo:   s.Demo,
			Period: s.Period,
		})
	}
	return wavBytes, out, nil
}
