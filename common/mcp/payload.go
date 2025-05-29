package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/go-viper/mapstructure/v2"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var NewLocalClient func(locals ...bool) (YakClientInterface, error)

func init() {
	NewLocalClient = func(locals ...bool) (YakClientInterface, error) {
		return nil, utils.Error("not register NewLocalClient")
	}
}

func RegisterNewLocalClient(f func(locals ...bool) (YakClientInterface, error)) {
	NewLocalClient = f
}

type YakClientInterface interface {
	ypb.YakClient
	GetProfileDatabase() *gorm.DB
}

var savePayloadToolOptions = []mcp.ToolOption{
	mcp.WithString("group",
		mcp.Description("Payload dictionary name"),
		mcp.Required(),
	),
	mcp.WithString("folder",
		mcp.Description("The folder where the payload should be saved, empty means root"),
	),
	mcp.WithBool("isNew",
		mcp.Description("Must be set to true if want to create a new payload dictionary"),
	),
	mcp.WithBool("saveAsFile",
		mcp.Description("Whether to save the payload as a file"),
	),
	mcp.WithOneOfStruct("source", []mcp.PropertyOption{
		mcp.Description("source from content or file"),
		mcp.Required(),
	}, []mcp.ToolOption{
		mcp.WithString("content",
			mcp.Description("The raw content of the payload. Can be multiple lines of content, with one payload per line."),
			mcp.Required(),
		),
	},
		[]mcp.ToolOption{
			mcp.WithStringArray("filename",
				mcp.Description("The filename(s) of the payload that want to import"),
				mcp.Required(),
			),
		},
	),
}

func init() {
	// query
	AddGlobalToolSet("payload",
		WithTool(mcp.NewTool("list_all_payload_dictionary_details",
			mcp.WithDescription("List all payload dictionary details, include current folder and sub-folder details, each detail include type(file/database/folder) and name"),
		), handleGetAllPayloadDictionaryDetails),
		WithTool(mcp.NewTool("query_payload",
			mcp.WithDescription(`Queries payload with flexible filters`),
			mcp.WithPaging("pagination",
				[]string{"id", "created_at", "updated_at", "deleted_at", "group", "folder", "group_index", "content", "hit_count", "is_file", "hash"},
				mcp.Description(`Pagination settings for the query. Only work for "database" type`),
				mcp.Required(),
			),
			mcp.WithString("keyword",
				mcp.Description(`Keyword to filter the payload. Only work for "database" type`),
			),
			mcp.WithString("group",
				mcp.Description("Payload group, also means dictionary name"),
				mcp.Required(),
			),
			mcp.WithString("folder",
				mcp.Description("Folder to filter the payload dictionary, empty means root"),
			),
		), handleQueryPayload),

		// create
		WithTool(mcp.NewTool("save_payload",
			append([]mcp.ToolOption{
				mcp.WithDescription("Save payload(s) to database"),
			}, savePayloadToolOptions...)...,
		), handleSavePayload),
		WithTool(mcp.NewTool("create_payload_folder",
			mcp.WithDescription("Create payload folder"),
			mcp.WithString("name",
				mcp.Description("The name of the folder"),
				mcp.Required(),
			),
		), handleCreatePayloadFolder),

		// delete
		WithTool(mcp.NewTool("delete_payload",
			mcp.WithDescription("Delete payload by group or folder"),
			mcp.WithString("group",
				mcp.Description("Payload group, also means dictionary name, if this is set, the folder parameter should be empty"),
			),
			mcp.WithString("folder",
				mcp.Description("Folder of the payload dictionary, empty means root, if this is set, the group parameter should be empty"),
			),
		), handleDeletePayload),

		// rename
		WithTool(mcp.NewTool("rename_payload_group",
			mcp.WithDescription("Rename payload group(dictionary name)"),
			mcp.WithString("name",
				mcp.Description("old payload dictionary name"),
				mcp.Required(),
			),
			mcp.WithString("newName",
				mcp.Description("new payload dictionary name"),
				mcp.Required(),
			),
		), handleRenamePayloadGroup),
		WithTool(mcp.NewTool("rename_payload_folder",
			mcp.WithDescription("Rename payload folder name"),
			mcp.WithString("name",
				mcp.Description("old folder name"),
				mcp.Required(),
			),
			mcp.WithString("newName",
				mcp.Description("new folder name"),
				mcp.Required(),
			),
		), handleRenamePayloadFolder),

		// update
		WithTool(mcp.NewTool("update_one_payload",
			mcp.WithDescription("Updates the one payload"),
			mcp.WithNumber("id",
				mcp.Description(`The ID of the payload to update`),
			),
			mcp.WithStruct("data",
				[]mcp.PropertyOption{
					mcp.Description("The payload data to update"),
					mcp.Required(),
				},
				mcp.WithString("group",
					mcp.Description("Payload group, also means dictionary name, empty means not update"),
				),
				mcp.WithString("content",
					mcp.Description("The content of the payload that want to saved"),
				),
				mcp.WithString("folder",
					mcp.Description("The folder of the payload"),
				),
				mcp.WithNumber("hitCount",
					mcp.Description("The hit count of the payload"),
				),
			),
		), handleUpdateOnePayload),

		WithTool(mcp.NewTool("update_payload_file_content",
			mcp.WithDescription(`Updates the all content for payload of "file" type`),
			mcp.WithString("groupName",
				mcp.Description("Payload group name, also means dictionary name"),
				mcp.Required(),
			),
			mcp.WithString("content",
				mcp.Description("The content of the payload that want to saved. Can be multiple lines of content, with one payload per line."),
			),
		), handleUpdatePayloadFileContent),
	)
}

func handleGetAllPayloadDictionaryDetails(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.Empty
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		rsp, err := s.grpcClient.GetAllPayloadGroup(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to get all payload dictionary details")
		}
		return NewCommonCallToolResult(rsp)
	}
}

func handleQueryPayload(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.QueryPayloadRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		payload, err := yakit.GetPayloadFirst(s.grpcClient.GetProfileDatabase(), req.Group)
		if err != nil {
			return nil, utils.Wrap(err, "failed to query payload first")
		}
		if payload.GetIsFile() {
			rsp, err := s.grpcClient.QueryPayloadFromFile(ctx, &ypb.QueryPayloadFromFileRequest{
				Group:  req.Group,
				Folder: req.Folder,
			})
			if err != nil {
				return nil, utils.Wrapf(err, "failed to query payload from file[%s]", payload.GetContent())
			}
			return NewCommonCallToolResult(rsp.Data)
		} else {
			rsp, err := s.grpcClient.QueryPayload(ctx, &req)
			if err != nil {
				return nil, utils.Wrap(err, "failed to query payload")
			}
			return NewCommonCallToolResult(rsp.Data)
		}
	}
}

func handleSavePayload(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments
		group, folder := utils.MapGetString(args, "group"), utils.MapGetString(args, "folder")
		isNew, saveAsFile := utils.MapGetBool(args, "isNew"), utils.MapGetBool(args, "saveAsFile")
		source := utils.MapGetRaw(args, "source")
		sourceMap, ok := source.(map[string]any)
		if !ok {
			return nil, utils.Error("invalid argument: source")
		}
		content, fileName := utils.MapGetString(sourceMap, "content"), utils.MapGetStringSlice(sourceMap, "filename")
		req := ypb.SavePayloadRequest{
			Group:  group,
			Folder: folder,
			IsNew:  isNew,
		}
		if content != "" {
			req.Content = content
		} else if len(fileName) > 0 {
			req.FileName = fileName
			req.IsFile = true
		}

		var progressToken mcp.ProgressToken
		meta := request.Params.Meta
		if meta != nil {
			progressToken = meta.ProgressToken
		}
		type StreamClient interface {
			Recv() (*ypb.SavePayloadProgress, error)
		}
		var (
			stream StreamClient
			err    error
		)

		if saveAsFile {
			stream, err = s.grpcClient.SavePayloadToFileStream(ctx, &req)
			if err != nil {
				return nil, utils.Wrap(err, "failed to save payload to file")
			}
		} else {
			stream, err = s.grpcClient.SavePayloadStream(ctx, &req)
			if err != nil {
				return nil, utils.Wrap(err, "failed to save payload")
			}
		}

		results := make([]any, 0, 4)
		for {
			msg, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					results = append(results, mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("[Error] %v", err),
					})
				}
				break
			}
			s.server.SendNotificationToClient("save_payload/progress", map[string]any{
				"progress":      msg.Progress,
				"message":       msg.Message,
				"progressToken": progressToken,
			})

		}
		if len(results) == 0 {
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: "save payload(s) success",
			})
		}

		return NewCommonCallToolResult(results)
	}
}

func handleCreatePayloadFolder(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.NameRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		rsp, err := s.grpcClient.CreatePayloadFolder(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to create payload folder")
		}
		return NewCommonCallToolResult(rsp)
	}
}

func handleDeletePayload(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments
		group, folder := utils.MapGetString(args, "group"), utils.MapGetString(args, "folder")
		if group != "" {
			req := ypb.DeletePayloadByGroupRequest{
				Group: group,
			}
			_, err := s.grpcClient.DeletePayloadByGroup(ctx, &req)
			if err != nil {
				return nil, utils.Wrap(err, "failed to delete payload(s)")
			}
			return NewCommonCallToolResult("delete payload(s) success")
		} else if folder != "" {
			req := ypb.NameRequest{
				Name: folder,
			}
			_, err := s.grpcClient.DeletePayloadByFolder(ctx, &req)
			if err != nil {
				return nil, utils.Wrap(err, "failed to delete payload(s)")
			}
			return NewCommonCallToolResult("delete payload(s) success")
		} else {
			return nil, utils.Error("all argument is empty")
		}
	}
}

func handleRenamePayloadGroup(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.RenameRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		_, err = s.grpcClient.RenamePayloadGroup(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to rename payload group")
		}
		return NewCommonCallToolResult("rename payload group success")
	}
}

func handleRenamePayloadFolder(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.RenameRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		_, err = s.grpcClient.RenamePayloadFolder(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to rename payload folder")
		}
		return NewCommonCallToolResult("rename payload group success")
	}
}

func handleUpdateOnePayload(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.UpdatePayloadRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		if req.Data == nil {
			return nil, utils.Error("argument:data is empty")
		}
		payload, err := yakit.GetPayloadFirst(s.grpcClient.GetProfileDatabase(), req.Group)
		if err != nil {
			return nil, utils.Wrap(err, "failed to query payload first")
		}
		if payload.GetIsFile() {
			return nil, utils.Error(`cannot update payload of "file" type`)
		}

		_, err = s.grpcClient.UpdatePayload(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to update payload")
		}
		return NewCommonCallToolResult("update payload success")
	}
}

func handleUpdatePayloadFileContent(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.UpdatePayloadToFileRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		payload, err := yakit.GetPayloadFirst(s.grpcClient.GetProfileDatabase(), req.GroupName)
		if err != nil {
			return nil, utils.Wrap(err, "failed to query payload first")
		}
		if !payload.GetIsFile() {
			return nil, utils.Error(`cannot update payload of "database" type`)
		}
		_, err = s.grpcClient.UpdatePayloadToFile(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to update payload to file")
		}
		return NewCommonCallToolResult("update payload to file success")
	}
}
