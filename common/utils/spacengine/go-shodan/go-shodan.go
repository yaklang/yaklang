/*Package shodan is an interface for the Shodan API*/
package shodan

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"
)

var _ base.IUserProfile = (*ShodanClient)(nil)

var (
	defaultAPIHost        = "https://api.shodan.io"
	defaultExploitAPIHost = "https://exploits.shodan.io"
	sessionKey            = "__YAK_BUILTIN_SHODAN_CLIENT__"
)

// ShodanClient stores shared data that is used to interact with the API.
type ShodanClient struct {
	*base.BaseSpaceEngineClient
	exploitAPIHost string
}

type ShodanUser struct {
	Member      bool        `json:"member"`
	Credits     int         `json:"credits"`
	DisplayName interface{} `json:"display_name"`
	Created     string      `json:"created"`
}

// Exploit is used to unmarshal the JSON response from '/api/search'.
type Exploit struct {
	Matches []struct {
		Source      string        `json:"source"`
		ID          interface{}   `json:"_id"`
		Author      interface{}   `json:"author"`
		Code        interface{}   `json:"code"`
		Date        time.Time     `json:"date"`
		Platform    interface{}   `json:"platform"`
		Port        int           `json:"port"`
		Type        string        `json:"type"`
		Description string        `json:"description"`
		Osvdb       []int         `json:"osvdb"`
		Bid         []int         `json:"bid"`
		Cve         []string      `json:"cve"`
		Msb         []interface{} `json:"msb"`
	} `json:"matches"`
	Total int `json:"total"`
}

// Host is used to unmarshal the JSON response from '/shodan/host/{ip}'.
type Host struct {
	RegionCode  string   `json:"region_code"`
	IP          int      `json:"ip"`
	AreaCode    int      `json:"area_code"`
	Latitude    float64  `json:"latitude"`
	Hostnames   []string `json:"hostnames"`
	PostalCode  string   `json:"postal_code"`
	DmaCode     int      `json:"dma_code"`
	CountryCode string   `json:"country_code"`
	Org         string   `json:"org"`
	Data        []struct {
		Product   string   `json:"product"`
		Title     string   `json:"title"`
		Opts      struct{} `json:"opts"`
		Timestamp string   `json:"timestamp"`
		Isp       string   `json:"isp"`
		Cpe       []string `json:"cpe"`
		Data      string   `json:"data"`
		HTML      string   `json:"html"`
		Location  struct {
			City         string  `json:"city"`
			RegionCode   string  `json:"region_code"`
			AreaCode     int     `json:"area_code"`
			Longitude    float64 `json:"longitude"`
			CountryCode3 string  `json:"country_code3"`
			Latitude     float64 `json:"latitude"`
			PostalCode   string  `json:"postal_code"`
			DmaCode      int     `json:"dma_code"`
			CountryCode  string  `json:"country_code"`
			CountryName  string  `json:"country_name"`
		} `json:"location"`
		IP        int         `json:"ip"`
		Domains   []string    `json:"domains"`
		Org       string      `json:"org"`
		Os        interface{} `json:"os"`
		Port      int         `json:"port"`
		Hostnames []string    `json:"hostnames"`
		IPStr     string      `json:"ip_str"`
	} `json:"data"`
	City         string      `json:"city"`
	Isp          string      `json:"isp"`
	Asn          string      `json:"asn"`
	Longitude    float64     `json:"longitude"`
	LastUpdate   string      `json:"last_update"`
	CountryCode3 string      `json:"country_code3"`
	CountryName  string      `json:"country_name"`
	IPStr        string      `json:"ip_str"`
	Os           interface{} `json:"os"`
	Ports        []int       `json:"ports"`
}

// HostCount is used to unmarshal the JSON response from '/shodan/host/count'.
type HostCount struct {
	Matches []interface{} `json:"matches"`
	Facets  struct {
		Org []struct {
			Count int    `json:"count"`
			Value string `json:"value"`
		} `json:"org"`
	} `json:"facets"`
	Total int `json:"total"`
}

// HostSearch is used to unmarshal the JSON response from '/shodan/host/search'.
type HostSearch struct {
	Matches []struct {
		Os        interface{}   `json:"os"`
		Timestamp string        `json:"timestamp"`
		Isp       string        `json:"isp"`
		Asn       string        `json:"asn"`
		Hostnames []interface{} `json:"hostnames"`
		Location  struct {
			City         interface{} `json:"city"`
			RegionCode   interface{} `json:"region_code"`
			AreaCode     interface{} `json:"area_code"`
			Longitude    float64     `json:"longitude"`
			CountryCode3 string      `json:"country_code3"`
			CountryName  string      `json:"country_name"`
			PostalCode   interface{} `json:"postal_code"`
			DmaCode      interface{} `json:"dma_code"`
			CountryCode  string      `json:"country_code"`
			Latitude     float64     `json:"latitude"`
		} `json:"location"`
		HTTP struct {
			RobotsHash      interface{}            `json:"robots_hash"`
			Redirects       []interface{}          `json:"redirects"`
			Securitytxt     interface{}            `json:"securitytxt"`
			Title           string                 `json:"title"`
			SitemapHash     interface{}            `json:"sitemap_hash"`
			Robots          interface{}            `json:"robots"`
			Server          string                 `json:"server"`
			Host            string                 `json:"host"`
			HTML            string                 `json:"html"`
			Location        string                 `json:"location"`
			Components      map[string]interface{} `json:"components"`
			SecuritytxtHash interface{}            `json:"securitytxt_hash"`
			Sitemap         interface{}            `json:"sitemap"`
			HTMLHash        int                    `json:"html_hash"`
		}
		IP      int64         `json:"ip"`
		Domains []interface{} `json:"domains"`
		Data    string        `json:"data"`
		Org     string        `json:"org"`
		Port    int           `json:"port"`
		IPStr   string        `json:"ip_str"`
	} `json:"matches"`
	Facets struct {
		Org []struct {
			Count int    `json:"count"`
			Value string `json:"value"`
		} `json:"org"`
	} `json:"facets"`
	Total int `json:"total"`
}

// HostSearchTokens is used to unmarshal the JSON response from '/shodan/host/search/tokens'.
type HostSearchTokens struct {
	Attributes struct {
		Ports []int `json:"ports"`
	} `json:"attributes"`
	Errors  []interface{} `json:"errors"`
	String  string        `json:"string"`
	Filters []string      `json:"filters"`
}

// Scan is used to unmarshal the JSON response from '/shodan/scan'.
// This is not implemented.
type Scan struct {
	ID          string `json:"id"`
	Count       int    `json:"count"`
	CreditsLeft int    `json:"credits_left"`
}

// ScanInternet is used to unmarshal the JSON response from '/shodan/scan/internet'.
// This is not implemented.
type ScanInternet struct {
	ID string `json:"id"`
}

// Query is used to unmarshal the JSON response from '/shodan/query/{search}'.
type Query struct {
	Total   int `json:"total"`
	Matches []struct {
		Votes       int      `json:"votes"`
		Description string   `json:"description"`
		Title       string   `json:"title"`
		Timestamp   string   `json:"timestamp"`
		Tags        []string `json:"tags"`
		Query       string   `json:"query"`
	} `json:"matches"`
}

// QueryTags is used to unmarshal the JSON response from '/shodan/query/tags'.
type QueryTags struct {
	Total   int `json:"total"`
	Matches []struct {
		Value string `json:"value"`
		Count int    `json:"count"`
	} `json:"matches"`
}

// APIInfo is used to unmarshal the JSON response from '/shodan/api-info'.
type APIInfo struct {
	QueryCredits int    `json:"query_credits"`
	ScanCredits  int    `json:"scan_credits"`
	Telnet       bool   `json:"telnet"`
	Plan         string `json:"plan"`
	HTTPS        bool   `json:"https"`
	Unlocked     bool   `json:"unlocked"`
}

// DNSResolve is used to transform the map[string]string response from '/dns/resolve'
// into a struct that is a bit easier to work with.
type DNSResolve struct {
	Hostname string
	IP       string
}

// DNSReverse is used to transform the map[string][]string response from '/dns/reverse'
// into a struct that is a bit easier to work with.
type DNSReverse struct {
	IP        string
	Hostnames []string
}

// Error used to unmarshal the JSON response of an error.
type Error struct {
	Error string `json:"error"`
}

// NewClient returns a new Client.
func NewClient(key string) *ShodanClient {
	return &ShodanClient{
		BaseSpaceEngineClient: base.NewBaseSpaceEngineClient(key, defaultAPIHost),
		exploitAPIHost:        defaultExploitAPIHost,
	}
}

// NewClientEx returns a new Client with custom host.
func NewClientEx(key string, apiHost string) *ShodanClient {
	if apiHost == "" {
		apiHost = defaultAPIHost
	}
	return &ShodanClient{
		BaseSpaceEngineClient: base.NewBaseSpaceEngineClient(key, defaultAPIHost),
		exploitAPIHost:        defaultExploitAPIHost,
	}
}

func (c *ShodanClient) UserProfile() ([]byte, error) {
	params := map[string]string{
		"key": c.Key,
	}
	rsp, err := c.doRequest("/account/profile", poc.WithReplaceAllHttpPacketQueryParams(params))
	return rsp.Body, err
}

// Host calls '/shodan/host/{ip}' and returns the unmarshalled response.
// ip is the IP address to search for.
// opts are all query paramters to pass in the request. You do not have to provide your API key.
func (c *ShodanClient) Host(ip string, params map[string]string) (*Host, error) {
	h := &Host{}
	params["key"] = c.Key
	err := c.doRequestAndUnmarshal("/shodan/host", &h, poc.WithReplaceAllHttpPacketQueryParams(params))
	return h, err
}

// HostCount calls '/shodan/host/count' and returns the unmarshalled response.
// query is the search query to pass in the request.
// facets are any facets to pass in the request.
func (c *ShodanClient) HostCount(query string, facets []string) (*HostCount, error) {
	h := &HostCount{}
	params := map[string]string{
		"key":    c.Key,
		"facets": strings.Join(facets, ","),
		"query":  query,
	}

	err := c.doRequestAndUnmarshal("/shodan/host/count", &h, poc.WithReplaceAllHttpPacketQueryParams(params))
	return h, err
}

// HostSearch calls '/shodan/host/search' and returns the unmarshalled response.
// query is the search query to pass in the request.
// facets are any facets to pass in the request.
// opts are any additional query parameters to set, such as page and minify.
func (c *ShodanClient) HostSearch(query string, facets []string, params map[string]string) (*HostSearch, error) {
	h := &HostSearch{}
	params["key"] = c.Key
	params["facets"] = strings.Join(facets, ",")
	params["query"] = query
	err := c.doRequestAndUnmarshal("/shodan/host/search", &h, poc.WithReplaceAllHttpPacketQueryParams(params))
	return h, err
}

// HostSearchTokens calls '/shodan/host/search/tokens' and returns the unmarshalled response.
// query is the search query to pass in the request.
func (c *ShodanClient) HostSearchTokens(query string) (*HostSearchTokens, error) {
	h := &HostSearchTokens{}
	params := map[string]string{
		"key":   c.Key,
		"query": query,
	}
	err := c.doRequestAndUnmarshal("/shodan/host/search/tokens", &h, poc.WithReplaceAllHttpPacketQueryParams(params))
	return h, err
}

// Protocols calls '/shodan/protocols' and returns the unmarshalled response.
func (c *ShodanClient) Protocols() (map[string]string, error) {
	m := make(map[string]string)
	params := map[string]string{
		"key": c.Key,
	}
	err := c.doRequestAndUnmarshal("/shodan/protocols", &m, poc.WithReplaceAllHttpPacketQueryParams(params))
	return m, err
}

// Services calls '/shodan/services' and returns the unmarshalled response.
func (c *ShodanClient) Services() (map[string]string, error) {
	m := make(map[string]string)
	params := map[string]string{
		"key": c.Key,
	}
	err := c.doRequestAndUnmarshal("/shodan/services", &m, poc.WithReplaceAllHttpPacketQueryParams(params))
	return m, err
}

// Query calls '/shodan/query' and returns the unmarshalled response.
// opts are additional query parameters. You do not need to provide your API key.
func (c *ShodanClient) Query(params map[string]string) (*Query, error) {
	q := &Query{}
	params["key"] = c.Key
	err := c.doRequestAndUnmarshal("/shodan/query", &q, poc.WithReplaceAllHttpPacketQueryParams(params))
	return q, err
}

// QuerySearch calls '/shodan/query/search' and returns the unmarshalled response.
// query is the search query to pass in the request.
// opts are additional query parameters. You do not need to provide your API key.
func (c *ShodanClient) QuerySearch(query string, params map[string]string) (*Query, error) {
	q := &Query{}
	params["key"] = c.Key
	params["query"] = query
	err := c.doRequestAndUnmarshal("/shodan/query/search", &q, poc.WithReplaceAllHttpPacketQueryParams(params))
	return q, err
}

// QueryTags calls '/shodan/query/tags' and returns the unmarshalled response.
// opts are additional query parameters. You do not need to provide your API key.
func (c *ShodanClient) QueryTags(params map[string]string) (*QueryTags, error) {
	q := &QueryTags{}
	params["key"] = c.Key

	err := c.doRequestAndUnmarshal("/shodan/query/tags", &q, poc.WithReplaceAllHttpPacketQueryParams(params))
	return q, err
}

// APIInfo calls '/api-info' and returns the unmarshalled response.
func (c *ShodanClient) APIInfo() (*APIInfo, error) {
	i := &APIInfo{}
	params := map[string]string{
		"key": c.Key,
	}
	err := c.doRequestAndUnmarshal("/api-info", &i, poc.WithReplaceAllHttpPacketQueryParams(params))
	return i, err
}

// DNSResolve calls '/dns/resolve' and returns the unmarshalled response.
func (c *ShodanClient) DNSResolve(hostnames []string) ([]DNSResolve, error) {
	d := []DNSResolve{}
	params := map[string]string{
		"key":       c.Key,
		"hostnames": strings.Join(hostnames, ","),
	}
	m := make(map[string]string)
	if err := c.doRequestAndUnmarshal("/dns/resolve", &m, poc.WithReplaceAllHttpPacketQueryParams(params)); err != nil {
		return d, err
	}
	for k, v := range m {
		d = append(d, DNSResolve{
			Hostname: k,
			IP:       v,
		})
	}
	return d, nil
}

// DNSReverse calls '/dns/reverse' and returns the unmarshalled response.
func (c *ShodanClient) DNSReverse(ips []string) ([]DNSReverse, error) {
	d := []DNSReverse{}
	params := map[string]string{
		"key": c.Key,
		"ips": strings.Join(ips, ","),
	}

	m := make(map[string][]string)
	if err := c.doRequestAndUnmarshal("/dns/reverse", &m, poc.WithReplaceAllHttpPacketQueryParams(params)); err != nil {
		return d, err
	}
	for k, v := range m {
		r := DNSReverse{IP: k}
		for _, n := range v {
			r.Hostnames = append(r.Hostnames, n)
		}
		d = append(d, r)
	}
	return d, nil
}

// Exploits calls '/api/exploits' from the exploit API and returns
// the unmarshalled response.
// query is the search query string.
// facets are any facets to add to the request.
func (c *ShodanClient) Exploits(query string, facets []string) (*Exploit, error) {
	e := &Exploit{}
	params := map[string]string{
		"key":    c.Key,
		"facets": strings.Join(facets, ","),
		"query":  query,
	}
	err := c.doRequestAndUnmarshal("/api/search", &e, poc.WithReplaceAllHttpPacketQueryParams(params))
	return e, err
}

func (c *ShodanClient) doRequest(path string, opts ...poc.PocConfigOption) (*base.SpaceEngineResponse, error) {
	opts = append(opts, poc.WithSession(sessionKey), poc.WithDeleteHeader("User-Agent"))
	rsp, err := c.Get(path, opts...)
	if err != nil {
		return nil, err
	}
	err = checkError(rsp.StatusCode, rsp.Body)
	return rsp, err
}

func (c *ShodanClient) doRequestAndUnmarshal(path string, thing interface{}, opts ...poc.PocConfigOption) error {
	opts = append(opts, poc.WithSession(sessionKey), poc.WithDeleteHeader("User-Agent"))
	rsp, err := c.Get(path, opts...)
	if err != nil {
		return err
	}

	if err := checkError(rsp.StatusCode, rsp.Body); err != nil {
		return err
	}

	err = json.Unmarshal(rsp.Body, &thing)
	return err
}

func checkError(statusCode int, data []byte) error {
	if statusCode >= 300 {
		return errors.New("invalid status code")
	} else {
		result := gjson.ParseBytes(data)
		if errmsg := result.Get("error").String(); errmsg != "" {
			return utils.Errorf("invalid response : %s", errmsg)
		}
	}
	return nil
}
