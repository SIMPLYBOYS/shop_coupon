package util

import "time"

func GetSpecificTime(hour, minute, second int) time.Time {
	specificTime := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), hour, minute, second, 0, time.UTC)
	return specificTime
}
