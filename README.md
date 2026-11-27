# CS2 Voice (toxic.report)

Get player voice out of Counter-Strike 2 FACEIT demos in your browser. Listen round by round, save the clipped audio, and get a small report for FACEIT support.

Hosted at **[toxic.report](https://toxic.report)**. Everything runs locally on your machine via Go WebAssembly.

## Why this exists

Reporting toxic voice on FACEIT if you didn't use the right tools meant opening the demo in Counter-Strike 2, trying to find the relevant part through rounds in the demo viewer, screen-recording, and manually writing timestamps. CS2 Voice lets you drop a `.dem` or `.dem.zst` file straight into the browser, jump straight to the rounds where someone talked, and export clean audio clips with matching timestamps in a couple of minutes.

## What you can do with it

- **Filter by team, rounds, and players**, then switch between *Talk only* (cuts out the quiet parts) and *Full round* (actual timing). Each player has a color-coded waveform, so you can see who's talking and click anywhere to jump there.
- **Generate a FACEIT report if needed.** Select the rounds and players to include in the report, mark the toxic moments (`M`) on the timeline, pick a reason, and copy the generated English report, complete with match link, SteamIDs, and exact timestamps for each exported file.
- **Just cut clips.** Switch to the *Listen & Cut* tab to drag ranges (`C`) and export audio clips (MP3 or WAV, zipped or separate) without the reporting UI.
- **Local cache.** Opened demos stay in your browser's private storage for 7 days (just the extracted voice) with your marks and selections. Clearable anytime.

## Under the hood

Demo parsing and audio processing are written in Go, compiled to WebAssembly, and run in a Web Worker. The UI is plain JS (ES modules) and CSS without a frontend build step, and cached demos are stored in the browser's OPFS. 

## Running locally

You'll need **Go 1.26+** and **Node.js** (for JS tests).

```bash
# Build the static site (Linux / macOS)
bash scripts/build-web.sh

# Or on Windows (PowerShell)
powershell -ExecutionPolicy Bypass -File scripts/build-web.ps1

# Serve locally
python -m http.server 8099 --bind 127.0.0.1 -d build/web
```

Run the test suite with `bash scripts/test.sh` (or `powershell -ExecutionPolicy Bypass -File scripts/test.ps1`). There's also a small native CLI (`go run ./cmd/demoinfo match.dem.zst`) for inspecting demos from the terminal.

## AI Disclaimer

Most of the code was written using AI coding tools with a human-in-the-loop approach.

## Want to help?

Contributions of any kind, like bug reports, code reviews, UX ideas, and new features are all welcome. 

- **Translations:** add a language in `frontend/lang/` (English and pt-BR are in there now).
- **All-talk rules:** if FACEIT changes its all-talk chat message in a future update, the patterns live in `internal/rules/rules.json`.
- **Code & features:** open an issue or PR (check [CONTRIBUTING.md](CONTRIBUTING.md)).

## Acknowledgements

The idea came from [csgo-voice-extractor](https://github.com/akiver/csgo-voice-extractor) by akiver, the voice extractor behind [CS Demo Manager](https://github.com/akiver/cs-demo-manager). 

Demo parsing is powered by [demoinfocs-golang](https://github.com/markus-wa/demoinfocs-golang) by Markus Walther.

## License

MIT — see [LICENSE](LICENSE). The web app also ships third-party libraries (including `demoinfocs-golang`, `opus-decoder` / libopus, and `wasm-media-encoders` / LAME); see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and [`licenses/`](licenses/).
