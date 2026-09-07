package yakgrpc

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	isInAttached         = utils.NewBool(false)
	attachOutputCallback = new(sync.Map)
)

func (s *Server) AttachCombinedOutput(req *ypb.AttachCombinedOutputRequest, server ypb.Yak_AttachCombinedOutputServer) error {
	lines := utils.GetCachedLog()
	server.Send(&ypb.ExecResult{
		Raw: []byte("===========History log=========\n" + strings.Join(lines, "\n") + "\n===========Real time log=========\n"),
	})
	return utils.HandleStdout(server.Context(), func(s string) {
		server.Send(&ypb.ExecResult{
			Raw: []byte(s),
		})
	})
}

func (s *Server) SaveTextToTemporalFile(ctx context.Context, req *ypb.SaveTextToTemporalFileRequest) (*ypb.SaveTextToTemporalFileResponse, error) {
	fileName, err := consts.TempFile("tmp*.txt")
	if err != nil {
		return nil, err
	}
	path := fileName.Name()
	keep := false
	defer func() {
		_ = fileName.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()

	written, err := fileName.Write(req.GetText())
	if err != nil {
		return nil, err
	}
	if written != len(req.GetText()) {
		return nil, io.ErrShortWrite
	}
	if err := fileName.Close(); err != nil {
		return nil, err
	}
	keep = true
	return &ypb.SaveTextToTemporalFileResponse{FileName: path}, nil
}

// UploadToTemporaryFile receives GUI-local bytes over the current gRPC
// connection and stores them in the engine's temporary filesystem. File
// Fuzztags always resolve paths on the engine, so callers must use the
// returned path instead of embedding a GUI-local path in remote mode.
func (s *Server) UploadToTemporaryFile(stream ypb.Yak_UploadToTemporaryFileServer) error {
	return receiveTemporaryFileUpload(stream.Recv, stream.SendAndClose)
}

func receiveTemporaryFileUpload(
	recv func() (*ypb.UploadToTemporaryFileRequest, error),
	finish func(*ypb.UploadToTemporaryFileResponse) error,
) error {
	file, err := consts.TempFile("fuzztag-upload-*")
	if err != nil {
		return err
	}
	path := file.Name()
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()

	var size int64
	for {
		chunk, recvErr := recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			return recvErr
		}
		data := chunk.GetData()
		if len(data) == 0 {
			continue
		}
		written, writeErr := file.Write(data)
		if writeErr != nil {
			return writeErr
		}
		if written != len(data) {
			return io.ErrShortWrite
		}
		size += int64(written)
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := finish(&ypb.UploadToTemporaryFileResponse{FileName: path, Size: size}); err != nil {
		return err
	}
	keep = true
	return nil
}
