package app

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func itoa(i int) string { return strconv.Itoa(i) }

func jsonUnmarshal(b []byte, v interface{}) error { return json.Unmarshal(b, v) }

func jsonMarshal(v interface{}) ([]byte, error) { return json.Marshal(v) }

func jsonUnmarshalString(s string) interface{} {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return gin.H{}
	}
	return v
}

func ioCopy(dst io.Writer, src io.Reader) (int64, error) { return io.Copy(dst, src) }

// newUUID returns a random RFC-4122 v4 UUID string (dashed, lower-case), e.g.
// "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx". Immich uses UUID primary keys and its
// OpenAPI contract requires the v4 UUID `pattern`, so we MUST emit the dashed
// form (not a bare 32-hex string) or every generated id fails schema checks.
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// nowISO returns the current time formatted the way Immich serialises dates.
func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
