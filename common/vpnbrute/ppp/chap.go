package ppp

var (
	CHAP_AUTH             = "CHAP"
	CHAP_AUTH_CODE uint16 = 0xC223
)

type CHAPAuth struct {
	Username string
	Password string
}

func (auth *CHAPAuth) Auth() bool {
	//todo CHAP auth
	return false
}

func (auth *CHAPAuth) AuthType() string {
	return CHAP_AUTH
}

func (auth *CHAPAuth) AuthCode() uint16 {
	return CHAP_AUTH_CODE
}
