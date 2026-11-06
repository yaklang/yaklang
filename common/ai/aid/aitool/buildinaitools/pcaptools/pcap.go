package pcaptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
)

type captureRunner func(context.Context, *pcapdb.InstanceManager, pcapdb.LiveCaptureConfig, func(pcapdb.PCAPFileDBMetadata)) (*pcapdb.PCAPFileDBMetadata, error)

func CreateTools() []*aitool.Tool {
	return createTools(pcapdb.DefaultManager, func(ctx context.Context, m *pcapdb.InstanceManager, c pcapdb.LiveCaptureConfig, started func(pcapdb.PCAPFileDBMetadata)) (*pcapdb.PCAPFileDBMetadata, error) {
		return m.RunLiveCapture(ctx, c, started)
	})
}

func captureResult(meta *pcapdb.PCAPFileDBMetadata) any {
	if meta == nil {
		return nil
	}
	return map[string]any{"dataset_id": meta.DatasetID, "packet_count": meta.PacketCount, "protocol_count": meta.ProtocolCount, "session_count": meta.SessionCount, "stream_count": meta.StreamCount, "last_error": meta.LastError}
}

func createTools(factory func() (*pcapdb.InstanceManager, error), run captureRunner) []*aitool.Tool {
	list := aitool.NewWithoutCallback("list_pcap_interfaces",
		aitool.WithDescription("List local capture interfaces, including devices without IP addresses. Use the returned name as interface for start_pcap_capture; opening a device still requires libpcap/Npcap and capture permissions."),
		aitool.WithVerboseNameZh("列出抓包网卡"), aitool.WithKeywords([]string{"pcap", "capture", "抓包", "网卡"}),
		aitool.WithCallback(func(ctx context.Context, _ aitool.InvokeParams, _ *aitool.ToolRuntimeConfig, _ io.Writer, _ io.Writer) (any, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			devices, err := pcap.FindAllDevs()
			if err != nil {
				return nil, err
			}
			items := make([]map[string]any, 0, len(devices))
			for _, device := range devices {
				addresses := make([]string, 0, len(device.Addresses))
				for _, address := range device.Addresses {
					if address.IP != nil {
						addresses = append(addresses, address.IP.String())
					}
				}
				items = append(items, map[string]any{"name": device.Name, "description": device.Description, "addresses": addresses})
			}
			return map[string]any{"interfaces": items}, nil
		}),
	)
	start := aitool.NewWithoutCallback("start_pcap_capture",
		aitool.WithDescription("Continuously capture one local interface into a persistent pcapdb dataset, with protocol/session/stream analysis. Runs asynchronously by default. Inspect execution stdout for the startup JSON containing dataset_id, then use query_pcapdb with that id while capture is running. No default duration or packet limit."),
		aitool.WithVerboseNameZh("启动持续抓包"), aitool.WithKeywords([]string{"pcap", "pcapdb", "capture", "实时流量分析", "抓包"}),
		aitool.WithDefaultAsync(true),
		aitool.WithUsage("First list_pcap_interfaces and choose one interface. Async submission returns execute_id; inspect through manage_async_execution_runtime until stdout contains the startup JSON with dataset_id. Query that dataset using query_pcapdb (metadata/protocols/packets/sessions/streams/details). An empty page does not mean capture ended. To stop, use manage_async_execution_runtime stop, then wait. Stop retains the same dataset and evidence IDs. Capture belongs to the current task and stops when that task ends. Submission alone does not prove startup succeeded."),
		aitool.WithStringParam("interface", aitool.WithParam_Required(true), aitool.WithParam_Description("One capture device name returned by list_pcap_interfaces")),
		aitool.WithStringParam("bpf", aitool.WithParam_Description("Optional capture BPF, for example tcp port 80")),
		aitool.WithIntegerParam("snaplen", aitool.WithParam_Default(65535), aitool.WithParam_Min(1), aitool.WithParam_Max(16<<20)),
		aitool.WithBoolParam("promisc", aitool.WithParam_Default(false)),
		aitool.WithNumberParam("duration_seconds", aitool.WithParam_Min(0), aitool.WithParam_Description("Optional duration; omitted or zero captures until stopped")),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, _ *aitool.ToolRuntimeConfig, stdout io.Writer, _ io.Writer) (any, error) {
			seconds := params.GetFloat("duration_seconds")
			if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > float64(math.MaxInt64)/float64(time.Second) {
				return nil, fmt.Errorf("invalid duration_seconds")
			}
			config := pcapdb.LiveCaptureConfig{Interface: params.GetString("interface"), BPF: params.GetString("bpf"), Snaplen: params.GetInteger("snaplen", 65535), Promisc: params.GetBool("promisc"), Duration: time.Duration(seconds * float64(time.Second))}
			if config.Interface == "" {
				return nil, fmt.Errorf("interface is required; use list_pcap_interfaces")
			}
			m, err := factory()
			if err != nil {
				return nil, err
			}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			var publishErr error
			meta, err := run(ctx, m, config, func(meta pcapdb.PCAPFileDBMetadata) {
				publishErr = json.NewEncoder(stdout).Encode(map[string]any{"dataset_id": meta.DatasetID})
				if publishErr != nil {
					cancel()
				}
			})
			return captureResult(meta), errors.Join(err, publishErr)
		}),
	)
	return []*aitool.Tool{list, start}
}
