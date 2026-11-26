// Package rules: who hears whom, from a small manually updated list in
// rules.json. Per platform and CS2 game version it says whether the server may
// announce all-talk in the chat ("[FACEIT^] Alltalk enabled." / "disabled.").
// If it may, the notices found in the demo are the all-talk times; if none are
// found, or the version says no, or we don't know: no all-talk during the match.
package rules

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"

	"cs2voice/internal/demo"
)

//go:embed rules.json
var embedded []byte

type Entry struct {
	FromGameVersion int      `json:"from_game_version"`
	AllTalkNotices  bool     `json:"alltalk_notices"`
	OnText          []string `json:"on_text"`
	OffText         []string `json:"off_text"`
	WarmupAllTalk   bool     `json:"warmup_all_talk"`
}

type file struct {
	Platforms map[string][]Entry `json:"platforms"`
}

func parse(b []byte) (map[string][]Entry, error) {
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f.Platforms, nil
}

// Load returns the rules embedded from rules.json.
func Load() map[string][]Entry {
	list, err := parse(embedded)
	if err != nil {
		panic("embedded rules.json: " + err.Error())
	}
	return list
}

// Select returns the entry for this platform and game version. Unknown
// platform or a version before every entry: nothing (= no all-talk).
func Select(list map[string][]Entry, platform string, gameVersion int) (Entry, bool) {
	entries := append([]Entry(nil), list[platform]...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].FromGameVersion < entries[j].FromGameVersion })
	var best Entry
	ok := false
	for _, e := range entries {
		if gameVersion == 0 || e.FromGameVersion <= gameVersion {
			best, ok = e, true
		}
	}
	return best, ok
}

// Apply sets the all-talk periods of the match and returns the spans found.
func Apply(m *demo.Match) []demo.Span {
	e, _ := Select(Load(), m.Platform, m.GameVersion) // not found: zero Entry = no all-talk
	var spans []demo.Span
	source := "none"
	if e.AllTalkNotices {
		spans = ChatSpans(m, e.OnText, e.OffText)
		source = "chat"
	}
	m.ApplyAllTalk(spans, e.WarmupAllTalk, false, source)
	return spans
}

// ChatSpans finds the all-talk spans announced in the server chat. An "on"
// without a matching "off" lasts until the end of the demo.
func ChatSpans(m *demo.Match, on, off []string) []demo.Span {
	var out []demo.Span
	open := false
	for _, msg := range m.ServerMessages {
		switch {
		case !open && containsAny(msg.Text, on):
			out = append(out, demo.Span{Start: msg.Time, StartTick: msg.Tick})
			open = true
		case open && containsAny(msg.Text, off):
			sp := &out[len(out)-1]
			sp.End, sp.EndTick = msg.Time, msg.Tick
			open = false
		}
	}
	if open {
		sp := &out[len(out)-1]
		sp.End, sp.EndTick = m.Duration, m.EndTick
	}
	kept := out[:0]
	for _, sp := range out {
		if sp.End > sp.Start {
			kept = append(kept, sp)
		}
	}
	return kept
}

func containsAny(text string, subs []string) bool {
	t := strings.ToLower(text)
	for _, s := range subs {
		if s != "" && strings.Contains(t, strings.ToLower(s)) {
			return true
		}
	}
	return false
}
