package app

import (
	"testing"
	"time"
)

func TestParseIntervalDuration(t *testing.T) {
	type durationTest struct {
		invalid        bool
		duration       string
		expectDuration time.Duration
	}

	tests := []durationTest{
		{
			duration:       `1s`,
			expectDuration: 1 * time.Second,
		},
		{
			duration:       `10s`,
			expectDuration: 10 * time.Second,
		},
		{
			duration:       `1000s`,
			expectDuration: 1000 * time.Second,
		},
		{
			duration:       `1m`,
			expectDuration: 1 * time.Minute,
		},
		{
			invalid:  true,
			duration: `-1s`,
		},
	}
	for _, test := range tests {
		durationStr, err := validateDuration(test.duration)
		if test.invalid {
			if err == nil {
				t.Logf("Expected %s to fail, but it did not", test.duration)
				t.Fail()
			}
		} else {
			if err != nil {
				t.Error(err)
			}
		}

		// Should always successfully parse valid duration
		duration, err := parseDuration(durationStr)
		if err != nil {
			t.Error(err)
		}
		if duration != test.expectDuration {
			t.Logf("Incorrect duration parsed: expected \"%s\", got \"%s\".", test.expectDuration, duration)
			t.Fail()
		}
	}
}
