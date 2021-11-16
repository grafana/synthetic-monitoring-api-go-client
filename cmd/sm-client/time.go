package main

import "time"

func formatSMTime(t float64) string {
	return time.Unix(int64(t), 0).Format(time.RFC3339)
}
