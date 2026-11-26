package demo

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"sort"
)

// CacheVersion identifies what MarshalBinary stores. Bump it whenever the
// layout below or the parser's output changes, so a match cached by an older
// build is parsed again instead of being reused with stale periods or voice.
const CacheVersion = 1

// ErrCacheVersion is returned by UnmarshalMatch for data written by a build
// with another CacheVersion.
var ErrCacheVersion = errors.New("demo: cached match has another cache version")

// cachedMatch holds a Match as parsed. All-talk results (Rounds after
// ApplyAllTalk, AllTalkSource) are left out on purpose: they are rebuilt from
// the parsed periods and server messages, so a newer rules.json also applies
// to matches cached before it.
type cachedMatch struct {
	File           string
	FaceitMatchID  string
	Map            string
	Server         string
	GameVersion    int
	BuildNum       int
	Platform       string
	RulesID        string
	TickRate       float64
	Duration       float64
	EndTick        int
	Periods        []Round // m.base
	Players        []Player
	Tracks         []cachedTrack
	VoiceFormat    string
	Warnings       []string
	ServerMessages []ServerMessage
}

// cachedTrack stores the packets column by column: one gob value per field
// instead of one per packet keeps a full match small and fast to decode.
type cachedTrack struct {
	SteamID    uint64
	Times      []float64
	Ticks      []int
	Samples    []int
	Sizes      []int  // len(Packet.Data); -1 = lost frame (nil Data, played as silence)
	Data       []byte // all payloads back to back
	Utterances []Utterance
}

// MarshalBinary encodes the match as parsed, voice packets included, so it can
// be cached and opened again without the demo file.
func (m *Match) MarshalBinary() ([]byte, error) {
	cm := cachedMatch{
		File: m.File, FaceitMatchID: m.FaceitMatchID, Map: m.Map, Server: m.Server,
		GameVersion: m.GameVersion, BuildNum: m.BuildNum, Platform: m.Platform, RulesID: m.RulesID,
		TickRate: m.TickRate, Duration: m.Duration, EndTick: m.EndTick,
		Periods: m.parsed(), Players: m.Players, VoiceFormat: m.VoiceFormat,
		Warnings: m.Warnings, ServerMessages: m.ServerMessages,
	}
	ids := make([]uint64, 0, len(m.Voice))
	for id := range m.Voice {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		cm.Tracks = append(cm.Tracks, packTrack(m.Voice[id]))
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	// The version goes first, on its own, so another version is rejected
	// before gob tries to decode a layout it may not match.
	if err := enc.Encode(CacheVersion); err != nil {
		return nil, err
	}
	if err := enc.Encode(&cm); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalMatch decodes data written by MarshalBinary. The result is the
// match as parsed: apply the all-talk rules again before using it.
func UnmarshalMatch(data []byte) (*Match, error) {
	dec := gob.NewDecoder(bytes.NewReader(data))
	var version int
	if err := dec.Decode(&version); err != nil {
		return nil, fmt.Errorf("demo: reading cached match: %w", err)
	}
	if version != CacheVersion {
		return nil, ErrCacheVersion
	}
	var cm cachedMatch
	if err := dec.Decode(&cm); err != nil {
		return nil, fmt.Errorf("demo: reading cached match: %w", err)
	}

	m := &Match{
		File: cm.File, FaceitMatchID: cm.FaceitMatchID, Map: cm.Map, Server: cm.Server,
		GameVersion: cm.GameVersion, BuildNum: cm.BuildNum, Platform: cm.Platform, RulesID: cm.RulesID,
		TickRate: cm.TickRate, Duration: cm.Duration, EndTick: cm.EndTick,
		Players: cm.Players, VoiceFormat: cm.VoiceFormat,
		Warnings: cm.Warnings, ServerMessages: cm.ServerMessages,
		Voice: make(map[uint64]*VoiceTrack, len(cm.Tracks)),
		base:  cm.Periods,
	}
	m.Rounds = cloneRounds(m.base)
	for i := range cm.Tracks {
		tr, err := unpackTrack(&cm.Tracks[i])
		if err != nil {
			return nil, err
		}
		m.Voice[tr.SteamID] = tr
	}
	return m, nil
}

func packTrack(tr *VoiceTrack) cachedTrack {
	n := len(tr.Packets)
	ct := cachedTrack{
		SteamID:    tr.SteamID,
		Times:      make([]float64, n),
		Ticks:      make([]int, n),
		Samples:    make([]int, n),
		Sizes:      make([]int, n),
		Utterances: tr.Utterances,
	}
	size := 0
	for _, p := range tr.Packets {
		size += len(p.Data)
	}
	ct.Data = make([]byte, 0, size)
	for i, p := range tr.Packets {
		ct.Times[i], ct.Ticks[i], ct.Samples[i] = p.Time, p.Tick, p.Samples
		if p.Data == nil {
			ct.Sizes[i] = -1
			continue
		}
		ct.Sizes[i] = len(p.Data)
		ct.Data = append(ct.Data, p.Data...)
	}
	return ct
}

func unpackTrack(ct *cachedTrack) (*VoiceTrack, error) {
	n := len(ct.Sizes)
	if len(ct.Times) != n || len(ct.Ticks) != n || len(ct.Samples) != n {
		return nil, fmt.Errorf("demo: cached voice track %d is inconsistent", ct.SteamID)
	}
	tr := &VoiceTrack{SteamID: ct.SteamID, Utterances: ct.Utterances}
	if n > 0 {
		tr.Packets = make([]Packet, n)
	}
	off := 0
	for i, size := range ct.Sizes {
		p := Packet{Time: ct.Times[i], Tick: ct.Ticks[i], Samples: ct.Samples[i]}
		if size >= 0 {
			if off+size > len(ct.Data) {
				return nil, fmt.Errorf("demo: cached voice track %d is truncated", ct.SteamID)
			}
			// Full slice expression: an append to one packet must never
			// overwrite the next one in the shared buffer.
			p.Data = ct.Data[off : off+size : off+size]
			off += size
		}
		tr.Packets[i] = p
	}
	return tr, nil
}
