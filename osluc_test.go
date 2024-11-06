package main

import (
	"testing"
	"time"
)

func TestFileTimeToGoTime(t *testing.T) {
	t.Parallel()

	testString := "133753723630000000"
	testTime := time.Date(2024, time.November, 6, 13, 12, 43, 0, time.UTC)
	resultTime, err := FileTimeToGoTime(testString)

	if err != nil {
		t.Fatalf(`Error %s`, err)
	}

	if resultTime != testTime {
		t.Fatalf(`FileTimeToGoTime(%v) = %s, want %s`, nil, resultTime, testTime)
	}
}
