package har

import "time"

type HTTPArchive struct {
	Log *Log `json:"log"`
}

type Log struct {
	Version string   `json:"version"`
	Creator *Creator `json:"creator"`
	Pages   []*Pages `json:"pages"`
	Entries *Entries `json:"entries"`
}

type Entries struct {
	Entries                []*HAREntry
	entriesChannel         <-chan *HAREntry // use this first if exist
	marshalEntryCallback   func(*HAREntry)
	unmarshalEntryCallback func(*HAREntry) error
}

func (e *Entries) SetEntriesChannel(ch <-chan *HAREntry) {
	e.entriesChannel = ch
}

func (e *Entries) SetMarshalEntryCallback(fn func(*HAREntry)) {
	e.marshalEntryCallback = fn
}

type Creator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Pages struct {
	StartedDateTime time.Time   `json:"startedDateTime"`
	ID              string      `json:"id"`
	Title           string      `json:"title"`
	PageTimings     PageTimings `json:"pageTimings,omitempty"`
}

type PageTimings struct {
	OnContentLoad float64 `json:"onContentLoad"`
	OnLoad        float64 `json:"onLoad"`
}
type HAREntry struct {
	Request         *HARRequest  `json:"request"`
	Response        *HARResponse `json:"response"`
	ServerIPAddress string       `json:"serverIPAddress"`
}

type HARKVPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type HARHTTPParam struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
}

type HARHTTPPostData struct {
	MimeType string          `json:"mimeType"`
	Params   []*HARHTTPParam `json:"params"`
	Text     string          `json:"text"`
}

type HARRequest struct {
	Method      string           `json:"method"`
	URL         string           `json:"url"`
	HTTPVersion string           `json:"httpVersion"`
	QueryString []*HARKVPair     `json:"queryString"`
	Headers     []*HARKVPair     `json:"headers"`
	HeadersSize int              `json:"headersSize"`
	BodySize    int              `json:"bodySize"`
	PostData    *HARHTTPPostData `json:"postData"`
	Timings     *Timings         `json:"timings,omitempty"`
}

type HARHTTPContent struct {
	Size     int    `json:"size"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
	Encoding string `json:"encoding"`
}

type HARResponse struct {
	StatusCode  int             `json:"status"`
	StatusText  string          `json:"statusText"`
	HTTPVersion string          `json:"httpVersion"`
	Headers     []*HARKVPair    `json:"headers"`
	Cookies     []*HARKVPair    `json:"cookies"`
	Content     *HARHTTPContent `json:"content"`
	HeadersSize int             `json:"headersSize"`
	BodySize    int             `json:"bodySize"`
}

type Timings struct {
	Blocked                  float64 `json:"blocked"`
	DNS                      int     `json:"dns"`
	Ssl                      int     `json:"ssl"`
	Connect                  int     `json:"connect"`
	Send                     float64 `json:"send"`
	Wait                     float64 `json:"wait"`
	Receive                  float64 `json:"receive"`
	BlockedQueueing          float64 `json:"_blocked_queueing"`
	BlockedProxy             float64 `json:"_blocked_proxy"`
	WorkerStart              int     `json:"_workerStart"`
	WorkerReady              int     `json:"_workerReady"`
	WorkerFetchStart         int     `json:"_workerFetchStart"`
	WorkerRespondWithSettled int     `json:"_workerRespondWithSettled"`
}
