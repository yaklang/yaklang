package yakgrpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 测试用证书和密钥
var (
	// 标准TLS证书 - CA证书
	standardCA = []byte(`-----BEGIN CERTIFICATE-----
MIIC7DCCAdSgAwIBAgIQbagy7yK5TDZwLFfQfVL4TzANBgkqhkiG9w0BAQsFADAP
MQ0wCwYDVQQDEwR0ZXN0MCAXDTk5MTIzMTE2MDAwMFoYDzIxMjExMDI5MDI1NTE4
WjAPMQ0wCwYDVQQDEwR0ZXN0MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
AQEApx0SglizLAq9APKDRw1om4Pl6jLpQcKV8pF1HYAJPxv+NOk9aQA5AdBwxwph
PD0puWn0IN/AezbBMDGXBgNOXEopVhrUsz2e6GVI6F/VjCOGchB63peumuFvl3nP
oNmTXDBaHlvnSCSVeyAnc8lp7AmR3zzRKDm4PJQdNcRtqa7BoGVktwo7oCYMsHdj
zAxqunzwVme8F9MHmRghSULzNFj9SDi2HTjlEuT/4iAbvXq4L9EGoKa2t5CzYtd3
3oJ13nrkoI3eExeNd8bx6xb6WgR/gVg4199phaMiLNiPiSIk3HUOj3IbUM4RwJIn
BG1+yxiOXSXdH02GBYPhauVsJQIDAQABo0IwQDAOBgNVHQ8BAf8EBAMCAaYwDwYD
VR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUPAyvxgjT9KrD+BzgKtOyzER6QtswDQYJ
KoZIhvcNAQELBQADggEBAITL0jFIQVHkc9k2SSKTgfch8NXk5rhi0QC1PybdNOTm
hNy36kMJLVtRBKPkBUGINzE13WDZ5tG3LMdfMZOW+aIuU7b/f0vdAn8P5yEHcX/r
3HSk+q9oylu1fDhSWYESdYDATe1LAuiOJ8l8K/117HrabjhpvTVYRg/b9gZ74dkJ
uBSceWyRKMKcfPRwoAyfVKPmkjhfR7Nl9JUz8tAnmHpE7l85gNnDn+rrKLmBenbs
yksXOO1/jVonZzplKPB93YVsSA6oIYmhDS0cvq3ufC13y9hPLjSDW00eBToa846d
AT0OXcdOarFX6SLMbCHtNiedgc5NlPR6W2M/Gcdvvhc=
-----END CERTIFICATE-----`)

	// 标准TLS证书 - 服务器证书
	standardServerCert = []byte(`-----BEGIN CERTIFICATE-----
MIIDAzCCAeugAwIBAgIQDA48LpEXWqsV2YhaPIVB6TANBgkqhkiG9w0BAQsFADAP
MQ0wCwYDVQQDEwR0ZXN0MB4XDTk5MTIzMTE2MDAwMFoXDTMyMTExOTAyNTUxOFow
FDESMBAGA1UEAxMJMTI3LjAuMC4xMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIB
CgKCAQEAvk6eYy4QqxhqDY54STpr7ga1f/lUtWGDi5hYQEx9UIYeTqMLQgIJUpgM
J0OlNd7zcwSMOtUHM2xl8HWyR7FVeIo+W+jeQo3wwT2zGKVq0BV06xhVgrF3zzb9
7X6+g/cgKCIuF5bNXQqR+iotDtAwfPQIDRQdmrahDfQZM7hWEDT9cUq7lBQ279ft
9iTrY8qj2pXA+zikZYcJRwGgL2S2l7K/d7xIJfon0ly69FIFw7uoF9Q6P0ABWPfF
m0kxSpyzgnOSat+w1ULyWRkZ8gJvMt9Vjb3S+/gASSsmPadNJOPuHeY5fNbR/A2a
IKtQJ9aI6FEnzz2YEGh6X/SBdvYbQQIDAQABo1YwVDAOBgNVHQ8BAf8EBAMCAqQw
EwYDVR0lBAwwCgYIKwYBBQUHAwEwDAYDVR0TAQH/BAIwADAfBgNVHSMEGDAWgBQ8
DK/GCNP0qsP4HOAq07LMRHpC2zANBgkqhkiG9w0BAQsFAAOCAQEAc0urtZg1Mdrc
wlS4jUEZNHg7tVUYlXH8gsjD7C6N8f3irTO24iKjIoCOCEU2jmPyjBXKiwmWJDps
NGuNZYN3iqeiHhW+w14u7NbHzsaN8iVuor+Mq0WGpEOY8un1APuiPpSV/AEcsElf
SRpC8o4z1fdUtlFCAZWvHVc6cMOoQhsGUG3pMewaPxvHPjQLHoIaYx9C8YUjfmqd
xPCYjshjweXRfjtFs/vUVG00m6sAizdPjM+yQPPK9OE+7xdvbGoKCtMWtLCDQ1Jd
ntD4M5dv/xkulQSjGtHsV613/mVGSUH/4GFh52oOFKQNKKhX2kmK/9VLTCrFi13j
6UgvH7nXbw==
-----END CERTIFICATE-----`)

	// 标准TLS证书 - 服务器私钥
	standardServerKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAvk6eYy4QqxhqDY54STpr7ga1f/lUtWGDi5hYQEx9UIYeTqML
QgIJUpgMJ0OlNd7zcwSMOtUHM2xl8HWyR7FVeIo+W+jeQo3wwT2zGKVq0BV06xhV
grF3zzb97X6+g/cgKCIuF5bNXQqR+iotDtAwfPQIDRQdmrahDfQZM7hWEDT9cUq7
lBQ279ft9iTrY8qj2pXA+zikZYcJRwGgL2S2l7K/d7xIJfon0ly69FIFw7uoF9Q6
P0ABWPfFm0kxSpyzgnOSat+w1ULyWRkZ8gJvMt9Vjb3S+/gASSsmPadNJOPuHeY5
fNbR/A2aIKtQJ9aI6FEnzz2YEGh6X/SBdvYbQQIDAQABAoIBACd0/XnqzyHqSfLN
mzrzlfUgBvmlpF6G/VMwHvwV39WWOSpsu6TP70bkp4BskhB9TVSHmNuJ15hd3TTh
8jjTF7mKUCuWOJ7r9wLZ3Aw8H81M5ZTo0rHqQcEA0d0v7ihGULCBhbT2W1XzHxkT
LYxoteTyY8jyZsDxJKtT9PW4Pn/VYSh+9ebX7a5pD4mdHtBwS2kenRfRGHZYmMKD
x9g+74F1AamvrYNXVQT77IfDdSGuah9T1c5Fm1nox9liol0LAT4gzedBNDQ2FWGp
f87kJFFCW/GxGzPIPa64CG2QvIzKUuOwA/3j1+8R6d4tFUUy0PDNRaTZq5oYoxZh
l5cAk8ECgYEA5seAyyOzeJlx3q7Bs57xqxpjWux6sP/6vmOlYOOCCdIkgoCVFvBk
0IsMMQwvu2e9yRYC4FUIchFeJShuBCXuH8ELuskd/Lu7dRSjrbRFFAiWESPZZyRb
CBwKiUs9ggvaOEyw7ueWwuYwPemYZEhV5W8iORb9Rbv2iO6ZolJIp4UCgYEA0xrT
13XrjEEFG/bj2bCEWDS92XbZYHQ5L8ZpZBJohR/nQW+0bt9uJYjcTdmqOWG6Rr7v
n/Xos7Afbx88BPtvFJnNgNC5H2L2dysCBJzrywyeejhcTZsb7KprdlUJ0cawo9fv
lryXcWBJQkoVpREDRgiNihLFs4yo29Ux6eicq40CgYEAsYlO4nevjJp3CDlmmHkx
L1EYmA0OgfYa/raHtlavZkC8h4zFpSUAWZJuqZjXa5NuZDDDu7KO0bnctDc7E4Pe
gZ0wGdy4bgI6PuLG3E2vSq8kS0FJ8Vf9k+qGjIJOaioWEXOmNdQBniQZfrei3Zrs
QZnSORsfcrMcANGVbVNhw0UCgYEAno4MxExuERaYxsslkVAx5qoeWaIZXIeOmCJ2
79GfrTUsFQrYQ1oPOaPUi6hLYPPU2+P2yHcDQ0qqIWUdSESsxpVKM1ERadCDezfT
OTG/K++bbAK+2Q8B5zyMoAD48hVAgJ7j9ZxKRr5h56cLIMJpagVsgWLeGKAyB4LW
DXBHk9UCgYBG69TuAA2U5mLjVfG7Dcs9b3uWbEPI4FcFSdBTomX4NN8QTSg2AFT+
Slt5quVQKA72QtYbGFXqa4gkQIpi+XfZVi99xxO9AiK3mT2pbKGFvsZ3CjNKQXwv
Cj0+wngpiUBQ7zFwEIJZbxFDhgHUzwx2v1Z/H44DMwNu7SsnkjtfMQ==
-----END RSA PRIVATE KEY-----`)

	// 标准TLS证书 - 客户端证书
	standardClientCert = []byte(`-----BEGIN CERTIFICATE-----
MIIDAjCCAeqgAwIBAgIQH8a4t+gGuGdADJrnO97PDTANBgkqhkiG9w0BAQsFADAP
MQ0wCwYDVQQDEwR0ZXN0MCAXDTk5MTIzMTE2MDAwMFoYDzIxMjExMDI5MDI1NTE4
WjARMQ8wDQYDVQQDEwZDTElFTlQwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEK
AoIBAQDLk0zKykya3XZKett6ZJDVfUqzPt2tNwsjiWtGP79Bum6ZmZHfSFIOR7+K
1m1mWa0WDvtH6Lt0mUu0s2p5JSFG+zfPnkgcC+m1avQGFYZPclO1DUt5wS7tagaZ
/ZeSUHSEI2uybR1bj5psiSkg6ZMAW9TAiueSSCQZZJ4dR6YInR50/3AE4Ypq5L9n
kYMBj/LkMTSEAzy84eOD61XNs6F7gBfH1Qk5XzZUoxkK6T9bi6qqNVpcSZf0HiE2
dcWTmRWVgTR9v2+ZzkG+acUGb2PIIFIkbIPp8aQg/0vIhV2KepZbO16AanOstnHc
Tk1t1dSmXLbkJz0cBCP7PAHNIYFHAgMBAAGjVjBUMA4GA1UdDwEB/wQEAwIBpjAT
BgNVHSUEDDAKBggrBgEFBQcDAjAMBgNVHRMBAf8EAjAAMB8GA1UdIwQYMBaAFDwM
r8YI0/Sqw/gc4CrTssxEekLbMA0GCSqGSIb3DQEBCwUAA4IBAQCGc5yEGDcRVC3k
JLROnjj+gexkwTRTt+9CV91a8rsPgqij1uepNXmS5y3g8SRQeOsL/zWXV5GnVy6O
4UvhFEiytuoy7bpMq3YvDGVI0zdq/CM6abGPDOSFNxbv46b+Q2o/qk2A/7qnUyPQ
/el90WSpNgb7DkhW8p3Inxkd6ABveGuoOk+495JzBq/hZZRmpL/7eI4poMINjZVj
bkVGoIJFtyycASdN7/yIr3owioShFRCE4D17Z+7qZr42gI+o/woS3H6+6ppLoYff
0fQ7Tf6JUoMRL2zwEe0f/jn+ZBWNdmSIhKDbSMlcwWsWoer2iiJGUmB4rmN7Dr/8
ExuooyMA
-----END CERTIFICATE-----`)

	// 标准TLS证书 - 客户端私钥
	standardClientKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAy5NMyspMmt12SnrbemSQ1X1Ksz7drTcLI4lrRj+/QbpumZmR
30hSDke/itZtZlmtFg77R+i7dJlLtLNqeSUhRvs3z55IHAvptWr0BhWGT3JTtQ1L
ecEu7WoGmf2XklB0hCNrsm0dW4+abIkpIOmTAFvUwIrnkkgkGWSeHUemCJ0edP9w
BOGKauS/Z5GDAY/y5DE0hAM8vOHjg+tVzbOhe4AXx9UJOV82VKMZCuk/W4uqqjVa
XEmX9B4hNnXFk5kVlYE0fb9vmc5BvmnFBm9jyCBSJGyD6fGkIP9LyIVdinqWWzte
gGpzrLZx3E5NbdXUply25Cc9HAQj+zwBzSGBRwIDAQABAoIBAGxGbxyY1n+z9JuO
lreVT3dNSXLmp+7eDN2c1GKruyTRbMvjYzOX+pS/0n+cptk+LxJBa6MGhNVyR1LX
7nR6rCVdroSN0hqgt3AXb6zgu+v7icwNQyyB9Fyv/MzglUJr6lzxnfFrmaa+TUsW
9LodoWMadKDoAFzMY+7hljtKhWOkgg/JhL8MyAPmyddQqgI8yLTpm5k3iv4XUBVW
Y7jx4DGMVvmUJS97sH+aMXpOqh556KXnGbYLLh4zAAtV1YUVol/WpFK9bR6hCwSY
BkHkiyZiFezncHx2/WVZHo7gBYIVqvbdHmLaGCfoebnAPKwWLi1lEzEnvDnnU0Ck
a1e+LBECgYEA0oNoJNJwbYpwX54qoEEoXOkf6feCIjwvTltvOC+gs7demSfLuijo
rkPJ5GGHctrFNixZe7qjOm4U2FXddXsaMBa7MamWc4NvCatZngW/PSnv/qNY3rSk
QOG7YvrsCzwvUSRGVF04I+BiTsTMHPTbA1ysrovrcE9Kh6rXau8BXH8CgYEA95Ae
4VFKlYu7diJ4Qo0e90XWZMErUDT2BEPiBk/Dw31BJuuST54sC7tt4+wTEn473+sB
FgVi2vu8KvYooRizQCRwRcirCdml7NW5JyhTom21FjGyazJD/s+7JunxkVVyMdvK
uE6+obdkjeQP4xN30ME8dWBXnCsf4PQ7WAk7lzkCgYEAmUoKydVa/Mj0LwxTacJI
i+9Nx+btIdTFdb9q63TzBiqefdPWq8YiONMv7ld+dAoN1PbSaiBrv55tG2LbEjMD
zMSgpvcgkRjCAD5/0WvJ59Xj5n43tmO/v2cgNmEVBNFcey947vG6cZVwwH7ZSrSZ
zobrT2afmHaEhOnIVxuW2C0CgYEA7UXvousD/jMP+BjvhHG3dS41XxoZhmVMSig5
0OzQZ2R8dm4gLDkgZBo/J82TNg1RG7skrlN5PQM7hT2rEUQYQWjrRqce73Dwa/8n
15T6G9rkTiJRrBZgPzAgYxqkEjSAH7NWJ7IpWdvo/2nPpEd7ddRPOvyc26wlgLj0
y9sFh1kCgYBwGgM4xpfiloycJA3OghI/m3RsO/PYTM84aMFPFeHIsr9fAoxy+RTp
BrKSqjLNV8P8dDI//k6gqyTUjV+fLxc1UqdxRNLyBScPVCfQED7pC+/9LGfO3XbS
rw7+Y9oqjInyd9bbtnY+6o7LMnbG+I+IDp7M9ELpwHAz6g1uf7D/nA==
-----END RSA PRIVATE KEY-----`)

	// 国密证书 - CA证书
	gmCA = []byte(`-----BEGIN CERTIFICATE-----
MIIB9jCCAZygAwIBAgIQWnrJ5ywzJj++KbgmeCpk+DAKBggqgRzPVQGDdTBaMQ0w
CwYDVQQGEwR0ZXN0MQ0wCwYDVQQIEwR0ZXN0MQ0wCwYDVQQHEwR0ZXN0MQ0wCwYD
VQQKEwR0ZXN0MQ0wCwYDVQQLEwR0ZXN0MQ0wCwYDVQQDEwR0ZXN0MCAXDTk5MTIz
MTE2MDAwMFoYDzIxMjQwNzI5MDM1NDUwWjBaMQ0wCwYDVQQGEwR0ZXN0MQ0wCwYD
VQQIEwR0ZXN0MQ0wCwYDVQQHEwR0ZXN0MQ0wCwYDVQQKEwR0ZXN0MQ0wCwYDVQQL
EwR0ZXN0MQ0wCwYDVQQDEwR0ZXN0MFkwEwYHKoZIzj0CAQYIKoEcz1UBgi0DQgAE
gtf3+bmT2BaQ1x9LHw3IPmwhLPB3T5WYCeMihZKnmK+zr7baovNJoqCcj5UZw7jx
arp9UbA+016ZMR9Gjuk3UaNCMEAwDgYDVR0PAQH/BAQDAgGmMB0GA1UdJQQWMBQG
CCsGAQUFBwMCBggrBgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MAoGCCqBHM9VAYN1
A0gAMEUCIA/K6vI/qdkyNupJ1CdQIL7ZS7qjsulUZ0OvIJirOtJHAiEAvBR5mDl7
O9O4kuFkCgzqRYgBYT4HNDJ93rASOSiF6M8=
-----END CERTIFICATE-----`)

	// 国密证书 - 服务器证书
	gmServerCert = []byte(`-----BEGIN CERTIFICATE-----
MIIBojCCAUigAwIBAgIRAJlgn/+0aF5oCYzKzqOyTw0wCgYIKoEcz1UBg3UwWjEN
MAsGA1UEBhMEdGVzdDENMAsGA1UECBMEdGVzdDENMAsGA1UEBxMEdGVzdDENMAsG
A1UEChMEdGVzdDENMAsGA1UECxMEdGVzdDENMAsGA1UEAxMEdGVzdDAeFw05OTEy
MzExNjAwMDBaFw0zNTA4MjAwMzU0NTBaMBQxEjAQBgNVBAMTCTEyNy4wLjAuMTBZ
MBMGByqGSM49AgEGCCqBHM9VAYItA0IABIu+kZDB5D4OkMIUWdcY5L3mTctW8QeO
NB8BCLdIF/VyJvBpADMUq5VN4p6l4jx3/qJt+8L1yT/A4bBcDRDledijNTAzMA4G
A1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggrBgEFBQcDATAMBgNVHRMBAf8EAjAA
MAoGCCqBHM9VAYN1A0gAMEUCIQDFh3M3QLfjwrLllylRASjRHv2AKNMREtny/2rN
9Lhr6AIgDbyxQi/pGSCISLUyCytSJcEq2t2GBtW6Qhr21ePStfQ=
-----END CERTIFICATE-----`)

	// 国密证书 - 服务器私钥
	gmServerKey = []byte(`-----BEGIN PRIVATE KEY-----
MIGTAgEAMBMGByqGSM49AgEGCCqBHM9VAYItBHkwdwIBAQQgS4jjdCZaKRrSl3EV
UzMRZE1zEAlUpHmNlk/6L9y3Mw+gCgYIKoEcz1UBgi2hRANCAASLvpGQweQ+DpDC
FFnXGOS95k3LVvEHjjQfAQi3SBf1cibwaQAzFKuVTeKepeI8d/6ibfvC9ck/wOGw
XA0Q5XnY
-----END PRIVATE KEY-----`)

	// 国密证书 - 客户端证书
	gmClientCert = []byte(`-----BEGIN CERTIFICATE-----
MIIBoTCCAUegAwIBAgIQWHoM6EOQ0bLyJya2ppwvSDAKBggqgRzPVQGDdTBaMQ0w
CwYDVQQGEwR0ZXN0MQ0wCwYDVQQIEwR0ZXN0MQ0wCwYDVQQHEwR0ZXN0MQ0wCwYD
VQQKEwR0ZXN0MQ0wCwYDVQQLEwR0ZXN0MQ0wCwYDVQQDEwR0ZXN0MB4XDTk5MTIz
MTE2MDAwMFoXDTM1MDgyMDAzNTQ1MFowFDESMBAGA1UEAwwJR01fQ0xJRU5UMFkw
EwYHKoZIzj0CAQYIKoEcz1UBgi0DQgAE5Ctz3BSodQtDoKt5OBIicU9sOeo4Ut+l
1D4QmARmcKzp9ku16MrmGsKSs+9SQm6BBTr6kcyRK1EVnCqpUbFuyqM1MDMwDgYD
VR0PAQH/BAQDAgKkMBMGA1UdJQQMMAoGCCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAw
CgYIKoEcz1UBg3UDSAAwRQIgS8LlOFDpVxSMrCVpASTAoxx81C+W0FTsHhdlgwr+
qGoCIQD78fRygR++WlvEQjLTlnRDX3XHs+DsvMPQ51cxe/6Ssw==
-----END CERTIFICATE-----`)

	// 国密证书 - 客户端私钥
	gmClientKey = []byte(`-----BEGIN PRIVATE KEY-----
MIGTAgEAMBMGByqGSM49AgEGCCqBHM9VAYItBHkwdwIBAQQg8fCQoDha5V6rzTVg
g8i13+nUyBDUeoS0QP1qHhJ0BaKgCgYIKoEcz1UBgi2hRANCAATkK3PcFKh1C0Og
q3k4EiJxT2w56jhS36XUPhCYBGZwrOn2S7XoyuYawpKz71JCboEFOvqRzJErURWc
KqlRsW7K
-----END PRIVATE KEY-----`)
)

// 测试场景结构
type TestScenario struct {
	Name        string
	EnableGM    bool // 是否启用国密
	EnableH2    bool // 是否启用HTTP/2
	SpecifyHost bool // 是否指定Host
}

// 启动测试用的TLS服务器
func startTestTLSServer(t *testing.T, host string, port int, isGM bool, requireClientCert bool) (string, int) {
	var config interface{}
	var err error

	if isGM {
		// 国密TLS服务器配置
		if requireClientCert {
			// 双向认证
			config, err = tlsutils.GetX509GMServerTlsConfigWithAuth(gmCA, gmServerCert, gmServerKey, true)
			if err != nil {
				t.Fatalf("创建国密双向认证TLS服务器配置失败: %v", err)
			}
		} else {
			// 单向认证
			config, err = tlsutils.GetX509GMServerTlsConfigWithAuth(gmCA, gmServerCert, gmServerKey, false)
			if err != nil {
				t.Fatalf("创建国密TLS服务器配置失败: %v", err)
			}
		}
	} else {
		// 标准TLS服务器配置
		if requireClientCert {
			// 双向认证
			config, err = tlsutils.GetX509MutualAuthServerTlsConfig(standardCA, standardServerCert, standardServerKey)
			if err != nil {
				t.Fatalf("创建标准双向认证TLS服务器配置失败: %v", err)
			}
		} else {
			// 单向认证
			config, err = tlsutils.GetX509ServerTlsConfig(standardCA, standardServerCert, standardServerKey)
			if err != nil {
				t.Fatalf("创建标准TLS服务器配置失败: %v", err)
			}
		}
	}

	// 创建HTTP服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		testToken := r.Header.Get("X-Test-Token")
		response := fmt.Sprintf("OK - Token: %s - TLS: %s", testToken,
			func() string {
				if isGM {
					return "GM"
				}
				return "Standard"
			}())
		w.WriteHeader(200)
		w.Write([]byte(response))
	})

	server := &http.Server{
		Addr:    utils.HostPort(host, port),
		Handler: mux,
	}

	// 启动服务器
	listener, err := net.Listen("tcp", utils.HostPort(host, port))
	if err != nil {
		t.Fatalf("创建监听器失败: %v", err)
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port

	log.Infof("启动%s TLS服务器在端口 %d (mTLS: %v)",
		func() string {
			if isGM {
				return "国密"
			}
			return "标准"
		}(), actualPort, requireClientCert)

	// 使用channel信号通知服务器启动完成
	serverReady := make(chan struct{})
	serverError := make(chan error, 1)

	go func() {
		// 立即发送启动信号
		close(serverReady)

		if isGM {
			// 国密TLS服务器需要特殊处理
			gmListener := gmtls.NewListener(listener, config.(*gmtls.Config))
			err := server.Serve(gmListener)
			if err != nil && err != http.ErrServerClosed {
				log.Errorf("国密TLS服务器启动失败: %v", err)
				select {
				case serverError <- err:
				default:
				}
			}
		} else {
			// 标准TLS服务器
			tlsListener := tls.NewListener(listener, config.(*tls.Config))
			err := server.Serve(tlsListener)
			if err != nil && err != http.ErrServerClosed {
				log.Errorf("标准TLS服务器启动失败: %v", err)
				select {
				case serverError <- err:
				default:
				}
			}
		}
	}()

	// 等待服务器启动信号
	<-serverReady

	// 使用utils.WaitConnect确保服务器真正可用
	serverAddr := utils.HostPort(host, actualPort)
	err = utils.WaitConnect(serverAddr, 3) // 等待3秒
	if err != nil {
		// 检查是否有启动错误
		select {
		case startErr := <-serverError:
			t.Fatalf("TLS服务器启动失败: %v", startErr)
		default:
			t.Fatalf("TLS服务器连接超时: %v", err)
		}
	}

	log.Infof("TLS服务器启动成功，地址: %s", serverAddr)

	return host, actualPort
}

// 执行单个测试场景
func runMTLSTestScenario(t *testing.T, scenario TestScenario, client ypb.YakClient) {
	log.Infof("=== 开始测试场景: %s ===", scenario.Name)

	// 启动测试服务器
	targetHost, targetPort := startTestTLSServer(t, "127.0.0.1", 0, scenario.EnableGM, true)
	targetURL := fmt.Sprintf("https://%s", utils.HostPort(targetHost, targetPort))
	testToken := uuid.New().String()

	// 配置客户端证书到netx全局状态（正确的方式）
	if scenario.EnableGM {
		// 加载国密客户端证书到netx
		p12Bytes, err := tlsutils.BuildP12(gmClientCert, gmClientKey, "", gmCA)
		if err != nil {
			t.Fatalf("构建国密P12证书失败: %v", err)
		}
		hostForCert := ""
		if scenario.SpecifyHost {
			hostForCert = targetHost
		}
		err = netx.LoadP12Bytes(p12Bytes, "", hostForCert)
		if err != nil {
			t.Fatalf("加载国密P12证书失败: %v", err)
		}
		log.Infof("已加载国密客户端证书，Host模式: %s", hostForCert)
	} else {
		// 加载标准客户端证书到netx
		p12Bytes, err := tlsutils.BuildP12(standardClientCert, standardClientKey, "", standardCA)
		if err != nil {
			t.Fatalf("构建标准P12证书失败: %v", err)
		}
		hostForCert := ""
		if scenario.SpecifyHost {
			hostForCert = targetHost
		}
		err = netx.LoadP12Bytes(p12Bytes, "", hostForCert)
		if err != nil {
			t.Fatalf("加载标准P12证书失败: %v", err)
		}
		log.Infof("已加载标准客户端证书，Host模式: %s", hostForCert)
	}
	defer netx.ResetPresetCertificates()

	// 获取可用端口
	mitmPort := utils.GetRandomAvailableTCPPort()
	ctx, cancel := context.WithCancel(context.Background())

	// 使用RunMITMV2TestServerEx启动MITM服务器，避免time.Sleep
	RunMITMV2TestServerEx(client, ctx,
		// onInit: 发送初始配置
		func(mitmClient ypb.Yak_MITMV2Client) {
			mitmRequest := &ypb.MITMV2Request{
				Host:             "127.0.0.1",
				Port:             uint32(mitmPort),
				SetAutoForward:   true,
				AutoForwardValue: true,
				EnableHttp2:      scenario.EnableH2,
				EnableGMTLS:      scenario.EnableGM,
				// 注意：不使用Certificates字段，客户端证书由netx.LoadP12Bytes()全局状态控制
			}
			err := mitmClient.Send(mitmRequest)
			require.NoError(t, err, "发送MITMv2请求失败")
		},
		// onLoad: MITM服务器启动后执行测试
		func(mitmClient ypb.Yak_MITMV2Client) {
			defer cancel()
			proxyURL := fmt.Sprintf("http://127.0.0.1:%d", mitmPort)
			log.Infof("MITM代理服务器已启动: %s", proxyURL)

			// 创建HTTP客户端，通过MITM代理发送请求
			httpClient := utils.NewDefaultHTTPClient()
			transport := httpClient.Transport.(*http.Transport)

			// 设置代理
			proxyUrl, err := url.Parse(proxyURL)
			require.NoError(t, err, "解析代理URL失败")
			transport.Proxy = http.ProxyURL(proxyUrl)

			// 设置TLS配置
			transport.TLSClientConfig = &tls.Config{
				InsecureSkipVerify: true,
			}

			// 创建请求
			req, err := http.NewRequest("GET", targetURL, nil)
			require.NoError(t, err, "创建HTTP请求失败")

			// 添加测试令牌
			req.Header.Set("X-Test-Token", testToken)
			req.Header.Set("Connection", "close")

			// 发送请求
			log.Infof("发送mTLS测试请求到: %s", targetURL)
			resp, err := httpClient.Do(req)
			testSuccess := false
			if err != nil {
				log.Errorf("mTLS请求失败: %v", err)
				testSuccess = false
			} else {
				defer resp.Body.Close()

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					log.Errorf("读取响应失败: %v", err)
					testSuccess = false
				} else {
					bodyStr := string(body)
					log.Infof("mTLS请求成功，状态码: %d, 响应: %s", resp.StatusCode, bodyStr)

					// 验证响应包含测试令牌
					if strings.Contains(bodyStr, testToken) && resp.StatusCode == 200 {
						log.Infof("mTLS测试成功")
						testSuccess = true
					} else {
						log.Errorf("mTLS测试失败: 响应不包含测试令牌或状态码错误")
						testSuccess = false
					}
				}
			}

			require.True(t, testSuccess, fmt.Sprintf("场景 %s 测试失败", scenario.Name))
			log.Infof("=== 场景 %s 测试完成 ===", scenario.Name)
		},
		// onRecv: 处理接收到的消息（可选）
		nil,
	)
}

// 测试1: MITMv2 mTLS 综合测试 - 覆盖所有场景组合
func TestGRPCMUSTPASS_MITMV2_MTLS_Comprehensive(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	// 定义所有测试场景
	scenarios := []TestScenario{
		{Name: "标准TLS + HTTP1.1 + 指定Host", EnableGM: false, EnableH2: false, SpecifyHost: true},
		{Name: "标准TLS + HTTP1.1 + 不指定Host", EnableGM: false, EnableH2: false, SpecifyHost: false},
		{Name: "标准TLS + HTTP2 + 指定Host", EnableGM: false, EnableH2: true, SpecifyHost: true},
		{Name: "标准TLS + HTTP2 + 不指定Host", EnableGM: false, EnableH2: true, SpecifyHost: false},
		{Name: "国密TLS + HTTP1.1 + 指定Host", EnableGM: true, EnableH2: false, SpecifyHost: true},
		{Name: "国密TLS + HTTP1.1 + 不指定Host", EnableGM: true, EnableH2: false, SpecifyHost: false},
		{Name: "国密TLS + HTTP2 + 指定Host", EnableGM: true, EnableH2: true, SpecifyHost: true},
		{Name: "国密TLS + HTTP2 + 不指定Host", EnableGM: true, EnableH2: true, SpecifyHost: false},
	}

	// 逐个执行测试场景
	for _, scenario := range scenarios {
		netx.ResetPresetCertificates()
		t.Run(scenario.Name, func(t *testing.T) {
			runMTLSTestScenario(t, scenario, client)
		})
	}
}

// 测试2: MITMv2 mTLS 无客户端证书测试 - 普通TLS服务器场景 - 应该失败
func TestGRPCMUSTPASS_MITMV2_MTLS_WithoutClientCert(t *testing.T) {
	netx.ResetPresetCertificates()
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	// 启动需要客户端证书的服务器
	targetHost, targetPort := startTestTLSServer(t, "127.0.0.1", 0, false, true)
	targetURL := fmt.Sprintf("https://%s", utils.HostPort(targetHost, targetPort))

	mitmPort := utils.GetRandomAvailableTCPPort()

	ctx, cancel := context.WithCancel(context.Background())

	// 使用RunMITMV2TestServerEx启动MITM服务器
	RunMITMV2TestServerEx(client, ctx,
		// onInit: 发送初始配置
		func(mitmClient ypb.Yak_MITMV2Client) {
			mitmRequest := &ypb.MITMV2Request{
				Host:             "127.0.0.1",
				Port:             uint32(mitmPort),
				SetAutoForward:   true,
				AutoForwardValue: true,
				EnableHttp2:      false,
				EnableGMTLS:      false,
				// 注意：不配置客户端证书，也不调用netx.LoadP12Bytes()
			}
			err := mitmClient.Send(mitmRequest)
			require.NoError(t, err, "发送MITMv2请求失败")
		},
		// onLoad: MITM服务器启动后执行测试
		func(mitmClient ypb.Yak_MITMV2Client) {
			defer cancel()
			proxyURL := fmt.Sprintf("http://127.0.0.1:%d", mitmPort)

			// 创建HTTP客户端，通过MITM代理发送请求（不配置客户端证书）
			httpClient := utils.NewDefaultHTTPClient()
			transport := httpClient.Transport.(*http.Transport)

			// 设置代理
			proxyUrl, err := url.Parse(proxyURL)
			require.NoError(t, err, "解析代理URL失败")
			transport.Proxy = http.ProxyURL(proxyUrl)

			// 设置TLS配置
			transport.TLSClientConfig = &tls.Config{
				InsecureSkipVerify: true,
			}

			// 创建请求
			req, err := http.NewRequest("GET", targetURL, nil)
			require.NoError(t, err, "创建HTTP请求失败")
			req.Header.Set("Connection", "close")

			// 发送请求（应该失败）
			log.Infof("开始无客户端证书的mTLS测试")
			resp, err := httpClient.Do(req)

			// 验证请求失败（此时通过mitm请求 mitm会返回500系状态码并在header头中以Waring头形式告知mitm失败原因 这里err应该为nil）
			if err != nil {
				t.Errorf("当客户端通过MITM访问一个需要MTLS认证的远程目标时，客户端应该正常收到一个状态码为500系的HTTP响应 同时在HEADER头中应该拿到MITM错误原因 err: %s", err.Error())
			} else {

				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				if v, ok := resp.Header["Warning"]; ok {
					if strings.Contains(v[0], "116") { // tls alert 116 = certificate_unknown
						// 这是预期的结果
					} else {
						t.Errorf("MITM错误信息不符合预期 %s", v[0])
					}
				} else {
					t.Errorf("请求成功了，但应该失败（因为没有客户端证书）: 状态码=%d, 响应=%s", resp.StatusCode, string(body))
				}
			}
		},
		// onRecv: 处理接收到的消息（可选）
		nil,
	)
}

// 测试3: MITMv2 mTLS 无客户端证书测试 - 国密TLS服务器场景 - 应该失败
func TestGRPCMUSTPASS_MITMV2_MTLS_WithoutClientCert_GM(t *testing.T) {
	netx.ResetPresetCertificates()
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	// 启动需要客户端证书的国密服务器
	targetHost, targetPort := startTestTLSServer(t, "127.0.0.1", 0, true, true)
	targetURL := fmt.Sprintf("https://%s", utils.HostPort(targetHost, targetPort))

	mitmPort := utils.GetRandomAvailableTCPPort()

	ctx, cancel := context.WithCancel(context.Background())

	// 使用RunMITMV2TestServerEx启动MITM服务器
	RunMITMV2TestServerEx(client, ctx,
		// onInit: 发送初始配置
		func(mitmClient ypb.Yak_MITMV2Client) {
			mitmRequest := &ypb.MITMV2Request{
				Host:             "127.0.0.1",
				Port:             uint32(mitmPort),
				SetAutoForward:   true,
				AutoForwardValue: true,
				EnableHttp2:      false,
				EnableGMTLS:      true, // 启用国密支持
				// 注意：不配置客户端证书，也不调用netx.LoadP12Bytes()
			}
			err := mitmClient.Send(mitmRequest)
			require.NoError(t, err, "发送MITMv2请求失败")
		},
		// onLoad: MITM服务器启动后执行测试
		func(mitmClient ypb.Yak_MITMV2Client) {
			defer cancel()
			proxyURL := fmt.Sprintf("http://127.0.0.1:%d", mitmPort)

			// 创建HTTP客户端，通过MITM代理发送请求（不配置客户端证书）
			httpClient := utils.NewDefaultHTTPClient()
			transport := httpClient.Transport.(*http.Transport)

			// 设置代理
			proxyUrl, err := url.Parse(proxyURL)
			require.NoError(t, err, "解析代理URL失败")
			transport.Proxy = http.ProxyURL(proxyUrl)

			// 设置TLS配置
			transport.TLSClientConfig = &tls.Config{
				InsecureSkipVerify: true,
			}

			// 创建请求
			req, err := http.NewRequest("GET", targetURL, nil)
			require.NoError(t, err, "创建HTTP请求失败")
			req.Header.Set("Connection", "close")

			// 发送请求（应该失败）
			log.Infof("开始无客户端证书的国密mTLS测试")
			resp, err := httpClient.Do(req)

			// 验证请求失败（此时通过mitm请求 mitm会返回500系状态码并在header头中以Warning头形式告知mitm失败原因 这里err应该为nil）
			if err != nil {
				t.Errorf("当客户端通过MITM访问一个需要国密MTLS认证的远程目标时，客户端应该正常收到一个状态码为500系的HTTP响应 同时在HEADER头中应该拿到MITM错误原因 err: %s", err.Error())
			} else {
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				if v, ok := resp.Header["Warning"]; ok {
					if strings.Contains(v[0], "bad certificate") { // tls alert 116 = certificate_unknown
						log.Infof("国密mTLS请求失败（符合预期）: %s", v[0])
						// 这是预期的结果
					} else {
						t.Errorf("国密MITM错误信息不符合预期 %s", v[0])
					}
				} else {
					t.Errorf("国密请求成功了，但应该失败（因为没有客户端证书）: 状态码=%d, 响应=%s", resp.StatusCode, string(body))
				}
			}
		},
		// onRecv: 处理接收到的消息（可选）
		nil,
	)
}

// 测试4: MITMv2 mTLS 混合场景测试 - 一个代理处理多种TLS类型
func TestGRPCMUSTPASS_MITMV2_MTLS_MixedScenarios(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	// 启动两个不同的TLS服务器
	standardHost, standardPort := startTestTLSServer(t, "127.0.0.1", 0, false, true)
	gmHost, gmPort := startTestTLSServer(t, "0.0.0.0", 0, true, true)

	// 配置标准客户端证书（指定Host）
	standardP12, err := tlsutils.BuildP12(standardClientCert, standardClientKey, "", standardCA)
	require.NoError(t, err)
	err = netx.LoadP12Bytes(standardP12, "", standardHost)
	require.NoError(t, err)

	// 配置国密客户端证书（指定Host）
	gmP12, err := tlsutils.BuildP12(gmClientCert, gmClientKey, "", gmCA)
	require.NoError(t, err)
	err = netx.LoadP12Bytes(gmP12, "", gmHost)
	require.NoError(t, err)

	mitmPort := utils.GetRandomAvailableTCPPort()
	ctx, cancel := context.WithCancel(context.Background())
	// 使用RunMITMV2TestServerEx启动MITM服务器
	RunMITMV2TestServerEx(client, ctx,
		// onInit: 发送初始配置
		func(mitmClient ypb.Yak_MITMV2Client) {
			mitmRequest := &ypb.MITMV2Request{
				Host:             "127.0.0.1",
				Port:             uint32(mitmPort),
				SetAutoForward:   true,
				AutoForwardValue: true,
				EnableHttp2:      false,
				EnableGMTLS:      true, // 启用国密支持
				// 客户端证书已通过netx.LoadP12Bytes()配置
			}
			err := mitmClient.Send(mitmRequest)
			require.NoError(t, err, "发送MITMv2请求失败")
		},
		// onLoad: MITM服务器启动后执行测试
		func(mitmClient ypb.Yak_MITMV2Client) {
			defer cancel()
			proxyURL := fmt.Sprintf("http://127.0.0.1:%d", mitmPort)

			// 测试1: 访问标准TLS服务器
			testToken1 := uuid.New().String()
			standardURL := fmt.Sprintf("https://%s", utils.HostPort(standardHost, standardPort))

			log.Infof("测试标准TLS服务器")
			httpClient1 := utils.NewDefaultHTTPClient()
			transport1 := httpClient1.Transport.(*http.Transport)
			proxyUrl1, _ := url.Parse(proxyURL)
			transport1.Proxy = http.ProxyURL(proxyUrl1)
			transport1.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

			req1, _ := http.NewRequest("GET", standardURL, nil)
			req1.Header.Set("X-Test-Token", testToken1)
			req1.Header.Set("Connection", "close")

			resp1, err := httpClient1.Do(req1)
			require.NoError(t, err, "标准TLS请求失败")
			defer resp1.Body.Close()

			body1, err := io.ReadAll(resp1.Body)
			require.NoError(t, err, "读取标准TLS响应失败")
			bodyStr1 := string(body1)
			log.Infof("标准TLS响应: %s", bodyStr1)

			require.Contains(t, bodyStr1, testToken1, "标准TLS测试失败: 未找到测试令牌")
			require.Equal(t, 200, resp1.StatusCode, "标准TLS测试失败: 状态码错误")

			// 测试2: 访问国密TLS服务器
			testToken2 := uuid.New().String()
			gmURL := fmt.Sprintf("https://%s", utils.HostPort(gmHost, gmPort))

			log.Infof("测试国密TLS服务器")
			httpClient2 := utils.NewDefaultHTTPClient()
			transport2 := httpClient2.Transport.(*http.Transport)
			proxyUrl2, _ := url.Parse(proxyURL)
			transport2.Proxy = http.ProxyURL(proxyUrl2)
			transport2.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

			req2, _ := http.NewRequest("GET", gmURL, nil)
			req2.Header.Set("X-Test-Token", testToken2)
			req2.Header.Set("Connection", "close")

			resp2, err := httpClient2.Do(req2)
			require.NoError(t, err, "国密TLS请求失败")
			defer resp2.Body.Close()

			body2, err := io.ReadAll(resp2.Body)
			require.NoError(t, err, "读取国密TLS响应失败")
			bodyStr2 := string(body2)
			log.Infof("国密TLS响应: %s", bodyStr2)

			require.Contains(t, bodyStr2, testToken2, "国密TLS测试失败: 未找到测试令牌")
			require.Equal(t, 200, resp2.StatusCode, "国密TLS测试失败: 状态码错误")

			log.Infof("混合场景测试完成")
		},
		// onRecv: 处理接收到的消息（可选）
		nil,
	)
}
