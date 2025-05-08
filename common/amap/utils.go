package amap

import (
	"errors"
	"strconv"
	"strings"
)

func parseLocation(s string) (float64, float64, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid location format, expected 'longitude,latitude'")
	}

	longitude, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, 0, err
	}

	latitude, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, 0, err
	}

	return longitude, latitude, nil
}
