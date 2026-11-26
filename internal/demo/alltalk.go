package demo

import (
	"fmt"
	"sort"
)

func cloneRounds(rs []Round) []Round {
	return append([]Round(nil), rs...) // Sides maps are shared, they are read-only
}

func (m *Match) parsed() []Round {
	if m.base == nil { // Match built by hand (tests)
		m.base = cloneRounds(m.Rounds)
	}
	return m.base
}

// PhaseBreaks returns the halftime / overtime breaks found from the game phase
// ("half ended" until the next round starts), as parsed.
func (m *Match) PhaseBreaks() []Span {
	var out []Span
	for _, r := range m.parsed() {
		if r.Kind == KindBreak {
			out = append(out, Span{Start: r.Start, End: r.Next, StartTick: r.StartTick, EndTick: r.NextTick})
		}
	}
	return out
}

// TickAt estimates the game tick at a demo time (for spans computed from a duration).
func (m *Match) TickAt(t float64) int {
	if m.Duration <= 0 {
		return 0
	}
	return int(t / m.Duration * float64(m.EndTick))
}

// ApplyAllTalk rebuilds m.Rounds from the parsed periods with the given all-talk
// spans cut out of the rounds around them (on FACEIT halftime runs from the end
// of round 12 into the freeze time of round 13). The game-phase breaks are only
// used to name the spans; without spans there are no break periods.
//
// warmup and matchEnd say whether those periods are all-talk. source is recorded
// in m.AllTalkSource ("chat", "phase", "none"). Utterances are reassigned to the
// new periods. Safe to call again.
func (m *Match) ApplyAllTalk(spans []Span, warmup, matchEnd bool, source string) {
	base := m.parsed()

	var out []Round
	emit := func(r Round) {
		r.ID = len(out)
		switch r.Kind {
		case KindWarmup:
			r.AllTalk = warmup
		case KindMatchEnd:
			r.AllTalk = matchEnd
		case KindBreak:
			r.AllTalk = true
		default:
			r.AllTalk = false
		}
		out = append(out, r)
	}

	// Rounds without the phase breaks (their time goes back to the round before).
	phaseBreak := map[int]bool{}
	var flat []Round
	for _, r := range base {
		if r.Kind == KindBreak {
			phaseBreak[r.Number] = true
			if len(flat) > 0 {
				prev := &flat[len(flat)-1]
				prev.Next, prev.NextTick = r.Next, r.NextTick
			}
			continue
		}
		flat = append(flat, r)
	}

	spans = append([]Span(nil), spans...)
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	done := make([]bool, len(spans))

	for _, r := range flat {
		start, startTk := r.Start, r.StartTick
		pieces := 0
		piece := func(from float64, fromTk int, to float64, toTk int) {
			p := r
			p.Start, p.StartTick, p.Next, p.NextTick = from, fromTk, to, toTk
			if pieces > 0 {
				p.Label += " (cont.)"
			}
			pieces++
			emit(p)
		}
		for i, sp := range spans {
			if sp.End <= start || sp.Start >= r.Next {
				continue
			}
			if sp.Start > start {
				piece(start, startTk, sp.Start, sp.StartTick)
			}
			if !done[i] {
				done[i] = true
				label := fmt.Sprintf("All talk (during %s)", r.Label)
				if r.Kind == KindRound && phaseBreak[r.Number] {
					label = breakLabel(r.Number)
				} else if r.Kind == KindWarmup {
					label = "All talk (warmup)"
				}
				emit(Round{
					Kind: KindBreak, Number: r.Number, Label: label,
					Start: sp.Start, StartTick: sp.StartTick, End: sp.End, EndTick: sp.EndTick, Next: sp.End, NextTick: sp.EndTick,
					TeamCT: r.TeamCT, TeamT: r.TeamT, ScoreCT: r.ScoreCT, ScoreT: r.ScoreT, Sides: r.Sides,
				})
			}
			start, startTk = sp.End, sp.EndTick
			if start >= r.Next {
				break
			}
		}
		if start < r.Next {
			piece(start, startTk, r.Next, r.NextTick)
		}
	}

	m.Rounds = out
	m.AllTalkSource = source
	for _, tr := range m.Voice {
		for i := range tr.Utterances {
			tr.Utterances[i].Period = PeriodAt(out, tr.Utterances[i].Start)
		}
	}
}
