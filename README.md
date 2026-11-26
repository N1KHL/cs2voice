# CS2 Voice

Extract the players' voice from **Counter-Strike 2 FACEIT demos**, listen to it
round by round in your browser, and export audio clips plus a ready-to-paste
report for **FACEIT** (toxic voice, mic spam, threats...).

Everything runs **100% locally in your browser** via Go WebAssembly — your demo
never leaves your computer and nothing is uploaded. Live at **[toxic.report](https://toxic.report)**.

Opened demos are kept **in your browser only** for 7 days (the extracted voice,
not the demo file), with your marks and selections, so a reload or reopening a
recent demo is instant. You can delete them from the start screen at any time.

## What it does

- **Opens** `.dem` and FACEIT `.dem.zst` files (drag and drop, or click **Open demo**).
- **Keeps your work:** marks, reasons, cuts and report edits are saved as you go;
  reloading the page reopens the demo, and recent demos reopen from the start
  screen without the file.
- **Report tab:**
  - **Listen (left side):** pick rounds and players and hear only what was said.
    - *Talk only* cuts the silences; *Full round* keeps the round's real timing.
    - A colored waveform per player shows who is talking, and you can click to seek.
  - **Export (right side):** a separate selection with its own preview.
    - Choose the players whose voice goes in the audio and mark who you are **reporting**.
    - Export one file per round or one merged file, as MP3 (default) or WAV, downloaded as one `.zip` (default) or separate files.
    - Optionally add a file with only each reported player's voice.
  - **Report:** an English text for FACEIT that updates as you go.
    - Pick the reason with chips, or write your own.
    - Gives the match link, the players' SteamIDs, and where to listen in each file.
    - **Mark the toxic moments** on the preview lane (or press `M`) and they go into the report.
    - You can edit the generated text by hand.
- **Listen & Cut tab:**
  - Listen per team, round, and selected players, drag on the waveform/cut lane (or press `C`) to select one or more ranges, and export just those audio clips (as `.zip` or separate files, MP3 or WAV), with no report tools.
- **Teams are never mixed.** In CS2 each team only hears itself, so the audio is
  always what one team heard.
  - Both teams are mixed only during **all-talk**: halftime and the overtime breaks on FACEIT.
  - All-talk is detected from the FACEIT server's chat notices in the demo.
- **Exports are never volume-normalized,** because how loud someone is can be part of
  the evidence. "Even out volume" exists only for listening.
- **Knife round** and warmup are shown as their own periods.
- **Light and dark themes.** The interface is in English or Portuguese (Brazil),
  following your browser language. More languages can be added in `frontend/lang/`.

Valve Matchmaking demos have no voice. FACEIT and servers with `tv_relayvoice` record it.

## Using it

1. Open the website and drop a `.dem` or `.dem.zst` demo onto the page, or click **Open demo**.
2. Listen on the left. When you find something, select those rounds on the right,
   tick the players, and click **Report** on the offender.
3. Mark the moments, pick a reason, click **Export…**, and paste the report into the
   FACEIT support ticket together with the exported files.

## Building and testing locally

Needs **Go 1.26+** and **Node.js** (for JS syntax/unit tests) — no C compiler is
required for web builds.

### Build the static site

On Windows (PowerShell):

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build-web.ps1
```

On Linux / macOS (Bash):

```bash
bash scripts/build-web.sh
```

Serve `build/web/` locally:

```bash
python -m http.server 8099 --bind 127.0.0.1 -d build/web
```

### Run all tests

On Windows (PowerShell):

```powershell
powershell -ExecutionPolicy Bypass -File scripts/test.ps1
```

On Linux / macOS (Bash):

```bash
bash scripts/test.sh
```

To also run Go tests that verify real demo parsing and reports:

```powershell
$env:CS2VOICE_TEST_DEMO = "C:/path/to/match.dem.zst"
go test -count=1 ./...
```

`cmd/demoinfo` is a command-line tool for development. It prints a demo's rounds,
players and voice, and can write them to JSON (`-json`) or one `.ogg` per player
(`-ogg`):

```bash
go run ./cmd/demoinfo match.dem.zst
```

### All-talk rules

Whether a server announces all-talk, and with which text, is kept per platform and
CS2 game version in `internal/rules/rules.json`. If FACEIT changes it in a new game
version, add an entry there.

## Acknowledgements

The idea came from [csgo-voice-extractor](https://github.com/akiver/csgo-voice-extractor)
by akiver: pulling only the players' audio out of a demo. It is the voice extractor
that [CS Demo Manager](https://github.com/akiver/cs-demo-manager) uses. CS2 Voice is
a separate program written from scratch, and no code was copied from it. Thanks to
akiver for showing it could be done.

Demo parsing is done by
[demoinfocs-golang](https://github.com/markus-wa/demoinfocs-golang) by Markus Walther.

## License

MIT — see [`LICENSE`](LICENSE). The web app also ships third-party libraries
(including `demoinfocs-golang`, `opus-decoder` / libopus, and `wasm-media-encoders` / LAME).
See [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) and the [`licenses/`](licenses/)
folder.
