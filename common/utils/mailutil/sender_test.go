package mailutil

import (
	"context"
	"gopkg.in/gomail.v2"
	"os"
	"testing"
)

func TestNewSMTPMailSender(t *testing.T) {
	yandexMail := os.Getenv("YandexMail")
	yandexPass := os.Getenv("YandexPass")
	yandexSender, err := NewSMTPMailSender(&SMTPConfig{
		Server:     "smtp.qq.com",
		Port:       465,
		ConnectSSL: true,
		Username:   yandexMail,
		Password:   yandexPass,
	})
	if err != nil {
		t.Errorf("build new smtp sender failed: %s", err)
		t.FailNow()
	}

	msg := gomail.NewMessage()
	msg.SetHeader("From", "tFrom Who")
	msg.SetHeader("To", "The One")
	msg.SetHeader("Subject", "This is Subject")
	msg.SetBody("text/html", "<h2>Hello World<h2>")

	if ok, _ := yandexSender.IsAvailable(context.Background()); !ok {
		t.Error("unavailable smtp client config")
		t.FailNow()
	}

	//err = yandexSender.SendWithContext(context.Background(), os.Getenv("SendMailTo"), msg)
	//if err != nil {
	//	t.Errorf("send mail failed; %s", err)
	//	t.FailNow()
	//}

}
