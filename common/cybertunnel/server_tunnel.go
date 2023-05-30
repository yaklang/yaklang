package cybertunnel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ReneKroon/ttlcache"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"sync"
	"time"
)

var tunnels = new(sync.Map)
var registeredTunnel = ttlcache.NewCache()
var portToTunnel = new(sync.Map)
var historicalTunnel = ttlcache.NewCache()

type Tunnel struct {
	Id string

	Host         string
	Port         int
	PublicKeyPEM []byte
	Secret       string
	Verbose      string

	// finished / registered / working
	Status string
}

func (t *Tunnel) GetAuth() []byte {
	var auth = make(map[string]string)
	auth["host"] = t.Host
	auth["port"] = fmt.Sprint(t.Port)
	auth["pubpem"] = codec.EncodeBase64(t.PublicKeyPEM)
	auth["secret"] = t.Secret
	raw, _ := json.Marshal(auth)
	return raw
}

func init() {
	historicalTunnel.SetTTL(24 * time.Hour * 3)
	registeredTunnel.SetTTL(5 * time.Minute)
	registeredTunnel.SetExpirationCallback(func(key string, value interface{}) {
		historicalTunnel.Set(key, value)
		ins, _ := value.(*Tunnel)
		if ins != nil {
			portToTunnel.Delete(ins.Port)
			tunnels.Delete(ins.Id)
		}
	})
	registeredTunnel.SetNewItemCallback(func(key string, value interface{}) {
		ins, _ := value.(*Tunnel)
		if ins != nil {
			portToTunnel.Store(ins.Port, ins)
			tunnels.Store(ins.Id, ins)
		}
	})
}

func NewTunnel(id string, host string, port int, pub []byte, secret string, verbose string) *Tunnel {
	t := &Tunnel{
		Id:           id,
		Host:         host,
		Port:         port,
		PublicKeyPEM: pub,
		Secret:       secret,
		Verbose:      verbose,
		Status:       "registered",
	}
	registeredTunnel.Set(id, t)
	return t
}

func GetTunnels() []*Tunnel {
	var tuns []*Tunnel
	tunnels.Range(func(key, value any) bool {
		t, _ := value.(*Tunnel)
		if t != nil {
			tuns = append(tuns, t)
		}
		return true
	})
	return tuns
}

func GetTunnel(id string) (*Tunnel, error) {
	ins, ok := tunnels.Load(id)
	//registeredTunnel.Set(id, ins)
	if ok {
		return ins.(*Tunnel), nil
	}
	return nil, errors.New("no such tunnel by id")
}

func RemoveTunnel(id string) {
	tunnels.Delete(id)
}

func (s *TunnelServer) RegisterTunnel(ctx context.Context, req *tpb.RegisterTunnelRequest) (*tpb.RegisterTunnelResponse, error) {
	id := uuid.NewV4().String()
	port := utils.GetRandomAvailableTCPPort()
	NewTunnel(id, s.ExternalIP, port, req.GetPublicKeyPEM(), req.GetSecret(), req.GetVerbose())
	return &tpb.RegisterTunnelResponse{
		Id: id,
	}, nil
}

func (t *TunnelServer) GetAllRegisteredTunnel(ctx context.Context, req *tpb.GetAllRegisteredTunnelRequest) (*tpb.GetAllRegisteredTunnelResponse, error) {
	if req.GetSecondaryPassword() != t.SecondaryPassword {
		log.Errorf("secondary password expected: %s got: %v", t.SecondaryPassword, req.GetSecondaryPassword())
		return nil, utils.Error("GetAllRegisteredTunnel 401")
	}
	return &tpb.GetAllRegisteredTunnelResponse{
		Tunnels: funk.Map(GetTunnels(), func(t *Tunnel) *tpb.RegisterTunnelMeta {
			return &tpb.RegisterTunnelMeta{
				ConnectHost: t.Host,
				ConnectPort: int64(t.Port),
				Id:          t.Id,
				Verbose:     t.Verbose,
			}
		}).([]*tpb.RegisterTunnelMeta),
	}, nil
}

func (t *TunnelServer) GetRegisteredTunnelDescriptionByID(ctx context.Context, req *tpb.GetRegisteredTunnelDescriptionByIDRequest) (*tpb.RegisteredTunnel, error) {
	if req.GetSecondaryPassword() != t.SecondaryPassword {
		log.Errorf("secondary password expected: %s got: %v", t.SecondaryPassword, req.GetSecondaryPassword())
		return nil, utils.Error("GetRegisteredTunnelDescriptionByID 401")
	}

	tun, err := GetTunnel(req.GetId())
	if err != nil {
		return nil, err
	}
	return &tpb.RegisteredTunnel{
		Info: &tpb.RegisterTunnelMeta{
			ConnectHost: tun.Host,
			ConnectPort: int64(tun.Port),
			Id:          tun.Id,
			Verbose:     tun.Verbose,
		},
		Auth: tun.GetAuth(),
	}, nil
}
