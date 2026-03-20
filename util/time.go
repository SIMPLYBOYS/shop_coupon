package util

import "time"

func GetSpecificTime(hour, minute, second int) time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, second, 0, time.UTC)
}
