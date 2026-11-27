# CS2 Voice — Agent Guide

Read this file and `CONTRIBUTING.md` before modifying anything in this
repository.

## 1. Project overview

**CS2 Voice** is a static web application (Go compiled to WebAssembly + Web
Workers + native ES modules) that extracts player voice from **Counter-Strike 2
FACEIT demos** (`*.dem.zst` or `*.dem`) entirely inside the user's browser.

- **Privacy invariant:** Everything runs client-side. Nothing is ever uploaded,
  and no external analytics, tracking, or third-party API calls may be added.
- **License:** MIT (`LICENSE`). Third-party notices and vendored library rules
  (including keeping LGPL `frontend/vendor/mp3.wasm` as a separate replaceable
  file) are tracked in `THIRD_PARTY_NOTICES.md` and `licenses/`.

## 2. Non-negotiable domain rules

Do not simplify or relax these rules without explicit maintainer approval:

1. **Teams are never mixed.** Each exported/previewed audio stream represents
   what one team heard.
2. **All-talk is the only exception**, detected strictly from FACEIT chat
   notices (`[FACEIT^] Alltalk enabled.` / `Alltalk disabled.`) configured per
   CS2 `GameVersion` in `internal/rules/rules.json`. When in doubt, never mix
   teams. "Both teams" in Export is only allowed when every selected period is
   all-talk.
3. **Warmup, knife round, and match end** are team-only on FACEIT. Knife round
   detection checks for knife/C4-only alive players at freeze end
   (`internal/demo/parse.go`).
4. **Exports are never normalized.** Loudness/mic-spam is evidence.
   `Plan.Normalize` ("Even out volume") is strictly for listening previews.
5. **Report text is always in English** (for FACEIT support), regardless of UI
   language. Times in the report map to positions inside each exported audio
   file, with demo time and tick in brackets.
6. **Marks & cut ranges** are stored in **demo time** (`demo.Span`) so they
   stay accurate across `Talk only` (`Compact`) and `Full round` (`Full`) modes.
7. **Export formats:** MP3 (default) and WAV only, delivered as one `.zip`
   (default, `localStorage` key `exportMode`) or separate files.
8. **Listen & Cut tab** has zero report features (no reported column, no report
   marks, no reasons, no report text, no FACEIT ticket button), and still obeys
   the team-isolation and no-normalization rules.
9. **No CS2 or FACEIT trademarks/logos** anywhere in assets.
10. **FACEIT support URL is fixed:**
    `https://support.faceit.com/hc/en-us/requests/new`.

## 3. Repository layout

```
cmd/web/             Go WASM worker RPC layer (//go:build js && wasm)
cmd/demoinfo/        Native dev CLI for inspecting demos
internal/demo/       Demo parser, match model, rounds, voice packets, all-talk
internal/rules/      Embedded rules.json for all-talk detection
internal/audio/      Plan, Render, Clip, Export, ExportToMemory, ZIP, Ogg, WAV
internal/codec/      Codec abstractions: js&&wasm bridges, !js stubs (WAV is pure Go)
internal/report/     English FACEIT report builder + tests
frontend/            Static web UI (ES modules, no bundler, no npm)
  lang/              en.json, pt-BR.json, index.json
  vendor/            Vendored opus-decoder + wasm-media-encoders + mp3.wasm
  _headers           Cloudflare Pages headers (application/wasm, caching)
tests/               Node built-in test suite (node --test tests/*.test.mjs)
scripts/             build-web.ps1, build-web.sh, test.ps1, test.sh
licenses/            Full license texts bundled into build/web/licenses.txt
```

### Frontend module DAG (keep unidirectional)

- Layer 0: `i18n.js`, `state.js`, `api.js`
- Layer 1: `ui.js`
- Layer 2: `player.js`
- Layer 3: `listen_panel.js`, `export_panel.js`, `cut.js` (must not import each other)
- Layer 4: `app.js`
- Worker: `worker.js` + `opus_bridge.js` + `mp3_bridge.js`

Never name a local variable `t` in `frontend/*.js` (it shadows `t(key, vars)`).

## 4. Build and verification commands

- **Full test & check suite:**
  - Windows: `powershell -ExecutionPolicy Bypass -File scripts/test.ps1`
  - Linux/macOS: `bash scripts/test.sh`
  Runs `gofmt -l .`, `go test ./...`, `GOOS=js GOARCH=wasm go vet`,
  `node --check` on `frontend/*.js`, and `node --test tests/*.test.mjs`.
- **Web build:**
  - Windows: `powershell -ExecutionPolicy Bypass -File scripts/build-web.ps1`
  - Linux/macOS (Cloudflare Pages): `bash scripts/build-web.sh`
  - Build always uses `-trimpath`. `wasm_exec.js` is copied from
    `$(go env GOROOT)/lib/wasm/wasm_exec.js` so it matches the Go compiler.
- **Real-demo Go tests (optional, skipped when unset):**
  ```powershell
  $env:CS2VOICE_TEST_DEMO = "C:/path/to/match.dem.zst"
  go test -count=1 ./...
  ```

## 5. Coding, commenting, and repo hygiene

- **Style & comments:**
  - Document *why* and invariants, not obvious *what*.
  - Add concise Go doc comments on exported symbols (starting with the symbol
    name).
  - Keep modules cohesive and avoid trivial micro-files, empty wrappers,
    duplicated helpers, or dead code.
  - In CSS (`frontend/style.css`), always use theme variables (`:root` /
    `:root[data-theme="light"]`), never hardcoded hex values in component rules.
- **Translations:** Any UI text change must update both `frontend/lang/en.json`
  and `frontend/lang/pt-BR.json` identically (verified by `tests/i18n.test.mjs`).
- **Testing:** Keep Go unit tests beside the code (`*_test.go`) and JS/bridge
  tests in `tests/*.test.mjs` so `frontend/` contains production assets only.
- **Git & security:**
  - Never commit demo files (`*.dem`, `*.dem.zst`), `build/web/`, binaries,
    secrets, or personal paths/usernames/emails.
  - Pull requests made with AI help **must** say so in the description, e.g. a
    line `AI-assisted`, so reviewers know what to check closely.
  - Update `THIRD_PARTY_NOTICES.md` and `licenses/` whenever dependencies change.
