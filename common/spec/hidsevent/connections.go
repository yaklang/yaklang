package hidsevent

type ConnectionStat struct {
	Fd     uint32  `json:"fd"`
	Family string  `json:"family"`
	Type   string  `json:"type"`
	Laddr  string  `json:"localaddr"`
	Raddr  string  `json:"remoteaddr"`
	Status string  `json:"status"`
	Uids   []int32 `json:"uids"`
	Pid    int32   `json:"pid"`

	//ProcessName string
}

type ConnectionInfo struct {
	Count            int
	EstablishedConns []*ConnectionStat `json:"established_conns"`
	ListenedConns    []*ConnectionStat `json:"listened_conns"`
	ExtraConns       []*ConnectionStat `json:"extra_conns"`
}

type ConnectionEventType string

const (
	ConnectionEventType_New       ConnectionEventType = "new"
	ConnectionEventType_Disappear ConnectionEventType = "disappear"
)

type ConnectionEvent struct {
	EventName  ConnectionEventType `json:"event_name"`
	Connection *ConnectionStat
}

//func GetNetConnectStatusInfo(c context.Context) (*ConnectionInfo, error) {
//	//var cachePname = map[int32]string{}
//
//	i := &ConnectionInfo{}
//
//	connStat, err := net.ConnectionsWithContext(c, "tcp")
//	if err != nil {
//		return nil, errors.Errorf("get conns info failed: %v", err)
//	}
//
//	// only report data filter on server
//	for _, statOrigin := range connStat {
//		//var procName string
//		//var ok bool
//		//if procName, ok = cachePname[statOrigin.Pid]; !ok {
//		//	if proc, err := process.NewProcess(statOrigin.Pid); err != nil {
//		//		procName = ""
//		//	} else {
//		//		procName, _ = proc.Name()
//		//		cachePname[statOrigin.Pid] = procName
//		//	}
//		//}
//
//		stat := &ConnectionStat{
//			ConnectionStat: statOrigin,
//			//ProcessName:    procName,
//		}
//
//		switch strings.ToUpper(stat.Status) {
//		case "ESTABLISHED":
//			i.EstablishedConns = append(i.EstablishedConns, stat)
//			i.Count = i.Count + 1
//		case "LISTEN":
//			i.ListenedConns = append(i.ListenedConns, stat)
//			i.Count = i.Count + 1
//		default:
//			i.ExtraConns = append(i.ExtraConns, stat)
//			i.Count = i.Count + 1
//		}
//	}
//	return i, nil
//}
