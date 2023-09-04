package generate

import (
	"bytes"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/regen"
	"golang.org/x/exp/slices"
	"net/http"
	"strconv"
	"time"
)

const Fallback modifier.Modifier = 1 << 20

var defaultRandom = map[modifier.Modifier]regen.Generator{
	modifier.HTTPMethod:   MustGenerator(`^(GET|POST|HEAD|PUT|DELETE|CONNECT|OPTIONS|TRACE|PATCH)`),
	modifier.HTTPUri:      MustGenerator(`^(\/[a-zA-Z0-9]{3,10}){2,5}`),
	modifier.HTTPStatCode: MustGenerator(`^[1-5][0-1][0-9]`),
	modifier.HTTPStatMsg:  MustGenerator(`^[A-Z][a-zA-Z]{2,5}`),
	modifier.HTTPHost:     MustGenerator(`^([a-zA-Z0-9]{3,6}\.){2,}[a-zA-Z0-9]{3,6}`),
	modifier.HTTPProtocol: MustGenerator(`^HTTP\/1\.1`),
	Fallback:              MustGenerator(`[a-zA-Z0-9]{6,10}`),
}

func MustGenerator(expr string) regen.Generator {
	generator, err := regen.NewGeneratorOne(expr, nil)
	if err != nil {
		panic(err)
	}
	return generator
}

func HTTPCombination(mp map[modifier.Modifier][]byte) []byte {
	for k := range mp {
		if slices.Index(modifier.HTTP_REQ_ONLY, k) != -1 {
			return httpreqCombination(mp)
		}
		if slices.Index(modifier.HTTP_RESP_ONLY, k) != -1 {
			return httprespCombination(mp)
		}
	}

	if mp[modifier.HTTPStart] != nil {
		if bytes.HasPrefix(mp[modifier.HTTPStart], []byte("HTTP/")) {
			return httprespCombination(mp)
		}
		return httpreqCombination(mp)
	}

	if mp[modifier.HTTPHeader] != nil {
		if bytes.Contains(mp[modifier.HTTPHeader], []byte("Host: ")) {
			return httpreqCombination(mp)
		}
	}

	if mp[modifier.HTTPHeaderRaw] != nil {
		if bytes.Contains(mp[modifier.HTTPHeader], []byte("Host: ")) {
			return httpreqCombination(mp)
		}
	}

	if randBool() {
		return httpreqCombination(mp)
	}
	return httprespCombination(mp)
}

func httpreqCombination(mp map[modifier.Modifier][]byte) []byte {
	var buf bytes.Buffer

	p := partProvider{mp}
	p.FillHTTPRequestLine(&buf)
	p.FillHTTPRequestHeader(&buf)
	buf.WriteString(lowhttp.CRLF)
	p.FillHTTPRequestBody(&buf)

	return buf.Bytes()
}

func httprespCombination(mp map[modifier.Modifier][]byte) []byte {
	var buf bytes.Buffer

	p := partProvider{mp}
	p.FillHTTPResponseLine(&buf)
	p.FillHTTPResponseHeader(&buf)
	buf.WriteString(lowhttp.CRLF)
	p.FillHTTPResponseBody(&buf)

	return buf.Bytes()
}

type partProvider struct {
	mp map[modifier.Modifier][]byte
}

type header struct {
	header modifier.Modifier
	prefix []byte
}

func (p *partProvider) getOrRandom(part modifier.Modifier) []byte {
	if p.mp[part] != nil {
		return p.mp[part]
	}
	if defaultRandom[part] != nil {
		return []byte(defaultRandom[part].Generate()[0])
	}
	return []byte(defaultRandom[Fallback].Generate()[0])
}

func (p *partProvider) FillHTTPRequestLine(w *bytes.Buffer) {
	if p.mp[modifier.HTTPRequestLine] != nil {
		_, _ = w.Write(p.mp[modifier.HTTPRequestLine])
		if !bytes.HasSuffix(p.mp[modifier.HTTPRequestLine], []byte(lowhttp.CRLF)) {
			_, _ = w.WriteString(lowhttp.CRLF)
		}
		return
	}
	if p.mp[modifier.HTTPStart] != nil {
		_, _ = w.Write(p.mp[modifier.HTTPStart])
		if !bytes.HasSuffix(p.mp[modifier.HTTPRequestLine], []byte(lowhttp.CRLF)) {
			_, _ = w.WriteString(lowhttp.CRLF)
		}
		return
	}

	// manually fill
	_, _ = w.Write(p.getOrRandom(modifier.HTTPMethod))
	_ = w.WriteByte(' ')
	if p.mp[modifier.HTTPUriRaw] != nil {
		_, _ = w.Write(p.mp[modifier.HTTPUriRaw])
	} else {
		_, _ = w.Write(p.getOrRandom(modifier.HTTPUri))
	}
	_ = w.WriteByte(' ')
	_, _ = w.Write(p.getOrRandom(modifier.HTTPProtocol))

	_, _ = w.WriteString(lowhttp.CRLF)
}

func (p *partProvider) fillHTTPHeaderBackground(w *bytes.Buffer) {
	if p.mp[modifier.HTTPHeaderRaw] != nil {
		w.Write(p.mp[modifier.HTTPHeaderRaw])
	} else if p.mp[modifier.HTTPHeader] != nil {
		w.Write(p.mp[modifier.HTTPHeader])
	}
	if !bytes.HasSuffix(w.Bytes(), []byte(lowhttp.CRLF)) {
		_, _ = w.WriteString(lowhttp.CRLF)
	}
}

func (p *partProvider) fillHTTPHeaderOthersIfExisted(w *bytes.Buffer, headers []header) {
	var mustfill []header

	// header Modifiers (check existed with list headers) -> mustfill
	for _, try := range headers {
		if p.mp[try.header] == nil {
			continue
		}

		for _, index := range bytesIndexAll(w.Bytes(), append(try.prefix, []byte(": ")...), true) {
			if index.Pos == 0 || index.Pos > 1 && bytes.Equal(w.Bytes()[index.Pos-2:index.Pos], []byte(lowhttp.CRLF)) {
				continue
			}
		}
		mustfill = append(mustfill, try)
	}

	// headernames Modifier -> mustfill
	if p.mp[modifier.HTTPHeaderNames] != nil {
		for _, hdr := range bytes.Fields(p.mp[modifier.HTTPHeaderNames]) {
			add := true

			// no add if existed in mustfill
			for _, inorder := range mustfill {
				if bytes.EqualFold(inorder.prefix, hdr) {
					add = false
				}
			}

			// no add if existed in buf
			for _, index := range bytesIndexAll(w.Bytes(), append(hdr, []byte(": ")...), true) {
				if index.Pos == 0 || index.Pos > 1 && bytes.Equal(w.Bytes()[index.Pos-2:index.Pos], []byte(lowhttp.CRLF)) {
					add = false
				}
			}

			if add {
				mustfill = append(mustfill, header{Fallback, hdr})
			}
		}
	}

	for _, try := range mustfill {
		_, _ = w.Write(try.prefix)
		_, _ = w.WriteString(": ")
		_, _ = w.Write(p.getOrRandom(try.header))
		_, _ = w.WriteString(lowhttp.CRLF)
	}
}

func (p *partProvider) FillHTTPRequestHeader(w *bytes.Buffer) {
	p.fillHTTPHeaderBackground(w)

	// try best to fill, may be not correct
	if !bytes.Contains(w.Bytes(), []byte("Host: ")) {
		_, _ = w.WriteString("Host: ")
		if p.mp[modifier.HTTPHostRaw] != nil {
			_, _ = w.Write(p.mp[modifier.HTTPHostRaw])
		} else {
			_, _ = w.Write(p.getOrRandom(modifier.HTTPHost))
		}
		_, _ = w.WriteString(lowhttp.CRLF)
	}

	p.fillHTTPHeaderOthersIfExisted(w, []header{
		{modifier.HTTPCookie, []byte("Cookie")},
		{modifier.HTTPUserAgent, []byte("User-Agent")},
		{modifier.HTTPReferer, []byte("Referer")},
		{modifier.HTTPAccept, []byte("Accept")},
		{modifier.HTTPAcceptLang, []byte("Accept-Language")},
		{modifier.HTTPAcceptEnc, []byte("Accept-Encoding")},
		{modifier.HTTPConnection, []byte("Connection")},
		{modifier.HTTPContentType, []byte("Content-Type")},
		{modifier.HTTPContentLen, []byte("Content-Length")},
	})
}

func (p *partProvider) FillHTTPRequestBody(w *bytes.Buffer) {
	for _, v := range []modifier.Modifier{modifier.HTTPRequestBody, modifier.FileData, modifier.Default} {
		if p.mp[v] != nil {
			_, _ = w.Write(p.mp[v])
		}
	}
	return
}

func (p *partProvider) FillHTTPResponseLine(w *bytes.Buffer) {
	if p.mp[modifier.HTTPResponseLine] != nil {
		_, _ = w.Write(p.mp[modifier.HTTPResponseLine])
		if !bytes.HasSuffix(p.mp[modifier.HTTPResponseLine], []byte(lowhttp.CRLF)) {
			_, _ = w.WriteString(lowhttp.CRLF)
		}
		return
	}
	if p.mp[modifier.HTTPStart] != nil {
		_, _ = w.Write(p.mp[modifier.HTTPStart])
		if !bytes.HasSuffix(p.mp[modifier.HTTPStart], []byte(lowhttp.CRLF)) {
			_, _ = w.WriteString(lowhttp.CRLF)
		}
		return
	}

	_, _ = w.Write(p.getOrRandom(modifier.HTTPProtocol))
	_ = w.WriteByte(' ')
	codebytes := p.getOrRandom(modifier.HTTPStatCode)
	_, _ = w.Write(codebytes)
	_ = w.WriteByte(' ')

	code, _ := strconv.Atoi(string(codebytes))
	if p.mp[modifier.HTTPStatMsg] == nil && http.StatusText(code) != "" {
		_, _ = w.WriteString(http.StatusText(code))
	} else {
		_, _ = w.Write(p.getOrRandom(modifier.HTTPStatMsg))
	}

	_, _ = w.WriteString(lowhttp.CRLF)
}

func (p *partProvider) FillHTTPResponseHeader(w *bytes.Buffer) {
	p.fillHTTPHeaderBackground(w)

	if !bytes.Contains(w.Bytes(), []byte("Date: ")) {
		_, _ = w.WriteString("Date: ")
		_, _ = w.WriteString(time.Now().Format(time.RFC1123))
		_, _ = w.WriteString(lowhttp.CRLF)
	}

	p.fillHTTPHeaderOthersIfExisted(w, []header{
		{modifier.HTTPServer, []byte("Server")},
		{modifier.HTTPLocation, []byte("Location")},
		{modifier.HTTPCookie, []byte("Set-Cookie")},
		{modifier.HTTPContentType, []byte("Content-Type")},
		{modifier.HTTPContentLen, []byte("Content-Length")},
		{modifier.HTTPConnection, []byte("Connection")},
	})
}

func (p *partProvider) FillHTTPResponseBody(w *bytes.Buffer) {
	for _, v := range []modifier.Modifier{modifier.HTTPResponseBody, modifier.FileData, modifier.Default} {
		if p.mp[v] != nil {
			_, _ = w.Write(p.mp[v])
		}
	}
	return
}
