package demo

import (
	"fmt"
	"strings"

	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
)

// computeScores fills the score after each round by counting round winners per team.
// Reading the score from the game state is unreliable: at half time / overtime the
// sides swap before the round is officially over.
//
// Teams are told apart by their players (who is on CT vs T), so it also works on
// servers without clan names.
// It returns team A (CT in the first round) for naming teams without a clan name.
func computeScores(rounds []Round, baseCT, baseT int) (teamA map[uint64]bool) {
	var teamB map[uint64]bool
	score := map[bool]int{} // true = team A
	for i := range rounds {
		r := &rounds[i]
		if r.Number == 0 || r.Kind == KindKnife || len(r.Sides) == 0 {
			continue
		}
		if teamA == nil {
			teamA, teamB = map[uint64]bool{}, map[uint64]bool{}
			for id, s := range r.Sides {
				if s == "CT" {
					teamA[id] = true
				} else {
					teamB[id] = true
				}
			}
			score[true], score[false] = baseCT, baseT
		}
		a, bb := 0, 0
		for id, s := range r.Sides {
			if s != "CT" {
				continue
			}
			if teamA[id] {
				a++
			}
			if teamB[id] {
				bb++
			}
		}
		ctIsA := a >= bb
		switch r.Winner {
		case "CT":
			score[ctIsA]++
		case "T":
			score[!ctIsA]++
		}
		r.ScoreCT, r.ScoreT = score[ctIsA], score[!ctIsA]

		// No clan names on this server: call the teams "Team A" / "Team B".
		if r.TeamCT == "" && r.TeamT == "" {
			r.TeamCT, r.TeamT = "Team A", "Team B"
			if !ctIsA {
				r.TeamCT, r.TeamT = r.TeamT, r.TeamCT
			}
			if r.Winner == "CT" {
				r.WinnerTeam = r.TeamCT
			} else if r.Winner == "T" {
				r.WinnerTeam = r.TeamT
			}
		}
	}
	return teamA
}

// periods merges the rounds and the breaks into one ordered list. A break cuts
// the end of the round before it: [round start, break start) [break start, next round).
func (b *builder) periods() []Round {
	var out []Round
	add := func(r Round) {
		r.ID = len(out)
		out = append(out, r)
	}
	for _, r := range b.pre {
		if r.Kind != KindKnife {
			r.Kind = KindWarmup
		}
		if r.Label == "" {
			r.Label = "Warmup"
		}
		add(r)
	}
	bi := 0
	for i, r := range b.rounds {
		switch {
		case i == 0:
			r.Kind = KindWarmup
			if r.Label == "" {
				r.Label = "Warmup"
			}
		case r.Kind == KindKnife: // knife round without a match start after it
			r.Label = "Knife"
		default:
			r.Kind, r.Label = KindRound, fmt.Sprintf("R%02d", r.Number)
		}
		for bi < len(b.breaks) && b.breaks[bi].afterRound < r.Number {
			bi++ // break of a round that was dropped (restart)
		}
		if bi < len(b.breaks) && b.breaks[bi].afterRound == r.Number && r.Kind == KindRound {
			br := b.breaks[bi]
			bi++
			end, endTk := r.Next, r.NextTick
			if br.end > br.start && br.end < end {
				end, endTk = br.end, br.endTk
			}
			if br.start > r.Start && br.start < end {
				r.Next, r.NextTick = br.start, br.startTk
				add(r)
				add(Round{
					Kind: KindBreak, Number: r.Number, Label: breakLabel(r.Number),
					Start: br.start, StartTick: br.startTk, End: end, EndTick: endTk, Next: end, NextTick: endTk,
					TeamCT: r.TeamCT, TeamT: r.TeamT, ScoreCT: r.ScoreCT, ScoreT: r.ScoreT,
					Sides: r.Sides,
				})
				// time between the break end and the next round start (normally none)
				continue
			}
		}
		add(r)
	}

	// Match end: from "game ended" to the end of the demo, cut from the last round.
	if b.endStart > 0 && len(out) > 1 {
		last := &out[len(out)-1]
		if last.Kind == KindRound && b.endStart > last.Start && b.endStart < last.Next {
			end, endTk := last.Next, last.NextTick
			last.Next, last.NextTick = b.endStart, b.endStartTk
			add(Round{
				Kind: KindMatchEnd, Number: last.Number, Label: "Match end",
				Start: b.endStart, StartTick: b.endStartTk, End: end, EndTick: endTk, Next: end, NextTick: endTk,
				TeamCT: last.TeamCT, TeamT: last.TeamT, ScoreCT: last.ScoreCT, ScoreT: last.ScoreT,
				Sides: last.Sides,
			})
		}
	}
	return out
}

// breakLabel names a break by the round before it (MR12, overtime MR3).
func breakLabel(after int) string {
	switch {
	case after == 12:
		return "Halftime"
	case after == 24:
		return "Break before overtime"
	case after > 24 && (after-24)%6 == 3:
		return "OT halftime"
	case after > 24 && (after-24)%6 == 0:
		return "Break between overtimes"
	}
	return fmt.Sprintf("Break after R%02d", after)
}

func reasonText(r events.RoundEndReason) string {
	switch r {
	case events.RoundEndReasonTargetBombed:
		return "bomb exploded"
	case events.RoundEndReasonBombDefused:
		return "bomb defused"
	case events.RoundEndReasonCTWin:
		return "Ts eliminated"
	case events.RoundEndReasonTerroristsWin:
		return "CTs eliminated"
	case events.RoundEndReasonTargetSaved:
		return "time ran out"
	case events.RoundEndReasonCTSurrender, events.RoundEndReasonTerroristsSurrender:
		return "surrender"
	case events.RoundEndReasonDraw:
		return "draw"
	}
	return ""
}

// cleanText removes the color codes and control characters of a chat notice.
func cleanText(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r >= 0x20 && r != 0x7f {
			sb.WriteRune(r)
		}
	}
	return strings.TrimSpace(sb.String())
}

func clock(sec float64) string {
	s := int(sec)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
