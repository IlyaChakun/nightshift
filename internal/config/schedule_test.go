package config

import (
	"testing"
	"time"
)

func TestQuietUntil(t *testing.T) {
	t.Parallel()
	zurich := "Europe/Zurich"
	cases := []struct {
		name, window, zone, now string // now is UTC
		quiet                   bool
		until                   string // UTC
	}{
		{"no window", "", zurich, "2026-11-26T23:00:00Z", false, ""},
		{"evening part", "22:00-09:00", zurich, "2026-11-26T21:30:00Z", true, "2026-11-27T08:00:00Z"}, // 22:30 local
		{"morning part", "22:00-09:00", zurich, "2026-11-27T05:00:00Z", true, "2026-11-27T08:00:00Z"}, // 06:00 local
		{"just before the start", "22:00-09:00", zurich, "2026-11-26T20:59:00Z", false, ""},           // 21:59 local
		{"at the end", "22:00-09:00", zurich, "2026-11-27T08:00:00Z", false, ""},                      // 09:00 local
		{"same-day window", "13:00-14:30", "UTC", "2026-11-26T13:10:00Z", true, "2026-11-26T14:30:00Z"},
		{"empty window", "10:00-10:00", "UTC", "2026-11-26T10:00:00Z", false, ""},
		{"empty zone is UTC", "22:00-09:00", "", "2026-11-26T23:00:00Z", true, "2026-11-27T09:00:00Z"},
		// 2026-03-29 02:00 Zurich jumps to 03:00: the wall clock still ends at 09:00 (07:00 UTC).
		{"spring forward", "22:00-09:00", zurich, "2026-03-28T23:30:00Z", true, "2026-03-29T07:00:00Z"},
		// 2026-10-25 03:00 Zurich falls back to 02:00: the end stays 09:00 local (08:00 UTC).
		{"fall back", "22:00-09:00", zurich, "2026-10-25T00:30:00Z", true, "2026-10-25T08:00:00Z"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			now, _ := time.Parse(time.RFC3339, c.now)
			until, quiet, err := Schedule{Timezone: c.zone, QuietHours: c.window}.QuietUntil(now)
			if err != nil {
				t.Fatal(err)
			}
			if quiet != c.quiet {
				t.Fatalf("quiet = %v, want %v", quiet, c.quiet)
			}
			if quiet && until.UTC().Format(time.RFC3339) != c.until {
				t.Errorf("until = %s, want %s", until.UTC().Format(time.RFC3339), c.until)
			}
		})
	}
}

func TestQuietUntilRejectsBadInput(t *testing.T) {
	t.Parallel()
	for _, s := range []Schedule{{QuietHours: "late"}, {QuietHours: "25:00-09:00"}, {QuietHours: "22:00-09:00xyz"}, {QuietHours: "22:00--5:00"}, {Timezone: "Mars/Base", QuietHours: "22:00-09:00"}} {
		if _, _, err := s.QuietUntil(time.Now()); err == nil {
			t.Errorf("%+v: want an error", s)
		}
	}
}
