package ppp

var (
	PAP_AUTH             = "PAP"
	PAP_AUTH_CODE uint16 = 0xC023
)

type PAPAuth struct {
	Username string
	Password string
}

func (auth *PAPAuth) Auth() bool {
	//todo PAP auth
	return false
}

func (auth *PAPAuth) AuthType() string {
	return PAP_AUTH
}

func (auth *PAPAuth) AuthCode() uint16 {
	return PAP_AUTH_CODE
}
