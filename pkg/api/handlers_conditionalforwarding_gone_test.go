package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleConditionalForwardingGone verifies the 410-Gone stub returns the
// documented JSON shape for every method on every /api/conditionalforwarding*
// route. Conditional Forwarding was removed in v0.27 (rules now live as
// Policy FORWARD entries); the stub keeps third-party tooling pinned to the
// old URL pointed at /api/policies. Slated for full removal in v0.28+.
func TestHandleConditionalForwardingGone(t *testing.T) {
	server := &Server{
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/conditionalforwarding"},
		{http.MethodPost, "/api/conditionalforwarding"},
		{http.MethodDelete, "/api/conditionalforwarding/abc-123"},
	}

	for _, tc := range cases {
		t.Run(tc.method+"_"+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()

			server.handleConditionalForwardingGone(w, req)

			assert.Equal(t, http.StatusGone, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var body map[string]string
			err := json.Unmarshal(w.Body.Bytes(), &body)
			require.NoError(t, err)

			assert.Equal(t, "gone", body["error"])
			assert.Equal(t, "/api/policies", body["migrate_to"])
			assert.Equal(t, "0.27.0", body["removed_in"])
			assert.Equal(t, "0.26.0", body["deprecated_in"])
			assert.Contains(t, body["message"], "Policy rules")
		})
	}
}
