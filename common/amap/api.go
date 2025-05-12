package amap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// Client represents an Amap API client.
type Client struct {
	config *Config
}

func NewClientByConfig(config *Config) (*Client, error) {
	if config.ApiKey == "" {
		return nil, fmt.Errorf("amap API key is required")
	}

	return &Client{
		config: config,
	}, nil
}

// NewClient creates a new Amap API client with the provided options.
func NewClient(options ...func(*Config)) (*Client, error) {
	config := NewConfig()
	for _, option := range options {
		option(config)
	}

	return NewClientByConfig(config)
}

// doRequest performs an HTTP request and decodes the response.
func (c *Client) doRequest(ctx context.Context, path string, params url.Values, v interface{}) error {
	// Add API key to parameters
	params.Set("key", c.config.ApiKey)
	params.Set("output", "JSON")
	paramsMap := map[string]string{}
	for k, v := range params {
		paramsMap[k] = v[0]
	}

	ishttps, req, err := lowhttp.ParseUrlToHttpRequestRaw("GET", c.config.BaseURL)
	if err != nil {
		return err
	}
	req = lowhttp.ReplaceHTTPPacketPath(req, path)
	req = lowhttp.ReplaceAllHTTPPacketQueryParams(req, paramsMap)

	opts := slices.Clone(c.config.lowhttpOptions)
	opts = append([]poc.PocConfigOption{
		poc.WithForceHTTPS(ishttps),
	}, opts...)
	rsp, _, err := poc.HTTP(req, opts...)
	if err != nil {
		return err
	}
	statusCode := lowhttp.GetStatusCodeFromResponse(rsp)
	body := lowhttp.GetHTTPPacketBody(rsp)
	// Check status code
	if statusCode != http.StatusOK {
		return fmt.Errorf("amap API request failed with status %d: %s", statusCode, string(body))
	}

	// Decode response
	json.NewDecoder(bytes.NewBuffer(body)).Decode(v)
	// if err := json.NewDecoder(bytes.NewBuffer(body)).Decode(v); err != nil {
	// 	return err
	// }

	// Check API status
	if baseResp, ok := v.(interface{ IsSuccess() bool }); ok {
		if !baseResp.IsSuccess() {
			return baseResp.(error)
		}
	}

	return nil
}

// GeocodingRequest represents parameters for geocoding API.
type GeocodingRequest struct {
	Address string
	City    string
	Batch   bool
}

// GeocodingResponse represents the response from the geocoding API.
type GeocodingResponse struct {
	BaseResponse
	Count    string           `json:"count"`
	Geocodes []*GeocodeResult `json:"geocodes"`
}

// GeocodeResult represents a single geocoding result.
type GeocodeResult struct {
	FormattedAddress string `json:"formatted_address"`
	Country          string `json:"country"`
	Province         string `json:"province"`
	City             string `json:"city"`
	CityCode         string `json:"citycode"`
	District         string `json:"district"`
	Township         string `json:"township"`
	Street           string `json:"street"`
	Number           string `json:"number"`
	AdCode           string `json:"adcode"`
	Location         string `json:"location"`
	Level            string `json:"level"`
}

// Geocode converts an address to coordinates.
func (c *Client) Geocode(ctx context.Context, request GeocodingRequest) (*GeocodingResponse, error) {
	params := url.Values{}
	params.Set("address", request.Address)

	if request.City != "" {
		params.Set("city", request.City)
	}

	if request.Batch {
		params.Set("batch", "true")
	}

	response := &GeocodingResponse{}
	err := c.doRequest(ctx, "/v3/geocode/geo", params, response)
	return response, err
}

// ReverseGeocodingRequest represents parameters for reverse geocoding API.
type ReverseGeocodingRequest struct {
	Location   Location
	Radius     int    // Optional: search radius in meters, default 1000
	Poitype    string // Optional: POI type
	Extensions string // Optional: base or all, default base
	Batch      bool   // Optional: batch processing
}

// ReverseGeocodingResponse represents the response from the reverse geocoding API.
type ReverseGeocodingResponse struct {
	BaseResponse
	RegeoCodes *RegeoCodeResult `json:"regeocode"`
}

// RegeoCodeResult represents a single reverse geocoding result.
type RegeoCodeResult struct {
	FormattedAddress string             `json:"formatted_address"`
	AddressComponent AddressComponent   `json:"addressComponent"`
	Pois             []POI              `json:"pois,omitempty"`
	Roads            []Road             `json:"roads,omitempty"`
	Roadinters       []RoadIntersection `json:"roadinters,omitempty"`
}

// AddressComponent represents the components of an address.
type AddressComponent struct {
	Country       string         `json:"country"`
	Province      string         `json:"province"`
	City          string         `json:"city"`
	District      string         `json:"district"`
	Township      string         `json:"township"`
	Street        string         `json:"street"`
	Number        string         `json:"number"`
	AdCode        string         `json:"adcode"`
	BusinessAreas []BusinessArea `json:"businessAreas,omitempty"`
}

// BusinessArea represents a business area.
type BusinessArea struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Location string `json:"location"`
}

// POI represents a point of interest.
type POI struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Tel          string `json:"tel"`
	Direction    string `json:"direction"`
	Distance     string `json:"distance"`
	Location     string `json:"location"`
	Address      string `json:"address"`
	Poiweight    string `json:"poiweight"`
	Businessarea string `json:"businessarea"`
}

// Road represents a road.
type Road struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Distance  string `json:"distance"`
	Direction string `json:"direction"`
	Location  string `json:"location"`
}

// RoadIntersection represents a road intersection.
type RoadIntersection struct {
	Distance   string `json:"distance"`
	Direction  string `json:"direction"`
	Location   string `json:"location"`
	FirstID    string `json:"first_id"`
	FirstName  string `json:"first_name"`
	SecondID   string `json:"second_id"`
	SecondName string `json:"second_name"`
}

// ReverseGeocode converts coordinates to an address.
func (c *Client) ReverseGeocode(ctx context.Context, request ReverseGeocodingRequest) (*ReverseGeocodingResponse, error) {
	params := url.Values{}
	params.Set("location", request.Location.String())

	if request.Radius > 0 {
		params.Set("radius", fmt.Sprintf("%d", request.Radius))
	}

	if request.Poitype != "" {
		params.Set("poitype", request.Poitype)
	}

	if request.Extensions != "" {
		params.Set("extensions", request.Extensions)
	}

	if request.Batch {
		params.Set("batch", "true")
	}

	response := &ReverseGeocodingResponse{}
	err := c.doRequest(ctx, "/v3/geocode/regeo", params, response)
	return response, err
}

// DirectionType represents the type of direction (driving, walking, transit, etc).
type DirectionType string

const (
	DirectionTypeDriving   DirectionType = "driving"
	DirectionTypeWalking   DirectionType = "walking"
	DirectionTypeTransit   DirectionType = "transit"
	DirectionTypeBicycling DirectionType = "bicycling"
)

// RouteStrategy represents the strategy for routing.
type RouteStrategy int

const (
	// For driving
	RouteStrategyFastest              RouteStrategy = 0 // Default, fastest route
	RouteStrategyAvoidToll            RouteStrategy = 1 // Avoid toll roads
	RouteStrategyShortestDistance     RouteStrategy = 2 // Shortest distance
	RouteStrategyNoHighways           RouteStrategy = 3 // Avoid highways
	RouteStrategyFewestTurns          RouteStrategy = 4 // Fewest turns
	RouteStrategyAvoidCongestion      RouteStrategy = 5 // Avoid congestion
	RouteStrategyProvincial           RouteStrategy = 6 // Provincial route
	RouteStrategyFewestHighways       RouteStrategy = 7 // Fewest highways
	RouteStrategyAvoidHighwaysAndToll RouteStrategy = 8 // Avoid highways and toll roads
	RouteStrategyFastestNoToll        RouteStrategy = 9 // Fast route without tolls
)

// DirectionRequest represents the common parameters for direction API requests.
type DirectionRequest struct {
	Origin           Location
	Destination      Location
	OutputExtensions string // base or all, default base
}

// DrivingRequest represents parameters for driving directions.
type DrivingRequest struct {
	DirectionRequest
	Strategy      RouteStrategy
	Waypoints     []Location
	AvoidPolygons []Location
	AvoidRoad     string
}

// WalkingRequest represents parameters for walking directions.
type WalkingRequest struct {
	DirectionRequest
}

// BicyclingRequest represents parameters for bicycling directions.
type BicyclingRequest struct {
	DirectionRequest
}

// TransitRequest represents parameters for transit directions.
type TransitRequest struct {
	DirectionRequest
	City      string // Required for transit
	CitydD    string // Destination city for cross-city transit
	Strategy  int    // Transit strategy
	NightFlag int    // Whether to consider night buses
	Date      string // yyyy-MM-dd format
	Time      string // HH:mm format
}

// DirectionResponse represents the common fields in direction API responses.
type DirectionResponse struct {
	BaseResponse
	Count string `json:"count"`
	Route Route  `json:"route"`
}

// Route represents a route in a direction response.
type Route struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	TaxiCost    string `json:"taxi_cost"`
	Paths       []Path `json:"paths"`
}

// Path represents a path in a route.
type Path struct {
	Distance      string `json:"distance"`
	Duration      string `json:"duration"`
	Strategy      string `json:"strategy"`
	Tolls         string `json:"tolls"`
	TollDistance  string `json:"toll_distance"`
	Restriction   string `json:"restriction"`
	TrafficLights string `json:"traffic_lights"`
	Steps         []Step `json:"steps"`
}

// Step represents a step in a path.
type Step struct {
	Instruction string `json:"instruction"`
	Orientation string `json:"orientation"`
	Road        string `json:"road"`
	Distance    string `json:"distance"`
	Duration    string `json:"duration"`
	Polyline    string `json:"polyline"`
	Action      string `json:"action"`
	Assistant   string `json:"assistant"`
	TollRoad    string `json:"toll_road"`
	TollCost    string `json:"toll_cost"`
}

// Driving calculates driving directions between two locations.
func (c *Client) Driving(ctx context.Context, request DrivingRequest) (*DirectionResponse, error) {
	params := url.Values{}
	params.Set("origin", request.Origin.String())
	params.Set("destination", request.Destination.String())

	if request.Strategy != 0 {
		params.Set("strategy", fmt.Sprintf("%d", request.Strategy))
	}

	if len(request.Waypoints) > 0 {
		waypoints := make([]string, len(request.Waypoints))
		for i, wp := range request.Waypoints {
			waypoints[i] = wp.String()
		}
		params.Set("waypoints", strings.Join(waypoints, ";"))
	}

	if len(request.AvoidPolygons) > 0 {
		avoidPolygons := make([]string, len(request.AvoidPolygons))
		for i, p := range request.AvoidPolygons {
			avoidPolygons[i] = p.String()
		}
		params.Set("avoidpolygons", strings.Join(avoidPolygons, ";"))
	}

	if request.AvoidRoad != "" {
		params.Set("avoidroad", request.AvoidRoad)
	}

	if request.OutputExtensions != "" {
		params.Set("extensions", request.OutputExtensions)
	}

	response := &DirectionResponse{}
	err := c.doRequest(ctx, "/v3/direction/driving", params, response)
	return response, err
}

// Walking calculates walking directions between two locations.
func (c *Client) Walking(ctx context.Context, request WalkingRequest) (*DirectionResponse, error) {
	params := url.Values{}
	params.Set("origin", request.Origin.String())
	params.Set("destination", request.Destination.String())

	if request.OutputExtensions != "" {
		params.Set("extensions", request.OutputExtensions)
	}

	response := &DirectionResponse{}
	err := c.doRequest(ctx, "/v3/direction/walking", params, response)
	return response, err
}

// BicyclingResponse represents the response from the bicycling direction API.
type BicyclingResponse struct {
	Errcode   int             `json:"errcode"`
	Errmsg    string          `json:"errmsg"`
	Errdetail string          `json:"errdetail"`
	Data      BicyclingResult `json:"data"`
}

type BicyclingResult struct {
	Origin      string          `json:"origin"`
	Destination string          `json:"destination"`
	Paths       []BicyclingPath `json:"paths"`
}

type BicyclingPath struct {
	Distance int             `json:"distance"`
	Duration int             `json:"duration"`
	Steps    []BicyclingStep `json:"steps"`
}

type BicyclingStep struct {
	Instruction string `json:"instruction"`
	Road        string `json:"road"`
	Distance    int    `json:"distance"`
	Duration    int    `json:"duration"`
	Polyline    string `json:"polyline"`
	Action      string `json:"action"`
	Assistant   string `json:"assistant"`
}

// Bicycling calculates bicycling directions between two locations.
func (c *Client) Bicycling(ctx context.Context, request BicyclingRequest) (*BicyclingResponse, error) {
	params := url.Values{}
	params.Set("origin", request.Origin.String())
	params.Set("destination", request.Destination.String())

	if request.OutputExtensions != "" {
		params.Set("extensions", request.OutputExtensions)
	}

	response := &BicyclingResponse{}
	err := c.doRequest(ctx, "/v4/direction/bicycling", params, response)
	return response, err
}

// TransitResponse represents the response from the transit direction API.
type TransitResponse struct {
	BaseResponse
	Count string       `json:"count"`
	Route TransitRoute `json:"route"`
}

// TransitRoute represents a transit route.
type TransitRoute struct {
	Origin      string        `json:"origin"`
	Destination string        `json:"destination"`
	Distance    string        `json:"distance"`
	TaxiCost    string        `json:"taxi_cost"`
	Transits    []TransitPath `json:"transits"`
}

// TransitPath represents a transit path.
type TransitPath struct {
	Distance string           `json:"distance"`
	Duration string           `json:"duration"`
	Walking  string           `json:"walking_distance"`
	Cost     string           `json:"cost"`
	Segments []TransitSegment `json:"segments"`
}

// TransitSegment represents a segment in a transit path.
type TransitSegment struct {
	Walking  TransitWalking  `json:"walking"`
	Bus      TransitBus      `json:"bus"`
	Entrance TransitEntrance `json:"entrance"`
	Exit     TransitExit     `json:"exit"`
}

// TransitWalking represents a walking segment in transit.
type TransitWalking struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	Distance    string `json:"distance"`
	Duration    string `json:"duration"`
	Steps       []Step `json:"steps"`
}

// TransitBus represents a bus segment in transit.
type TransitBus struct {
	BusLines []BusLine `json:"buslines"`
}

// BusLine represents a bus line.
type BusLine struct {
	Name      string    `json:"name"`
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Distance  string    `json:"distance"`
	Duration  string    `json:"duration"`
	Polyline  string    `json:"polyline"`
	Departure BusStop   `json:"departure_stop"`
	Arrival   BusStop   `json:"arrival_stop"`
	Via       string    `json:"via_stops"`
	ViaStops  []BusStop `json:"via_stops"`
}

type BusStop struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Location string `json:"location"`
}

// Stop represents a transit stop.
type Stop struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Location string `json:"location"`
}

// TransitEntrance represents an entrance to a transit station.
type TransitEntrance struct {
	Name     string `json:"name"`
	Location string `json:"location"`
}

// TransitExit represents an exit from a transit station.
type TransitExit struct {
	Name     string `json:"name"`
	Location string `json:"location"`
}

// Transit calculates transit directions between two locations.
func (c *Client) Transit(ctx context.Context, request TransitRequest) (*TransitResponse, error) {
	params := url.Values{}
	params.Set("origin", request.Origin.String())
	params.Set("destination", request.Destination.String())

	if request.City == "" {
		return nil, fmt.Errorf("city is required for transit directions")
	}
	params.Set("city", request.City)

	if request.CitydD != "" {
		params.Set("cityd", request.CitydD)
	}

	if request.Strategy != 0 {
		params.Set("strategy", fmt.Sprintf("%d", request.Strategy))
	}

	if request.NightFlag != 0 {
		params.Set("nightflag", fmt.Sprintf("%d", request.NightFlag))
	}

	if request.Date != "" {
		params.Set("date", request.Date)
	}

	if request.Time != "" {
		params.Set("time", request.Time)
	}

	if request.OutputExtensions != "" {
		params.Set("extensions", request.OutputExtensions)
	}

	response := &TransitResponse{}
	err := c.doRequest(ctx, "/v3/direction/transit/integrated", params, response)
	return response, err
}

// DistanceRequest represents parameters for the distance API.
type DistanceRequest struct {
	Origins     []Location
	Destination Location
	Type        DirectionType
}

// DistanceResponse represents the response from the distance API.
type DistanceResponse struct {
	BaseResponse
	Results []DistanceResult `json:"results"`
}

// DistanceResult represents a result from the distance API.
type DistanceResult struct {
	OriginID string `json:"origin_id"`
	DestID   string `json:"dest_id"`
	Distance string `json:"distance"`
	Duration string `json:"duration"`
}

// Distance calculates the distance between multiple origins and a destination.
func (c *Client) Distance(ctx context.Context, request DistanceRequest) (*DistanceResponse, error) {
	params := url.Values{}

	if len(request.Origins) == 0 {
		return nil, fmt.Errorf("at least one origin is required")
	}

	origins := make([]string, len(request.Origins))
	for i, origin := range request.Origins {
		origins[i] = origin.String()
	}
	params.Set("origins", strings.Join(origins, "|"))

	params.Set("destination", request.Destination.String())

	if request.Type != "" {
		params.Set("type", string(request.Type))
	}

	response := &DistanceResponse{}
	err := c.doRequest(ctx, "/v3/distance", params, response)
	return response, err
}

// IPLocationRequest represents parameters for the IP location API.
type IPLocationRequest struct {
	IP string // IP address to locate, if empty uses requester's IP
}

// IPLocationResponse represents the response from the IP location API.
type IPLocationResponse struct {
	BaseResponse
	Province  string `json:"province"`  // Province name, returns empty for foreign or invalid IPs
	City      string `json:"city"`      // City name, returns empty for foreign or invalid IPs
	AdCode    string `json:"adcode"`    // Administrative area code
	Rectangle string `json:"rectangle"` // Rectangle area of the city, "minLng,minLat,maxLng,maxLat"
}

// IPLocationV3 locates an IP address geographically.
func (c *Client) IPLocationV3(ctx context.Context, request IPLocationRequest) (*IPLocationResponse, error) {
	params := url.Values{}

	if request.IP != "" {
		params.Set("ip", request.IP)
	}

	response := &IPLocationResponse{}
	err := c.doRequest(ctx, "/v3/ip", params, response)
	return response, err
}

// SearchPOIRequest represents parameters for the POI search API.
type SearchPOIRequest struct {
	Keywords   string    // Search keywords, required if Keywords or Types is not specified
	Types      string    // POI types, required if Keywords is not specified
	City       string    // City name, citycode, or adcode
	CityLimit  bool      // Whether to limit the search to the specified city
	Children   int       // Whether to include child categories (0-not include, 1-include)
	Offset     int       // Result offset, default 0
	Page       int       // Page number, default 1
	Extensions string    // Return basic or all information, default base
	Output     string    // Return format, default JSON
	Location   *Location // Specify location for nearby search
	Radius     int       // Search radius in meters, default 3000m (max 50000m)
	SortRule   string    // Sort rule: "distance"(default for nearby search) or "weight"(default for keyword search)
	Polygon    string    // Search within a polygon, format: "lng1,lat1;lng2,lat2...;lngn,latn"
}

// SearchPOIResponse represents the response from the POI search API.
type SearchPOIResponse struct {
	BaseResponse
	Count      string `json:"count"`
	Pois       []POI  `json:"pois"`
	Suggestion struct {
		Keywords []string `json:"keywords"`
		Cities   []string `json:"cities"`
	} `json:"suggestion"`
}

// SearchPOI searches for POIs.
func (c *Client) SearchPOI(ctx context.Context, request SearchPOIRequest) (*SearchPOIResponse, error) {
	params := url.Values{}

	if request.Keywords != "" {
		params.Set("keywords", request.Keywords)
	}

	if request.Types != "" {
		params.Set("types", request.Types)
	}

	if request.Keywords == "" && request.Types == "" {
		return nil, fmt.Errorf("either keywords or types must be specified")
	}

	if request.City != "" {
		params.Set("city", request.City)
	}

	if request.CityLimit {
		params.Set("citylimit", "true")
	}

	if request.Children != 0 {
		params.Set("children", fmt.Sprintf("%d", request.Children))
	}

	if request.Offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", request.Offset))
	}

	if request.Page > 0 {
		params.Set("page", fmt.Sprintf("%d", request.Page))
	}

	if request.Extensions != "" {
		params.Set("extensions", request.Extensions)
	}

	if request.Output != "" {
		params.Set("output", request.Output)
	}

	if request.Location != nil {
		params.Set("location", request.Location.String())
	}

	if request.Radius > 0 {
		params.Set("radius", fmt.Sprintf("%d", request.Radius))
	}

	if request.SortRule != "" {
		params.Set("sortrule", request.SortRule)
	}

	if request.Polygon != "" {
		params.Set("polygon", request.Polygon)
	}

	response := &SearchPOIResponse{}
	err := c.doRequest(ctx, "/v3/place/text", params, response)
	return response, err
}

// SearchNearbyPOIRequest represents parameters for nearby POI search.
type SearchNearbyPOIRequest struct {
	Location   Location // Required: center point of nearby search
	Radius     int      // Search radius in meters, default 3000m (max 50000m)
	Keywords   string   // Search keywords
	Types      string   // POI types
	City       string   // City name, citycode, or adcode
	SortRule   string   // Sort rule: "distance"(default) or "weight"
	Offset     int      // Result offset, default 0
	Page       int      // Page number, default 1
	Extensions string   // Return basic or all information, default base
}

// SearchNearbyPOI searches for POIs around a location.
func (c *Client) SearchNearbyPOI(ctx context.Context, request SearchNearbyPOIRequest) (*SearchPOIResponse, error) {
	params := url.Values{}

	// Required parameter
	params.Set("location", request.Location.String())

	if request.Radius > 0 {
		params.Set("radius", fmt.Sprintf("%d", request.Radius))
	}

	if request.Keywords != "" {
		params.Set("keywords", request.Keywords)
	}

	if request.Types != "" {
		params.Set("types", request.Types)
	}

	if request.City != "" {
		params.Set("city", request.City)
	}

	if request.SortRule != "" {
		params.Set("sortrule", request.SortRule)
	}

	if request.Offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", request.Offset))
	}

	if request.Page > 0 {
		params.Set("page", fmt.Sprintf("%d", request.Page))
	}

	if request.Extensions != "" {
		params.Set("extensions", request.Extensions)
	}

	response := &SearchPOIResponse{}
	err := c.doRequest(ctx, "/v3/place/around", params, response)
	return response, err
}

// SearchPolygonPOIRequest represents parameters for polygon POI search.
type SearchPolygonPOIRequest struct {
	Polygon    string // Required: search within a polygon, format: "lng1,lat1;lng2,lat2...;lngn,latn"
	Keywords   string // Search keywords
	Types      string // POI types
	Offset     int    // Result offset, default 0
	Page       int    // Page number, default 1
	Extensions string // Return basic or all information, default base
}

// SearchPolygonPOI searches for POIs within a polygon.
func (c *Client) SearchPolygonPOI(ctx context.Context, request SearchPolygonPOIRequest) (*SearchPOIResponse, error) {
	params := url.Values{}

	// Required parameter
	if request.Polygon == "" {
		return nil, fmt.Errorf("polygon is required")
	}
	params.Set("polygon", request.Polygon)

	if request.Keywords != "" {
		params.Set("keywords", request.Keywords)
	}

	if request.Types != "" {
		params.Set("types", request.Types)
	}

	if request.Offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", request.Offset))
	}

	if request.Page > 0 {
		params.Set("page", fmt.Sprintf("%d", request.Page))
	}

	if request.Extensions != "" {
		params.Set("extensions", request.Extensions)
	}

	response := &SearchPOIResponse{}
	err := c.doRequest(ctx, "/v3/place/polygon", params, response)
	return response, err
}

// SearchPOIByIDRequest represents parameters for POI ID search.
type SearchPOIByIDRequest struct {
	ID         string // Required: POI ID
	Extensions string // Return basic or all information, default base
}

// SearchPOIByID searches for a POI by its ID.
func (c *Client) SearchPOIByID(ctx context.Context, request SearchPOIByIDRequest) (*SearchPOIResponse, error) {
	params := url.Values{}

	// Required parameter
	if request.ID == "" {
		return nil, fmt.Errorf("POI ID is required")
	}
	params.Set("id", request.ID)

	if request.Extensions != "" {
		params.Set("extensions", request.Extensions)
	}

	response := &SearchPOIResponse{}
	err := c.doRequest(ctx, "/v3/place/detail", params, response)
	return response, err
}

// AOIBoundaryRequest represents parameters for AOI boundary query.
type AOIBoundaryRequest struct {
	ID string // AOI ID, required
}

// AOIBoundaryResponse represents the response from the AOI boundary API.
type AOIBoundaryResponse struct {
	BaseResponse
	Status string `json:"status"`
	Info   string `json:"info"`
	Aois   []AOI  `json:"aois"`
}

// AOI represents an Area of Interest.
type AOI struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Location string `json:"location"`
	Polyline string `json:"polyline"`
	Type     string `json:"type"`
	TypeCode string `json:"typecode"`
	PName    string `json:"pname"`    // Province name
	CityName string `json:"cityname"` // City name
	AdName   string `json:"adname"`   // District name
	Address  string `json:"address"`
	PCode    string `json:"pcode"`    // Province code
	CityCode string `json:"citycode"` // City code
	AdCode   string `json:"adcode"`   // District code
}

// AOIBoundary retrieves the boundary of an AOI by its ID.
func (c *Client) AOIBoundary(ctx context.Context, request AOIBoundaryRequest) (*AOIBoundaryResponse, error) {
	params := url.Values{}

	// Required parameter
	if request.ID == "" {
		return nil, fmt.Errorf("AOI ID is required")
	}
	params.Set("id", request.ID)

	response := &AOIBoundaryResponse{}
	err := c.doRequest(ctx, "/v5/aoi/polyline", params, response)
	return response, err
}

// WeatherType represents the type of weather information to request
type WeatherType string

const (
	WeatherTypeBase WeatherType = "base" // Only return current weather condition
	WeatherTypeAll  WeatherType = "all"  // Return weather forecast for the next few days
)

// WeatherRequest represents parameters for weather API.
type WeatherRequest struct {
	City       string      // Required: city code (adcode)
	Extensions WeatherType // Optional: base for current weather, all for forecast
}

// WeatherResponse represents the response from the weather API.
type WeatherResponse struct {
	BaseResponse
	Count    string            `json:"count"`
	Lives    []WeatherLive     `json:"lives,omitempty"`     // Current weather data
	Forecast []WeatherForecast `json:"forecasts,omitempty"` // Weather forecast data
}

// WeatherLive represents current weather data.
type WeatherLive struct {
	Province      string `json:"province"`      // Province name
	City          string `json:"city"`          // City name
	AdCode        string `json:"adcode"`        // Administrative area code
	Weather       string `json:"weather"`       // Weather description (in Chinese)
	Temperature   string `json:"temperature"`   // Current temperature in Celsius
	WindDirection string `json:"winddirection"` // Wind direction
	WindPower     string `json:"windpower"`     // Wind power level
	Humidity      string `json:"humidity"`      // Humidity
	ReportTime    string `json:"reporttime"`    // Report time
}

// WeatherForecast represents weather forecast data.
type WeatherForecast struct {
	City       string       `json:"city"`       // City name
	AdCode     string       `json:"adcode"`     // Administrative area code
	Province   string       `json:"province"`   // Province name
	ReportTime string       `json:"reporttime"` // Report time
	Casts      []WeatherDay `json:"casts"`      // Daily forecasts
}

// WeatherDay represents a single day's weather forecast.
type WeatherDay struct {
	Date         string `json:"date"`         // Date
	Week         string `json:"week"`         // Day of week
	DayWeather   string `json:"dayweather"`   // Day weather
	NightWeather string `json:"nightweather"` // Night weather
	DayTemp      string `json:"daytemp"`      // Day temperature
	NightTemp    string `json:"nighttemp"`    // Night temperature
	DayWind      string `json:"daywind"`      // Day wind direction
	NightWind    string `json:"nightwind"`    // Night wind direction
	DayPower     string `json:"daypower"`     // Day wind power
	NightPower   string `json:"nightpower"`   // Night wind power
}

// Weather queries the weather for a city.
func (c *Client) Weather(ctx context.Context, request WeatherRequest) (*WeatherResponse, error) {
	params := url.Values{}

	// Required parameter
	if request.City == "" {
		return nil, fmt.Errorf("city code is required")
	}
	params.Set("city", request.City)

	if request.Extensions != "" {
		params.Set("extensions", string(request.Extensions))
	}

	response := &WeatherResponse{}
	err := c.doRequest(ctx, "/v3/weather/weatherInfo", params, response)
	return response, err
}
