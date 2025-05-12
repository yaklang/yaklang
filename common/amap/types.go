package amap

import (
	"fmt"
	"strconv"
	"strings"
)

// BaseResponse represents the common fields in all Amap API responses.
type BaseResponse struct {
	Status   string `json:"status"`
	Info     string `json:"info"`
	InfoCode string `json:"infocode"`
}

// IsSuccess returns true if the API call was successful.
func (r BaseResponse) IsSuccess() bool {
	return r.Status == "1"
}

// Error implements the error interface for API response errors.
func (r BaseResponse) Error() string {
	return "amap API error: " + r.Info
}

// Location represents a geographical point with latitude and longitude.
type Location struct {
	Longitude float64 `json:"longitude,omitempty"`
	Latitude  float64 `json:"latitude,omitempty"`
}

// String returns the location formatted as longitude,latitude.
func (l Location) String() string {
	return fmt.Sprintf("%.6f,%.6f", l.Longitude, l.Latitude)
}

// LatLngString returns the location formatted as latitude,longitude.
func (l Location) LatLngString() string {
	return fmt.Sprintf("%.6f,%.6f", l.Latitude, l.Longitude)
}

// LocationFromString parses a string in the format "longitude,latitude" into a Location.
func LocationFromString(s string) (Location, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return Location{}, fmt.Errorf("invalid location format: %s", s)
	}

	lng, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return Location{}, fmt.Errorf("invalid longitude: %s", parts[0])
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return Location{}, fmt.Errorf("invalid latitude: %s", parts[1])
	}

	return Location{Longitude: lng, Latitude: lat}, nil
}
