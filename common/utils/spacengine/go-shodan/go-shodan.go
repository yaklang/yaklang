/*Package shodan is an interface for the Shodan API*/
package shodan

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// APIHost is the URL of the Shodan API.
// Debug toggles debug information.
var (
	APIHost        = "https://api.shodan.io"
	ExploitAPIHost = "https://exploits.shodan.io"
)

// Client stores shared data that is used to interact with the API.
// Key is our Shodan API Key.
type Client struct {
	Key string
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
		Product string `json:"product"`
		Title   string `json:"title"`
		Opts    struct {
		} `json:"opts"`
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

// New returns a new Client.
func New(key string) *Client {
	return &Client{
		Key: key,
	}
}

// Host calls '/shodan/host/{ip}' and returns the unmarshalled response.
// ip is the IP address to search for.
// opts are all query paramters to pass in the request. You do not have to provide your API key.
func (c *Client) Host(ip string, opts url.Values) (*Host, error) {
	h := &Host{}
	opts.Set("key", c.Key)
	req, err := http.NewRequest("GET", APIHost+"/shodan/host/"+ip+"?"+opts.Encode(), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return h, err
	}
	err = doRequestAndUnmarshal(req, &h)
	return h, err
}

// HostCount calls '/shodan/host/count' and returns the unmarshalled response.
// query is the search query to pass in the request.
// facets are any facets to pass in the request.
func (c *Client) HostCount(query string, facets []string) (*HostCount, error) {
	h := &HostCount{}
	opts := url.Values{}
	opts.Set("key", c.Key)
	opts.Set("facets", strings.Join(facets, ","))
	opts.Set("query", query)
	req, err := http.NewRequest("GET", APIHost+"/shodan/host/count?"+opts.Encode(), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return h, err
	}
	err = doRequestAndUnmarshal(req, &h)
	return h, err
}

// HostSearch calls '/shodan/host/search' and returns the unmarshalled response.
// query is the search query to pass in the request.
// facets are any facets to pass in the request.
// opts are any additional query parameters to set, such as page and minify.
func (c *Client) HostSearch(query string, facets []string, opts url.Values) (*HostSearch, error) {
	h := &HostSearch{}
	opts.Set("key", c.Key)
	opts.Set("facets", strings.Join(facets, ","))
	opts.Set("query", query)
	req, err := http.NewRequest("GET", APIHost+"/shodan/host/search?"+opts.Encode(), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return h, err
	}
	err = doRequestAndUnmarshal(req, &h)
	return h, err
}

// HostSearchTokens calls '/shodan/host/search/tokens' and returns the unmarshalled response.
// query is the search query to pass in the request.
func (c *Client) HostSearchTokens(query string) (*HostSearchTokens, error) {
	h := &HostSearchTokens{}
	opts := url.Values{}
	opts.Set("key", c.Key)
	opts.Set("query", query)
	req, err := http.NewRequest("GET", APIHost+"/shodan/host/search/tokens?"+opts.Encode(), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return h, err
	}
	err = doRequestAndUnmarshal(req, &h)
	return h, err
}

// Protocols calls '/shodan/protocols' and returns the unmarshalled response.
func (c *Client) Protocols() (map[string]string, error) {
	m := make(map[string]string)
	opts := url.Values{}
	opts.Set("key", c.Key)
	req, err := http.NewRequest("GET", APIHost+"/shodan/protocols?"+opts.Encode(), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return m, err
	}
	err = doRequestAndUnmarshal(req, &m)
	return m, err
}

// Services calls '/shodan/services' and returns the unmarshalled response.
func (c *Client) Services() (map[string]string, error) {
	m := make(map[string]string)
	opts := url.Values{}
	opts.Set("key", c.Key)
	req, err := http.NewRequest("GET", APIHost+"/shodan/services?"+opts.Encode(), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return m, err
	}
	err = doRequestAndUnmarshal(req, &m)
	return m, err
}

// Query calls '/shodan/query' and returns the unmarshalled response.
// opts are additional query parameters. You do not need to provide your API key.
func (c *Client) Query(opts url.Values) (*Query, error) {
	q := &Query{}
	opts.Set("key", c.Key)
	req, err := http.NewRequest("GET", APIHost+"/shodan/query?"+opts.Encode(), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return q, err
	}
	err = doRequestAndUnmarshal(req, &q)
	return q, err
}

// QuerySearch calls '/shodan/query/search' and returns the unmarshalled response.
// query is the search query to pass in the request.
// opts are additional query parameters. You do not need to provide your API key.
func (c *Client) QuerySearch(query string, opts url.Values) (*Query, error) {
	q := &Query{}
	opts.Set("key", c.Key)
	opts.Set("query", query)
	req, err := http.NewRequest("GET", APIHost+"/shodan/query/search?"+opts.Encode(), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return q, err
	}
	err = doRequestAndUnmarshal(req, &q)
	return q, err
}

// QueryTags calls '/shodan/query/tags' and returns the unmarshalled response.
// opts are additional query parameters. You do not need to provide your API key.
func (c *Client) QueryTags(opts url.Values) (*QueryTags, error) {
	q := &QueryTags{}
	opts.Set("key", c.Key)
	req, err := http.NewRequest("GET", APIHost+"/shodan/query/tags?"+opts.Encode(), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return q, err
	}
	err = doRequestAndUnmarshal(req, &q)
	return q, err
}

// APIInfo calls '/api-info' and returns the unmarshalled response.
func (c *Client) APIInfo() (*APIInfo, error) {
	i := &APIInfo{}
	opts := url.Values{}
	opts.Set("key", c.Key)
	req, err := http.NewRequest("GET", APIHost+"/api-info?"+opts.Encode(), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return i, err
	}
	err = doRequestAndUnmarshal(req, &i)
	return i, err
}

// DNSResolve calls '/dns/resolve' and returns the unmarshalled response.
func (c *Client) DNSResolve(hostnames []string) ([]DNSResolve, error) {
	d := []DNSResolve{}
	req, err := http.NewRequest("GET", APIHost+"/dns/resolve?key="+c.Key+"&hostnames="+strings.Join(hostnames, ","), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return d, err
	}
	m := make(map[string]string)
	if err := doRequestAndUnmarshal(req, &m); err != nil {
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
func (c *Client) DNSReverse(ips []string) ([]DNSReverse, error) {
	d := []DNSReverse{}
	req, err := http.NewRequest("GET", APIHost+"/dns/reverse?key="+c.Key+"&ips="+strings.Join(ips, ","), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return d, err
	}
	m := make(map[string][]string)
	if err := doRequestAndUnmarshal(req, &m); err != nil {
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
func (c *Client) Exploits(query string, facets []string) (*Exploit, error) {
	e := &Exploit{}
	opts := url.Values{}
	opts.Set("key", c.Key)
	opts.Set("facets", strings.Join(facets, ","))
	opts.Set("query", query)
	req, err := http.NewRequest("GET", ExploitAPIHost+"/api/search?"+opts.Encode(), nil)
	debug("GET " + req.URL.String())
	if err != nil {
		return e, err
	}
	err = doRequestAndUnmarshal(req, &e)
	return e, err
}

func doRequestAndUnmarshal(req *http.Request, thing interface{}) error {
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := checkError(resp, data); err != nil {
		return err
	}
	err = json.Unmarshal(data, &thing)
	return err
}

func checkError(resp *http.Response, data []byte) error {
	if resp.StatusCode >= 300 {
		debug("Non 2XX response")
		e := &Error{}
		if err := json.Unmarshal(data, &e); err != nil {
			debug("Error parsing JSON")
			return err
		}
		return errors.New(e.Error)
	}
	return nil
}

func debug(msg string) {
	if os.Getenv("SHODAN_DEBUG") != "" {
		log.Println(msg)
	}
}
