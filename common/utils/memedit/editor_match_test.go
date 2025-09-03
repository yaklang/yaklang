package memedit

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindStringRange(t *testing.T) {
	editor := NewMemEditor("Hello, world! This is a test string. Welcome to the world of Go.你好世界,你好Golang")

	tests := []struct {
		name     string
		feature  string
		expected []string // expected outputs are the substrings matched
		wantErr  bool
	}{
		{"Find existing string", "world", []string{"world", "world"}, false},
		{"Find utf8 string", "你好", []string{"你好", "你好"}, false},
		{"Find non-existing string", "nonexistent", nil, false},
		{"Find at the beginning", "Hello", []string{"Hello"}, false},
		{"Find at the end", "Go.", []string{"Go."}, false},
		{"Find case sensitive", "hello", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var results []string
			err := editor.FindStringRange(tt.feature, func(r *Range) error {
				start, end := r.GetStart(), r.GetEnd()
				results = append(results, editor.GetTextFromPosition(start, end))
				return nil
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("FindStringRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(results, tt.expected) {
				t.Errorf("FindStringRange() got = %v, want %v", results, tt.expected)
			}
		})
	}
}

func TestFindStringRangeShort(t *testing.T) {
	editor := NewMemEditor("a")
	feature := "abc"
	var results []string
	err := editor.FindStringRange(feature, func(r *Range) error {
		start, end := r.GetStart(), r.GetEnd()
		results = append(results, editor.GetTextFromPosition(start, end))
		return nil
	})
	require.NoError(t, err)
	require.Nil(t, results)
}

func TestFindRegexpRange_Edge(t *testing.T) {
	editor := NewMemEditor("Hello, world! 123 This is a test string. Welcome to the world of Go 456. Numbers 789, 012")

	tests := []struct {
		name     string
		pattern  string
		expected []string // expected outputs are the substrings matched
		wantErr  bool
	}{
		{"Find simple regex", `\bworld\b`, []string{"world", "world"}, false},
		{"Find digits", `\d+`, []string{"123", "456", "789", "012"}, false},
		{"Regex no match", `xyz`, nil, false},
		{"Invalid regex pattern", `[`, nil, true},
		{"Match at string start", `^Hello`, []string{"Hello"}, false},
		{"Match at string end", `012$`, []string{"012"}, false},
		{"Complex pattern", `(\d+)\s*,\s*(\d+)`, []string{"789, 012"}, false},
		{"Overlapping matches", `o`, []string{"o", "o", "o", "o", "o", "o", "o"}, false},
		{"Case sensitivity", `HELLO`, nil, false},
		{"Whole words only", `\btest\b`, []string{"test"}, false},
		{"Multi-line search", `\bGo\b.*\bNumbers\b`, []string{"Go 456. Numbers"}, false},
		{"Include newline", `\bis\b.*\bto\b`, []string{"is a test string. Welcome to"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var results []string
			err := editor.FindRegexpRange(tt.pattern, func(r *Range) error {
				start, end := r.GetStart(), r.GetEnd()
				// Assuming GetTextFromPosition(start, end) retrieves the text between start and end position
				results = append(results, editor.GetTextFromPosition(start, end))
				return nil
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("FindRegexpRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(results, tt.expected) {
				t.Errorf("FindRegexpRange() got = %v, want %v", results, tt.expected)
			}
		})
	}
}

func TestFindRegexpRange(t *testing.T) {
	editor := NewMemEditor("Hello, world! 123 This is a test string. Welcome to the world of Go 456.你好世界,你好中国")

	tests := []struct {
		name     string
		pattern  string
		expected []string // expected outputs are the substrings matched
		wantErr  bool
	}{
		{"Find simple regex", `\bworld\b`, []string{"world", "world"}, false},
		{"Find utf8", `你好`, []string{"你好", "你好"}, false},
		{"Find digits", `\d+`, []string{"123", "456"}, false},
		{"Regex no match", `xyz`, nil, false},
		{"Invalid regex pattern", `[`, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var results []string
			err := editor.FindRegexpRange(tt.pattern, func(r *Range) error {
				start, end := r.GetStart(), r.GetEnd()
				results = append(results, editor.GetTextFromPosition(start, end))
				return nil
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("FindRegexpRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(results, tt.expected) {
				t.Errorf("FindRegexpRange() got = %v, want %v", results, tt.expected)
			}
		})
	}
}

func TestGetContextAroundRange(t *testing.T) {
	editor := NewMemEditor("Hello, world!\nThis is a test string.\nWelcome to the world of Go.\nAnother line here.")

	startPos := NewPosition(1, 0) // Start of first line
	endPos := NewPosition(1, 12)  // End of "Hello, world!"

	tests := []struct {
		name     string
		startPos *Position
		endPos   *Position
		n        int
		expected string
		wantErr  bool
	}{
		{"Context with 1 line", startPos, endPos, 1, "Hello, world!\nThis is a test string.\n", false},
		{"Context with 0 line", startPos, endPos, 0, "Hello, world!\n", false},
		{"Context out of bounds", startPos, endPos, 100, "Hello, world!\nThis is a test string.\nWelcome to the world of Go.\nAnother line here.\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := editor.GetContextAroundRange(tt.startPos, tt.endPos, tt.n)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetContextAroundRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if result != tt.expected {
				t.Errorf("GetContextAroundRange() got = %q, want %q", result, tt.expected)
			}
		})
	}
}
