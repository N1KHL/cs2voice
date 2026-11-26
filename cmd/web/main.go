//go:build js && wasm

// web is the browser WebAssembly worker backend for CS2 Voice.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"strconv"
	"sync"
	"syscall/js"

	"cs2voice/internal/demo"
	"cs2voice/internal/rules"
)

const version = "0.1.0"

var (
	mu           sync.Mutex
	currentMatch *demo.Match
)

type MatchView struct {
	File          string       `json:"file"`
	Map           string       `json:"map"`
	Server        string       `json:"server"`
	FaceitMatchID string       `json:"faceitMatchId"`
	Platform      string       `json:"platform"`
	GameVersion   int          `json:"gameVersion"`
	Duration      float64      `json:"duration"`
	VoiceFormat   string       `json:"voiceFormat"`
	AllTalkSource string       `json:"allTalkSource"`
	Warnings      []string     `json:"warnings"`
	Teams         []string     `json:"teams"`
	Players       []PlayerView `json:"players"`
	Periods       []PeriodView `json:"periods"`
	FinalScore    string       `json:"finalScore"`
}

type PlayerView struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Team    string  `json:"team"`
	Playing bool    `json:"playing"`
	Talk    float64 `json:"talk"`
}

type PeriodView struct {
	ID         int                `json:"id"`
	Kind       string             `json:"kind"`
	Label      string             `json:"label"`
	Number     int                `json:"number"`
	AllTalk    bool               `json:"allTalk"`
	Start      float64            `json:"start"`
	Length     float64            `json:"length"`
	Winner     string             `json:"winner"`
	WinnerTeam string             `json:"winnerTeam"`
	Reason     string             `json:"reason"`
	TeamCT     string             `json:"teamCT"`
	TeamT      string             `json:"teamT"`
	ScoreCT    int                `json:"scoreCT"`
	ScoreT     int                `json:"scoreT"`
	Talk       map[string]float64 `json:"talk"`
}

type AboutView struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Modified bool   `json:"modified"`
}

type openResult struct {
	Data  *MatchView `json:"data,omitempty"`
	Error string     `json:"error,omitempty"`
}

func main() {
	js.Global().Set("cs2openDemo", js.FuncOf(func(_ js.Value, args []js.Value) any {
		file, onProgress, onDone := args[0], args[1], args[2]
		go func() {
			mv, err := openDemo(file, onProgress)
			var res openResult
			if err != nil {
				res.Error = err.Error()
			} else {
				res.Data = mv
			}
			out, _ := json.Marshal(res)
			onDone.Invoke(string(out))
		}()
		return nil
	}))

	js.Global().Set("cs2about", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		commit, modified := buildCommit()
		out, _ := json.Marshal(AboutView{
			Version:  version,
			Commit:   commit,
			Modified: modified,
		})
		return string(out)
	}))

	registerPreview()
	registerExport()
	registerCut()

	select {}
}

func openDemo(file, onProgress js.Value) (*MatchView, error) {
	size := int64(file.Get("size").Float())
	name := file.Get("name").String()
	br := &blobReader{blob: file, size: size, onProgress: onProgress}
	m, err := demo.ParseReader(bufio.NewReaderSize(br, 4<<20), name, nil)
	if err != nil {
		return nil, err
	}
	rules.Apply(m)
	if m.VoiceFormat == "" {
		m.Warnings = append(m.Warnings, "This demo has no voice. Valve Matchmaking demos never have voice; other servers record it only with tv_relayvoice.")
	}
	mu.Lock()
	currentMatch = m
	mu.Unlock()
	return matchView(m), nil
}

func matchView(m *demo.Match) *MatchView {
	v := &MatchView{
		File: m.File, Map: m.Map, Server: m.Server, FaceitMatchID: m.FaceitMatchID,
		Platform: m.Platform, GameVersion: m.GameVersion, Duration: m.Duration,
		VoiceFormat: m.VoiceFormat, AllTalkSource: m.AllTalkSource, Warnings: m.Warnings,
	}
	var ids []uint64
	for _, p := range m.Players {
		ids = append(ids, p.SteamID)
	}
	v.Teams, _ = m.SplitByTeam(ids)

	talk := map[uint64]float64{}
	perPeriod := map[int]map[string]float64{}
	for id, tr := range m.Voice {
		for _, u := range tr.Utterances {
			d := u.End - u.Start
			talk[id] += d
			if perPeriod[u.Period] == nil {
				perPeriod[u.Period] = map[string]float64{}
			}
			perPeriod[u.Period][idStr(id)] += d
		}
	}
	for _, p := range m.Players {
		v.Players = append(v.Players, PlayerView{
			ID:      idStr(p.SteamID),
			Name:    p.Name,
			Team:    p.Team,
			Playing: p.Playing,
			Talk:    talk[p.SteamID],
		})
	}
	for _, r := range m.Rounds {
		if r.Next-r.Start <= 0.01 {
			continue // empty warmup when the demo starts at round 1
		}
		v.Periods = append(v.Periods, PeriodView{
			ID: r.ID, Kind: r.Kind, Label: r.Label, Number: r.Number, AllTalk: r.AllTalk,
			Start: r.Start, Length: r.Next - r.Start, Winner: r.Winner, WinnerTeam: r.WinnerTeam, Reason: r.Reason,
			TeamCT: r.TeamCT, TeamT: r.TeamT, ScoreCT: r.ScoreCT, ScoreT: r.ScoreT, Talk: perPeriod[r.ID],
		})
		if r.Kind == demo.KindRound && r.Winner != "" {
			v.FinalScore = fmt.Sprintf("%s %d - %d %s", r.TeamCT, r.ScoreCT, r.ScoreT, r.TeamT)
		}
	}
	return v
}

func idStr(id uint64) string { return strconv.FormatUint(id, 10) }

func buildCommit() (commit string, modified bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
			if len(commit) > 7 {
				commit = commit[:7]
			}
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return commit, modified
}

// blobReader reads a JS File/Blob piece by piece, so the whole file is never
// copied into WebAssembly memory.
type blobReader struct {
	blob       js.Value
	off, size  int64
	onProgress js.Value
}

func (b *blobReader) Read(p []byte) (int, error) {
	if b.off >= b.size {
		return 0, io.EOF
	}
	end := min(b.off+int64(len(p)), b.size)
	buf, err := await(b.blob.Call("slice", b.off, end).Call("arrayBuffer"))
	if err != nil {
		return 0, err
	}
	n := js.CopyBytesToGo(p, js.Global().Get("Uint8Array").New(buf))
	b.off += int64(n)
	if b.size > 0 {
		b.onProgress.Invoke(float64(b.off) / float64(b.size))
	}
	return n, nil
}

// await blocks this goroutine until the JS promise settles.
func await(promise js.Value) (js.Value, error) {
	ok, fail := make(chan js.Value, 1), make(chan js.Value, 1)
	then := js.FuncOf(func(_ js.Value, a []js.Value) any { ok <- a[0]; return nil })
	catch := js.FuncOf(func(_ js.Value, a []js.Value) any { fail <- a[0]; return nil })
	defer then.Release()
	defer catch.Release()
	promise.Call("then", then, catch)
	select {
	case v := <-ok:
		return v, nil
	case e := <-fail:
		return js.Undefined(), fmt.Errorf("reading the file: %s", e.Call("toString").String())
	}
}
