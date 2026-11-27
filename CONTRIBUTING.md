# Contributing to CS2 Voice

Thank you for wanting to contribute to **CS2 Voice**! This project extracts
player voice from **Counter-Strike 2 FACEIT demos** completely inside the user's
browser (Go compiled to WebAssembly + Web Workers), so players can review rounds
and export audio evidence with an English report for FACEIT support.

## Core principles

1. **Privacy first:** Everything runs 100% locally in the browser. Never upload
   demos, phone home, or add analytics/telemetry.
2. **Domain rules (do not break):**
   - **Teams are never mixed.** Each audio file represents what one team heard.
   - **All-talk is the only exception**, detected strictly from FACEIT chat
     notices (`[FACEIT^] Alltalk enabled.` / `Alltalk disabled.`) via
     `internal/rules/rules.json`. When in doubt, never mix teams.
   - **Exports are never volume-normalized** (loudness/mic-spam is evidence).
     "Even out volume" is a listening-only preview option.
   - **Reports are always in English**, regardless of UI language.
   - **FACEIT demos only** (`.dem.zst` or `.dem`).
   - **No CS2 or FACEIT trademarks/logos** in UI assets.

## Architecture overview

### Go backend & WASM worker

- `cmd/web/`: WebAssembly worker entrypoint (`GOOS=js GOARCH=wasm`). Thin RPC
  glue over `internal/*` (`main.go`, `helpers.go`, `preview.go`, `export.go`,
  `cut.go`).
- `cmd/demoinfo/`: Native CLI helper for inspecting demos (rounds, players,
  voice; optional JSON and per-player `.ogg`). It cannot render MP3/WAV: the
  codecs only exist in the browser.
- `internal/demo/`: Demo parsing, period/round timeline, voice packet/utterance
  extraction, and all-talk spans (`match.go`, `parse.go`, `rounds.go`,
  `voice.go`, `alltalk.go`, `open.go`).
- `internal/rules/`: Embedded `rules.json` per CS2 game version and platform.
- `internal/audio/`: Pure-Go timeline planning, clipping (`Selection.Clip`),
  mixing, waveform peak generation, in-memory WAV/MP3/ZIP rendering, and Ogg
  helpers.
- `internal/codec/`: Build-tagged codec layer:
  - `codec.go`, `wav.go` (pure Go, all platforms)
  - `opus_wasm.go`, `mp3_wasm.go` (`js && wasm`, synchronous JS bridges)
  - `stub.go` (`!js`, lets `go test ./...` and `cmd/demoinfo` build without
    any codec; tests that need Opus or MP3 skip)
- `internal/report/`: English FACEIT ticket generator (`report.Build`).

### Frontend (ES modules, no bundler, no npm)

`frontend/` uses native ES modules organized as a strict unidirectional
dependency graph (DAG) — lower layers never import upper layers:

- **Layer 0 (leaves):** `i18n.js`, `state.js`, `api.js`
- **Layer 1 (shared UI & download helpers):** `ui.js`
- **Layer 2 (shared player & waveform):** `player.js`
- **Layer 3 (independent panels/tabs):** `listen_panel.js`, `export_panel.js`, `cut.js`
- **Layer 4 (orchestration):** `app.js`
- **Worker & WASM bridges:** `worker.js`, `opus_bridge.js`, `mp3_bridge.js`, `vendor/`

## Development setup

You only need **Go 1.26+** and **Node.js** (no C compiler or npm required for web
development).

### Build & run locally

```powershell
# Windows (PowerShell)
powershell -ExecutionPolicy Bypass -File scripts/build-web.ps1
```

```bash
# Linux / macOS
bash scripts/build-web.sh
```

Serve `build/web/` locally:

```bash
python -m http.server 8099 --bind 127.0.0.1 -d build/web
```

### Run tests

Run the full check suite (formatting, Go unit tests, `js/wasm` vet, JS syntax,
and Node unit tests):

```powershell
# Windows (PowerShell)
powershell -ExecutionPolicy Bypass -File scripts/test.ps1
```

```bash
# Linux / macOS
bash scripts/test.sh
```

- Go tests live alongside the code (`*_test.go`) following standard Go
  conventions.
- Frontend/bridge unit tests live in `tests/*.test.mjs` (using built-in
  `node --test`) so test files are not copied into static deploy output
  (`build/web/`).
- To also run Go integration tests against a real demo:
  ```powershell
  $env:CS2VOICE_TEST_DEMO = "C:/path/to/match.dem.zst"
  go test -count=1 ./...
  ```

## Code style and commenting guidelines

Keep the codebase straightforward, predictable, and easy for a human to read
top to bottom:

- **Comment *why* and invariants, not *what*.** If the code clearly states what
  it does, don't repeat it in a comment. Document non-obvious domain reasons
  (e.g., why exports aren't normalized, why `Uint8Array` chunks from WASM are
  copied, why `GOGC=200` is used) and tricky edge-case invariants.
- **Document exported Go symbols** with concise Go doc comments starting with
  the symbol name (`// NewPlan lays out ...`).
- **Keep modules cohesive:** avoid scattering trivial one-function files,
  empty wrapper interfaces, or duplicated helpers.
- **Keep dependencies unidirectional:** never create circular imports between
  frontend modules.
- **Avoid variable shadowing gotchas:** in `frontend/*.js`, never name a local
  variable `t` (it shadows the `t(key, vars)` translation function).
- **Formatting:** always run `gofmt` and keep LF line endings (`.gitattributes`).
- **Themes:** use CSS variables from `:root` / `:root[data-theme="light"]` in
  `frontend/style.css`; never hardcode hex colors inside component rules.

## Translations (`frontend/lang/`)

- `frontend/lang/en.json` and `frontend/lang/pt-BR.json` must have identical
  keys and matching `{var}` placeholders (enforced by `tests/i18n.test.mjs`).
- To add a language, create `frontend/lang/<code>.json` with `"_name"` and add
  its code to `frontend/lang/index.json`.

## Pull request checklist

1. Never commit demo files (`*.dem`, `*.dem.zst`), build artifacts
   (`build/web/`, `*.wasm`, `*.exe`), or personal data/secrets.
2. Build Go binaries with `-trimpath`.
3. If you add, upgrade, or remove any Go module or vendored JS/WASM library:
   - Update `THIRD_PARTY_NOTICES.md` and `licenses/*.txt`.
   - Keep LGPL dependencies (`frontend/vendor/mp3.wasm`) as separate,
     replaceable files.
4. Ensure the test script (`scripts/test.ps1` or `scripts/test.sh`) and the
   web build (`scripts/build-web.ps1` or `scripts/build-web.sh`) pass before
   opening your pull request.
5. If AI tools helped, say so in the pull request description (e.g.,
   `AI-assisted`).
