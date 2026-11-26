package demo

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	dem "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/msg"
)

var faceitIDRe = regexp.MustCompile(`1-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// breakSpan is a halftime-like pause where all players can talk to each other.
type breakSpan struct {
	afterRound     int
	start, end     float64
	startTk, endTk int
}

// Parse reads a demo file (.dem or .dem.zst).
func Parse(path string, progress Progress) (*Match, error) {
	r, err := openDemo(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return parseRaw(r, path, progress)
}

// ParseReader reads a demo (.dem or .dem.zst) from src, for callers
// without a file path, such as the browser build. name is the file name.
func ParseReader(src io.Reader, name string, progress Progress) (*Match, error) {
	r, err := decompress(src, name)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return parseRaw(r, name, progress)
}

// parseRaw parses the uncompressed demo bytes in r.
func parseRaw(r io.Reader, path string, progress Progress) (*Match, error) {
	m := &Match{
		File:          path,
		FaceitMatchID: faceitIDRe.FindString(strings.ToLower(filepath.Base(path))),
		Voice:         map[uint64]*VoiceTrack{},
	}

	cfg := dem.DefaultParserConfig
	p := dem.NewParserWithConfig(r, cfg)
	defer p.Close()

	b := &builder{m: m, p: p, players: map[uint64]*Player{}, steam: map[uint64]*steamState{}}
	b.rounds = []Round{{Number: 0, Sides: map[uint64]string{}}} // warmup
	b.register()

	err := p.ParseToEnd()
	switch {
	case errors.Is(err, dem.ErrUnexpectedEndOfDemo):
		m.Warnings = append(m.Warnings, "the demo ends unexpectedly (truncated or corrupt); everything up to that point was read")
	case err != nil:
		return nil, fmt.Errorf("failed to read demo: %w", err)
	}

	b.finish()
	return m, nil
}

type builder struct {
	m       *Match
	p       dem.Parser
	rounds  []Round
	pre     []Round // finished pre-match periods (warmup, knife round) before rounds[0]
	players map[uint64]*Player
	steam   map[uint64]*steamState
	inRound bool // the last round has started and not been replaced

	// Score when the first match round started (non-zero if the demo starts mid-match).
	baseCT, baseT int

	breaks  []breakSpan
	inBreak bool

	endStart   float64 // game phase "game ended": the match is over
	endStartTk int
}

func (b *builder) now() (float64, int) {
	return b.p.CurrentTime().Seconds(), b.p.GameState().IngameTick()
}

func (b *builder) last() *Round { return &b.rounds[len(b.rounds)-1] }

func (b *builder) player(id uint64) *Player {
	pl := b.players[id]
	if pl == nil {
		pl = &Player{SteamID: id}
		b.players[id] = pl
	}
	return pl
}

func (b *builder) register() {
	p := b.p

	p.RegisterNetMessageHandler(func(h *msg.CDemoFileHeader) {
		b.m.Map = h.GetMapName()
		b.m.Server = h.GetServerName()
		b.m.GameVersion = int(h.GetPatchVersion())
		b.m.BuildNum = int(h.GetBuildNum())
	})

	// begin_new_match: everything before it (warmup, knife, restarts) is pre-match.
	// Knife rounds keep their own period; the rest goes back into the warmup.
	p.RegisterEventHandler(func(events.MatchStart) {
		t, tick := b.now()
		for _, r := range b.rounds[1:] {
			if r.Winner != "" && r.Kind != KindKnife { // a finished round is being thrown away (restart)
				b.m.Warnings = append(b.m.Warnings, fmt.Sprintf("match restarted at %s; the rounds played before it count as warmup", clock(t)))
				break
			}
		}
		w := b.rounds[0]
		for _, r := range b.rounds[1:] {
			for id, side := range r.Sides {
				w.Sides[id] = side
			}
		}
		from, fromTk := w.Start, w.StartTick
		for _, r := range b.rounds[1:] {
			if r.Kind != KindKnife {
				continue
			}
			if r.Start-from > 0.5 {
				piece := w
				piece.Next, piece.NextTick = r.Start, r.StartTick
				piece.Start, piece.StartTick = from, fromTk
				b.pre = append(b.pre, piece)
			}
			if r.Next <= r.Start {
				r.Next, r.NextTick = t, tick
			}
			r.Number, r.Label = 0, "Knife"
			b.pre = append(b.pre, r)
			from, fromTk = r.Next, r.NextTick
			w.Label = "Warmup (after knife)"
		}
		w.Start, w.StartTick = from, fromTk
		w.Next, w.NextTick = t, tick
		b.rounds = []Round{w}
		b.inRound = false
		b.breaks, b.inBreak = nil, false
	})

	// Halftime and the breaks around overtime: the game phase is "half ended"
	// and all players can talk to each other.
	p.RegisterEventHandler(func(e events.GamePhaseChanged) {
		t, tick := b.now()
		if e.NewGamePhase == common.GamePhaseGameHalfEnded && !b.inBreak && b.inRound {
			b.breaks = append(b.breaks, breakSpan{afterRound: b.last().Number, start: t, startTk: tick})
			b.inBreak = true
		} else if e.NewGamePhase == common.GamePhaseGameEnded && b.inRound && b.endStart == 0 {
			b.endStart, b.endStartTk = t, tick
		}
		if e.OldGamePhase == common.GamePhaseGameHalfEnded && b.inBreak {
			br := &b.breaks[len(b.breaks)-1]
			br.end, br.endTk = t, tick
			b.inBreak = false
		}
	})

	p.RegisterEventHandler(func(events.RoundStart) {
		gs := p.GameState()
		t, tick := b.now()
		if gs.IsWarmupPeriod() {
			// warmup after the knife round: the knife round ends here
			if l := b.last(); b.inRound && l.Kind == KindKnife && l.Next <= l.Start {
				l.Next, l.NextTick = t, tick
			}
			return
		}
		n := gs.TotalRoundsPlayed() + 1

		// A round number we already have means the round was restarted or a backup
		// was loaded: drop the old attempts from that round on.
		if n < len(b.rounds) {
			b.m.Warnings = append(b.m.Warnings, fmt.Sprintf("round %d was played again at %s (restart or backup); the earlier attempt was dropped", n, clock(t)))
			b.rounds = b.rounds[:n]
		}
		prev := b.last()
		prev.Next, prev.NextTick = t, tick

		// Rounds missing in between (demo started late): leave a gap, numbers stay right.
		for len(b.rounds) < n {
			b.rounds = append(b.rounds, Round{Number: len(b.rounds), Start: t, Next: t, StartTick: tick, NextTick: tick, Sides: map[uint64]string{}})
		}
		if len(b.rounds) == 1 {
			b.baseCT, b.baseT = 0, 0
			if ct, tt := gs.TeamCounterTerrorists(), gs.TeamTerrorists(); ct != nil && tt != nil && n > 1 {
				b.baseCT, b.baseT = ct.Score(), tt.Score()
			}
		}
		b.rounds = append(b.rounds, Round{Number: n, Start: t, StartTick: tick, Sides: map[uint64]string{}})
		b.inRound = true
		b.snapshotTeams()
	})

	p.RegisterEventHandler(func(events.RoundFreezetimeEnd) {
		if !b.inRound {
			return
		}
		r := b.last()
		r.FreezeEnd, r.FreezeEndTick = b.now()
		b.snapshotTeams()
		if b.knifeOnly() {
			r.Kind = KindKnife
		}
	})

	p.RegisterEventHandler(func(e events.RoundEnd) {
		if !b.inRound {
			return
		}
		r := b.last()
		r.End, r.EndTick = b.now()
		switch e.Winner {
		case common.TeamCounterTerrorists:
			r.Winner = "CT"
		case common.TeamTerrorists:
			r.Winner = "T"
		}
		if e.WinnerState != nil {
			r.WinnerTeam = e.WinnerState.ClanName()
		}
		r.Reason = reasonText(e.Reason)
	})

	p.RegisterNetMessageHandler(func(v *msg.CSVCMsg_VoiceData) { b.voice(v) })

	p.RegisterNetMessageHandler(func(tm *msg.CUserMessageTextMsg) {
		t, tick := b.now()
		for _, param := range tm.GetParam() {
			if txt := cleanText(param); txt != "" {
				b.m.ServerMessages = append(b.m.ServerMessages, ServerMessage{Time: t, Tick: tick, Text: txt})
			}
		}
	})
}

// snapshotTeams records who plays on which side in the current round.
func (b *builder) snapshotTeams() {
	gs := b.p.GameState()
	r := b.last()
	if ct := gs.TeamCounterTerrorists(); ct != nil {
		r.TeamCT = ct.ClanName()
	}
	if t := gs.TeamTerrorists(); t != nil {
		r.TeamT = t.ClanName()
	}
	for _, pl := range gs.Participants().Playing() {
		if pl == nil || pl.SteamID64 == 0 || pl.IsBot {
			continue
		}
		info := b.player(pl.SteamID64)
		info.Name = pl.Name
		info.Playing = true
		switch pl.Team {
		case common.TeamCounterTerrorists:
			r.Sides[pl.SteamID64] = "CT"
			info.Team = r.TeamCT
		case common.TeamTerrorists:
			r.Sides[pl.SteamID64] = "T"
			info.Team = r.TeamT
		}
	}
}

// knifeOnly: every player holds only a knife (and maybe the bomb), as in the
// knife round that decides the sides. In a real round everyone has a pistol.
func (b *builder) knifeOnly() bool {
	n := 0
	for _, pl := range b.p.GameState().Participants().Playing() {
		if pl == nil || !pl.IsAlive() {
			continue
		}
		for _, w := range pl.Weapons() {
			if w == nil {
				continue
			}
			if w.Type != common.EqKnife && w.Type != common.EqBomb {
				return false
			}
		}
		n++
	}
	return n > 0
}

func (b *builder) voice(v *msg.CSVCMsg_VoiceData) {
	id := v.GetXuid()
	audio := v.GetAudio()
	if id == 0 || audio == nil || len(audio.GetVoiceData()) == 0 {
		return
	}
	t, tick := b.now()

	pl := b.player(id)
	if pl.Name == "" {
		if p := b.p.GameState().Participants().ByUserID(); p != nil {
			for _, x := range p {
				if x != nil && x.SteamID64 == id {
					pl.Name = x.Name
					break
				}
			}
		}
	}

	track := b.m.Voice[id]
	if track == nil {
		track = &VoiceTrack{SteamID: id}
		b.m.Voice[id] = track
	}

	data := append([]byte(nil), audio.GetVoiceData()...)
	switch audio.GetFormat() {
	case msg.VoiceDataFormatT_VOICEDATA_FORMAT_OPUS:
		b.m.VoiceFormat = "opus"
		for _, pkt := range splitOpus(data, audio.GetPacketOffsets()) {
			if n := PacketSamples(pkt); n > 0 {
				track.Packets = append(track.Packets, Packet{Time: t, Tick: tick, Data: pkt, Samples: n})
			}
		}

	case msg.VoiceDataFormatT_VOICEDATA_FORMAT_STEAM:
		b.m.VoiceFormat = "steam"
		st := b.steam[id]
		if st == nil {
			st = &steamState{}
			b.steam[id] = st
		}
		frames, _ := st.frames(data)
		for _, f := range frames {
			for i := 0; i < f.lost; i++ {
				track.Packets = append(track.Packets, Packet{Time: t, Tick: tick, Samples: 960})
			}
			if n := PacketSamples(f.data); n > 0 {
				track.Packets = append(track.Packets, Packet{Time: t, Tick: tick, Data: f.data, Samples: n})
			}
		}

	default:
		if !strings.Contains(strings.Join(b.m.Warnings, "\n"), "unsupported voice format") {
			b.m.Warnings = append(b.m.Warnings, "unsupported voice format: "+audio.GetFormat().String())
		}
	}
}

func (b *builder) finish() {
	m := b.m
	m.Duration = b.p.CurrentTime().Seconds()
	m.EndTick = b.p.GameState().IngameTick()
	m.TickRate = b.p.TickRate()

	teamA := computeScores(b.rounds, b.baseCT, b.baseT)
	for _, pl := range b.players {
		if pl.Playing && pl.Team == "" {
			pl.Team = "Team B"
			if teamA[pl.SteamID] {
				pl.Team = "Team A"
			}
		}
	}
	last := b.last()
	if last.Next == 0 || last.Next < last.Start {
		last.Next, last.NextTick = m.Duration, m.EndTick
	}
	if len(b.rounds) == 1 { // no match rounds at all: warmup covers everything
		b.rounds[0].Next, b.rounds[0].NextTick = m.Duration, m.EndTick
	}
	m.base = b.periods()
	m.Rounds = cloneRounds(m.base)
	m.Platform = PlatformOther
	if m.FaceitMatchID != "" || strings.Contains(strings.ToLower(m.Server), "faceit") {
		m.Platform = PlatformFACEIT
	}

	// Players: everyone who played plus everyone who talked.
	for id, pl := range b.players {
		if !pl.Playing && m.Voice[id] == nil {
			continue
		}
		if pl.Name == "" {
			pl.Name = fmt.Sprintf("%d", id)
		}
		m.Players = append(m.Players, *pl)
	}
	sort.Slice(m.Players, func(i, j int) bool {
		a, c := m.Players[i], m.Players[j]
		if a.Playing != c.Playing {
			return a.Playing
		}
		if a.Team != c.Team {
			return a.Team < c.Team
		}
		return strings.ToLower(a.Name) < strings.ToLower(c.Name)
	})

	for _, tr := range m.Voice {
		tr.Utterances = buildUtterances(tr.Packets, m.Rounds)
	}
}
