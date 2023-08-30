package codec

import "strconv"

func Atoi(i string) int {
	raw, _ := strconv.Atoi(i)
	return raw
}

func Atof(i string) float64 {
	raw, _ := strconv.ParseFloat(i, 64)
	return raw
}

func Atob(i string) bool {
	raw, _ := strconv.ParseBool(i)
	return raw
}
