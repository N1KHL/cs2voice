package audio

import (
	"archive/zip"
	"bytes"
	"strings"
	"time"

	"cs2voice/internal/demo"
)

// ExportedData holds one rendered audio file in memory along with its metadata.
type ExportedData struct {
	File ExportedFile
	Data []byte
}

// ExportToMemory renders all planned export files into memory without touching disk.
func ExportToMemory(m *demo.Match, o ExportOptions) ([]ExportedData, error) {
	planned, err := planExports(m, o)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]ExportedData, 0, len(planned))
	for i := range planned {
		pe := &planned[i]
		pe.file.Name = uniqueName(seen, pe.base+"."+pe.ext)
		var buf memWriteSeeker
		if _, err := pe.plan.WriteTo(&buf, Format(pe.ext), 0); err != nil {
			return nil, err
		}
		out = append(out, ExportedData{File: pe.file, Data: buf.buf})
	}
	return out, nil
}

// ExportZipToMemory renders each file sequentially and streams it directly into
// a ZIP archive so large exports never keep all uncompressed/encoded buffers in
// memory at once. If buildReport is non-nil and returns a non-empty string,
// report.txt is added to the archive.
func ExportZipToMemory(m *demo.Match, o ExportOptions, buildReport func([]ExportedFile) string) ([]ExportedFile, []byte, error) {
	planned, err := planExports(m, o)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	files := make([]ExportedFile, len(planned))
	for i := range planned {
		planned[i].file.Name = uniqueName(seen, planned[i].base+"."+planned[i].ext)
		files[i] = planned[i].file
	}

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	now := time.Now()
	for i := range planned {
		var buf memWriteSeeker
		if _, err := planned[i].plan.WriteTo(&buf, Format(planned[i].ext), 0); err != nil {
			zw.Close()
			return nil, nil, err
		}
		if err := writeZipEntry(zw, planned[i].file.Name, buf.buf, now); err != nil {
			zw.Close()
			return nil, nil, err
		}
		buf.buf = nil
	}
	if buildReport != nil {
		if reportText := buildReport(files); reportText != "" {
			if err := writeZipEntry(zw, "report.txt", []byte(reportText), now); err != nil {
				zw.Close()
				return nil, nil, err
			}
		}
	}
	if err := zw.Close(); err != nil {
		return nil, nil, err
	}
	return files, out.Bytes(), nil
}

// BuildZip packages pre-rendered audio files and an optional report.txt into a
// ZIP archive.
func BuildZip(files []ExportedData, reportText string) ([]byte, error) {
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	now := time.Now()
	for _, f := range files {
		if err := writeZipEntry(zw, f.File.Name, f.Data, now); err != nil {
			zw.Close()
			return nil, err
		}
	}
	if reportText != "" {
		if err := writeZipEntry(zw, "report.txt", []byte(reportText), now); err != nil {
			zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeZipEntry(zw *zip.Writer, name string, data []byte, modified time.Time) error {
	method := zip.Deflate
	if strings.HasSuffix(strings.ToLower(name), ".mp3") {
		method = zip.Store // MP3 is already compressed
	}
	fh := &zip.FileHeader{
		Name:     name,
		Method:   method,
		Modified: modified,
	}
	w, err := zw.CreateHeader(fh)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
