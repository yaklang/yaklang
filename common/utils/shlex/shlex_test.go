/*
Copyright 2012 Google Inc. All Rights Reserved.

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

package shlex

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// one two "three four" "five \"six\"" seven#eight # nine # ten
// eleven 'twelve\'
var testString = "one two \"three four\" \"five \\\"six\\\"\" seven#eight # nine # ten\n eleven 'twelve\\' thirteen=13 fourteen/14"

func TestClassifier(t *testing.T) {
	classifier := newDefaultClassifier()
	tests := map[rune]runeTokenClass{
		' ':  spaceRuneClass,
		'"':  doubleQuoteRuneClass,
		'\'': singleQuoteRuneClass,
		'#':  commentRuneClass,
	}
	for runeChar, want := range tests {
		got := classifier.ClassifyRune(runeChar)
		require.Equal(t, want, got, "ClassifyRune(%v) -> %v. Want: %v", runeChar, got, want)
	}
}

func TestTokenizer(t *testing.T) {
	testInput := strings.NewReader(testString)
	expectedTokens := []*Token{
		{WordToken, "one"},
		{WordToken, "two"},
		{WordToken, "three four"},
		{WordToken, "five \"six\""},
		{WordToken, "seven#eight"},
		{CommentToken, " nine # ten"},
		{WordToken, "eleven"},
		{WordToken, "twelve\\"},
		{WordToken, "thirteen=13"},
		{WordToken, "fourteen/14"},
	}

	tokenizer := NewTokenizer(testInput)
	for i, want := range expectedTokens {
		got, err := tokenizer.Next()
		require.NoError(t, err)
		require.True(t, got.Equal(want), "Tokenizer.Next()[%v] of %q -> %v. Want: %v", i, testString, got, want)
	}
}

func TestLexer(t *testing.T) {
	testInput := strings.NewReader(testString)
	expectedStrings := []string{"one", "two", "three four", "five \"six\"", "seven#eight", "eleven", "twelve\\", "thirteen=13", "fourteen/14"}

	lexer := NewLexer(testInput)
	for i, want := range expectedStrings {
		got, err := lexer.Next()
		require.NoError(t, err)
		require.Equal(t, want, got, "Lexer.Next()[%v] of %q -> %v. Want: %v", i, testString, got, want)
	}
}

func TestSplit(t *testing.T) {
	want := []string{"one", "two", "three four", "five \"six\"", "seven#eight", "eleven", "twelve\\", "thirteen=13", "fourteen/14"}
	got, err := Split(testString)
	require.NoError(t, err)
	require.Len(t, got, len(want), "Split(%q) -> %v. Want: %v", testString, got, want)
	for i := range got {
		require.Equal(t, want[i], got[i], "Split(%q)[%v] -> %v. Want: %v", testString, i, got[i], want[i])
	}
}

func TestANSICQuotedSplit(t *testing.T) {
	testString := `echo $'\60\61'`
	want := []string{"echo", "01"}
	got, err := Split(testString)
	require.NoError(t, err)
	require.Len(t, got, len(want), "Split(%q) -> %v. Want: %v", testString, got, want)
	for i := range got {
		require.Equal(t, want[i], got[i], "Split(%q)[%v] -> %v. Want: %v", testString, i, got[i], want[i])
	}
}

// TestSingleQuotePreservesNewlines tests that single quotes preserve newlines and other characters
func TestSingleQuotePreservesNewlines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			// Single quotes preserve everything including newlines
			name:     "single quotes with newline",
			input:    "bash -c 'echo hello\necho world'",
			expected: []string{"bash", "-c", "echo hello\necho world"},
		},
		{
			// Single quotes preserve backslash-n as two characters
			name:     "single quotes with backslash n",
			input:    `bash -c 'echo hello\necho world'`,
			expected: []string{"bash", "-c", `echo hello\necho world`},
		},
		{
			// Real multiline bash command
			name:     "multiline bash command",
			input:    "bash -c 'echo \"=== 系统信息 ===\";\nsw_vers -productVersion;\necho -e \"\\n=== 架构 ===\";\nuname -m'",
			expected: []string{"bash", "-c", "echo \"=== 系统信息 ===\";\nsw_vers -productVersion;\necho -e \"\\n=== 架构 ===\";\nuname -m"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Split(tt.input)
			require.NoError(t, err, "Split(%q) failed: %v", tt.input, err)
			require.Equal(t, tt.expected, got, "Split(%q) = %v, want %v", tt.input, got, tt.expected)
		})
	}
}

// TestDoubleQuoteEscapePreservation tests that backslashes are preserved for
// characters that cannot be escaped in double quotes (POSIX compliance)
func TestDoubleQuoteEscapePreservation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			// Valid escape: \" -> "
			name:     "valid escape double quote",
			input:    `echo "hello\"world"`,
			expected: []string{"echo", `hello"world`},
		},
		{
			// Valid escape: \\ -> \
			name:     "valid escape backslash",
			input:    `echo "hello\\world"`,
			expected: []string{"echo", `hello\world`},
		},
		{
			// Valid escape: \$ -> $
			name:     "valid escape dollar",
			input:    `echo "hello\$world"`,
			expected: []string{"echo", `hello$world`},
		},
		{
			// Valid escape: \` -> `
			name:     "valid escape backtick",
			input:    "echo \"hello\\`world\"",
			expected: []string{"echo", "hello`world"},
		},
		{
			// Invalid escape: \n should preserve backslash (this is the bug fix)
			name:     "invalid escape n preserves backslash",
			input:    `echo "hello\nworld"`,
			expected: []string{"echo", `hello\nworld`},
		},
		{
			// Invalid escape: \t should preserve backslash
			name:     "invalid escape t preserves backslash",
			input:    `echo "hello\tworld"`,
			expected: []string{"echo", `hello\tworld`},
		},
		{
			// Invalid escape: \r should preserve backslash
			name:     "invalid escape r preserves backslash",
			input:    `echo "hello\rworld"`,
			expected: []string{"echo", `hello\rworld`},
		},
		{
			// Mixed: bash -c with escaped newlines
			name:     "bash command with escaped newlines",
			input:    `bash -c "echo hello\necho world"`,
			expected: []string{"bash", "-c", `echo hello\necho world`},
		},
		{
			// Real case: multiple commands with \n separators
			name:     "multiple commands separated by backslash n",
			input:    `bash -c "echo 'test';\nps aux"`,
			expected: []string{"bash", "-c", `echo 'test';\nps aux`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Split(tt.input)
			require.NoError(t, err, "Split(%q) failed: %v", tt.input, err)
			require.Equal(t, tt.expected, got, "Split(%q) = %v, want %v", tt.input, got, tt.expected)
		})
	}
}
