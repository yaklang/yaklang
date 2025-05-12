package amap

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// IPLocationResultEx contains IP geolocation information
type IPLocationResultEx struct {
	IP        string
	Province  string
	City      string
	AdCode    string
	Rectangle string
}

// POIResultEx contains simplified POI information
type POIResultEx struct {
	Name         string
	Address      string
	Location     Location
	Distance     string
	Type         string
	TelNumber    string
	Province     string
	City         string
	District     string
	POIId        string
	AdCode       string
	BusinessArea string
}

// SearchPOIResultEx contains search result information
type SearchPOIResultEx struct {
	Total       int
	Results     []POIResultEx
	SugKeywords []string
	SugCities   []string
}

// AOIBoundaryResultEx contains AOI boundary information
type AOIBoundaryResultEx struct {
	Name       string
	ID         string
	Type       string
	Address    string
	Location   Location
	Province   string
	City       string
	District   string
	AdCode     string
	Polyline   string     // Boundary coordinates
	PolyPoints []Location // Parsed boundary points
}

// Geocode converts an address to coordinates
func Geocode(address string, options ...AmapConfigOption) ([]*GeocodeResult, error) {
	config := NewConfig(options...)
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	req := GeocodingRequest{
		Address: address,
	}

	if config.City != "" {
		req.City = config.City
	}

	resp, err := client.Geocode(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Geocodes) == 0 {
		return nil, fmt.Errorf("no geocode results found")
	}
	return resp.Geocodes, nil
}

// ReverseGeocode converts coordinates to an address
func ReverseGeocode(longitude, latitude float64, options ...AmapConfigOption) (*RegeoCodeResult, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	location := Location{Longitude: longitude, Latitude: latitude}
	req := ReverseGeocodingRequest{
		Location: location,
	}

	if config.Extensions != "" {
		req.Extensions = config.Extensions
	}

	resp, err := client.ReverseGeocode(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.RegeoCodes == nil {
		return nil, fmt.Errorf("no reverse geocode results found")
	}

	return resp.RegeoCodes, nil
}

// DrivingDrivingPlan calculates a driving route
func DrivingPlan(origin, destination string, options ...AmapConfigOption) (*DirectionResponse, error) {
	originGeocodes, err := Geocode(origin, options...)
	if err != nil {
		return nil, err
	}
	destinationGeocodes, err := Geocode(destination, options...)
	if err != nil {
		return nil, err
	}

	config := NewConfig(options...)
	originGeocode := config.GeocodeFilter(originGeocodes)
	destinationGeocode := config.GeocodeFilter(destinationGeocodes)
	originLocation, err := LocationFromString(originGeocode.Location)
	if err != nil {
		return nil, err
	}
	destinationLocation, err := LocationFromString(destinationGeocode.Location)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	req := DrivingRequest{
		DirectionRequest: DirectionRequest{
			Origin:      originLocation,
			Destination: destinationLocation,
		},
		Strategy: RouteStrategyFastest,
	}

	if config.Extensions != "" {
		req.OutputExtensions = config.Extensions
	}

	return client.Driving(ctx, req)
}

// WalkingPlan calculates a walking route from address strings
func WalkingPlan(origin, destination string, options ...AmapConfigOption) (*V5WalkingResponse, error) {
	originGeocodes, err := Geocode(origin, options...)
	if err != nil {
		return nil, err
	}
	destinationGeocodes, err := Geocode(destination, options...)
	if err != nil {
		return nil, err
	}

	config := NewConfig(options...)
	originGeocode := config.GeocodeFilter(originGeocodes)
	destinationGeocode := config.GeocodeFilter(destinationGeocodes)
	originLocation, err := LocationFromString(originGeocode.Location)
	if err != nil {
		return nil, err
	}
	destinationLocation, err := LocationFromString(destinationGeocode.Location)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	req := V5WalkingRequest{
		Origin:      originLocation,
		Destination: destinationLocation,
	}

	if config.Extensions != "" {
		req.Extensions = config.Extensions
	}

	return client.V5Walking(ctx, req)
}

// BicyclingPlan calculates a bicycling route from address strings
func BicyclingPlan(origin, destination string, options ...AmapConfigOption) (*BicyclingResult, error) {
	originGeocodes, err := Geocode(origin, options...)
	if err != nil {
		return nil, err
	}
	destinationGeocodes, err := Geocode(destination, options...)
	if err != nil {
		return nil, err
	}

	config := NewConfig(options...)
	originGeocode := config.GeocodeFilter(originGeocodes)
	destinationGeocode := config.GeocodeFilter(destinationGeocodes)
	originLocation, err := LocationFromString(originGeocode.Location)
	if err != nil {
		return nil, err
	}
	destinationLocation, err := LocationFromString(destinationGeocode.Location)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	req := BicyclingRequest{
		DirectionRequest: DirectionRequest{
			Origin:      originLocation,
			Destination: destinationLocation,
		},
	}

	if config.Extensions != "" {
		req.OutputExtensions = config.Extensions
	}

	resp, err := client.Bicycling(ctx, req)
	if err != nil {
		return nil, err
	}

	return &resp.Data, nil
}

// TransitPlan calculates a transit route from address strings
func TransitPlan(origin, destination string, options ...AmapConfigOption) (*TransitResponse, error) {
	originGeocodes, err := Geocode(origin, options...)
	if err != nil {
		return nil, err
	}
	destinationGeocodes, err := Geocode(destination, options...)
	if err != nil {
		return nil, err
	}

	config := NewConfig(options...)
	originGeocode := config.GeocodeFilter(originGeocodes)
	destinationGeocode := config.GeocodeFilter(destinationGeocodes)
	originLocation, err := LocationFromString(originGeocode.Location)
	if err != nil {
		return nil, err
	}
	destinationLocation, err := LocationFromString(destinationGeocode.Location)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	req := TransitRequest{
		DirectionRequest: DirectionRequest{
			Origin:      originLocation,
			Destination: destinationLocation,
		},
		City:   originGeocode.CityCode,
		CitydD: destinationGeocode.CityCode,
	}

	if config.Extensions != "" {
		req.OutputExtensions = config.Extensions
	}

	return client.Transit(ctx, req)
}

// Distance calculates distance between multiple origins and one destination
func Distance(origin, destination string, options ...AmapConfigOption) (*DistanceResult, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	originGeocodes, err := Geocode(origin, options...)
	if err != nil {
		return nil, err
	}

	originGeocode := config.GeocodeFilter(originGeocodes)
	originLocation, err := LocationFromString(originGeocode.Location)
	if err != nil {
		return nil, err
	}
	destinationGeocodes, err := Geocode(destination, options...)
	if err != nil {
		return nil, err
	}
	destinationGeocode := config.GeocodeFilter(destinationGeocodes)
	destinationLocation, err := LocationFromString(destinationGeocode.Location)
	if err != nil {
		return nil, err
	}
	req := DistanceRequest{
		Origins:     []Location{originLocation},
		Destination: destinationLocation,
		Type:        DirectionType(config.Type),
	}
	distance, err := client.Distance(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(distance.Results) == 0 {
		return nil, fmt.Errorf("no distance results found")
	}
	return &distance.Results[0], nil
}

// IPLocation locates an IP address or the requester's IP if ip is empty
func IPLocation(ip string, options ...AmapConfigOption) (*IPLocationResultEx, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	resp, err := client.IPLocationV3(ctx, IPLocationRequest{
		IP: ip,
	})
	if err != nil {
		return nil, err
	}

	return &IPLocationResultEx{
		IP:        ip,
		Province:  resp.Province,
		City:      resp.City,
		AdCode:    resp.AdCode,
		Rectangle: resp.Rectangle,
	}, nil
}

// SearchPOI provides simplified keyword-based POI search
func SearchPOI(keywords string, options ...AmapConfigOption) (*SearchPOIResultEx, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	req := SearchPOIRequest{
		Keywords:   keywords,
		Page:       config.Page,
		Offset:     config.PageSize,
		Extensions: config.Extensions,
		City:       config.City,
		CityLimit:  true,
	}

	if config.City != "" {
		req.City = config.City
	}

	resp, err := client.SearchPOI(ctx, req)
	if err != nil {
		return nil, err
	}

	total, _ := strconv.Atoi(resp.Count)
	result := &SearchPOIResultEx{
		Total:       total,
		Results:     make([]POIResultEx, 0, len(resp.Pois)),
		SugKeywords: resp.Suggestion.Keywords,
		SugCities:   resp.Suggestion.Cities,
	}

	for _, poi := range resp.Pois {
		location, _ := LocationFromString(poi.Location)
		result.Results = append(result.Results, POIResultEx{
			Name:         poi.Name,
			Address:      poi.Address,
			Location:     location,
			Distance:     poi.Distance,
			Type:         poi.Type,
			TelNumber:    poi.Tel,
			POIId:        poi.ID,
			BusinessArea: poi.Businessarea,
		})
	}

	return result, nil
}

// SearchNearbyPOI provides simplified location-based POI search
func SearchNearbyPOI(location string, keywords string, options ...AmapConfigOption) (*SearchPOIResultEx, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	locationGeocodes, err := Geocode(location, options...)
	if err != nil {
		return nil, err
	}
	locationGeocode := config.GeocodeFilter(locationGeocodes)
	locationLocation, err := LocationFromString(locationGeocode.Location)
	if err != nil {
		return nil, err
	}
	req := SearchNearbyPOIRequest{
		Location:   locationLocation,
		Keywords:   keywords,
		Radius:     config.Radius,
		SortRule:   config.SortRule,
		Extensions: config.Extensions,
		Page:       config.Page,
		Offset:     config.PageSize,
		City:       config.City,
	}

	if config.City != "" {
		req.City = config.City
	}

	resp, err := client.SearchNearbyPOI(ctx, req)
	if err != nil {
		return nil, err
	}

	total, _ := strconv.Atoi(resp.Count)
	result := &SearchPOIResultEx{
		Total:       total,
		Results:     make([]POIResultEx, 0, len(resp.Pois)),
		SugKeywords: resp.Suggestion.Keywords,
		SugCities:   resp.Suggestion.Cities,
	}

	for _, poi := range resp.Pois {
		location, _ := LocationFromString(poi.Location)
		result.Results = append(result.Results, POIResultEx{
			Name:         poi.Name,
			Address:      poi.Address,
			Location:     location,
			Distance:     poi.Distance,
			Type:         poi.Type,
			TelNumber:    poi.Tel,
			POIId:        poi.ID,
			BusinessArea: poi.Businessarea,
		})
	}

	return result, nil
}

// GetPOIDetail provides simplified POI detail lookup by ID
func GetPOIDetail(poiID string, options ...AmapConfigOption) (*POIResultEx, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	resp, err := client.SearchPOIByID(ctx, SearchPOIByIDRequest{
		ID:         poiID,
		Extensions: config.Extensions,
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Pois) == 0 {
		return nil, fmt.Errorf("POI not found: %s", poiID)
	}

	poi := resp.Pois[0]
	location, _ := LocationFromString(poi.Location)

	return &POIResultEx{
		Name:         poi.Name,
		Address:      poi.Address,
		Location:     location,
		Distance:     poi.Distance,
		Type:         poi.Type,
		TelNumber:    poi.Tel,
		POIId:        poi.ID,
		BusinessArea: poi.Businessarea,
	}, nil
}

// GetAOIBoundary retrieves boundary information for an Area of Interest
func _GetAOIBoundary(aoiID string, options ...AmapConfigOption) (*AOIBoundaryResultEx, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	resp, err := client.AOIBoundary(ctx, AOIBoundaryRequest{
		ID: aoiID,
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Aois) == 0 {
		return nil, fmt.Errorf("AOI not found: %s", aoiID)
	}

	aoi := resp.Aois[0]
	location, _ := LocationFromString(aoi.Location)

	// Process the polyline
	polyPoints := make([]Location, 0)
	if aoi.Polyline != "" {
		parts := strings.Split(strings.Replace(aoi.Polyline, "_", ";", -1), ";")
		for _, part := range parts {
			if loc, err := LocationFromString(part); err == nil {
				polyPoints = append(polyPoints, loc)
			}
		}
	}

	return &AOIBoundaryResultEx{
		Name:       aoi.Name,
		ID:         aoi.ID,
		Type:       aoi.Type,
		Address:    aoi.Address,
		Location:   location,
		Province:   aoi.PName,
		City:       aoi.CityName,
		District:   aoi.AdName,
		AdCode:     aoi.AdCode,
		Polyline:   aoi.Polyline,
		PolyPoints: polyPoints,
	}, nil
}

// WeatherResultEx contains simplified weather information
type WeatherResultEx struct {
	City          string // City name
	AdCode        string // Administrative area code
	Province      string // Province name
	Weather       string // Current weather condition
	Temperature   string // Current temperature in Celsius
	WindDirection string // Wind direction
	WindPower     string // Wind power level
	Humidity      string // Humidity percentage
	ReportTime    string // Report time

	// Forecast data (when available)
	Forecast []WeatherForecastEx
}

// WeatherForecastEx contains simplified forecast information
type WeatherForecastEx struct {
	Date         string // Date (YYYY-MM-DD)
	Week         string // Day of week
	DayWeather   string // Day weather condition
	NightWeather string // Night weather condition
	DayTemp      string // Day temperature in Celsius
	NightTemp    string // Night temperature in Celsius
	DayWind      string // Day wind direction
	NightWind    string // Night wind direction
}

// GetWeather retrieves weather information for a city
func GetWeather(cityCode string, options ...AmapConfigOption) (*WeatherResponse, error) {
	config := NewConfig(options...)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := NewClientByConfig(config)
	if err != nil {
		return nil, err
	}

	extensions := WeatherTypeBase
	if config.EnableWeatherForecast {
		extensions = WeatherTypeAll
	}

	return client.Weather(ctx, WeatherRequest{
		City:       cityCode,
		Extensions: extensions,
	})
}
