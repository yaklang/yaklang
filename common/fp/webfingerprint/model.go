package webfingerprint

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/pkg/errors"
	"regexp"
	"strings"
)

//////////////////////////////////////////////////////////////////////////////////////
//////////////////////////////////CPE MODEL///////////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////
type CPE struct {
	Part     string `yaml:"part,omitempty" json:"part"`
	Vendor   string `yaml:"vendor,omitempty" json:"vendor"`
	Product  string `yaml:"product,omitempty" json:"product"`
	Version  string `yaml:"version,omitempty" json:"version"`
	Update   string `yaml:"update,omitempty" json:"update"`
	Edition  string `yaml:"edition,omitempty" json:"edition"`
	Language string `yaml:"language,omitempty" json:"language"`
}

func (c *CPE) init() {
	if c.Part == "" {
		c.Part = "a"
	}

	setWildstart := func(raw *string) {
		if *raw == "" {
			*raw = "*"
		}
	}

	setWildstart(&c.Vendor)
	setWildstart(&c.Product)
	setWildstart(&c.Version)
	setWildstart(&c.Update)
	setWildstart(&c.Edition)
	setWildstart(&c.Language)
}

func (c *CPE) String() string {
	c.init()
	raw := fmt.Sprintf("cpe:/%s:%s:%s:%s:%s:%s:%s", c.Part, c.Vendor, c.Product, c.Version, c.Update, c.Edition, c.Language)
	raw = strings.ReplaceAll(raw, " ", "_")
	raw = strings.ToLower(raw)
	return raw
}

func (c *CPE) LikeSearchString() string {
	c.init()

	cpe := "cpe:/" + c.Part

	concat := func(cpe string, next string) string {
		if next != "*" && next != "" {
			return cpe + ":" + next
		} else {
			if strings.HasSuffix(cpe, "%") {
				return cpe
			} else {
				return cpe + ":" + "%"
			}
		}
	}
	for _, r := range []string{c.Vendor, c.Product, c.Version, c.Update, c.Edition, c.Language} {
		cpe = concat(cpe, r)
	}

	if !strings.HasSuffix(cpe, "%") {
		cpe += "%"
	}

	if !strings.HasPrefix(cpe, "%") {
		cpe = "%" + cpe
	}

	if strings.HasSuffix(cpe, ":%") {
		cpe = cpe[:len(cpe)-2] + "%"
	}
	return strings.ToLower(cpe)
}

//////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////Keyword Matcher Model////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////
type KeywordMatcher struct {
	CPE `yaml:"cpe,inline,omitempty"`

	regexp *regexp.Regexp

	Regexp        string `yaml:"regexp,omitempty"`
	VendorIndex   int    `yaml:"vendor_index,omitempty"`
	ProductIndex  int    `yaml:"product_index,omitempty"`
	VersionIndex  int    `yaml:"version_index,omitempty"`
	UpdateIndex   int    `yaml:"update_index,omitempty"`
	EditionIndex  int    `yaml:"edition_index,omitempty"`
	LanguageIndex int    `yaml:"language_index,omitempty"`
}

func (k *KeywordMatcher) Match(raw string) (*CPE, error) {
	var err error

	if k.regexp == nil {
		if k.Regexp == "" {
			return nil, errors.New("empty regexp")
		}

		k.regexp, err = regexp.Compile(k.Regexp)
		if err != nil {
			return nil, errors.Errorf("compile [%s] to re failed: %s", k.Regexp, err)
		}
	}

	saveToCPE := func(dst *string, list []string, index int) {
		if len(list) <= index || index <= 0 {
			return
		}
		*dst = list[index]
	}

	for _, r := range k.regexp.FindAllStringSubmatch(raw, 1) {
		saveToCPE(&k.Vendor, r, k.VendorIndex)
		saveToCPE(&k.Product, r, k.ProductIndex)
		saveToCPE(&k.Version, r, k.VersionIndex)
		saveToCPE(&k.Edition, r, k.EditionIndex)
		saveToCPE(&k.Update, r, k.UpdateIndex)
		saveToCPE(&k.Language, r, k.LanguageIndex)

		return &k.CPE, nil
	}
	return nil, errors.New("no matched")
}

//////////////////////////////////////////////////////////////////////////////////////
//////////////////////////////HTTPHeader Matcher Model////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////
type HTTPHeaderMatcher struct {
	HeaderName  string         `yaml:"key"`
	HeaderValue KeywordMatcher `yaml:"value"`
}

func (h *HTTPHeaderMatcher) String() string {
	return fmt.Sprintf("%v: %v", h.HeaderValue, h.HeaderValue.Regexp)
}

func (h *HTTPHeaderMatcher) Match(name string, value string) (*CPE, error) {
	if h.HeaderName != "" {
		if name != h.HeaderName {
			return nil, errors.Errorf("not matched in header name: %s", name)
		}
	}
	return h.HeaderValue.Match(value)
}

//////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////MD5 Matcher Model////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////
type MD5Matcher struct {
	CPE

	md5 []byte
	MD5 string `yaml:"md5"`
}

func (h *MD5Matcher) Match(raw []byte) (*CPE, error) {
	rawMd5 := md5.Sum(raw)

	if len(h.md5) <= 0 {
		bytes, err := hex.DecodeString(h.MD5)
		if err != nil {
			return nil, errors.Errorf("parse md5 failed: %s", err)
		}

		if len(bytes) != 16 {
			return nil, errors.New("bad md5")
		}

		h.md5 = bytes
	}

	for i := range rawMd5 {
		if rawMd5[i] != h.md5[i] {
			return nil, errors.New("no matched")
		}
	}

	return &h.CPE, nil
}

//////////////////////////////////////////////////////////////////////////////////////
///////////////////////////WebMatcherMethods Model ///////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////
type WebMatcherMethods struct {
	Keywords    []*KeywordMatcher    `yaml:"keywords,omitempty"`
	HTTPHeaders []*HTTPHeaderMatcher `yaml:"headers,omitempty"`
	MD5s        []*MD5Matcher        `yaml:"md5s,omitempty"`
}

//////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////////////Web Rule/////////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////
type WebRule struct {
	Path     string `yaml:"path,omitempty"`
	Methods  []*WebMatcherMethods
	NextStep *WebRule `yaml:"next,omitempty"`
}

func (w *WebRule) IsActiveToProbePath() bool {
	if w.Path != "" {
		return true
	}
	return false
}

//////////////////////////////////////////////////////////////////////////////////////
//////////////////////////////Service Rule Model /////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////
type TransportProto string

var (
	TCP TransportProto = "tcp"
	UDP TransportProto = "udp"
)

type ServiceRule struct {
	Probe    string            `yaml:"probe,omitempty"`
	Proto    TransportProto    `yaml:"proto,omitempty"`
	Keywords []*KeywordMatcher `yaml:"keywords,omitempty"`
}

func ParseToCPE(cpe string) (*CPE, error) {
	if (!strings.HasPrefix(cpe, "cpe:/")) && (!strings.HasPrefix(cpe, "cpe:2.3:")) {
		return nil, errors.Errorf("raw [%s] is not a valid cpe", cpe)
	}

	if strings.HasPrefix(cpe, "cpe:2.3:") {
		cpe = strings.Replace(cpe, "cpe:2.3:", "cpe:/", 1)
	}

	var cpeArgs [7]string
	s := strings.Split(cpe, ":")
	for i := 1; i <= len(s)-1; i++ {
		var ret = strings.ReplaceAll(s[i], "%", "")
		cpeArgs[i-1] = ret
		if i == 7 {
			break
		}
	}
	cpeArgs[0] = cpeArgs[0][1:]
	cpeModel := &CPE{
		Part:     cpeArgs[0],
		Vendor:   cpeArgs[1],
		Product:  cpeArgs[2],
		Version:  cpeArgs[3],
		Update:   cpeArgs[4],
		Edition:  cpeArgs[5],
		Language: cpeArgs[6],
	}
	cpeModel.init()
	return cpeModel, nil
}
