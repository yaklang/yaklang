package sfvm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConstComparator_Matches(t *testing.T) {
	tests := []struct {
		name        string
		toCompared  string
		target      string
		condition   BinaryCondition
		expected    bool
		description string
	}{
		// Numeric comparison tests
		{
			name:        "Equal numbers - true",
			toCompared:  "42",
			target:      "42",
			condition:   BinaryConditionEqual,
			expected:    true,
			description: "Equal numbers should return true with == operator",
		},
		{
			name:        "Equal numbers - false",
			toCompared:  "42",
			target:      "24",
			condition:   BinaryConditionEqual,
			expected:    false,
			description: "Different numbers should return false with == operator",
		},
		{
			name:        "Not equal numbers - true",
			toCompared:  "42",
			target:      "24",
			condition:   BinaryConditionNotEqual,
			expected:    true,
			description: "Different numbers should return true with != operator",
		},
		{
			name:        "Not equal numbers - false",
			toCompared:  "42",
			target:      "42",
			condition:   BinaryConditionNotEqual,
			expected:    false,
			description: "Equal numbers should return false with != operator",
		},
		{
			name:        "Greater than - true",
			toCompared:  "25",
			target:      "50",
			condition:   BinaryConditionGt,
			expected:    true,
			description: "50 > 25 should return true",
		},
		{
			name:        "Greater than - false",
			toCompared:  "50",
			target:      "25",
			condition:   BinaryConditionGt,
			expected:    false,
			description: "25 > 50 should return false",
		},
		{
			name:        "Greater than or equal - true (greater)",
			toCompared:  "25",
			target:      "50",
			condition:   BinaryConditionGtEq,
			expected:    true,
			description: "50 >= 25 should return true",
		},
		{
			name:        "Greater than or equal - true (equal)",
			toCompared:  "50",
			target:      "50",
			condition:   BinaryConditionGtEq,
			expected:    true,
			description: "50 >= 50 should return true",
		},
		{
			name:        "Greater than or equal - false",
			toCompared:  "50",
			target:      "25",
			condition:   BinaryConditionGtEq,
			expected:    false,
			description: "25 >= 50 should return false",
		},
		{
			name:        "Less than - true",
			toCompared:  "50",
			target:      "25",
			condition:   BinaryConditionLt,
			expected:    true,
			description: "25 < 50 should return true",
		},
		{
			name:        "Less than - false",
			toCompared:  "25",
			target:      "50",
			condition:   BinaryConditionLt,
			expected:    false,
			description: "50 < 25 should return false",
		},
		{
			name:        "Less than or equal - true (less)",
			toCompared:  "50",
			target:      "25",
			condition:   BinaryConditionLtEq,
			expected:    true,
			description: "25 <= 50 should return true",
		},
		{
			name:        "Less than or equal - true (equal)",
			toCompared:  "50",
			target:      "50",
			condition:   BinaryConditionLtEq,
			expected:    true,
			description: "50 <= 50 should return true",
		},
		{
			name:        "Less than or equal - false",
			toCompared:  "25",
			target:      "50",
			condition:   BinaryConditionLtEq,
			expected:    false,
			description: "50 <= 25 should return false",
		},

		// Boolean comparison tests
		{
			name:        "Equal booleans - true",
			toCompared:  "true",
			target:      "true",
			condition:   BinaryConditionEqual,
			expected:    true,
			description: "true == true should return true",
		},
		{
			name:        "Equal booleans - false",
			toCompared:  "true",
			target:      "false",
			condition:   BinaryConditionEqual,
			expected:    false,
			description: "true == false should return false",
		},
		{
			name:        "Not equal booleans - true",
			toCompared:  "true",
			target:      "false",
			condition:   BinaryConditionNotEqual,
			expected:    true,
			description: "true != false should return true",
		},
		{
			name:        "Not equal booleans - false",
			toCompared:  "true",
			target:      "true",
			condition:   BinaryConditionNotEqual,
			expected:    false,
			description: "true != true should return false",
		},

		// String comparison tests
		{
			name:        "Equal strings - true",
			toCompared:  "hello",
			target:      "hello",
			condition:   BinaryConditionEqual,
			expected:    true,
			description: "Identical strings should return true with == operator",
		},
		{
			name:        "Equal strings - false",
			toCompared:  "hello",
			target:      "world",
			condition:   BinaryConditionEqual,
			expected:    false,
			description: "Different strings should return false with == operator",
		},
		{
			name:        "Not equal strings - true",
			toCompared:  "hello",
			target:      "world",
			condition:   BinaryConditionNotEqual,
			expected:    true,
			description: "Different strings should return true with != operator",
		},
		{
			name:        "Not equal strings - false",
			toCompared:  "hello",
			target:      "hello",
			condition:   BinaryConditionNotEqual,
			expected:    false,
			description: "Identical strings should return false with != operator",
		},
		{
			name:        "Greater than string - true",
			toCompared:  "zebra",
			target:      "apple",
			condition:   BinaryConditionGt,
			expected:    true,
			description: "Lexicographically 'zebra' > 'apple' should return true",
		},
		{
			name:        "Greater than string - false",
			toCompared:  "apple",
			target:      "zebra",
			condition:   BinaryConditionGt,
			expected:    false,
			description: "Lexicographically 'apple' > 'zebra' should return false",
		},
		{
			name:        "Greater than or equal string - true (greater)",
			toCompared:  "zebra",
			target:      "apple",
			condition:   BinaryConditionGtEq,
			expected:    true,
			description: "Lexicographically 'zebra' >= 'apple' should return true",
		},
		{
			name:        "Greater than or equal string - true (equal)",
			toCompared:  "apple",
			target:      "apple",
			condition:   BinaryConditionGtEq,
			expected:    true,
			description: "'apple' >= 'apple' should return true",
		},
		{
			name:        "Greater than or equal string - false",
			toCompared:  "apple",
			target:      "zebra",
			condition:   BinaryConditionGtEq,
			expected:    false,
			description: "Lexicographically 'apple' >= 'zebra' should return false",
		},
		{
			name:        "Less than string - true",
			toCompared:  "apple",
			target:      "zebra",
			condition:   BinaryConditionLt,
			expected:    true,
			description: "Lexicographically 'apple' < 'zebra' should return true",
		},
		{
			name:        "Less than string - false",
			toCompared:  "zebra",
			target:      "apple",
			condition:   BinaryConditionLt,
			expected:    false,
			description: "Lexicographically 'zebra' < 'apple' should return false",
		},
		{
			name:        "Less than or equal string - true (less)",
			toCompared:  "apple",
			target:      "zebra",
			condition:   BinaryConditionLtEq,
			expected:    true,
			description: "Lexicographically 'apple' <= 'zebra' should return true",
		},
		{
			name:        "Less than or equal string - true (equal)",
			toCompared:  "apple",
			target:      "apple",
			condition:   BinaryConditionLtEq,
			expected:    true,
			description: "'apple' <= 'apple' should return true",
		},
		{
			name:        "Less than or equal string - false",
			toCompared:  "zebra",
			target:      "apple",
			condition:   BinaryConditionLtEq,
			expected:    false,
			description: "Lexicographically 'zebra' <= 'apple' should return false",
		},

		// Mixed type tests
		{
			name:        "String that can't be parsed as number",
			toCompared:  "not-a-number",
			target:      "42",
			condition:   BinaryConditionEqual,
			expected:    false,
			description: "A string that can't be parsed as a number should be compared as strings",
		},
		{
			name:        "String with special characters compared lexicographically",
			toCompared:  "!special",
			target:      "special",
			condition:   BinaryConditionLt,
			expected:    true,
			description: "Strings with special characters should be compared lexicographically",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comparator := NewConstComparator(tt.toCompared, tt.condition)
			result := comparator.Matches(tt.target)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

// Test for nil comparator
func TestConstComparator_Nil(t *testing.T) {
	var comparator *ConstComparator
	assert.False(t, comparator.Matches("anything"), "Nil comparator should return false")
}

// Test for invalid binary condition
func TestConstComparator_InvalidCondition(t *testing.T) {
	comparator := NewConstComparator("42", "invalid condition")
	assert.False(t, comparator.Matches("42"), "Invalid binary condition should return false")
}

// Update the function parameter tests to reflect the new comparison order
func TestConstComparatorUsage(t *testing.T) {
	t.Run("Simple equality comparison", func(t *testing.T) {
		comparator := NewConstComparator("1", BinaryConditionEqual)
		assert.True(t, comparator.Matches("1"), "Comparing '1' == '1' should be true")
		assert.False(t, comparator.Matches("2"), "Comparing '2' == '1' should be false")
	})

	t.Run("Parameter count comparison", func(t *testing.T) {
		// Simulating comparing parameter count with 2
		comparator := NewConstComparator("2", BinaryConditionEqual)
		assert.True(t, comparator.Matches("2"), "Parameter count 2 == 2 should be true")
		assert.False(t, comparator.Matches("1"), "Parameter count 1 == 2 should be false")
	})

	t.Run("Numeric comparisons with different operators", func(t *testing.T) {
		// Less than
		ltComparator := NewConstComparator("10", BinaryConditionLt)
		assert.True(t, ltComparator.Matches("5"), "5 < 10 should be true")
		assert.False(t, ltComparator.Matches("15"), "15 < 10 should be false")

		// Greater than
		gtComparator := NewConstComparator("3", BinaryConditionGt)
		assert.True(t, gtComparator.Matches("5"), "5 > 3 should be true")
		assert.False(t, gtComparator.Matches("1"), "1 > 3 should be false")
	})
}
