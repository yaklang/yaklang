package yakgrpc

import (
	"errors"
	"io"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	oneMB = 1024 * 1024
)

func (s *Server) ReadFile(req *ypb.ReadFileRequest, stream ypb.Yak_ReadFileServer) error {
	bufSize, filePath := req.GetBufSize(), req.GetFilePath()
	if bufSize == 0 {
		bufSize = oneMB
	} else if bufSize < 0 {
		return utils.Error("bufSize must be positive")
	}

	var fs filesys_interface.FileSystem
	if req.GetFileSystem() == "local" {
		fs = filesys.NewLocalFs()
	} else if req.GetFileSystem() == "ssadb" {
		// return utils.Error("unsupported file system")
		fs = ssadb.NewIrSourceFs()
	} else {
		// default
		fs = filesys.NewLocalFs()
	}

	fh, err := fs.Open(filePath)
	if err != nil {
		return utils.Wrap(err, "read file error")
	}
	defer fh.Close()

	buf := make([]byte, bufSize)
	for {
		n, err := fh.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if err := stream.Send(&ypb.ReadFileResponse{Data: buf[:n], EOF: true}); err != nil {
					return utils.Wrap(err, "send file data from stream error")
				}
				break
			} else {
				return utils.Wrap(err, "read file error")
			}
		}
		if n == 0 {
			break
		}
		if err := stream.Send(&ypb.ReadFileResponse{Data: buf[:n], EOF: false}); err != nil {
			return utils.Wrap(err, "send file data from stream error")
		}
	}

	return nil
}
