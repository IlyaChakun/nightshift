package main

import (
	"testing"
	"time"
)

func TestDigestNowIsInTheTenantsZone(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 26, 13, 12, 0, 0, time.UTC)
	got, err := digestNow("Europe/Zurich", at)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(at) || got.Format("15:04 MST") != "14:12 CET" {
		t.Errorf("got %s, want the same instant as 14:12 CET", got.Format(time.RFC3339))
	}
	if utc, err := digestNow("", at); err != nil || utc.Location() != time.UTC {
		t.Errorf("no zone in the tenant file means UTC, as in the daily job: %v %v", utc, err)
	}
	if _, err := digestNow("Mars/Olympus", at); err == nil {
		t.Error("an unknown zone must be an error, not a silent UTC")
	}
}
