//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"cs2voice/internal/audio"
	"cs2voice/internal/demo"
)

func registerCut() {
	js.Global().Set("cs2exportCut", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return jsError("missing cut export request")
		}
		exportMode := "zip"
		if len(args) >= 2 && args[1].Type() == js.TypeString && args[1].String() != "" {
			exportMode = args[1].String()
		}
		return handleExportCut(args[0].String(), exportMode)
	}))
}

type cutRangeJSON struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type cutExportRequest struct {
	Periods       []int          `json:"periods"`
	Players       []string       `json:"players"`
	Team          string         `json:"team"`
	Mode          string         `json:"mode"`
	Format        string         `json:"format"`
	MatchIDPrefix bool           `json:"matchIdPrefix"`
	Clips         []cutRangeJSON `json:"clips"`
}

func handleExportCut(reqJSON, exportMode string) js.Value {
	var req cutExportRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return jsError("invalid request: " + err.Error())
	}

	m, err := currentMatchOrErr()
	if err != nil {
		return jsError(err.Error())
	}

	clips := make([]demo.Span, 0, len(req.Clips))
	for _, c := range req.Clips {
		clips = append(clips, demo.Span{Start: c.Start, End: c.End})
	}

	files, err := audio.ExportCut(m, audio.CutOptions{
		Periods: req.Periods,
		Players: parseIDs(req.Players),
		Team:    req.Team,
		Mode:    parseAudioMode(req.Mode),
		Format:  parseFormat(req.Format),
		Prefix:  matchPrefix(m, req.MatchIDPrefix),
		Clips:   clips,
	})
	if err != nil {
		return jsError(err.Error())
	}

	obj := js.Global().Get("Object").New()
	if exportMode == "files" {
		obj.Set("files", cutFilesMetaJS(files, true))
		return obj
	}

	zipBytes, err := audio.CutZip(files)
	if err != nil {
		return jsError(err.Error())
	}
	zipName := "cs2voice_cuts.zip"
	if id := sanitizeName(m.FaceitMatchID); id != "" {
		zipName = "cs2voice_" + id + "_cuts.zip"
	}
	obj.Set("zipName", zipName)
	obj.Set("zipBytes", bytesToJS(zipBytes))
	obj.Set("files", cutFilesMetaJS(files, false))
	return obj
}
