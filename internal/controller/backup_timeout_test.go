/*
Copyright 2026 Paperclip Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"testing"
	"time"
)

func TestParseBackupTimeout(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{
			name:     "empty string returns default",
			input:    "",
			expected: 30 * time.Minute,
		},
		{
			name:     "valid 15 minutes",
			input:    "15m",
			expected: 15 * time.Minute,
		},
		{
			name:     "valid 1 hour",
			input:    "1h",
			expected: 1 * time.Hour,
		},
		{
			name:     "valid 2 hours",
			input:    "2h",
			expected: 2 * time.Hour,
		},
		{
			name:     "valid 30 minutes (default value explicitly)",
			input:    "30m",
			expected: 30 * time.Minute,
		},
		{
			name:     "below minimum clamps to 5 minutes",
			input:    "1m",
			expected: 5 * time.Minute,
		},
		{
			name:     "zero clamps to 5 minutes",
			input:    "0s",
			expected: 5 * time.Minute,
		},
		{
			name:     "above maximum clamps to 24 hours",
			input:    "48h",
			expected: 24 * time.Hour,
		},
		{
			name:     "exactly at minimum boundary",
			input:    "5m",
			expected: 5 * time.Minute,
		},
		{
			name:     "exactly at maximum boundary",
			input:    "24h",
			expected: 24 * time.Hour,
		},
		{
			name:     "invalid string returns default",
			input:    "invalid",
			expected: 30 * time.Minute,
		},
		{
			name:     "negative duration clamps to minimum",
			input:    "-10m",
			expected: 5 * time.Minute,
		},
		{
			name:     "mixed duration format",
			input:    "1h30m",
			expected: 90 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBackupTimeout(tt.input)
			if got != tt.expected {
				t.Errorf("parseBackupTimeout(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
