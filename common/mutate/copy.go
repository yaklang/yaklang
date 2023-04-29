package mutate

import (
	"encoding/json"
	"net/http"
	"net/textproto"
	"net/url"
	"yaklang/common/utils"
	"sync"
)

func deepCopyHeader(h http.Header) (http.Header, error) {
	var newHeaders = make(http.Header)
	raw, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &newHeaders)
	if err != nil {
		return nil, err
	}
	return newHeaders, nil
}

func deepCopyMIMEHeader(h textproto.MIMEHeader) (textproto.MIMEHeader, error) {
	r, err := deepCopyHeader(http.Header(h))
	if err != nil {
		return nil, err
	}
	return textproto.MIMEHeader(r), nil
}

func deepCopyUrlValues(h url.Values) (url.Values, error) {
	var newValues = make(url.Values)
	raw, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &newValues)
	if err != nil {
		return nil, err
	}
	return newValues, nil
}

func deepCopyMapRaw(h map[string]interface{}) (map[string]interface{}, error) {
	var newValues = make(map[string]interface{})
	raw, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &newValues)
	if err != nil {
		return nil, err
	}
	return newValues, nil
}

func deepCopySyncMapCookie(s *sync.Map) (*sync.Map, error) {
	if s == nil {
		return nil, utils.Errorf("empty sync.Map for cookie")
	}

	newMap := new(sync.Map)
	s.Range(func(key, value interface{}) bool {
		c, ok := value.(*http.Cookie)
		if !ok {
			return true
		}

		newMap.Store(key, &http.Cookie{
			Name:       c.Name,       //       "",
			Value:      c.Value,      //      "",
			Path:       c.Path,       //       "",
			Domain:     c.Domain,     //     "",
			Expires:    c.Expires,    //    time.Time{},
			RawExpires: c.RawExpires, // "",
			MaxAge:     c.MaxAge,     //     0,
			Secure:     c.Secure,     //     false,
			HttpOnly:   c.HttpOnly,   //   false,
			SameSite:   c.SameSite,   //   0,
			Raw:        c.Raw,        //        "",
			Unparsed:   c.Unparsed,   //   nil,
		})
		return true
	})
	return newMap, nil
}

func deepCopyMultipartData(m *multipartData) (*multipartData, error) {
	var mdata multipartData
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &mdata)
	if err != nil {
		return nil, err
	}

	if mdata.Files == nil {
		mdata.Files = make(map[string][]*formItem)
	}

	if mdata.Values == nil {
		mdata.Values = make(map[string][]*formItem)
	}
	return &mdata, nil
}
