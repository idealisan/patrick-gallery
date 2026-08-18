package video

// qt_faststart.go — pure Go qt-faststart implementation.
// Relocates the moov atom before mdat in an MP4 file so browsers can start
// playback without downloading the entire file. No CGO, no CLI.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// mp4Atom represents a top-level MP4 atom.
type mp4Atom struct {
	offset int64  // file offset of atom start (including size+type)
	size   int64  // total size including the 8-byte header
	fourcc [4]byte
}

// qtFaststart checks whether the MP4 file needs faststart (moov after mdat)
// and if so, rewrites the file with moov before mdat. It modifies the file
// in-place. Returns nil if no relocation was needed or if successful.
func QTFaststart(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}
	fileSize := stat.Size()
	if fileSize < 8 {
		return errors.New("faststart: file too small")
	}

	// Parse top-level atoms.
	atoms, err := parseTopLevelAtoms(f, fileSize)
	if err != nil {
		return err
	}

	// Find ftyp, moov, mdat.
	var ftypIdx, moovIdx, mdatIdx int = -1, -1, -1
	for i, a := range atoms {
		switch string(a.fourcc[:]) {
		case "ftyp":
			ftypIdx = i
		case "moov":
			moovIdx = i
		case "mdat":
		mdatIdx = i
		}
	}

	// No moov or mdat -> nothing to do (or invalid file).
	if moovIdx < 0 || mdatIdx < 0 {
		return nil
	}

	// If moov is already before mdat, no relocation needed.
	if moovIdx < mdatIdx {
		return nil
	}

	// Need to relocate moov before mdat.
	// The file layout will be: [ftyp] [moov] [free/other] [mdat]
	// We rewrite the file by copying in order: ftyp, moov, then mdat.

	moovAtom := atoms[moovIdx]
	moovData := make([]byte, moovAtom.size)
	if _, err := f.ReadAt(moovData, moovAtom.offset); err != nil {
		return errors.New("faststart: read moov")
	}

	// Create temp file for the output.
	tmp, err := os.CreateTemp("", "faststart-*.mp4")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName) // cleanup on error; rename on success
	}()

	// Write ftyp first (if present).
	if ftypIdx >= 0 {
		ftypAtom := atoms[ftypIdx]
		ftypData := make([]byte, ftypAtom.size)
		if _, err := f.ReadAt(ftypData, ftypAtom.offset); err != nil {
			return errors.New("faststart: read ftyp")
		}
		if _, err := tmp.Write(ftypData); err != nil {
			return errors.New("faststart: write ftyp")
		}
	}

	// Write moov.
	if _, err := tmp.Write(moovData); err != nil {
		return errors.New("faststart: write moov")
	}

	// Write free atoms that were between ftyp and moov (if any).
	for i, a := range atoms {
		if i == moovIdx {
			continue
		}
		fourcc := string(a.fourcc[:])
		if fourcc == "free" || fourcc == "wide" || fourcc == "mdat" || fourcc == "moov" || fourcc == "ftyp" {
			continue
		}
		// Write any other atoms that precede mdat.
		if i < mdatIdx {
			data := make([]byte, a.size)
			if _, err := f.ReadAt(data, a.offset); err != nil {
				continue // skip on error
			}
			tmp.Write(data)
		}
	}

	// Write mdat — use ReadAt to avoid seek-position state issues with the
	// same file handle used earlier for ftyp/moov reads.
	mdatAtom := atoms[mdatIdx]
	mdatData := make([]byte, mdatAtom.size)
	if _, err := f.ReadAt(mdatData, mdatAtom.offset); err != nil {
		return errors.New("faststart: read mdat")
	}
	if _, err := tmp.Write(mdatData); err != nil {
		return errors.New("faststart: write mdat")
	}

	// Preserve any atoms after mdat (shouldn't exist in valid files, but be safe).
	for _, a := range atoms {
		fourcc := string(a.fourcc[:])
		if fourcc == "ftyp" || fourcc == "moov" || fourcc == "mdat" {
			continue
		}
		if a.offset > mdatAtom.offset {
			data := make([]byte, a.size)
			if _, err := f.ReadAt(data, a.offset); err != nil {
				continue
			}
			tmp.Write(data)
		}
	}

	// Sync and close before rename.
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Atomic replace.
	return os.Rename(tmpName, path)
}

// parseTopLevelAtoms reads the top-level atom structure of an MP4 file.
func parseTopLevelAtoms(r io.ReaderAt, fileSize int64) ([]mp4Atom, error) {
	var atoms []mp4Atom
	var offset int64

	for offset < fileSize-8 {
		var size uint32
		var fourcc [4]byte
		hdr := io.NewSectionReader(r, offset, 8)
		if err := binary.Read(hdr, binary.BigEndian, &size); err != nil {
			break
		}
		if _, err := hdr.Read(fourcc[:]); err != nil {
			break
		}
		if size == 0 {
			// atom extends to EOF
			size = uint32(fileSize - offset)
		}
		if size < 8 {
			break // invalid
		}
		atoms = append(atoms, mp4Atom{
			offset: offset,
			size:   int64(size),
			fourcc: fourcc,
		})
		offset += int64(size)
	}

	return atoms, nil
}

// needsFaststart reports whether the file needs faststart processing.
// Returns true if moov atom is after mdat.
func needsFaststart(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return false, err
	}
	fileSize := stat.Size()
	if fileSize < 8 {
		return false, nil
	}

	atoms, err := parseTopLevelAtoms(f, fileSize)
	if err != nil {
		return false, err
	}

	var moovOffset, mdatOffset int64 = -1, -1
	for _, a := range atoms {
		if bytes.Equal(a.fourcc[:], []byte("moov")) {
			moovOffset = a.offset
		}
		if bytes.Equal(a.fourcc[:], []byte("mdat")) {
			mdatOffset = a.offset
		}
	}

	if moovOffset < 0 || mdatOffset < 0 {
		return false, nil
	}

	return moovOffset > mdatOffset, nil
}
