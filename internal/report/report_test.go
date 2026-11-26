package report

import (
	"os"
	"strings"
	"testing"

	"cs2voice/internal/audio"
	"cs2voice/internal/demo"
	"cs2voice/internal/rules"
)

// Prints a report with a mark, to read it (go test -v -run TestReportWithMark).
func TestReportWithMark(t *testing.T) {
	path := os.Getenv("CS2VOICE_TEST_DEMO")
	if path == "" {
		t.Skip("CS2VOICE_TEST_DEMO not set")
	}
	m, err := demo.Parse(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	rules.Apply(m)
	var round demo.Round
	for _, r := range m.Rounds {
		if r.Kind == demo.KindRound && r.Number == 2 {
			round = r
		}
	}
	team := m.Players[0].Team
	var ids []uint64
	for _, p := range m.Players {
		if p.Team == team {
			ids = append(ids, p.SteamID)
		}
	}
	files, err := audio.PlanExport(m, audio.ExportOptions{Periods: []int{round.ID}, AudioPlayers: ids, Reported: ids[:3],
		PerRound: true, IncludeFull: true, IncludeIsolated: true})
	if err != nil || len(files) == 0 {
		t.Fatal(err, len(files))
	}
	sp := files[0].Speech[len(files[0].Speech)/2]
	mark := demo.Span{Start: sp.DemoStart, End: sp.DemoStart + 4}
	txt := Build(Input{Match: m, Reported: ids[:3], Files: files, Marks: []demo.Span{mark}})
	t.Log("\n" + txt)
	if !strings.Contains(txt, "#1 "+files[0].Name+" at ") {
		t.Fatal("mark not placed in the team file")
	}
}

// The report for each combination of mode (talk only / full round) and split
// (one file per round / one file). Run with -v to read them.
func TestReportCases(t *testing.T) {
	path := os.Getenv("CS2VOICE_TEST_DEMO")
	if path == "" {
		t.Skip("CS2VOICE_TEST_DEMO not set")
	}
	m, err := demo.Parse(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	rules.Apply(m)
	var periods []int
	for _, r := range m.Rounds {
		if r.Kind == demo.KindRound && (r.Number == 2 || r.Number == 3) {
			periods = append(periods, r.ID)
		}
	}
	team := m.Players[0].Team
	var ids []uint64
	for _, p := range m.Players {
		if p.Team == team {
			ids = append(ids, p.SteamID)
		}
	}
	rep := ids[:1]
	for _, mode := range []audio.Mode{audio.Compact, audio.Full} {
		for _, perRound := range []bool{true, false} {
			files, err := audio.PlanExport(m, audio.ExportOptions{Periods: periods, AudioPlayers: ids[:2], Reported: rep,
				Mode: mode, PerRound: perRound, IncludeFull: true, IncludeIsolated: true})
			if err != nil {
				t.Fatal(err)
			}
			txt := Build(Input{Match: m, Reported: rep, Files: files})
			i := strings.Index(txt, "Evidence")
			t.Logf("\n===== mode=%v perRound=%v =====\n%s", mode, perRound, txt[i:])
			for _, f := range files {
				if f.Kind == "isolated" && f.Mode != mode {
					t.Fatalf("isolated file %s has mode %v, want %v", f.Name, f.Mode, mode)
				}
			}
		}
	}

	// Only the reported player in the audio: the team file already is their
	// voice, so there is no separate isolated file.
	files, _ := audio.PlanExport(m, audio.ExportOptions{Periods: periods, AudioPlayers: rep, Reported: rep,
		PerRound: true, IncludeFull: true, IncludeIsolated: true})
	for _, f := range files {
		if f.Kind == "isolated" {
			t.Fatalf("duplicate isolated file %s", f.Name)
		}
	}
}

func syntheticReportMatch() *demo.Match {
	const pktSamples = 960 // 20 ms at 48 kHz
	makePackets := func(start float64, startTick, count int) []demo.Packet {
		pkts := make([]demo.Packet, count)
		for i := range pkts {
			pkts[i] = demo.Packet{
				Time:    start + float64(i*pktSamples)/audio.SR,
				Tick:    startTick + i,
				Samples: pktSamples,
			}
		}
		return pkts
	}

	const (
		toxicID    = uint64(76561198000001001)
		mateID     = uint64(76561198000001002)
		opponentID = uint64(76561198000002001)
	)

	// toxicID:
	// - R01: [12, 14) and [15.5, 17) (1.5s gap <= speechJoinGap 2.0s -> merges) + [25, 27)
	// - R02: [55, 57)
	// - Halftime: [105, 107)
	toxicPkts := append(makePackets(12.0, 768, 100), makePackets(15.5, 992, 75)...)
	toxicPkts = append(toxicPkts, makePackets(25.0, 1600, 100)...)
	toxicPkts = append(toxicPkts, makePackets(55.0, 3520, 100)...)
	toxicPkts = append(toxicPkts, makePackets(105.0, 6720, 100)...)

	matePkts := makePackets(18.0, 1152, 100)
	oppPkts := makePackets(108.0, 6912, 100)

	return &demo.Match{
		File:          "faceit_match.dem.zst",
		FaceitMatchID: "1-abc12345-1234-1234-1234-1234567890ab",
		Map:           "de_mirage",
		GameVersion:   14201,
		Platform:      demo.PlatformFACEIT,
		TickRate:      64,
		Duration:      150,
		EndTick:       9600,
		Rounds: []demo.Round{
			{ID: 0, Kind: demo.KindWarmup, Label: "Warmup", Start: 0, Next: 10, StartTick: 0, NextTick: 640},
			{ID: 1, Kind: demo.KindRound, Number: 1, Label: "R01", Start: 10, Next: 50, StartTick: 640, NextTick: 3200, Winner: "CT", TeamCT: "TeamA", TeamT: "TeamB", ScoreCT: 1, ScoreT: 0},
			{ID: 2, Kind: demo.KindRound, Number: 2, Label: "R02", Start: 50, Next: 100, StartTick: 3200, NextTick: 6400, Winner: "T", TeamCT: "TeamA", TeamT: "TeamB", ScoreCT: 1, ScoreT: 1},
			{ID: 3, Kind: demo.KindBreak, Number: 12, Label: "Halftime", AllTalk: true, Start: 100, Next: 120, StartTick: 6400, NextTick: 7680, TeamCT: "TeamA", TeamT: "TeamB", ScoreCT: 1, ScoreT: 1},
		},
		Players: []demo.Player{
			{SteamID: toxicID, Name: "ToxicPlayer", Team: "TeamA", Playing: true},
			{SteamID: mateID, Name: "Mate", Team: "TeamA", Playing: true},
			{SteamID: opponentID, Name: "Opponent", Team: "TeamB", Playing: true},
		},
		Voice: map[uint64]*demo.VoiceTrack{
			toxicID: {
				SteamID: toxicID,
				Packets: toxicPkts,
				Utterances: []demo.Utterance{
					{Start: 12.0, End: 14.0, StartTick: 768, First: 0, Last: 99, Period: 1},
					{Start: 15.5, End: 17.0, StartTick: 992, First: 100, Last: 174, Period: 1},
					{Start: 25.0, End: 27.0, StartTick: 1600, First: 175, Last: 274, Period: 1},
					{Start: 55.0, End: 57.0, StartTick: 3520, First: 275, Last: 374, Period: 2},
					{Start: 105.0, End: 107.0, StartTick: 6720, First: 375, Last: 474, Period: 3},
				},
			},
			mateID: {
				SteamID: mateID,
				Packets: matePkts,
				Utterances: []demo.Utterance{
					{Start: 18.0, End: 20.0, StartTick: 1152, First: 0, Last: 99, Period: 1},
				},
			},
			opponentID: {
				SteamID: opponentID,
				Packets: oppPkts,
				Utterances: []demo.Utterance{
					{Start: 108.0, End: 110.0, StartTick: 6912, First: 0, Last: 99, Period: 3},
				},
			},
		},
	}
}

func TestBuildSyntheticCases(t *testing.T) {
	m := syntheticReportMatch()
	toxic := m.Players[0].SteamID
	mate := m.Players[1].SteamID
	teamA := []uint64{toxic, mate}
	rep := []uint64{toxic}

	// 1. Single round, team + isolated, Compact vs Full.
	for _, mode := range []audio.Mode{audio.Compact, audio.Full} {
		files, err := audio.PlanExport(m, audio.ExportOptions{
			Periods: []int{1}, AudioPlayers: teamA, Reported: rep,
			Mode: mode, PerRound: true, IncludeFull: true, IncludeIsolated: true,
		})
		if err != nil {
			t.Fatalf("mode %v single round Export: %v", mode, err)
		}
		if len(files) != 2 {
			t.Fatalf("mode %v single round files = %d, want 2", mode, len(files))
		}
		txt := Build(Input{Match: m, Reported: rep, Files: files})

		teamSectionIdx := strings.Index(txt, files[0].Name)
		isoSectionIdx := strings.Index(txt, files[1].Name)
		if teamSectionIdx < 0 || isoSectionIdx <= teamSectionIdx {
			t.Fatalf("missing file sections in report:\n%s", txt)
		}
		teamSec := txt[teamSectionIdx:isoSectionIdx]
		isoSec := txt[isoSectionIdx:]

		if !strings.Contains(txt, "Also audible in the clips: Mate.") {
			t.Errorf("mode %v missing other audible player:\n%s", mode, txt)
		}
		if mode == audio.Compact {
			// [12, 14) and [15.5, 17) merge into 0:00-0:04; Mate [18, 20) + cutGap pushes [25, 27) to 0:07-0:09.
			if !strings.Contains(teamSec, "ToxicPlayer speaks at 0:00-0:04, 0:07-0:09") {
				t.Errorf("compact team stretches mismatch:\n%s", teamSec)
			}
			if strings.Contains(isoSec, "speaks at") {
				t.Errorf("compact single-round isolated file should not list speaks at:\n%s", isoSec)
			}
		} else {
			if !strings.Contains(teamSec, "Full round in real time: 0:00 is the round start (freeze time included).") {
				t.Errorf("full team missing round start note:\n%s", teamSec)
			}
			if !strings.Contains(teamSec, "ToxicPlayer speaks at 0:02-0:07, 0:15-0:17") {
				t.Errorf("full team stretches mismatch:\n%s", teamSec)
			}
			if !strings.Contains(isoSec, "ToxicPlayer speaks at 0:02-0:07, 0:15-0:17") {
				t.Errorf("full isolated missing stretches:\n%s", isoSec)
			}
		}
	}

	// 2. Multi-round, team + isolated, Compact vs Full.
	for _, mode := range []audio.Mode{audio.Compact, audio.Full} {
		files, err := audio.PlanExport(m, audio.ExportOptions{
			Periods: []int{1, 2}, AudioPlayers: teamA, Reported: rep,
			Mode: mode, PerRound: false, IncludeFull: true, IncludeIsolated: true,
		})
		if err != nil {
			t.Fatalf("mode %v multi round Export: %v", mode, err)
		}
		txt := Build(Input{Match: m, Reported: rep, Files: files})
		if !strings.Contains(txt, "Round 1: starts at 0:00") || !strings.Contains(txt, "Round 2: starts at ") {
			t.Errorf("mode %v multi missing round headers:\n%s", mode, txt)
		}
		if mode == audio.Full && !strings.Contains(txt, "Full rounds in real time, one after the other.") {
			t.Errorf("full multi missing multi-round note:\n%s", txt)
		}
		isoIdx := strings.Index(txt, files[1].Name)
		isoSec := txt[isoIdx:]
		if mode == audio.Compact && strings.Contains(isoSec, "speaks at") {
			t.Errorf("compact multi isolated should only have round start headers:\n%s", isoSec)
		}
		if mode == audio.Full && !strings.Contains(isoSec, "ToxicPlayer speaks at") {
			t.Errorf("full multi isolated should list stretches:\n%s", isoSec)
		}
	}

	// 3. Only the reported player in AudioPlayers -> no duplicate isolated file.
	files, err := audio.PlanExport(m, audio.ExportOptions{
		Periods: []int{1, 2}, AudioPlayers: rep, Reported: rep,
		PerRound: true, IncludeFull: true, IncludeIsolated: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Kind == "isolated" {
			t.Fatalf("unexpected duplicate isolated file %s", f.Name)
		}
	}

	// 4. All-talk period includes both teams.
	atFiles, err := audio.PlanExport(m, audio.ExportOptions{
		Periods: []int{3}, AudioPlayers: []uint64{toxic, m.Players[2].SteamID}, Reported: rep,
		Mode: audio.Compact, PerRound: true, IncludeFull: true, IncludeIsolated: false,
	})
	if err != nil || len(atFiles) != 1 || atFiles[0].Kind != "alltalk" {
		t.Fatalf("alltalk export = %+v, err=%v", atFiles, err)
	}
	atTxt := Build(Input{Match: m, Reported: rep, Files: atFiles})
	if !strings.Contains(atTxt, "All-talk (both teams could hear each other)") || !strings.Contains(atTxt, "Also audible in the clips: Opponent.") {
		t.Errorf("alltalk report mismatch:\n%s", atTxt)
	}

	// 5. Team file where the reported player does not speak.
	silentFiles, err := audio.PlanExport(m, audio.ExportOptions{
		Periods: []int{1}, AudioPlayers: []uint64{mate}, Reported: rep,
		Mode: audio.Compact, PerRound: true, IncludeFull: true, IncludeIsolated: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	silentTxt := Build(Input{Match: m, Reported: rep, Files: silentFiles})
	if !strings.Contains(silentTxt, "(the reported player(s) don't speak in this file)") {
		t.Errorf("missing silent reported notice:\n%s", silentTxt)
	}
}

func TestBuildSyntheticMarksHeaderAndLimits(t *testing.T) {
	m := syntheticReportMatch()
	toxic := m.Players[0].SteamID

	// Empty Reported + empty Files.
	emptyTxt := Build(Input{Match: m})
	for _, want := range []string{
		"Reported player(s): (select the reported player(s))",
		"Reason: Toxic voice communication",
		"Match: https://www.faceit.com/en/cs2/room/1-abc12345-1234-1234-1234-1234567890ab",
		"Map: de_mirage — final score TeamA 1 - 1 TeamB",
		"Demo: faceit_match.dem.zst (CS2 game version 14201)",
		"Evidence: (nothing selected to export)",
	} {
		if !strings.Contains(emptyTxt, want) {
			t.Errorf("empty report missing %q:\n%s", want, emptyTxt)
		}
	}

	// Marks (including out-of-order and one in cut silence) + >10 stretches truncation.
	var speeches []audio.Speech
	for i := 0; i < 12; i++ {
		start := float64(i * 4)
		speeches = append(speeches, audio.Speech{
			Player:    toxic,
			Period:    1,
			DemoStart: 12.0 + start,
			Tick:      768 + i*256,
			Duration:  1.0,
			FileStart: start,
		})
	}
	file := audio.ExportedFile{
		Name:     "TeamA_R01.mp3",
		Kind:     "team",
		Team:     "TeamA",
		Periods:  []int{1},
		Mode:     audio.Full,
		Duration: 50.0,
		Speech:   speeches,
		Segments: []audio.Segment{{Period: 1, Out: 0, Demo: 10.0, Len: int(40 * audio.SR)}},
	}

	txt := Build(Input{
		Match:    m,
		Reported: []uint64{toxic},
		Files:    []audio.ExportedFile{file},
		Reason:   "Verbal abuse / insults",
		Marks: []demo.Span{
			{Start: 70.0, End: 72.0}, // outside R01 segments
			{Start: 12.0, End: 13.0}, // inside first speech
		},
	})
	for _, want := range []string{
		"- ToxicPlayer (SteamID64: 76561198000001001, https://steamcommunity.com/profiles/76561198000001001)",
		"Reason: Verbal abuse / insults",
		"#1 TeamA_R01.mp3 at 0:02-0:03",
		"Round 1 (demo 0:12-0:13, tick 768) - speaking: ToxicPlayer",
		"#2 (not in the selected files)",
		"Marked: #1 at 0:02-0:03",
		"… (+2 more)",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("marks/limits report missing %q:\n%s", want, txt)
		}
	}
}
