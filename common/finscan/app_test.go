package finscan

import (
	"context"
	"fmt"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
	"time"
	"yaklang.io/yaklang/common/log"
)

func TestNewScanner(t *testing.T) {
	options, err := CreateConfigOptionsByTargetNetworkOrDomain("124.222.42.210", 5*time.Second)
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

	//err = scanner.RandomScan("47.52.100.105/16", "80", false)
	err = scanner.RandomScan("124.222.42.210", "80", false)
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
	options = append(options, WithTcpSetter(func(tcp *layers.TCP) {
		tcp.FIN = true
	}))
	options = append(options, WithTcpFilter(func(tcp *layers.TCP) bool {
		return false
	}))
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
	log.SetLevel(log.DebugLevel)
	options, err := CreateConfigOptionsByTargetNetworkOrDomain("8.8.8.8", 5*time.Second)
	if !assert.Nil(t, err) {
		t.FailNow()
	}

	config, err := NewConfig(options...)
	if !assert.Nil(t, err) {
		t.FailNow()
	}
	scanner, err := NewScanner(context.Background(), config)
	defer func() {
		scanner.Close()
	}()
	if err != nil {
		assert.Nil(t, err)
		t.FailNow()
	}
	scanner.RegisterRstAckHandler("TEST", func(ip net.IP, port int) { fmt.Printf("%v closed|filtered port %v\n", ip.String(), port) })
	scanner.RegisterNoRspHandler("TEST", func(ip net.IP, port int) { fmt.Printf("%v open port %v\n", ip.String(), port) })
	_ = scanner
	err = scanner.RandomScan("124.222.42.210", "8011", false)
	if err != nil {
		log.Error(err)
	}
	time.Sleep(1 * time.Second)
}
