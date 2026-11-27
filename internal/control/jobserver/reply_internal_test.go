package jobserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A reply that cannot be encoded is a 500 with a reason, never the intended status with an
// empty body that the worker would read as success.
func TestReplyThatCannotBeEncodedIsAServerError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		v      any
		status int
	}{
		{"encodable", map[string]string{"ok": "yes"}, http.StatusOK},
		{"a channel", map[string]any{"c": make(chan int)}, http.StatusInternalServerError},
	} {
		rec := httptest.NewRecorder()
		reply(rec, http.StatusOK, tc.v)
		if rec.Code != tc.status {
			t.Errorf("%s: status %d, want %d", tc.name, rec.Code, tc.status)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body) == 0 {
			t.Errorf("%s: body %q is not a JSON object: %v", tc.name, rec.Body, err)
		}
	}
}
