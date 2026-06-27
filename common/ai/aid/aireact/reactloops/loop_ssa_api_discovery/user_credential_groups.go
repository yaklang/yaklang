package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// UserCredentialAccount is one username/password pair within a group.
type UserCredentialAccount struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Label    string `json:"label,omitempty"`
}

// UserCredentialGroup is a named set of accounts for one auth surface (admin/web/api/...).
type UserCredentialGroup struct {
	GroupID  string                  `json:"group_id"`
	Label    string                  `json:"label,omitempty"`
	Accounts []UserCredentialAccount `json:"accounts"`
}

var (
	reRealmAuthLine = regexp.MustCompile(`(?im)^\s*(admin|user|web|api|oauth|member)[-_]?auth\s*[:：=]\s*(.+?)\s*$`)
	reAuthGroupLine = regexp.MustCompile(`(?im)^\s*auth[-_]?group\s*[:：=]\s*(\w+)\s*[:：=]\s*(.+?)\s*$`)
)

// parseCredentialGroupsFromUserText extracts multi-group credentials from labeled lines.
// Supported forms:
//   - admin_auth: admin1/pass1, admin2/pass2
//   - user_auth: user1/test123; user2/test456
//   - auth_group: admin: admin1/pass1, admin2/pass2
//   - auth_group=web: user1/pass1
func parseCredentialGroupsFromUserText(userText string) []UserCredentialGroup {
	userText = normalizeMarkdownEscapesInUserInput(userText)
	byID := map[string]*UserCredentialGroup{}
	addAccounts := func(groupID, rawValue string) {
		groupID = normalizeCredentialGroupID(groupID)
		if groupID == "" {
			return
		}
		accs := parseAuthAccountList(rawValue)
		if len(accs) == 0 {
			return
		}
		g, ok := byID[groupID]
		if !ok {
			g = &UserCredentialGroup{GroupID: groupID, Label: credentialGroupLabel(groupID)}
			byID[groupID] = g
		}
		g.Accounts = appendUniqueAccounts(g.Accounts, accs...)
	}
	for _, line := range strings.Split(userText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := reRealmAuthLine.FindStringSubmatch(line); len(m) > 2 {
			addAccounts(m[1], m[2])
			continue
		}
		if m := reAuthGroupLine.FindStringSubmatch(line); len(m) > 2 {
			addAccounts(m[1], m[2])
		}
	}
	if len(byID) == 0 {
		return nil
	}
	order := []string{"admin", "user", "web", "api", "oauth", "member", "default"}
	var out []UserCredentialGroup
	seen := map[string]struct{}{}
	for _, id := range order {
		if g, ok := byID[id]; ok {
			out = append(out, *g)
			seen[id] = struct{}{}
		}
	}
	for id, g := range byID {
		if _, ok := seen[id]; ok {
			continue
		}
		out = append(out, *g)
	}
	return out
}

func normalizeCredentialGroupID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.TrimSuffix(id, "_auth")
	id = strings.TrimSuffix(id, "-auth")
	switch id {
	case "", "auth", "cred", "credentials":
		return "default"
	default:
		return id
	}
}

func credentialGroupLabel(groupID string) string {
	switch groupID {
	case "admin":
		return "Admin / backend login accounts"
	case "user", "web":
		return "Front-end / member login accounts"
	case "api":
		return "API / app-token login accounts"
	case "oauth":
		return "OAuth client accounts"
	case "member":
		return "Member portal accounts"
	case "default":
		return "Default login accounts"
	default:
		return groupID
	}
}

var reUserPass = regexp.MustCompile(`(?i)^\s*(\S+?)\s*[:/]\s*(\S+)\s*$`)

func parseAuthCredentials(raw string) (username, password string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	parts := strings.SplitN(raw, "/", 2)
	if len(parts) == 2 {
		u, p := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if u != "" && p != "" {
			return u, p
		}
	}
	if m := reUserPass.FindStringSubmatch(raw); len(m) > 2 {
		return strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	}
	return "", ""
}

func parseAuthAccountList(raw string) []UserCredentialAccount {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var chunks []string
	for _, sep := range []string{";", ",", "|"} {
		if strings.Contains(raw, sep) {
			chunks = strings.Split(raw, sep)
			break
		}
	}
	if len(chunks) == 0 {
		chunks = []string{raw}
	}
	var out []UserCredentialAccount
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		u, p := parseAuthCredentials(chunk)
		if u == "" && p == "" {
			continue
		}
		out = append(out, UserCredentialAccount{Username: u, Password: p})
	}
	return out
}

func appendUniqueAccounts(base []UserCredentialAccount, more ...UserCredentialAccount) []UserCredentialAccount {
	seen := map[string]struct{}{}
	for _, a := range base {
		seen[accountKey(a)] = struct{}{}
	}
	for _, a := range more {
		k := accountKey(a)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		base = append(base, a)
	}
	return base
}

func accountKey(a UserCredentialAccount) string {
	return strings.ToLower(strings.TrimSpace(a.Username)) + "\x00" + strings.TrimSpace(a.Password)
}

func mergeCredentialGroups(base, overlay []UserCredentialGroup) []UserCredentialGroup {
	if len(base) == 0 {
		return overlay
	}
	if len(overlay) == 0 {
		return base
	}
	byID := map[string]*UserCredentialGroup{}
	for i := range base {
		g := base[i]
		cp := g
		byID[g.GroupID] = &cp
	}
	for _, g := range overlay {
		if existing, ok := byID[g.GroupID]; ok {
			existing.Accounts = appendUniqueAccounts(existing.Accounts, g.Accounts...)
			continue
		}
		cp := g
		byID[g.GroupID] = &cp
	}
	return flattenCredentialGroups(byID)
}

func flattenCredentialGroups(byID map[string]*UserCredentialGroup) []UserCredentialGroup {
	order := []string{"admin", "user", "web", "api", "oauth", "member", "default"}
	var out []UserCredentialGroup
	seen := map[string]struct{}{}
	for _, id := range order {
		if g, ok := byID[id]; ok && len(g.Accounts) > 0 {
			out = append(out, *g)
			seen[id] = struct{}{}
		}
	}
	for id, g := range byID {
		if _, ok := seen[id]; ok || len(g.Accounts) == 0 {
			continue
		}
		out = append(out, *g)
	}
	return out
}

func ensureDefaultCredentialGroup(parsed *ParsedUserInput) {
	if parsed == nil {
		return
	}
	u, p := resolveLegacyUserCredentials(parsed)
	if u == "" && p == "" {
		return
	}
	for _, g := range parsed.AuthCredentialGroups {
		if g.GroupID == "default" && len(g.Accounts) > 0 {
			return
		}
	}
	legacy := UserCredentialAccount{Username: u, Password: p}
	for _, g := range parsed.AuthCredentialGroups {
		for _, a := range g.Accounts {
			if accountKey(a) == accountKey(legacy) {
				return
			}
		}
	}
	parsed.AuthCredentialGroups = append(parsed.AuthCredentialGroups, UserCredentialGroup{
		GroupID:  "default",
		Label:    credentialGroupLabel("default"),
		Accounts: []UserCredentialAccount{legacy},
	})
}

// resolveLegacyUserCredentials reads auth: / auth-username lines only (not AuthCredentialGroups).
func resolveLegacyUserCredentials(parsed *ParsedUserInput) (username, password string) {
	if parsed == nil {
		return "", ""
	}
	if u := strings.TrimSpace(parsed.AuthUsername); u != "" {
		username = u
	}
	if p := strings.TrimSpace(parsed.AuthPassword); p != "" {
		password = p
	}
	if username != "" && password != "" {
		return username, password
	}
	if line := strings.TrimSpace(parsed.AuthLine); line != "" {
		u, p := parseAuthCredentials(line)
		if username == "" && u != "" {
			username = u
		}
		if password == "" && p != "" {
			password = p
		}
	}
	return username, password
}

func syncLegacyAuthFieldsFromGroups(parsed *ParsedUserInput) {
	if parsed == nil || len(parsed.AuthCredentialGroups) == 0 {
		return
	}
	if strings.TrimSpace(parsed.AuthUsername) != "" && strings.TrimSpace(parsed.AuthPassword) != "" {
		return
	}
	acc := firstCredentialAccount(parsed.AuthCredentialGroups)
	if acc == nil {
		return
	}
	if strings.TrimSpace(parsed.AuthUsername) == "" {
		parsed.AuthUsername = acc.Username
	}
	if strings.TrimSpace(parsed.AuthPassword) == "" {
		parsed.AuthPassword = acc.Password
	}
}

func firstCredentialAccount(groups []UserCredentialGroup) *UserCredentialAccount {
	for _, id := range []string{"default", "admin", "user", "web", "api"} {
		for _, g := range groups {
			if g.GroupID != id || len(g.Accounts) == 0 {
				continue
			}
			return &g.Accounts[0]
		}
	}
	for i := range groups {
		if len(groups[i].Accounts) > 0 {
			return &groups[i].Accounts[0]
		}
	}
	return nil
}

// CredentialGroupIDsForAuthRealm maps an auth_realm to preferred user credential group ids (in try order).
func CredentialGroupIDsForAuthRealm(authRealm string) []string {
	realm := NormalizeAuthRealm(authRealm)
	switch realm {
	case AuthRealmAdmin:
		return []string{"admin", "default"}
	case AuthRealmAPI:
		return []string{"api", "admin", "default"}
	case AuthRealmWeb, "member":
		return []string{"user", "web", "default"}
	case AuthRealmOAuth:
		return []string{"oauth", "user", "default"}
	default:
		return []string{realm, "default"}
	}
}

// AccountsForAuthRealm returns user-provided accounts for a realm (group order + fallback).
func AccountsForAuthRealm(groups []UserCredentialGroup, authRealm string) []UserCredentialAccount {
	if len(groups) == 0 {
		return nil
	}
	byID := map[string]UserCredentialGroup{}
	for _, g := range groups {
		byID[g.GroupID] = g
	}
	var out []UserCredentialAccount
	for _, gid := range CredentialGroupIDsForAuthRealm(authRealm) {
		if g, ok := byID[gid]; ok {
			out = appendUniqueAccounts(out, g.Accounts...)
		}
	}
	if len(out) == 0 {
		if g, ok := byID["default"]; ok {
			out = append(out, g.Accounts...)
		}
	}
	return out
}

func (rt *Runtime) UserCredentialGroups() []UserCredentialGroup {
	if rt == nil {
		return nil
	}
	return rt.UserAuthCredentialGroups
}

func (rt *Runtime) AccountsForAuthRealm(authRealm string) []UserCredentialAccount {
	return AccountsForAuthRealm(rt.UserCredentialGroups(), authRealm)
}

func primaryUserCredentials(rt *Runtime) (username, password string) {
	if rt == nil {
		return "", ""
	}
	if acc := firstCredentialAccount(rt.UserAuthCredentialGroups); acc != nil {
		return acc.Username, acc.Password
	}
	return strings.TrimSpace(rt.UserAuthUsername), strings.TrimSpace(rt.UserAuthPassword)
}

// FormatUserCredentialGroupsJSON returns redacted-safe JSON for prompts (passwords included — task-local).
func FormatUserCredentialGroupsJSON(groups []UserCredentialGroup) string {
	if len(groups) == 0 {
		return "[]"
	}
	b, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(b)
}

// FormatUserCredentialGroupsInstruction is appended to auth-related playbooks.
func FormatUserCredentialGroupsInstruction(rt *Runtime) string {
	groups := rt.UserCredentialGroups()
	if len(groups) == 0 {
		return "## 用户凭证组\n\n（未提供多组账号；可使用 legacy 单账号字段或代码默认测试账号。）"
	}
	var b strings.Builder
	b.WriteString("## 用户凭证组（输入阶段注入）\n\n")
	b.WriteString("登录/鉴权校准时 **必须** 按 `auth_realm` + `login_page_kind` 选用**权限匹配**的 `group_id` 下账号。\n")
	b.WriteString("**同组轮换（强制）**：某一账号登录失败（密码错误、仍回登录页、带 Cookie 访问目标面仍 401/跳转 login、账户锁定）→ **立即**换同 group 下一账号；**禁止**只试第一条就停止。\n")
	b.WriteString("**跨组禁止**：低权限 group 的账号不得用于 backend/admin 登录面；高权限 admin 账号不得代替 web/前台 member 面（除非该 realm 无对应 group 且 evidence 表明共用登录口）。\n")
	b.WriteString("**全部失败**：当前 realm 允许的所有 group 内账号都试完仍失败，才可标记该 realm 未校准；须记录每个账号的失败原因。\n\n")
	b.WriteString("### group_id → 权限与 auth_realm\n")
	b.WriteString("- `admin` → 后台/管理面（`login_page_kind=backend`，`auth_realm=admin`）\n")
	b.WriteString("- `user` / `web` → 前台/会员面（`login_page_kind=frontend`，`auth_realm=web`）\n")
	b.WriteString("- `api` → API/App 登录面（`auth_realm=api`）\n")
	b.WriteString("- `default` → 未标注 group 时的兜底；优先尝试与当前 realm 匹配的专用 group\n\n")
	b.WriteString("### 凭证数据\n```json\n")
	b.WriteString(FormatUserCredentialGroupsJSON(groups))
	b.WriteString("\n```\n")
	return b.String()
}

func countCredentialAccounts(groups []UserCredentialGroup) int {
	n := 0
	for _, g := range groups {
		n += len(g.Accounts)
	}
	return n
}

func formatCredentialGroupHintForRealm(rt *Runtime, authRealm string) string {
	if rt == nil {
		return ""
	}
	groups := rt.UserCredentialGroups()
	if len(groups) == 0 {
		return FormatUserCredentialGroupsInstruction(rt)
	}
	authRealm = NormalizeAuthRealm(authRealm)
	allowed := CredentialGroupIDsForAuthRealm(authRealm)
	accs := rt.AccountsForAuthRealm(authRealm)

	var b strings.Builder
	b.WriteString("## 本 realm 凭证轮换与权限（必读）\n\n")
	b.WriteString(fmt.Sprintf("auth_realm=%q\n\n", authRealm))

	b.WriteString("### 权限匹配规则\n")
	b.WriteString("- `login_page_kind=backend` / 后台登录口 → **仅**使用 `admin`（及本 realm 允许的 admin 类 group）账号。\n")
	b.WriteString("- `login_page_kind=frontend` / 前台会员登录口 → **仅**使用 `user`/`web` group 账号。\n")
	b.WriteString("- **禁止**用 `user`/`web` 账号 POST 到后台 `login_post_path`（会出现 Wrong password / 仅 JSESSIONID 无管理 Cookie）。\n")
	b.WriteString("- **禁止**用 `admin` 账号校准 web 前台面（除非 mechanism 明确同一登录口且 evidence 支持）。\n\n")

	b.WriteString("### 本 realm 允许的 group（按顺序）\n")
	for _, gid := range allowed {
		for _, g := range groups {
			if g.GroupID != gid || len(g.Accounts) == 0 {
				continue
			}
			label := g.Label
			if label == "" {
				label = credentialGroupLabel(gid)
			}
			b.WriteString(fmt.Sprintf("- `%s` — %s (%d account(s))\n", gid, label, len(g.Accounts)))
		}
	}

	if forbidden := forbiddenCredentialGroupsForRealm(authRealm, groups, allowed); len(forbidden) > 0 {
		b.WriteString("\n### 本 realm 禁止使用的 group\n")
		for _, line := range forbidden {
			b.WriteString("- " + line + "\n")
		}
	}

	b.WriteString("\n### 试登顺序（失败必须换下一个，不得只试一条）\n")
	if len(accs) == 0 {
		b.WriteString("（无映射到此 realm 的账号；检查 group_id 是否与 auth_realm 匹配）\n")
	} else {
		for i, a := range accs {
			gid := groupIDForAccount(groups, a.Username, a.Password)
			b.WriteString(fmt.Sprintf("%d. group=%q username=%q\n", i+1, gid, a.Username))
		}
		if len(accs) == 1 {
			b.WriteString("\n仅 1 个账号映射到此 realm：若失败，先确认 login_post_path/login_page_kind 与 group 权限是否匹配，再考虑是否缺其他 group 输入。\n")
		}
	}

	b.WriteString("\n### 单账号失败判定（满足任一则换下一账号）\n")
	b.WriteString("- 响应仍含登录页 / 密码错误或用户不存在文案\n")
	b.WriteString("- 302 但无 realm 对应 session Cookie，或带 Cookie GET 后台仍跳转 login\n")
	b.WriteString("- 账户锁定 / 验证码失败\n")
	b.WriteString("- 同账号重复 POST 超过 2 次仍失败 → 换账号，勿 brute-force\n")

	return b.String()
}

func forbiddenCredentialGroupsForRealm(authRealm string, groups []UserCredentialGroup, allowed []string) []string {
	allowedSet := map[string]struct{}{}
	for _, id := range allowed {
		allowedSet[id] = struct{}{}
	}
	var out []string
	for _, g := range groups {
		if len(g.Accounts) == 0 {
			continue
		}
		if _, ok := allowedSet[g.GroupID]; ok {
			continue
		}
		reason := credentialGroupMismatchReason(g.GroupID, authRealm)
		out = append(out, fmt.Sprintf("`%s` (%d account(s)): %s", g.GroupID, len(g.Accounts), reason))
	}
	return out
}

func credentialGroupMismatchReason(groupID, authRealm string) string {
	switch NormalizeAuthRealm(authRealm) {
	case AuthRealmAdmin:
		return "front-end/API credentials must not be used for backend admin login"
	case AuthRealmWeb, AuthRealmMember:
		return "admin credentials belong to admin realm unless login_page evidence shows shared entry"
	case AuthRealmAPI:
		return "use api group (or admin if shared token login) only"
	default:
		return "group not mapped to auth_realm=" + authRealm
	}
}

func groupIDForAccount(groups []UserCredentialGroup, username, password string) string {
	for _, g := range groups {
		for _, a := range g.Accounts {
			if a.Username == username && (password == "" || a.Password == password) {
				return g.GroupID
			}
		}
	}
	return "unknown"
}

func credentialGroupsTimelineSummary(groups []UserCredentialGroup) string {
	if len(groups) == 0 {
		return ""
	}
	var parts []string
	for _, g := range groups {
		parts = append(parts, fmt.Sprintf("%s×%d", g.GroupID, len(g.Accounts)))
	}
	return fmt.Sprintf("groups=%d accounts=%d [%s]", len(groups), countCredentialAccounts(groups), strings.Join(parts, ", "))
}

// credentialsForRuntime returns the first account from default/admin group or legacy fields.
func credentialsForRuntime(rt *Runtime) (string, string) {
	if rt != nil {
		if u, p := primaryUserCredentials(rt); u != "" && p != "" {
			return u, p
		}
		if u, p := primaryUserCredentials(rt); p != "" {
			if u == "" {
				u = "admin"
			}
			return u, p
		}
	}
	return defaultAuthCredentials()
}
