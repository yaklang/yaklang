package yaklib

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type aiPayloadFilter struct {
	Group     string  `json:"group"`
	Folder    *string `json:"folder"`
	Query     string  `json:"query"`
	PayloadID int64   `json:"payload_id"`
}

func aiPayloadReferences(group string) map[string]any {
	result := map[string]any{"next_step": "Use query_payloads to inspect the dictionary, then do_http_request with fuzztag=true and max-requests. Use raw query/form values; URL mode encodes once. Multiple tags form a Cartesian product; ::row pairs values. Native tag is payload (singular)."}
	if group != "" && strings.TrimSpace(group) == group && !strings.ContainsAny(group, ",/{}()\r\n") && group != "*" {
		result["fuzztag"] = "{{payload(" + group + ")}}"
		result["fuzztag_full"] = "{{payload:full(" + group + ")}}"
		result["fuzztag_nodup"] = "{{payload:nodup(" + group + ")}}"
		result["http_example"] = map[string]any{"url": "https://target.example/submit", "method": "POST", "form": map[string]any{"value": "{{payload(" + group + ")}}"}, "fuzztag": true, "max-requests": 50}
	} else if group != "" {
		result["reference_error"] = "Group name contains FuzzTag delimiters; use query_payloads samples as variables and {{params(name)}} instead."
	}
	return result
}

func QueryPayloads(filterJSON string, opts ...AIQueryOption) (map[string]any, error) {
	c, err := aiQueryOptions(opts)
	if err != nil {
		return nil, err
	}
	f := aiPayloadFilter{}
	if err := decodeAIResourceFilter(filterJSON, &f); err != nil {
		return nil, err
	}
	if f.PayloadID < 0 {
		return nil, fmt.Errorf("payload_id must be nonnegative")
	}
	if c.projectID != 0 {
		return nil, fmt.Errorf("payloads use the runtime profile database")
	}
	db := c.profileDB
	if db == nil {
		return nil, fmt.Errorf("profile database is unavailable")
	}
	if f.PayloadID > 0 && f.Group == "" {
		var row schema.Payload
		if err := db.Select("id, `group`").First(&row, f.PayloadID).Error; err != nil {
			return nil, err
		}
		f.Group = row.Group
	}
	hits := make([]map[string]any, 0)
	if f.Group == "" {
		// Empty-folder sentinels are not dictionaries and cannot be referenced.
		q := db.Model(&schema.Payload{}).Where("substr(`group`, -8) <> ?", "///empty")
		if f.Folder != nil {
			q = q.Where("COALESCE(folder,'') = ?", *f.Folder)
		}
		if f.Query != "" {
			q = q.Where("instr(lower(`group`),lower(?)) > 0", f.Query)
		}
		var total int
		if err := q.Select("COUNT(DISTINCT `group`)").Row().Scan(&total); err != nil {
			return nil, err
		}
		var groups []string
		if err := q.Order("`group` ASC").Limit(c.limit).Offset(c.offset).Pluck("DISTINCT `group`", &groups).Error; err != nil {
			return nil, err
		}
		for _, group := range groups {
			if err := c.ctx.Err(); err != nil {
				return nil, err
			}
			mode, err := yakit.InspectPayloadGroupStorage(db, group)
			if err != nil {
				return nil, err
			}
			var row schema.Payload
			if err := db.Select("folder").Where("`group` = ?", group).First(&row).Error; err != nil {
				return nil, err
			}
			var count int
			if err := db.Model(&schema.Payload{}).Where("`group` = ?", group).Count(&count).Error; err != nil {
				return nil, err
			}
			hit := aiPayloadReferences(group)
			hit["group"], hit["folder"], hit["storage"], hit["stored_rows"] = group, row.Folder, mode.String(), count
			hits = append(hits, hit)
		}
		result := aiResourcePage(c, hits, total)
		result["view"] = "groups"
		result["usage"] = aiPayloadReferences("")
		return result, nil
	}
	mode, err := yakit.InspectPayloadGroupStorage(db, f.Group)
	if err != nil {
		return nil, err
	}
	if mode == yakit.PayloadGroupStorageInconsistent {
		return nil, fmt.Errorf("group %q has inconsistent storage; repair it in Yakit first", f.Group)
	}
	q := db.Model(&schema.Payload{}).Where("`group` = ?", f.Group)
	if f.Folder != nil {
		q = q.Where("COALESCE(folder,'') = ?", *f.Folder)
	}
	if f.PayloadID > 0 {
		q = q.Where("id = ?", f.PayloadID)
	}
	if mode == yakit.PayloadGroupStorageFile {
		var row schema.Payload
		if err := q.First(&row).Error; err != nil {
			return nil, err
		}
		info, err := os.Stat(row.GetContentRaw())
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("payload file is not regular")
		}
		file, err := os.Open(row.GetContentRaw())
		if err != nil {
			return nil, err
		}
		defer file.Close()
		info, err = file.Stat()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("payload file is not regular")
		}
		// File content is sampled by matching line offset, never returned as a path.
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		matched, line, scannedBytes := 0, 0, 0
		for scanner.Scan() {
			if err := c.ctx.Err(); err != nil {
				return nil, err
			}
			line++
			scannedBytes += len(scanner.Bytes())
			if line > 100000 || scannedBytes > 64<<20 {
				return nil, fmt.Errorf("file sample exceeds scan budget; narrow the query or use Yakit")
			}
			value := aiUnquote(scanner.Text())
			if f.Query != "" && !strings.Contains(strings.ToLower(value), strings.ToLower(f.Query)) {
				continue
			}
			matched++
			if matched <= c.offset {
				continue
			}
			hit := map[string]any{"payload_id": row.ID, "line_number": line, "group": f.Group, "editable": false}
			aiResourceContent(hit, value, c)
			hits = append(hits, hit)
			if len(hits) > c.limit {
				break
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		hasMore := len(hits) > c.limit
		if hasMore {
			hits = hits[:c.limit]
		}
		result := map[string]any{"hits": hits, "storage": mode.String(), "view": "entries", "group": f.Group, "offset": c.offset, "next_offset": c.offset + len(hits), "has_more": hasMore, "usage": aiPayloadReferences(f.Group)}
		result["management_hint"] = "File-backed dictionary contents are read-only here; delete_group removes the registry only. Import it as a database dictionary in Yakit to edit individual payloads."
		return result, nil
	}
	// DB payloads are quoted. Search the raw quoted representation, escaping the
	// user's keyword so quotes, backslashes and newlines match stored content.
	if f.Query != "" {
		quoted := strconv.Quote(f.Query)
		q = q.Where("instr(lower(content),lower(?)) > 0", quoted[1:len(quoted)-1])
	}
	var total int
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []*schema.Payload
	if err := q.Order("hit_count DESC, id ASC").Limit(c.limit).Offset(c.offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		hit := map[string]any{"payload_id": row.ID, "group": row.Group, "folder": row.Folder, "hit_count": row.HitCount, "editable": true}
		aiResourceContent(hit, row.GetContentRaw(), c)
		hits = append(hits, hit)
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	result := aiResourcePage(c, hits, total)
	result["view"], result["storage"], result["group"], result["usage"] = "entries", mode.String(), f.Group, aiPayloadReferences(f.Group)
	return result, nil
}

type aiPayloadAmendment struct {
	Operator   string   `json:"operator"`
	Group      string   `json:"group"`
	Folder     *string  `json:"folder"`
	Contents   []string `json:"contents"`
	PayloadIDs []int64  `json:"payload_ids"`
}

// ManagePayloads manages exact rows or explicitly named groups transactionally.
// Content is quoted exactly like Yakit; deduplication never resets hit counters.
func ManagePayloads(requestJSON string, opts ...AIQueryOption) (map[string]any, error) {
	c, err := aiQueryOptions(opts)
	if err != nil {
		return nil, err
	}
	f := aiPayloadAmendment{}
	if err := decodeAIResourceFilter(requestJSON, &f); err != nil {
		return nil, err
	}
	if c.projectID != 0 {
		return nil, fmt.Errorf("payloads use the runtime profile database")
	}
	if strings.TrimSpace(f.Group) == "" || strings.HasSuffix(f.Group, "///empty") {
		return nil, fmt.Errorf("a nonempty dictionary group is required")
	}
	if len(f.Contents) > 200 || len(f.PayloadIDs) > 200 {
		return nil, fmt.Errorf("at most 200 payloads per operation")
	}
	bytes := 0
	for _, s := range f.Contents {
		bytes += len(s)
	}
	if bytes > 256<<10 {
		return nil, fmt.Errorf("payload batch exceeds 256 KiB")
	}
	for _, id := range f.PayloadIDs {
		if id <= 0 {
			return nil, fmt.Errorf("payload IDs must be positive")
		}
	}
	switch f.Operator {
	case "add":
		if len(f.Contents) == 0 || len(f.PayloadIDs) != 0 {
			return nil, fmt.Errorf("add requires contents and no payload_ids")
		}
	case "change":
		if len(f.Contents) != 1 || len(f.PayloadIDs) != 1 || f.Folder != nil {
			return nil, fmt.Errorf("change requires one payload_id and one content; folder is only for add")
		}
	case "delete":
		if len(f.PayloadIDs) == 0 || len(f.Contents) != 0 || f.Folder != nil {
			return nil, fmt.Errorf("delete requires payload_ids and no contents/folder")
		}
	case "delete_group":
		if len(f.PayloadIDs) != 0 || len(f.Contents) != 0 || f.Folder != nil {
			return nil, fmt.Errorf("delete_group requires only the exact group")
		}
	default:
		return nil, fmt.Errorf("operator must be add, change, delete or delete_group")
	}
	db := c.profileDB
	if db == nil {
		return nil, fmt.Errorf("profile database is unavailable")
	}
	ids := make([]int64, 0)
	affected, skipped := 0, 0
	err = utils.GormTransaction(db, func(tx *gorm.DB) error {
		if err := c.ctx.Err(); err != nil {
			return err
		}
		mode, err := yakit.InspectPayloadGroupStorage(tx, f.Group)
		if err != nil {
			return err
		}
		if mode == yakit.PayloadGroupStorageInconsistent {
			return fmt.Errorf("group has inconsistent storage; repair it in Yakit first")
		}
		if f.Operator == "delete_group" {
			result := tx.Where("`group` = ?", f.Group).Unscoped().Delete(&schema.Payload{})
			affected = int(result.RowsAffected)
			if result.Error != nil {
				return result.Error
			}
			return c.ctx.Err()
		}
		if mode == yakit.PayloadGroupStorageFile {
			return fmt.Errorf("file-backed payload contents cannot be edited as database rows; import a database dictionary in Yakit first")
		}
		if mode == yakit.PayloadGroupStorageLegacyFileFlag {
			if err := tx.Model(&schema.Payload{}).Where("`group` = ?", f.Group).UpdateColumn("is_file", false).Error; err != nil {
				return err
			}
		}
		if f.Operator == "add" {
			folder := ""
			if f.Folder != nil {
				folder = *f.Folder
			}
			var existing schema.Payload
			result := tx.Select("folder, group_index").Where("`group` = ?", f.Group).First(&existing)
			if result.Error != nil && !gorm.IsRecordNotFoundError(result.Error) {
				return result.Error
			}
			if result.Error == nil {
				if existing.Folder != nil {
					if f.Folder != nil && folder != *existing.Folder {
						return fmt.Errorf("existing group belongs to another folder")
					}
					folder = *existing.Folder
				}
			}
			for _, value := range f.Contents {
				if err := c.ctx.Err(); err != nil {
					return err
				}
				row := yakit.NewPayload(f.Group, strconv.Quote(value))
				row.Folder = &folder
				row.GroupIndex = existing.GroupIndex
				row.Hash = row.CalcHash()
				var duplicate schema.Payload
				result := tx.Where("hash = ?", row.Hash).First(&duplicate)
				if result.Error == nil {
					ids = append(ids, int64(duplicate.ID))
					skipped++
					continue
				}
				if !gorm.IsRecordNotFoundError(result.Error) {
					return result.Error
				}
				if err := tx.Create(row).Error; err != nil {
					return err
				}
				ids = append(ids, int64(row.ID))
				affected++
			}
		} else {
			var rows []*schema.Payload
			if err := tx.Where("`group` = ? AND id IN (?)", f.Group, f.PayloadIDs).Find(&rows).Error; err != nil {
				return err
			}
			unique := map[int64]bool{}
			for _, id := range f.PayloadIDs {
				unique[id] = true
			}
			if len(rows) != len(unique) {
				return fmt.Errorf("one or more payload IDs are missing or belong to another group")
			}
			for _, row := range rows {
				if err := c.ctx.Err(); err != nil {
					return err
				}
				if f.Operator == "delete" {
					if err := yakit.DeletePayloadByID(tx, int64(row.ID)); err != nil {
						return err
					}
				} else {
					content := strconv.Quote(f.Contents[0])
					row.Content = &content
					row.Hash = row.CalcHash()
					if err := yakit.UpdatePayload(tx, int(row.ID), row); err != nil {
						return err
					}
				}
				ids = append(ids, int64(row.ID))
				affected++
			}
		}
		return c.ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"operator": f.Operator, "group": f.Group, "affected": affected, "duplicates_skipped": skipped, "payload_ids": ids, "usage": aiPayloadReferences(f.Group), "physical_file_deleted": false}, nil
}
