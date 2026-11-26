package audio

import (
	"math"
	"os"
	"testing"

	"cs2voice/internal/codec"
	"cs2voice/internal/demo"
	"cs2voice/internal/rules"
)

func TestPeriodsLabel(t *testing.T) {
	m := &demo.Match{Rounds: []demo.Round{{ID: 0, Kind: demo.KindWarmup}}}
	for n := 1; n <= 30; n++ {
		m.Rounds = append(m.Rounds, demo.Round{ID: len(m.Rounds), Kind: demo.KindRound, Number: n})
		if n == 12 || n == 27 {
			m.Rounds = append(m.Rounds, demo.Round{ID: len(m.Rounds), Kind: demo.KindBreak, Number: n, AllTalk: true})
		}
	}
	id := func(kind string, n int) int {
		for _, r := range m.Rounds {
			if r.Kind == kind && r.Number == n {
				return r.ID
			}
		}
		t.Fatalf("no %s %d", kind, n)
		return -1
	}
	R := func(n int) int { return id(demo.KindRound, n) }
	cases := map[string][]int{
		"R05":              {R(5)},
		"R05-R07_R12":      {R(5), R(6), R(7), R(12)},
		"warmup_R01-R03":   {0, R(1), R(2), R(3)},
		"R12_halftime_R13": {R(12), id(demo.KindBreak, 12), R(13)},
		"R11-R14":          {R(11), R(12), R(13), R(14)}, // halftime not selected
		"break-after-R27":  {id(demo.KindBreak, 27)},
		"R01_R03":          {R(1), R(3)},
	}
	for want, in := range cases {
		if got := periodsLabel(m, in); got != want {
			t.Errorf("periodsLabel(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestSoftClip(t *testing.T) {
	for _, v := range []float32{0, 0.5, -0.8} {
		if softClip(v) != v {
			t.Errorf("softClip(%v) changed a normal level", v)
		}
	}
	for _, v := range []float32{1.5, -3, 10} {
		if c := softClip(v); c > 1 || c < -1 {
			t.Errorf("softClip(%v) = %v, out of range", v, c)
		}
	}
}

func loadTestDemo(t *testing.T) *demo.Match {
	path := os.Getenv("CS2VOICE_TEST_DEMO")
	if path == "" {
		t.Skip("CS2VOICE_TEST_DEMO not set")
	}
	m, err := demo.Parse(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	rules.Apply(m)
	return m
}

// Set CS2VOICE_TEST_DEMO to a demo with voice to run this one.
func TestRenderRealDemo(t *testing.T) {
	m := loadTestDemo(t)
	var all []uint64
	for _, p := range m.Players {
		all = append(all, p.SteamID)
	}
	teams, byTeam := m.SplitByTeam(all)
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %v", teams)
	}

	var round, brk *demo.Round
	for i := range m.Rounds {
		r := &m.Rounds[i]
		if r.Kind == demo.KindRound && r.Number == 6 {
			round = r
		}
		if r.Kind == demo.KindBreak && brk == nil {
			brk = r
		}
	}
	if round == nil || brk == nil {
		t.Fatal("test demo needs round 6 and a halftime")
	}

	// Normal round: both teams without a point of view is refused...
	if _, err := NewPlan(m, Selection{Periods: []int{round.ID}, Players: all}); err != ErrMixedTeams {
		t.Fatalf("both teams in a normal round: got %v, want ErrMixedTeams", err)
	}
	// ...with a point of view, only that team is heard.
	p, err := NewPlan(m, Selection{Periods: []int{round.ID}, Players: all, Team: teams[0]})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range p.Players {
		if m.TeamOf(id) != teams[0] {
			t.Errorf("round 6 from %s's view contains %d of the other team", teams[0], id)
		}
	}

	// Halftime is all-talk: both teams are heard.
	hp, err := NewPlan(m, Selection{Periods: []int{brk.ID}, Players: all})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, id := range hp.Players {
		seen[m.TeamOf(id)] = true
	}
	t.Logf("%s: %.1fs, teams heard: %v", brk.Label, hp.Duration(), seen)

	// Rendering decodes Opus, which only exists in the browser build; native
	// test runs get the codec stub, so the checks above are all they can do.
	dec, err := codec.NewOpusDecoder()
	if err != nil {
		t.Skipf("Opus decoder unavailable, skipping render checks: %v", err)
	}
	dec.Close()

	for _, mode := range []Mode{Compact, Full} {
		p, err := NewPlan(m, Selection{Periods: []int{round.ID}, Players: byTeam[teams[0]], Mode: mode})
		if err != nil {
			t.Fatal(err)
		}
		total := 0
		wave, err := p.Render(func(b []float32) error { total += len(b); return nil }, 50)
		if err != nil {
			t.Fatal(err)
		}
		if total != p.Length {
			t.Errorf("mode %d: rendered %d samples, plan says %d", mode, total, p.Length)
		}
		if len(wave.Peaks) != len(wave.Players) || len(wave.Players) == 0 {
			t.Fatalf("mode %d: waveform has %d players", mode, len(wave.Players))
		}
		speaking := 0
		for _, pk := range wave.Peaks {
			for _, v := range pk {
				if v > 20 {
					speaking++
				}
			}
		}
		if speaking == 0 {
			t.Errorf("mode %d: waveform is flat", mode)
		}
		for _, s := range p.Segments {
			if _, id, ok := p.OutToDemo(float64(s.Out) / SR); !ok || id != round.ID {
				t.Errorf("mode %d: segment maps to period %d, want %d", mode, id, round.ID)
			}
		}
		t.Logf("mode %d: %.1fs, %d segments, %d utterances", mode, p.Duration(), len(p.Segments), len(p.Placements))
	}
}

// Marks are kept in demo time; DemoToFile must land them on the right spot of
// an exported file, in compact and full mode.
func TestDemoToFile(t *testing.T) {
	m := loadTestDemo(t)
	var round int = -1
	for _, r := range m.Rounds {
		if r.Kind == demo.KindRound {
			round = r.ID
			break
		}
	}
	team := m.Players[0].Team
	_, byTeam := m.SplitByTeam(func() (ids []uint64) {
		for _, p := range m.Players {
			ids = append(ids, p.SteamID)
		}
		return
	}())
	for _, mode := range []Mode{Compact, Full} {
		files, err := PlanExport(m, ExportOptions{Periods: []int{round}, AudioPlayers: byTeam[team], Mode: mode,
			PerRound: true, IncludeFull: true})
		if err != nil || len(files) == 0 {
			t.Fatalf("mode %v: %v (%d files)", mode, err, len(files))
		}
		f := files[0]
		for _, sp := range f.Speech {
			s, _, ok := f.DemoToFile(sp.DemoStart, sp.DemoStart+0.2)
			if !ok || math.Abs(s-sp.FileStart) > 0.05 {
				t.Fatalf("mode %v: speech at demo %.2f is at %.2f in the file, DemoToFile says %.2f (ok=%v)", mode, sp.DemoStart, sp.FileStart, s, ok)
			}
		}
	}
}
