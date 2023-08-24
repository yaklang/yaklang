// Package simulator
// @Author bcy2007  2023/8/22 10:24
package simulator

type Result interface {
	Username() string
	Password() string
	Status() bool

	Info() string
	Base64() string

	LoginToken() string
	LoginSuccessUrl() string
}

type BruteResult struct {
	username string
	password string
	status   bool

	bruteInfo string
	b64       string

	token           string
	loginSuccessUrl string
}

func (result *BruteResult) Username() string {
	return result.username
}

func (result *BruteResult) Password() string {
	return result.password
}

func (result *BruteResult) Status() bool {
	return result.status
}

func (result *BruteResult) Info() string {
	return result.bruteInfo
}

func (result *BruteResult) Base64() string {
	return result.b64
}

func (result *BruteResult) LoginToken() string {
	return result.token
}

func (result *BruteResult) LoginSuccessUrl() string {
	return result.loginSuccessUrl
}
