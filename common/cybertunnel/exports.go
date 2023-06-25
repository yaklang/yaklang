package cybertunnel

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"google.golang.org/grpc"
	grpcMetadata "google.golang.org/grpc/metadata"
	"net"
	"strings"

	"time"
)

func GetTunnelServerExternalIP(addr string, secret string) (net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if secret != "" {
		ctx = grpcMetadata.AppendToOutgoingContext(
			ctx,
			"authorization", fmt.Sprintf("bearer %v", secret),
		)
	}

	conn, err := grpc.Dial(
		addr,
		grpc.WithInsecure(),
		grpc.WithNoProxy(),
	)
	if err != nil {
		log.Errorf("dial %s failed: %s", addr, err)
		return nil, err
	}
	defer conn.Close()

	client := tpb.NewTunnelClient(conn)
	rsp, err := client.RemoteIP(
		ctx, &tpb.Empty{},
	)
	if err != nil {
		log.Errorf("call remote-ip(%v) failed: %s", addr, err)
		return nil, err
	}
	log.Infof("tunnel server: %s", rsp.GetIPAddress())
	ipIns := net.ParseIP(rsp.GetIPAddress())
	if ipIns == nil {
		return nil, utils.Errorf("parse ip fialed: %s", rsp.GetIPAddress())
	}
	return ipIns, nil
}

func GetClient(ctx context.Context, addr, secret string) (context.Context, tpb.TunnelClient, *grpc.ClientConn, error) {
	conn, err := grpc.Dial(
		addr,
		grpc.WithInsecure(),
		grpc.WithNoProxy(),
	)
	if err != nil {
		log.Errorf("dial %s failed: %s", addr, err)
		return ctx, nil, nil, err
	}

	// 设置密码
	if secret != "" {
		ctx = grpcMetadata.AppendToOutgoingContext(
			ctx,
			"authorization", fmt.Sprintf("bearer %v", secret),
		)
	}

	// create tunnel
	client := tpb.NewTunnelClient(conn)
	return ctx, client, conn, nil
}

func GetDNSLogClient(addr string) (tpb.DNSLogClient, *grpc.ClientConn, error) {
	conn, err := grpc.Dial(
		addr,
		grpc.WithInsecure(),
		grpc.WithNoProxy(),
	)
	if err != nil {
		log.Errorf("dial %s failed: %s", addr, err)
		return nil, nil, err
	}
	return tpb.NewDNSLogClient(conn), conn, nil
}

func MirrorLocalPortToRemote(
	network string,
	localPort int,
	remotePort int,
	id string,
	addr,
	secret string,
	ctx context.Context,
	fs ...func(remoteAddr string, localAddr string),
) error {
	return MirrorLocalPortToRemoteEx(network, "127.0.0.1", localPort, remotePort, id, addr, secret, ctx, fs...)
}

func MirrorLocalPortToRemoteEx(
	network string,
	localHost string,
	localPort int,
	remotePort int,
	id string,
	addr,
	secret string,
	ctx context.Context,
	fs ...func(remoteAddr string, localAddr string),
) error {
	return MirrorLocalPortToRemoteWithRegisterEx(false, nil, "", "", network, localHost, localPort, remotePort, id, addr, secret, ctx, fs...)
}

func MirrorLocalPortToRemoteWithRegisterEx(
	enableRegister bool,
	pubKey []byte,
	grpcSecret string,
	verbose string,

	network string,
	localHost string,
	localPort int,
	remotePort int,
	id string,
	addr,
	secret string,
	ctx context.Context,
	fs ...func(remoteAddr string, localAddr string),
) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = fmt.Errorf("panic unexpected: %s", fErr)
		}
	}()
	if network == "" {
		network = "tcp"
	}

	conn, err := grpc.Dial(
		addr,
		grpc.WithInsecure(),
		grpc.WithNoProxy(),
	)
	if err != nil {
		log.Errorf("dial %s failed: %s", addr, err)
		return err
	}
	defer conn.Close()

	// 设置密码
	if secret != "" {
		ctx = grpcMetadata.AppendToOutgoingContext(
			ctx,
			"authorization", fmt.Sprintf("bearer %v", secret),
		)
	}
	client := tpb.NewTunnelClient(conn)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if enableRegister {
		rsp, err := client.RegisterTunnel(ctx, &tpb.RegisterTunnelRequest{
			PublicKeyPEM: pubKey,
			Secret:       grpcSecret,
			Verbose:      verbose,
		})
		if err != nil {
			return utils.Errorf("register grpc remote tunnel failed: %s", err)
		}
		id = rsp.Id
	}

	// create tunnel
	stream, err := client.CreateTunnel(ctx)
	if err != nil {
		log.Errorf("create tunnel call[%v] failed: %s", addr, err)
		return err
	}

	err = HoldingCreateTunnelClient(stream, localHost, localPort, remotePort, id, fs...)
	if err != nil {
		return err
	}
	return nil
}

func RequirePortByToken(
	token string,
	addr, secret string,
	ctx context.Context,
) (*tpb.RequireRandomPortTriggerResponse, error) {
	if ctx == nil {
		ctx = utils.TimeoutContextSeconds(5)
	}

	ctx, client, conn, err := GetClient(ctx, addr, secret)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return client.RequireRandomPortTrigger(ctx, &tpb.RequireRandomPortTriggerParams{
		Token:      token,
		TTLSeconds: 60,
	})
}

func RequireDNSLogDomain(addr string) (domain string, token string, _ error) {
	if addr == "" {
		addr = consts.GetDefaultPublicReverseServer()
	}
	client, conn, err := GetDNSLogClient(addr)
	if err != nil {
		return "", "", err
	}
	defer conn.Close()

	var count = 0
	for {
		count++
		rsp, err := client.RequireDomain(utils.TimeoutContextSeconds(10), &tpb.RequireDomainParams{})
		if err != nil {
			if count > 3 {
				return "", "", utils.Errorf("require dns domain failed: %s", err)
			}

			if strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded") {
				continue
			}
			return "", "", err
		}
		return rsp.Domain, rsp.Token, nil
	}

}

func QueryExistedDNSLogEvents(addr string, token string) ([]*tpb.DNSLogEvent, error) {
	return QueryExistedDNSLogEventsEx(addr, token, 10)
}

func QueryExistedDNSLogEventsEx(addr string, token string, timeout ...float64) ([]*tpb.DNSLogEvent, error) {
	var f = 5.0
	if len(timeout) > 0 {
		f = timeout[0]
	}
	if f <= 0 {
		f = 5
	}

	if addr == "" {
		addr = consts.GetDefaultPublicReverseServer()
	}
	client, conn, err := GetDNSLogClient(addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var count = 0
	for {
		count++
		rsp, err := client.QueryExistedDNSLog(utils.TimeoutContextSeconds(f), &tpb.QueryExistedDNSLogParams{Token: token})
		if err != nil {
			if count > 3 {
				return nil, utils.Errorf("retry query existed dnslog[retry: %v] failed: %s", count, err.Error())
			}

			reason := strings.ToLower(err.Error())
			if strings.Contains(reason, "i/o timeout") || strings.Contains(reason, "context deadline exceeded") {
				continue
			}
			return nil, err
		}
		return rsp.Events, nil
	}
}

func QueryExistedRandomPortTriggerEvents(token, addr, secret string, ctx context.Context) (*tpb.RandomPortTriggerEvent, error) {
	if ctx == nil {
		ctx = utils.TimeoutContextSeconds(5)
	}

	ctx, client, conn, err := GetClient(ctx, addr, secret)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	events, err := client.QueryExistedRandomPortTrigger(ctx, &tpb.QueryExistedRandomPortTriggerRequest{Token: token})
	if err != nil {
		return nil, err
	}
	if len(events.Events) > 0 {
		return events.Events[0], nil
	}
	return nil, utils.Error("empty")
}

func QueryICMPLengthTriggerNotifications(length int, addr, secret string, ctx context.Context) (*tpb.ICMPTriggerNotification, error) {
	if ctx == nil {
		ctx = utils.TimeoutContextSeconds(10)
	}

	ctx, client, conn, err := GetClient(ctx, addr, secret)
	if err != nil {
		return nil, utils.Errorf("get yak bridge client failed: %s", err)
	}
	defer conn.Close()

	rsp, err := client.QuerySpecificICMPLengthTrigger(ctx, &tpb.QuerySpecificICMPLengthTriggerParams{Length: int32(length)})
	if err != nil {
		return nil, err
	}
	if len(rsp.Notifications) > 0 {
		return rsp.Notifications[0], nil
	}
	return nil, utils.Errorf("empty result (icmp length trigger[%v])", length)
}
