// demoinfo is a test tool: it reads a demo and prints what the app will see
// (rounds, players, voice), and can write a quick .ogg per player.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"cs2voice/internal/audio"
	"cs2voice/internal/demo"
	"cs2voice/internal/rules"
)

func main() {
	jsonOut := flag.String("json", "", "write the parsed match (without audio data) to this JSON file")
	oggDir := flag.String("ogg", "", "write one compact .ogg per player into this folder")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: demoinfo [-json out.json] [-ogg dir] demo.dem[.zst]")
		os.Exit(2)
	}

	start := time.Now()
	m, err := demo.Parse(flag.Arg(0), nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("parsed in %s\n\n", time.Since(start).Round(time.Millisecond))

	rules.Apply(m)
	fmt.Printf("platform: %s | game version: %d (build %d) | all-talk from: %s\n",
		m.Platform, m.GameVersion, m.BuildNum, m.AllTalkSource)

	fmt.Printf("map: %s | server: %s | faceit: %s\n", m.Map, m.Server, m.FaceitMatchID)
	fmt.Printf("duration: %s | tickrate: %.0f | end tick: %d | voice: %q\n\n", clock(m.Duration), m.TickRate, m.EndTick, m.VoiceFormat)

	names := map[uint64]string{}
	for _, p := range m.Players {
		names[p.SteamID] = p.Name
	}

	fmt.Println("rounds:")
	for _, r := range m.Rounds {
		talk := 0.0
		for _, tr := range m.Voice {
			for _, u := range tr.Utterances {
				if u.Period == r.ID {
					talk += u.End - u.Start
				}
			}
		}
		label := r.Label
		if r.AllTalk {
			label += " [ALL TALK]"
		}
		fmt.Printf("  %2d %-24s %s-%s (%5.1fs)  win=%-2s %-15s CT %-12s %2d - %-2d %-12s T  players=%d talk=%5.1fs\n",
			r.ID, label, clock(r.Start), clock(r.Next), r.Next-r.Start, r.Winner, r.Reason,
			r.TeamCT, r.ScoreCT, r.ScoreT, r.TeamT, len(r.Sides), talk)
	}

	fmt.Println("\nplayers:")
	for _, p := range m.Players {
		tr := m.Voice[p.SteamID]
		talk, utt, pk, lost := 0.0, 0, 0, 0
		if tr != nil {
			utt, pk = len(tr.Utterances), len(tr.Packets)
			for _, u := range tr.Utterances {
				talk += u.End - u.Start
			}
			for _, x := range tr.Packets {
				if x.Data == nil {
					lost++
				}
			}
		}
		fmt.Printf("  %-20s %17d  team=%-12s playing=%-5v talk=%6.1fs utterances=%4d packets=%6d lost=%d\n",
			p.Name, p.SteamID, p.Team, p.Playing, talk, utt, pk, lost)
	}

	if len(m.Warnings) > 0 {
		fmt.Println("\nwarnings:")
		for _, w := range m.Warnings {
			fmt.Println("  -", w)
		}
	}

	if *jsonOut != "" {
		type lite struct {
			*demo.Match
			Voice map[uint64][]demo.Utterance
		}
		l := lite{Match: m, Voice: map[uint64][]demo.Utterance{}}
		for id, tr := range m.Voice {
			l.Voice[id] = tr.Utterances
		}
		b, _ := json.MarshalIndent(l, "", "  ")
		if err := os.WriteFile(*jsonOut, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "json:", err)
		}
	}

	if *oggDir != "" {
		os.MkdirAll(*oggDir, 0o755)
		bad := regexp.MustCompile(`[\\/:*?"<>|]`)
		for id, tr := range m.Voice {
			name := bad.ReplaceAllString(names[id], "")
			path := filepath.Join(*oggDir, fmt.Sprintf("%s_%d.ogg", name, id))
			if err := audio.WriteOggCompact(path, tr.Packets, uint32(id)); err != nil {
				fmt.Fprintln(os.Stderr, "ogg:", err)
			}
		}
		fmt.Println("\nogg files written to", *oggDir)
	}
}

func clock(sec float64) string {
	s := int(sec)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
