package mcpclient

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// ServerInfo is a public registry entry. Environment and header values stay in
// the profile database; scripts can connect by name without reading credentials.
type ServerInfo struct {
	ID         uint     `json:"id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	URL        string   `json:"url,omitempty"`
	Enable     bool     `json:"enable"`
	EnvKeys    []string `json:"env_keys"`
	HeaderKeys []string `json:"header_keys"`
}

type ServerPage struct {
	Servers    []*ServerInfo `json:"servers"`
	Total      int           `json:"total"`
	Offset     int           `json:"offset"`
	Limit      int           `json:"limit"`
	HasMore    bool          `json:"has_more"`
	NextOffset int           `json:"next_offset"`
}

func registryOptions(opts ...Option) (*options, error) {
	o, err := newOptions(opts...)
	if err != nil {
		return nil, err
	}
	if o.db == nil {
		o.db = consts.GetGormProfileDatabase()
	}
	if o.db == nil {
		return nil, fmt.Errorf("MCP profile database is not initialized")
	}
	return o, nil
}

func stringMap(values map[string]any) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = fmt.Sprint(value)
	}
	return result
}

func schemaMap(values map[string]any) schema.MapStringAny {
	result := make(schema.MapStringAny, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func serverInfo(s *schema.MCPServer) *ServerInfo {
	info := &ServerInfo{ID: s.ID, Name: s.Name, Type: s.Type, Enable: s.Enable,
		EnvKeys: []string{}, HeaderKeys: []string{}}
	// Commands may contain tokens in arbitrary positional arguments. Listing
	// a server never needs those arguments. URL query/userinfo can also hold
	// credentials; only show the endpoint's scheme, host and path.
	if u, err := url.Parse(s.URL); err == nil && s.URL != "" {
		u.User, u.RawQuery, u.Fragment = nil, "", ""
		info.URL = u.String()
	}
	for key := range s.Envs {
		info.EnvKeys = append(info.EnvKeys, key)
	}
	for key := range s.Headers {
		info.HeaderKeys = append(info.HeaderKeys, key)
	}
	sort.Strings(info.EnvKeys)
	sort.Strings(info.HeaderKeys)
	return info
}

// AddServer persists a new server in Yakit's profile registry. It does not
// overwrite an existing name or start a subprocess/network connection.
//
// Example:
// ```
// mcp.AddServer("local", "stdio", `python3 "/tmp/my server.py"`, mcp.env({"TOKEN": "value"}))~
// page = mcp.ListServers("local")~
// dump(page.Servers)
// die(mcp.DeleteServer("local"))
// ```
func AddServer(name, transport, endpoint string, opts ...Option) (*ServerInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("MCP server name is required")
	}
	if err := validateEndpoint(transport, endpoint); err != nil {
		return nil, err
	}
	o, err := registryOptions(opts...)
	if err != nil {
		return nil, err
	}
	srv := &schema.MCPServer{Name: name, Type: transport, Enable: o.enabled,
		Envs: schemaMap(o.env), Headers: schemaMap(o.headers)}
	if transport == "stdio" {
		srv.Command = endpoint
	} else {
		srv.URL = endpoint
	}
	if err := yakit.CreateMCPServer(o.db, srv); err != nil {
		return nil, err
	}
	// GORM fills the default true on Create; yakit's explicit false update
	// persists the requested value but does not restore the in-memory field.
	srv.Enable = o.enabled
	return serverInfo(srv), nil
}

// DeleteServer deletes exactly one named server and its tool/bridge caches via
// the existing transactional Yakit operation. A missing name is an error.
func DeleteServer(name string, opts ...Option) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("MCP server name is required")
	}
	o, err := registryOptions(opts...)
	if err != nil {
		return err
	}
	srv, err := yakit.GetMCPServerByName(o.db, name)
	if err != nil {
		return err
	}
	return yakit.DeleteMCPServer(o.db, int64(srv.ID))
}

// ListServers includes enabled and disabled servers, ordered by name then ID.
// The keyword matches public names/types, never credential-bearing fields.
func ListServers(keyword string, opts ...Option) (*ServerPage, error) {
	o, err := registryOptions(opts...)
	if err != nil {
		return nil, err
	}
	query := o.db.Model(&schema.MCPServer{})
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		query = query.Where("name LIKE ? OR type LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	page := &ServerPage{Servers: []*ServerInfo{}, Offset: o.offset, Limit: o.limit}
	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}
	var rows []*schema.MCPServer
	if err := query.Order("name ASC, id ASC").Offset(o.offset).Limit(o.limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		page.Servers = append(page.Servers, serverInfo(row))
	}
	page.NextOffset = page.Offset + len(page.Servers)
	page.HasMore = page.NextOffset < page.Total
	return page, nil
}
