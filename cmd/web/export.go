//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"cs2voice/internal/audio"
	"cs2voice/internal/demo"
	"cs2voice/internal/report"
)

type ExportRequest struct {
	Periods         []int      `json:"periods"`
	AudioPlayers    []string   `json:"audioPlayers"`
	Reported        []string   `json:"reported"`
	Mode            string     `json:"mode"`
	PerRound        bool       `json:"perRound"`
	IncludeFull     bool       `json:"includeFull"`
	IncludeIsolated bool       `json:"includeIsolated"`
	Format          string     `json:"format"`
	Reason          string     `json:"reason"`
	MatchIDPrefix   bool       `json:"matchIdPrefix"`
	Marks           []MarkView `json:"marks"`
	ReportOverride  string     `json:"reportOverride,omitempty"`
}

type MarkView struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type FileView struct {
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Team     string  `json:"team"`
	Player   string  `json:"player"`
	Periods  []int   `json:"periods"`
	Duration float64 `json:"duration"`
}

type ExportView struct {
	Files      []FileView `json:"files"`
	Report     string     `json:"report"`
	ReportFile string     `json:"reportFile"`
}

type planExportResult struct {
	Data  *ExportView `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

func registerExport() {
	js.Global().Set("cs2planExport", js.FuncOf(func(_ js.Value, args []js.Value) any {
		var res planExportResult
		if len(args) < 1 {
			res.Error = "missing export request"
			b, _ := json.Marshal(res)
			return string(b)
		}
		v, err := planExportWeb(args[0].String())
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Data = v
		}
		b, _ := json.Marshal(res)
		return string(b)
	}))

	js.Global().Set("cs2export", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return jsError("missing export request")
		}
		mode := "zip"
		if len(args) > 1 && args[1].Type() == js.TypeString {
			mode = args[1].String()
		}
		return exportWeb(args[0].String(), mode)
	}))
}

func planExportWeb(reqJSON string) (*ExportView, error) {
	var req ExportRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return nil, err
	}
	m, err := currentMatchOrErr()
	if err != nil {
		return nil, err
	}
	files, err := audio.PlanExport(m, toExportOptions(m, req))
	if err != nil {
		return nil, err
	}
	return buildExportView(m, req, files), nil
}

func exportWeb(reqJSON, deliveryMode string) js.Value {
	var req ExportRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return jsError(err.Error())
	}
	m, err := currentMatchOrErr()
	if err != nil {
		return jsError(err.Error())
	}
	opts := toExportOptions(m, req)

	out := js.Global().Get("Object").New()
	var files []audio.ExportedFile
	var reportText string
	resolveReport := func(fs []audio.ExportedFile) string {
		if req.ReportOverride != "" {
			return req.ReportOverride
		}
		return buildReportText(m, req, fs)
	}

	if deliveryMode == "files" {
		exported, err := audio.ExportToMemory(m, opts)
		if err != nil {
			return jsError(err.Error())
		}
		files = make([]audio.ExportedFile, len(exported))
		arr := js.Global().Get("Array").New(len(exported))
		for i, item := range exported {
			files[i] = item.File
			fobj := js.Global().Get("Object").New()
			fobj.Set("name", item.File.Name)
			fobj.Set("bytes", bytesToJS(item.Data))
			arr.SetIndex(i, fobj)
		}
		reportText = resolveReport(files)
		out.Set("files", arr)
		out.Set("reportFile", "report.txt")
		out.Set("reportText", reportText)
	} else {
		var zipBytes []byte
		files, zipBytes, err = audio.ExportZipToMemory(m, opts, func(fs []audio.ExportedFile) string {
			reportText = resolveReport(fs)
			return reportText
		})
		if err != nil {
			return jsError(err.Error())
		}
		out.Set("zipName", zipFileName(m))
		out.Set("zipBytes", bytesToJS(zipBytes))
	}

	if len(files) == 0 {
		return jsError("nothing to export: nobody selected talks in these rounds")
	}

	v := buildExportViewWithReport(files, reportText)
	v.ReportFile = "report.txt"
	viewJS, err := marshalToJS(v)
	if err != nil {
		return jsError(err.Error())
	}
	out.Set("view", viewJS)
	return out
}

func toExportOptions(m *demo.Match, req ExportRequest) audio.ExportOptions {
	return audio.ExportOptions{
		Prefix:          matchPrefix(m, req.MatchIDPrefix),
		Periods:         req.Periods,
		AudioPlayers:    parseIDs(req.AudioPlayers),
		Reported:        parseIDs(req.Reported),
		Mode:            parseAudioMode(req.Mode),
		PerRound:        req.PerRound,
		IncludeFull:     req.IncludeFull,
		IncludeIsolated: req.IncludeIsolated,
		Format:          parseFormat(req.Format),
	}
}

func buildExportView(m *demo.Match, req ExportRequest, files []audio.ExportedFile) *ExportView {
	return buildExportViewWithReport(files, buildReportText(m, req, files))
}

func buildExportViewWithReport(files []audio.ExportedFile, reportText string) *ExportView {
	v := &ExportView{Report: reportText}
	for _, f := range files {
		fv := FileView{
			Name:     f.Name,
			Kind:     f.Kind,
			Team:     f.Team,
			Periods:  f.Periods,
			Duration: f.Duration,
		}
		if f.Player != 0 {
			fv.Player = idStr(f.Player)
		}
		v.Files = append(v.Files, fv)
	}
	return v
}

func buildReportText(m *demo.Match, req ExportRequest, files []audio.ExportedFile) string {
	marks := make([]demo.Span, 0, len(req.Marks))
	for _, mk := range req.Marks {
		marks = append(marks, demo.Span{Start: mk.Start, End: mk.End})
	}
	return report.Build(report.Input{
		Match:    m,
		Reported: parseIDs(req.Reported),
		Files:    files,
		Reason:   req.Reason,
		Marks:    marks,
	})
}

func zipFileName(m *demo.Match) string {
	id := sanitizeName(m.FaceitMatchID)
	if id == "" {
		return "cs2voice.zip"
	}
	return "cs2voice_" + id + ".zip"
}
