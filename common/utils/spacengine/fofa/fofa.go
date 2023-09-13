package fofa

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// Fofa a fofa client can be used to make queries
type Fofa struct {
	email string
	key   string
	*http.Client
}

// User struct for fofa user
type User struct {
	Email  string `json:"email,omitempty"`
	Fcoin  int    `json:"fcoin,omitempty"`
	Vip    bool   `json:"bool,omitempty"`
	Avatar string `json:"avatar,omitempty"`
	Err    string `json:"errmsg,omitempty"`
}

const (
	defaultAPIUrl      = "https://fofa.info/api/v1/search/all"
	defaultUserInfoUrl = "https://fofa.info/api/v1/info/my"
)

var (
	errFofaReplyWrongFormat = errors.New("Fofa Reply With Wrong Format")
	errFofaReplyNoData      = errors.New("No Data In Fofa Reply")
)

func NewFofaClient(email, key string) *Fofa {

	transCfg := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	return &Fofa{
		email: email,
		key:   key,
		Client: &http.Client{
			Transport: transCfg, // disable tls verify
		},
	}
}

func (ff *Fofa) Get(urlStr string, val url.Values) ([]byte, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	u.RawQuery = val.Encode()

	body, err := ff.Client.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer body.Body.Close()
	content, err := ioutil.ReadAll(body.Body)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func (ff *Fofa) QueryAsJSON(page, pageSize int, args ...string) ([]byte, error) {
	var (
		query  = ""
		fields = "host,title,ip,domain,port,country,city"
	)
	switch {
	case len(args) == 1:
		query = args[0]
	case len(args) == 2:
		query = args[0]
		fields = args[1]
	}
	val := url.Values{}
	val.Set("email", ff.email)
	val.Set("key", ff.key)
	val.Set("qbase64", codec.EncodeBase64(query))
	val.Set("fields", fields)
	val.Set("page", fmt.Sprint(page))
	val.Set("size", fmt.Sprint(pageSize))
	content, err := ff.Get(defaultAPIUrl, val)
	if err != nil {
		return nil, err
	}
	errmsg := gjson.GetBytes(content, "errmsg").String()
	if errmsg != "" {
		err = errors.New(errmsg)
	}
	return content, nil
}

func (ff *Fofa) UserInfo() (user *User, err error) {
	user = new(User)
	val := url.Values{}
	val.Set("email", ff.email)
	val.Set("key", ff.key)

	content, err := ff.Get(defaultUserInfoUrl, val)

	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(content, user); err != nil {
		return nil, err
	}

	if len(user.Err) != 0 {
		return nil, errors.New(user.Err)
	}

	return user, nil
}

func (u *User) String() string {
	data, err := json.Marshal(u)
	if err != nil {
		log.Fatalf("json marshal failed. err: %s\n", err)
		return ""
	}
	return string(data)
}
