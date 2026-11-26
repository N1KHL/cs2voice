package demo

import (
	"bufio"
	"io"
	"os"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// openDemo opens a .dem file, or a FACEIT .dem.zst, and returns a reader with
// the raw demo bytes.
func openDemo(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	r, err := decompress(f, path)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &wrapped{Reader: r, close: func() error { r.Close(); return f.Close() }}, nil
}

// decompress returns the raw demo bytes of src, which holds a .dem or a .dem.zst;
// name is the file name (a hint next to the magic bytes). Closing the
// result does not close src.
func decompress(src io.Reader, name string) (io.ReadCloser, error) {
	br := bufio.NewReaderSize(src, 1<<20)

	// Detect the format by its magic bytes, not only by the file extension.
	magic, _ := br.Peek(4)
	lower := strings.ToLower(name)
	switch {
	case len(magic) == 4 && magic[0] == 0x28 && magic[1] == 0xB5 && magic[2] == 0x2F && magic[3] == 0xFD,
		strings.HasSuffix(lower, ".zst"):
		dec, err := zstd.NewReader(br, zstd.WithDecoderMaxWindow(1<<31), zstd.WithDecoderConcurrency(0))
		if err != nil {
			return nil, err
		}
		return &wrapped{Reader: dec, close: func() error { dec.Close(); return nil }}, nil
	}

	return &wrapped{Reader: br, close: func() error { return nil }}, nil
}

type wrapped struct {
	io.Reader
	close func() error
}

func (w *wrapped) Close() error { return w.close() }
