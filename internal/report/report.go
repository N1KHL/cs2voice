// Package report builds the English report text for FACEIT from an export.
package report

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"cs2voice/internal/audio"
	"cs2voice/internal/demo"
)

type Input struct {
	Match    *demo.Match
	Reported []uint64
	Files    []audio.ExportedFile // planned or exported files
	Reason   string               // e.g. "Toxic voice communication"
	Marks    []demo.Span          // moments marked by the user, in demo time
}

func Build(in Input) string {
	m := in.Match
	names := map[uint64]string{}
	for _, p := range m.Players {
		names[p.SteamID] = p.Name
	}
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }

	// Header
	if len(in.Reported) == 0 {
		line("Reported player(s): (select the reported player(s))")
	} else {
		line("Reported player(s):")
		for _, id := range in.Reported {
			line("- %s (SteamID64: %d, https://steamcommunity.com/profiles/%d)", names[id], id, id)
		}
	}
	reason := in.Reason
	if reason == "" {
		reason = "Toxic voice communication"
	}
	line("Reason: %s", reason)
	line("")
	if m.FaceitMatchID != "" {
		line("Match: https://www.faceit.com/en/cs2/room/%s", m.FaceitMatchID)
	}
	line("Map: %s%s", m.Map, finalScore(m))
	line("Demo: %s (CS2 game version %d)", filepath.Base(m.File), m.GameVersion)
	line("")

	if len(in.Files) == 0 {
		line("Evidence: (nothing selected to export)")
		return b.String()
	}

	marks := validMarks(in.Marks)
	if len(marks) > 0 {
		line("Marked moments (where to listen in the audio files):")
		for i, mk := range marks {
			var at []string
			for _, f := range in.Files {
				if s, e, ok := f.DemoToFile(mk.Start, mk.End); ok {
					at = append(at, fmt.Sprintf("%s at %s-%s", f.Name, clock(s), clock(e)))
				}
			}
			if len(at) == 0 {
				at = []string{"(not in the selected files)"}
			}
			line("  #%d %s", i+1, strings.Join(at, "; "))
			line("     %s", describeMark(m, mk, in.Files, names))
		}
		line("")
	}

	line("Evidence (voice extracted from the match demo; times are positions in each audio file):")
	reported := map[uint64]bool{}
	for _, id := range in.Reported {
		reported[id] = true
	}
	others := map[uint64]bool{}

	for _, f := range in.Files {
		line("")
		line("%s (%s)", f.Name, clock(f.Duration))
		line("  %s", describe(m, f, names))
		evidence(line, m, f, reported, others, names)
		var at []string
		for i, mk := range marks {
			if s, e, ok := f.DemoToFile(mk.Start, mk.End); ok {
				at = append(at, fmt.Sprintf("#%d at %s-%s", i+1, clock(s), clock(e)))
			}
		}
		if len(at) > 0 {
			line("  Marked: %s", strings.Join(at, ", "))
		}
	}

	if len(others) > 0 {
		var ns []string
		for id := range others {
			ns = append(ns, names[id])
		}
		sort.Strings(ns)
		line("")
		line("Also audible in the clips: %s.", strings.Join(ns, ", "))
	}
	return b.String()
}

func validMarks(ms []demo.Span) []demo.Span {
	var out []demo.Span
	for _, mk := range ms {
		if mk.End > mk.Start {
			out = append(out, mk)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// describeMark: "Round 2 (demo 3:57-4:03, tick 15232) - speaking: BOSZY"
func describeMark(m *demo.Match, mk demo.Span, files []audio.ExportedFile, names map[uint64]string) string {
	id := demo.PeriodAt(m.Rounds, mk.Start)
	r := m.Rounds[id]
	name := r.Label
	if r.Kind == demo.KindRound {
		name = fmt.Sprintf("Round %d", r.Number)
	}
	tick := r.StartTick + int((mk.Start-r.Start)*m.TickRate)
	s := fmt.Sprintf("%s (demo %s-%s, tick %d)", name, clock(mk.Start), clock(mk.End), tick)
	seen := map[uint64]bool{}
	var who []string
	for _, f := range files {
		for _, sp := range f.Speech {
			if sp.DemoStart < mk.End && sp.DemoStart+sp.Duration > mk.Start && !seen[sp.Player] {
				seen[sp.Player] = true
				who = append(who, names[sp.Player])
			}
		}
	}
	if len(who) > 0 {
		sort.Strings(who)
		s += " - speaking: " + strings.Join(who, ", ")
	}
	return s
}

// evidence writes where the reported players can be heard in one file.
//
//	team / all-talk file      -> per round, each reported player's stretches
//	isolated, talk only       -> nothing (the whole file is that player); with
//	                             several rounds, where each round starts
//	isolated, full round      -> per round, that player's stretches
//
// A file with several rounds gets one header per round with where it starts.
func evidence(line func(string, ...any), m *demo.Match, f audio.ExportedFile, reported, others map[uint64]bool, names map[uint64]string) {
	isolated := f.Kind == "isolated"
	full := f.Mode == audio.Full
	multi := len(f.Periods) > 1
	if full {
		if multi {
			line("  Full rounds in real time, one after the other.")
		} else {
			line("  Full round in real time: 0:00 is the round start (freeze time included).")
		}
	}

	spans := map[int]map[uint64][]span{} // period -> player -> stretches in the file
	for _, sp := range f.Speech {
		if !reported[sp.Player] {
			others[sp.Player] = true
			continue
		}
		if spans[sp.Period] == nil {
			spans[sp.Period] = map[uint64][]span{}
		}
		spans[sp.Period][sp.Player] = append(spans[sp.Period][sp.Player], span{sp.FileStart, sp.FileStart + sp.Duration})
	}

	anyone := false
	for _, id := range f.Periods {
		start, ok := periodStart(f, id)
		if !ok || id < 0 || id >= len(m.Rounds) {
			continue // nothing of this period made it into the file
		}
		indent := "  "
		if multi {
			line("  %s: starts at %s", shortName(m.Rounds[id]), clock(start))
			indent = "    "
		}
		if isolated && !full {
			continue
		}
		var ids []uint64
		for pid := range spans[id] {
			ids = append(ids, pid)
		}
		sort.Slice(ids, func(i, j int) bool { return spans[id][ids[i]][0].start < spans[id][ids[j]][0].start })
		for _, pid := range ids {
			anyone = true
			line("%s%s speaks at %s", indent, names[pid], strings.Join(limit(formatSpans(mergeSpans(spans[id][pid], speechJoinGap)), 10), ", "))
		}
	}
	if !anyone && !isolated {
		line("  (the reported player(s) don't speak in this file)")
	}
}

// periodStart: where a period begins in the file (its first segment).
func periodStart(f audio.ExportedFile, period int) (float64, bool) {
	for _, g := range f.Segments {
		if g.Period == period {
			return float64(g.Out) / audio.SR, true
		}
	}
	return 0, false
}

func describe(m *demo.Match, f audio.ExportedFile, names map[uint64]string) string {
	var ps []string
	for _, id := range f.Periods {
		if id >= 0 && id < len(m.Rounds) {
			ps = append(ps, periodName(m.Rounds[id]))
		}
	}
	where := strings.Join(ps, ", ")
	switch f.Kind {
	case "isolated":
		if f.Mode == audio.Full {
			return fmt.Sprintf("Voice of %s only (isolated) — %s", names[f.Player], where)
		}
		return fmt.Sprintf("Voice of %s only (isolated, silences cut) — %s", names[f.Player], where)
	case "alltalk":
		return fmt.Sprintf("All-talk (both teams could hear each other) — %s", where)
	default:
		return fmt.Sprintf("Voice comms of %s (what this team heard) — %s", f.Team, where)
	}
}

func periodName(r demo.Round) string {
	switch r.Kind {
	case demo.KindRound:
		s := fmt.Sprintf("Round %d", r.Number)
		if r.TeamCT != "" {
			s += fmt.Sprintf(" (%s %d - %d %s after the round)", r.TeamCT, r.ScoreCT, r.ScoreT, r.TeamT)
		}
		return s
	case demo.KindBreak:
		if r.AllTalk {
			return r.Label + " (all-talk)"
		}
		return r.Label
	}
	return r.Label
}

// shortName: "Round 2", "Halftime", "Knife"... (periodName without the score).
func shortName(r demo.Round) string {
	if r.Kind == demo.KindRound {
		return fmt.Sprintf("Round %d", r.Number)
	}
	return r.Label
}

func finalScore(m *demo.Match) string {
	for i := len(m.Rounds) - 1; i >= 0; i-- {
		r := m.Rounds[i]
		if r.Kind == demo.KindRound && r.Winner != "" {
			return fmt.Sprintf(" — final score %s %d - %d %s", r.TeamCT, r.ScoreCT, r.ScoreT, r.TeamT)
		}
	}
	return ""
}

// speechJoinGap: talk with pauses up to this long (seconds) is one stretch in the report.
const speechJoinGap = 2.0

type span struct{ start, end float64 }

func mergeSpans(in []span, gap float64) []span {
	sort.Slice(in, func(i, j int) bool { return in[i].start < in[j].start })
	var out []span
	for _, sp := range in {
		if n := len(out); n > 0 && sp.start-out[n-1].end <= gap {
			out[n-1].end = max(out[n-1].end, sp.end)
			continue
		}
		out = append(out, sp)
	}
	return out
}

// formatSpans: "0:02-0:05" for a stretch, "0:09" when it starts and ends in the same second.
func formatSpans(in []span) []string {
	var out []string
	for _, sp := range in {
		a, b := clock(sp.start), clock(sp.end)
		if a == b {
			out = append(out, a)
		} else {
			out = append(out, a+"-"+b)
		}
	}
	return out
}

func limit(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return append(s[:n:n], fmt.Sprintf("… (+%d more)", len(s)-n))
}

func clock(sec float64) string {
	s := int(sec + 0.5)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
