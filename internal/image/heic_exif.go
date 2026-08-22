package image

import (
	"bytes"
	"encoding/binary"
)

// heic_exif.go extracts the raw EXIF payload embedded in a HEIC/HEIF/AVIF
// container (iPhone stills carry their EXIF as an ISO-BMFF meta item of type
// 'Exif'). The walk is intentionally minimal: find the 'meta' box, read 'iinf'
// to learn which item id carries the 'Exif' type, resolve its byte extents via
// 'iloc' and slice the payload out of the original buffer. No CGO.

var heicBrands = [][]byte{
	[]byte("heic"), []byte("heix"), []byte("heim"),
	[]byte("hevc"), []byte("hevx"), []byte("hevm"), []byte("hevs"),
	[]byte("mif1"), []byte("msf1"), []byte("avif"),
}

// HasHEICBrand reports whether raw starts with a HEIC/HEIF/AVIF ftyp brand.
func HasHEICBrand(raw []byte) bool {
	if len(raw) < 12 || !bytes.Equal(raw[4:8], []byte("ftyp")) {
		return false
	}
	tail := raw[8:min(64, len(raw))]
	for _, b := range heicBrands {
		if bytes.Contains(tail, b) {
			return true
		}
	}
	return false
}

// box is one ISOBMFF box header.
type box struct {
	typ   string // fourcc
	start int    // offset of the body (after header)
	end   int    // exclusive end of the body
}

// nextBox parses the box header at off. Returns ok=false at EOF/truncation.
func nextBox(raw []byte, off int) (box, bool) {
	if off+8 > len(raw) {
		return box{}, false
	}
	size := int(binary.BigEndian.Uint32(raw[off : off+4]))
	header := 8
	if size == 1 {
		if off+16 > len(raw) {
			return box{}, false
		}
		size64 := binary.BigEndian.Uint64(raw[off+8 : off+16])
		if size64 > uint64(len(raw)-off) {
			return box{}, false
		}
		header = 16
	} else if size == 0 {
		size = len(raw) - off
	}
	if size < header || off+size > len(raw) {
		return box{}, false
	}
	return box{typ: string(raw[off+4 : off+8]), start: off + header, end: off + size}, true
}

// childBoxes iterates the boxes contained in [start,end).
func childBoxes(raw []byte, start, end int) []box {
	var out []box
	for off := start; off < end; {
		b, ok := nextBox(raw, off)
		if !ok || b.end > end || b.end <= b.start {
			break
		}
		out = append(out, b)
		off = b.end
	}
	return out
}

// heicItem describes one iinf entry resolved through iloc.
type heicItem struct {
	typ     string
	offsets []int // absolute file offsets of the item's extents
	lengths []int
}

// heicExtractEXIF returns the TIFF block carried in the container's 'Exif'
// item ready for exif.Decode, or nil when absent/malformed.
func heicExtractEXIF(raw []byte) []byte {
	if !HasHEICBrand(raw) {
		return nil
	}
	for off := 0; off < len(raw); {
		top, ok := nextBox(raw, off)
		if !ok {
			return nil
		}
		if top.typ == "meta" {
			// meta is a FullBox: skip its 4 version/flags bytes.
			metaBody := box{typ: "meta", start: top.start + 4, end: top.end}
			if metaBody.start > metaBody.end {
				return nil
			}
			var exifID uint32
			var ilocAt *box
			for _, b := range childBoxes(raw, metaBody.start, metaBody.end) {
				switch b.typ {
				case "iinf":
					exifID = heicFindExifItemID(raw, b)
				case "iloc":
					bb := b
					ilocAt = &bb
				}
			}
			if exifID == 0 || ilocAt == nil {
				return nil
			}
			extents := heicResolveExtents(raw, *ilocAt, exifID)
			payload := heicSliceExtents(raw, extents)
			return normalizeTIFF(payload)
		}
		off = top.end
	}
	return nil
}

// heicFindExifItemID walks iinf entries and returns the item id typed 'Exif'.
func heicFindExifItemID(raw []byte, iinf box) uint32 {
	body := raw[iinf.start:iinf.end]
	if len(body) < 6 {
		return 0
	}
	version := body[0]
	pos := 4 // version + flags
	var count int
	if version >= 2 {
		if pos+4 > len(body) {
			return 0
		}
		count = int(binary.BigEndian.Uint32(body[pos : pos+4]))
		pos += 4
	} else {
		if pos+2 > len(body) {
			return 0
		}
		count = int(binary.BigEndian.Uint16(body[pos : pos+2]))
		pos += 2
	}
	for i := 0; i < count; i++ {
		ib, ok := nextBox(raw, iinf.start+pos)
		if !ok || ib.typ != "infe" || ib.end > iinf.end {
			return 0
		}
		infe := raw[ib.start:ib.end]
		if len(infe) >= 4 {
			v := infe[0]
			p := 4 // version + flags inside infe
			var itemID uint32
			switch {
			case v == 2:
				if p+2 <= len(infe) {
					itemID = uint32(binary.BigEndian.Uint16(infe[p : p+2]))
					p += 2 + 2 // + protection_index
				}
			case v >= 3:
				if p+4 <= len(infe) {
					itemID = binary.BigEndian.Uint32(infe[p : p+4])
					p += 4 + 2
				}
			default: // v0/v1
				if p+2 <= len(infe) {
					itemID = uint32(binary.BigEndian.Uint16(infe[p : p+2]))
					p += 4 // item_ID(u16) + protection_index(u16); type follows in v1 only
				}
			}
			if p+4 <= len(infe) {
				typ := string(infe[p : p+4])
				if typ == "Exif" {
					return itemID
				}
			}
		}
		pos = ib.end - iinf.start
	}
	return 0
}

// heicResolveExtents locates the iloc entry for itemID. Writers differ in how
// many fixed bytes sit between item_ID and extent_count (construction_method /
// data_reference_index / reserved), so we try each plausible stride and keep
// the one that yields an exact, fully in-bounds parse of every entry.
func heicResolveExtents(raw []byte, iloc box, itemID uint32) [][2]int {
	body := raw[iloc.start:iloc.end]
	if len(body) < 8 {
		return nil
	}
	version := body[0]
	offsetSize := int(body[4] >> 4)
	lengthSize := int(body[4] & 0xf)
	baseOffsetSize := int(body[5] >> 4)
	idLen := 2
	if version >= 2 {
		idLen = 4
	}
	pos := 8 // version+flags(4) + sizes(2) + item_count(2); per-entry fields follow

	readN := func(p, n int) (int, int) {
		v := 0
		for i := 0; i < n && p+i < len(body); i++ {
			v = v<<8 | int(body[p+i])
		}
		return v, p + n
	}
	_ = readN
	count := 0
	if pos+2 <= len(body) {
		count = int(binary.BigEndian.Uint16(body[6:8]))
	}
	if version >= 2 && pos+4 <= len(body) {
		count = int(binary.BigEndian.Uint32(body[6:10]))
		pos = 10
	}

	for _, gap := range [...]int{3, 4, 2} { // ISO v1/v0 layout first, then writer variants
		extents, ok := heicTryStride(body, len(raw), pos, count, idLen, gap, offsetSize, lengthSize, baseOffsetSize, itemID)
		if ok {
			return extents
		}
	}
	return nil
}

// heicTryStride attempts one full iloc parse with a fixed gap between item_ID
// and extent_count. ok requires consuming exactly all entries with extents in
// file bounds. raw bounds the absolute extent offsets.
func heicTryStride(body []byte, rawLen, start, count, idLen, gap, osz, lsz, bosz int, wantID uint32) ([][2]int, bool) {
	pos := start
	readN := func(n int) (int, bool) {
		if pos+n > len(body) {
			return 0, false
		}
		v := 0
		for i := 0; i < n; i++ {
			v = v<<8 | int(body[pos+i])
		}
		pos += n
		return v, true
	}
	var want [][2]int
	for i := 0; i < count; i++ {
		id, ok := readN(idLen)
		if !ok {
			return nil, false
		}
		pos += gap
		base := 0
		if bosz > 0 {
			base, ok = readN(bosz)
			if !ok {
				return nil, false
			}
		}
		ec, ok := readN(2)
		if !ok || ec < 0 || ec > (1<<20) {
			return nil, false
		}
		for j := 0; j < ec; j++ {
			o, ok1 := readN(osz)
			l, ok2 := readN(lsz)
			if !ok1 || !ok2 {
				return nil, false
			}
			abs, ln := base+o, l
			if abs < 0 || ln <= 0 || abs+ln > rawLen || abs+ln < abs {
				return nil, false
			}
			if uint32(id) == wantID {
				want = append(want, [2]int{abs, ln})
			}
		}
	}
	return want, true
}

func heicSliceExtents(raw []byte, extents [][2]int) []byte {
	var buf []byte
	for _, e := range extents {
		o, l := e[0], e[1]
		if o < 0 || l <= 0 || o+l > len(raw) || o+l < o {
			continue
		}
		buf = append(buf, raw[o:o+l]...)
	}
	return buf
}

// normalizeTIFF strips the 'Exif\0\0' item prefix (and any leading tiff-header
// offset field) so exif.Decode sees "II*\0"/"MM\0*".
func normalizeTIFF(payload []byte) []byte {
	if len(payload) < 8 {
		return nil
	}
	if payload[0] == 'I' && payload[1] == 'I' || payload[0] == 'M' && payload[1] == 'M' {
		return payload
	}
	// Apple style: u32 tiff-offset then "Exif\0\0" then TIFF.
	if len(payload) >= 12 && bytes.Equal(payload[4:10], []byte("Exif\x00\x00")) {
		p := payload[10:]
		if len(p) >= 4 && (p[0] == 'I' && p[1] == 'I' || p[0] == 'M' && p[1] == 'M') {
			return p
		}
	}
	// Generic: u32 big-endian offset straight to "MM"/"II".
	hdrOff := int(binary.BigEndian.Uint32(payload[:4]))
	if hdrOff >= 4 && hdrOff+4 <= len(payload) {
		p := payload[hdrOff:]
		if p[0] == 'I' && p[1] == 'I' || p[0] == 'M' && p[1] == 'M' {
			return p
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
