//go:build linux || darwin

package video

import "testing"

// The purego backend pins FFmpeg 7.x struct offsets; a mismatched runtime must
// be refused (and degrade to the placeholder) instead of corrupting memory.
func TestCheckABIRejectsMismatchedFFmpeg(t *testing.T) {
	mk := func(major, minor, micro int) int { return major<<16 | minor<<8 | micro }
	cases := []struct {
		name          string
		codec, format int
		wantErr       bool
	}{
		{"FFmpeg 7.1 (libavcodec 61 / libavformat 61)", mk(61, 19, 100), mk(61, 7, 100), false},
		{"Debian 13 trixie 7.1.5", mk(61, 19, 100), mk(61, 19, 100), false},
		{"Debian 12 bookworm 5.1 (59/59)", mk(59, 37, 100), mk(59, 27, 100), true},
		{"Ubuntu 24.04 6.1 (60/60)", mk(60, 31, 100), mk(60, 16, 100), true},
		{"future major 8.x (62/62)", mk(62, 0, 0), mk(62, 0, 0), true},
		{"codec ok / format wrong", mk(61, 19, 100), mk(60, 16, 100), true},
	}
	for _, c := range cases {
		err := checkABI(c.codec, c.format)
		if c.wantErr && err == nil {
			t.Errorf("%s: expected an ABI rejection, got nil", c.name)
		}
		if !c.wantErr && err != nil {
			t.Errorf("%s: expected acceptance, got %v", c.name, err)
		}
	}
}

func TestLibVersionMajor(t *testing.T) {
	if got := libVersionMajor(61<<16 | 19<<8 | 100); got != 61 {
		t.Fatalf("major = %d, want 61", got)
	}
}
