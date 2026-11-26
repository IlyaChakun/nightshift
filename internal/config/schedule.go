package config

import (
	"fmt"
	"regexp"
	"time"
)

// quietRe is the shape the tenant schema allows.
var quietRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):([0-5][0-9])-([01][0-9]|2[0-3]):([0-5][0-9])$`)

func atoi(s string) int { return int(s[0]-'0')*10 + int(s[1]-'0') }

// Schedule is the tenant's clock: its time zone and when the factory stays quiet (design section 17).
type Schedule struct {
	Timezone       string `yaml:"timezone"`
	MorningSummary string `yaml:"morning_summary"` // "09:00"; the morning digest goes out after it
	// QuietHours is "HH:MM-HH:MM" in the tenant's time zone and may cross midnight ("22:00-09:00").
	// Empty means no quiet hours.
	QuietHours string `yaml:"quiet_hours"`
	// ReleasesInQuietHours lets a production release run in quiet hours; the default is no.
	ReleasesInQuietHours bool `yaml:"releases_in_quiet_hours"`
}

// QuietUntil reports whether now is inside the quiet hours and, if so, when they end. A window
// whose start equals its end is empty. The wall clock decides, so a daylight-saving change moves
// the window with it.
func (s Schedule) QuietUntil(now time.Time) (time.Time, bool, error) {
	if s.QuietHours == "" {
		return time.Time{}, false, nil
	}
	m := quietRe.FindStringSubmatch(s.QuietHours)
	if m == nil {
		return time.Time{}, false, fmt.Errorf("config: quiet_hours %q: want HH:MM-HH:MM", s.QuietHours)
	}
	sh, sm, eh, em := atoi(m[1]), atoi(m[2]), atoi(m[3]), atoi(m[4])
	name := s.Timezone
	if name == "" {
		name = "UTC"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("config: timezone %q: %w", name, err)
	}
	local := now.In(loc)
	at := func(day time.Time, h, m int) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, loc)
	}
	start, end := sh*60+sm, eh*60+em
	cur := local.Hour()*60 + local.Minute()
	switch {
	case start == end:
		return time.Time{}, false, nil
	case start < end && cur >= start && cur < end: // same-day window
		return at(local, eh, em), true, nil
	case start > end && cur >= start: // evening part of a window across midnight
		return at(local.AddDate(0, 0, 1), eh, em), true, nil
	case start > end && cur < end: // morning part
		return at(local, eh, em), true, nil
	}
	return time.Time{}, false, nil
}
