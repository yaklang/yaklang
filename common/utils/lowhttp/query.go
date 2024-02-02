package lowhttp

import (
	"bufio"
	"bytes"
	"net/url"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func ForceStringToUrl(i string) *url.URL {
	ins, err := url.Parse(i)
	if err != nil {
		return utils.ParseStringToUrl(i)
	}
	return ins
}

func ExtractQueryFromRequestURI(i string) *QueryParams {
	return NewQueryParams(ForceStringToUrl(i).RawQuery)
}

type QueryParamItem struct {
	NoAutoEncode bool
	Raw          string
	Key          string
	Value        string
}

func (item *QueryParamItem) Encode() string {
	var buf bytes.Buffer
	if item.Key == "" && item.Value == "" {
		if item.NoAutoEncode {
			buf.WriteString(item.Raw)
		} else {
			buf.WriteString(codec.QueryEscape(item.Raw))
		}
		return buf.String()
	}

	if item.NoAutoEncode {
		buf.WriteString(item.Key)
	} else {
		buf.WriteString(codec.QueryEscape(item.Key))
	}
	buf.WriteByte('=')
	if item.NoAutoEncode {
		buf.WriteString(item.Value)
	} else {
		buf.WriteString(codec.QueryEscape(item.Value))
	}
	return buf.String()
}

type QueryParams struct {
	NoAutoEncode bool
	Items        []*QueryParamItem
}

func NewQueryParams(s string) *QueryParams {
	return ParseQueryParams(s)
}

func ParseQueryParams(s string) *QueryParams {
	scanner := bufio.NewReaderSize(bytes.NewBufferString(s), len(s))
	var items []*QueryParamItem

	handle := func(pair []byte) {
		if len(pair) <= 0 {
			return
		}
		pair = bytes.Trim(pair, "&")
		key, val, ok := bytes.Cut(pair, []byte{'='})
		if ok {
			items = append(items, &QueryParamItem{
				Raw:   codec.ForceQueryUnescape(string(pair)),
				Key:   codec.ForceQueryUnescape(string(key)),
				Value: codec.ForceQueryUnescape(string(val)),
			})
		} else {
			items = append(items, &QueryParamItem{
				Raw: codec.ForceQueryUnescape(string(pair)),
			})
		}
	}

	for {
		pair, err := scanner.ReadBytes('&')
		if err != nil {
			handle(pair)
			break
		}
		handle(pair)
	}
	return &QueryParams{Items: items}
}

func (d *QueryParams) DisableAutoEncode(b bool) *QueryParams {
	if d != nil {
		d.NoAutoEncode = b
	}
	return d
}

func (q *QueryParams) Add(key, val string) {
	q.Items = append(q.Items, &QueryParamItem{Key: key, Value: val, NoAutoEncode: q.NoAutoEncode})
}

func (q *QueryParams) Set(key, val string) {
	for i := 0; i < len(q.Items); i++ {
		if q.Items[i].Key == key {
			q.Items[i].Value = val
			q.Items[i].NoAutoEncode = q.NoAutoEncode
			return
		}
	}
	q.Items = append(q.Items, &QueryParamItem{Key: key, Value: val, NoAutoEncode: q.NoAutoEncode})
}

func (q *QueryParams) Have(key string) bool {
	for _, item := range q.Items {
		if item.Key == key {
			return true
		}
	}
	return false
}

func (q *QueryParams) Remove(key string) {
	q.Items = lo.Filter(q.Items, func(item *QueryParamItem, _ int) bool {
		if item.Key == key {
			return false
		}
		return true
	})
}

func (q *QueryParams) Del(key string) {
	q.Remove(key)
}

func (q *QueryParams) Get(key string) string {
	for _, item := range q.Items {
		if item.Key == key {
			return item.Value
		}
	}
	return ""
}

func (q *QueryParams) AppendRaw(raw string) {
	q.Items = append(q.Items, &QueryParamItem{Raw: raw, NoAutoEncode: q.NoAutoEncode})
}

func (q *QueryParams) GetLast(key string) string {
	for i := len(q.Items) - 1; i >= 0; i-- {
		if q.Items[i].Key == key {
			return q.Items[i].Value
		}
	}
	return ""
}

func (q *QueryParams) GetAll(key string) []string {
	values := make([]string, 0, len(q.Items))
	for _, item := range q.Items {
		if item.Key == key {
			values = append(values, item.Value)
		}
	}
	return values
}

func (q *QueryParams) Encode() string {
	var buf bytes.Buffer
	for _, item := range q.Items {
		if buf.Len() > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(item.Encode())
	}
	return buf.String()
}

func (q *QueryParams) Clear() {
	q.Items = make([]*QueryParamItem, 0)
}
