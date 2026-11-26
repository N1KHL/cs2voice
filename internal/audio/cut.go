package audio

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"cs2voice/internal/demo"
)

type planItem struct {
	player uint64
	u      demo.Utterance
	skip   int
	n      int
}

// clipItem trims an utterance to the demo-time clip window [clip.Start, clip.End].
func clipItem(tr *demo.VoiceTrack, it planItem, clip demo.Span) (planItem, bool) {
	if it.n <= 0 || clip.End <= clip.Start {
		return planItem{}, false
	}
	u := it.u
	uStart := u.Start
	uEnd := uStart + float64(it.n)/SR
	lo := max(uStart, clip.Start)
	hi := min(uEnd, clip.End)
	if hi <= lo {
		return planItem{}, false
	}

	startSample := max(0, int(math.Round((lo-uStart)*SR)))
	endSample := min(it.n, int(math.Round((hi-uStart)*SR)))
	if endSample <= startSample {
		return planItem{}, false
	}

	pos := 0
	newFirst, newLast := -1, -1
	skip := 0
	for i := u.First; i <= u.Last; i++ {
		ps := tr.Packets[i].Samples
		if pos+ps > startSample && newFirst < 0 {
			newFirst = i
			skip = startSample - pos
		}
		if pos < endSample {
			newLast = i
		}
		pos += ps
	}
	if newFirst < 0 || newLast < newFirst {
		return planItem{}, false
	}

	u.First = newFirst
	u.Last = newLast
	u.Start = uStart + float64(startSample)/SR
	u.End = uStart + float64(endSample)/SR
	if tk := tr.Packets[newFirst].Tick; tk != 0 {
		u.StartTick = tk
	}
	return planItem{
		player: it.player,
		u:      u,
		skip:   skip,
		n:      endSample - startSample,
	}, true
}

// CutFileName builds `[matchID_]who_R12_1m23-1m45.ext` for a clipped selection.
func CutFileName(m *demo.Match, sel Selection, prefix string, format Format) string {
	ids := sortUniquePeriods(sel.Periods)

	who := cutWho(m, sel, ids)
	label := periodsLabel(m, ids)
	pfx := safeName(prefix)

	var parts []string
	if pfx != "" {
		parts = append(parts, pfx)
	}
	parts = append(parts, who)
	if label != "" {
		parts = append(parts, label)
	}
	if sel.Clip != nil {
		parts = append(parts, formatCutSpan(*sel.Clip))
	}

	ext := string(format)
	if ext == "" {
		ext = string(MP3)
	}
	return strings.Join(parts, "_") + "." + ext
}

func cutWho(m *demo.Match, sel Selection, ids []int) string {
	if len(sel.Players) == 1 {
		id := sel.Players[0]
		for _, p := range m.Players {
			if p.SteamID == id {
				if s := safeName(p.Name); s != "" {
					return s
				}
				break
			}
		}
		return strconv.FormatUint(id, 10)
	}

	allTalk := allPeriodsAllTalk(m, ids)
	teams, _ := m.SplitByTeam(sel.Players)
	if allTalk && (sel.Team == "" || len(teams) > 1) {
		return "alltalk"
	}
	team := sel.Team
	if team == "" && len(teams) == 1 {
		team = teams[0]
	}
	if s := safeName(team); s != "" {
		return s
	}
	return "spectators"
}

func formatCutSpan(sp demo.Span) string {
	return formatCutSec(sp.Start) + "-" + formatCutSec(sp.End)
}

func formatCutSec(s float64) string {
	sec := int(math.Max(0, math.Round(s)))
	return fmt.Sprintf("%dm%02d", sec/60, sec%60)
}

// CutOptions configures exporting one or more clipped ranges from Listen & Cut.
type CutOptions struct {
	Periods []int
	Players []uint64
	Team    string
	Mode    Mode
	Format  Format
	Prefix  string // optional FACEIT match ID prefix
	Clips   []demo.Span
}

// CutFile is one rendered audio clip in memory.
type CutFile struct {
	Name     string
	Duration float64
	Data     []byte
}

// ExportCut renders each clipped range in memory (never normalized).
func ExportCut(m *demo.Match, o CutOptions) ([]CutFile, error) {
	if len(o.Clips) == 0 {
		return nil, errors.New("select at least one range to cut")
	}
	f := o.Format
	if f == "" {
		f = MP3
	}

	used := map[string]bool{}
	var files []CutFile
	for _, clip := range o.Clips {
		c := clip
		sel := Selection{
			Periods: o.Periods,
			Players: o.Players,
			Team:    o.Team,
			Mode:    o.Mode,
			Clip:    &c,
		}
		p, err := NewPlan(m, sel)
		if err != nil {
			return nil, err
		}
		if p.Length == 0 {
			continue
		}
		// Exports are never normalized: loudness can be part of the evidence.
		p.Normalize = false

		var ws memWriteSeeker
		if _, err := p.WriteTo(&ws, f, 0); err != nil {
			return nil, err
		}
		name := uniqueName(used, CutFileName(m, sel, o.Prefix, f))
		files = append(files, CutFile{
			Name:     name,
			Duration: p.Duration(),
			Data:     ws.buf,
		})
	}
	if len(files) == 0 {
		return nil, errors.New("nothing to export in the selected ranges")
	}
	return files, nil
}

// CutZip packs rendered cut clips into a ZIP archive in memory.
func CutZip(files []CutFile) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	now := time.Now()
	for _, f := range files {
		if err := writeZipEntry(zw, f.Name, f.Data, now); err != nil {
			zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
