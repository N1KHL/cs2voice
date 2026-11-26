package demo

import (
	"bytes"
	"encoding/gob"
	"errors"
	"os"
	"reflect"
	"testing"
)

func cacheMatch() *Match {
	m := &Match{
		File: "1-abc.dem.zst", FaceitMatchID: "1-abc", Map: "de_mirage", Server: "FACEIT",
		GameVersion: 14201, BuildNum: 9999, Platform: PlatformFACEIT,
		TickRate: 64, Duration: 120, EndTick: 7680,
		Players: []Player{
			{SteamID: 1, Name: "a", Team: "Alpha", Playing: true},
			{SteamID: 2, Name: "b", Team: "Bravo", Playing: true},
		},
		VoiceFormat: "opus",
		Warnings:    []string{"round 3 was played again"},
		ServerMessages: []ServerMessage{
			{Time: 50, Tick: 3200, Text: "[FACEIT^] Alltalk enabled."},
		},
		Voice: map[uint64]*VoiceTrack{
			1: {SteamID: 1, Packets: []Packet{
				{Time: 10, Tick: 640, Data: []byte{1, 2, 3}, Samples: 960},
				{Time: 10.02, Tick: 641, Samples: 960}, // lost frame
				{Time: 10.04, Tick: 642, Data: []byte{}, Samples: 0},
				{Time: 10.06, Tick: 643, Data: []byte{4}, Samples: 960},
			}, Utterances: []Utterance{{Start: 10, End: 10.08, StartTick: 640, First: 0, Last: 3, Period: 1}}},
			2: {SteamID: 2},
		},
	}
	m.base = []Round{
		{ID: 0, Kind: KindWarmup, Label: "Warmup", Start: 0, Next: 5},
		{ID: 1, Kind: KindRound, Number: 1, Label: "R01", Start: 5, Next: 120,
			Winner: "CT", TeamCT: "Alpha", TeamT: "Bravo", ScoreCT: 1,
			Sides: map[uint64]string{1: "CT", 2: "T"}},
	}
	m.Rounds = cloneRounds(m.base)
	return m
}

func TestMatchCacheRoundTrip(t *testing.T) {
	m := cacheMatch()
	data, err := m.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalMatch(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, m) {
		t.Fatalf("round trip changed the match:\n got %+v\nwant %+v", got, m)
	}
	pk := got.Voice[1].Packets
	if pk[1].Data != nil {
		t.Errorf("lost frame came back with data %v", pk[1].Data)
	}
	if pk[2].Data == nil {
		t.Errorf("empty packet came back as a lost frame")
	}
	// Packets share one buffer; appending to one must not touch the next.
	_ = append(pk[0].Data, 9)
	if pk[3].Data[0] != 4 {
		t.Errorf("append to packet 0 overwrote packet 3: %v", pk[3].Data)
	}
}

// The cache stores the periods as parsed, not the all-talk result, so a newer
// rules.json still applies to a cached match.
func TestMatchCacheKeepsParsedPeriods(t *testing.T) {
	m := cacheMatch()
	m.ApplyAllTalk([]Span{{Start: 50, End: 60}}, true, false, "chat")
	data, err := m.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalMatch(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.AllTalkSource != "" || len(got.Rounds) != 2 {
		t.Fatalf("cached match kept all-talk results: source=%q rounds=%d", got.AllTalkSource, len(got.Rounds))
	}
	got.ApplyAllTalk([]Span{{Start: 50, End: 60}}, true, false, "chat")
	if !reflect.DeepEqual(got.Rounds, m.Rounds) || !reflect.DeepEqual(got.Voice, m.Voice) {
		t.Fatalf("applying all-talk to the cached match gave another result")
	}
}

func TestMatchCacheRejectsOtherVersion(t *testing.T) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(CacheVersion + 1); err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalMatch(buf.Bytes()); !errors.Is(err, ErrCacheVersion) {
		t.Fatalf("other version: err = %v, want ErrCacheVersion", err)
	}
	if _, err := UnmarshalMatch([]byte("not a cached match")); err == nil || errors.Is(err, ErrCacheVersion) {
		t.Fatalf("garbage: err = %v, want a decode error", err)
	}
}

func TestMatchCacheRejectsTruncatedTrack(t *testing.T) {
	ct := packTrack(cacheMatch().Voice[1])
	ct.Data = ct.Data[:2]
	if _, err := unpackTrack(&ct); err == nil {
		t.Fatal("truncated track decoded without error")
	}
}

func TestMatchCacheRealDemo(t *testing.T) {
	path := os.Getenv("CS2VOICE_TEST_DEMO")
	if path == "" {
		t.Skip("CS2VOICE_TEST_DEMO not set")
	}
	m, err := Parse(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := m.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cached match: %.1f MB", float64(len(data))/(1<<20))
	got, err := UnmarshalMatch(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, m) {
		t.Fatal("round trip changed the parsed match")
	}
}
