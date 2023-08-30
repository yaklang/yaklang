package numrange

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"strconv"
	"strings"
)

type NumRange struct {
	// Op can be '>' '<' '=' '-' '!'.
	// they are larger,smaller,equal,between,not
	Op   int
	Num1 int
	Num2 int
}

func ParseNumRange(s string) (r *NumRange, err error) {
	if len(s) == 0 {
		return nil, errors.New("empty string")
	}

	switch s[0] {
	case '<':
		if len(s) < 2 {
			return nil, errors.New("no char after '<'")
		}
		num, err := strconv.Atoi(s[1:])
		if err != nil {
			return nil, errors.Wrap(err, "invalid number after '<'")
		}
		return &NumRange{Op: '<', Num1: num}, nil
	case '>':
		if len(s) < 2 {
			return nil, errors.New("no char after '>'")
		}
		num, err := strconv.Atoi(s[1:])
		if err != nil {
			return nil, errors.Wrap(err, "invalid number after '>'")
		}
		return &NumRange{Op: '>', Num1: num}, nil
	case '!':
		if len(s) < 2 {
			return nil, errors.New("no char after '!'")
		}
		num, err := strconv.Atoi(s[1:])
		if err != nil {
			return nil, errors.Wrap(err, "invalid number after '!'")
		}
		return &NumRange{Op: '!', Num1: num}, nil
	default:
		if !(s[0] >= '0' && s[0] <= '9') {
			return nil, errors.New("invalid string")
		}
	}

	if num, err := strconv.Atoi(s); err == nil {
		return &NumRange{Op: '=', Num1: num}, nil
	}

	if idx := strings.IndexByte(s, '-'); idx != -1 {
		if idx >= len(s)-1 {
			return nil, errors.New("no number afterr '-'")
		}

		num1, err := strconv.Atoi(s[:idx])
		if err != nil {
			return nil, errors.Wrap(err, "invalid number before '-'")
		}

		num2, err := strconv.Atoi(s[idx+1:])
		if err != nil {
			return nil, errors.Wrap(err, "invalid number bedore '-'")
		}

		return &NumRange{Op: '-', Num1: num1, Num2: num2}, nil
	}

	if idx := strings.Index(s, "<>"); idx != -1 {
		if idx >= len(s)-2 {
			return nil, errors.New("no number after \"<>\"")
		}

		num1, err := strconv.Atoi(s[:idx])
		if err != nil {
			return nil, errors.Wrap(err, "invalid number before \"<>\"")
		}

		num2, err := strconv.Atoi(s[idx+2:])
		if err != nil {
			return nil, errors.Wrap(err, "invalid number after \"<>\"")
		}

		return &NumRange{Op: '-', Num1: num1, Num2: num2}, nil
	}

	return nil, errors.New("invalid string")
}

func (r *NumRange) Match(num int) bool {
	switch r.Op {
	case '>':
		return num > r.Num1
	case '<':
		return num < r.Num1
	case '=':
		return num == r.Num1
	case '-':
		return num >= r.Num1 && num <= r.Num2
	case '!':
		return num != r.Num1
	}
	return false
}

func (r *NumRange) Generate() int {
	switch r.Op {
	case '>':
		return r.Num1 + rand.Intn(utils.Max(r.Num1/5, 20))
	case '<':
		return rand.Intn(r.Num1)
	case '=':
		return r.Num1
	case '-':
		return r.Num1 + rand.Intn(r.Num2-r.Num1)
	case '!':
		n := rand.Intn(1023)
		if n >= r.Num1 {
			return n + 1
		}
		return n
	}
	return rand.Intn(1024)
}
