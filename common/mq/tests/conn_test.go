package tests

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mq"
	"github.com/yaklang/yaklang/common/thirdpartyservices"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"strings"
	"testing"
	"time"
)

func Test_Conn(t *testing.T) {
	//获得ampq地址
	u := thirdpartyservices.GetAMQPUrl()

	test := assert.New(t)
	//设置message broker, 第二个参数是config方法覆盖默认值
	broker, err := mq.NewBroker(utils.TimeoutContext(5*time.Second), mq.WithAMQPUrl(u))
	if err != nil {
		test.FailNow(err.Error())
	}
	//创建一个broker server, 设置地址为 test?? 并发200
	server, err := mq.NewListener(broker, "test", 200)
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	err = broker.RunBackground()
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	log.Infof("server is started")
	client, err := mq.NewConnectionWithBroker("test-client", "test", broker)
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	log.Info("client is created")

	go func() {
		client.Write([]byte("hello"))
		time.Sleep(2 * time.Second)
		client.Close()
	}()

	time.Sleep(1 * time.Second)
	conn, err := server.Accept()
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	log.Info("start to recv data from client")
	bytes, _ := ioutil.ReadAll(conn)
	test.True(string(bytes) == "hello")

	client, err = mq.NewConnectionWithBroker("test-client", "test", broker)
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	go func() {
		client.Write([]byte(strings.Repeat("hello", 1026)))
		time.Sleep(500 * time.Millisecond)
	}()
	c, err := server.Accept()
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		c.Close()
	}()
	raw, _ := ioutil.ReadAll(c)
	test.True(len(raw) > 4096)
	s := string(raw)
	data := strings.ReplaceAll(s, "hello", "")
	spew.Dump(data)
	test.True("" == data)
}
