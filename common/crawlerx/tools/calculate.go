// Package tools
// @Author bcy2007  2025/9/25 11:27
package tools

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var calculateReg = regexp.MustCompile(`(\d+)\s*([+\-_×xX])\s*(\d+)(\=\?)?`)

func GetCalculateResult(formula string) (string, error) {
	formula = strings.TrimSpace(formula)
	matches := calculateReg.FindStringSubmatch(formula)
	if len(matches) < 4 {
		return "", fmt.Errorf("invalid formula format: %s", formula)
	}

	leftStr := matches[1]
	operator := matches[2]
	rightStr := matches[3]
	left, err := strconv.Atoi(leftStr)
	if err != nil {
		return "", fmt.Errorf("invalid left operand: %s", leftStr)
	}
	right, err := strconv.Atoi(rightStr)
	if err != nil {
		return "", fmt.Errorf("invalid right operand: %s", rightStr)
	}

	var result int
	switch operator {
	case "+":
		result = left + right
	case "-", "_":
		result = left - right
	case "×", "x", "X":
		result = left * right
	default:
		return "", fmt.Errorf("unsupported operator: %s", operator)
	}
	return strconv.Itoa(result), nil
}
