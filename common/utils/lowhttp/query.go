package lowhttp

import (
	"bufio"
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type HttpParamPositionType string

var (
	PosMethod              HttpParamPositionType = "method"
	PosBody                HttpParamPositionType = "body"
	PosGetQuery            HttpParamPositionType = "get-query"
	PosGetQueryBase64      HttpParamPositionType = "get-query-base64"
	PosGetQueryJson        HttpParamPositionType = "get-query-json"
	PosGetQueryBase64Json  HttpParamPositionType = "get-query-base64-json"
	PosPath                HttpParamPositionType = "path"
	PosHeader              HttpParamPositionType = "header"
	PosPostQuery           HttpParamPositionType = "post-query"
	PosPostXML             HttpParamPositionType = "post-xml"
	PosPostQueryBase64     HttpParamPositionType = "post-query-base64"
	PosPostQueryJson       HttpParamPositionType = "post-query-json"
	PosPostQueryBase64Json HttpParamPositionType = "post-query-base64-json"
	PosPostJson            HttpParamPositionType = "post-json"
	PosCookie              HttpParamPositionType = "cookie"
	PosCookieBase64        HttpParamPositionType = "cookie-base64"
	PosCookieJson          HttpParamPositionType = "cookie-json"
	PosCookieBase64Json    HttpParamPositionType = "cookie-base64-json"
	PosPathAppend          HttpParamPositionType = "path-append"
	PosPathBlock           HttpParamPositionType = "path-block"
)

func ForceStringToUrl(i string) *url.URL {
	ins, err := url.Parse(i)
	if err != nil {
		return utils.ParseStringToUrl(i)
	}
	return ins
}

func ExtractQueryFromRequestURI(i string) *QueryParams {
	return ParseQueryParams(ForceStringToUrl(i).RawQuery)
}

type QueryParamItem struct {
	NoAutoEncode    bool
	Raw             string
	Key             string
	Value, ValueRaw string
	Position        HttpParamPositionType
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

func (item *QueryParamItem) fuzzEncode() string {
	var buf bytes.Buffer
	format := "{{urlescape(%s)}}"
	if item.Key == "" && item.Value == "" {
		if item.NoAutoEncode {
			buf.WriteString(item.Raw)
		} else {
			if utils.NeedsURLEncoding(item.Raw) {
				buf.WriteString(fmt.Sprintf(format, item.Raw))
			} else {
				buf.WriteString(item.Raw)
			}
		}
		return buf.String()
	}

	if item.NoAutoEncode {
		buf.WriteString(item.Key)
	} else {
		if utils.NeedsURLEncoding(item.Key) {
			buf.WriteString(fmt.Sprintf(format, item.Key))
		} else {
			buf.WriteString(item.Key)
		}
	}
	buf.WriteByte('=')
	if item.NoAutoEncode {
		buf.WriteString(item.Value)
	} else {
		if utils.NeedsURLEncoding(item.Value) {
			buf.WriteString(fmt.Sprintf(format, item.Value))
		} else {
			buf.WriteString(item.Value)
		}
	}
	return buf.String()
}

type QueryParams struct {
	NoAutoEncode    bool
	friendlyDisplay bool
	Position        HttpParamPositionType
	Items           []*QueryParamItem
}

func NewQueryParams(options ...QueryOption) *QueryParams {
	query := &QueryParams{}

	for _, option := range options {
		option(query)
	}
	return query
}

type QueryOption func(q *QueryParams)

func ParseQueryParams(s string, options ...QueryOption) *QueryParams {
	query := &QueryParams{}

	for _, option := range options {
		option(query)
	}

	scanner := bufio.NewReaderSize(bytes.NewBufferString(s), len(s))
	var items []*QueryParamItem

	// 获取 position，如果有的话
	position := query.Position

	handle := func(pair string) {
		if len(pair) <= 0 {
			return
		}
		pair = strings.Trim(pair, "&")
		key, val, ok := strings.Cut(pair, "=")
		if ok {
			if strings.HasPrefix(key, "{{urlescape(") ||
				strings.HasPrefix(val, "{{urlescape(") {
				key = strings.TrimPrefix(key, "{{urlescape(")
				key = strings.TrimSuffix(key, ")}}")
				val = strings.TrimPrefix(val, "{{urlescape(")
				val = strings.TrimSuffix(val, ")}}")
				pair = fmt.Sprintf("%s=%s", key, val)
			}
			items = append(items, &QueryParamItem{
				Raw:          codec.ForceQueryUnescape(pair),
				Key:          codec.ForceQueryUnescape(key),
				Value:        codec.ForceQueryUnescape(val),
				ValueRaw:     val,
				Position:     position,
				NoAutoEncode: query.NoAutoEncode,
			})
		} else {
			items = append(items, &QueryParamItem{
				Raw:          codec.ForceQueryUnescape(pair),
				Key:          codec.ForceQueryUnescape(key),
				Position:     position,
				NoAutoEncode: query.NoAutoEncode,
			})
		}
	}

	for {
		pair, err := scanner.ReadString('&')
		if err != nil {
			handle(pair)
			break
		}
		handle(pair)
	}
	query.Items = items
	return query
}

func WithPosition(p HttpParamPositionType) QueryOption {
	return func(q *QueryParams) {
		q.Position = p
	}
}

func WithDisableAutoEncode(b bool) QueryOption {
	return func(q *QueryParams) {
		q.NoAutoEncode = b
	}
}

func WithFriendlyDisplay(b bool) QueryOption {
	return func(q *QueryParams) {
		q.friendlyDisplay = b
	}
}

func (q *QueryParams) DisableAutoEncode(b bool) *QueryParams {
	if q != nil {
		q.NoAutoEncode = b
	}
	return q
}

func (q *QueryParams) SetFriendlyDisplay(b bool) *QueryParams {
	if q != nil {
		q.friendlyDisplay = b
	}
	return q
}

func (q *QueryParams) SetPosition(p HttpParamPositionType) *QueryParams {
	if q != nil {
		q.Position = p
	}
	return q
}

func (q *QueryParams) Add(key, val string, forceNoEncode ...bool) {
	if q == nil {
		return
	}
	noAutoEncode := q.NoAutoEncode
	if len(forceNoEncode) > 0 {
		noAutoEncode = forceNoEncode[0]
	}
	q.Items = append(q.Items, &QueryParamItem{Key: key, Value: val, NoAutoEncode: noAutoEncode})
}

func (q *QueryParams) Set(key, val string, forceNoEncode ...bool) {
	if q == nil {
		return
	}
	noAutoEncode := q.NoAutoEncode
	if len(forceNoEncode) > 0 {
		noAutoEncode = forceNoEncode[0]
	}

	for i := 0; i < len(q.Items); i++ {
		if q.Items[i].Key == key && q.Items[i].Position == q.Position {
			q.Items[i].Value = val
			q.Items[i].NoAutoEncode = noAutoEncode
			q.Items[i].Position = q.Position
			return
		}
	}
	q.Items = append(q.Items,
		&QueryParamItem{
			Key:          key,
			Value:        val,
			NoAutoEncode: noAutoEncode,
			Position:     q.Position,
		},
	)
}

func (q *QueryParams) Have(key string) bool {
	if q == nil {
		return false
	}

	for _, item := range q.Items {
		if item.Key == key {
			return true
		}
	}
	return false
}

func (q *QueryParams) Remove(key string) {
	if q == nil {
		return
	}
	q.Items = lo.Filter(q.Items, func(item *QueryParamItem, _ int) bool {
		if item.Key == key {
			return false
		}
		return true
	})
}

func (q *QueryParams) Del(key string) {
	if q == nil {
		return
	}
	q.Remove(key)
}

func (q *QueryParams) Get(key string) string {
	if q == nil {
		return ""
	}
	for _, item := range q.Items {
		if item.Key == key {
			return item.Value
		}
	}
	return ""
}

func (q *QueryParams) AppendRaw(raw string) {
	if q == nil {
		return
	}

	q.Items = append(q.Items, &QueryParamItem{Raw: raw, NoAutoEncode: q.NoAutoEncode})
}

func (q *QueryParams) GetLast(key string) string {
	if q == nil {
		return ""
	}
	for i := len(q.Items) - 1; i >= 0; i-- {
		if q.Items[i].Key == key {
			return q.Items[i].Value
		}
	}
	return ""
}

func (q *QueryParams) GetAll(key string) []string {
	if q == nil {
		return nil
	}
	values := make([]string, 0, len(q.Items))
	for _, item := range q.Items {
		if item.Key == key {
			values = append(values, item.Value)
		}
	}
	return values
}

func (q *QueryParams) Encode() string {
	if q == nil {
		return ""
	}

	var buf bytes.Buffer
	for _, item := range q.Items {
		if buf.Len() > 0 {
			buf.WriteByte('&')
		}
		if q.friendlyDisplay {
			buf.WriteString(item.fuzzEncode())
		} else {
			buf.WriteString(item.Encode())
		}
	}
	return buf.String()
}

func (q *QueryParams) EncodeByPos(pos HttpParamPositionType) string {
	if q == nil {
		return ""
	}
	var buf bytes.Buffer
	for _, item := range q.Items {
		if item.Position == pos {
			if buf.Len() > 0 {
				buf.WriteByte('&')
			}
			if q.friendlyDisplay {
				buf.WriteString(item.fuzzEncode())
			} else {
				buf.WriteString(item.Encode())
			}
		}
	}
	return buf.String()
}

func (q *QueryParams) Clear() {
	if q == nil {
		return
	}
	q.Items = make([]*QueryParamItem, 0)
}

func (q *QueryParams) IsEmpty() bool {
	if q == nil {
		return true
	}
	return len(q.Items) == 0
}
