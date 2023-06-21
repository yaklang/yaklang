// Package httpbrute
// @Author bcy2007  2023/6/20 14:47
package httpbrute

type Result interface {
	Username() string
	Password() string
	Status() bool

	Info() string
	Base64() string
}

type BruteResult struct {
	username string
	password string
	status   bool

	bruteInfo string
	loginB64  string
}

func (bruteResult *BruteResult) Username() string {
	return bruteResult.username
}

func (bruteResult *BruteResult) Password() string {
	return bruteResult.password
}

func (bruteResult *BruteResult) Status() bool {
	return bruteResult.status
}

func (bruteResult *BruteResult) Info() string {
	return bruteResult.bruteInfo
}

func (bruteResult *BruteResult) Base64() string {
	return bruteResult.loginB64
}
