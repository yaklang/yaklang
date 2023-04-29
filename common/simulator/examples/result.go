package examples

type BruteForceResult struct {
	username string
	password string

	loginPngB64 string

	cookie string

	logs []string
}

func (result *BruteForceResult) SetUsername(username string) {
	result.username = username
}

func (result *BruteForceResult) SetPassword(password string) {
	result.password = password
}

func (result *BruteForceResult) SetLoginPngB64(b64 string) {
	result.loginPngB64 = b64
}

func (result *BruteForceResult) SetCookie(cookie string) {
	result.cookie = cookie
}

func (result *BruteForceResult) AddLog(logStr string) {
	result.logs = append(result.logs, logStr)
}

func (result *BruteForceResult) Username() string {
	return result.username
}

func (result *BruteForceResult) Password() string {
	return result.password
}

func (result *BruteForceResult) LoginPngB64() string {
	return result.loginPngB64
}

func (result *BruteForceResult) Cookie() string {
	return result.cookie
}

func (result *BruteForceResult) Log() []string {
	return result.logs
}
