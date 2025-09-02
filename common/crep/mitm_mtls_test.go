package crep

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

var ca = []byte(`-----BEGIN CERTIFICATE-----
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

var serverCert = []byte(`-----BEGIN CERTIFICATE-----
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

var serverKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
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

var clientCert = []byte(`-----BEGIN CERTIFICATE-----
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

var clientKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
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

var gmCA = []byte(`-----BEGIN CERTIFICATE-----
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

var gmCAKey = []byte(`-----BEGIN PRIVATE KEY-----
MIGTAgEAMBMGByqGSM49AgEGCCqBHM9VAYItBHkwdwIBAQQgEzTXyGC6rwpLp7jt
/jNNrqJoyWvwboFHBE4x0krpXcegCgYIKoEcz1UBgi2hRANCAASC1/f5uZPYFpDX
H0sfDcg+bCEs8HdPlZgJ4yKFkqeYr7Ovttqi80mioJyPlRnDuPFqun1RsD7TXpkx
H0aO6TdR
-----END PRIVATE KEY-----`)

var gmServerCert = []byte(`-----BEGIN CERTIFICATE-----
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

var gmServerKey = []byte(`-----BEGIN PRIVATE KEY-----
MIGTAgEAMBMGByqGSM49AgEGCCqBHM9VAYItBHkwdwIBAQQgS4jjdCZaKRrSl3EV
UzMRZE1zEAlUpHmNlk/6L9y3Mw+gCgYIKoEcz1UBgi2hRANCAASLvpGQweQ+DpDC
FFnXGOS95k3LVvEHjjQfAQi3SBf1cibwaQAzFKuVTeKepeI8d/6ibfvC9ck/wOGw
XA0Q5XnY
-----END PRIVATE KEY-----`)

var gmClientCert = []byte(`-----BEGIN CERTIFICATE-----
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

var gmClientKey = []byte(`-----BEGIN PRIVATE KEY-----
MIGTAgEAMBMGByqGSM49AgEGCCqBHM9VAYItBHkwdwIBAQQg8fCQoDha5V6rzTVg
g8i13+nUyBDUeoS0QP1qHhJ0BaKgCgYIKoEcz1UBgi2hRANCAATkK3PcFKh1C0Og
q3k4EiJxT2w56jhS36XUPhCYBGZwrOn2S7XoyuYawpKz71JCboEFOvqRzJErURWc
KqlRsW7K
-----END PRIVATE KEY-----`)

func TestMTLS_MITM_GENERATE_CERTS(t *testing.T) {
	ca, key, _ := tlsutils.GenerateSelfSignedCertKeyWithCommonName("test", "", nil, nil)
	sCert, sKey, _ := tlsutils.SignServerCrtNKeyWithParams(ca, key, "127.0.0.1", time.Now().Add(24*time.Hour*365*10), true)
	println(string(ca))
	println(string(sCert))
	println(string(sKey))
	clientCert, clientKey, _ := tlsutils.SignClientCrtNKeyEx(ca, key, "CLIENT", true)
	println(string(clientCert))
	println(string(clientKey))
}

// 国密证书生成测试函数
func TestGMMTLS_MITM_GENERATE_CERTS(t *testing.T) {
	// 生成国密CA证书和私钥
	gmCA, gmCAKey, err := tlsutils.GenerateGMSelfSignedCertKey("test")
	if err != nil {
		t.Fatalf("生成国密CA证书失败: %v", err)
	}
	println("国密CA证书:")
	println(string(gmCA))
	println("国密CA私钥:")
	println(string(gmCAKey))

	// 生成国密服务器证书和私钥
	gmServerCert, gmServerKey, err := tlsutils.SignGMServerCrtNKeyWithParams(gmCA, gmCAKey, "127.0.0.1", time.Now().Add(24*time.Hour*365*10), true)
	if err != nil {
		t.Fatalf("生成国密服务器证书失败: %v", err)
	}
	println("国密服务器证书:")
	println(string(gmServerCert))
	println("国密服务器私钥:")
	println(string(gmServerKey))

	// 生成国密客户端证书和私钥 - 使用现有的SignGMClientCrtNKeyWithParams函数来生成客户端证书
	gmClientCert, gmClientKey, err := tlsutils.SignGMClientCrtNKeyWithParams(gmCA, gmCAKey, "GM_CLIENT", time.Now().Add(24*time.Hour*365*10), true)
	if err != nil {
		t.Fatalf("生成国密客户端证书失败: %v", err)
	}
	println("国密客户端证书:")
	println(string(gmClientCert))
	println("国密客户端私钥:")
	println(string(gmClientKey))
}

func TestMTLS_MITM_StartLongServer(t *testing.T) {
	uid := uuid.New().String()
	println(uid)
	println("Start Long mtls server")
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(uid))
	}))
	server.TLS, _ = tlsutils.GetX509MutualAuthServerTlsConfig(ca, serverCert, serverKey)
	server.StartTLS()
	println(server.URL)
	time.Sleep(time.Hour)
}

func TestGMMTLS_MITM_StartLongServer(t *testing.T) {
	uid := uuid.New().String()
	println(uid)
	println("Start Long GM mTLS server")
	port := utils.GetRandomAvailableTCPPort()

	// 配置国密双向认证TLS
	gmConfig, err := tlsutils.GetX509GMServerTlsConfigWithAuth(gmCA, gmServerCert, gmServerKey, true)
	if err != nil {
		t.Fatalf("配置国密双向认证服务器失败: %v", err)
	}
	ln, err := gmtls.Listen("tcp", fmt.Sprintf(":%d", port), gmConfig)
	if err != nil {
		log.Println(err)
		return
	}
	defer ln.Close()

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(uid))
	})
	fmt.Printf(">> HTTP Over [GMSSL/TLS] running on port %d...\n", port)
	go func() {
		err = http.Serve(ln, nil)
		if err != nil {
			panic(err)
		}
	}()
	time.Sleep(time.Hour)
}

func TestMTLS_MITM(t *testing.T) {
	uid := uuid.New().String()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(uid))
	}))
	server.TLS, _ = tlsutils.GetX509MutualAuthServerTlsConfig(ca, serverCert, serverKey)
	go func() {
		server.StartTLS()
	}()
	time.Sleep(time.Second)

	client := utils.NewDefaultHTTPClient()
	_, err := client.Get(server.URL)
	if err == nil {
		panic("mTLS not valid")
	}
	spew.Dump(err)

	tr := client.Transport.(*http.Transport)
	tr.TLSClientConfig, err = tlsutils.GetX509MutualAuthClientTlsConfig(clientCert, clientKey, ca)
	if err != nil {
		panic(err)
	}
	tr.TLSClientConfig.InsecureSkipVerify = true
	var rsp, _ = client.Get(server.URL)
	if rsp == nil {
		panic("mTLS not valid in realtime")
	}
	var result, _ = utils.HttpDumpWithBody(rsp, true)
	if !utils.IContains(string(result), uid) {
		panic("mTLS failed")
	}

	p12Bytes, err := tlsutils.BuildP12(clientCert, clientKey, "", ca)
	if err != nil {
		t.Fatal(err)
	}
	err = netx.LoadP12Bytes(p12Bytes, "", "")
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewMITMServer()
	if err != nil {
		panic(err)
	}
	proxyPort := utils.GetRandomAvailableTCPPort()
	proxyUrl := fmt.Sprintf("http://127.0.0.1:%v", proxyPort)
	go func() {
		err := proxy.Serve(context.Background(), "127.0.0.1:"+fmt.Sprint(proxyPort))
		if err != nil {
			panic(err)
		}
	}()
	time.Sleep(time.Second)

	client = utils.NewDefaultHTTPClient()
	tr = client.Transport.(*http.Transport)
	tr.Proxy = func(request *http.Request) (*url.URL, error) {
		return url.Parse(proxyUrl)
	}
	_ = tr
	//tr.TLSClientConfig.GetConfigForClient = func(info *tls.ClientHelloInfo) (*tls.Config, error) {
	//	return
	//}
	rsp, err = client.Get(server.URL)
	if err != nil {
		panic(err)
	}
	raw, _ := utils.HttpDumpWithBody(rsp, true)
	if !utils.IContains(string(raw), uid) {
		panic("MITM mTLS Failed")
	}
}

func TestGMMTLS_MITM(t *testing.T) {
	uid := uuid.New().String()
	port := utils.GetRandomAvailableTCPPort()

	// 配置国密双向认证TLS服务器
	gmConfig, err := tlsutils.GetX509GMServerTlsConfigWithAuth(gmCA, gmServerCert, gmServerKey, true)
	if err != nil {
		t.Fatalf("配置国密双向认证服务器失败: %v", err)
	}

	// 启动真正的国密TLS服务器
	ln, err := gmtls.Listen("tcp", fmt.Sprintf(":%d", port), gmConfig)
	if err != nil {
		t.Fatalf("启动国密TLS监听失败: %v", err)
	}
	defer ln.Close()

	serverURL := fmt.Sprintf("https://127.0.0.1:%d", port)

	// 启动HTTP服务器
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
			writer.Write([]byte(uid))
		})
		err = http.Serve(ln, mux)
		if err != nil {
			t.Logf("国密HTTP服务器错误: %v", err)
		}
	}()
	time.Sleep(time.Second * 1)

	// 测试1: 验证没有客户端证书时连接失败（使用标准HTTP客户端）
	client := utils.NewDefaultHTTPClient()
	tr := client.Transport.(*http.Transport)
	tr.TLSClientConfig.InsecureSkipVerify = true
	_, err = client.Get(serverURL)
	if err == nil {
		t.Fatal("国密mTLS验证失败: 应该要求客户端证书")
	}
	spew.Dump(err)

	// 测试2: 使用国密客户端直接连接国密服务器
	gmClientTLSConfig, err := tlsutils.GetX509GMMutualAuthClientTlsConfig(gmClientCert, gmClientKey, gmCA)
	if err != nil {
		t.Fatalf("配置国密双向认证客户端失败: %v", err)
	}

	// 使用国密TLS客户端连接
	gmConn, err := gmtls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port), gmClientTLSConfig)
	if err != nil {
		t.Fatalf("国密TLS客户端连接失败: %v", err)
	}
	defer gmConn.Close()

	// 发送HTTP请求
	httpReq := fmt.Sprintf("GET / HTTP/1.1\r\nHost: 127.0.0.1:%d\r\nConnection: close\r\n\r\n", port)
	_, err = gmConn.Write([]byte(httpReq))
	if err != nil {
		t.Fatalf("发送HTTP请求失败: %v", err)
	}

	// 读取响应
	response := make([]byte, 1024)
	n, err := gmConn.Read(response)
	if err != nil && err != io.EOF {
		t.Fatalf("读取响应失败: %v", err)
	}

	responseStr := string(response[:n])
	if !utils.IContains(responseStr, uid) {
		t.Fatalf("国密mTLS直接连接响应验证失败，期望包含: %s, 实际响应: %s", uid, responseStr)
	}
	t.Logf("国密mTLS直接连接测试成功!")

	// 配置国密客户端证书
	p12Bytes, err := tlsutils.BuildP12(gmClientCert, gmClientKey, "", gmCA)
	if err != nil {
		t.Fatal(err)
	}
	err = netx.LoadP12Bytes(p12Bytes, "", "")
	if err != nil {
		t.Fatal(err)
	}
	// 测试3: 通过MITM代理进行国密mTLS
	proxy, err := NewMITMServer(
		MITM_SetGM(true),       // 启用国密支持
		MITM_SetGMPrefer(true), // 优先使用国密
	)
	if err != nil {
		t.Fatalf("创建国密MITM代理失败: %v", err)
	}

	proxyPort := utils.GetRandomAvailableTCPPort()
	proxyUrl := fmt.Sprintf("http://127.0.0.1:%v", proxyPort)

	go func() {
		err := proxy.Serve(context.Background(), "127.0.0.1:"+fmt.Sprint(proxyPort))
		if err != nil {
			t.Logf("MITM代理服务错误: %v", err)
		}
	}()
	time.Sleep(time.Second)

	// 配置客户端使用MITM代理
	client = utils.NewDefaultHTTPClient()
	tr = client.Transport.(*http.Transport)
	tr.Proxy = func(request *http.Request) (*url.URL, error) {
		return url.Parse(proxyUrl)
	}
	tr.TLSClientConfig.InsecureSkipVerify = true

	client.Timeout = time.Second * 60
	rsp, err := client.Get(serverURL)
	if err != nil {
		t.Fatalf("通过MITM代理的国密mTLS连接失败: %v", err)
	}

	raw, _ := utils.HttpDumpWithBody(rsp, true)
	if !utils.IContains(string(raw), uid) {
		t.Error(rsp)
		t.Fatal("MITM国密mTLS代理测试失败")
	}

	t.Logf("国密mTLS MITM代理测试成功!")
}
