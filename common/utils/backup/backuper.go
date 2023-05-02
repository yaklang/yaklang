package backup

import (
	"context"
	"encoding/json"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"github.com/yaklang/yaklang/common/utils"
)

type Item struct {
	Id             string    `json:"id"`
	Time           time.Time `json:"time"`
	BackupFilePath string    `json:"file_path"`
	Name           string    `json:"name"`
	OriginPath     string    `json:"origin_path"`
	IsDir          bool      `json:"is_dir"`
}

type Config struct {
	Path  string
	Items []Item
}

func (c *Config) Save(item Item) error {
	for _, i := range c.Items {
		if i.Id == item.Id {
			return utils.Errorf("cannot save existed id: %s", item.Id)
		}
	}
	c.Items = append(c.Items, item)
	return c.save()
}

func (c *Config) save() error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_ = os.RemoveAll(c.Path)
	return ioutil.WriteFile(c.Path, raw, os.ModePerm)
}

type Backuper struct {
	mainDir        string
	mainConfigName string

	config *Config
}

func (b *Backuper) Backup(ctx context.Context, path string) error {
	if !filepath.IsAbs(path) {
		return utils.Errorf("abs path only")
	}

	if len(path) <= 1 {
		return utils.Errorf("empty or root path is not allowed")
	}

	stats, err := os.Stat(path)
	if err != nil {
		return utils.Errorf("stats [%s] Err: %v", path, err)
	}

	id := uuid.NewV4().String()
	if stats.IsDir() {
		// tar
		fileName := fmt.Sprintf(".%v.tar.gz", id)
		filePath := filepath.Join(b.mainDir, fileName)
		cmd := exec.CommandContext(ctx, "tar", "-zcvf", filePath, stats.Name())
		cmd.Dir = filepath.Dir(path)
		raw, err := cmd.CombinedOutput()
		if err != nil {
			return utils.Errorf("tar -zcvf for [%s] to [%s] failed: %s", path, filePath, string(raw))
		}

		item := Item{
			Id:             id,
			Time:           time.Now(),
			Name:           stats.Name(),
			BackupFilePath: filePath,
			OriginPath:     path,
			IsDir:          true,
		}
		err = b.config.Save(item)
		if err != nil {
			return err
		}
	} else {
		fileName := fmt.Sprintf(".%v.backup", id)
		filePath := filepath.Join(b.mainDir, fileName)
		err := exec.CommandContext(ctx, "cp", path, filePath).Run()
		if err != nil {
			return utils.Errorf("cp backup [%s] failed: %s", path, err)
		}

		item := Item{
			Id:             id,
			Time:           time.Now(),
			BackupFilePath: filePath,
			Name:           stats.Name(),
			OriginPath:     path,
			IsDir:          false,
		}
		err = b.config.Save(item)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *Backuper) Recover(ctx context.Context, id string) error {
	item, err := b.GetItemById(id)
	if err != nil {
		return err
	}

	if item.IsDir {
		tmpPath := fmt.Sprintf("/tmp/%v", uuid.NewV4().String())
		_ = os.MkdirAll(tmpPath, 0777)
		raw, err := exec.CommandContext(ctx, "tar", "-zxvf", item.BackupFilePath, "-C", tmpPath).CombinedOutput()
		if err != nil {
			return utils.Errorf("un-extract failed: %s", string(raw))
		}

		defer func() {
			_ = os.RemoveAll(tmpPath)
		}()

		originParent := filepath.Dir(item.OriginPath)

		if stat, _ := os.Stat(originParent); stat != nil {
			_ = os.RemoveAll(item.OriginPath)
		} else {
			_ = os.MkdirAll(originParent, 0777)
		}

		err = exec.CommandContext(ctx, "cp", "-r", filepath.Join(tmpPath, item.Name), item.OriginPath).Run()
		if err != nil {
			return utils.Errorf("cp backup to origin path failed: %s", err)
		}
		return nil
	} else {
		if r, _ := utils.PathExists(item.BackupFilePath); !r {
			return utils.Errorf("backup file not existed: %s", item.BackupFilePath)
		}
		_ = exec.CommandContext(ctx, "rm", item.OriginPath).Run()
		err = exec.CommandContext(ctx, "cp", item.BackupFilePath, item.OriginPath).Run()
		if err != nil {
			return utils.Errorf("cp backup file path to origin path[%s] failed: %s", item.OriginPath, err)
		}
		return nil
	}
}

func (b *Backuper) GetItemById(id string) (*Item, error) {
	for _, item := range b.config.Items {
		if item.Id == id {
			return &item, nil
		}
	}
	return nil, utils.Errorf("get item by id [%s] failed", id)
}

func (b *Backuper) GetAllItems() []Item {
	return b.config.Items
}

func NewDefaultBackuper() (*Backuper, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, utils.Errorf("get user home dir failed: %s", err)
	}
	if !filepath.IsAbs(homeDir) {
		return nil, utils.Errorf("home dir is not abs path")
	}
	homeDir, err = filepath.Abs(homeDir)
	if err != nil {
		return nil, utils.Errorf("convert home dir failed: %s", err)
	}
	return NewBackuper(filepath.Join(homeDir, ".palm-backup"))
}

func NewBackuper(dir string) (*Backuper, error) {
	if !filepath.IsAbs(dir) {
		return nil, utils.Errorf("dir should be abs path")
	}

	err := os.MkdirAll(dir, 0777)
	if err != nil {
		return nil, err
	}

	configName := filepath.Join(dir, ".backup.yaml")
	var config = Config{
		Path:  configName,
		Items: nil,
	}

	if r, _ := utils.PathExists(configName); r {
		raw, _ := ioutil.ReadFile(configName)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &config)
		}
	}

	return &Backuper{mainDir: dir, mainConfigName: configName, config: &config}, nil
}
