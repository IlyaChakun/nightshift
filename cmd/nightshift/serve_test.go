package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type pingFunc func(context.Context) error

func (f pingFunc) Ping(ctx context.Context) error { return f(ctx) }

func TestHealthHandler(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		ping error
		want int
	}{
		"book up":   {nil, http.StatusOK},
		"book down": {errors.New("refused"), http.StatusServiceUnavailable},
	} {
		h := healthHandler(pingFunc(func(context.Context) error { return tc.ping }))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rec.Code != tc.want {
			t.Errorf("%s: code %d, want %d", name, rec.Code, tc.want)
		}
	}
}
