package ssaapi

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

type config_info struct {
	Kind string `json:"kind"`
	// The kind of the parse: "local", "compression"

	/*
		"local":
			* "local_file":  path to the local directory
		"compression":
			* "local_file":  path to the local compressed file
		"git":
			* "git_url":  git url
			"git_branch":  git branch
			"ce": {{
				"user_name"  + "user_password",
				"token",
				"ssh_key",
			}}
		"svn":
			* "svn_url":  svn url
			"svn_branch":  svn branch
			"ce": {{
				"user_name"  + "user_password",
				"token"
			}}
		"jar":
			"local_file":  path to the local jar file
	*/

	LocalFile string `json:"local_file"`
	// git or svn
	URL    string `json:"url"`
	Branch string `json:"branch"`
	CE     ce     `json:"ce"`
}
type ce struct {
	UserName     string `json:"user_name"`
	UserPassword string `json:"user_password"`

	Token string `json:"token"`

	SSHkey string `json:"ssh_key"`
}

func (c *config) initializeFromInfo() error {
	raw := c.info
	if raw == "" {
		return nil
	}
	info := config_info{}
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return utils.Errorf("error unmarshal info: %v", err)
	}
	switch info.Kind {
	case "local":
		c.fs = filesys.NewRelLocalFs(info.LocalFile)
	case "compression":
		fs, err := filesys.NewZipFSFromLocal(info.LocalFile)
		if err != nil {
			return utils.Errorf("compression file error: %v", err)
		}
		c.fs = fs
	case "jar":
		fs, err := javaclassparser.NewJarFSFromLocal(info.LocalFile)
		if err != nil {
			return utils.Errorf("jar file error: %v", err)
		}
		c.fs = fs
	case "git":
		return utils.Errorf("git is not supported")
	case "svn":
		return utils.Errorf("svn is not supported")
	default:
		return utils.Errorf("unsupported kind: %s", info.Kind)
	}
	return nil
}

// func localPathToInfo(path string) string {
// 	info := config_info{
// 		Kind:      "info",
// 		LocalFile: path,
// 	}
// 	b, err := json.Marshal(info)
// 	if err != nil {
// 		return ""
// 	}
// 	return string(b)
// }

func (info config_info) String() string {
	b, _ := json.Marshal(info)
	return string(b)
}
