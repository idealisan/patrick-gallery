package image

import (
	"bytes"
	"os"
	"testing"

	"github.com/rwcarlsen/goexif/exif"
)

func TestHeicExtractEXIFReal(t *testing.T) {
	path := os.Getenv("HEIC_PATH")
	if path == "" {
		t.Skip("HEIC_PATH not set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	payload := heicExtractEXIF(raw)
	if payload == nil {
		t.Fatal("no exif payload found")
	}
	t.Logf("payload head: %x", payload[:16])
	x, err := exif.Decode(bytesReader(payload))
	if err != nil {
		t.Fatalf("goexif decode: %v", err)
	}
	for _, f := range []exif.FieldName{exif.Make, exif.Model, exif.DateTimeOriginal, exif.Orientation} {
		if tag, err := x.Get(f); err == nil {
			s, _ := tag.StringVal()
			t.Logf("%s = %q", f, s)
		}
	}
	if lat, long, err := x.LatLong(); err == nil {
		t.Logf("gps: %v,%v", lat, long)
	}
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
