package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestImport(t *testing.T) {
	t.Run("function", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		import (
			"gorm.io/driver/sqlite"
			"gorm.io/gorm"
			"log"
		)

		func main() {
			db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
			if err != nil {
				log.Fatal("failed to connect to database:", err)
			}
			println(db)
			println(err)
		}

		`, []string{"Undefined-db(valid)", "Undefined-err(valid)"}, t)
	})

	t.Run("struct", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		import (
			"fmt"
			"net/url"
		)

		func main() {
			rawURL := "https://www.example.com:8080/path?query=123#fragment"
			parsedURL, err := url.Parse(rawURL)
			if err != nil {
				fmt.Println("Error parsing URL:", err)
				return
			}

			println(parsedURL.Scheme)
		}

		`, []string{"Undefined-parsedURL.Scheme(valid)"}, t)
	})

	t.Run("struct value", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		import (
			"fmt"
			"net/url"
		)

		func main() {
			u := url.URL{
				Scheme: "https",
				Host:   "www.example.com",
				Path:   "/path",
				RawQuery: "query=123",
			}

			println(u.Scheme)
			println(u.Host)
		}

		`, []string{"\"https\"", "\"www.example.com\""}, t)
	})

	t.Run("struct value-muti", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		import (
			"fmt"
			"net/url"
		)

		func main() {
			u := url.URL{
				Scheme: url.URL{
					Scheme: []string{"https"},
				},
				Host:   "www.example.com",
			}

			println(u.Scheme.Scheme[0])
			println(u.Host)
		}

		`, []string{"\"https\"", "\"www.example.com\""}, t)
	})
}

func TestInterface_ImplementationRelationship(t *testing.T) {
	t.Run("protobuf server interface with multiple implementations", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

type AppServiceServer interface {
	GenApiKey(req *GenApiKeyReq) (*GenApiKeyResp, error)
	GetApiKeyList(req *GetApiKeyListReq) (*ApiKeyInfoList, error)
	DelApiKey(req *DelApiKeyReq) error
}

type UnimplementedAppServiceServer struct{}

func (UnimplementedAppServiceServer) GenApiKey(req *GenApiKeyReq) (*GenApiKeyResp, error) {
	return nil, nil
}

func (UnimplementedAppServiceServer) GetApiKeyList(req *GetApiKeyListReq) (*ApiKeyInfoList, error) {
	return nil, nil
}

func (UnimplementedAppServiceServer) DelApiKey(req *DelApiKeyReq) error {
	return nil
}

type appServiceClient struct {
	cc interface{}
}

func (c *appServiceClient) GenApiKey(req *GenApiKeyReq) (*GenApiKeyResp, error) {
	return nil, nil
}

func (c *appServiceClient) GetApiKeyList(req *GetApiKeyListReq) (*ApiKeyInfoList, error) {
	return nil, nil
}

func (c *appServiceClient) DelApiKey(req *DelApiKeyReq) error {
	return nil
}

type GenApiKeyReq struct{}
type GenApiKeyResp struct{}
type GetApiKeyListReq struct{}
type ApiKeyInfoList struct{}
type DelApiKeyReq struct{}

func main() {
	var s AppServiceServer = UnimplementedAppServiceServer{}
	var c AppServiceServer = &appServiceClient{}
	println(s)
	println(c)
}
		`, []string{"make(struct {type  interface{}})", "make(struct {})"}, t)
	})

	t.Run("interface with single method multiple implementations", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

type Reader interface {
	Read() ([]byte, error)
}

type FileReader struct {
	path string
}

func (f *FileReader) Read() ([]byte, error) {
	return nil, nil
}

type NetworkReader struct {
	url string
}

func (n *NetworkReader) Read() ([]byte, error) {
	return nil, nil
}

type MemoryReader struct {
	data []byte
}

func (m *MemoryReader) Read() ([]byte, error) {
	return m.data, nil
}

func main() {
	var r1 Reader = &FileReader{path: "file.txt"}
	var r2 Reader = &NetworkReader{url: "http://example.com"}
	var r3 Reader = &MemoryReader{data: []byte("test")}
	println(r1)
	println(r2)
	println(r3)
}
		`, []string{"make(struct {bytes})", "make(struct {string})", "make(struct {string})"}, t)
	})

	t.Run("grpc client-server pattern", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

type GreeterServer interface {
	SayHello(req *HelloRequest) (*HelloReply, error)
	SayHelloAgain(req *HelloRequest) (*HelloReply, error)
}

type UnimplementedGreeterServer struct{}

func (UnimplementedGreeterServer) SayHello(req *HelloRequest) (*HelloReply, error) {
	return nil, nil
}

func (UnimplementedGreeterServer) SayHelloAgain(req *HelloRequest) (*HelloReply, error) {
	return nil, nil
}

type greeterClient struct {
	cc interface{}
}

func (c *greeterClient) SayHello(req *HelloRequest) (*HelloReply, error) {
	return nil, nil
}

func (c *greeterClient) SayHelloAgain(req *HelloRequest) (*HelloReply, error) {
	return nil, nil
}

type HelloRequest struct {
	Name string
}

type HelloReply struct {
	Message string
}

func main() {
	var server GreeterServer = UnimplementedGreeterServer{}
	var client GreeterServer = &greeterClient{}
	println(server)
	println(client)
}
		`, []string{"make(struct {type  interface{}})", "make(struct {})"}, t)
	})

	t.Run("interface hierarchy without circular dependency", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

type BaseInterface interface {
	BaseMethod() string
}

type ExtendedInterface interface {
	BaseInterface
	ExtendedMethod() int
}

type Implementation struct{}

func (Implementation) BaseMethod() string {
	return "base"
}

func (Implementation) ExtendedMethod() int {
	return 42
}

func main() {
	var base BaseInterface = Implementation{}
	var extended ExtendedInterface = Implementation{}
	println(base)
	println(extended)
}
		`, []string{"make(struct {})", "make(struct {})"}, t)
	})

	t.Run("multiple interfaces same implementation", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

type Reader interface {
	Read() error
}

type Writer interface {
	Write() error
}

type ReadWriter interface {
	Read() error
	Write() error
}

type FileHandler struct{}

func (FileHandler) Read() error {
	return nil
}

func (FileHandler) Write() error {
	return nil
}

func main() {
	var r Reader = FileHandler{}
	var w Writer = FileHandler{}
	var rw ReadWriter = FileHandler{}
	println(r)
	println(w)
	println(rw)
}
		`, []string{"make(struct {})", "make(struct {})", "make(struct {})"}, t)
	})
}
