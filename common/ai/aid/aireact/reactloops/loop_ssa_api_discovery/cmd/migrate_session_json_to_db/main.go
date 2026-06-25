package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func main() {
	workDir := flag.String("workdir", "", "discovery task work directory (contains ssa_discovery/)")
	flag.Parse()
	if *workDir == "" {
		log.Fatal("usage: migrate_session_json_to_db --workdir=/path/to/task")
	}
	workDirAbs, err := filepath.Abs(*workDir)
	if err != nil {
		log.Fatal(err)
	}

	db, err := store.OpenSessionDB(workDirAbs)
	if err != nil {
		log.Fatal(err)
	}
	defer closeDB(db)

	repo := store.NewRepository(db)
	sess, err := repo.GetLatestSession()
	if err != nil {
		log.Fatal(err)
	}

	rt := &loop_ssa_api_discovery.Runtime{
		WorkDir:    workDirAbs,
		SQLitePath: store.DBPath(workDirAbs),
		DB:         db,
		Repo:       repo,
		Session:    sess,
	}

	ingests := []struct {
		kind string
		path string
	}{
		{store.ArtifactStaticRouteHints, store.StaticRouteHintsPath(workDirAbs)},
		{store.ArtifactAuthSurface, store.AuthSurfacePath(workDirAbs)},
		{store.ArtifactDependencies, store.DependenciesInventoryPath(workDirAbs)},
		{store.ArtifactPhase1PrepBundle, store.Phase1PrepBundlePath(workDirAbs)},
		{store.ArtifactCodeReadingPlan, store.CodeReadingPlanPath(workDirAbs)},
		{store.ArtifactForwardingProfile, store.ForwardingProfilePath(workDirAbs)},
		{store.ArtifactApiPreanalysisFull, store.ApiPreanalysisReportPath(workDirAbs)},
		{store.ArtifactSyntaxflowSummary, store.SyntaxflowSummaryPath(workDirAbs)},
	}
	for _, item := range ingests {
		if _, err := os.Stat(item.path); err != nil {
			continue
		}
		if err := loop_ssa_api_discovery.IngestPhaseArtifactFromPath(rt, item.kind, item.path); err != nil {
			log.Printf("warn ingest %s: %v", item.kind, err)
			continue
		}
		fmt.Printf("ingested artifact: %s\n", item.kind)
	}

	checklistPath := store.VulnChecklistPath(workDirAbs)
	if b, err := os.ReadFile(checklistPath); err == nil {
		var items []loop_ssa_api_discovery.VulnChecklistItem
		if err := json.Unmarshal(b, &items); err == nil && len(items) > 0 {
			storeRows := make([]store.VulnChecklistItem, 0, len(items))
			for _, it := range items {
				storeRows = append(storeRows, store.VulnChecklistItem{
					SessionID:         sess.ID,
					FindingID:         it.FindingID,
					EndpointID:        it.EndpointID,
					VerifiedHttpApiID: it.VerifiedHttpApiID,
					RuleName:          it.RuleName,
					Severity:          it.Severity,
					Title:             it.Title,
					MatchedFile:       it.MatchedFile,
					DataFlowHint:      it.DataFlowHint,
					Method:            it.Method,
					PathPattern:       it.PathPattern,
					FullSampleURL:     it.FullSampleURL,
					HandlerClass:      it.HandlerClass,
					Priority:          it.Priority,
					AssocConfidence:   it.AssocConfidence,
					Status:            store.VulnChecklistStatusPending,
				})
			}
			if err := repo.ReplaceVulnChecklistItems(sess.ID, storeRows); err != nil {
				log.Printf("warn vuln_checklist migrate: %v", err)
			} else {
				fmt.Printf("migrated vuln_checklist_items: %d rows\n", len(storeRows))
			}
		}
	}

	if _, err := loop_ssa_api_discovery.ExportDiscoverySnapshotJSON(rt); err != nil {
		log.Printf("warn snapshot export: %v", err)
	} else {
		fmt.Println("refreshed discovery_snapshot.json")
	}
	fmt.Println("migration done")
}

func closeDB(db *gorm.DB) {
	if db == nil {
		return
	}
	if s := db.DB(); s != nil {
		_ = s.Close()
	}
}
