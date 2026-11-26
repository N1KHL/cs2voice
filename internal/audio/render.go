// Package audio builds playable audio from a selection of rounds and players:
// "compact" (only the talking, with short pauses) or "full" (real time).
package audio

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"cs2voice/internal/codec"
	"cs2voice/internal/demo"
)

const SR = codec.SampleRate

type Mode int

const (
	Compact Mode = iota // only the talking, silences cut
	Full                // the whole round in real time
)

// Compact mode timing.
const (
	keepGap    = 0.6 // real pauses up to this long are kept as they are
	cutGap     = 0.5 // longer pauses are replaced by this
	roundGap   = 1.0 // pause between rounds (compact, or non-consecutive rounds in full)
	blockSize  = SR / 10
	softKnee   = 0.8
	maxSamples = SR * 60 * 60 * 4 // 4 hours: safety limit
)

// Selection is what to build: which periods (rounds, breaks, warmup, by ID),
// which players, and from which team's point of view.
//
// The audio is what one team heard: in normal rounds only that team's players
// (teams can't hear each other); in all-talk periods (halftime, overtime
// breaks) every selected player, from both teams.
type Selection struct {
	Periods []int
	Players []uint64
	Team    string // point of view; "" = the team of the selected players
	Mode    Mode
	Clip    *demo.Span // optional demo-time window [Start, End]
}

// Placement is one utterance placed in the output.
type Placement struct {
	Player    uint64
	Utterance demo.Utterance
	Out       int // first output sample
	Len       int // samples
	Skip      int // leading samples to skip after decoding Utterance (when clipped)
}

// Segment is a stretch of the output that follows demo time continuously.
// Between segments there is a cut (compact) or a period change.
type Segment struct {
	Out, Len int     // output samples
	Demo     float64 // demo time (seconds) at Out
	Period   int     // period ID
}

type Plan struct {
	m          *demo.Match
	Team       string   // point of view
	Players    []uint64 // players that are heard somewhere in the output
	Placements []Placement
	Segments   []Segment
	Length     int // samples

	// Normalize evens out loudness per utterance. For LISTENING only: exports
	// keep the original levels, because loudness itself can be evidence
	// (someone blowing into the mic or screaming on purpose).
	Normalize bool
}

func (p *Plan) Duration() float64 { return float64(p.Length) / SR }

// OutToDemo maps an output position (seconds) to demo time and period, for the UI.
// Inside a cut (the short pause between segments) it returns the start of the
// next segment.
func (p *Plan) OutToDemo(sec float64) (demoSec float64, period int, ok bool) {
	s := int(math.Round(sec * SR))
	for _, g := range p.Segments {
		if s < g.Out {
			return g.Demo, g.Period, true
		}
		if s < g.Out+g.Len {
			return g.Demo + float64(s-g.Out)/SR, g.Period, true
		}
	}
	if n := len(p.Segments); n > 0 {
		g := p.Segments[n-1]
		return g.Demo + float64(g.Len)/SR, g.Period, true
	}
	return 0, 0, false
}

func uttSamples(tr *demo.VoiceTrack, u demo.Utterance) int {
	n := 0
	for i := u.First; i <= u.Last; i++ {
		n += tr.Packets[i].Samples
	}
	return n
}

// ErrMixedTeams: in normal rounds each team only hears its own players, so a
// selection with players from both teams needs a point of view (Selection.Team).
var ErrMixedTeams = errors.New("the selection has players from both teams in normal rounds; audio is built from one team's point of view")

// NewPlan lays out the selection on an output timeline. No audio is decoded yet.
func NewPlan(m *demo.Match, sel Selection) (*Plan, error) {
	ids := sortUniquePeriods(sel.Periods)
	var periods []demo.Round
	var validIDs []int
	for _, id := range ids {
		if id >= 0 && id < len(m.Rounds) {
			periods = append(periods, m.Rounds[id])
			validIDs = append(validIDs, id)
		}
	}

	var selected []uint64
	seen := map[uint64]bool{}
	for _, id := range sel.Players {
		if m.Voice[id] != nil && !seen[id] {
			seen[id] = true
			selected = append(selected, id)
		}
	}

	team := sel.Team
	teamPeriods := len(validIDs) > 0 && !allPeriodsAllTalk(m, validIDs)
	if team == "" && teamPeriods && len(selected) > 0 {
		teams, _ := m.SplitByTeam(selected)
		if len(teams) > 1 {
			return nil, ErrMixedTeams
		}
		team = teams[0]
	}
	p := &Plan{m: m, Team: team}
	if sel.Clip != nil && sel.Clip.End <= sel.Clip.Start {
		return p, nil
	}

	// Who is heard in a period.
	heard := func(r demo.Round) []uint64 {
		if r.AllTalk {
			return selected
		}
		var out []uint64
		for _, id := range selected {
			if m.TeamOf(id) == team {
				out = append(out, id)
			}
		}
		return out
	}

	lastEnd := map[uint64]int{} // per player: never overlap a player with themself
	inPlan := map[uint64]bool{}
	cursor := 0
	prevID := -10

	for _, r := range periods {
		var items []planItem
		for _, id := range heard(r) {
			tr := m.Voice[id]
			for _, u := range tr.Utterances {
				if u.Period != r.ID {
					continue
				}
				it := planItem{player: id, u: u, n: uttSamples(tr, u)}
				if sel.Clip != nil {
					var ok bool
					it, ok = clipItem(tr, it, *sel.Clip)
					if !ok {
						continue
					}
				}
				items = append(items, it)
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].u.Start < items[j].u.Start })
		addPlayers := func() {
			for _, it := range items {
				if !inPlan[it.player] {
					inPlan[it.player] = true
					p.Players = append(p.Players, it.player)
				}
			}
		}

		if sel.Mode == Full {
			segStartDemo := r.Start
			segEndDemo := r.Next
			if sel.Clip != nil {
				segStartDemo = max(r.Start, sel.Clip.Start)
				segEndDemo = min(r.Next, sel.Clip.End)
				if segEndDemo <= segStartDemo && len(items) == 0 {
					continue
				}
				if segEndDemo < segStartDemo {
					segEndDemo = segStartDemo
				}
			}
			addPlayers()
			// Adjacent periods (R12, halftime, R13) follow each other without a gap.
			if prevID >= 0 && r.ID != prevID+1 {
				cursor += int(roundGap * SR)
			}
			base := cursor
			end := base + int(math.Round((segEndDemo-segStartDemo)*SR))
			for _, it := range items {
				out := max(base+int(math.Round((it.u.Start-segStartDemo)*SR)), lastEnd[it.player])
				p.Placements = append(p.Placements, Placement{it.player, it.u, out, it.n, it.skip})
				lastEnd[it.player] = out + it.n
				end = max(end, out+it.n)
			}
			p.Segments = append(p.Segments, Segment{Out: base, Len: end - base, Demo: segStartDemo, Period: r.ID})
			cursor = end
			prevID = r.ID
			continue
		}

		// Compact: group utterances that are close in time into clusters, keep
		// their real timing inside a cluster, and cut the silence between clusters.
		if len(items) == 0 {
			continue
		}
		addPlayers()
		if len(p.Segments) > 0 {
			cursor += int(roundGap * SR)
		}
		var segStart int
		var segDemo, clusterEnd float64 // demo time
		for i, it := range items {
			if i == 0 || it.u.Start > clusterEnd+keepGap {
				if i > 0 {
					p.Segments = append(p.Segments, Segment{Out: segStart, Len: cursor - segStart, Demo: segDemo, Period: r.ID})
					cursor += int(cutGap * SR)
				}
				segStart, segDemo, clusterEnd = cursor, it.u.Start, it.u.Start
			}
			out := max(segStart+int(math.Round((it.u.Start-segDemo)*SR)), lastEnd[it.player])
			p.Placements = append(p.Placements, Placement{it.player, it.u, out, it.n, it.skip})
			lastEnd[it.player] = out + it.n
			cursor = max(cursor, out+it.n)
			clusterEnd = max(clusterEnd, it.u.Start+float64(it.n)/SR)
		}
		p.Segments = append(p.Segments, Segment{Out: segStart, Len: cursor - segStart, Demo: segDemo, Period: r.ID})
	}

	p.Length = cursor
	if p.Length > maxSamples {
		return nil, fmt.Errorf("selection too long (%.0f minutes)", p.Duration()/60)
	}
	sort.SliceStable(p.Placements, func(i, j int) bool { return p.Placements[i].Out < p.Placements[j].Out })
	return p, nil
}

// Waveform holds, per player, the peak level of each short bucket of the output.
// The UI draws it as one line colored by who is talking.
type Waveform struct {
	Rate    int       // buckets per second
	Players []uint64  // same order as Peaks
	Peaks   [][]uint8 // 0..255
}

// Render decodes and mixes the plan block by block and passes each block to sink.
// If waveRate > 0, it also returns the waveform with that many buckets per second.
func (p *Plan) Render(sink func([]float32) error, waveRate int) (*Waveform, error) {
	decoders := map[uint64]*codec.OpusDecoder{}
	defer func() {
		for _, d := range decoders {
			d.Close()
		}
	}()

	var wave *Waveform
	pIndex := map[uint64]int{}
	var peaks [][]float32
	bucket := 0
	if waveRate > 0 {
		wave = &Waveform{Rate: waveRate, Players: p.Players}
		nb := int(math.Ceil(p.Duration()*float64(waveRate))) + 1
		bucket = SR / waveRate
		for i, id := range p.Players {
			pIndex[id] = i
			peaks = append(peaks, make([]float32, nb))
		}
	}

	type active struct {
		player uint64
		out    int
		pcm    []float32
	}
	var act []active
	next := 0
	block := make([]float32, blockSize)

	for s := 0; s < p.Length; s += blockSize {
		n := min(blockSize, p.Length-s)
		blk := block[:n]
		clear(blk)

		for next < len(p.Placements) && p.Placements[next].Out < s+n {
			pl := p.Placements[next]
			next++
			pcm, err := decodeUtterance(func() (*codec.OpusDecoder, error) {
				if d := decoders[pl.Player]; d != nil {
					return d, nil
				}
				d, err := codec.NewOpusDecoder()
				if err != nil {
					return nil, err
				}
				decoders[pl.Player] = d
				return d, nil
			}, p.m.Voice[pl.Player], pl.Utterance, pl.Skip, pl.Len)
			if err != nil {
				return nil, err
			}
			if p.Normalize {
				normalize(pcm)
			}
			act = append(act, active{pl.Player, pl.Out, pcm})
		}

		keep := act[:0]
		for _, a := range act {
			from := max(s, a.out)
			to := min(s+n, a.out+len(a.pcm))
			var pk []float32
			if wave != nil {
				pk = peaks[pIndex[a.player]]
			}
			for i := from; i < to; i++ {
				v := a.pcm[i-a.out]
				blk[i-s] += v
				if pk != nil {
					if av := float32(math.Abs(float64(v))); av > pk[i/bucket] {
						pk[i/bucket] = av
					}
				}
			}
			if a.out+len(a.pcm) > s+n {
				keep = append(keep, a)
			}
		}
		act = keep

		for i, v := range blk {
			blk[i] = softClip(v)
		}
		if err := sink(blk); err != nil {
			return nil, err
		}
	}

	if wave != nil {
		for _, pk := range peaks {
			out := make([]uint8, len(pk))
			for i, v := range pk {
				// sqrt makes quiet speech visible without clipping loud parts
				out[i] = uint8(math.Min(255, math.Sqrt(float64(v))*255))
			}
			wave.Peaks = append(wave.Peaks, out)
		}
	}
	return wave, nil
}

// decodeUtterance decodes an utterance, skipping skip leading samples and
// returning exactly n samples.
func decodeUtterance(getDec func() (*codec.OpusDecoder, error), tr *demo.VoiceTrack, u demo.Utterance, skip, n int) ([]float32, error) {
	pcm := make([]float32, 0, skip+n)
	for i := u.First; i <= u.Last; i++ {
		pk := tr.Packets[i]
		before := len(pcm)
		if pk.Data != nil {
			d, err := getDec()
			if err != nil {
				return nil, err
			}
			pcm, _ = d.Decode(pcm, pk.Data)
		}
		// lost or undecodable packet: silence of the right length
		for len(pcm) < before+pk.Samples {
			pcm = append(pcm, 0)
		}
		pcm = pcm[:before+pk.Samples]
	}
	if skip > 0 {
		if skip < len(pcm) {
			pcm = pcm[skip:]
		} else {
			pcm = pcm[:0]
		}
	}
	if len(pcm) > n {
		pcm = pcm[:n]
	}
	for len(pcm) < n {
		pcm = append(pcm, 0)
	}
	return pcm, nil
}

// normalize scales an utterance towards a common loudness (-20 dBFS RMS),
// within limits so silence and noise aren't blown up.
func normalize(pcm []float32) {
	const target, minGain, maxGain = 0.1, 0.25, 6.0
	var sum float64
	n := 0
	for _, v := range pcm {
		if v != 0 {
			sum += float64(v) * float64(v)
			n++
		}
	}
	if n == 0 {
		return
	}
	rms := math.Sqrt(sum / float64(n))
	if rms < 1e-4 {
		return
	}
	g := float32(math.Max(minGain, math.Min(maxGain, target/rms)))
	for i := range pcm {
		pcm[i] *= g
	}
}

// softClip leaves normal levels untouched and bends peaks above the knee
// smoothly towards 1, so overlapping voices don't crackle.
func softClip(v float32) float32 {
	a := math.Abs(float64(v))
	if a <= softKnee {
		return v
	}
	y := softKnee + (1-softKnee)*math.Tanh((a-softKnee)/(1-softKnee))
	return float32(math.Copysign(y, float64(v)))
}
