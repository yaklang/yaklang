package suricata

import (
	"bytes"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/regen"
	"golang.org/x/exp/slices"
)

const Fallback Modifier = 1 << 20

var defaultRandom = map[Modifier]regen.Generator{
	HTTPMethod:   MustGenerator(`^(GET|POST|HEAD|PUT|DELETE|CONNECT|OPTIONS|TRACE|PATCH)`),
	HTTPUri:      MustGenerator(`^(\/[a-zA-Z0-9]{3,10}){2,5}`),
	HTTPStatCode: MustGenerator(`^[1-5][0-1][0-9]`),
	HTTPHost:     MustGenerator(`^([a-zA-Z0-9]{3,6}\.){2,}[a-zA-Z0-9]{3,6}`),
	HTTPProtocol: MustGenerator(`^HTTP\/1\.1`),
	Fallback:     MustGenerator(`[a-zA-Z0-9]{6,10}`),
}

func MustGenerator(expr string) regen.Generator {
	generator, err := regen.NewGeneratorOne(expr, nil)
	if err != nil {
		panic(err)
	}
	return generator
}

func HTTPCombination(mp map[Modifier][]byte) []byte {
	for k := range mp {
		if slices.Index(HTTP_REQ_ONLY, k) != -1 {
			return httpreqCombination(mp)
		}
		if slices.Index(HTTP_RESP_ONLY, k) != -1 {
			return httprespCombination(mp)
		}
	}

	if mp[HTTPStart] != nil {
		if bytes.HasPrefix(mp[HTTPStart], []byte("HTTP/")) {
			return httprespCombination(mp)
		}
		return httpreqCombination(mp)
	}

	if mp[HTTPHeader] != nil {
		if bytes.Contains(mp[HTTPHeader], []byte("Host: ")) {
			return httpreqCombination(mp)
		}
	}

	if mp[HTTPHeaderRaw] != nil {
		if bytes.Contains(mp[HTTPHeader], []byte("Host: ")) {
			return httpreqCombination(mp)
		}
	}

	if randBool() {
		return httpreqCombination(mp)
	}
	return httprespCombination(mp)
}

func httpreqCombination(mp map[Modifier][]byte) []byte {
	var buf bytes.Buffer

	p := partProvider{mp}
	p.FillHTTPRequestLine(&buf)
	p.FillHTTPRequestHeader(&buf)
	buf.WriteString(lowhttp.CRLF)
	p.FillHTTPRequestBody(&buf)

	return buf.Bytes()
}

func httprespCombination(mp map[Modifier][]byte) []byte {
	return nil
}

type partProvider struct {
	mp map[Modifier][]byte
}

type header struct {
	header Modifier
	prefix []byte
}

func (p *partProvider) getOrRandom(part Modifier) []byte {
	if p.mp[part] != nil {
		return p.mp[part]
	}
	if defaultRandom[part] != nil {
		return []byte(defaultRandom[part].Generate()[0])
	}
	return []byte(defaultRandom[Fallback].Generate()[0])
}

func (p *partProvider) FillHTTPRequestLine(w *bytes.Buffer) {
	if p.mp[HTTPRequestLine] != nil {
		_, _ = w.Write(p.mp[HTTPRequestLine])
		if !bytes.HasSuffix(p.mp[HTTPRequestLine], []byte(lowhttp.CRLF)) {
			_, _ = w.WriteString(lowhttp.CRLF)
		}
		return
	}
	if p.mp[HTTPStart] != nil {
		_, _ = w.Write(p.mp[HTTPStart])
		if !bytes.HasSuffix(p.mp[HTTPRequestLine], []byte(lowhttp.CRLF)) {
			_, _ = w.WriteString(lowhttp.CRLF)
		}
		return
	}

	// manually fill
	_, _ = w.Write(p.getOrRandom(HTTPMethod))
	_ = w.WriteByte(' ')
	if p.mp[HTTPUriRaw] != nil {
		_, _ = w.Write(p.mp[HTTPUriRaw])
	} else {
		_, _ = w.Write(p.getOrRandom(HTTPUri))
	}
	_ = w.WriteByte(' ')
	_, _ = w.Write(p.getOrRandom(HTTPProtocol))

	_, _ = w.WriteString(lowhttp.CRLF)
}

func (p *partProvider) FillHTTPRequestHeader(w *bytes.Buffer) {
	var buf []byte
	if p.mp[HTTPHeaderRaw] != nil {
		w.Write(p.mp[HTTPHeaderRaw])
		buf = p.mp[HTTPHeaderRaw]
	} else if p.mp[HTTPHeader] != nil {
		w.Write(p.mp[HTTPHeader])
		buf = p.mp[HTTPHeader]
	}
	if buf != nil && !bytes.HasSuffix(buf, []byte(lowhttp.CRLF)) {
		_, _ = w.WriteString(lowhttp.CRLF)
	}

	// try best to fill, may be not correct

	if !bytes.Contains(buf, []byte("Host: ")) {
		_, _ = w.WriteString("Host: ")
		if p.mp[HTTPHostRaw] != nil {
			_, _ = w.Write(p.mp[HTTPHostRaw])
		} else {
			_, _ = w.Write(p.getOrRandom(HTTPHost))
		}
		_, _ = w.WriteString(lowhttp.CRLF)
	}

	var tryheaders = []header{
		{HTTPCookie, []byte("Cookie")},
		{HTTPUserAgent, []byte("User-Agent")},
		{HTTPReferer, []byte("Referer")},
		{HTTPAccept, []byte("Accept")},
		{HTTPAcceptLang, []byte("Accept-Language")},
		{HTTPAcceptEnc, []byte("Accept-Encoding")},
		{HTTPConnection, []byte("Connection")},
		{HTTPContentType, []byte("Content-Type")},
		{HTTPContentLen, []byte("Content-Length")},
	}

	if p.mp[HTTPHeaderNames] != nil {
		for _, hdr := range bytes.Fields(p.mp[HTTPHeaderNames]) {
			for _, try := range tryheaders {
				if bytes.EqualFold(try.prefix, hdr) {
					continue
				}
			}
			tryheaders = append(tryheaders, header{Fallback, hdr})
		}
	}

	for _, try := range tryheaders {
		if p.mp[try.header] != nil {
			indexes := bytesIndexAll(buf, append(try.prefix, []byte(": ")...), true)
			for _, index := range indexes {
				if index.pos == 0 || index.pos > 1 && bytes.Equal(buf[index.pos-2:index.pos], []byte(lowhttp.CRLF)) {
					continue
				}
			}
			_, _ = w.Write(try.prefix)
			_, _ = w.WriteString(": ")
			_, _ = w.Write(p.mp[try.header])
			_, _ = w.WriteString(lowhttp.CRLF)
		}
	}
}

func (p *partProvider) FillHTTPRequestBody(w *bytes.Buffer) {
	for _, v := range []Modifier{HTTPRequestBody, FileData, Default} {
		if p.mp[v] != nil {
			_, _ = w.Write(p.mp[v])
			if !bytes.HasSuffix(p.mp[v], []byte(lowhttp.CRLF)) {
				_, _ = w.WriteString(lowhttp.CRLF)
			}
			return
		}
	}
	return
}
