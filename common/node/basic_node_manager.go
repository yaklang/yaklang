package node

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mq"
	base2 "github.com/yaklang/yaklang/common/node/baserpc"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"time"
)

func (b *NodeBase) initBasicNodeManagerAPI() {
	server := base2.NewManagerAPIServerHelper()
	server.DoManagerAPI_Echo = func(ctx context.Context, node string, req *base2.ManagerAPI_EchoRequest, broker *mq.Broker) (*base2.ManagerAPI_EchoResponse, error) {
		return &base2.ManagerAPI_EchoResponse{
			Data: req.Data,
		}, nil
	}
	server.DoManagerAPI_Exec = func(ctx context.Context, node string, req *base2.ManagerAPI_ExecRequest, broker *mq.Broker) (*base2.ManagerAPI_ExecResponse, error) {
		var raw []byte
		if req.TimeoutStr == "" {
			req.TimeoutStr = "10s"
		}

		timeout, err := time.ParseDuration(req.TimeoutStr)
		if err != nil {
			return nil, utils.Errorf("parse [%s] timeout: %s", req.TimeoutStr, err)
		}

		raw, err = exec.CommandContext(utils.TimeoutContext(timeout), req.Binary, req.Args...).CombinedOutput()
		if err != nil {
			return nil, utils.Errorf("err: %v raw: \n%v", err, spew.Sdump(raw))
		}
		return &base2.ManagerAPI_ExecResponse{CombinedOutput: raw}, nil
	}
	server.DoManagerAPI_ReadDir = func(ctx context.Context, node string, req *base2.ManagerAPI_ReadDirRequest, broker *mq.Broker) (*base2.ManagerAPI_ReadDirResponse, error) {
		var infos []*base2.FileInfo
		fs, err := ioutil.ReadDir(req.Target)
		if err != nil {
			return nil, utils.Errorf("read files recursively failed: %s", err)
		}

		for _, f := range fs {
			infos = append(infos, &base2.FileInfo{
				Name:            f.Name(),
				Path:            filepath.Join(req.Target, f.Name()),
				IsDir:           f.IsDir(),
				ModifyTimestamp: f.ModTime().Unix(),
				BytesSize:       f.Size(),
				Mode:            uint32(f.Mode()),
			})
		}
		return &base2.ManagerAPI_ReadDirResponse{Infos: infos}, nil
	}
	server.DoManagerAPI_ReadDirRecursive = func(ctx context.Context, node string, req *base2.ManagerAPI_ReadDirRecursiveRequest, broker *mq.Broker) (*base2.ManagerAPI_ReadDirRecursiveResponse, error) {
		fs, err := utils.ReadFilesRecursively(req.Target)
		if err != nil {
			return nil, utils.Errorf("read files recursively failed: %s", err)
		}

		var ifs []*base2.FileInfo
		for _, f := range fs {
			ifs = append(ifs, &base2.FileInfo{
				Name:            f.Name,
				Path:            f.Path,
				IsDir:           f.IsDir,
				ModifyTimestamp: f.BuildIn.ModTime().Unix(),
				BytesSize:       f.BuildIn.Size(),
				Mode:            uint32(f.BuildIn.Mode()),
			})
		}
		return &base2.ManagerAPI_ReadDirRecursiveResponse{Infos: ifs}, nil
	}
	server.DoManagerAPI_ReadFile = func(ctx context.Context, node string, req *base2.ManagerAPI_ReadFileRequest, broker *mq.Broker) (*base2.ManagerAPI_ReadFileResponse, error) {
		raw, err := ioutil.ReadFile(req.FileName)
		if err != nil {
			return nil, utils.Errorf("read filed[%s] failed: %s", req.FileName, err)
		}
		return &base2.ManagerAPI_ReadFileResponse{Raw: raw}, nil
	}
	server.DoManagerAPI_Restart = func(ctx context.Context, node string, req *base2.ManagerAPI_RestartRequest, broker *mq.Broker) (*base2.ManagerAPI_RestartResponse, error) {
		log.Error("restart it not implemeted")
		return &base2.ManagerAPI_RestartResponse{}, nil
	}
	server.DoManagerAPI_Shutdown = func(ctx context.Context, node string, req *base2.ManagerAPI_ShutdownRequest, broker *mq.Broker) (*base2.ManagerAPI_ShutdownResponse, error) {
		b.Shutdown()
		return &base2.ManagerAPI_ShutdownResponse{}, nil
	}

	b.rpcServer.RegisterServices(base2.MethodList, server.Do)
}
