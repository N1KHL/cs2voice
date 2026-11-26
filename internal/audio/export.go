package audio

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"cs2voice/internal/demo"
)

type ExportOptions struct {
	Periods      []int    // period IDs (rounds, breaks, warmup)
	AudioPlayers []uint64 // voices in the full audio
	Reported     []uint64 // reported players (get an isolated file each)
	Mode         Mode     // Compact (talk only) or Full (real round time), for team and isolated files

	PerRound        bool // one file per period (default) or everything in one file
	IncludeFull     bool // the team audio with the AudioPlayers
	IncludeIsolated bool // only the voice of each reported player

	Format Format

	// Prefix goes in front of every file name (e.g. the FACEIT match ID), so
	// files from several matches can be told apart. Empty = no prefix.
	Prefix string
}

// Speech is one utterance inside an exported file, for the report.
type Speech struct {
	Player    uint64
	Period    int     // period ID
	DemoStart float64 // seconds since demo start
	Tick      int
	Duration  float64
	FileStart float64 // where it starts in the exported file
}

type ExportedFile struct {
	Name     string
	Kind     string // "team" (what a team heard), "alltalk" (a break: both teams) or "isolated" (one player)
	Team     string // point of view ("" for alltalk)
	Periods  []int
	Player   uint64 // isolated: whose voice
	Mode     Mode   // Compact or Full
	Duration float64
	Speech   []Speech
	Segments []Segment // how file time maps to demo time
}

// DemoToFile maps a demo time range to this file's time. ok is false when no
// part of the range is in the file (e.g. it falls in a cut silence).
func (f ExportedFile) DemoToFile(a, b float64) (start, end float64, ok bool) {
	for _, g := range f.Segments {
		d0, d1 := g.Demo, g.Demo+float64(g.Len)/SR
		lo, hi := max(a, d0), min(b, d1)
		if hi <= lo {
			continue
		}
		s := float64(g.Out)/SR + (lo - d0)
		e := float64(g.Out)/SR + (hi - d0)
		if !ok || s < start {
			start = s
		}
		if !ok || e > end {
			end = e
		}
		ok = true
	}
	return
}

type plannedExport struct {
	plan *Plan
	base string
	ext  string
	file ExportedFile
}

// PlanExport lists the files an export would contain (names, durations,
// speech) without rendering them, so the report can be shown before
// exporting. Selections with no voice at all (e.g. a reported player who said
// nothing in that round) are skipped.
//
// Team audio is one file per team (what that team heard). A file that only
// covers all-talk periods (halftime, overtime breaks) is a single file with
// both teams.
func PlanExport(m *demo.Match, o ExportOptions) ([]ExportedFile, error) {
	planned, err := planExports(m, o)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	files := make([]ExportedFile, len(planned))
	for i, pe := range planned {
		pe.file.Name = uniqueName(seen, pe.base+"."+pe.ext)
		files[i] = pe.file
	}
	return files, nil
}

func planExports(m *demo.Match, o ExportOptions) ([]plannedExport, error) {
	if !o.IncludeFull && !o.IncludeIsolated {
		return nil, fmt.Errorf("nothing to export: choose the team audio, the isolated voice, or both")
	}
	if o.Format == "" {
		o.Format = MP3
	}
	ids := sortUniquePeriods(o.Periods)

	groups := [][]int{ids}
	if o.PerRound {
		groups = nil
		for _, id := range ids {
			groups = append(groups, []int{id})
		}
	}

	names := map[uint64]string{}
	for _, p := range m.Players {
		names[p.SteamID] = p.Name
	}
	// File names: [prefix_]<who>_<rounds>, e.g. team_x_R12, myth1300_R12,
	// 1-abc..._alltalk_halftime.
	prefix := safeName(o.Prefix)
	if prefix != "" {
		prefix += "_"
	}

	var planned []plannedExport
	add := func(sel Selection, name, kind string, g []int, player uint64) error {
		pe, err := planOne(m, sel, o.Format, name)
		if err != nil || pe == nil {
			return err
		}
		pe.file.Kind, pe.file.Team, pe.file.Periods, pe.file.Player, pe.file.Mode = kind, sel.Team, g, player, sel.Mode
		planned = append(planned, *pe)
		return nil
	}

	for _, g := range groups {
		label := periodsLabel(m, g)
		if o.IncludeFull {
			if allPeriodsAllTalk(m, g) {
				sel := Selection{Periods: g, Players: o.AudioPlayers, Mode: o.Mode}
				if err := add(sel, prefix+"alltalk_"+label, "alltalk", g, 0); err != nil {
					return nil, err
				}
			} else {
				teams, _ := m.SplitByTeam(o.AudioPlayers)
				for _, team := range teams {
					teamName := safeName(team)
					if teamName == "" {
						teamName = "spectators"
					}
					sel := Selection{Periods: g, Players: o.AudioPlayers, Team: team, Mode: o.Mode}
					if err := add(sel, prefix+teamName+"_"+label, "team", g, 0); err != nil {
						return nil, err
					}
				}
			}
		}
		if o.IncludeIsolated {
			for _, id := range o.Reported {
				// The team file already is only this player's voice: no duplicate.
				if o.IncludeFull && len(o.AudioPlayers) == 1 && o.AudioPlayers[0] == id {
					continue
				}
				sel := Selection{Periods: g, Players: []uint64{id}, Team: m.TeamOf(id), Mode: o.Mode}
				name := prefix + safeName(names[id]) + "_" + label
				if err := add(sel, name, "isolated", g, id); err != nil {
					return nil, err
				}
			}
		}
	}
	return planned, nil
}

func planOne(m *demo.Match, sel Selection, format Format, base string) (*plannedExport, error) {
	p, err := NewPlan(m, sel)
	if err != nil {
		return nil, err
	}
	if len(p.Placements) == 0 {
		return nil, nil
	}
	f := ExportedFile{Name: base + "." + string(format), Duration: p.Duration(), Segments: p.Segments}
	for _, pl := range p.Placements {
		f.Speech = append(f.Speech, Speech{
			Player:    pl.Player,
			Period:    pl.Utterance.Period,
			DemoStart: pl.Utterance.Start,
			Tick:      pl.Utterance.StartTick,
			Duration:  float64(pl.Len) / SR,
			FileStart: float64(pl.Out) / SR,
		})
	}
	return &plannedExport{plan: p, base: base, ext: string(format), file: f}, nil
}

// periodsLabel names a set of periods for a file name:
// R05 | R05-R07_R12 | R12_halftime_R13 | warmup_R01-R03.
func periodsLabel(m *demo.Match, ids []int) string {
	var parts []string
	runStart, runEnd := -1, -1
	flush := func() {
		if runStart < 0 {
			return
		}
		if runStart == runEnd {
			parts = append(parts, fmt.Sprintf("R%02d", runStart))
		} else {
			parts = append(parts, fmt.Sprintf("R%02d-R%02d", runStart, runEnd))
		}
		runStart, runEnd = -1, -1
	}
	for _, id := range ids {
		if id < 0 || id >= len(m.Rounds) {
			continue
		}
		r := m.Rounds[id]
		switch r.Kind {
		case demo.KindRound:
			if runStart >= 0 && r.Number == runEnd+1 {
				runEnd = r.Number
				continue
			}
			flush()
			runStart, runEnd = r.Number, r.Number
		case demo.KindBreak:
			flush()
			if r.Number == 12 {
				parts = append(parts, "halftime")
			} else {
				parts = append(parts, fmt.Sprintf("break-after-R%02d", r.Number))
			}
		case demo.KindKnife:
			flush()
			parts = append(parts, "knife")
		default:
			flush()
			parts = append(parts, "warmup")
		}
	}
	flush()
	s := strings.Join(parts, "_")
	if len(s) > 60 {
		s = fmt.Sprintf("%d_periods", len(ids))
	}
	return s
}

var badChars = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)

func safeName(s string) string {
	s = badChars.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), "-")
	s = strings.Trim(s, ". ")
	if len([]rune(s)) > 40 {
		s = string([]rune(s)[:40])
	}
	return s
}

// sortUniquePeriods returns a sorted, deduplicated copy of period IDs.
func sortUniquePeriods(ids []int) []int {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

// allPeriodsAllTalk reports whether ids is non-empty and every ID is an all-talk period.
func allPeriodsAllTalk(m *demo.Match, ids []int) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if id < 0 || id >= len(m.Rounds) || !m.Rounds[id].AllTalk {
			return false
		}
	}
	return true
}

// uniqueName returns name if not yet in used, or "stem (2).ext", "stem (3).ext", ...
// and marks the returned name in used.
func uniqueName(used map[string]bool, name string) string {
	if !used[name] {
		used[name] = true
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		cand := stem + " (" + strconv.Itoa(i) + ")" + ext
		if !used[cand] {
			used[cand] = true
			return cand
		}
	}
}
