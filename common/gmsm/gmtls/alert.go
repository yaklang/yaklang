/*
Copyright Suzhou Tongji Fintech Research Institute 2017 All Rights Reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gmtls

import "strconv"

type alert uint8

const (
	// alert level
	alertLevelWarning = 1
	alertLevelError   = 2
)

const (
	alertCloseNotify            alert = 0
	alertUnexpectedMessage      alert = 10
	alertBadRecordMAC           alert = 20
	alertDecryptionFailed       alert = 21
	alertRecordOverflow         alert = 22
	alertDecompressionFailure   alert = 30
	alertHandshakeFailure       alert = 40
	alertBadCertificate         alert = 42
	alertUnsupportedCertificate alert = 43
	alertCertificateRevoked     alert = 44
	alertCertificateExpired     alert = 45
	alertCertificateUnknown     alert = 46
	alertIllegalParameter       alert = 47
	alertUnknownCA              alert = 48
	alertAccessDenied           alert = 49
	alertDecodeError            alert = 50
	alertDecryptError           alert = 51
	alertProtocolVersion        alert = 70
	alertInsufficientSecurity   alert = 71
	alertInternalError          alert = 80
	alertInappropriateFallback  alert = 86
	alertUserCanceled           alert = 90
	alertNoRenegotiation        alert = 100
	alertNoApplicationProtocol  alert = 120
	//GMT0024
	alertUnspporttedSite2Site alert = 200
	alertNoArea               alert = 201
	alertUnspportedAreaType   alert = 202
	alertBadIBCParam          alert = 203
	alertUnspportedIBCParam   alert = 204
	alertIdentityNeed         alert = 205
)

var alertText = map[alert]string{
	alertCloseNotify:            "close notify",
	alertUnexpectedMessage:      "unexpected message",
	alertBadRecordMAC:           "bad record MAC",
	alertDecryptionFailed:       "decryption failed",
	alertRecordOverflow:         "record overflow",
	alertDecompressionFailure:   "decompression failure",
	alertHandshakeFailure:       "handshake failure",
	alertBadCertificate:         "bad certificate",
	alertUnsupportedCertificate: "unsupported certificate",
	alertCertificateRevoked:     "revoked certificate",
	alertCertificateExpired:     "expired certificate",
	alertCertificateUnknown:     "unknown certificate",
	alertIllegalParameter:       "illegal parameter",
	alertUnknownCA:              "unknown certificate authority",
	alertAccessDenied:           "access denied",
	alertDecodeError:            "error decoding message",
	alertDecryptError:           "error decrypting message",
	alertProtocolVersion:        "protocol version not supported",
	alertInsufficientSecurity:   "insufficient security level",
	alertInternalError:          "internal error",
	alertInappropriateFallback:  "inappropriate fallback",
	alertUserCanceled:           "user canceled",
	alertNoRenegotiation:        "no renegotiation",
	alertNoApplicationProtocol:  "no application protocol",
	//GMT0024
	alertUnspporttedSite2Site: "不支持site2site",
	alertNoArea:               "没有保护域",
	alertUnspportedAreaType:   "不支持的保护域类型",
	alertBadIBCParam:          "接收到一个无效的ibc公共参数",
	alertUnspportedIBCParam:   "不支持ibc参数中定义的信息",
	alertIdentityNeed:         "缺少对方的ibc标识",
}

func (e alert) String() string {
	s, ok := alertText[e]
	if ok {
		return "tls: " + s
	}
	return "tls: alert(" + strconv.Itoa(int(e)) + ")"
}

func (e alert) Error() string {
	return e.String()
}
