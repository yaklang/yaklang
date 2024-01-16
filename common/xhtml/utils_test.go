package xhtml

import "testing"

func TestMatchBetween(t *testing.T) {
	testscases := []struct {
		src          string
		start, end   string
		max          int
		wantPosition int
		want         string
	}{
		{"123456789", "2", "6", -1, 2, "345"},
		{"123456789", "3", "7", -1, 3, "456"},
		{"123456789", "3", "9", 2, -1, ""},
		{"123456789", "a", "b", -1, -1, ""},
	}

	for _, testcase := range testscases {
		pos, got := MatchBetween(testcase.src, testcase.start, testcase.end, testcase.max)
		if pos != testcase.wantPosition || got != testcase.want {
			t.Errorf("MatchBetween(%v, %v, %v, %v) = %v, %v, want %v, %v",
				testcase.src, testcase.start, testcase.end, testcase.max,
				pos, got, testcase.wantPosition, testcase.want)
		}
	}
}
