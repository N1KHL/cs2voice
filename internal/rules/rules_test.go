package rules

import (
	"testing"

	"cs2voice/internal/demo"
)

func TestSelect(t *testing.T) {
	list, err := parse([]byte(`{"platforms":{"faceit":[
		{"from_game_version":0,"alltalk_notices":false},
		{"from_game_version":14100,"alltalk_notices":true},
		{"from_game_version":15000,"alltalk_notices":false}
	]}}`))
	if err != nil {
		t.Fatal(err)
	}
	for v, want := range map[int]bool{14000: false, 14100: true, 14201: true, 14999: true, 15000: false, 16000: false} {
		e, _ := Select(list, "faceit", v)
		if e.AllTalkNotices != want {
			t.Errorf("version %d: notices=%v, want %v", v, e.AllTalkNotices, want)
		}
	}
	if _, ok := Select(list, "unknown", 14201); ok {
		t.Errorf("unknown platform must give nothing (no all-talk)")
	}
}

func TestEmbedded(t *testing.T) {
	list, err := parse(embedded)
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := Select(list, demo.PlatformFACEIT, 14201); !e.AllTalkNotices || e.WarmupAllTalk {
		t.Errorf("FACEIT 14201: notices yes, warmup team only; got %+v", e)
	}
	if e, _ := Select(list, demo.PlatformOther, 14201); e.AllTalkNotices || e.WarmupAllTalk {
		t.Errorf("other servers: no all-talk in doubt, got %+v", e)
	}
}

// warmup, R12, phase halftime, R13, match end
func handMatch() *demo.Match {
	return &demo.Match{
		Platform: demo.PlatformFACEIT, GameVersion: 14201, Duration: 400,
		Rounds: []demo.Round{
			{Kind: demo.KindWarmup, Label: "Warmup", Start: 0, Next: 100},
			{Kind: demo.KindRound, Number: 12, Label: "R12", Start: 100, Next: 200},
			{Kind: demo.KindBreak, Number: 12, Label: "Halftime", Start: 200, Next: 208},
			{Kind: demo.KindRound, Number: 13, Label: "R13", Start: 208, Next: 300},
			{Kind: demo.KindMatchEnd, Label: "Match end", Start: 300, Next: 400},
		},
		Voice: map[uint64]*demo.VoiceTrack{1: {Utterances: []demo.Utterance{{Start: 205}, {Start: 212}, {Start: 230}}}},
	}
}

func TestApplyWithNotices(t *testing.T) {
	m := handMatch()
	m.ServerMessages = []demo.ServerMessage{
		{Time: 200, Text: "[FACEIT^] Alltalk enabled."},
		{Time: 215, Text: "[FACEIT^] Alltalk disabled."},
	}
	Apply(m)
	type exp struct {
		label      string
		start, end float64
		all        bool
	}
	want := []exp{{"Warmup", 0, 100, false}, {"R12", 100, 200, false}, {"Halftime", 200, 215, true}, {"R13", 215, 300, false}, {"Match end", 300, 400, false}}
	if len(m.Rounds) != len(want) {
		t.Fatalf("got %+v", m.Rounds)
	}
	for i, w := range want {
		g := m.Rounds[i]
		if g.Label != w.label || g.Start != w.start || g.Next != w.end || g.AllTalk != w.all || g.ID != i {
			t.Errorf("period %d = %s [%v,%v) all=%v, want %+v", i, g.Label, g.Start, g.Next, g.AllTalk, w)
		}
	}
	if u := m.Voice[1].Utterances; u[0].Period != 2 || u[1].Period != 2 || u[2].Period != 3 {
		t.Errorf("utterances not moved to the new periods: %+v", u)
	}
}

// No notice found: no all-talk, the halftime time stays with R12.
func TestApplyWithoutNotices(t *testing.T) {
	m := handMatch()
	Apply(m)
	for _, r := range m.Rounds {
		if r.Kind == demo.KindBreak || r.AllTalk {
			t.Errorf("unexpected all-talk %+v", r)
		}
	}
	if m.Rounds[1].Next != 208 {
		t.Errorf("R12 should run until R13 starts, got %v", m.Rounds[1].Next)
	}
}

func TestAllTalkMidRound(t *testing.T) {
	m := handMatch()
	m.ServerMessages = []demo.ServerMessage{{Time: 240, Text: "Alltalk enabled"}, {Time: 260, Text: "Alltalk disabled"}}
	Apply(m)
	var labels []string
	for _, r := range m.Rounds {
		labels = append(labels, r.Label)
	}
	want := []string{"Warmup", "R12", "R13", "All talk (during R13)", "R13 (cont.)", "Match end"}
	if len(labels) != len(want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("labels = %v, want %v", labels, want)
		}
	}
}
