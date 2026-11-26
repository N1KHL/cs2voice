// Package demo reads a CS2 demo and extracts what the app needs:
// match info, rounds, players and the players' voice packets.
package demo

const (
	KindWarmup   = "warmup"
	KindKnife    = "knife" // knife round before the match (picks the sides); team only
	KindRound    = "round"
	KindBreak    = "break"     // halftime and the breaks around overtime
	KindMatchEnd = "match_end" // after the last round, until the demo ends
)

const (
	PlatformFACEIT = "faceit"
	PlatformOther  = "other"
)

// Progress is called from time to time with a value between 0 and 1.
type Progress func(fraction float64)

// Match is everything read from one demo.
type Match struct {
	File          string
	FaceitMatchID string // from the file name, when it's a FACEIT demo
	Map           string
	Server        string
	GameVersion   int // CS2 network protocol / patch version, e.g. 14201
	BuildNum      int
	Platform      string // PlatformFACEIT or PlatformOther
	RulesID       string // ruleset applied (package rules)
	TickRate      float64
	Duration      float64 // seconds
	EndTick       int

	// Rounds lists every period in order: Rounds[0] is the warmup / pre-match,
	// then the rounds, with the breaks (halftime, overtime halftimes) between
	// them. A period's ID is its index in this slice.
	Rounds      []Round
	Players     []Player
	Voice       map[uint64]*VoiceTrack
	VoiceFormat string // "opus", "steam" or "" when there is no voice
	Warnings    []string

	// ServerMessages are the server's chat notices (e.g. "[FACEIT^] Alltalk enabled.").
	ServerMessages []ServerMessage
	// AllTalkSource tells how the all-talk breaks were found: "chat" (server
	// notices), "phase" (game phase + a duration) or "none". Set by ApplyAllTalk.
	AllTalkSource string

	base []Round // periods as parsed, before ApplyAllTalk
}

type ServerMessage struct {
	Time float64
	Tick int
	Text string // control characters removed
}

// Span is a stretch of the demo (e.g. an all-talk break or a marked moment).
type Span struct {
	Start, End         float64
	StartTick, EndTick int
}

type Round struct {
	ID     int    // index in Match.Rounds; selections and utterances refer to it
	Kind   string // KindWarmup, KindKnife, KindRound, KindBreak or KindMatchEnd
	Number int    // round number; for a break, the round just before it; 0 = warmup
	Label  string // "Warmup", "R05", "Halftime", "OT halftime", ...

	// AllTalk: both teams hear each other. Set by rules.Apply from the
	// versioned rules (warmup, halftime/overtime breaks, match end).
	AllTalk bool

	// The round window is [Start, Next): from round start (freeze time included)
	// to the start of the next round, so the talk after the round end is included.
	Start, FreezeEnd, End, Next                 float64 // seconds since demo start
	StartTick, FreezeEndTick, EndTick, NextTick int

	Winner     string // "CT", "T" or "" (draw / not finished)
	WinnerTeam string // clan name of the winner
	Reason     string
	TeamCT     string // clan names during this round
	TeamT      string
	ScoreCT    int // score after this round
	ScoreT     int
	Sides      map[uint64]string // player -> "CT" / "T" in this round
}

type Player struct {
	SteamID uint64
	Name    string
	Team    string // clan name of the team the player last played for ("" = never played)
	Playing bool   // false for spectators, casters and coaches
}

type VoiceTrack struct {
	SteamID    uint64
	Packets    []Packet
	Utterances []Utterance
}

// Packet is one Opus packet as it arrived in the demo.
type Packet struct {
	Time    float64 // seconds since demo start
	Tick    int
	Data    []byte // nil = lost frame (play as silence)
	Samples int    // duration at 48 kHz
}

// Utterance is a run of packets without a pause longer than utterancePause.
type Utterance struct {
	Start, End  float64
	StartTick   int
	First, Last int // packet indexes, inclusive
	Period      int // ID of the period (index in Match.Rounds) it starts in
}

// TeamOf returns the player's team, or "" for spectators, casters and coaches.
// Each team only hears its own players, so audio is always built per team.
func (m *Match) TeamOf(id uint64) string {
	for _, p := range m.Players {
		if p.SteamID == id {
			return p.Team
		}
	}
	return ""
}

// SplitByTeam groups players by team, keeping the order of m.Players
// (teams first in that order, "" = spectators/coaches last).
func (m *Match) SplitByTeam(ids []uint64) (teams []string, byTeam map[string][]uint64) {
	want := map[uint64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	byTeam = map[string][]uint64{}
	for _, p := range m.Players {
		if !want[p.SteamID] {
			continue
		}
		if _, ok := byTeam[p.Team]; !ok {
			teams = append(teams, p.Team)
		}
		byTeam[p.Team] = append(byTeam[p.Team], p.SteamID)
	}
	return teams, byTeam
}

// PeriodAt returns the ID of the period (round, break, warmup) that contains t.
func PeriodAt(rounds []Round, t float64) int {
	for i := len(rounds) - 1; i >= 0; i-- {
		if t >= rounds[i].Start {
			return rounds[i].ID
		}
	}
	return 0
}
