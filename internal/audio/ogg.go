package audio

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"

	"cs2voice/internal/demo"
)

// Minimal Ogg Opus muxer (RFC 7845). It wraps Opus packets as they are,
// with no decoding, so it needs no CGO or libopus.

var oggCRCTable = func() [256]uint32 {
	var t [256]uint32
	for i := range t {
		r := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if r&0x80000000 != 0 {
				r = (r << 1) ^ 0x04c11db7
			} else {
				r <<= 1
			}
		}
		t[i] = r
	}
	return t
}()

func oggCRC(b []byte) uint32 {
	var crc uint32
	for _, v := range b {
		crc = (crc << 8) ^ oggCRCTable[byte(crc>>24)^v]
	}
	return crc
}

type oggWriter struct {
	w       io.Writer
	serial  uint32
	seq     uint32
	granule uint64

	// packets waiting for the current page
	pending  [][]byte
	segments int
	err      error
}

const (
	flagBOS = 0x02
	flagEOS = 0x04
)

func newOggWriter(w io.Writer, serial uint32, inputSampleRate uint32) *oggWriter {
	o := &oggWriter{w: w, serial: serial}

	head := make([]byte, 19)
	copy(head, "OpusHead")
	head[8] = 1                                 // version
	head[9] = 1                                 // channels (mono)
	binary.LittleEndian.PutUint16(head[10:], 0) // pre-skip
	binary.LittleEndian.PutUint32(head[12:], inputSampleRate)
	binary.LittleEndian.PutUint16(head[16:], 0) // output gain
	head[18] = 0                                // mapping family
	o.writePage([][]byte{head}, 0, flagBOS)

	vendor := "cs2voice"
	tags := make([]byte, 8+4+len(vendor)+4)
	copy(tags, "OpusTags")
	binary.LittleEndian.PutUint32(tags[8:], uint32(len(vendor)))
	copy(tags[12:], vendor)
	binary.LittleEndian.PutUint32(tags[12+len(vendor):], 0)
	o.writePage([][]byte{tags}, 0, 0)

	return o
}

func lacingCount(n int) int { return n/255 + 1 }

// WritePacket adds one Opus packet that lasts `samples` samples at 48 kHz.
func (o *oggWriter) WritePacket(p []byte, samples int) {
	if o.err != nil {
		return
	}
	need := lacingCount(len(p))
	if o.segments+need > 255 || len(o.pending) >= 50 {
		o.flush(0)
	}
	o.pending = append(o.pending, p)
	o.segments += need
	o.granule += uint64(samples)
}

func (o *oggWriter) flush(flags byte) {
	if len(o.pending) == 0 && flags&flagEOS == 0 {
		return
	}
	o.writePage(o.pending, o.granule, flags)
	o.pending = o.pending[:0]
	o.segments = 0
}

func (o *oggWriter) Close() error {
	o.flush(flagEOS)
	return o.err
}

func (o *oggWriter) writePage(packets [][]byte, granule uint64, flags byte) {
	if o.err != nil {
		return
	}
	var lacing []byte
	bodyLen := 0
	for _, p := range packets {
		n := len(p)
		for n >= 255 {
			lacing = append(lacing, 255)
			n -= 255
		}
		lacing = append(lacing, byte(n))
		bodyLen += len(p)
	}

	page := make([]byte, 27+len(lacing), 27+len(lacing)+bodyLen)
	copy(page, "OggS")
	page[4] = 0
	page[5] = flags
	binary.LittleEndian.PutUint64(page[6:], granule)
	binary.LittleEndian.PutUint32(page[14:], o.serial)
	binary.LittleEndian.PutUint32(page[18:], o.seq)
	page[26] = byte(len(lacing))
	copy(page[27:], lacing)
	for _, p := range packets {
		page = append(page, p...)
	}
	binary.LittleEndian.PutUint32(page[22:], oggCRC(page))

	o.seq++
	_, o.err = o.w.Write(page)
}

// WriteOggCompact writes the packets one after another, without the silences
// between them, to an Ogg Opus file. No decoding involved: a quick way to listen.
func WriteOggCompact(path string, packets []demo.Packet, serial uint32) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)

	o := newOggWriter(w, serial, 48000)
	for _, p := range packets {
		if p.Data == nil {
			writeSilence(o, p.Samples)
			continue
		}
		o.WritePacket(p.Data, p.Samples)
	}
	if err := o.Close(); err != nil {
		return err
	}
	return w.Flush()
}

// Silence: CELT fullband mono Opus packets with the "silence" flag set.
// Used to fill gaps when writing Ogg Opus without decoding.
var (
	silence120ms = []byte{0xFB, 0x06, 0xFF, 0xFE, 0xFF, 0xFE, 0xFF, 0xFE, 0xFF, 0xFE, 0xFF, 0xFE, 0xFF, 0xFE} // code 3, 6x20ms CBR
	silence20ms  = []byte{0xF8, 0xFF, 0xFE}
	silence10ms  = []byte{0xF0, 0xFF, 0xFE}
	silence5ms   = []byte{0xE8, 0xFF, 0xFE}
	silence2_5ms = []byte{0xE0, 0xFF, 0xFE}
)

// writeSilence writes about `samples` samples of silence (rounded down to 2.5 ms)
// and returns how many samples it actually wrote.
func writeSilence(o *oggWriter, samples int) (written int) {
	for _, s := range []struct {
		p []byte
		n int
	}{{silence120ms, 5760}, {silence20ms, 960}, {silence10ms, 480}, {silence5ms, 240}, {silence2_5ms, 120}} {
		for samples >= s.n {
			o.WritePacket(s.p, s.n)
			samples -= s.n
			written += s.n
		}
	}
	return written
}
