package server

import (
	"testing"
	"time"
)

func TestParseRelativeTime(t *testing.T) {
	// Use a fixed base time for consistent testing
	baseTime := time.Date(2025, 8, 4, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		input       string
		expected    time.Time
		shouldError bool
	}{
		{
			name:     "Unix timestamp",
			input:    "1753500964",
			expected: time.Unix(1753500964, 0),
		},
		{
			name:     "3 days ago",
			input:    "-3d",
			expected: baseTime.Add(-3 * 24 * time.Hour),
		},
		{
			name:     "2 hours ago",
			input:    "-2h",
			expected: baseTime.Add(-2 * time.Hour),
		},
		{
			name:     "30 minutes ago",
			input:    "-30m",
			expected: baseTime.Add(-30 * time.Minute),
		},
		{
			name:     "5 seconds ago",
			input:    "-5s",
			expected: baseTime.Add(-5 * time.Second),
		},
		{
			name:     "3 days in future",
			input:    "3d",
			expected: baseTime.Add(3 * 24 * time.Hour),
		},
		{
			name:     "1 hour in future",
			input:    "1h",
			expected: baseTime.Add(1 * time.Hour),
		},
		{
			name:        "empty string",
			input:       "",
			shouldError: true,
		},
		{
			name:        "invalid format",
			input:       "invalid",
			shouldError: true,
		},
		{
			name:        "invalid unit",
			input:       "3x",
			shouldError: true,
		},
		{
			name:        "no number",
			input:       "-d",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseRelativeTime(tt.input, baseTime)

			if tt.shouldError {
				if err == nil {
					t.Errorf("parseRelativeTime(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("parseRelativeTime(%q) unexpected error: %v", tt.input, err)
				return
			}

			if !result.Equal(tt.expected) {
				t.Errorf("parseRelativeTime(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseRelativeTimeEdgeCases(t *testing.T) {
	baseTime := time.Date(2025, 8, 4, 12, 0, 0, 0, time.UTC)

	// Test large values
	result, err := parseRelativeTime("365d", baseTime)
	if err != nil {
		t.Errorf("parseRelativeTime('365d') unexpected error: %v", err)
	}
	expected := baseTime.Add(365 * 24 * time.Hour)
	if !result.Equal(expected) {
		t.Errorf("parseRelativeTime('365d') = %v, expected %v", result, expected)
	}

	// Test zero values
	result, err = parseRelativeTime("0s", baseTime)
	if err != nil {
		t.Errorf("parseRelativeTime('0s') unexpected error: %v", err)
	}
	if !result.Equal(baseTime) {
		t.Errorf("parseRelativeTime('0s') = %v, expected %v", result, baseTime)
	}
}
