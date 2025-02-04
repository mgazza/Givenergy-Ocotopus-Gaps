// find_rate_test.go
package main

import (
	"testing"
	"time"
)

func floatPtr(f float64) *float64 {
	return &f
}

func TestFindRateForTime(t *testing.T) {
	tests := []struct {
		name   string
		time   time.Time
		rates  []TariffData
		expect *float64
	}{
		{
			name: "Match within range",
			time: time.Date(2025, 1, 1, 12, 15, 0, 0, time.UTC),
			rates: []TariffData{
				{Rate: 10.5, ValidFrom: ptrTime(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)), ValidTo: ptrTime(time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC))},
			},
			expect: floatPtr(10.5),
		},
		{
			name: "No match, before all ranges",
			time: time.Date(2025, 1, 1, 11, 45, 0, 0, time.UTC),
			rates: []TariffData{
				{Rate: 10.5, ValidFrom: ptrTime(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)), ValidTo: ptrTime(time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC))},
			},
			expect: nil,
		},
		{
			name: "No match, after all ranges",
			time: time.Date(2025, 1, 1, 12, 45, 0, 0, time.UTC),
			rates: []TariffData{
				{Rate: 10.5, ValidFrom: ptrTime(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)), ValidTo: ptrTime(time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC))},
			},
			expect: nil,
		},
		{
			name: "Multiple ranges, match in the middle",
			time: time.Date(2025, 1, 1, 12, 15, 0, 0, time.UTC),
			rates: []TariffData{
				{Rate: 5.0, ValidFrom: ptrTime(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)), ValidTo: ptrTime(time.Date(2025, 1, 1, 12, 10, 0, 0, time.UTC))},
				{Rate: 10.5, ValidFrom: ptrTime(time.Date(2025, 1, 1, 12, 10, 0, 0, time.UTC)), ValidTo: ptrTime(time.Date(2025, 1, 1, 12, 20, 0, 0, time.UTC))},
				{Rate: 7.5, ValidFrom: ptrTime(time.Date(2025, 1, 1, 12, 20, 0, 0, time.UTC)), ValidTo: ptrTime(time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC))},
			},
			expect: floatPtr(10.5),
		},
		{
			name:   "Empty rates list",
			time:   time.Date(2025, 1, 1, 12, 15, 0, 0, time.UTC),
			rates:  []TariffData{},
			expect: nil,
		},
		{
			name: "Open-ended rate",
			time: time.Date(2025, 1, 1, 12, 15, 0, 0, time.UTC),
			rates: []TariffData{
				{Rate: 10.5, ValidFrom: ptrTime(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)), ValidTo: nil},
			},
			expect: floatPtr(10.5),
		},
		{
			name: "Open-starting rate",
			time: time.Date(2025, 1, 1, 12, 15, 0, 0, time.UTC),
			rates: []TariffData{
				{Rate: 10.5, ValidFrom: nil, ValidTo: ptrTime(time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC))},
			},
			expect: floatPtr(10.5),
		},
		{
			name: "Fully open rate",
			time: time.Date(2025, 1, 1, 12, 15, 0, 0, time.UTC),
			rates: []TariffData{
				{Rate: 10.5, ValidFrom: nil, ValidTo: nil},
			},
			expect: floatPtr(10.5),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := findRateForTime(test.time, test.rates)

			if test.expect == nil && result != nil {
				t.Errorf("Test %s failed: expected nil, got %.2f", test.name, *result)
			} else if test.expect != nil && result == nil {
				t.Errorf("Test %s failed: expected %.2f, got nil", test.name, *test.expect)
			} else if test.expect != nil && result != nil && *test.expect != *test.expect {
				t.Errorf("Test %s failed: expected %.2f, got %v.2f", test.name, *test.expect, *result)
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
