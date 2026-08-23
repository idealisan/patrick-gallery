package image

import "testing"

func TestSniffFormat(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	png := []byte{0x89, 'P', 'N', 'G', 0x0D}
	gif := []byte("GIF89a")
	if f := SniffFormat(jpeg); f != "jpeg" {
		t.Fatalf("jpeg got %q", f)
	}
	if f := SniffFormat(png); f != "png" {
		t.Fatalf("png got %q", f)
	}
	if f := SniffFormat(gif); f != "gif" {
		t.Fatalf("gif got %q", f)
	}
	if f := SniffFormat([]byte("nothing")); f != "" {
		t.Fatalf("unknown got %q", f)
	}
}

func TestFamilyVersionsBump(t *testing.T) {
	if CurrentCacheVersion(FamilyHEIC) != 1 {
		t.Fatal("heic should start at v1")
	}
	if BumpCacheVersion(FamilyHEIC) != 2 {
		t.Fatal("bump should yield 2")
	}
	defer BumpCacheVersion(FamilyHEIC) // restore for other tests? (process-local)
	if CurrentCacheVersion(FamilyJPEG) != 1 {
		t.Fatal("jpeg must be unaffected by heic bump")
	}
}

func TestFamilyForFormat(t *testing.T) {
	cases := map[string]CacheFamily{
		"jpeg": FamilyJPEG, "heic": FamilyHEIC, "avif": FamilyHEIC,
		"webp": FamilyWebP, "gif": FamilyGIF, "bmp": "",
	}
	for in, want := range cases {
		if got := FamilyForFormat(in); got != want {
			t.Fatalf("FamilyForFormat(%q)=%v want %v", in, got, want)
		}
	}
}
