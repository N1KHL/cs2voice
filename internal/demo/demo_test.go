package demo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"math"
	"os"
	"reflect"
	"testing"
)

func TestPacketSamples(t *testing.T) {
	tests := []struct {
		name string
		pkt  []byte
		want int
	}{
		{"empty", nil, 0},
		{"silk_10ms_1frame", []byte{0 << 3}, 480},
		{"silk_20ms_1frame", []byte{1 << 3}, 960},
		{"silk_40ms_1frame", []byte{2 << 3}, 1920},
		{"silk_60ms_1frame", []byte{3 << 3}, 2880},
		{"hybrid_10ms_1frame", []byte{12 << 3}, 480},
		{"hybrid_20ms_1frame", []byte{13 << 3}, 960},
		{"celt_2_5ms_1frame", []byte{16 << 3}, 120},
		{"celt_5ms_1frame", []byte{17 << 3}, 240},
		{"celt_10ms_1frame", []byte{18 << 3}, 480},
		{"celt_20ms_1frame", []byte{19 << 3}, 960},
		{"code1_2frames_celt2_5ms", []byte{(16 << 3) | 1}, 240},
		{"code2_2frames_celt20ms", []byte{(19 << 3) | 2}, 1920},
		{"code3_missing_count_byte", []byte{(19 << 3) | 3}, 0},
		{"code3_3frames_celt20ms", []byte{(19 << 3) | 3, 3}, 2880},
		{"code3_zero_frames", []byte{(19 << 3) | 3, 0}, 0},
		{"code3_exceeds_120ms", []byte{(3 << 3) | 3, 3}, 0}, // 2880 * 3 = 8640 > 5760
	}
	for _, tc := range tests {
		if got := PacketSamples(tc.pkt); got != tc.want {
			t.Errorf("%s: PacketSamples = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestSplitOpus(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	if got := splitOpus(data, nil); !reflect.DeepEqual(got, [][]byte{data}) {
		t.Fatalf("nil offsets: got %v", got)
	}
	if got := splitOpus(data, []uint32{8}); !reflect.DeepEqual(got, [][]byte{data}) {
		t.Fatalf("single offset: got %v", got)
	}

	want := [][]byte{{1, 2, 3}, {4, 5}, {6, 7, 8}}
	if got := splitOpus(data, []uint32{0, 3, 5}); !reflect.DeepEqual(got, want) {
		t.Fatalf("start offsets: got %v, want %v", got, want)
	}
	if got := splitOpus(data, []uint32{3, 5, 8}); !reflect.DeepEqual(got, want) {
		t.Fatalf("end offsets: got %v, want %v", got, want)
	}

	// Invalid bounds fall back to returning the whole payload.
	if got := splitOpus(data, []uint32{0, 3, 20}); !reflect.DeepEqual(got, [][]byte{data}) {
		t.Fatalf("out-of-range offsets: got %v", got)
	}
	if got := splitOpus(data, []uint32{3, 3, 8}); !reflect.DeepEqual(got, [][]byte{data}) {
		t.Fatalf("empty slice offset: got %v", got)
	}
}

func buildSteamPacket(voiceType byte, payload []byte) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint64(76561198000000001))
	buf.WriteByte(0x0B)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(48000))
	buf.WriteByte(voiceType)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(len(payload)))
	buf.Write(payload)
	crc := crc32.ChecksumIEEE(buf.Bytes())
	_ = binary.Write(&buf, binary.LittleEndian, crc)
	return buf.Bytes()
}

func appendSteamChunk(buf *bytes.Buffer, seq uint16, data []byte) {
	_ = binary.Write(buf, binary.LittleEndian, int16(len(data)))
	_ = binary.Write(buf, binary.LittleEndian, seq)
	buf.Write(data)
}

func TestSteamStateFrames(t *testing.T) {
	var st steamState

	if _, err := st.frames(make([]byte, 10)); !errors.Is(err, errBadSteamPacket) {
		t.Fatalf("short packet: err = %v, want errBadSteamPacket", err)
	}

	badCRC := buildSteamPacket(0x00, nil)
	badCRC[len(badCRC)-1] ^= 0xFF
	if _, err := st.frames(badCRC); !errors.Is(err, errBadSteamPacket) {
		t.Fatalf("bad CRC: err = %v, want errBadSteamPacket", err)
	}

	silence := buildSteamPacket(0x00, nil)
	if frames, err := st.frames(silence); err != nil || len(frames) != 0 {
		t.Fatalf("silence packet: frames=%v err=%v, want nil, nil", frames, err)
	}

	badType := buildSteamPacket(0x05, []byte{1, 2})
	if _, err := st.frames(badType); !errors.Is(err, errBadSteamPacket) {
		t.Fatalf("bad voiceType: err = %v, want errBadSteamPacket", err)
	}

	// Sequential chunks 10 and 11.
	var p1 bytes.Buffer
	appendSteamChunk(&p1, 10, []byte{0x98, 0x01, 0x02})
	appendSteamChunk(&p1, 11, []byte{0x98, 0x03})
	frames, err := st.frames(buildSteamPacket(0x06, p1.Bytes()))
	if err != nil || len(frames) != 2 {
		t.Fatalf("packet 1: frames=%v err=%v", frames, err)
	}
	if frames[0].lost != 0 || frames[1].lost != 0 || !bytes.Equal(frames[0].data, []byte{0x98, 0x01, 0x02}) {
		t.Fatalf("unexpected frames in packet 1: %+v", frames)
	}

	// Gap (expected 12, got 14 -> lost=2), duplicate/old seq 13 skipped, then seq 15.
	var p2 bytes.Buffer
	appendSteamChunk(&p2, 14, []byte{0x98, 0x04})
	appendSteamChunk(&p2, 13, []byte{0x98, 0x99})
	appendSteamChunk(&p2, 15, []byte{0x98, 0x05})
	frames, err = st.frames(buildSteamPacket(0x06, p2.Bytes()))
	if err != nil || len(frames) != 2 {
		t.Fatalf("packet 2: frames=%+v err=%v", frames, err)
	}
	if frames[0].lost != 2 || !bytes.Equal(frames[0].data, []byte{0x98, 0x04}) {
		t.Fatalf("frame 0: got %+v, want lost=2", frames[0])
	}
	if frames[1].lost != 0 || !bytes.Equal(frames[1].data, []byte{0x98, 0x05}) {
		t.Fatalf("frame 1: got %+v, want lost=0", frames[1])
	}

	// Large gap is capped at 10 lost frames, and chunkLen == -1 resets synced state.
	var p3 bytes.Buffer
	appendSteamChunk(&p3, 50, []byte{0x98, 0x06})
	_ = binary.Write(&p3, binary.LittleEndian, int16(-1))
	frames, err = st.frames(buildSteamPacket(0x06, p3.Bytes()))
	if err != nil || len(frames) != 1 || frames[0].lost != 10 {
		t.Fatalf("packet 3: frames=%+v err=%v, want 1 frame with lost=10", frames, err)
	}
	if st.synced {
		t.Fatalf("expected synced=false after end-of-stream marker")
	}

	var p4 bytes.Buffer
	appendSteamChunk(&p4, 1, []byte{0x98, 0x07})
	frames, err = st.frames(buildSteamPacket(0x06, p4.Bytes()))
	if err != nil || len(frames) != 1 || frames[0].lost != 0 {
		t.Fatalf("packet 4 after reset: frames=%+v err=%v", frames, err)
	}
}

func TestPeriodAt(t *testing.T) {
	rounds := []Round{
		{ID: 0, Kind: KindWarmup, Start: 0, Next: 10},
		{ID: 1, Kind: KindRound, Number: 1, Start: 10, Next: 40},
		{ID: 2, Kind: KindBreak, Number: 12, Start: 40, Next: 55},
		{ID: 3, Kind: KindRound, Number: 13, Start: 55, Next: 85},
	}
	cases := []struct {
		sec  float64
		want int
	}{
		{-1, 0},
		{0, 0},
		{9.99, 0},
		{10.0, 1},
		{40.0, 2},
		{54.5, 2},
		{55.0, 3},
		{200.0, 3},
	}
	for _, tc := range cases {
		if got := PeriodAt(rounds, tc.sec); got != tc.want {
			t.Errorf("PeriodAt(%v) = %d, want %d", tc.sec, got, tc.want)
		}
	}
}

func TestBuildUtterances(t *testing.T) {
	rounds := []Round{
		{ID: 0, Kind: KindWarmup, Start: 0, Next: 10},
		{ID: 1, Kind: KindRound, Number: 1, Start: 10, Next: 40},
		{ID: 2, Kind: KindRound, Number: 2, Start: 40, Next: 80},
	}
	pkts := []Packet{
		{Time: 12.00, Tick: 100, Samples: 960}, // 0.02s -> ends 12.02
		{Time: 12.20, Tick: 112, Samples: 960}, // gap 0.18 <= 0.35 -> same utterance
		{Time: 12.50, Tick: 130, Samples: 960}, // gap 0.28 <= 0.35 -> same utterance
		{Time: 41.00, Tick: 500, Samples: 960}, // gap > 0.35 -> new utterance in period 2
	}
	got := buildUtterances(pkts, rounds)
	if len(got) != 2 {
		t.Fatalf("len(Utterances) = %d, want 2: %+v", len(got), got)
	}
	if got[0].First != 0 || got[0].Last != 2 || got[0].StartTick != 100 || got[0].Period != 1 {
		t.Fatalf("utterance 0 = %+v", got[0])
	}
	if math.Abs(got[0].Start-12.0) > 1e-6 || math.Abs(got[0].End-12.52) > 1e-6 {
		t.Fatalf("utterance 0 span = [%.4f, %.4f], want [12.0, 12.52]", got[0].Start, got[0].End)
	}
	if got[1].First != 3 || got[1].Last != 3 || got[1].StartTick != 500 || got[1].Period != 2 {
		t.Fatalf("utterance 1 = %+v", got[1])
	}
}

func TestComputeScores(t *testing.T) {
	rounds := []Round{
		{Number: 0, Kind: KindWarmup, Sides: map[uint64]string{1: "CT", 2: "T"}},
		{Number: 0, Kind: KindKnife, Winner: "CT", Sides: map[uint64]string{1: "CT", 2: "T"}},
		{Number: 1, Winner: "CT", Sides: map[uint64]string{1: "CT", 2: "CT", 3: "T", 4: "T"}},
		// Halftime side swap: 1&2 move to T, 3&4 move to CT.
		{Number: 13, Winner: "T", Sides: map[uint64]string{1: "T", 2: "T", 3: "CT", 4: "CT"}},
		{Number: 14, Winner: "CT", Sides: map[uint64]string{1: "T", 2: "T", 3: "CT", 4: "CT"}},
	}

	teamA := computeScores(rounds, 3, 2)
	if !teamA[1] || !teamA[2] || teamA[3] || teamA[4] {
		t.Fatalf("unexpected teamA membership: %+v", teamA)
	}

	// Warmup and Knife must stay untouched at 0-0.
	if rounds[0].ScoreCT != 0 || rounds[1].ScoreCT != 0 {
		t.Fatalf("warmup/knife scores modified: %+v %+v", rounds[0], rounds[1])
	}
	// R01: CT is Team A (base 3+1=4), T is Team B (base 2).
	if rounds[2].ScoreCT != 4 || rounds[2].ScoreT != 2 || rounds[2].TeamCT != "Team A" || rounds[2].TeamT != "Team B" || rounds[2].WinnerTeam != "Team A" {
		t.Fatalf("R01 = %+v", rounds[2])
	}
	// R13: Team A swapped to T and won -> CT(B)=2, T(A)=5.
	if rounds[3].ScoreCT != 2 || rounds[3].ScoreT != 5 || rounds[3].TeamCT != "Team B" || rounds[3].TeamT != "Team A" || rounds[3].WinnerTeam != "Team A" {
		t.Fatalf("R13 = %+v", rounds[3])
	}
	// R14: Team B on CT won -> CT(B)=3, T(A)=5.
	if rounds[4].ScoreCT != 3 || rounds[4].ScoreT != 5 || rounds[4].WinnerTeam != "Team B" {
		t.Fatalf("R14 = %+v", rounds[4])
	}

	// Existing clan names are preserved.
	named := []Round{
		{Number: 1, Winner: "T", TeamCT: "Alphas", TeamT: "Betas", WinnerTeam: "Betas", Sides: map[uint64]string{1: "CT", 2: "T"}},
	}
	computeScores(named, 0, 0)
	if named[0].TeamCT != "Alphas" || named[0].TeamT != "Betas" || named[0].ScoreCT != 0 || named[0].ScoreT != 1 {
		t.Fatalf("named round = %+v", named[0])
	}
}

func TestApplyAllTalk(t *testing.T) {
	baseMatch := func() *Match {
		return &Match{
			Duration: 400,
			EndTick:  25600,
			Rounds: []Round{
				{ID: 0, Kind: KindWarmup, Label: "Warmup", Start: 0, Next: 100},
				{ID: 1, Kind: KindRound, Number: 12, Label: "R12", Start: 100, Next: 200},
				{ID: 2, Kind: KindBreak, Number: 12, Label: "Halftime", Start: 200, Next: 208},
				{ID: 3, Kind: KindRound, Number: 13, Label: "R13", Start: 208, Next: 300},
				{ID: 4, Kind: KindMatchEnd, Label: "Match end", Start: 300, Next: 400},
			},
			Voice: map[uint64]*VoiceTrack{
				1: {Utterances: []Utterance{{Start: 50}, {Start: 205}, {Start: 212}, {Start: 250}, {Start: 350}}},
			},
		}
	}

	// Without spans: phase break returns to R12, no AllTalk periods.
	m := baseMatch()
	m.ApplyAllTalk(nil, false, false, "none")
	if m.AllTalkSource != "none" || len(m.Rounds) != 4 {
		t.Fatalf("no-span ApplyAllTalk: source=%q len=%d", m.AllTalkSource, len(m.Rounds))
	}
	if m.Rounds[1].Next != 208 || m.Rounds[1].AllTalk {
		t.Fatalf("R12 after removing phase break = %+v", m.Rounds[1])
	}

	// Re-applying with a halftime span [200, 215) works cleanly from m.base.
	m.ApplyAllTalk([]Span{{Start: 200, End: 215}}, false, false, "chat")
	if m.AllTalkSource != "chat" || len(m.Rounds) != 5 {
		t.Fatalf("halftime ApplyAllTalk: source=%q len=%d", m.AllTalkSource, len(m.Rounds))
	}
	ht := m.Rounds[2]
	if ht.Kind != KindBreak || ht.Label != "Halftime" || !ht.AllTalk || ht.Start != 200 || ht.Next != 215 {
		t.Fatalf("halftime period = %+v", ht)
	}
	if m.Rounds[3].Start != 215 || m.Rounds[3].Next != 300 {
		t.Fatalf("R13 after halftime = %+v", m.Rounds[3])
	}
	u := m.Voice[1].Utterances
	if u[0].Period != 0 || u[1].Period != 2 || u[2].Period != 2 || u[3].Period != 3 || u[4].Period != 4 {
		t.Fatalf("utterance periods = %+v", u)
	}

	// Mid-round all-talk span + warmup/matchEnd flags.
	m.ApplyAllTalk([]Span{{Start: 240, End: 260}}, true, true, "chat")
	wantLabels := []string{"Warmup", "R12", "R13", "All talk (during R13)", "R13 (cont.)", "Match end"}
	if len(m.Rounds) != len(wantLabels) {
		t.Fatalf("mid-round len = %d, want %d: %+v", len(m.Rounds), len(wantLabels), m.Rounds)
	}
	for i, w := range wantLabels {
		if m.Rounds[i].Label != w || m.Rounds[i].ID != i {
			t.Errorf("round %d = (ID=%d, Label=%q), want (%d, %q)", i, m.Rounds[i].ID, m.Rounds[i].Label, i, w)
		}
	}
	if !m.Rounds[0].AllTalk || !m.Rounds[3].AllTalk || !m.Rounds[5].AllTalk || m.Rounds[2].AllTalk || m.Rounds[4].AllTalk {
		t.Fatalf("unexpected AllTalk flags: %+v", m.Rounds)
	}
	if m.Voice[1].Utterances[3].Period != 3 { // 250 falls in "All talk (during R13)"
		t.Fatalf("utterance at 250 period = %d, want 3", m.Voice[1].Utterances[3].Period)
	}
}

func TestTeamOfAndSplitByTeam(t *testing.T) {
	m := &Match{
		Players: []Player{
			{SteamID: 1, Name: "Alice", Team: "Alpha", Playing: true},
			{SteamID: 2, Name: "Bob", Team: "Alpha", Playing: true},
			{SteamID: 3, Name: "Carol", Team: "Beta", Playing: true},
			{SteamID: 4, Name: "Spec", Team: "", Playing: false},
		},
	}
	if got := m.TeamOf(3); got != "Beta" {
		t.Fatalf("TeamOf(3) = %q, want Beta", got)
	}
	if got := m.TeamOf(999); got != "" {
		t.Fatalf("TeamOf(999) = %q, want empty", got)
	}
	teams, byTeam := m.SplitByTeam([]uint64{4, 3, 1})
	if !reflect.DeepEqual(teams, []string{"Alpha", "Beta", ""}) {
		t.Fatalf("teams = %v", teams)
	}
	if !reflect.DeepEqual(byTeam["Alpha"], []uint64{1}) || !reflect.DeepEqual(byTeam["Beta"], []uint64{3}) || !reflect.DeepEqual(byTeam[""], []uint64{4}) {
		t.Fatalf("byTeam = %v", byTeam)
	}
}

func TestParseDemoOptional(t *testing.T) {
	path := os.Getenv("CS2VOICE_TEST_DEMO")
	if path == "" {
		t.Skip("CS2VOICE_TEST_DEMO not set")
	}
	m, err := Parse(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Rounds) == 0 || len(m.Players) == 0 || m.Duration <= 0 || m.TickRate <= 0 {
		t.Fatalf("unexpected parsed match metadata: rounds=%d players=%d duration=%v tickrate=%v",
			len(m.Rounds), len(m.Players), m.Duration, m.TickRate)
	}
	if id := PeriodAt(m.Rounds, m.Duration/2); id < 0 || id >= len(m.Rounds) {
		t.Fatalf("PeriodAt midpoint = %d out of range [0,%d)", id, len(m.Rounds))
	}
}
