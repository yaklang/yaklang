package yakgit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// PRFileChange 表示 PR 中的一个文件变更
type PRFileChange struct {
	Filename         string `json:"filename"`
	Status           string `json:"status"` // added, modified, removed, renamed
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch,omitempty"`
	PreviousFilename string `json:"previous_filename,omitempty"` // for renamed files
}

// PullRequestInfo PR 基本信息
type PullRequestInfo struct {
	Base struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
	Head struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
}

// getPullRequestInfo 获取 PR 基本信息
func getPullRequestInfo(owner, repo string, prNumber int, token string) (*PullRequestInfo, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, prNumber)

	req := lowhttp.BasicRequest()
	req = lowhttp.SetHTTPPacketUrl(req, apiURL)
	req = lowhttp.ReplaceHTTPPacketMethod(req, "GET")
	req = lowhttp.ReplaceHTTPPacketHeader(req, "Accept", "application/vnd.github.v3+json")
	if token != "" {
		req = lowhttp.ReplaceHTTPPacketHeader(req, "Authorization", "token "+token)
	}

	resp, err := lowhttp.HTTP(lowhttp.WithRequest(string(req)), lowhttp.WithHttps(true))
	if err != nil {
		return nil, utils.Wrap(err, "GitHub API request failed")
	}

	statusCode := lowhttp.GetStatusCodeFromResponse(resp.RawPacket)
	if statusCode != http.StatusOK {
		body := lowhttp.GetHTTPPacketBody(resp.RawPacket)
		return nil, utils.Errorf("GitHub API error: %d - %s", statusCode, string(body))
	}

	body := lowhttp.GetHTTPPacketBody(resp.RawPacket)
	var pr PullRequestInfo
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, utils.Wrap(err, "parse PR info failed")
	}

	return &pr, nil
}

// getPullRequestFiles 获取 PR 的文件变更列表
func getPullRequestFiles(owner, repo string, prNumber int, token string) ([]PRFileChange, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/files", owner, repo, prNumber)

	req := lowhttp.BasicRequest()
	req = lowhttp.SetHTTPPacketUrl(req, apiURL)
	req = lowhttp.ReplaceHTTPPacketMethod(req, "GET")
	req = lowhttp.ReplaceHTTPPacketHeader(req, "Accept", "application/vnd.github.v3+json")
	if token != "" {
		req = lowhttp.ReplaceHTTPPacketHeader(req, "Authorization", "token "+token)
	}

	resp, err := lowhttp.HTTP(lowhttp.WithRequest(string(req)), lowhttp.WithHttps(true))
	if err != nil {
		return nil, utils.Wrap(err, "GitHub API request failed")
	}

	statusCode := lowhttp.GetStatusCodeFromResponse(resp.RawPacket)
	if statusCode != http.StatusOK {
		body := lowhttp.GetHTTPPacketBody(resp.RawPacket)
		return nil, utils.Errorf("GitHub API error: %d - %s", statusCode, string(body))
	}

	body := lowhttp.GetHTTPPacketBody(resp.RawPacket)
	var files []PRFileChange
	if err := json.Unmarshal(body, &files); err != nil {
		return nil, utils.Wrap(err, "parse PR files failed")
	}

	return files, nil
}

// getFileContentFromPR 从 PR 的 head commit 获取文件内容
func getFileContentFromPR(owner, repo, sha, filepath, token string) (string, error) {
	// URL encode the filepath
	encodedPath := url.QueryEscape(filepath)
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		owner, repo, encodedPath, sha)

	req := lowhttp.BasicRequest()
	req = lowhttp.SetHTTPPacketUrl(req, apiURL)
	req = lowhttp.ReplaceHTTPPacketMethod(req, "GET")
	req = lowhttp.ReplaceHTTPPacketHeader(req, "Accept", "application/vnd.github.v3+json")
	if token != "" {
		req = lowhttp.ReplaceHTTPPacketHeader(req, "Authorization", "token "+token)
	}

	resp, err := lowhttp.HTTP(lowhttp.WithRequest(string(req)), lowhttp.WithHttps(true))
	if err != nil {
		return "", utils.Wrap(err, "GitHub API request failed")
	}

	statusCode := lowhttp.GetStatusCodeFromResponse(resp.RawPacket)
	if statusCode != http.StatusOK {
		body := lowhttp.GetHTTPPacketBody(resp.RawPacket)
		return "", utils.Errorf("GitHub API error: %d - %s", statusCode, string(body))
	}

	body := lowhttp.GetHTTPPacketBody(resp.RawPacket)
	var content struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}

	if err := json.Unmarshal(body, &content); err != nil {
		return "", utils.Wrap(err, "parse file content failed")
	}

	// GitHub API 返回的是 base64 编码的内容
	if content.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content.Content, "\n", ""))
		if err != nil {
			return "", utils.Wrap(err, "decode base64 content failed")
		}
		return string(decoded), nil
	}

	return content.Content, nil
}

// FromPullRequest 从 GitHub PR 获取文件系统
// owner: 仓库所有者（如 "yaklang"）
// repo: 仓库名（如 "yaklang"）
// prNumber: PR 编号
// token: GitHub 访问令牌（可选，用于私有仓库或提高速率限制）
// localRepo: 本地仓库路径（可选，如果提供则优先使用本地仓库获取文件内容）
func FromPullRequest(owner, repo string, prNumber int, token, localRepo string) (*filesys.VirtualFS, error) {
	// 1. 获取 PR 信息（包含 base 和 head 信息）
	pr, err := getPullRequestInfo(owner, repo, prNumber, token)
	if err != nil {
		return nil, utils.Wrap(err, "get pull request info")
	}

	log.Infof("PR #%d: %s (base: %s@%s, head: %s@%s)",
		prNumber, pr.Title, pr.Base.Ref, pr.Base.SHA[:8], pr.Head.Ref, pr.Head.SHA[:8])

	// 2. 获取 PR 的文件变更列表
	files, err := getPullRequestFiles(owner, repo, prNumber, token)
	if err != nil {
		return nil, utils.Wrap(err, "get pull request files")
	}

	log.Infof("PR #%d: %d files changed", prNumber, len(files))

	// 3. 获取 base commit 的完整文件系统
	var basevfs *filesys.VirtualFS
	if localRepo != "" {
		// 优先使用本地仓库
		res, err := GitOpenRepositoryWithCache(localRepo)
		if err != nil {
			log.Warnf("failed to open local repository, fallback to GitHub API: %v", err)
		} else {
			// basevfs, err = fetchCommitFullTree(res, pr.Base.SHA)
			basevfs, err = fetchRespos(res, pr.Base.SHA)
			if err != nil {
				log.Warnf("failed to fetch base commit from local repo, fallback to GitHub API: %v", err)
			}
		}
	}

	// 如果本地仓库获取失败，创建一个空的 VirtualFS
	if basevfs == nil {
		basevfs = filesys.NewVirtualFs()
		log.Infof("using empty base filesystem (files will be added from PR changes)")
	}

	// 4. 应用 PR 的文件变更
	fs := basevfs
	var res *git.Repository
	if localRepo != "" {
		var err error
		res, err = GitOpenRepositoryWithCache(localRepo)
		if err != nil {
			log.Warnf("failed to open local repository for file content: %v", err)
		}
	}

	count := 0
	for _, fileChange := range files {
		switch fileChange.Status {
		case "added", "modified":
			var content string
			var err error

			// 优先从本地仓库获取文件内容
			if res != nil {
				commit, err := GetCommitHashEx(res, pr.Head.SHA)
				if err == nil {
					tree, err := commit.Tree()
					if err == nil {
						file, err := tree.File(fileChange.Filename)
						if err == nil {
							content, err = file.Contents()
							if err == nil {
								log.Debugf("got file %s from local repository", fileChange.Filename)
							}
						}
					}
				}
			}

			// 如果本地获取失败，使用 GitHub API
			if content == "" && err != nil {
				content, err = getFileContentFromPR(owner, repo, pr.Head.SHA, fileChange.Filename, token)
				if err != nil {
					log.Warnf("failed to get file content for %s: %v", fileChange.Filename, err)
					continue
				}
				log.Debugf("got file %s from GitHub API", fileChange.Filename)
			}

			if exists, _ := fs.Exists(fileChange.Filename); exists {
				fs.RemoveFileOrDir(fileChange.Filename)
			}
			fs.AddFile(fileChange.Filename, content)
			count++

		case "removed":
			if exists, _ := fs.Exists(fileChange.Filename); exists {
				fs.RemoveFileOrDir(fileChange.Filename)
				count++
			}

		case "renamed":
			// 处理重命名：删除旧文件，添加新文件
			oldFilename := fileChange.PreviousFilename
			if oldFilename == "" {
				oldFilename = fileChange.Filename // fallback
			}

			// 获取新文件内容
			var content string
			var err error

			if res != nil {
				commit, err := GetCommitHashEx(res, pr.Head.SHA)
				if err == nil {
					tree, err := commit.Tree()
					if err == nil {
						file, err := tree.File(fileChange.Filename)
						if err == nil {
							content, err = file.Contents()
						}
					}
				}
			}

			if content == "" && err != nil {
				content, err = getFileContentFromPR(owner, repo, pr.Head.SHA, fileChange.Filename, token)
				if err != nil {
					log.Warnf("failed to get renamed file content for %s: %v", fileChange.Filename, err)
					continue
				}
			}

			// 删除旧文件
			if exists, _ := fs.Exists(oldFilename); exists {
				fs.RemoveFileOrDir(oldFilename)
			}

			// 添加新文件
			if exists, _ := fs.Exists(fileChange.Filename); exists {
				fs.RemoveFileOrDir(fileChange.Filename)
			}
			fs.AddFile(fileChange.Filename, content)
			count++
		}
	}

	log.Infof("PR #%d: applied %d file changes", prNumber, count)
	return fs, nil
}
