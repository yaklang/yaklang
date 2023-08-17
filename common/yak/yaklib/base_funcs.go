package yaklib

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"strconv"
)

func parseInt(s string) int {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		log.Errorf("parse int[%s] failed: %s", s, err)
		return 0
	}
	return int(i)
}

func parseFloat(s string) float64 {
	i, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Errorf("parse float[%s] failed: %s", s, err)
		return 0
	}
	return float64(i)
}

func parseString(i interface{}) string {
	return fmt.Sprintf("%v", i)
}

func parseBool(i interface{}) bool {
	r, _ := strconv.ParseBool(fmt.Sprint(i))
	return r
}

func _input(s ...string) string {
	var input string
	if len(s) > 0 {
		fmt.Print(s[0])
	}
	n, err := fmt.Scanln(&input)
	if err != nil && n != 0 {
		panic(err)
	}
	return input
}
