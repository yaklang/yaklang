package yaklib

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
)

func TestMUSTPASS_QueryYakProjectsFullPageLimit(t *testing.T) {
	profile, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	defer profile.Close()
	require.NoError(t, profile.AutoMigrate(&schema.Project{}).Error)
	for i := 0; i < 55; i++ {
		require.NoError(t, profile.Create(&schema.Project{ProjectName: "project-" + strconv.Itoa(i), DatabasePath: "project-" + strconv.Itoa(i) + ".db", Type: "project"}).Error)
	}
	opts := []AIQueryOption{WithAIQueryDatabases(profile, profile), WithAIQueryLimit(50)}
	first, err := QueryYakProjects("", opts...)
	require.NoError(t, err)
	require.Equal(t, 55, first["total"])
	require.Len(t, first["projects"], 50)
	require.Equal(t, 50, first["next_offset"])
	second, err := QueryYakProjects("", append(opts, WithAIQueryOffset(50))...)
	require.NoError(t, err)
	require.Len(t, second["projects"], 5)
	require.Equal(t, 55, second["next_offset"])
	_, err = QueryYakProjects("", append(opts, WithAIQueryLimit(51))...)
	require.Error(t, err)
}

func TestMUSTPASS_QueryHTTPHistoryProjectIDsAndFilters(t *testing.T) {
	profile, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	defer profile.Close()
	require.NoError(t, profile.AutoMigrate(&schema.Project{}).Error)
	current, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "current.db"))
	require.NoError(t, err)
	defer current.Close()
	targetPath := filepath.Join(t.TempDir(), "target special ?.db")
	target, err := gorm.Open("sqlite3", (&url.URL{Scheme: "file", Path: targetPath}).String())
	require.NoError(t, err)
	defer target.Close()
	for _, db := range []*gorm.DB{current, target} {
		require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)
	}
	require.NoError(t, current.Create(&schema.HTTPFlow{Url: "https://current.example/", Method: "GET", StatusCode: 200}).Error)
	flow := &schema.HTTPFlow{Url: "https://target.example/login", Method: "POST", StatusCode: 401, Request: strconv.Quote("POST /login HTTP/1.1\r\n\r\npassword=sentinel"), Response: strconv.Quote("HTTP/1.1 401 Unauthorized\r\n\r\n登录失败")}
	require.NoError(t, target.Create(flow).Error)
	project := &schema.Project{ProjectName: "nested target", DatabasePath: targetPath, Type: "project", FolderID: 22}
	require.NoError(t, profile.Create(project).Error)
	opts := []AIQueryOption{WithAIQueryDatabases(current, profile)}
	projects, err := QueryYakProjects("", opts...)
	require.NoError(t, err)
	require.Len(t, projects["projects"], 1)
	result, err := QueryHTTPHistory(`{"Methods":"GET"}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 1, result["total"])
	targetOpts := append(opts, WithAIProject(int64(project.ID)))
	result, err = QueryHTTPHistory(`{"Keyword":"password=sentinel","Methods":"POST","StatusCode":"400-499"}`, targetOpts...)
	require.NoError(t, err)
	require.Equal(t, 1, result["total"])
	hits := result["hits"].([]map[string]any)
	require.Equal(t, flow.Url, hits[0]["url"])
	require.NotContains(t, hits[0], "request")
	result, err = QueryHTTPHistory(`{"IncludeId":[`+strconv.Itoa(int(flow.ID))+`]}`, append(targetOpts, WithAIContentLimit(10))...)
	require.NoError(t, err)
	hits = result["hits"].([]map[string]any)
	require.True(t, hits[0]["request_truncated"].(bool))
	require.Len(t, []rune(hits[0]["request"].(string)), 10)
	_, err = QueryHTTPHistory(`{"Methods":"GET"} {}`, opts...)
	require.Error(t, err)
	_, err = QueryHTTPHistory(`{"MadeUpFilter":true}`, opts...)
	require.Error(t, err)
	_, err = QueryHTTPHistory(`{}`, append(opts, WithAIProject(999999))...)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = QueryHTTPHistory(`{}`, append(opts, WithAIQueryContext(ctx))...)
	require.ErrorIs(t, err, context.Canceled)
	db, closeDB, err := aiHTTPDatabase(&aiQueryConfig{profileDB: profile, projectID: int64(project.ID)})
	require.NoError(t, err)
	defer closeDB()
	require.Error(t, db.Exec("DELETE FROM http_flows").Error)
}
