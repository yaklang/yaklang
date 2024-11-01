package sfweb

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type InvalidSchemeError struct {
	scheme string
}

func (e *InvalidSchemeError) Error() string {
	return "unsupported scheme: " + e.scheme
}

func NewInvalidSchemeError(scheme string) *InvalidSchemeError {
	return &InvalidSchemeError{scheme: scheme}
}

type YakURLRequest struct {
	Body     string  `json:"body"`
	Method   Method  `json:"method"`
	Page     int64   `json:"page,omitempty"`
	Pagesize int64   `json:"pagesize,omitempty"`
	URL      *YakURL `json:"url"`
}

func (r *YakURLRequest) toYpb() *ypb.RequestYakURLParams {
	if r == nil {
		return nil
	}
	return &ypb.RequestYakURLParams{
		Body:     []byte(r.Body),
		Method:   string(r.Method),
		Page:     r.Page,
		PageSize: r.Pagesize,
		Url:      r.URL.toYpb(),
	}
}

type YakURL struct {
	FromRaw  string   `json:"from_raw,omitempty"`
	Location string   `json:"location"`
	Pass     string   `json:"pass,omitempty"`
	Path     string   `json:"path"`
	Query    []*Query `json:"query,omitempty"`
	Schema   string   `json:"schema"`
	User     string   `json:"user,omitempty"`
}

func ypbToYakURL(u *ypb.YakURL) *YakURL {
	if u == nil {
		return nil
	}
	query := lo.Map(u.Query, func(q *ypb.KVPair, _ int) *Query {
		return ypbToQuery(q)
	})
	return &YakURL{
		FromRaw:  u.FromRaw,
		Location: u.Location,
		Pass:     u.Pass,
		Query:    query,
		Schema:   u.Schema,
		User:     u.User,
	}
}

func (u *YakURL) toYpb() *ypb.YakURL {
	if u == nil {
		return nil
	}
	queryParams := lo.Map(u.Query, func(q *Query, _ int) *ypb.KVPair {
		return q.toYpb()
	})
	return &ypb.YakURL{
		FromRaw:  u.FromRaw,
		Schema:   u.Schema,
		Location: u.Location,
		Pass:     u.Pass,
		User:     u.User,
		Path:     u.Path,
		Query:    queryParams,
	}
}

type Query struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func ypbToQuery(q *ypb.KVPair) *Query {
	if q == nil {
		return nil
	}
	return &Query{
		Key:   q.Key,
		Value: q.Value,
	}
}

func (q *Query) toYpb() *ypb.KVPair {
	if q == nil {
		return nil
	}
	return &ypb.KVPair{
		Key:   q.Key,
		Value: q.Value,
	}
}

type YakURLResponse struct {
	Page      int64             `json:"page"`
	PageSize  int64             `json:"page_size"`
	Resources []*YakURLResource `json:"resources"`
	Total     int64             `json:"total"`
}

func ypbToYakURLResponse(r *ypb.RequestYakURLResponse) *YakURLResponse {
	if r == nil {
		return nil
	}
	resources := lo.Map(r.Resources, func(res *ypb.YakURLResource, _ int) *YakURLResource {
		return ypbToYakURLResource(res)
	})
	return &YakURLResponse{
		Page:      r.Page,
		PageSize:  r.PageSize,
		Resources: resources,
		Total:     r.Total,
	}
}

type YakURLResource struct {
	Extra             []*YakURLKVPair `json:"extra"`
	HaveChildrenNodes bool            `json:"have_children_nodes"`
	ModifiedTimestamp int64           `json:"modified_timestamp"`
	Path              string          `json:"path"`
	ResourceName      string          `json:"resource_name"`
	ResourceType      string          `json:"resource_type"`
	Size              int64           `json:"size"`
	URL               *YakURL         `json:"url"`
	VerboseName       string          `json:"verbose_name"`
	VerboseSize       string          `json:"verbose_size"`
	VerboseType       string          `json:"verbose_type"`
	YakURLVerbose     string          `json:"yak_url_verbose"`
}

func ypbToYakURLResource(r *ypb.YakURLResource) *YakURLResource {
	if r == nil {
		return nil
	}
	extra := lo.Map(r.Extra, func(e *ypb.KVPair, _ int) *YakURLKVPair {
		return ypbToYakURLKVPair(e)
	})
	return &YakURLResource{
		Extra:             extra,
		HaveChildrenNodes: r.HaveChildrenNodes,
		ModifiedTimestamp: r.ModifiedTimestamp,
		Path:              r.Path,
		ResourceName:      r.ResourceName,
		ResourceType:      r.ResourceType,
		Size:              r.Size,
		URL:               ypbToYakURL(r.Url),
		VerboseName:       r.VerboseName,
		VerboseSize:       r.SizeVerbose,
		VerboseType:       r.VerboseType,
		YakURLVerbose:     r.YakURLVerbose,
	}
}

type YakURLKVPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func ypbToYakURLKVPair(e *ypb.KVPair) *YakURLKVPair {
	if e == nil {
		return nil
	}
	return &YakURLKVPair{
		Key:   e.Key,
		Value: e.Value,
	}
}

type Method string

const (
	Delete Method = "DELETE"
	Get    Method = "GET"
	Head   Method = "HEAD"
	Post   Method = "POST"
	Put    Method = "PUT"
)

func (s *SyntaxFlowWebServer) registerYakURLRoute() {
	s.router.HandleFunc("/yakurl", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErrorJson(w, utils.Wrap(err, "read body error"))
			return
		}
		var req YakURLRequest
		if err = json.Unmarshal(body, &req); err != nil {
			writeErrorJson(w, utils.Wrap(err, "unmarshal request error"))
			return
		}

		fromRaw := req.URL.FromRaw
		if fromRaw != "" {
			u := utils.ParseStringToUrl(fromRaw)
			if u.Scheme != "syntaxflow" {
				writeErrorJson(w, NewInvalidSchemeError(u.Scheme))
				return
			}
		} else if req.URL.Schema != "syntaxflow" {
			writeErrorJson(w, NewInvalidSchemeError(req.URL.Schema))
			return
		}

		grpcRsp, err := s.grpcClient.RequestYakURL(r.Context(), req.toYpb())
		if err != nil {
			writeErrorJson(w, utils.Wrap(err, "grpc request error"))
			return
		}
		rsp := ypbToYakURLResponse(grpcRsp)
		writeJson(w, rsp)
	}).Name("yakurl").Methods(http.MethodPost, http.MethodOptions)
}
