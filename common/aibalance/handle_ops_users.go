package aibalance

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/crypto/bcrypt"
)

// ==================== SQL Security Helpers ====================

// escapeLikePattern escapes special characters in LIKE patterns to prevent SQL injection
// This function escapes %, _, and \ characters which have special meaning in SQL LIKE clauses
func escapeLikePattern(input string) string {
	// Escape backslash first, then other special characters
	result := strings.ReplaceAll(input, "\\", "\\\\")
	result = strings.ReplaceAll(result, "%", "\\%")
	result = strings.ReplaceAll(result, "_", "\\_")
	return result
}

// safeLikePattern creates a safe LIKE pattern with wildcards
// It escapes the input and wraps it with % wildcards for partial matching
func safeLikePattern(input string) string {
	escaped := escapeLikePattern(input)
	return "%" + escaped + "%"
}

// ==================== OPS User Database Operations ====================

// SaveOpsUser saves an OpsUser to the database
func SaveOpsUser(user *schema.OpsUser) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	return db.Save(user).Error
}

// GetOpsUserByID retrieves an OpsUser by ID
func GetOpsUserByID(id uint) (*schema.OpsUser, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var user schema.OpsUser
	if err := db.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// GetOpsUserByUsername retrieves an OpsUser by username
func GetOpsUserByUsername(username string) (*schema.OpsUser, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var user schema.OpsUser
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// GetOpsUserByOpsKey retrieves an OpsUser by OpsKey
func GetOpsUserByOpsKey(opsKey string) (*schema.OpsUser, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var user schema.OpsUser
	if err := db.Where("ops_key = ?", opsKey).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// GetAllOpsUsers retrieves all OpsUsers
func GetAllOpsUsers() ([]*schema.OpsUser, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var users []*schema.OpsUser
	if err := db.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// DeleteOpsUser deletes an OpsUser by ID
func DeleteOpsUser(id uint) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	return db.Delete(&schema.OpsUser{}, id).Error
}

// ==================== Password Utilities ====================

// GenerateRandomPassword generates a random password with given length
// Format: ops-{random12chars}
func GenerateRandomPassword() string {
	return "ops-" + utils.RandStringBytes(12)
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword compares a hashed password with a plain password
func CheckPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// GenerateOpsKey generates a new OPS key
// Format: ops-{uuid}
func GenerateOpsKey() string {
	return "ops-" + uuid.New().String()
}

// ==================== HTTP Handlers ====================

// handleListOpsUsers handles GET /portal/api/ops-users
// Returns list of all OPS users (admin only)
func (c *ServerConfig) handleListOpsUsers(conn net.Conn, request *http.Request) {
	c.logInfo("Handling list OPS users request")

	// Auth check is done by middleware, but double-check for safety
	authInfo := c.getAuthInfo(request)
	if !authInfo.IsAdmin() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "Admin access required",
		})
		return
	}

	users, err := GetAllOpsUsers()
	if err != nil {
		c.logError("Failed to get OPS users: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve users",
		})
		return
	}

	// Convert to response format (hide password)
	type UserResponse struct {
		ID           uint   `json:"id"`
		Username     string `json:"username"`
		Role         string `json:"role"`
		Active       bool   `json:"active"`
		OpsKey       string `json:"ops_key"`
		DefaultLimit int64  `json:"default_limit"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
	}

	var response []UserResponse
	for _, u := range users {
		response = append(response, UserResponse{
			ID:           u.ID,
			Username:     u.Username,
			Role:         u.Role,
			Active:       u.Active,
			OpsKey:       u.OpsKey,
			DefaultLimit: u.DefaultLimit,
			CreatedAt:    u.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:    u.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"users":   response,
		"total":   len(response),
	})
}

// handleCreateOpsUser handles POST /portal/api/ops-users
// Creates a new OPS user (admin only)
func (c *ServerConfig) handleCreateOpsUser(conn net.Conn, request *http.Request) {
	c.logInfo("Handling create OPS user request")

	// Auth check
	authInfo := c.getAuthInfo(request)
	if !authInfo.IsAdmin() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "Admin access required",
		})
		return
	}

	// Parse request body
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()

	var reqBody struct {
		Username     string `json:"username"`
		DefaultLimit int64  `json:"default_limit"`
	}

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
		return
	}

	// Validate username
	if reqBody.Username == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Username is required",
		})
		return
	}

	// Check if username already exists
	existingUser, _ := GetOpsUserByUsername(reqBody.Username)
	if existingUser != nil {
		c.writeJSONResponse(conn, http.StatusConflict, map[string]string{
			"error": "Username already exists",
		})
		return
	}

	// Generate password and OPS key
	plainPassword := GenerateRandomPassword()
	hashedPassword, err := HashPassword(plainPassword)
	if err != nil {
		c.logError("Failed to hash password: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to create user",
		})
		return
	}

	opsKey := GenerateOpsKey()

	// Set default limit (50MB if not specified)
	defaultLimit := reqBody.DefaultLimit
	if defaultLimit <= 0 {
		defaultLimit = 52428800 // 50MB
	}

	// Create user
	user := &schema.OpsUser{
		Username:     reqBody.Username,
		Password:     hashedPassword,
		OpsKey:       opsKey,
		Role:         "ops",
		Active:       true,
		DefaultLimit: defaultLimit,
	}

	if err := SaveOpsUser(user); err != nil {
		c.logError("Failed to save OPS user: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to create user",
		})
		return
	}

	log.Infof("Created OPS user: %s (ID: %d)", user.Username, user.ID)

	// Return user info with plain password (only shown once)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":  true,
		"message":  "User created successfully",
		"user_id":  user.ID,
		"username": user.Username,
		"password": plainPassword, // Show only once
		"ops_key":  opsKey,
	})
}

// handleDeleteOpsUser handles DELETE /portal/api/ops-users/{id}
// Deletes an OPS user (admin only)
func (c *ServerConfig) handleDeleteOpsUser(conn net.Conn, request *http.Request, path string) {
	c.logInfo("Handling delete OPS user request: %s", path)

	// Auth check
	authInfo := c.getAuthInfo(request)
	if !authInfo.IsAdmin() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "Admin access required",
		})
		return
	}

	// Extract user ID from path
	// Path format: /portal/api/ops-users/{id}
	parts := strings.Split(path, "/")
	if len(parts) < 5 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid path format",
		})
		return
	}

	idStr := parts[len(parts)-1]
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid user ID",
		})
		return
	}

	// Check if user exists
	user, err := GetOpsUserByID(uint(id))
	if err != nil {
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
		return
	}

	// Delete user
	if err := DeleteOpsUser(uint(id)); err != nil {
		c.logError("Failed to delete OPS user: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to delete user",
		})
		return
	}

	log.Infof("Deleted OPS user: %s (ID: %d)", user.Username, user.ID)

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("User %s deleted successfully", user.Username),
	})
}

// handleUpdateOpsUser handles PUT /portal/api/ops-users/{id}
// Updates an OPS user (admin only)
func (c *ServerConfig) handleUpdateOpsUser(conn net.Conn, request *http.Request, path string) {
	c.logInfo("Handling update OPS user request: %s", path)

	// Auth check
	authInfo := c.getAuthInfo(request)
	if !authInfo.IsAdmin() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "Admin access required",
		})
		return
	}

	// Extract user ID from path
	parts := strings.Split(path, "/")
	if len(parts) < 5 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid path format",
		})
		return
	}

	idStr := parts[len(parts)-1]
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid user ID",
		})
		return
	}

	// Get existing user
	user, err := GetOpsUserByID(uint(id))
	if err != nil {
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
		return
	}

	// Parse request body
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()

	var reqBody struct {
		Active       *bool  `json:"active"`
		DefaultLimit *int64 `json:"default_limit"`
	}

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
		return
	}

	// Update fields
	if reqBody.Active != nil {
		user.Active = *reqBody.Active
	}
	if reqBody.DefaultLimit != nil {
		user.DefaultLimit = *reqBody.DefaultLimit
	}

	if err := SaveOpsUser(user); err != nil {
		c.logError("Failed to update OPS user: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to update user",
		})
		return
	}

	log.Infof("Updated OPS user: %s (ID: %d)", user.Username, user.ID)

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "User updated successfully",
	})
}

// handleResetOpsUserPassword handles POST /portal/api/ops-users/{id}/reset-password
// Resets an OPS user's password (admin only)
func (c *ServerConfig) handleResetOpsUserPassword(conn net.Conn, request *http.Request, path string) {
	c.logInfo("Handling reset OPS user password request: %s", path)

	// Auth check
	authInfo := c.getAuthInfo(request)
	if !authInfo.IsAdmin() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "Admin access required",
		})
		return
	}

	// Extract user ID from path
	// Path format: /portal/api/ops-users/{id}/reset-password
	parts := strings.Split(path, "/")
	if len(parts) < 6 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid path format",
		})
		return
	}

	idStr := parts[len(parts)-2] // ID is second to last
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid user ID",
		})
		return
	}

	// Get existing user
	user, err := GetOpsUserByID(uint(id))
	if err != nil {
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
		return
	}

	// Generate new password
	plainPassword := GenerateRandomPassword()
	hashedPassword, err := HashPassword(plainPassword)
	if err != nil {
		c.logError("Failed to hash password: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to reset password",
		})
		return
	}

	user.Password = hashedPassword
	if err := SaveOpsUser(user); err != nil {
		c.logError("Failed to save OPS user: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to reset password",
		})
		return
	}

	log.Infof("Reset password for OPS user: %s (ID: %d)", user.Username, user.ID)

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":      true,
		"message":      "Password reset successfully",
		"new_password": plainPassword, // Show only once
	})
}

// handleResetOpsUserKey handles POST /portal/api/ops-users/{id}/reset-key
// Resets an OPS user's OPS key (admin only)
func (c *ServerConfig) handleResetOpsUserKey(conn net.Conn, request *http.Request, path string) {
	c.logInfo("Handling reset OPS user key request: %s", path)

	// Auth check
	authInfo := c.getAuthInfo(request)
	if !authInfo.IsAdmin() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "Admin access required",
		})
		return
	}

	// Extract user ID from path
	parts := strings.Split(path, "/")
	if len(parts) < 6 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid path format",
		})
		return
	}

	idStr := parts[len(parts)-2]
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid user ID",
		})
		return
	}

	// Get existing user
	user, err := GetOpsUserByID(uint(id))
	if err != nil {
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
		return
	}

	// Generate new OPS key
	newOpsKey := GenerateOpsKey()
	user.OpsKey = newOpsKey

	if err := SaveOpsUser(user); err != nil {
		c.logError("Failed to save OPS user: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to reset OPS key",
		})
		return
	}

	log.Infof("Reset OPS key for user: %s (ID: %d)", user.Username, user.ID)

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":     true,
		"message":     "OPS key reset successfully",
		"new_ops_key": newOpsKey,
	})
}

// ==================== OPS Self-Service Handlers ====================

// handleOpsChangePassword handles POST /ops/change-password
// Allows OPS user to change their own password
func (c *ServerConfig) handleOpsChangePassword(conn net.Conn, request *http.Request) {
	c.logInfo("Handling OPS change password request")

	// Auth check - must be authenticated OPS user
	authInfo := c.getAuthInfo(request)
	if !authInfo.Authenticated {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
		return
	}

	// Parse request body
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()

	var reqBody struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
		return
	}

	// Validate input
	if reqBody.OldPassword == "" || reqBody.NewPassword == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Both old_password and new_password are required",
		})
		return
	}

	if len(reqBody.NewPassword) < 8 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "New password must be at least 8 characters",
		})
		return
	}

	// Get user
	user, err := GetOpsUserByID(authInfo.UserID)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
		return
	}

	// Verify old password
	if !CheckPassword(user.Password, reqBody.OldPassword) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{
			"error": "Old password is incorrect",
		})
		return
	}

	// Hash and save new password
	hashedPassword, err := HashPassword(reqBody.NewPassword)
	if err != nil {
		c.logError("Failed to hash password: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to change password",
		})
		return
	}

	user.Password = hashedPassword
	if err := SaveOpsUser(user); err != nil {
		c.logError("Failed to save OPS user: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to change password",
		})
		return
	}

	log.Infof("OPS user %s changed their password", user.Username)

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Password changed successfully",
	})
}

// handleOpsResetOwnKey handles POST /ops/reset-key
// Allows OPS user to reset their own OPS key
func (c *ServerConfig) handleOpsResetOwnKey(conn net.Conn, request *http.Request) {
	c.logInfo("Handling OPS reset own key request")

	// Auth check - must be OPS user
	authInfo := c.getAuthInfo(request)
	if !authInfo.IsOps() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "OPS user access required",
		})
		return
	}

	// Get user
	user, err := GetOpsUserByID(authInfo.UserID)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
		return
	}

	// Generate new OPS key
	newOpsKey := GenerateOpsKey()
	user.OpsKey = newOpsKey

	if err := SaveOpsUser(user); err != nil {
		c.logError("Failed to save OPS user: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to reset OPS key",
		})
		return
	}

	log.Infof("OPS user %s reset their own OPS key", user.Username)

	// Log this action
	LogOpsAction(user.ID, user.Username, "reset_ops_key", "ops_user", fmt.Sprintf("%d", user.ID), "", request)

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":     true,
		"message":     "OPS key reset successfully",
		"new_ops_key": newOpsKey,
	})
}

// handleOpsGetMyInfo handles GET /ops/my-info
// Returns current OPS user's information
func (c *ServerConfig) handleOpsGetMyInfo(conn net.Conn, request *http.Request) {
	c.logInfo("Handling OPS get my info request")

	// Auth check - must be OPS user
	authInfo := c.getAuthInfo(request)
	if !authInfo.IsOps() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "OPS user access required",
		})
		return
	}

	// Get user
	user, err := GetOpsUserByID(authInfo.UserID)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
		return
	}

	// Count API keys created by this user
	var apiKeyCount int64
	GetDB().Model(&schema.AiApiKeys{}).Where("created_by_ops_id = ?", user.ID).Count(&apiKeyCount)

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":        true,
		"user_id":        user.ID,
		"username":       user.Username,
		"role":           user.Role,
		"active":         user.Active,
		"ops_key":        user.OpsKey,
		"default_limit":  user.DefaultLimit,
		"api_keys_count": apiKeyCount,
		"created_at":     user.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}

// ==================== Action Logging ====================

// LogOpsAction logs an OPS user action
func LogOpsAction(operatorID uint, operatorName, action, targetType, targetID, detail string, request *http.Request) {
	db := GetDB()
	if db == nil {
		log.Errorf("Cannot log OPS action: database not initialized")
		return
	}

	// Get client IP
	ipAddress := ""
	if request != nil {
		ipAddress = request.RemoteAddr
		// Try to get real IP from X-Forwarded-For header
		if xff := request.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				ipAddress = strings.TrimSpace(parts[0])
			}
		}
	}

	logEntry := &schema.OpsActionLog{
		OperatorID:   operatorID,
		OperatorName: operatorName,
		Action:       action,
		TargetType:   targetType,
		TargetID:     targetID,
		Detail:       detail,
		IPAddress:    ipAddress,
	}

	if err := db.Create(logEntry).Error; err != nil {
		log.Errorf("Failed to log OPS action: %v", err)
	} else {
		log.Debugf("Logged OPS action: user=%s, action=%s, target=%s/%s",
			operatorName, action, targetType, targetID)
	}
}

// ==================== OPS API Key Creation ====================

// handleOpsCreateApiKey handles POST /ops/create-api-key or /ops/api/create-api-key
// Allows OPS user to create API keys with traffic limit enforced
func (c *ServerConfig) handleOpsCreateApiKey(conn net.Conn, request *http.Request, authInfo *AuthInfo) {
	c.logInfo("Handling OPS create API key request")

	// Auth check - must be OPS user
	if !authInfo.IsOps() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "OPS user access required",
		})
		return
	}

	// Get OPS user to check default limit
	user, err := GetOpsUserByID(authInfo.UserID)
	if err != nil {
		c.logError("Failed to get OPS user info: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to get user info",
		})
		return
	}

	// Parse request body
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()

	var reqBody struct {
		AllowedModels []string `json:"allowed_models"`
		TrafficLimit  int64    `json:"traffic_limit"` // Optional, 0 or negative means unlimited
		Unlimited     bool     `json:"unlimited"`     // Explicitly set unlimited traffic
	}

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
		return
	}

	// Validate allowed models
	if len(reqBody.AllowedModels) == 0 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "At least one allowed model must be specified",
		})
		return
	}

	// Determine traffic limit settings
	// OPS users can now create unlimited API keys
	var trafficLimit int64 = 0
	var trafficLimitEnable bool = false

	if reqBody.Unlimited {
		// Explicitly unlimited
		trafficLimit = 0
		trafficLimitEnable = false
	} else if reqBody.TrafficLimit > 0 {
		// Use specified traffic limit
		trafficLimit = reqBody.TrafficLimit
		trafficLimitEnable = true
	} else {
		// Default: unlimited (no traffic restriction)
		trafficLimit = 0
		trafficLimitEnable = false
	}

	// Generate API key
	apiKey := "mf-" + uuid.New().String()

	// Sort allowed models for consistent storage
	sort.Strings(reqBody.AllowedModels)
	
	// Create API key record
	allowedModelsStr := strings.Join(reqBody.AllowedModels, ",")
	apiKeyRecord := &schema.AiApiKeys{
		APIKey:             apiKey,
		AllowedModels:      allowedModelsStr,
		Active:             true,
		TrafficLimitEnable: trafficLimitEnable,
		TrafficLimit:       trafficLimit,
		TrafficUsed:        0,
		CreatedByOpsID:     user.ID,
		CreatedByOpsName:   user.Username,
	}

	db := GetDB()
	if db == nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Database not initialized",
		})
		return
	}

	if err := db.Create(apiKeyRecord).Error; err != nil {
		c.logError("Failed to create API key: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to create API key",
		})
		return
	}

	if trafficLimitEnable {
		log.Infof("OPS user %s created API key: %s with traffic limit: %d bytes",
			user.Username, apiKey[:20]+"...", trafficLimit)
	} else {
		log.Infof("OPS user %s created API key: %s with unlimited traffic",
			user.Username, apiKey[:20]+"...")
	}

	// Log the action
	detailJSON, _ := json.Marshal(map[string]interface{}{
		"allowed_models":       reqBody.AllowedModels,
		"traffic_limit":        trafficLimit,
		"traffic_limit_enable": trafficLimitEnable,
		"unlimited":            !trafficLimitEnable,
	})
	LogOpsAction(user.ID, user.Username, "create_api_key", "api_key", fmt.Sprintf("%d", apiKeyRecord.ID), string(detailJSON), request)

	// Reload API keys to update in-memory cache
	if err := c.LoadAPIKeysFromDB(); err != nil {
		c.logError("Failed to reload API keys: %v", err)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":              true,
		"api_key":              apiKey,
		"allowed_models":       reqBody.AllowedModels,
		"traffic_limit":        trafficLimit,
		"traffic_limit_enable": trafficLimitEnable,
		"unlimited":            !trafficLimitEnable,
		"message":              "API key created successfully",
	})
}

// handleOpsGetMyKeys handles GET /ops/api/my-keys
// Returns all API keys created by the current OPS user
func (c *ServerConfig) handleOpsGetMyKeys(conn net.Conn, request *http.Request, authInfo *AuthInfo) {
	c.logInfo("Handling OPS get my keys request")

	// Auth check - must be OPS user
	if !authInfo.IsOps() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "OPS user access required",
		})
		return
	}

	db := GetDB()
	if db == nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Database not initialized",
		})
		return
	}

	// Get all API keys created by this OPS user
	var apiKeys []schema.AiApiKeys
	if err := db.Where("created_by_ops_id = ?", authInfo.UserID).Order("created_at DESC").Find(&apiKeys).Error; err != nil {
		c.logError("Failed to get OPS user API keys: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve API keys",
		})
		return
	}

	// Format response
	keys := make([]map[string]interface{}, 0, len(apiKeys))
	for _, key := range apiKeys {
		// Sort allowed models for consistent display
		models := strings.Split(key.AllowedModels, ",")
		sort.Strings(models)
		
		keys = append(keys, map[string]interface{}{
			"id":                   key.ID,
			"api_key":              key.APIKey,
			"allowed_models":       models,
			"traffic_used":         key.TrafficUsed,
			"traffic_limit":        key.TrafficLimit,
			"traffic_limit_enable": key.TrafficLimitEnable,
			"active":               key.Active,
			"created_at":           key.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"keys":    keys,
		"total":   len(keys),
	})
}

// handleOpsDeleteApiKey handles POST /ops/api/delete-api-key
// Allows OPS user to delete their own API keys
func (c *ServerConfig) handleOpsDeleteApiKey(conn net.Conn, request *http.Request, authInfo *AuthInfo) {
	c.logInfo("Handling OPS delete API key request")

	// Auth check - must be OPS user
	if !authInfo.IsOps() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "OPS user access required",
		})
		return
	}

	// Parse request body
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()

	var reqBody struct {
		ApiKey string `json:"api_key"`
	}

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
		return
	}

	if reqBody.ApiKey == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "API key is required",
		})
		return
	}

	db := GetDB()
	if db == nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Database not initialized",
		})
		return
	}

	// Find the API key and verify ownership
	var apiKey schema.AiApiKeys
	if err := db.Where("api_key = ?", reqBody.ApiKey).First(&apiKey).Error; err != nil {
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{
			"error": "API key not found",
		})
		return
	}

	// Verify this key was created by the current OPS user
	if apiKey.CreatedByOpsID != authInfo.UserID {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "You can only delete API keys you created",
		})
		return
	}

	// Delete the API key
	if err := db.Delete(&apiKey).Error; err != nil {
		c.logError("Failed to delete API key: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to delete API key",
		})
		return
	}

	// Log the action
	LogOpsAction(authInfo.UserID, authInfo.Username, "delete_api_key", "api_key", fmt.Sprintf("%d", apiKey.ID), "", request)

	// Reload API keys to update in-memory cache
	if err := c.LoadAPIKeysFromDB(); err != nil {
		c.logError("Failed to reload API keys: %v", err)
	}

	log.Infof("OPS user %s deleted API key: %s", authInfo.Username, reqBody.ApiKey[:20]+"...")

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "API key deleted successfully",
	})
}

// handleOpsUpdateApiKey handles POST /ops/api/update-api-key
// Allows OPS user to update their own API keys (allowed models, traffic settings)
func (c *ServerConfig) handleOpsUpdateApiKey(conn net.Conn, request *http.Request, authInfo *AuthInfo) {
	c.logInfo("Handling OPS update API key request")

	// Auth check - must be OPS user
	if !authInfo.IsOps() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "OPS user access required",
		})
		return
	}

	// Parse request body
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()

	var reqBody struct {
		ApiKey        string   `json:"api_key"`
		AllowedModels []string `json:"allowed_models"`
		TrafficLimit  int64    `json:"traffic_limit"`
		Unlimited     bool     `json:"unlimited"`
	}

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
		return
	}

	if reqBody.ApiKey == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "API key is required",
		})
		return
	}

	db := GetDB()
	if db == nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Database not initialized",
		})
		return
	}

	// Find the API key and verify ownership
	var apiKey schema.AiApiKeys
	if err := db.Where("api_key = ?", reqBody.ApiKey).First(&apiKey).Error; err != nil {
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{
			"error": "API key not found",
		})
		return
	}

	// Verify this key was created by the current OPS user
	if apiKey.CreatedByOpsID != authInfo.UserID {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "You can only update API keys you created",
		})
		return
	}

	// Update allowed models if provided
	if len(reqBody.AllowedModels) > 0 {
		// Sort models for consistent ordering
		sort.Strings(reqBody.AllowedModels)
		apiKey.AllowedModels = strings.Join(reqBody.AllowedModels, ",")
	}

	// Update traffic settings
	if reqBody.Unlimited {
		apiKey.TrafficLimitEnable = false
		apiKey.TrafficLimit = 0
	} else if reqBody.TrafficLimit > 0 {
		apiKey.TrafficLimitEnable = true
		apiKey.TrafficLimit = reqBody.TrafficLimit
	}

	// Save changes
	if err := db.Save(&apiKey).Error; err != nil {
		c.logError("Failed to update API key: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to update API key",
		})
		return
	}

	// Log the action
	detailJSON, _ := json.Marshal(map[string]interface{}{
		"allowed_models":       reqBody.AllowedModels,
		"traffic_limit":        apiKey.TrafficLimit,
		"traffic_limit_enable": apiKey.TrafficLimitEnable,
	})
	LogOpsAction(authInfo.UserID, authInfo.Username, "update_api_key", "api_key", fmt.Sprintf("%d", apiKey.ID), string(detailJSON), request)

	// Reload API keys to update in-memory cache
	if err := c.LoadAPIKeysFromDB(); err != nil {
		c.logError("Failed to reload API keys: %v", err)
	}

	log.Infof("OPS user %s updated API key: %s", authInfo.Username, reqBody.ApiKey[:20]+"...")

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":              true,
		"message":              "API key updated successfully",
		"traffic_limit":        apiKey.TrafficLimit,
		"traffic_limit_enable": apiKey.TrafficLimitEnable,
		"allowed_models":       reqBody.AllowedModels,
	})
}

// handleOpsResetApiKeyTraffic handles POST /ops/api/reset-traffic
// Allows OPS user to reset traffic counter for their own API keys
func (c *ServerConfig) handleOpsResetApiKeyTraffic(conn net.Conn, request *http.Request, authInfo *AuthInfo) {
	c.logInfo("Handling OPS reset API key traffic request")

	// Auth check - must be OPS user
	if !authInfo.IsOps() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "OPS user access required",
		})
		return
	}

	// Parse request body
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()

	var reqBody struct {
		ApiKey string `json:"api_key"`
	}

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
		return
	}

	if reqBody.ApiKey == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "API key is required",
		})
		return
	}

	db := GetDB()
	if db == nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Database not initialized",
		})
		return
	}

	// Find the API key and verify ownership
	var apiKey schema.AiApiKeys
	if err := db.Where("api_key = ?", reqBody.ApiKey).First(&apiKey).Error; err != nil {
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{
			"error": "API key not found",
		})
		return
	}

	// Verify this key was created by the current OPS user
	if apiKey.CreatedByOpsID != authInfo.UserID {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "You can only reset traffic for API keys you created",
		})
		return
	}

	// Reset traffic
	apiKey.TrafficUsed = 0
	if err := db.Save(&apiKey).Error; err != nil {
		c.logError("Failed to reset API key traffic: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to reset traffic",
		})
		return
	}

	// Log the action
	LogOpsAction(authInfo.UserID, authInfo.Username, "reset_api_key_traffic", "api_key", fmt.Sprintf("%d", apiKey.ID), "", request)

	log.Infof("OPS user %s reset traffic for API key: %s", authInfo.Username, reqBody.ApiKey[:20]+"...")

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Traffic reset successfully",
	})
}

// ==================== OPS Logs and Stats ====================

// handleGetOpsLogs handles GET /portal/api/ops-logs
// Returns operation logs (admin only)
func (c *ServerConfig) handleGetOpsLogs(conn net.Conn, request *http.Request) {
	c.logInfo("Handling get OPS logs request")

	// Auth check - admin only
	authInfo := c.getAuthInfo(request)
	if !authInfo.IsAdmin() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "Admin access required",
		})
		return
	}

	// Parse query parameters
	query := request.URL.Query()
	pageStr := query.Get("page")
	pageSizeStr := query.Get("page_size")
	operatorName := query.Get("operator_name")
	action := query.Get("action")

	page := 1
	pageSize := 50
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	db := GetDB()
	if db == nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Database not initialized",
		})
		return
	}

	// Build query with safe LIKE pattern to prevent SQL injection
	dbQuery := db.Model(&schema.OpsActionLog{})
	if operatorName != "" {
		// Use safeLikePattern to escape special characters in LIKE queries
		dbQuery = dbQuery.Where("operator_name LIKE ?", safeLikePattern(operatorName))
	}
	if action != "" {
		dbQuery = dbQuery.Where("action = ?", action)
	}

	// Count total
	var total int64
	dbQuery.Count(&total)

	// Get logs with pagination
	var logs []schema.OpsActionLog
	offset := (page - 1) * pageSize
	if err := dbQuery.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		c.logError("Failed to get OPS logs: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve logs",
		})
		return
	}

	// Format response
	type LogResponse struct {
		ID           uint   `json:"id"`
		OperatorID   uint   `json:"operator_id"`
		OperatorName string `json:"operator_name"`
		Action       string `json:"action"`
		TargetType   string `json:"target_type"`
		TargetID     string `json:"target_id"`
		Detail       string `json:"detail"`
		IPAddress    string `json:"ip_address"`
		CreatedAt    string `json:"created_at"`
	}

	var response []LogResponse
	for _, l := range logs {
		response = append(response, LogResponse{
			ID:           l.ID,
			OperatorID:   l.OperatorID,
			OperatorName: l.OperatorName,
			Action:       l.Action,
			TargetType:   l.TargetType,
			TargetID:     l.TargetID,
			Detail:       l.Detail,
			IPAddress:    l.IPAddress,
			CreatedAt:    l.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":   true,
		"logs":      response,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// handleGetOpsStats handles GET /portal/api/ops-stats
// Returns statistics about OPS users and their created API keys (admin only)
func (c *ServerConfig) handleGetOpsStats(conn net.Conn, request *http.Request) {
	c.logInfo("Handling get OPS stats request")

	// Auth check - admin only
	authInfo := c.getAuthInfo(request)
	if !authInfo.IsAdmin() {
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error": "Admin access required",
		})
		return
	}

	db := GetDB()
	if db == nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Database not initialized",
		})
		return
	}

	// Get all OPS users
	users, err := GetAllOpsUsers()
	if err != nil {
		c.logError("Failed to get OPS users: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve users",
		})
		return
	}

	type UserStats struct {
		UserID          uint   `json:"user_id"`
		Username        string `json:"username"`
		Active          bool   `json:"active"`
		ApiKeysCreated  int64  `json:"api_keys_created"`
		TotalTrafficUsed int64  `json:"total_traffic_used"`
		LastActivity    string `json:"last_activity"`
	}

	var stats []UserStats
	var totalApiKeys int64
	var totalOpsUsers int64

	for _, u := range users {
		totalOpsUsers++

		// Count API keys created by this user
		var keyCount int64
		db.Model(&schema.AiApiKeys{}).Where("created_by_ops_id = ?", u.ID).Count(&keyCount)
		totalApiKeys += keyCount

		// Calculate total traffic used by keys created by this user
		var trafficSum struct {
			Total int64
		}
		db.Model(&schema.AiApiKeys{}).
			Where("created_by_ops_id = ?", u.ID).
			Select("COALESCE(SUM(traffic_used), 0) as total").
			Scan(&trafficSum)

		// Get last activity from logs
		var lastLog schema.OpsActionLog
		lastActivity := ""
		if err := db.Where("operator_id = ?", u.ID).Order("created_at DESC").First(&lastLog).Error; err == nil {
			lastActivity = lastLog.CreatedAt.Format("2006-01-02 15:04:05")
		}

		stats = append(stats, UserStats{
			UserID:          u.ID,
			Username:        u.Username,
			Active:          u.Active,
			ApiKeysCreated:  keyCount,
			TotalTrafficUsed: trafficSum.Total,
			LastActivity:    lastActivity,
		})
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":         true,
		"total_ops_users": totalOpsUsers,
		"total_api_keys":  totalApiKeys,
		"user_stats":      stats,
	})
}
