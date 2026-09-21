package main

import (
	"testing"
	"time"
)

func TestRunDayRangeStopsAtFirstIncompleteDay(t *testing.T) {
	start := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	var visited []string
	runDayRange(start, start.AddDate(0, 0, 3), func(day time.Time) bool {
		visited = append(visited, day.Format("2006-01-02"))
		return day.Before(start.AddDate(0, 0, 1))
	})
	if len(visited) != 2 || visited[1] != "2026-09-11" {
		t.Fatalf("range advanced past failed day: %#v", visited)
	}
}
