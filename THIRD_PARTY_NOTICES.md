# Third-party notices

CS2 Voice itself is MIT licensed (see `LICENSE`). The web app ships and runs
the software below in your browser. The full license texts are in
[`licenses/`](licenses/).

## Web libraries (`frontend/vendor/`)

| Library | Version | License | Text |
|---|---|---|---|
| [opus-decoder](https://github.com/eshaz/wasm-audio-decoders/tree/main/src/opus-decoder) — WASM Opus voice decoding (embeds libopus) | 0.7.12 | MIT (libopus: BSD-3-Clause) | `licenses/opus-decoder.txt`, `licenses/opus.txt` |
| [wasm-media-encoders](https://github.com/arseneyr/wasm-media-encoders) — JS wrapper (`WasmMediaEncoder.min.js`) | 0.7.0 | MIT | `licenses/wasm-media-encoders.txt` |
| [LAME](https://lame.sourceforge.io/) via [wasm-media-encoders](https://github.com/arseneyr/wasm-media-encoders) — MP3 WASM encoder (`mp3.wasm`) | 0.7.0 (LAME) | LGPL-2.0-or-later | `licenses/LGPL-2.1.txt` |

### LGPL on the web (`mp3.wasm`)

The LAME MP3 encoder is shipped as a **separate, replaceable WebAssembly binary** at
`frontend/vendor/mp3.wasm` alongside `frontend/vendor/WasmMediaEncoder.min.js`.
To replace it with your own modified build of LAME, rebuild `mp3.wasm` from the
[LAME](https://lame.sourceforge.io/) and
[wasm-media-encoders](https://github.com/arseneyr/wasm-media-encoders) sources and
drop the resulting `mp3.wasm` into `frontend/vendor/mp3.wasm` (or `mp3.wasm` in
the deployed `vendor/` directory).

## Go modules

| Module | Version | License | Text |
|---|---|---|---|
| Go standard library | toolchain in `go.mod` | BSD-3-Clause | `licenses/go.txt` |
| [demoinfocs-golang](https://github.com/markus-wa/demoinfocs-golang) — CS2 demo parser | v5.3.0 | MIT | `licenses/demoinfocs-golang.txt` |
| [klauspost/compress](https://github.com/klauspost/compress) — zstd | v1.20.5 | BSD-3-Clause and others (see text) | `licenses/klauspost-compress.txt` |
| [golang/geo](https://github.com/golang/geo) | v0.0.0-20230421003525 | Apache-2.0 | `licenses/golang-geo.txt` |
| [golang/snappy](https://github.com/golang/snappy) | v1.0.0 | BSD-3-Clause | `licenses/golang-snappy.txt` |
| [markus-wa/go-unassert](https://github.com/markus-wa/go-unassert) | v0.1.3 | MIT | `licenses/go-unassert.txt` |
| [markus-wa/gobitread](https://github.com/markus-wa/gobitread) | v0.2.5-0.20241202000432 | MIT | `licenses/gobitread.txt` |
| [markus-wa/godispatch](https://github.com/markus-wa/godispatch) | v1.4.1 | MIT | `licenses/godispatch.txt` |
| [markus-wa/ice-cipher-go](https://github.com/markus-wa/ice-cipher-go) | v0.0.0-20230901094113 | MIT | `licenses/ice-cipher-go.txt` |
| [markus-wa/quickhull-go](https://github.com/markus-wa/quickhull-go) | v2.2.0 | MIT | `licenses/quickhull-go.txt` |
| [oklog/ulid](https://github.com/oklog/ulid) | v2.1.1 | Apache-2.0 | `licenses/oklog-ulid.txt` |
| [pkg/errors](https://github.com/pkg/errors) | v0.9.1 | BSD-2-Clause | `licenses/pkg-errors.txt` |
| [golang.org/x/exp](https://pkg.go.dev/golang.org/x/exp) | v0.0.0-20260410095643 | BSD-3-Clause | `licenses/golang-x-exp.txt` |
| [google.golang.org/protobuf](https://github.com/protocolbuffers/protobuf-go) | v1.36.11 | BSD-3-Clause | `licenses/protobuf-go.txt` |

## Keeping this file up to date

When a dependency is added, removed or upgraded:

1. List the Go modules compiled into the WebAssembly binary (`go version -m`
   cannot read `.wasm` files):
   ```bash
   GOOS=js GOARCH=wasm go list -deps -f '{{with .Module}}{{.Path}} {{.Version}}{{end}}' ./cmd/web | sort -u
   ```
   Modules that only appear in `go.mod` (e.g. test dependencies of a
   dependency) are not shipped and are not listed here. Then check the vendored
   JS/WASM libraries in `frontend/vendor/`.
2. Copy each license file into `licenses/`. Go module licenses are in
   `$(go env GOMODCACHE)/<module>@<version>/`.
3. Update the tables above.
