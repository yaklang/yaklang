package filesys

import "os"

type FileNode struct {
	Info     os.FileInfo
	Path     string
	Children map[string]*FileNode
	Parent   *FileNode
	IsRoot   bool
}

func (n *FileNode) IsDir() bool {
	return n.Info.IsDir()
}
