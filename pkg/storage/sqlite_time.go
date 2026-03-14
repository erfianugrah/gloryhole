package storage

import (
	"strconv"
	"strings"
	"time"
)

func parseSQLiteTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}

	// Strip Go's monotonic clock suffix (e.g. " m=+0.005160883")
	// This appears when MIN()/MAX() operates on Go time.Time values stored as text
	if idx := strings.Index(value, " m="); idx > 0 {
		value = value[:idx]
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
		// Go time.Time.String() format: "2006-01-02 15:04:05.999999999 +0100 CET"
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts
		}
	}
	return time.Time{}
}

func parseUnixString(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	secs, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(secs, 0).UTC(), nil
}
