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
