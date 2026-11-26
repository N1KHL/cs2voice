package audio

import (
	"archive/zip"
	"bytes"
	"io"
	"math"
	"strings"
	"testing"

	"cs2voice/internal/demo"
)

func makeSyntheticMatch() *demo.Match {
	const pktSamples = 960 // 20 ms at 48 kHz
	makePackets := func(start float64, startTick, count int) []demo.Packet {
		pkts := make([]demo.Packet, count)
		for i := range pkts {
			pkts[i] = demo.Packet{
				Time:    start + float64(i*pktSamples)/SR,
				Tick:    startTick + i,
				Samples: pktSamples,
			}
		}
		return pkts
	}

	aliceID := uint64(1001)
	bobID := uint64(1002)
	carolID := uint64(2001)

	alicePkts := append(makePackets(12.0, 100, 100), makePackets(20.0, 600, 50)...)
	alicePkts = append(alicePkts, makePackets(42.0, 2000, 50)...)
	bobPkts := append(makePackets(13.5, 200, 50), makePackets(60.0, 3000, 100)...)
	carolPkts := append(makePackets(12.5, 130, 50), makePackets(44.0, 2200, 50)...)

	return &demo.Match{
		FaceitMatchID: "1-abc123",
		Duration:      85.0,
		Rounds: []demo.Round{
			{ID: 0, Kind: demo.KindWarmup, Label: "Warmup", Start: 0, Next: 10},
			{ID: 1, Kind: demo.KindRound, Number: 1, Label: "R01", Start: 10, Next: 40},
			{ID: 2, Kind: demo.KindBreak, Number: 12, Label: "Halftime", Start: 40, Next: 55, AllTalk: true},
			{ID: 3, Kind: demo.KindRound, Number: 13, Label: "R13", Start: 55, Next: 85},
		},
		Players: []demo.Player{
			{SteamID: aliceID, Name: "Alice", Team: "TeamA", Playing: true},
			{SteamID: bobID, Name: "Bob", Team: "TeamA", Playing: true},
			{SteamID: carolID, Name: "Carol / Toxic", Team: "TeamB", Playing: true},
		},
		Voice: map[uint64]*demo.VoiceTrack{
			aliceID: {
				SteamID: aliceID,
				Packets: alicePkts,
				Utterances: []demo.Utterance{
					{Start: 12.0, End: 14.0, StartTick: 100, First: 0, Last: 99, Period: 1},
					{Start: 20.0, End: 21.0, StartTick: 600, First: 100, Last: 149, Period: 1},
					{Start: 42.0, End: 43.0, StartTick: 2000, First: 150, Last: 199, Period: 2},
				},
			},
			bobID: {
				SteamID: bobID,
				Packets: bobPkts,
				Utterances: []demo.Utterance{
					{Start: 13.5, End: 14.5, StartTick: 200, First: 0, Last: 49, Period: 1},
					{Start: 60.0, End: 62.0, StartTick: 3000, First: 50, Last: 149, Period: 3},
				},
			},
			carolID: {
				SteamID: carolID,
				Packets: carolPkts,
				Utterances: []demo.Utterance{
					{Start: 12.5, End: 13.5, StartTick: 130, First: 0, Last: 49, Period: 1},
					{Start: 44.0, End: 45.0, StartTick: 2200, First: 50, Last: 99, Period: 2},
				},
			},
		},
	}
}

func assertNear(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-4 {
		t.Fatalf("%s = %.6f, want %.6f", name, got, want)
	}
}

func TestClipFullSinglePeriod(t *testing.T) {
	m := makeSyntheticMatch()
	clip := demo.Span{Start: 12.5, End: 14.0}
	p, err := NewPlan(m, Selection{
		Periods: []int{1},
		Players: []uint64{1001, 1002, 2001},
		Team:    "TeamA",
		Mode:    Full,
		Clip:    &clip,
	})
	if err != nil {
		t.Fatal(err)
	}

	assertNear(t, "Duration", p.Duration(), 1.5)
	if len(p.Segments) != 1 {
		t.Fatalf("len(Segments) = %d, want 1", len(p.Segments))
	}
	seg := p.Segments[0]
	if seg.Out != 0 || seg.Len != int(1.5*SR) || seg.Period != 1 {
		t.Fatalf("unexpected segment: %+v", seg)
	}
	assertNear(t, "seg.Demo", seg.Demo, 12.5)

	// Only TeamA players should be present (Carol on TeamB excluded in normal round).
	if len(p.Players) != 2 || p.Players[0] != 1001 || p.Players[1] != 1002 {
		t.Fatalf("Players = %v, want [1001 1002]", p.Players)
	}
	// Alice's [20,21) utterance is outside [12.5,14.0), so 2 placements remain.
	if len(p.Placements) != 2 {
		t.Fatalf("len(Placements) = %d, want 2: %+v", len(p.Placements), p.Placements)
	}
	pl0, pl1 := p.Placements[0], p.Placements[1]
	if pl0.Player != 1001 || pl0.Out != 0 || pl0.Len != int(1.5*SR) || pl0.Skip != 0 || pl0.Utterance.First != 25 || pl0.Utterance.Last != 99 {
		t.Fatalf("unexpected Alice placement: %+v", pl0)
	}
	if pl1.Player != 1002 || pl1.Out != SR || pl1.Len != int(0.5*SR) || pl1.Skip != 0 || pl1.Utterance.First != 0 || pl1.Utterance.Last != 24 {
		t.Fatalf("unexpected Bob placement: %+v", pl1)
	}

	d0, per0, ok := p.OutToDemo(0)
	if !ok || per0 != 1 {
		t.Fatalf("OutToDemo(0) = (%v, %d, %v)", d0, per0, ok)
	}
	assertNear(t, "OutToDemo(0)", d0, 12.5)
	d1, per1, ok := p.OutToDemo(1.0)
	if !ok || per1 != 1 {
		t.Fatalf("OutToDemo(1.0) = (%v, %d, %v)", d1, per1, ok)
	}
	assertNear(t, "OutToDemo(1.0)", d1, 13.5)
}

func TestClipSubPacketSkip(t *testing.T) {
	m := makeSyntheticMatch()
	// 12.01 is 480 samples into Alice's first 960-sample packet; 12.05 ends 480 samples into packet 2.
	clip := demo.Span{Start: 12.01, End: 12.05}
	p, err := NewPlan(m, Selection{
		Periods: []int{1},
		Players: []uint64{1001},
		Team:    "TeamA",
		Mode:    Compact,
		Clip:    &clip,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Placements) != 1 {
		t.Fatalf("len(Placements) = %d, want 1", len(p.Placements))
	}
	pl := p.Placements[0]
	if pl.Utterance.First != 0 || pl.Utterance.Last != 2 || pl.Skip != 480 || pl.Len != 1920 {
		t.Fatalf("sub-packet clip placement = %+v, want First=0 Last=2 Skip=480 Len=1920", pl)
	}
	assertNear(t, "Duration", p.Duration(), 0.04)
}

func TestClipCompactMode(t *testing.T) {
	m := makeSyntheticMatch()
	clip := demo.Span{Start: 13.0, End: 20.5}
	p, err := NewPlan(m, Selection{
		Periods: []int{1},
		Players: []uint64{1001, 1002},
		Team:    "TeamA",
		Mode:    Compact,
		Clip:    &clip,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Cluster 1: Alice [13.0, 14.0) + Bob [13.5, 14.5) => 1.5s
	// cutGap: 0.5s
	// Cluster 2: Alice [20.0, 20.5) => 0.5s
	// Total: 2.5s
	assertNear(t, "Duration", p.Duration(), 2.5)
	if len(p.Segments) != 2 {
		t.Fatalf("len(Segments) = %d, want 2", len(p.Segments))
	}
	assertNear(t, "seg0.Demo", p.Segments[0].Demo, 13.0)
	assertNear(t, "seg1.Demo", p.Segments[1].Demo, 20.0)
	if p.Segments[0].Out != 0 || p.Segments[0].Len != int(1.5*SR) {
		t.Fatalf("unexpected seg0: %+v", p.Segments[0])
	}
	if p.Segments[1].Out != 2*SR || p.Segments[1].Len != int(0.5*SR) {
		t.Fatalf("unexpected seg1: %+v", p.Segments[1])
	}

	d0, per0, ok := p.OutToDemo(0.5)
	if !ok || per0 != 1 {
		t.Fatalf("OutToDemo(0.5) = (%v, %d, %v)", d0, per0, ok)
	}
	assertNear(t, "OutToDemo(0.5)", d0, 13.5)

	d1, per1, ok := p.OutToDemo(2.25)
	if !ok || per1 != 1 {
		t.Fatalf("OutToDemo(2.25) = (%v, %d, %v)", d1, per1, ok)
	}
	assertNear(t, "OutToDemo(2.25)", d1, 20.25)
}

func TestClipMultiPeriodFullAndCompact(t *testing.T) {
	m := makeSyntheticMatch()

	// Adjacent periods 1 [10,40) and 2 [40,55) (AllTalk): no roundGap between them in Full.
	clip := demo.Span{Start: 38.0, End: 45.0}
	pf, err := NewPlan(m, Selection{
		Periods: []int{1, 2},
		Players: []uint64{1001, 2001},
		Team:    "TeamA",
		Mode:    Full,
		Clip:    &clip,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNear(t, "adjacent Full Duration", pf.Duration(), 7.0)
	if len(pf.Segments) != 2 {
		t.Fatalf("len(Segments) = %d, want 2", len(pf.Segments))
	}
	if len(pf.Players) != 2 {
		t.Fatalf("Players = %v, want both Alice and Carol (AllTalk)", pf.Players)
	}

	// Compact across periods 1 & 2 with [38.0, 45.0]: period 1 has no speech in [38,40),
	// so only period 2's speech [42,43) and [44,45) remain (1s + 0.5s cutGap + 1s = 2.5s).
	pc, err := NewPlan(m, Selection{
		Periods: []int{1, 2},
		Players: []uint64{1001, 2001},
		Team:    "TeamA",
		Mode:    Compact,
		Clip:    &clip,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNear(t, "multi Compact Duration", pc.Duration(), 2.5)
	if len(pc.Placements) != 2 {
		t.Fatalf("len(Placements) = %d, want 2", len(pc.Placements))
	}

	// Non-consecutive periods 1 [10,40) and 3 [55,85): 1.0s roundGap inserted in Full.
	nonAdjClip := demo.Span{Start: 38.0, End: 62.0}
	pn, err := NewPlan(m, Selection{
		Periods: []int{1, 3},
		Players: []uint64{1002},
		Team:    "TeamA",
		Mode:    Full,
		Clip:    &nonAdjClip,
	})
	if err != nil {
		t.Fatal(err)
	}
	// [38,40) = 2s + 1s roundGap + [55,62) = 7s => 10s
	assertNear(t, "non-adjacent Full Duration", pn.Duration(), 10.0)
}

func TestClipEdgeCases(t *testing.T) {
	m := makeSyntheticMatch()

	// Mixed teams without POV in normal round must still error.
	clip := demo.Span{Start: 12.0, End: 15.0}
	if _, err := NewPlan(m, Selection{Periods: []int{1}, Players: []uint64{1001, 2001}, Clip: &clip}); err != ErrMixedTeams {
		t.Fatalf("got %v, want ErrMixedTeams", err)
	}

	// Outside any selected period/utterance -> empty plan.
	outClip := demo.Span{Start: 100.0, End: 110.0}
	for _, mode := range []Mode{Compact, Full} {
		p, err := NewPlan(m, Selection{Periods: []int{1}, Players: []uint64{1001}, Team: "TeamA", Mode: mode, Clip: &outClip})
		if err != nil {
			t.Fatal(err)
		}
		if p.Duration() != 0 || len(p.Segments) != 0 || len(p.Placements) != 0 || len(p.Players) != 0 {
			t.Fatalf("mode %v out-of-range plan not empty: dur=%v segs=%d pls=%d", mode, p.Duration(), len(p.Segments), len(p.Placements))
		}
	}

	// Zero/inverted clip -> empty plan.
	for _, sp := range []demo.Span{{Start: 20, End: 20}, {Start: 25, End: 20}} {
		sp := sp
		p, err := NewPlan(m, Selection{Periods: []int{1}, Players: []uint64{1001}, Team: "TeamA", Mode: Full, Clip: &sp})
		if err != nil {
			t.Fatal(err)
		}
		if p.Duration() != 0 || len(p.Segments) != 0 || len(p.Placements) != 0 {
			t.Fatalf("invalid clip %+v produced non-empty plan", sp)
		}
	}
}

func TestCutFileName(t *testing.T) {
	m := makeSyntheticMatch()

	got := CutFileName(m, Selection{
		Periods: []int{1},
		Players: []uint64{1001, 1002},
		Team:    "TeamA",
		Clip:    &demo.Span{Start: 83, End: 105},
	}, m.FaceitMatchID, MP3)
	if got != "1-abc123_TeamA_R01_1m23-1m45.mp3" {
		t.Fatalf("team filename = %q", got)
	}

	gotIso := CutFileName(m, Selection{
		Periods: []int{1},
		Players: []uint64{2001},
		Team:    "TeamB",
		Clip:    &demo.Span{Start: 12.2, End: 14.4},
	}, "", WAV)
	if gotIso != "Carol-Toxic_R01_0m12-0m14.wav" {
		t.Fatalf("isolated filename = %q", gotIso)
	}

	gotAllTalk := CutFileName(m, Selection{
		Periods: []int{2},
		Players: []uint64{1001, 2001},
		Team:    "TeamA",
		Clip:    &demo.Span{Start: 42, End: 45},
	}, "", MP3)
	if gotAllTalk != "alltalk_halftime_0m42-0m45.mp3" {
		t.Fatalf("alltalk filename = %q", gotAllTalk)
	}

	used := map[string]bool{}
	if a := uniqueName(used, "clip.mp3"); a != "clip.mp3" {
		t.Fatalf("first = %q", a)
	}
	if b := uniqueName(used, "clip.mp3"); b != "clip (2).mp3" {
		t.Fatalf("second = %q", b)
	}
	if c := uniqueName(used, "clip.mp3"); c != "clip (3).mp3" {
		t.Fatalf("third = %q", c)
	}
}

func TestCutZip(t *testing.T) {
	data, err := CutZip([]CutFile{
		{Name: "a.wav", Data: []byte("RIFF1")},
		{Name: "b.mp3", Data: []byte("ID3")},
	})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 2 || zr.File[0].Name != "a.wav" || zr.File[1].Name != "b.mp3" {
		t.Fatalf("unexpected zip entries: %+v", zr.File)
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil || string(got) != "RIFF1" {
		t.Fatalf("entry 0 = %q, %v", got, err)
	}
}

func TestExportCutWAV(t *testing.T) {
	m := makeSyntheticMatch()
	files, err := ExportCut(m, CutOptions{
		Periods: []int{1},
		Players: []uint64{1001},
		Team:    "TeamA",
		Mode:    Full,
		Format:  WAV,
		Prefix:  m.FaceitMatchID,
		Clips:   []demo.Span{{Start: 12.0, End: 13.0}, {Start: 12.0, End: 13.0}},
	})
	if err != nil {
		t.Fatalf("ExportCut WAV: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}
	if files[0].Name != "1-abc123_Alice_R01_0m12-0m13.wav" || files[1].Name != "1-abc123_Alice_R01_0m12-0m13 (2).wav" {
		t.Fatalf("unexpected deduped cut names: %q, %q", files[0].Name, files[1].Name)
	}
}

func TestExportCutMP3(t *testing.T) {
	m := makeSyntheticMatch()
	mp3Files, err := ExportCut(m, CutOptions{
		Periods: []int{1},
		Players: []uint64{1001},
		Team:    "TeamA",
		Mode:    Compact,
		Format:  MP3,
		Clips:   []demo.Span{{Start: 12.0, End: 13.0}},
	})
	if err != nil {
		if strings.Contains(err.Error(), "codec unavailable") ||
			strings.Contains(err.Error(), "not available in browser") ||
			strings.Contains(err.Error(), "cs2opus") {
			t.Skipf("MP3 codec unavailable in test harness: %v", err)
		}
		t.Fatalf("ExportCut MP3: %v", err)
	}
	if len(mp3Files) != 1 || len(mp3Files[0].Data) < 4 {
		t.Fatalf("unexpected MP3 cut output: %d files", len(mp3Files))
	}
}
