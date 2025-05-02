package aibalance

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed templates/portal.html templates/login.html templates/add_provider.html templates/healthy_check.html templates/api_keys.html
var templatesFS embed.FS

// ProviderData 包含用于模板渲染的提供者数据
type ProviderData struct {
	ID            uint
	WrapperName   string
	ModelName     string
	TypeName      string
	DomainOrURL   string
	TotalRequests int64
	SuccessRate   float64
	LastLatency   int64
	IsHealthy     bool
}

// PortalData 包含管理面板页面的所有数据
type PortalData struct {
	CurrentTime      string
	TotalProviders   int
	HealthyProviders int
	TotalRequests    int64
	SuccessRate      float64
	Providers        []ProviderData
	AllowedModels    map[string]string
}

// Session 代表一个用户会话
type Session struct {
	ID        string    // 会话ID
	CreatedAt time.Time // 创建时间
	ExpiresAt time.Time // 过期时间
}

// SessionManager 管理用户会话
type SessionManager struct {
	sessions map[string]*Session // 会话存储
	mutex    sync.RWMutex        // 读写锁保护会话映射
}

// NewSessionManager 创建新的会话管理器
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession 创建新会话
func (sm *SessionManager) CreateSession() *Session {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// 创建新的UUID作为会话ID
	sessionID := uuid.NewString()

	// 创建会话，设置24小时过期
	now := time.Now()
	session := &Session{
		ID:        sessionID,
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
	}

	// 存储会话
	sm.sessions[sessionID] = session

	return session
}

// GetSession 获取会话
func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil, false
	}

	// 检查会话是否过期
	if time.Now().After(session.ExpiresAt) {
		// 删除过期会话
		sm.mutex.RUnlock()
		sm.mutex.Lock()
		delete(sm.sessions, sessionID)
		sm.mutex.Unlock()
		sm.mutex.RLock()

		return nil, false
	}

	return session, true
}

// DeleteSession 删除会话
func (sm *SessionManager) DeleteSession(sessionID string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	delete(sm.sessions, sessionID)
}

// CleanupExpiredSessions 清理过期会话
func (sm *SessionManager) CleanupExpiredSessions() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	now := time.Now()
	for id, session := range sm.sessions {
		if now.After(session.ExpiresAt) {
			delete(sm.sessions, id)
		}
	}
}

// checkAuth 检查管理员认证，使用会话ID而非直接使用密码
func (c *ServerConfig) checkAuth(request *http.Request) bool {
	// 从Cookie中获取会话ID
	cookies := request.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "admin_session" {
			// 验证会话是否有效
			_, valid := c.SessionManager.GetSession(cookie.Value)
			if valid {
				return true
			}
		}
	}

	// 从查询参数获取密码认证
	query := request.URL.Query()
	password := query.Get("password")
	if password == c.AdminPassword {
		// 密码认证成功，但不生成会话，这仅用于单次访问
		return true
	}

	return false
}

// serveLoginPage 显示登录页面
func (c *ServerConfig) serveLoginPage(conn net.Conn) {
	c.logInfo("Serving login page")

	var tmpl *template.Template
	var err error

	// 尝试从文件系统读取模板
	if result := utils.GetFirstExistedFile(
		"common/aibalance/templates/login.html",
		"templates/login.html",
		"../templates/login.html",
	); result != "" {
		rawTemp, err := os.ReadFile(result)
		if err != nil {
			c.logError("Failed to read login template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板读取失败: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
		tmpl, err = template.New("login").Parse(string(rawTemp))
		if err != nil {
			c.logError("Failed to parse login template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板解析失败: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	} else {
		// 使用嵌入式文件系统中的模板
		tmpl, err = template.ParseFS(templatesFS, "templates/login.html")
		if err != nil {
			c.logError("Failed to parse embedded login template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板解析失败: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	}

	// 创建一个缓冲区来保存渲染后的HTML
	var htmlBuffer bytes.Buffer
	err = tmpl.Execute(&htmlBuffer, nil)
	if err != nil {
		c.logError("Failed to execute login template: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板渲染失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 准备HTTP响应头
	header := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", htmlBuffer.Len()) + "\r\n" +
		"\r\n"

	// 写入头部和HTML内容
	conn.Write([]byte(header))
	conn.Write(htmlBuffer.Bytes())
}

// processLogin 处理登录请求
func (c *ServerConfig) processLogin(conn net.Conn, request *http.Request) {
	// 解析表单数据
	err := request.ParseForm()
	if err != nil {
		c.logError("Failed to parse login form: %v", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	// 获取提交的密码
	password := request.PostForm.Get("password")

	// 验证密码
	if password != c.AdminPassword {
		// 密码错误，重定向回登录页
		header := "HTTP/1.1 303 See Other\r\n" +
			"Location: /portal?error=invalid_password\r\n" +
			"\r\n"
		conn.Write([]byte(header))
		return
	}

	// 创建新会话
	session := c.SessionManager.CreateSession()

	// 设置会话Cookie并重定向到管理面板
	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /portal\r\n" +
		"Set-Cookie: admin_session=" + session.ID + "; Path=/; HttpOnly; SameSite=Strict\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

// servePortal 处理管理面板页面的请求
func (c *ServerConfig) servePortal(conn net.Conn) {
	c.logInfo("Serving portal page")

	// 获取所有提供者
	providers, err := GetAllAiProviders()
	if err != nil {
		c.logError("Failed to get providers: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n获取提供者信息失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 准备模板数据
	data := PortalData{
		CurrentTime:   time.Now().Format("2006-01-02 15:04:05"),
		TotalRequests: 0,
	}

	// 处理提供者数据
	var totalSuccess int64
	healthyCount := 0

	for _, p := range providers {
		// 计算成功率
		successRate := 0.0
		if p.TotalRequests > 0 {
			successRate = float64(p.SuccessCount) / float64(p.TotalRequests) * 100
		}

		// 添加到提供者列表
		data.Providers = append(data.Providers, ProviderData{
			ID:            p.ID,
			WrapperName:   p.WrapperName,
			ModelName:     p.ModelName,
			TypeName:      p.TypeName,
			DomainOrURL:   p.DomainOrURL,
			TotalRequests: p.TotalRequests,
			SuccessRate:   successRate,
			LastLatency:   p.LastLatency,
			IsHealthy:     p.IsHealthy,
		})

		// 累计统计数据
		data.TotalRequests += p.TotalRequests
		totalSuccess += p.SuccessCount
		if p.IsHealthy {
			healthyCount++
		}
	}

	// 设置总体统计数据
	data.TotalProviders = len(providers)
	data.HealthyProviders = healthyCount

	// 计算总体成功率
	if data.TotalRequests > 0 {
		data.SuccessRate = float64(totalSuccess) / float64(data.TotalRequests) * 100
	}

	// 获取API密钥和允许的模型
	data.AllowedModels = make(map[string]string)
	for _, key := range c.KeyAllowedModels.Keys() {
		models, _ := c.KeyAllowedModels.Get(key)
		modelNames := make([]string, 0, len(models))
		for model := range models {
			modelNames = append(modelNames, model)
		}
		data.AllowedModels[key] = strings.Join(modelNames, ", ")
	}

	var tmpl *template.Template

	if result := utils.GetFirstExistedFile(
		"common/aibalance/templates/portal.html",
		"templates/portal.html",
		"../templates/portal.html",
	); result != "" {
		rawTemp, err := os.ReadFile(result)
		if err != nil {
			c.logError("Failed to read template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板读取失败: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
		tmpl, err = template.New("portal").Parse(string(rawTemp))
		if err != nil {
			c.logError("Failed to parse template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板解析失败: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	} else {
		// 渲染模板
		tmpl, err = template.ParseFS(templatesFS, "templates/portal.html")
		if err != nil {
			c.logError("Failed to parse template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板解析失败: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	}

	// 创建一个缓冲区来保存渲染后的HTML
	var htmlBuffer bytes.Buffer
	err = tmpl.Execute(&htmlBuffer, data)
	if err != nil {
		c.logError("Failed to execute template: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板渲染失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 准备HTTP响应头
	header := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", htmlBuffer.Len()) + "\r\n" +
		"\r\n"

	// 写入头部和HTML内容
	conn.Write([]byte(header))
	conn.Write(htmlBuffer.Bytes())
}

// servePortalWithAuth 处理管理面板请求，使用会话ID而非密码
func (c *ServerConfig) servePortalWithAuth(conn net.Conn) {
	// 直接调用渲染页面的方法，认证已在上层完成
	c.servePortal(conn)
}

// serveAddProviderPage 处理添加AI提供者请求
func (c *ServerConfig) serveAddProviderPage(conn net.Conn, request *http.Request) {
	// 判断是GET还是POST请求
	if request.Method == "POST" {
		// 处理添加提供者的表单提交
		// TODO: 解析表单数据并添加新的AI提供者

		// 重定向回主页
		header := "HTTP/1.1 303 See Other\r\n" +
			"Location: /portal\r\n" +
			"\r\n"
		conn.Write([]byte(header))
	} else {
		c.logInfo("Serving add provider page")

		var tmpl *template.Template
		var err error

		// 尝试从文件系统读取模板
		if result := utils.GetFirstExistedFile(
			"common/aibalance/templates/add_provider.html",
			"templates/add_provider.html",
			"../templates/add_provider.html",
		); result != "" {
			rawTemp, err := os.ReadFile(result)
			if err != nil {
				c.logError("Failed to read add provider template: %v", err)
				errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板读取失败: %v", err)
				conn.Write([]byte(errorResponse))
				return
			}
			tmpl, err = template.New("add_provider").Parse(string(rawTemp))
			if err != nil {
				c.logError("Failed to parse add provider template: %v", err)
				errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板解析失败: %v", err)
				conn.Write([]byte(errorResponse))
				return
			}
		} else {
			// 使用嵌入式文件系统中的模板
			tmpl, err = template.ParseFS(templatesFS, "templates/add_provider.html")
			if err != nil {
				c.logError("Failed to parse embedded add provider template: %v", err)
				errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板解析失败: %v", err)
				conn.Write([]byte(errorResponse))
				return
			}
		}

		// 创建一个缓冲区来保存渲染后的HTML
		var htmlBuffer bytes.Buffer
		err = tmpl.Execute(&htmlBuffer, nil)
		if err != nil {
			c.logError("Failed to execute add provider template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板渲染失败: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}

		// 准备HTTP响应头
		header := "HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/html; charset=utf-8\r\n" +
			"Content-Length: " + fmt.Sprintf("%d", htmlBuffer.Len()) + "\r\n" +
			"\r\n"

		// 写入头部和HTML内容
		conn.Write([]byte(header))
		conn.Write(htmlBuffer.Bytes())
	}
}

// processAddProviders 处理批量添加AI提供者请求
func (c *ServerConfig) processAddProviders(conn net.Conn, request *http.Request) {
	c.logInfo("Processing add providers request")

	// 解析表单数据
	err := request.ParseForm()
	if err != nil {
		c.logError("Failed to parse form: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 400 Bad Request\r\n\r\n表单解析失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 获取表单数据
	wrapperName := request.PostForm.Get("wrapper_name")
	modelName := request.PostForm.Get("model_name")
	modelType := request.PostForm.Get("model_type")
	domainOrURL := request.PostForm.Get("domain_or_url")
	apiKeysStr := request.PostForm.Get("api_keys")

	// 验证必填字段
	if wrapperName == "" || modelName == "" || modelType == "" || domainOrURL == "" || apiKeysStr == "" {
		c.logError("Missing required fields")
		errorResponse := "HTTP/1.1 400 Bad Request\r\n\r\n所有字段都是必填的"
		conn.Write([]byte(errorResponse))
		return
	}

	// 按行分割API密钥
	apiKeys := make([]string, 0)
	for _, line := range strings.Split(apiKeysStr, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			apiKeys = append(apiKeys, line)
		}
	}

	if len(apiKeys) == 0 {
		c.logError("No valid API keys provided")
		errorResponse := "HTTP/1.1 400 Bad Request\r\n\r\n没有提供有效的API密钥"
		conn.Write([]byte(errorResponse))
		return
	}

	// 创建ConfigProvider对象
	configProvider := &ConfigProvider{
		ModelName:   modelName,
		TypeName:    modelType,
		DomainOrURL: domainOrURL,
		Keys:        apiKeys,
	}

	// 转换为Provider对象
	providers := configProvider.ToProviders()
	if len(providers) == 0 {
		c.logError("Failed to create providers")
		errorResponse := "HTTP/1.1 400 Bad Request\r\n\r\n创建提供者失败，请检查输入"
		conn.Write([]byte(errorResponse))
		return
	}

	// 成功添加的计数
	successCount := 0

	// 保存到数据库
	for _, provider := range providers {
		// 创建数据库对象
		dbProvider := &schema.AiProvider{
			ModelName:       modelName,
			TypeName:        modelType,
			DomainOrURL:     domainOrURL,
			APIKey:          provider.APIKey,
			WrapperName:     wrapperName, // 使用表单中提供的WrapperName
			IsHealthy:       true,        // 默认设置为健康
			LastRequestTime: time.Now(),  // 设置最后请求时间
			HealthCheckTime: time.Now(),  // 设置健康检查时间
		}

		// 保存到数据库
		err = SaveAiProvider(dbProvider)
		if err != nil {
			c.logError("Failed to save provider: %v", err)
			continue
		}

		// 关联数据库对象到Provider
		provider.DbProvider = dbProvider
		successCount++
	}

	// 构建结果消息
	resultMessage := fmt.Sprintf("成功添加 %d 个提供者(共 %d 个)", successCount, len(providers))
	c.logInfo(resultMessage)

	// 重定向回主页
	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /portal\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

// serveHealthyCheckPage 处理健康检查请求
func (c *ServerConfig) serveHealthyCheckPage(conn net.Conn, request *http.Request) {
	c.logInfo("Serving healthy check page")

	// 检查是否要执行新的健康检查
	query := request.URL.Query()
	runCheck := query.Get("run") == "true"

	var results []*HealthCheckResult
	if runCheck {
		// 如果指定了run=true参数，才执行实时健康检查
		c.logInfo("Running manual health check as requested")
		var err error
		results, err = RunManualHealthCheck()
		if err != nil {
			c.logError("Failed to run health check: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n健康检查失败: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	}

	// 获取所有提供者（获取最新状态）
	providers, err := GetAllAiProviders()
	if err != nil {
		c.logError("Failed to get providers: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n获取提供者信息失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 统计健康提供者数量
	healthyCount := 0
	for _, p := range providers {
		if p.IsHealthy {
			healthyCount++
		}
	}

	// 计算健康率
	totalProviders := len(providers)
	healthRate := 0.0
	if totalProviders > 0 {
		healthRate = float64(healthyCount) * 100 / float64(totalProviders)
	}

	// 准备模板数据
	data := struct {
		Providers      []*schema.AiProvider
		CheckTime      string
		TotalProviders int
		HealthyCount   int
		HealthRate     float64
		CheckResults   []*HealthCheckResult // 添加健康检查结果
		JustRanCheck   bool                 // 是否刚刚执行了检查
	}{
		Providers:      providers,
		CheckTime:      time.Now().Format("2006-01-02 15:04:05"),
		TotalProviders: totalProviders,
		HealthyCount:   healthyCount,
		HealthRate:     healthRate,
		CheckResults:   results,
		JustRanCheck:   runCheck,
	}

	var tmpl *template.Template

	// 尝试从文件系统读取模板
	if result := utils.GetFirstExistedFile(
		"common/aibalance/templates/healthy_check.html",
		"templates/healthy_check.html",
		"../templates/healthy_check.html",
	); result != "" {
		rawTemp, err := os.ReadFile(result)
		if err != nil {
			c.logError("Failed to read healthy check template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板读取失败: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
		tmpl, err = template.New("healthy_check").Parse(string(rawTemp))
		if err != nil {
			c.logError("Failed to parse healthy check template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板解析失败: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	} else {
		// 使用嵌入式文件系统中的模板
		tmpl, err = template.ParseFS(templatesFS, "templates/healthy_check.html")
		if err != nil {
			c.logError("Failed to parse embedded healthy check template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板解析失败: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	}

	// 创建一个缓冲区来保存渲染后的HTML
	var htmlBuffer bytes.Buffer
	err = tmpl.Execute(&htmlBuffer, data)
	if err != nil {
		c.logError("Failed to execute healthy check template: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板渲染失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 准备HTTP响应头
	header := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", htmlBuffer.Len()) + "\r\n" +
		"\r\n"

	// 写入头部和HTML内容
	conn.Write([]byte(header))
	conn.Write(htmlBuffer.Bytes())
}

// handleLogout 处理登出请求
func (c *ServerConfig) handleLogout(conn net.Conn, request *http.Request) {
	// 从Cookie中获取会话ID
	cookies := request.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "admin_session" {
			// 删除会话
			c.SessionManager.DeleteSession(cookie.Value)
			break
		}
	}

	// 清除Cookie并重定向到登录页
	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /portal\r\n" +
		"Set-Cookie: admin_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; HttpOnly; SameSite=Strict\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

// serveAutoCompleteData 提供自动补全数据
func (c *ServerConfig) serveAutoCompleteData(conn net.Conn, request *http.Request) {
	c.logInfo("Serving autocomplete data")

	// 获取所有提供者数据
	providers, err := GetAllAiProviders()
	if err != nil {
		c.logError("Failed to get providers for autocomplete: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n获取提供者信息失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 提取不重复的数据
	wrapperNames := make(map[string]bool)
	modelNames := make(map[string]bool)
	modelTypes := make(map[string]bool)

	for _, p := range providers {
		if p.WrapperName != "" {
			wrapperNames[p.WrapperName] = true
		}
		if p.ModelName != "" {
			modelNames[p.ModelName] = true
		}
		if p.TypeName != "" {
			modelTypes[p.TypeName] = true
		}
	}

	// 转换为数组
	wrapperNamesList := make([]string, 0, len(wrapperNames))
	for name := range wrapperNames {
		wrapperNamesList = append(wrapperNamesList, name)
	}

	modelNamesList := make([]string, 0, len(modelNames))
	for name := range modelNames {
		modelNamesList = append(modelNamesList, name)
	}

	modelTypesList := make([]string, 0, len(modelTypes))
	for typeName := range modelTypes {
		modelTypesList = append(modelTypesList, typeName)
	}

	// 添加一些常见的模型类型，如果数据库中没有的话
	commonTypes := []string{"ChatCompletion", "TextCompletion", "Embedding"}
	for _, typeName := range commonTypes {
		if _, exists := modelTypes[typeName]; !exists {
			modelTypesList = append(modelTypesList, typeName)
		}
	}

	// 构建JSON响应
	autoCompleteData := struct {
		WrapperNames []string `json:"wrapper_names"`
		ModelNames   []string `json:"model_names"`
		ModelTypes   []string `json:"model_types"`
	}{
		WrapperNames: wrapperNamesList,
		ModelNames:   modelNamesList,
		ModelTypes:   modelTypesList,
	}

	// 转换为JSON
	jsonData, err := json.Marshal(autoCompleteData)
	if err != nil {
		c.logError("Failed to encode autocomplete data: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n数据编码失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 准备HTTP响应头
	header := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", len(jsonData)) + "\r\n" +
		"\r\n"

	// 写入头部和JSON内容
	conn.Write([]byte(header))
	conn.Write(jsonData)
}

// serveAPIKeysPage 展示API密钥信息的页面
func (c *ServerConfig) serveAPIKeysPage(conn net.Conn) {
	c.logInfo("Serving API keys page")

	// 准备模板数据
	data := struct {
		CurrentTime  string
		APIKeys      map[string]string
		AllModelList []string // 所有可用的模型列表，用于创建新API密钥
	}{
		CurrentTime: time.Now().Format("2006-01-02 15:04:05"),
		APIKeys:     make(map[string]string),
	}

	// 从数据库获取API密钥
	dbApiKeys, err := GetAllAiApiKeys()
	if err == nil && len(dbApiKeys) > 0 {
		// 数据库中有API密钥，使用数据库记录
		for _, apiKey := range dbApiKeys {
			data.APIKeys[apiKey.APIKey] = apiKey.AllowedModels
		}
	} else {
		// 从内存配置中获取API密钥和允许的模型（用作备选方案）
		for _, key := range c.KeyAllowedModels.Keys() {
			models, _ := c.KeyAllowedModels.Get(key)
			modelNames := make([]string, 0, len(models))
			for model := range models {
				modelNames = append(modelNames, model)
			}
			data.APIKeys[key] = strings.Join(modelNames, ", ")
		}
	}

	// 获取所有可用的模型列表
	providers, err := GetAllAiProviders()
	if err == nil {
		modelSet := make(map[string]bool)
		for _, p := range providers {
			if p.ModelName != "" {
				modelSet[p.ModelName] = true
			}
		}

		data.AllModelList = make([]string, 0, len(modelSet))
		for model := range modelSet {
			data.AllModelList = append(data.AllModelList, model)
		}
	}

	// 渲染模板
	var htmlBuffer bytes.Buffer
	tmpl, err := template.ParseFS(templatesFS, "templates/api_keys.html")
	if err != nil {
		c.logError("Failed to parse api_keys template: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板解析失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	err = tmpl.Execute(&htmlBuffer, data)
	if err != nil {
		c.logError("Failed to execute api_keys template: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n模板渲染失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 准备HTTP响应头
	header := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", htmlBuffer.Len()) + "\r\n" +
		"\r\n"

	// 写入头部和HTML内容
	conn.Write([]byte(header))
	conn.Write(htmlBuffer.Bytes())
}

// processCreateAPIKey 处理创建新API密钥的请求
func (c *ServerConfig) processCreateAPIKey(conn net.Conn, request *http.Request) {
	c.logInfo("Processing create API key request")

	// 解析表单数据
	err := request.ParseForm()
	if err != nil {
		c.logError("Failed to parse form: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 400 Bad Request\r\n\r\n表单解析失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 获取表单数据
	apiKey := request.PostForm.Get("api_key")
	allowedModels := request.PostForm["allowed_models"] // 多选值

	// 验证必填字段
	if apiKey == "" || len(allowedModels) == 0 {
		c.logError("Missing required fields for API key creation")
		errorResponse := "HTTP/1.1 400 Bad Request\r\n\r\nAPI密钥和允许的模型都是必填的"
		conn.Write([]byte(errorResponse))
		return
	}

	// 将允许的模型添加到配置中
	modelMap := make(map[string]bool)
	for _, model := range allowedModels {
		modelMap[model] = true
	}

	// 添加到配置（保持内存配置的兼容性）
	c.KeyAllowedModels.allowedModels[apiKey] = modelMap

	// 将API密钥保存到数据库
	allowedModelsStr := strings.Join(allowedModels, ",")
	err = SaveAiApiKey(apiKey, allowedModelsStr)
	if err != nil {
		c.logError("Failed to save API key to database: %v", err)
		// 继续执行，因为已经添加到内存配置中了
	}

	// 构建结果消息
	c.logInfo("Successfully created API key: %s with %d allowed models", apiKey, len(allowedModels))

	// 重定向回API密钥页面
	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /portal/api-keys\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

// HandlePortalRequest 处理管理门户请求的主入口
func (c *ServerConfig) HandlePortalRequest(conn net.Conn, request *http.Request, uriIns *url.URL) {
	c.logInfo("Processing portal request: %s", uriIns.Path)

	// 处理登录POST请求
	if uriIns.Path == "/portal/login" && request.Method == "POST" {
		c.processLogin(conn, request)
		return
	}

	// 检查认证状态（除了登录页面外）
	if !c.checkAuth(request) {
		c.serveLoginPage(conn)
		return
	}

	// 根据路径处理不同的请求
	if uriIns.Path == "/portal" || uriIns.Path == "/portal/" {
		c.servePortalWithAuth(conn)
	} else if uriIns.Path == "/portal/add-ai-provider" {
		c.serveAddProviderPage(conn, request)
	} else if uriIns.Path == "/portal/add-providers" && request.Method == "POST" {
		c.processAddProviders(conn, request)
	} else if uriIns.Path == "/portal/autocomplete" {
		c.serveAutoCompleteData(conn, request)
	} else if uriIns.Path == "/portal/api-keys" {
		c.serveAPIKeysPage(conn)
	} else if uriIns.Path == "/portal/create-api-key" && request.Method == "POST" {
		c.processCreateAPIKey(conn, request)
	} else if uriIns.Path == "/portal/healthy-check" {
		c.serveHealthyCheckPage(conn, request)
	} else if uriIns.Path == "/portal/single-provider-health-check" {
		c.serveSingleProviderHealthCheckPage(conn, request)
	} else if uriIns.Path == "/portal/delete-providers" && request.Method == "POST" {
		c.processDeleteProviders(conn, request)
	} else if uriIns.Path == "/portal/logout" {
		c.handleLogout(conn, request)
	} else {
		// 默认返回主页
		c.servePortalWithAuth(conn)
	}
}

// serveSingleProviderHealthCheckPage 处理单个提供者的健康检查请求
func (c *ServerConfig) serveSingleProviderHealthCheckPage(conn net.Conn, request *http.Request) {
	c.logInfo("Serving single provider health check page")

	// 解析查询参数获取提供者ID
	query := request.URL.Query()
	providerIDStr := query.Get("id")
	if providerIDStr == "" {
		c.logError("Missing provider ID")
		errorResponse := "HTTP/1.1 400 Bad Request\r\n\r\n缺少提供者ID参数"
		conn.Write([]byte(errorResponse))
		return
	}

	// 将ID转换为数字
	providerIDInt, err := strconv.ParseUint(providerIDStr, 10, 64)
	if err != nil {
		c.logError("Invalid provider ID: %s, %v", providerIDStr, err)
		errorResponse := fmt.Sprintf("HTTP/1.1 400 Bad Request\r\n\r\n无效的提供者ID: %s", providerIDStr)
		conn.Write([]byte(errorResponse))
		return
	}
	providerID := uint(providerIDInt)

	// 执行健康检查
	result, err := RunSingleProviderHealthCheck(providerID)
	if err != nil {
		c.logError("Failed to run health check for provider %d: %v", providerID, err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n健康检查失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 准备JSON响应
	responseData := struct {
		Success      bool   `json:"success"`
		ProviderID   uint   `json:"provider_id"`
		ProviderName string `json:"provider_name"`
		IsHealthy    bool   `json:"is_healthy"`
		ResponseTime int64  `json:"response_time"`
		Message      string `json:"message"`
		CheckTime    string `json:"check_time"`
	}{
		Success:      true,
		ProviderID:   providerID,
		ProviderName: result.Provider.WrapperName,
		IsHealthy:    result.IsHealthy,
		ResponseTime: result.ResponseTime,
		CheckTime:    time.Now().Format("2006-01-02 15:04:05"),
	}

	if result.Error != nil {
		responseData.Message = result.Error.Error()
	} else if result.IsHealthy {
		responseData.Message = "健康检查成功，提供者状态良好"
	} else {
		responseData.Message = "健康检查完成，提供者状态不佳"
	}

	// 转换为JSON
	jsonData, err := json.Marshal(responseData)
	if err != nil {
		c.logError("Failed to encode response data: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n数据编码失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 准备HTTP响应头
	header := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", len(jsonData)) + "\r\n" +
		"\r\n"

	// 写入头部和JSON内容
	conn.Write([]byte(header))
	conn.Write(jsonData)
}

// processDeleteProviders 处理删除AI提供者请求
func (c *ServerConfig) processDeleteProviders(conn net.Conn, request *http.Request) {
	c.logInfo("Processing delete providers request")

	// 限制请求体大小，避免潜在的DOS攻击
	request.Body = http.MaxBytesReader(nil, request.Body, 1024*1024)

	// 解析请求体
	var requestData struct {
		ProviderIDs []uint `json:"provider_ids"`
	}

	err := json.NewDecoder(request.Body).Decode(&requestData)
	if err != nil {
		c.logError("Failed to parse delete providers request: %v", err)
		responseData := map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("解析请求失败: %v", err),
		}
		c.sendJSONResponse(conn, responseData, http.StatusBadRequest)
		return
	}

	// 验证提供者ID
	if len(requestData.ProviderIDs) == 0 {
		c.logError("No provider IDs specified for deletion")
		responseData := map[string]interface{}{
			"success": false,
			"message": "没有指定要删除的提供者",
		}
		c.sendJSONResponse(conn, responseData, http.StatusBadRequest)
		return
	}

	// 记录删除操作
	c.logInfo("Deleting %d providers: %v", len(requestData.ProviderIDs), requestData.ProviderIDs)

	// 执行删除
	var failedIDs []uint
	for _, id := range requestData.ProviderIDs {
		err := DeleteAiProviderByID(id)
		if err != nil {
			c.logError("Failed to delete provider ID %d: %v", id, err)
			failedIDs = append(failedIDs, id)
		}
	}

	// 创建响应
	if len(failedIDs) > 0 {
		responseData := map[string]interface{}{
			"success":    false,
			"message":    fmt.Sprintf("部分提供者删除失败: %v", failedIDs),
			"failed_ids": failedIDs,
		}
		c.sendJSONResponse(conn, responseData, http.StatusInternalServerError)
	} else {
		responseData := map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("成功删除 %d 个提供者", len(requestData.ProviderIDs)),
		}
		c.sendJSONResponse(conn, responseData, http.StatusOK)
	}
}

// sendJSONResponse 发送JSON格式的响应
func (c *ServerConfig) sendJSONResponse(conn net.Conn, data interface{}, statusCode int) {
	// 将数据转换为JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		c.logError("Failed to encode JSON response: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\n数据编码失败: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 准备HTTP状态行
	statusText := http.StatusText(statusCode)
	if statusText == "" {
		statusText = "Unknown"
	}

	// 准备HTTP响应头
	header := fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, statusText) +
		"Content-Type: application/json; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", len(jsonData)) + "\r\n" +
		"\r\n"

	// 写入头部和JSON内容
	conn.Write([]byte(header))
	conn.Write(jsonData)
}
