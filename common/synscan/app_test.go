package synscan

import (
	"context"
	"fmt"
	uuid2 "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
	"time"
	"yaklang/common/log"
)

func TestNewScanner(t *testing.T) {
	options, err := CreateConfigOptionsByTargetNetworkOrDomain("8.8.8.8", 5*time.Second)
	if !assert.Nil(t, err) {
		t.FailNow()
	}

	config, err := NewConfig(options...)
	if !assert.Nil(t, err) {
		t.FailNow()
	}

	scanner, err := NewScanner(context.Background(), config)
	if err != nil {
		assert.Nil(t, err)
		t.FailNow()
	}

	_ = scanner
	scanner.RegisterSynAckHandler(uuid2.NewV4().String(), func(ip net.IP, port int) {
		println(fmt.Sprintf("%v:%v", ip.String(), port))
	})

	//err = scanner.RandomScan("47.52.100.105/16", "80", false)
	// 192.168.3.63
	err = scanner.RandomScan(
		//"124.222.42.210,192.168.3.63,47.52.100.105/24",
		"192.168.3.63/24",
		"22,80", false)
	if err != nil {
		log.Error(err)
	}
	time.Sleep(5 * time.Minute)
}

func TestSendTcpPacket(t *testing.T) {
	options, err := CreateConfigOptionsByTargetNetworkOrDomain("8.8.8.8", 5*time.Second)
	if !assert.Nil(t, err) {
		t.FailNow()
	}
	config, err := NewConfig(options...)
	if !assert.Nil(t, err) {
		t.FailNow()
	}
	scanner, err := NewScanner(context.Background(), config)
	if err != nil {
		assert.Nil(t, err)
		t.FailNow()
	}

	_ = scanner
	err = scanner.RandomScan("124.222.42.210", "80", false)
	if err != nil {
		log.Error(err)
	}
	time.Sleep(1 * time.Second)
}

func TestResultHook(t *testing.T) {
	options, err := CreateConfigOptionsByTargetNetworkOrDomain("8.8.8.8", 5*time.Second)
	if !assert.Nil(t, err) {
		t.FailNow()
	}

	config, err := NewConfig(options...)
	if !assert.Nil(t, err) {
		t.FailNow()
	}
	scanner, err := NewScanner(context.Background(), config)
	if err != nil {
		assert.Nil(t, err)
		t.FailNow()
	}
	scanner.RegisterSynAckHandler("TEST", func(ip net.IP, port int) { fmt.Printf("%v open port %v\n", ip.String(), port) })
	_ = scanner
	err = scanner.RandomScan("124.222.42.210", "80", false)
	if err != nil {
		log.Error(err)
	}
	time.Sleep(1 * time.Second)
}
