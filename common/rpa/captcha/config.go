package captcha

type CaptchaRequest struct {
	Project_name string `json:"project_name"`
	Image        string `json:"image"`
}

type CaptchaResult struct {
	Uuid    string `json:"uuid"`
	Data    string `json:"data"`
	Success bool   `json:"success"`
}

const CAPTCHA_URL = "http://101.35.184.3:19199/runtime/text/invoke"
