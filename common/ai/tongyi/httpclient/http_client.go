package httpclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

type HTTPOption func(c *HTTPCli)

//go:generate mockgen -destination=http_client_mock.go -package=httpclient . IHttpClient
type IHttpClient interface {
	PostSSE(ctx context.Context, urll string, reqbody interface{}, options ...HTTPOption) (chan string, error)
	Post(ctx context.Context, urll string, reqbody interface{}, resp interface{}, options ...HTTPOption) error
	Get(ctx context.Context, urll string, params map[string]string, resp interface{}, options ...HTTPOption) error
	GetImage(ctx context.Context, imgURL string, options ...HTTPOption) ([]byte, error)
}

type HTTPCli struct {
	client http.Client
	req    *http.Request

	sseStream chan string
}

var _ IHttpClient = (*HTTPCli)(nil)

func NewHTTPClient() *HTTPCli {
	return &HTTPCli{
		client:    http.Client{},
		sseStream: nil,
	}
}

func (c *HTTPCli) Get(ctx context.Context, urll string, params map[string]string, respbody interface{}, options ...HTTPOption) error {
	if params != nil {
		var flag bool
		for k, v := range params {
			if !flag {
				urll += "?"
				flag = true
			}
			urll = fmt.Sprintf("%s%s=%s&", urll, k, url.QueryEscape(v))
		}
		urll = strings.TrimSuffix(urll, "&")
	}

	resp, err := c.httpInner(ctx, "GET", urll, nil, options...)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// fmt.Println("result: ", string(result))

	err = json.Unmarshal(result, &respbody)
	if err != nil {
		return &WrapMessageError{Message: "Unmarshal Json failed", Cause: err}
	}

	return nil
}

func (c *HTTPCli) GetImage(ctx context.Context, imgURL string, options ...HTTPOption) ([]byte, error) {
	resp, err := c.httpInner(ctx, "GET", imgURL, nil, options...)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, &WrapMessageError{Message: "Decode image failed", Cause: err}
	}

	buf := new(bytes.Buffer)
	err = png.Encode(buf, img)
	if err != nil {
		return nil, &WrapMessageError{Message: "Encode image topng failed", Cause: err}
	}

	return buf.Bytes(), nil
}

// nolint:lll
func (c *HTTPCli) PostSSE(ctx context.Context, urll string, reqbody interface{}, options ...HTTPOption) (chan string, error) {
	if reqbody == nil {
		err := &EmptyRequestBodyError{}
		return nil, err
	}

	chanBuffer := 500
	sseStream := make(chan string, chanBuffer)
	c.sseStream = sseStream

	options = append(options, WithStream(), WithHeader(HeaderMap{"content-type": "application/json"}))

	errChan := make(chan error)

	go func() {
		resp, err := c.httpInner(ctx, "POST", urll, reqbody, options...)
		if err != nil {
			errChan <- err
		} else {
			errChan <- nil
		}
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			c.sseStream <- line
		}

		close(c.sseStream)
	}()

	err := <-errChan
	if err != nil {
		return nil, err
	}

	return c.sseStream, nil
}

// nolint:lll
func (c *HTTPCli) Post(ctx context.Context, urll string, reqbody interface{}, respbody interface{}, options ...HTTPOption) error {
	// options = append(options, WithHeader(HeaderMap{"content-type": "application/json"}))

	if reqbody == nil {
		err := &EmptyRequestBodyError{}
		return err
	}

	resp, err := c.httpInner(ctx, "POST", urll, reqbody, options...)
	if err != nil {
		return err
	}
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()

	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	// fmt.Println("result: ", string(result))

	if len(result) != 0 && respbody != nil {
		err = json.Unmarshal(result, &respbody)
		if err != nil {
			return &WrapMessageError{Message: "Unmarshal Json failed", Cause: err}
		}
	}

	return nil
}

func (c *HTTPCli) EncodeJSONBody(body interface{}) (*bytes.Buffer, error) {
	if body != nil {
		var bodyJSON []byte
		switch body := body.(type) {
		case *bytes.Buffer:
			return body, nil
		case []byte:
			bodyJSON = body
		default:
			var err error
			bodyJSON, err = json.Marshal(body)
			if err != nil {
				return nil, err
			}
		}
		return bytes.NewBuffer(bodyJSON), nil
	}
	return bytes.NewBuffer(nil), nil
}

// nolint:lll
func (c *HTTPCli) httpInner(ctx context.Context, method, url string, body interface{}, options ...HTTPOption) (*http.Response, error) {
	var err error

	bodyBuffer, err := c.EncodeJSONBody(body)
	if err != nil {
		return nil, err
	}
	// fmt.Printf("debug... req-body: %+v\n", bodyBuffer.String())

	c.req, err = http.NewRequestWithContext(ctx, method, url, bodyBuffer)
	if err != nil {
		return nil, err
	}

	for _, option := range options {
		option(c)
	}

	resp, err := c.client.Do(c.req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		result, errIo := io.ReadAll(resp.Body)
		if errIo != nil {
			err = errIo
			return resp, err
		}

		err = &HTTPRequestError{Message: "request Failed: " + string(result), Code: resp.StatusCode}
		return resp, err
	}
	// Note: close rsp at outer function
	return resp, nil
}

func NetworkStatus() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "8.8.8.8")
	_, err := cmd.CombinedOutput()
	if err != nil {
		return false, ErrNetwork
	}

	return true, nil
}
