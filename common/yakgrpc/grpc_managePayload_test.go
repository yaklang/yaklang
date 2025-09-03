package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func _findIDByPayloadContent(payloads []*ypb.Payload, content string) int64 {
	for _, p := range payloads {
		if p.Content == content {
			return p.Id
		}
	}
	return -1
}

func _getPayloadFromYpbPayloads(payloads []*ypb.Payload) string {
	l := lo.Map(payloads, func(p *ypb.Payload, index int) string {
		return p.Content
	})
	sort.Strings(l)
	return strings.Join(l, "\n")
}

func convertPayloadGroupToDatabase(local ypb.YakClient, t *testing.T, group string) {
	client, err := local.ConvertPayloadGroupToDatabase(context.Background(), &ypb.NameRequest{
		Name: group,
	})
	if err != nil {
		t.Fatal(err)
	}

	for {
		_, err := client.Recv()
		if err != nil {
			t.Log(err)
			break
		}
	}
}

func backUpOrCopyPayloads(local ypb.YakClient, t *testing.T, ids []int64, group, folder string, copy bool) {
	t.Helper()
	_, err := local.BackUpOrCopyPayloads(context.Background(), &ypb.BackUpOrCopyPayloadsRequest{
		Ids:    ids,
		Group:  group,
		Folder: folder,
		Copy:   copy,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func getAllPayloadGroup(local ypb.YakClient, t *testing.T) []*ypb.PayloadGroupNode {
	t.Helper()
	rsp, err := local.GetAllPayloadGroup(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	return rsp.Nodes
}

func updateAllPayloadGroup(local ypb.YakClient, t *testing.T, nodes []*ypb.PayloadGroupNode) {
	t.Helper()
	_, err := local.UpdateAllPayloadGroup(context.Background(), &ypb.UpdateAllPayloadGroupRequest{
		Nodes: nodes,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func renamePayloadGroup(local ypb.YakClient, t *testing.T, group, newGroup string) {
	t.Helper()
	_, err := local.RenamePayloadGroup(context.Background(), &ypb.RenameRequest{
		Name:    group,
		NewName: newGroup,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func deletePayload(local ypb.YakClient, t *testing.T, id int64) {
	t.Helper()
	_, err := local.DeletePayload(context.Background(), &ypb.DeletePayloadRequest{
		Id: id,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func deletePayloads(local ypb.YakClient, t *testing.T, ids []int64) {
	t.Helper()
	_, err := local.DeletePayload(context.Background(), &ypb.DeletePayloadRequest{
		Ids: ids,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func deleteGroup(local ypb.YakClient, t *testing.T, group string) {
	t.Helper()
	_, err := local.DeletePayloadByGroup(context.Background(), &ypb.DeletePayloadByGroupRequest{
		Group: group,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func save2database(local ypb.YakClient, t *testing.T, group, folder, data string, errorHandler ...func(*testing.T, error)) {
	// t.Helper()
	var (
		err    error
		client ypb.Yak_SavePayloadStreamClient
		ret    *ypb.SavePayloadProgress
	)

	rsp, err := local.SaveTextToTemporalFile(context.Background(), &ypb.SaveTextToTemporalFileRequest{
		Text: []byte(data),
	})
	if err != nil {
		t.Fatal(err)
	}
	fileName := rsp.FileName

	client, err = local.SavePayloadStream(context.Background(), &ypb.SavePayloadRequest{
		IsFile:  true,
		Group:   group,
		Folder:  folder,
		Content: "",
		FileName: []string{
			fileName,
		},
		IsNew: true,
	})

	for {
		ret, err = client.Recv()
		if err != nil {
			t.Log(err)
			break
		}
		t.Log(ret)
	}
	if len(errorHandler) > 0 {
		errorHandler[0](t, err)
	} else if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}

func save2file(local ypb.YakClient, t *testing.T, group, folder, data string, errorHandler ...func(*testing.T, error)) {
	t.Helper()
	var (
		err    error
		client ypb.Yak_SavePayloadToFileStreamClient
		ret    *ypb.SavePayloadProgress
	)

	rsp, err := local.SaveTextToTemporalFile(context.Background(), &ypb.SaveTextToTemporalFileRequest{
		Text: []byte(data),
	})
	if err != nil {
		t.Fatal(err)
	}
	fileName := rsp.FileName

	client, err = local.SavePayloadToFileStream(context.Background(), &ypb.SavePayloadRequest{
		IsFile:  true,
		Group:   group,
		Folder:  "",
		Content: "",
		FileName: []string{
			fileName,
		},
		IsNew: true,
	})

	for {
		ret, err = client.Recv()
		if err != nil {
			break
		}
		t.Log(ret)
	}
	if len(errorHandler) > 0 {
		errorHandler[0](t, err)
	} else if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}

func save2LargeFile(local ypb.YakClient, t *testing.T, ctx context.Context, group, folder, filename string, errorHandler ...func(*testing.T, error)) {
	t.Helper()
	var (
		err    error
		client ypb.Yak_SavePayloadToFileStreamClient
		ret    *ypb.SavePayloadProgress
	)
	client, err = local.SaveLargePayloadToFileStream(ctx, &ypb.SavePayloadRequest{
		IsFile:  true,
		Group:   group,
		Folder:  "",
		Content: "",
		FileName: []string{
			filename,
		},
		IsNew: true,
	})

	for {
		ret, err = client.Recv()
		if err != nil {
			break
		}
		t.Log(ret)
	}
	if len(errorHandler) > 0 {
		errorHandler[0](t, err)
	} else if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}

func updatePayload(local ypb.YakClient, t *testing.T, id int64, group, content string) {
	t.Helper()
	_, err := local.UpdatePayload(context.Background(), &ypb.UpdatePayloadRequest{
		Id: id,
		Data: &ypb.Payload{
			Id:           id,
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
	t.Helper()
	rsp, err := local.GetAllPayloadGroup(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(rsp)
	return rsp.Nodes
}

func updateGroup(local ypb.YakClient, t *testing.T) {
	t.Helper()
	nodes := getGroup(local, t)
	local.UpdateAllPayloadGroup(context.Background(), &ypb.UpdateAllPayloadGroupRequest{
		Nodes: nodes,
	})
}

func createPayloadFolder(local ypb.YakClient, t *testing.T, folder string) {
	t.Helper()
	_, err := local.CreatePayloadFolder(context.Background(), &ypb.NameRequest{
		Name: folder,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func renamePayloadFolder(local ypb.YakClient, t *testing.T, folder, newFolder string) {
	t.Helper()
	_, err := local.RenamePayloadFolder(context.Background(), &ypb.RenameRequest{
		Name:    folder,
		NewName: newFolder,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func deletePayloadFolder(local ypb.YakClient, t *testing.T, folder string) {
	t.Helper()
	_, err := local.DeletePayloadByFolder(context.Background(), &ypb.NameRequest{
		Name: folder,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func queryFromDatabase(local ypb.YakClient, t *testing.T, group, folder string) *ypb.QueryPayloadResponse {
	t.Helper()
	rsp, err := local.QueryPayload(context.Background(), &ypb.QueryPayloadRequest{
		Pagination: &ypb.Paging{},
		Group:      group,
		Keyword:    "",
		Folder:     folder,
	})
	if err != nil {
		t.Fatal(err)
	}
	return rsp
}

func queryFromFile(local ypb.YakClient, t *testing.T, group, folder string) *ypb.QueryPayloadFromFileResponse {
	t.Helper()
	rsp, err := local.QueryPayloadFromFile(context.Background(), &ypb.QueryPayloadFromFileRequest{
		Group:  group,
		Folder: folder,
	})
	if err != nil {
		t.Fatal(err)
	}
	return rsp
}

func exportPayloadFromFile(local ypb.YakClient, t *testing.T, group, folder, savePath string) string {
	t.Helper()
	client, err := local.ExportAllPayloadFromFile(context.Background(), &ypb.GetAllPayloadRequest{
		Group:    group,
		Folder:   folder,
		SavePath: savePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	for {
		re, err := client.Recv()
		if err != nil {
			t.Log("get payload from file client error :", err)
			break
		}
		t.Log(re)
	}
	content, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func exportPayload(local ypb.YakClient, t *testing.T, group, folder string, savePath string) string {
	t.Helper()
	client, err := local.ExportAllPayload(context.Background(), &ypb.GetAllPayloadRequest{
		Group:    group,
		Folder:   folder,
		SavePath: savePath,
	})
	if err != nil {
		t.Fatal(err)
	}

	for {
		re, err := client.Recv()
		if err != nil {
			t.Log("get payload from file client error :", err)
			break
		}
		t.Log(re)
	}

	content, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func updateToFile(local ypb.YakClient, t *testing.T, group, data string) {
	t.Helper()
	_, err := local.UpdatePayloadToFile(context.Background(), &ypb.UpdatePayloadToFileRequest{
		GroupName: group,
		Content:   data,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func removeDuplicatePayloads(local ypb.YakClient, t *testing.T, group string) {
	t.Helper()
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
	t.Helper()
	for _, n := range node {
		if n.Name == name {
			if n.Type != typ {
				t.Fatalf("group %s type error : want(%s) vs got(%s)", name, typ, n.Type)
			}
		}
	}
}

func comparePayloadByGroupFolder(local ypb.YakClient, group, folder string, want string, t *testing.T) {
	t.Helper()
	rsp := queryFromDatabase(local, t, group, folder)
	got := _getPayloadFromYpbPayloads(rsp.Data)
	comparePayload(got, want, t)
}

func comparePayload(got, want string, t *testing.T) {
	t.Helper()
	got = strings.TrimSpace(strings.ReplaceAll(got, "\r", ""))
	want = strings.TrimSpace(strings.ReplaceAll(want, "\r", ""))
	wantL := strings.Split(want, "\n")
	wantL = lo.Filter(wantL, func(item string, index int) bool { return item != "" })
	gotL := strings.Split(got, "\n")
	gotL = lo.Filter(gotL, func(item string, index int) bool { return item != "" })

	if len(gotL) != len(wantL) {
		t.Fatalf("compare length error : want(%v) vs got(%v)", wantL, gotL)
	}
	for i := range gotL {
		if gotL[i] != wantL[i] {
			t.Fatalf("compare error : want(%s) vs got(%s)", wantL[i], gotL[i])
		}
	}
}

func generateLargePayloadFile(lines int) (filename string, clean func(), err error) {
	fd, err := os.CreateTemp("", "large_payload_file")
	if err != nil {
		return "", nil, err
	}

	for i := 0; i < lines; i++ {
		fd.WriteString(utils.RandAlphaNumStringBytes(16) + "\n")
	}

	return fd.Name(), func() {
		fd.Close()
		os.Remove(fd.Name())
	}, nil
}

func TestLargePayload(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)
	filename, clean, err := generateLargePayloadFile(1e6)
	defer clean()
	require.NoError(t, err)
	group := uuid.NewString()
	ctx := utils.TimeoutContextSeconds(20)
	save2LargeFile(local, t, ctx, group, "", filename)

	rsp := queryFromFile(local, t, group, "")
	require.True(t, rsp.IsBigFile)
	// t.Logf("big file size: %d", len(rsp.Data))
	// t.Logf("big file content:\n%s", rsp.Data[:])
}

func TestPayload(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("DataBase_CRUD", func(t *testing.T) {
		group := uuid.NewString()
		newGroup := uuid.NewString()
		want := "asd\nqwe\nzxc\n"

		// save database
		save2database(local, t, group, "", want)
		defer func() {
			// delete group
			deleteGroup(local, t, newGroup)
			rsp := queryFromDatabase(local, t, newGroup, "")
			if len(rsp.Data) != 0 {
				t.Fatal("after delete,group should be empty")
			}
		}()
		// rename group
		renamePayloadGroup(local, t, group, newGroup)
		group = newGroup

		// query database
		rsp := queryFromDatabase(local, t, group, "")

		// compare payload
		got := _getPayloadFromYpbPayloads(rsp.Data)
		comparePayload(got, want, t)

		// update payload
		for i, p := range rsp.Data {
			updatePayload(local, t, p.Id, group, fmt.Sprint(i))
		}
		// query database
		want = "0\n1\n2\n"
		rsp = queryFromDatabase(local, t, group, "")

		// compare payload
		got = _getPayloadFromYpbPayloads(rsp.Data)
		comparePayload(want, got, t)

		// delete single payload
		deletePayload(local, t, _findIDByPayloadContent(rsp.Data, "0"))

		// query database
		want = "1\n2\n"
		rsp = queryFromDatabase(local, t, group, "")

		// compare payload
		got = _getPayloadFromYpbPayloads(rsp.Data)
		comparePayload(got, want, t)

		// delete multi payload
		deletePayloads(local, t, []int64{_findIDByPayloadContent(rsp.Data, "1")})

		// query database
		want = "2\n"
		rsp = queryFromDatabase(local, t, group, "")

		// compare payload
		got = _getPayloadFromYpbPayloads(rsp.Data)
		comparePayload(got, want, t)
	})

	t.Run("File_CRUD", func(t *testing.T) {
		group := uuid.NewString()
		bigGroup := uuid.NewString()
		want := "asd\nqwe\nzxc"

		// save file
		save2file(local, t, group, "", want)
		save2file(local, t, bigGroup, "", strings.Repeat("123\n456\n", 10000))
		// delete group
		defer deleteGroup(local, t, group)

		// query file
		rsp := queryFromFile(local, t, group, "")

		// compare payload
		got := string(rsp.Data)
		comparePayload(got, want, t)

		// update(append) file
		updateToFile(local, t, group, "123\n456\n456\n")
		want = "123\n456\n456\n"

		// query file
		rsp = queryFromFile(local, t, group, "")

		// compare payload
		got = string(rsp.Data)
		comparePayload(got, want, t)

		// remove duplicate
		removeDuplicatePayloads(local, t, group)
		want = "123\n456\n"

		// query file
		rsp = queryFromFile(local, t, group, "")

		// compare payload
		got = string(rsp.Data)
		comparePayload(got, want, t)

		// remove duplicate big
		removeDuplicatePayloads(local, t, bigGroup)
		want = "123\n456\n"

		// query file
		rsp = queryFromFile(local, t, group, "")

		// compare payload
		got = string(rsp.Data)
		comparePayload(got, want, t)
	})

	t.Run("Folder_CRUD", func(t *testing.T) {
		folder, newFolder := uuid.NewString(), uuid.NewString()
		group := uuid.NewString()
		// create folder
		createPayloadFolder(local, t, folder)

		defer func() {
			// delete folder
			deletePayloadFolder(local, t, newFolder)
			rsp := queryFromDatabase(local, t, group, folder)
			if len(rsp.Data) != 0 {
				t.Fatal("after delete,group should be empty")
			}
		}()

		// save to folder
		want := "asd\nqwe\nzxc\n"
		save2database(local, t, group, folder, want)

		// query payload
		rsp := queryFromDatabase(local, t, group, folder)

		// compare payload
		got := _getPayloadFromYpbPayloads(rsp.Data)
		comparePayload(got, want, t)

		// rename folder
		renamePayloadFolder(local, t, folder, newFolder)
		folder = newFolder

		// query payload
		rsp = queryFromDatabase(local, t, group, folder)

		// compare payload
		got = _getPayloadFromYpbPayloads(rsp.Data)
		comparePayload(got, want, t)
	})

	t.Run("GetAndUpdatePayloadGroup", func(t *testing.T) {
		data := "123\n456\n"
		group1, group2 := uuid.NewString(), uuid.NewString()
		// create 2 group
		save2database(local, t, group1, "", data)
		save2database(local, t, group2, "", data)

		defer func() {
			// delete group
			deleteGroup(local, t, group1)
			deleteGroup(local, t, group2)
		}()

		// get nodes
		nodes := getAllPayloadGroup(local, t)

		// find index and swap
		id1, id2 := 0, 0
		for index, node := range nodes {
			if node.Name == group1 {
				id1 = index
			} else if node.Name == group2 {
				id2 = index
			}
		}
		nodes1, nodes2 := nodes[id1], nodes[id2]
		nodes[id1], nodes[id2] = nodes[id2], nodes[id1]

		// update nodes
		updateAllPayloadGroup(local, t, nodes)

		// get new nodes
		newNodes := getAllPayloadGroup(local, t)

		// check
		newNodes1, newNodes2 := newNodes[id2], newNodes[id1]

		if newNodes1.Name != nodes1.Name || newNodes1.Type != nodes1.Type {
			t.Fatalf("swap group error: want(%v) vs got(%v)", nodes1, newNodes1)
		}
		if newNodes2.Name != nodes2.Name || newNodes2.Type != nodes2.Type {
			t.Fatalf("swap group error: want(%v) vs got(%v)", nodes2, newNodes2)
		}
	})

	t.Run("BackUpOrCopyPayloads", func(t *testing.T) {
		data1, data2, data3 := "123\n456\n", "qwe\nasd\n", "zxc\n"
		group1, group2, group3 := uuid.NewString(), uuid.NewString(), uuid.NewString()
		// create 3 group
		save2database(local, t, group1, "", data1)
		save2database(local, t, group2, "", data2)
		save2database(local, t, group3, "", data3)

		defer func() {
			// delete group
			deleteGroup(local, t, group2)
			deleteGroup(local, t, group3)
		}()

		// query group1 payload
		rsp := queryFromDatabase(local, t, group1, "")
		ids := lo.Map(rsp.Data, func(p *ypb.Payload, index int) int64 {
			return p.Id
		})

		// copy group1 payload to group2
		backUpOrCopyPayloads(local, t, ids, group2, "", true)

		// query group2 payload
		rsp = queryFromDatabase(local, t, group2, "")
		want := "123\n456\nasd\nqwe\n"

		// compare payload
		got := _getPayloadFromYpbPayloads(rsp.Data)
		comparePayload(got, want, t)

		// move group1 payload to group3
		backUpOrCopyPayloads(local, t, ids, group3, "", false)

		// query group3 payload
		rsp = queryFromDatabase(local, t, group3, "")
		want = "123\n456\nzxc\n"

		// compare payload
		got = _getPayloadFromYpbPayloads(rsp.Data)
		comparePayload(got, want, t)

		// query group1 payload
		rsp = queryFromDatabase(local, t, group1, "")
		if len(rsp.Data) != 0 {
			t.Fatal("after move, group1 should be empty")
		}
	})

	t.Run("ExportPayload", func(t *testing.T) {
		data := "123\n456\n"
		group1, group2 := uuid.NewString(), uuid.NewString()
		// save database
		save2database(local, t, group1, "", data)
		// save file
		save2file(local, t, group2, "", data)
		// create tempfile
		f, err := os.CreateTemp("", "temp-payload")
		if err != nil {
			t.Fatal(err)
		}
		f2, err := os.CreateTemp("", "temp-payload")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			// delete group
			deleteGroup(local, t, group1)
			deleteGroup(local, t, group2)
			os.Remove(f.Name())
			os.Remove(f2.Name())
		}()

		// export group1 payload
		got1 := exportPayload(local, t, group1, "", f.Name())
		got2 := exportPayloadFromFile(local, t, group2, "", f2.Name())

		// compare payload
		comparePayload(got1, data, t)
		comparePayload(got2, data, t)
	})

	t.Run("ConvertPayloadGroupToDatabase", func(t *testing.T) {
		data := "123\n456\n"
		group := uuid.NewString()
		// save file
		save2file(local, t, group, "", data)
		defer deleteGroup(local, t, group) // delete group

		// convert to database
		convertPayloadGroupToDatabase(local, t, group)

		// query payload
		rsp := queryFromDatabase(local, t, group, "")
		if len(rsp.Data) != 2 {
			t.Fatalf("convert file to database error, want 2 but got %d", len(rsp.Data))
		}
		for _, p := range rsp.Data {
			if p.IsFile {
				t.Fatal("convert file to database error")
			}
		}
	})
	t.Run("UniqueHash", func(t *testing.T) {
		data := "123\n456\n"
		group := uuid.NewString()
		// save twice
		save2database(local, t, group, "", data)
		save2database(local, t, group, "", data, func(t *testing.T, err error) {
			if err == nil {
				t.Fatal("expect error but got nil")
			} else {
				t.Log(err)
			}
		})
		defer deleteGroup(local, t, group) // delete group

		// query payload
		rsp := queryFromDatabase(local, t, group, "")
		if len(rsp.Data) != 2 {
			t.Fatalf("unique hash error, want 2 but got %d", len(rsp.Data))
		}
	})

	t.Run("SaveEmptyFile", func(t *testing.T) {
		group1, group2 := uuid.NewString(), uuid.NewString()
		// save to database and file
		save2database(local, t, group1, "", "", func(t *testing.T, err error) {
			if err == nil {
				t.Fatal("expect error but got nil")
			} else {
				t.Log(err)
			}
		})
		save2file(local, t, group2, "", "", func(t *testing.T, err error) {
			if err == nil {
				t.Fatal("expect error but got nil")
			} else {
				t.Log(err)
			}
		})
	})

	t.Run("FIX-BackupOrMovePayloads", func(t *testing.T) {
		// FIX:
		// same payload backup or move to same group will cause error

		group1, group2 := uuid.NewString(), uuid.NewString()
		want := "123\n456\n"
		// save to database
		save2database(local, t, group1, "", want)
		save2database(local, t, group2, "", want)
		defer func() {
			deleteGroup(local, t, group1)
			// deleteGroup(local, t, group2)
		}()
		rsp := queryFromDatabase(local, t, group2, "")

		ids := lo.Map(rsp.Data, func(p *ypb.Payload, index int) int64 {
			return p.Id
		})

		// copy
		backUpOrCopyPayloads(local, t, ids, group1, "", true)
		// group1 should still have 2 payload
		comparePayloadByGroupFolder(local, group1, "", want, t)
		// group2 should still have 2 payload
		comparePayloadByGroupFolder(local, group2, "", want, t)

		// move
		backUpOrCopyPayloads(local, t, ids, group1, "", false)
		// group1 should still have 2 payload
		comparePayloadByGroupFolder(local, group1, "", want, t)
		// group2 should be empty
		comparePayloadByGroupFolder(local, group2, "", "", t)
	})

	t.Run("Trim-Left", func(t *testing.T) {
		want := "  xxx\n xxx\nxxx\n"
		group1, group2 := uuid.NewString(), uuid.NewString()
		// database
		save2database(local, t, group1, "", want)
		defer deleteGroup(local, t, group1) // delete group
		rsp := queryFromDatabase(local, t, group1, "")
		got := _getPayloadFromYpbPayloads(rsp.Data)
		comparePayload(got, want, t)
		// file
		save2file(local, t, group2, "", want)
		defer deleteGroup(local, t, group2) // delete group

		rsp2 := queryFromFile(local, t, group2, "")
		got = string(rsp2.Data)
		comparePayload(got, want, t)
	})

	t.Run("ExportPayloadBatch", func(t *testing.T) {
		group1, group2 := uuid.NewString(), uuid.NewString()
		data1 := "payload1\npayload2"
		data2 := "payload3\npayload4"

		save2database(local, t, group1, "", data1)
		save2database(local, t, group2, "", data2)
		defer func() {
			deleteGroup(local, t, group1)
			deleteGroup(local, t, group2)
		}()

		// 创建临时目录
		dir, err := os.MkdirTemp("", "export-dir")
		if err != nil {
			t.Fatal("create temp dir failed:", err)
		}
		defer os.RemoveAll(dir)

		// 执行导出操作
		streamCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stream, err := local.ExportPayloadBatch(streamCtx, &ypb.ExportPayloadBatchRequest{
			Group:    fmt.Sprintf("%s,%s", group1, group2),
			SavePath: dir,
		})
		if err != nil {
			t.Fatal("export payload failed:", err)
		}

		// 验证进度报告
		var progresses []float64
		for {
			res, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal("stream recv error:", err)
			}
			progresses = append(progresses, res.Progress)
		}

		if len(progresses) < 2 {
			t.Fatal("progress events too few, expected at least 2")
		}
		if progresses[len(progresses)-1] != 1.0 {
			t.Fatalf("final progress not 1.0, got: %f", progresses[len(progresses)-1])
		}

		// 验证生成的文件
		expectedFiles := []string{
			filepath.Join(dir, group1+".csv"),
			filepath.Join(dir, group2+".csv"),
		}

		for _, file := range expectedFiles {
			if _, err := os.Stat(file); os.IsNotExist(err) {
				t.Fatalf("file not created: %s", file)
			}
		}

		// 验证文件内容
		testCases := []struct {
			file    string
			content string
			expect  []string
		}{
			{
				file:    expectedFiles[0],
				content: data1,
				expect:  []string{"content,hit_count", "payload1,0", "payload2,0"},
			},
			{
				file:    expectedFiles[1],
				content: data2,
				expect:  []string{"content,hit_count", "payload3,0", "payload4,0"},
			},
		}

		for _, tc := range testCases {
			contentBytes, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatal("read file failed:", err)
			}

			lines := strings.Split(strings.TrimSpace(string(contentBytes)), "\n")
			if len(lines) != len(tc.expect) {
				t.Fatalf("line count mismatch in %s: expected %d, got %d",
					tc.file, len(tc.expect), len(lines))
			}

			for i, line := range lines {
				if line != tc.expect[i] {
					t.Fatalf("content mismatch in %s line %d:\nExpect: %q\nGot:    %q",
						tc.file, i+1, tc.expect[i], line)
				}
			}
		}
	})

	t.Run("ExportBatchPayload", func(t *testing.T) {
		data := "123\n456\n"
		// 文件型 group
		groupFile1, groupFile2 := uuid.NewString(), uuid.NewString()
		save2file(local, t, groupFile1, "", data)
		save2file(local, t, groupFile2, "", data)

		// 数据库型 group
		groupDB1, groupDB2 := uuid.NewString(), uuid.NewString()
		save2database(local, t, groupDB1, "", data)
		save2database(local, t, groupDB2, "", data)

		saveDir, err := os.MkdirTemp("", "temp-payload")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			deleteGroup(local, t, groupFile1)
			deleteGroup(local, t, groupFile2)
			deleteGroup(local, t, groupDB1)
			deleteGroup(local, t, groupDB2)
			os.RemoveAll(saveDir)
		}()

		groups := []string{groupFile1, groupFile2, groupDB1, groupDB2}
		results := exportBatchPayload(local, t, groups, saveDir)

		// 校验文件型 group
		for _, g := range []string{groupFile1, groupFile2} {
			content, ok := results[g+".txt"]
			if !ok {
				t.Fatalf("expected txt result for group %s", g)
			}
			comparePayload(content, data, t)
		}

		// 校验数据库型 group
		for _, g := range []string{groupDB1, groupDB2} {
			content, ok := results[g+".csv"]
			if !ok {
				t.Fatalf("expected csv result for group %s", g)
			}
			// csv 应该有 header + 数据
			lines := strings.Split(strings.TrimSpace(content), "\n")
			if len(lines) != 1+len(strings.Split(strings.TrimSpace(data), "\n")) {
				t.Fatalf("unexpected csv line count for group %s: got %d", g, len(lines))
			}
			if lines[0] != "content,hit_count" {
				t.Fatalf("expected csv header in group %s, got %s", g, lines[0])
			}
		}
	})

}

func exportBatchPayload(local ypb.YakClient, t *testing.T, groups []string, saveDir string) map[string]string {
	t.Helper()
	client, err := local.ExportBatchPayload(context.Background(), &ypb.ExportBatchPayloadRequest{
		Groups:   groups,
		SavePath: saveDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	for {
		re, err := client.Recv()
		if err != nil {
			t.Log("batch export stream end:", err)
			break
		}
		t.Log("progress:", re.Progress)
	}

	// 读取导出结果
	results := make(map[string]string)
	for _, g := range groups {
		// 文件型 group → txt，数据库型 group → csv
		txtPath := filepath.Join(saveDir, fmt.Sprintf("%s.txt", g))
		csvPath := filepath.Join(saveDir, fmt.Sprintf("%s.csv", g))

		if _, err := os.Stat(txtPath); err == nil {
			content, _ := os.ReadFile(txtPath)
			results[g+".txt"] = string(content)
		} else if _, err := os.Stat(csvPath); err == nil {
			content, _ := os.ReadFile(csvPath)
			results[g+".csv"] = string(content)
		} else {
			t.Fatalf("no exported file found for group %s", g)
		}
	}
	return results
}
