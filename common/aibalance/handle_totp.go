package aibalance

import (
	"fmt"
	"net"
	"net/http"
)

// ==================== TOTP Settings Handlers ====================

// serveTOTPSettingsPage serves the TOTP settings page for administrators
func (c *ServerConfig) serveTOTPSettingsPage(conn net.Conn) {
	c.logInfo("Serving TOTP settings page")

	// Get current TOTP information
	secret := GetTOTPSecret()
	wrappedUUID := GetWrappedTOTPUUID()
	currentCode := GetCurrentTOTPCode()

	// Build HTML response
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Memfit TOTP Settings - AI Balance Portal</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%%, #16213e 50%%, #0f3460 100%%);
            min-height: 100vh;
            color: #e8e8e8;
            padding: 20px;
        }
        .container {
            max-width: 800px;
            margin: 0 auto;
        }
        .header {
            text-align: center;
            margin-bottom: 30px;
        }
        .header h1 {
            font-size: 2rem;
            color: #00d4aa;
            margin-bottom: 10px;
        }
        .header p {
            color: #888;
        }
        .card {
            background: rgba(255, 255, 255, 0.05);
            border-radius: 16px;
            padding: 30px;
            margin-bottom: 20px;
            border: 1px solid rgba(255, 255, 255, 0.1);
        }
        .card h2 {
            color: #00d4aa;
            margin-bottom: 20px;
            font-size: 1.3rem;
        }
        .info-row {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 15px 0;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }
        .info-row:last-child {
            border-bottom: none;
        }
        .info-label {
            color: #888;
            font-size: 0.9rem;
        }
        .info-value {
            font-family: 'Monaco', 'Menlo', 'Courier New', monospace;
            color: #fff;
            background: rgba(0, 0, 0, 0.3);
            padding: 8px 16px;
            border-radius: 8px;
            max-width: 500px;
            word-break: break-all;
        }
        .info-value.secret {
            color: #ff6b6b;
            font-size: 0.9rem;
        }
        .info-value.code {
            color: #4ecdc4;
            font-size: 1.5rem;
            letter-spacing: 4px;
        }
        .actions {
            display: flex;
            gap: 15px;
            margin-top: 30px;
        }
        .btn {
            padding: 12px 24px;
            border-radius: 8px;
            font-size: 1rem;
            cursor: pointer;
            border: none;
            transition: all 0.3s ease;
        }
        .btn-primary {
            background: linear-gradient(135deg, #00d4aa 0%%, #00a085 100%%);
            color: #fff;
        }
        .btn-primary:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 15px rgba(0, 212, 170, 0.4);
        }
        .btn-danger {
            background: linear-gradient(135deg, #ff6b6b 0%%, #ee5a5a 100%%);
            color: #fff;
        }
        .btn-danger:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 15px rgba(255, 107, 107, 0.4);
        }
        .btn-secondary {
            background: rgba(255, 255, 255, 0.1);
            color: #fff;
            border: 1px solid rgba(255, 255, 255, 0.2);
        }
        .btn-secondary:hover {
            background: rgba(255, 255, 255, 0.15);
        }
        .warning-box {
            background: rgba(255, 193, 7, 0.1);
            border: 1px solid rgba(255, 193, 7, 0.3);
            border-radius: 8px;
            padding: 15px;
            margin-top: 20px;
            color: #ffc107;
        }
        .warning-box strong {
            display: block;
            margin-bottom: 5px;
        }
        .nav-link {
            color: #00d4aa;
            text-decoration: none;
        }
        .nav-link:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>TOTP Settings</h1>
            <p>Manage TOTP authentication for Memfit models</p>
            <p style="margin-top: 10px;"><a href="/portal/" class="nav-link">Back to Portal</a></p>
        </div>

        <div class="card">
            <h2>Current TOTP Configuration</h2>
            <div class="info-row">
                <span class="info-label">TOTP Secret (UUID)</span>
                <span class="info-value secret">%s</span>
            </div>
            <div class="info-row">
                <span class="info-label">Wrapped UUID (For Clients)</span>
                <span class="info-value">%s</span>
            </div>
            <div class="info-row">
                <span class="info-label">Current TOTP Code</span>
                <span class="info-value code">%s</span>
            </div>
            <div class="info-row">
                <span class="info-label">Code Validity</span>
                <span class="info-value">30 seconds (with 60s tolerance)</span>
            </div>
        </div>

        <div class="card">
            <h2>Actions</h2>
            <p style="color: #888; margin-bottom: 15px;">
                Refreshing the TOTP secret will invalidate all existing client configurations.
                Clients will need to fetch the new secret.
            </p>
            <div class="actions">
                <button class="btn btn-danger" onclick="refreshTOTP()">Refresh TOTP Secret</button>
                <button class="btn btn-secondary" onclick="location.reload()">Refresh Page</button>
            </div>
            <div class="warning-box">
                <strong>Warning</strong>
                Refreshing the TOTP secret will require all clients using memfit- models to update their TOTP configuration.
                Only do this if you suspect the secret has been compromised.
            </div>
        </div>

        <div class="card">
            <h2>Usage Guide</h2>
            <p style="color: #888; line-height: 1.8;">
                1. Clients can get the TOTP UUID from: <code style="background: rgba(0,0,0,0.3); padding: 2px 6px; border-radius: 4px;">/v1/memfit-totp-uuid</code><br>
                2. When accessing memfit- models, include header: <code style="background: rgba(0,0,0,0.3); padding: 2px 6px; border-radius: 4px;">X-Memfit-OTP-Auth: {base64_encoded_totp_code}</code><br>
                3. TOTP codes are valid for 30 seconds with a 60-second tolerance window.<br>
                4. If authentication fails, clients should refresh their TOTP secret.
            </p>
        </div>
    </div>

    <script>
        function refreshTOTP() {
            if (!confirm('Are you sure you want to refresh the TOTP secret? This will invalidate all existing client configurations.')) {
                return;
            }
            fetch('/portal/refresh-totp', { method: 'POST', credentials: 'same-origin' })
                .then(response => response.json())
                .then(data => {
                    if (data.success) {
                        alert('TOTP secret refreshed successfully!');
                        location.reload();
                    } else {
                        alert('Failed to refresh TOTP secret: ' + data.message);
                    }
                })
                .catch(err => {
                    alert('Error: ' + err.message);
                });
        }
    </script>
</body>
</html>`, secret, wrappedUUID, currentCode)

	// Send response
	header := fmt.Sprintf("HTTP/1.1 200 OK\r\n"+
		"Content-Type: text/html; charset=utf-8\r\n"+
		"Content-Length: %d\r\n"+
		"\r\n", len(html))

	conn.Write([]byte(header))
	conn.Write([]byte(html))
	c.logInfo("TOTP settings page sent")
}

// handleRefreshTOTP handles the request to refresh the TOTP secret
func (c *ServerConfig) handleRefreshTOTP(conn net.Conn, request *http.Request) {
	c.logInfo("Handling TOTP refresh request")

	newSecret, err := RefreshTOTPSecret()
	if err != nil {
		c.logError("Failed to refresh TOTP secret: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to refresh TOTP secret: %v", err),
		})
		return
	}

	c.logInfo("TOTP secret refreshed successfully, new secret: %s", newSecret)

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":    true,
		"message":    "TOTP secret refreshed successfully",
		"new_secret": newSecret,
		"wrapped":    GetWrappedTOTPUUID(),
	})
}

// handleGetTOTPCode handles the request to get current TOTP code
func (c *ServerConfig) handleGetTOTPCode(conn net.Conn, request *http.Request) {
	c.logInfo("Handling get TOTP code request")

	code := GetCurrentTOTPCode()
	if code == "" {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "TOTP not initialized",
		})
		return
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"code":    code,
	})
}
