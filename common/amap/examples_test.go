package amap

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

var apikey string

func init() {
	apikey_byts, _ := os.ReadFile("/tmp/amap_apikey")
	apikey = strings.TrimSpace(string(apikey_byts))
}

func TestExampleClient_Geocode(t *testing.T) {
	client, _ := NewClient(WithApiKey(apikey))

	resp, err := client.Geocode(context.Background(), GeocodingRequest{
		Address: "北京市朝阳区阜通东大街6号",
	})
	if err != nil {
		t.Fatal(err)
	}

	spew.Dump(resp.Geocodes)
}

func TestExampleClient_ReverseGeocode(t *testing.T) {
	client, _ := NewClient(WithApiKey(apikey))

	location := Location{Longitude: 116.481028, Latitude: 39.989643}
	resp, err := client.ReverseGeocode(context.Background(), ReverseGeocodingRequest{
		Location: location,
	})
	if err != nil {
		t.Fatal(err)
	}

	spew.Dump(resp)
}

func TestExampleClient_Driving(t *testing.T) {
	client, _ := NewClient(WithApiKey(apikey))

	origin := Location{Longitude: 116.481028, Latitude: 39.989643}
	destination := Location{Longitude: 116.434446, Latitude: 39.90816}

	resp, err := client.Driving(context.Background(), DrivingRequest{
		DirectionRequest: DirectionRequest{
			Origin:      origin,
			Destination: destination,
		},
		Strategy: RouteStrategyFastest,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Route.Paths) > 0 {
		fmt.Printf("Distance: %s meters\n", resp.Route.Paths[0].Distance)
		fmt.Printf("Duration: %s seconds\n", resp.Route.Paths[0].Duration)
	}
}

func TestExampleClient_Walking(t *testing.T) {
	client, _ := NewClient(WithApiKey(apikey))

	origin := Location{Longitude: 116.481028, Latitude: 39.989643}
	destination := Location{Longitude: 116.434446, Latitude: 39.90816}

	resp, err := client.Walking(context.Background(), WalkingRequest{
		DirectionRequest: DirectionRequest{
			Origin:      origin,
			Destination: destination,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Route.Paths) > 0 {
		fmt.Printf("Walking distance: %s meters\n", resp.Route.Paths[0].Distance)
		fmt.Printf("Walking duration: %s seconds\n", resp.Route.Paths[0].Duration)
	}
}

func TestExampleClient_Transit(t *testing.T) {
	client, _ := NewClient(WithApiKey(apikey))

	origin := Location{Longitude: 116.481028, Latitude: 39.989643}
	destination := Location{Longitude: 116.434446, Latitude: 39.90816}

	resp, err := client.Transit(context.Background(), TransitRequest{
		DirectionRequest: DirectionRequest{
			Origin:      origin,
			Destination: destination,
		},
		City: "北京",
	})
	if err != nil {
		t.Fatal(err)
	}

	spew.Dump(resp)
}

func TestExampleClient_Distance(t *testing.T) {
	client, _ := NewClient(WithApiKey(apikey))

	// Example: Calculate distance between multiple origins and one destination
	origins := []Location{
		{Longitude: 116.481028, Latitude: 39.989643},
		{Longitude: 116.465302, Latitude: 40.004717},
	}
	destination := Location{Longitude: 116.434446, Latitude: 39.90816}

	resp, err := client.Distance(context.Background(), DistanceRequest{
		Origins:     origins,
		Destination: destination,
		Type:        DirectionTypeDriving,
	})
	if err != nil {
		t.Fatal(err)
	}

	for i, result := range resp.Results {
		fmt.Printf("Origin %d to destination: %s meters, %s seconds\n",
			i+1, result.Distance, result.Duration)
	}
}

func TestExampleClient_IPLocation(t *testing.T) {
	client, _ := NewClient(WithApiKey(apikey))

	resp, err := client.IPLocationV3(context.Background(), IPLocationRequest{
		IP: "114.247.50.2", // Example IP address
	})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("IP Location - Province: %s, City: %s, AdCode: %s\n",
		resp.Province, resp.City, resp.AdCode)
}

func TestExampleClient_SearchPOI(t *testing.T) {
	client, _ := NewClient(WithApiKey(apikey))

	// Example: Search POI by keywords
	resp, err := client.SearchPOI(context.Background(), SearchPOIRequest{
		Keywords: "美食",
		City:     "北京",
		Offset:   10, // Return 10 results per page
		Page:     1,  // First page
	})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Found %s POIs for '美食' in 北京\n", resp.Count)
	for i, poi := range resp.Pois {
		if i >= 3 {
			break // Show only first 3 results
		}
		fmt.Printf("POI %d: %s, Address: %s\n", i+1, poi.Name, poi.Address)
	}
}

func TestExampleClient_SearchNearbyPOI(t *testing.T) {
	client, _ := NewClient(WithApiKey(apikey))

	// Example: Search nearby POIs
	location := Location{Longitude: 116.481028, Latitude: 39.989643} // Beijing
	resp, err := client.SearchNearbyPOI(context.Background(), SearchNearbyPOIRequest{
		Location: location,
		Radius:   2000,       // 2km radius
		Keywords: "咖啡",       // Coffee shops
		SortRule: "distance", // Sort by distance
	})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Found %s coffee shops within 2km\n", resp.Count)
	for i, poi := range resp.Pois {
		if i >= 3 {
			break // Show only first 3 results
		}
		fmt.Printf("Coffee shop %d: %s, Distance: %s meters\n", i+1, poi.Name, poi.Distance)
	}
}

func TestExampleClient_SearchPOIByID(t *testing.T) {
	client, _ := NewClient(WithApiKey(apikey))

	// Example: Search POI by ID
	// Note: Use a valid POI ID from previous search results
	// This is just an example ID and may not exist
	poiID := "B000A83M61"
	resp, err := client.SearchPOIByID(context.Background(), SearchPOIByIDRequest{
		ID:         poiID,
		Extensions: "all", // Get detailed information
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Pois) > 0 {
		poi := resp.Pois[0]
		fmt.Printf("POI Detail - Name: %s, Address: %s, Tel: %s\n",
			poi.Name, poi.Address, poi.Tel)
	} else {
		fmt.Printf("No POI found with ID %s\n", poiID)
	}
}

func TestExampleClient_SearchPolygonPOI(t *testing.T) {
	client, _ := NewClient(WithApiKey(apikey))

	// Example: Search POI within a polygon
	// Define a small polygon in Beijing
	polygon := "116.460988,39.998331;116.486029,39.998998;116.487151,39.986308;116.458596,39.985486"
	resp, err := client.SearchPolygonPOI(context.Background(), SearchPolygonPOIRequest{
		Polygon:  polygon,
		Keywords: "银行", // Banks
	})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Found %s banks within the specified area\n", resp.Count)
	for i, poi := range resp.Pois {
		if i >= 3 {
			break // Show only first 3 results
		}
		fmt.Printf("Bank %d: %s, Address: %s\n", i+1, poi.Name, poi.Address)
	}
}

func TestExampleClient_AOIBoundary(t *testing.T) {
	client, _ := NewClient(WithApiKey(apikey))

	// Example: Get AOI boundary
	// Note: Use a valid AOI ID from previous search results
	// This is just an example ID and may not exist
	aoiID := "B000A7I2QZ"
	resp, err := client.AOIBoundary(context.Background(), AOIBoundaryRequest{
		ID: aoiID,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Aois) > 0 {
		aoi := resp.Aois[0]
		fmt.Printf("AOI Boundary - Name: %s, Type: %s\n", aoi.Name, aoi.Type)
		fmt.Printf("Location: %s\n", aoi.Location)
		fmt.Printf("Polyline (first 50 chars): %s...\n", aoi.Polyline[:min(50, len(aoi.Polyline))])
	} else {
		fmt.Printf("No AOI found with ID %s\n", aoiID)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func ExampleGetWeather() {
	// Retrieve current weather for Beijing (adcode: 110101)
	result, err := GetWeather("110101", false, WithApiKey("your_api_key"))
	if err != nil {
		log.Fatal(err)
	}
	spew.Dump(result)
}
