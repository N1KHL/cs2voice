package audio

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"cs2voice/internal/demo"
)

func syntheticMatch() *demo.Match {
	pkts := []demo.Packet{
		{Samples: 960},
		{Samples: 960},
	}
	return &demo.Match{
		File:          "faceit_match.dem.zst",
		FaceitMatchID: "1-abc123",
		Map:           "de_mirage",
		GameVersion:   14201,
		TickRate:      64,
		VoiceFormat:   "opus",
		Players: []demo.Player{
			{SteamID: 1001, Name: "Alice", Team: "Team A", Playing: true},
			{SteamID: 1002, Name: "Bob", Team: "Team A", Playing: true},
			{SteamID: 2001, Name: "Eve", Team: "Team B", Playing: true},
		},
		Rounds: []demo.Round{
			{ID: 0, Kind: demo.KindRound, Number: 1, Label: "Round 1", Start: 10, Next: 40, TeamCT: "Team A", TeamT: "Team B", ScoreCT: 1, ScoreT: 0},
			{ID: 1, Kind: demo.KindBreak, Number: 12, Label: "Halftime", Start: 40, Next: 55, AllTalk: true},
		},
		Voice: map[uint64]*demo.VoiceTrack{
			1001: {
				SteamID: 1001,
				Packets: pkts,
				Utterances: []demo.Utterance{
					{Period: 0, Start: 12.0, End: 12.04, StartTick: 768, First: 0, Last: 1},
					{Period: 1, Start: 42.0, End: 42.04, StartTick: 2688, First: 0, Last: 1},
				},
			},
			1002: {
				SteamID: 1002,
				Packets: pkts,
				Utterances: []demo.Utterance{
					{Period: 0, Start: 15.0, End: 15.04, StartTick: 960, First: 0, Last: 1},
				},
			},
			2001: {
				SteamID: 2001,
				Packets: pkts,
				Utterances: []demo.Utterance{
					{Period: 0, Start: 13.0, End: 13.04, StartTick: 832, First: 0, Last: 1},
					{Period: 1, Start: 44.0, End: 44.04, StartTick: 2816, First: 0, Last: 1},
				},
			},
		},
	}
}

func TestExportToMemoryWAVAndZip(t *testing.T) {
	m := syntheticMatch()
	opts := ExportOptions{
		Prefix:          m.FaceitMatchID,
		Periods:         []int{0},
		AudioPlayers:    []uint64{1001, 1002},
		Reported:        []uint64{1001},
		Mode:            Compact,
		PerRound:        true,
		IncludeFull:     true,
		IncludeIsolated: true,
		Format:          WAV,
	}
	files, err := ExportToMemory(m, opts)
	if err != nil {
		t.Fatalf("ExportToMemory: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2 (team + isolated)", len(files))
	}
	if files[0].File.Name != "1-abc123_Team-A_R01.wav" {
		t.Errorf("file[0].Name = %q", files[0].File.Name)
	}
	if files[1].File.Name != "1-abc123_Alice_R01.wav" {
		t.Errorf("file[1].Name = %q", files[1].File.Name)
	}
	for _, f := range files {
		if len(f.Data) <= 44 || string(f.Data[:4]) != "RIFF" || string(f.Data[8:12]) != "WAVE" {
			t.Fatalf("invalid WAV header in %s (len=%d)", f.File.Name, len(f.Data))
		}
	}

	zipBytes, err := BuildZip(files, "Report body\n")
	if err != nil {
		t.Fatalf("BuildZip: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	got := map[string][]byte{}
	for _, zf := range zr.File {
		rc, err := zf.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		got[zf.Name] = b
	}
	if string(got["report.txt"]) != "Report body\n" {
		t.Errorf("report.txt = %q", string(got["report.txt"]))
	}
	if !bytes.Equal(got["1-abc123_Team-A_R01.wav"], files[0].Data) {
		t.Errorf("team WAV mismatch in zip")
	}
	if !bytes.Equal(got["1-abc123_Alice_R01.wav"], files[1].Data) {
		t.Errorf("isolated WAV mismatch in zip")
	}
}

func TestExportZipToMemoryRules(t *testing.T) {
	m := syntheticMatch()

	// No duplicate isolated file when team audio only has that reported player.
	files, zipBytes, err := ExportZipToMemory(m, ExportOptions{
		Periods:         []int{0},
		AudioPlayers:    []uint64{1001},
		Reported:        []uint64{1001},
		Mode:            Compact,
		PerRound:        true,
		IncludeFull:     true,
		IncludeIsolated: true,
		Format:          WAV,
	}, func(fs []ExportedFile) string {
		if len(fs) != 1 {
			t.Fatalf("expected 1 file when only reported player in AudioPlayers, got %d", len(fs))
		}
		return "single-player-report"
	})
	if err != nil {
		t.Fatalf("ExportZipToMemory: %v", err)
	}
	if len(files) != 1 || files[0].Kind != "team" {
		t.Fatalf("unexpected files: %+v", files)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("zip entries = %d, want 2 (1 wav + report.txt)", len(zr.File))
	}

	// All-talk period mixes both teams cleanly into one alltalk file.
	atFiles, atZip, err := ExportZipToMemory(m, ExportOptions{
		Periods:      []int{1},
		AudioPlayers: []uint64{1001, 2001},
		Mode:         Full,
		PerRound:     true,
		IncludeFull:  true,
		Format:       WAV,
	}, nil)
	if err != nil {
		t.Fatalf("all-talk ExportZipToMemory: %v", err)
	}
	if len(atFiles) != 1 || atFiles[0].Kind != "alltalk" || !strings.Contains(atFiles[0].Name, "alltalk_halftime") {
		t.Fatalf("unexpected all-talk files: %+v", atFiles)
	}
	if len(atZip) == 0 {
		t.Fatal("empty all-talk zip")
	}
}

func TestExportToMemoryMP3(t *testing.T) {
	m := syntheticMatch()
	files, err := ExportToMemory(m, ExportOptions{
		Periods:      []int{0},
		AudioPlayers: []uint64{1001, 1002},
		Mode:         Compact,
		PerRound:     true,
		IncludeFull:  true,
		Format:       MP3,
	})
	if err != nil {
		if strings.Contains(err.Error(), "codec unavailable") ||
			strings.Contains(err.Error(), "not available in browser") ||
			strings.Contains(err.Error(), "cs2opus") {
			t.Skipf("MP3 codec unavailable in test harness: %v", err)
		}
		t.Fatalf("ExportToMemory MP3: %v", err)
	}
	if len(files) != 1 || len(files[0].Data) < 4 {
		t.Fatalf("unexpected MP3 output: %d files", len(files))
	}
	if files[0].Data[0] != 0xff || (files[0].Data[1]&0xe0) != 0xe0 {
		t.Fatalf("expected MP3 frame sync, got %x %x", files[0].Data[0], files[0].Data[1])
	}
}
