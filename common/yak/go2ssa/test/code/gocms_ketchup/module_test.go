package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/octavore/naga/service"
	"github.com/octavore/nagax/router"
	"github.com/octavore/nagax/router/middleware"
	"github.com/octavore/nagax/util/errors"
	"github.com/stretchr/testify/assert"

	"github.com/ketchuphq/ketchup/proto/ketchup/models"
	"github.com/ketchuphq/ketchup/util/testutil/memlogger"
)

type testEnv struct {
	module  *Module
	logger  *memlogger.MemoryLogger
	counter *int
	stop    func()
}

var testMiddleware = func() (*int, middleware.Middleware) {
	c := 0
	counter := &c
	return counter, func(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
		(*counter)++
		next(rw, req)
	}
}

func setup() testEnv {
	module := &Module{}
	counter, mw := testMiddleware()
	stop := service.New(module).StartForTest()
	logger := &memlogger.MemoryLogger{}
	module.Logger.Logger = logger
	module.Middleware.Set(mw)
	return testEnv{
		module:  module,
		logger:  logger,
		counter: counter,
		stop:    stop,
	}
}

func TestSubrouter(t *testing.T) {
	te := setup()
	defer te.stop()
	te.module.Handle("GET", "/test-path", func(rw http.ResponseWriter, req *http.Request) {
		rw.Write([]byte("good night"))
	})

	subrouter := te.module.Subrouter("/sub/")
	subrouter.HandlerFunc("GET", "/sub/path", func(rw http.ResponseWriter, req *http.Request) {
		rw.Write([]byte("hello world"))
	})

	rw := httptest.NewRecorder()
	te.module.Middleware.ServeHTTP(rw, httptest.NewRequest("GET", "/not-a-path", nil))
	assert.Equal(t, 404, rw.Code)
	assert.Equal(t, "404 page not found\n", rw.Body.String())

	rw = httptest.NewRecorder()
	te.module.Middleware.ServeHTTP(rw, httptest.NewRequest("GET", "/test-path", nil))
	assert.Equal(t, 200, rw.Code)
	assert.Equal(t, "good night", rw.Body.String())

	rw = httptest.NewRecorder()
	te.module.Middleware.ServeHTTP(rw, httptest.NewRequest("GET", "/sub/path", nil))
	assert.Equal(t, 200, rw.Code)
	assert.Equal(t, "hello world", rw.Body.String())
}

func TestProto(t *testing.T) {
	te := setup()
	defer te.stop()
	rw := httptest.NewRecorder()
	user := &models.User{
		Uuid: proto.String("1234"),
	}
	err := router.ProtoOK(rw, user)
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{"uuid":"1234"}`, rw.Body.String())
	}
}

func TestNotFound(t *testing.T) {
	te := setup()
	defer te.stop()
	rw := httptest.NewRecorder()
	te.module.NotFound(rw)
	assert.Equal(t, 404, rw.Code)
	assert.JSONEq(t, `{
		"code": "NOT_FOUND",
		"detail": "Not found."
	}`, rw.Body.String())
}

func TestInternalError(t *testing.T) {
	te := setup()
	defer te.stop()

	rw := httptest.NewRecorder()
	te.module.InternalError(rw, router.ErrNotFound)
	assert.Equal(t, 404, rw.Code, "expected code 404 but got %v", rw.Code)
	assert.Equal(t, 0, te.logger.Count())

	rw = httptest.NewRecorder()
	te.logger.Reset()
	te.module.InternalError(rw, fmt.Errorf("some generic error"))
	assert.Equal(t, 500, rw.Code, "expected code 500 but got %v", rw.Code)
	assert.Equal(t,
		"router: internal error some generic error",
		te.logger.Errors[0])

	rw = httptest.NewRecorder()
	te.logger.Reset()
	te.module.InternalError(rw, errors.Wrap(fmt.Errorf("some wrapped error")))
	assert.Equal(t, 500, rw.Code, "expected code 500 but got %v", rw.Code)
	assert.Regexp(t,
		`\[github.com/ketchuphq/ketchup/server/router/module_test.go:\d+\] some wrapped error`,
		te.logger.Errors[0])
}
