package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"time"
)

func itoa(i int) string { return strconv.Itoa(i) }

func jsonUnmarshal(b []byte, v interface{}) error { return json.Unmarshal(b, v) }

func ioCopy(dst io.Writer, src io.Reader) (int64, error) { return io.Copy(dst, src) }

// newUUID returns a random RFC-4122 v4 UUID string. Immich uses UUID primary
// keys; we generate them client-side-equivalent here without an extra dep.
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return hex.EncodeToString(b)
}

// nowISO returns the current time formatted the way Immich serialises dates.
func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
