package testutils

import (
	"crypto/tls"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"sync"
	"time"
)

var (
	tlsTestConfig       *tls.Config
	mtlsTestConfig      *tls.Config
	tlsTestOnce         sync.Once
	gmtlsTestConfig     *gmtls.Config
	onlyGmtlsTestConfig *gmtls.Config
	mgmtlsTestConfig    *gmtls.Config
	clientCrt           []byte
	clientKey           []byte
)

func RegisterDefaultTLSConfigGenerator(h func() (*tls.Config, *gmtls.Config, *gmtls.Config, *tls.Config, *gmtls.Config, []byte, []byte)) {
	go tlsTestOnce.Do(func() {
		tlsTestConfig, gmtlsTestConfig, onlyGmtlsTestConfig, mtlsTestConfig, mgmtlsTestConfig, clientCrt, clientKey = h()
	})
}

func doRegisterDefaultTLSConfigGenerator() {
	RegisterDefaultTLSConfigGenerator(func() (*tls.Config, *gmtls.Config, *gmtls.Config, *tls.Config, *gmtls.Config, []byte, []byte) {
		ca, key, _ := tlsutils.GenerateSelfSignedCertKeyWithCommonName("test", "127.0.0.1", nil, nil)
		sCa, sKey, _ := tlsutils.SignServerCrtNKey(ca, key)
		cCa, cKey, _ := tlsutils.SignClientCrtNKey(sCa, sKey)
		stls, _ := tlsutils.GetX509ServerTlsConfig(ca, sCa, sKey)
		mstls, _ := tlsutils.GetX509MutualAuthServerTlsConfig(ca, sCa, sKey)
		gmtlsConfig, _ := tlsutils.GetX509GMServerTlsConfigWithAuth(ca, sCa, sKey, false)
		onlyGmtlsTestConfig, _ := tlsutils.GetX509GMServerTlsConfigWithOnly(ca, sCa, sKey, false)
		mgmtlsConfig, _ := tlsutils.GetX509GMServerTlsConfigWithAuth(ca, sCa, sKey, true)
		return stls, gmtlsConfig, onlyGmtlsTestConfig, mstls, mgmtlsConfig, cCa, cKey
	})
}

func GetDefaultTLSConfig(i float64) *tls.Config {
	expectedEnd := time.Now().Add(floatSecondDuration(i))
	for {
		if tlsTestConfig != nil {
			log.Infof("fetch default tls config finished: %p", tlsTestConfig)
			return tlsTestConfig
		}
		doRegisterDefaultTLSConfigGenerator()
		time.Sleep(50 * time.Millisecond)
		if !expectedEnd.After(time.Now()) {
			break
		}
	}
	log.Error("fetch default tls config failed")
	return nil
}

func GetDefaultGMTLSConfig(i float64) *gmtls.Config {
	expectedEnd := time.Now().Add(floatSecondDuration(i))
	for {
		if gmtlsTestConfig != nil {
			log.Infof("fetch default gmtls config finished: %p", gmtlsTestConfig)
			return gmtlsTestConfig
		}
		doRegisterDefaultTLSConfigGenerator()
		time.Sleep(50 * time.Millisecond)
		if !expectedEnd.After(time.Now()) {
			break
		}
	}
	log.Error("fetch default tls config failed")
	return nil
}

func GetDefaultOnlyGMTLSConfig(i float64) *gmtls.Config {
	expectedEnd := time.Now().Add(floatSecondDuration(i))
	for {
		if onlyGmtlsTestConfig != nil {
			log.Infof("fetch default gmtls only config finished: %p", onlyGmtlsTestConfig)
			return onlyGmtlsTestConfig
		}
		doRegisterDefaultTLSConfigGenerator()
		time.Sleep(50 * time.Millisecond)
		if !expectedEnd.After(time.Now()) {
			break
		}
	}
	log.Error("fetch default tls config failed")
	return nil
}

func floatSecondDuration(f float64) time.Duration {
	return time.Duration(float64(time.Second) * f)
}
