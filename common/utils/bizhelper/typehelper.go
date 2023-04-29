package bizhelper

import "math"

func Int64P(i int64) *int64 {
	return &i
}

func BoolP(b bool) *bool {
	return &b
}

func StrP(i string) *string {
	return &i
}

func Float64P(i float64) *float64 {
	return &i
}
func Float64PWithFixed(i float64) *float64 {
	i = math.Round(i*100) / 100
	return &i
}

func GetInt64ValueOr(raw *int64, value int64) int64 {
	if raw == nil {
		return value
	}
	return *raw
}

func GetStrValueOr(raw *string, value string) string {
	if raw == nil {
		return value
	}
	return *raw
}

func StrEmptyOr(raw *string, value string) string {
	if raw == nil {
		return value
	}

	if *raw == "" {
		return value
	}

	return *raw
}

func GetFloat64ValueOr(raw *float64, value float64) float64 {
	if raw == nil {
		return value
	}
	return *raw
}

func Str(raw *string) string {
	return GetStrValueOr(raw, "")
}

func Int64(raw *int64) int64 {
	return GetInt64ValueOr(raw, 0)
}

func Int(raw *int64) int {
	return int(Int64(raw))
}

func Int64P2Int(raw *int64) int {
	return int(Int64(raw))
}

func Float64(raw *float64) float64 {
	return GetFloat64ValueOr(raw, 0)
}

func IntToInt64P(i int) *int64 {
	return Int64P(int64(i))
}

func Bool(raw *bool) bool {
	if raw == nil {
		return false
	}
	return *raw
}
