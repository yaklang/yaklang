//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/aivizhttp"
)

func main() {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		fmt.Fprintln(os.Stderr, "db nil")
		os.Exit(1)
	}
	sessionID := "WkjKLnG9OfrqUO5KzU75NtYvuiTl3JFacIR4yH8x"
	var events []*schema.AiOutputEvent
	if err := db.Where("session_id = ?", sessionID).Order("id asc").Find(&events).Error; err != nil {
		fmt.Fprintln(os.Stderr, "query failed:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "loaded %d events\n", len(events))
	proj := aivizhttp.NewContextProjector()
	resp := proj.ProjectEvents(events)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(resp)
}
