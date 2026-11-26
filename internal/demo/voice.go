package demo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
)

// PacketSamples returns how long an Opus packet lasts, in samples at 48 kHz,
// read from its TOC byte (RFC 6716 section 3.1). It returns 0 if the packet is invalid.
func PacketSamples(p []byte) int {
	if len(p) == 0 {
		return 0
	}
	toc := p[0]
	config := int(toc >> 3)

	var frame int // samples per frame at 48 kHz
	switch {
	case config < 12: // SILK: 10/20/40/60 ms
		frame = []int{480, 960, 1920, 2880}[config%4]
	case config < 16: // Hybrid: 10/20 ms
		frame = []int{480, 960}[config%2]
	default: // CELT: 2.5/5/10/20 ms
		frame = []int{120, 240, 480, 960}[config%4]
	}

	var count int
	switch toc & 3 {
	case 0:
		count = 1
	case 1, 2:
		count = 2
	case 3:
		if len(p) < 2 {
			return 0
		}
		count = int(p[1] & 0x3F)
	}

	total := frame * count
	if total == 0 || total > 5760 { // 120 ms max
		return 0
	}
	return total
}

// splitOpus splits the voice_data of an Opus-format message into packets.
// Usually there is one packet per message; when packet_offsets is set, the
// data holds several packets back to back.
func splitOpus(data []byte, offsets []uint32) [][]byte {
	if len(offsets) <= 1 {
		return [][]byte{data}
	}
	// Offsets can be start offsets (first == 0) or end offsets (last == len).
	var bounds []int
	if offsets[0] == 0 {
		for _, o := range offsets {
			bounds = append(bounds, int(o))
		}
		bounds = append(bounds, len(data))
	} else {
		bounds = append(bounds, 0)
		for _, o := range offsets {
			bounds = append(bounds, int(o))
		}
	}
	var out [][]byte
	for i := 0; i+1 < len(bounds); i++ {
		a, b := bounds[i], bounds[i+1]
		if a < 0 || b > len(data) || a >= b {
			return [][]byte{data} // unexpected layout, keep it whole
		}
		out = append(out, data[a:b])
	}
	return out
}

// ---------- "Steam Voice" format (CS2 demos from before 7 Feb 2024) ----------
//
// Layout: steamid(8) | 0x0B | sampleRate(2) | voiceType(1) | len(2) | data | crc32(4)
// With voiceType 0x06, "data" is a list of: chunkLen(int16) | seq(uint16) | opus packet.

var errBadSteamPacket = errors.New("invalid Steam voice packet")

type steamState struct {
	synced  bool
	nextSeq uint16
}

// steamFrame is one Opus packet from a Steam voice payload, with the number of
// 20 ms frames that were lost just before it.
type steamFrame struct {
	data []byte
	lost int
}

func (s *steamState) frames(b []byte) ([]steamFrame, error) {
	if len(b) < 18 {
		return nil, errBadSteamPacket
	}
	if crc32.ChecksumIEEE(b[:len(b)-4]) != binary.LittleEndian.Uint32(b[len(b)-4:]) {
		return nil, errBadSteamPacket
	}
	if b[8] != 0x0B {
		return nil, errBadSteamPacket
	}
	voiceType := b[11]
	length := int(binary.LittleEndian.Uint16(b[12:14]))
	if voiceType == 0x00 { // silence
		return nil, nil
	}
	if voiceType != 0x06 || 14+length > len(b)-4 {
		return nil, errBadSteamPacket
	}

	var out []steamFrame
	buf := bytes.NewReader(b[14 : 14+length])
	for buf.Len() > 0 {
		var chunkLen int16
		if err := binary.Read(buf, binary.LittleEndian, &chunkLen); err != nil {
			return out, err
		}
		if chunkLen == -1 { // end of stream, sequence restarts
			s.synced = false
			break
		}
		var seq uint16
		if err := binary.Read(buf, binary.LittleEndian, &seq); err != nil {
			return out, err
		}
		if chunkLen < 0 || int(chunkLen) > buf.Len() {
			return out, errBadSteamPacket
		}
		data := make([]byte, chunkLen)
		buf.Read(data)

		lost := 0
		if s.synced {
			if seq < s.nextSeq {
				continue // old or duplicate frame
			}
			lost = min(int(seq-s.nextSeq), 10)
		}
		s.synced = true
		s.nextSeq = seq + 1
		if len(data) == 0 {
			continue
		}
		out = append(out, steamFrame{data: data, lost: lost})
	}
	return out, nil
}

const utterancePause = 0.35 // seconds

func buildUtterances(pk []Packet, rounds []Round) []Utterance {
	var out []Utterance
	for i, p := range pk {
		end := p.Time + float64(p.Samples)/48000
		if len(out) > 0 && p.Time-out[len(out)-1].End <= utterancePause {
			u := &out[len(out)-1]
			u.Last = i
			u.End = max(u.End+float64(p.Samples)/48000, end)
			continue
		}
		out = append(out, Utterance{Start: p.Time, End: end, StartTick: p.Tick, First: i, Last: i})
	}
	for i := range out {
		out[i].Period = PeriodAt(rounds, out[i].Start)
	}
	return out
}
