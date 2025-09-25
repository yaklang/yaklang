package forge

import (
	"context"
	"testing"
)

func TestNewLoginElementExtractor(t *testing.T) {
	extractor, err := NewLoginElementExtractor()
	if err != nil {
		t.Fatalf("Failed to create login element extractor: %v", err)
	}

	if extractor == nil {
		t.Fatal("Extractor should not be nil")
	}

	if extractor.LiteForge == nil {
		t.Fatal("LiteForge should not be nil")
	}
}

func TestExtractLoginElements_BasicForm(t *testing.T) {
	extractor, err := NewLoginElementExtractor()
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	htmlContent := `
<!DOCTYPE html>
<html>
<head>
    <title>登录页面</title>
</head>
<body>
    <form id="loginForm">
        <div>
            <label for="username">用户名:</label>
            <input type="text" id="username" name="username" required>
        </div>
        <div>
            <label for="password">密码:</label>
            <input type="password" id="password" name="password" required>
        </div>
        <button type="submit" id="loginBtn">登录</button>
    </form>
</body>
</html>`

	ctx := context.Background()
	elements, err := extractor.ExtractLoginElements(ctx, htmlContent)
	if err != nil {
		t.Fatalf("Failed to extract login elements: %v", err)
	}

	// 验证必需字段
	if elements.UsernameSelector == "" {
		t.Error("UsernameSelector should not be empty")
	}
	if elements.PasswordSelector == "" {
		t.Error("PasswordSelector should not be empty")
	}
	if elements.LoginButtonSelector == "" {
		t.Error("LoginButtonSelector should not be empty")
	}
	if elements.Confidence < 0 || elements.Confidence > 1 {
		t.Errorf("Confidence should be between 0 and 1, got: %f", elements.Confidence)
	}

	t.Logf("Extracted elements: %+v", elements)
}

func TestExtractLoginElements_WithCaptcha(t *testing.T) {
	extractor, err := NewLoginElementExtractor()
	if err != nil {
		t.Fatalf("Failed to create extractor: %v", err)
	}

	htmlContent := `
<!DOCTYPE html>
<html>
<body>
    <div class="login-container">
        <form class="login-form" action="/login" method="post">
            <input type="text" name="user" id="user-input" placeholder="用户名">
            <input type="password" name="pass" id="pass-input" placeholder="密码">
            <div class="captcha-section">
                <img src="/captcha.jpg" alt="验证码" id="captcha-image">
                <input type="text" name="captcha" id="captcha-input" placeholder="验证码">
            </div>
            <button type="submit" class="submit-btn">登录</button>
        </form>
    </div>
</body>
</html>`

	ctx := context.Background()
	elements, err := extractor.ExtractLoginElements(ctx, htmlContent)
	if err != nil {
		t.Fatalf("Failed to extract login elements: %v", err)
	}

	// 验证基本字段
	if elements.UsernameSelector == "" {
		t.Error("UsernameSelector should not be empty")
	}
	if elements.PasswordSelector == "" {
		t.Error("PasswordSelector should not be empty")
	}
	if elements.LoginButtonSelector == "" {
		t.Error("LoginButtonSelector should not be empty")
	}

	// 验证验证码相关字段（应该被识别）
	if elements.CaptchaImageSelector == "" {
		t.Error("CaptchaImageSelector should not be empty for this HTML")
	}
	if elements.CaptchaInputSelector == "" {
		t.Error("CaptchaInputSelector should not be empty for this HTML")
	}

	t.Logf("Extracted elements with captcha: %+v", elements)
}
