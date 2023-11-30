package yakgrpc

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func deleteGroup(local ypb.YakClient, t *testing.T, group string) {
	_, err := local.DeletePayloadByGroup(context.Background(), &ypb.NameRequest{
		Name: group,
	})
	if err != nil {
		t.Fatal(err)
	}
}
func save2database(local ypb.YakClient, t *testing.T, group, data string) {
	rsp, err := local.SaveTextToTemporalFile(context.Background(), &ypb.SaveTextToTemporalFileRequest{
		Text: []byte(data),
	})
	if err != nil {
		t.Fatal(err)
	}
	fileName := rsp.FileName

	client, err := local.SavePayloadStream(context.Background(), &ypb.SavePayloadRequest{
		IsFile:  true,
		Group:   group,
		Folder:  "",
		Content: "",
		FileName: []string{
			fileName,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for {
		re, err := client.Recv()
		if err != nil {
			t.Log(err)
			break
		}
		t.Log(re)
	}
}

func save2file(local ypb.YakClient, t *testing.T, group, data string) {
	rsp, err := local.SaveTextToTemporalFile(context.Background(), &ypb.SaveTextToTemporalFileRequest{
		Text: []byte(data),
	})
	if err != nil {
		t.Fatal(err)
	}
	fileName := rsp.FileName

	client, err := local.SavePayloadToFileStream(context.Background(), &ypb.SavePayloadRequest{
		IsFile:  true,
		Group:   group,
		Folder:  "",
		Content: "",
		FileName: []string{
			fileName,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for {
		re, err := client.Recv()
		if err != nil {
			break
		}
		t.Log(re)
	}
}

func updatePayload(local ypb.YakClient, t *testing.T, id int, group, content string) {
	_, err := local.UpdatePayload(context.Background(), &ypb.UpdatePayloadRequest{
		Id: int64(id),
		Data: &ypb.Payload{
			Id:           int64(id),
			Group:        group,
			ContentBytes: []byte{},
			Content:      content,
			Folder:       "",
			HitCount:     0,
			IsFile:       false,
		},
	})

	if err != nil {
		t.Fatal(err)
	}

}

func getGroup(local ypb.YakClient, t *testing.T) []*ypb.PayloadGroupNode {
	rsp, err := local.GetAllPayloadGroup(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(rsp)
	return rsp.Nodes
}

func UpdateGroup(local ypb.YakClient, t *testing.T) {
	nodes := getGroup(local, t)
	local.UpdateAllPayloadGroup(context.Background(), &ypb.UpdateAllPayloadGroupRequest{
		Nodes: nodes,
	})
}

func queryFromDatabase(local ypb.YakClient, t *testing.T, group string) string {
	rsp, err := local.QueryPayload(context.Background(), &ypb.QueryPayloadRequest{
		Pagination: &ypb.Paging{},
		Group:      group,
		Keyword:    "",
		Folder:     "",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(rsp)
	return ""

}
func queryFromFile(local ypb.YakClient, t *testing.T, group string) string {
	rsp, err := local.QueryPayloadFromFile(context.Background(), &ypb.QueryPayloadFromFileRequest{
		Group:  group,
		Folder: "",
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Log(rsp)
	return string(rsp.Data)
}

func getPayloadFromFile(local ypb.YakClient, t *testing.T, group string) string {
	client, err := local.GetAllPayloadFromFile(context.Background(), &ypb.GetAllPayloadRequest{
		Group:  group,
		Folder: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	str := ""
	for {
		re, err := client.Recv()
		if err != nil {
			t.Log("get payload from file client error :", err)
			break
		}
		t.Log(re)
		str += string(re.Data)
	}
	return str

}
func getPayload(local ypb.YakClient, t *testing.T, group string) (string, []int) {
	rsp, err := local.GetAllPayload(context.Background(), &ypb.GetAllPayloadRequest{
		Group:  group,
		Folder: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(rsp)
	data := lo.Map(rsp.Data, func(item *ypb.Payload, _ int) string { return item.Content })
	return strings.Join(data, "\r\n"), lo.Map(rsp.Data, func(item *ypb.Payload, _ int) int { return int(item.Id) })

}
func updateToFile(local ypb.YakClient, t *testing.T, group, data string) {
	_, err := local.UpdatePayloadToFile(context.Background(), &ypb.UpdatePayloadToFileRequest{
		GroupName: group,
		Content:   data,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func coverToDatabase(local ypb.YakClient, t *testing.T, group string) {
	stream, err := local.CoverPayloadGroupToDatabase(context.Background(), &ypb.NameRequest{
		Name: group,
	})

	if err != nil {
		t.Fatal(err)
	}

	for {
		re, err := stream.Recv()
		if err != nil {
			break
		}
		t.Log(re)
	}
}

func removeDuplicatePayloads(local ypb.YakClient, t *testing.T, group string) {
	stream, err := local.RemoveDuplicatePayloads(context.Background(), &ypb.NameRequest{
		Name: group,
	})

	if err != nil {
		t.Fatal(err)
	}

	for {
		re, err := stream.Recv()
		if err != nil {
			break
		}
		t.Log(re)
	}
}

func checkNode(node []*ypb.PayloadGroupNode, t *testing.T, name, typ string) {
	for _, n := range node {
		if n.Name == name {
			if n.Type != typ {
				t.Fatalf("group %s type error : want(%s) vs got(%s)", name, typ, n.Type)
			}
		}
	}
}

func comparePayload(got, want string, t *testing.T) {
	wantL := strings.Split(want, "\n")
	gotL := strings.Split(got, "\r\n")
	gotL = lo.Filter(gotL, func(item string, index int) bool { return item != "" })

	if len(gotL) != len(wantL) {
		t.Fatalf("compare length error : want(%d) vs got(%d)", len(wantL), len(gotL))
	}
	for i := range gotL {
		if gotL[i] != wantL[i] {
			t.Fatalf("compare error : want(%s) vs got(%s)", wantL[i], gotL[i])
		}
	}
}

func TestGroupInDatabase(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	UpdateGroup(local, t)

	t.Log("\n\n================= save payload ")
	data := "arsoten\nzxmccvmkzxcv\nqpqwfp;yuqywufnp;u\n12412341234\n123\n123"

	group := uuid.NewString()
	save2database(local, t, group, data)
	defer deleteGroup(local, t, group)

	nodes := getGroup(local, t)
	checkNode(nodes, t, group, "DataBase")
	if nodes[len(nodes)-1].Name != group {
		t.Fatalf("group %s should in last, node: %s", group, nodes)
	}

	got, _ := getPayload(local, t, group)
	data = "arsoten\nzxmccvmkzxcv\nqpqwfp;yuqywufnp;u\n12412341234\n123"
	comparePayload(got, data, t)

	t.Log("\n\n================= extern payload ")
	// // 扩充字典
	data2 := "zxcv\n123\n123\narsoten"
	save2database(local, t, group, data2)
	want := data + "\nzxcv"
	got, ids := getPayload(local, t, group)
	comparePayload(got, want, t)

	t.Log("\n\n================= update payload ")
	data3 := "zzzz"
	updatePayload(local, t, ids[0], group, data3)
	want = "zzzz\nzxmccvmkzxcv\nqpqwfp;yuqywufnp;u\n12412341234\n123\nzxcv"
	got, _ = getPayload(local, t, group)
	comparePayload(got, want, t)
}

// check : [save/query/getAllPayload/removeDuplicate/coverToDatabase]
func TestSavePayloadInFile(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	group := uuid.NewString()

	UpdateGroup(local, t)

	// save data to file
	data := "rsotein\naeiornst\nzxcveno"
	save2file(local, t, group, data)
	defer deleteGroup(local, t, group)

	// all group
	nodes := getGroup(local, t)
	checkNode(nodes, t, group, "File")
	if nodes[len(nodes)-1].Name != group {
		t.Fatalf("group %s should in last, node: %s", group, nodes)
	}

	// query date
	comparePayload(queryFromFile(local, t, group), data, t)
}

func TestUpdatePayloadInFile(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	group := uuid.NewString()

	// save data to file
	data := "rsotein\naeiornst\nzxcveno"
	save2file(local, t, group, data)
	defer deleteGroup(local, t, group)

	// update
	data = "12121212\n3333333\n112\n112"
	updateToFile(local, t, group, data)

	// check data
	comparePayload(queryFromFile(local, t, group), data, t)
	// remove duplicate
	removeDuplicatePayloads(local, t, group)
	// check
	dataRemoveDuplicate := "12121212\n3333333\n112"
	comparePayload(queryFromFile(local, t, group), dataRemoveDuplicate, t)

	t.Log("get all payload from file")
	getPayloadFromFile(local, t, group)

}
func TestCoverPayloadInFile(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	group := uuid.NewString()

	// save data to file
	data := "rsotein\naeiornst\nzxcveno"
	save2file(local, t, group, data)
	defer deleteGroup(local, t, group)

	orgNodes := getGroup(local, t)
	// cover to database
	coverToDatabase(local, t, group)

	// all group
	nodes := getGroup(local, t)
	checkNode(nodes, t, group, "DataBase")
	if len(nodes) != len(orgNodes) {
		t.Fatalf("nodes length error: got(%d) vs want(%d)", len(nodes), len(orgNodes))
	}
	for i := range nodes {
		if nodes[i].Name != orgNodes[i].Name {
			t.Fatalf("nodes length error: got(%v) vs want(%v)", nodes[i], orgNodes[i])
		}
	}
	got, _ := getPayload(local, t, group)
	comparePayload(got, data, t)
}
