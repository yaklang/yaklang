package yaklib

import "github.com/yaklang/yaklang/common/utils/bot"

var BotExports = map[string]interface{}{
	"New":               bot.New,
	"FromEnv":           bot.FromEnv,
	"webhook":           bot.WithWebhook,
	"webhookWithSecret": bot.WithWebhookWithSecret,
	"workwx":            bot.WithWebhook,
	"ding":              bot.WithWebhookWithSecret,
}
