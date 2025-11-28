package yakcmds

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// VacuumSQLiteCommand is the CLI command for vacuuming SQLite databases
var VacuumSQLiteCommand = &cli.Command{
	Name:    "vacuum-sqlite",
	Aliases: []string{"vacuum"},
	Usage:   "Vacuum SQLite databases to reclaim unused space and reduce file size",
	Flags: []cli.Flag{
		cli.StringSliceFlag{
			Name:  "db-file",
			Usage: "Custom SQLite database file(s) to vacuum (can be specified multiple times)",
		},
		cli.BoolFlag{
			Name:  "all",
			Usage: "Vacuum all .db files in the yakit-projects directory",
		},
		cli.BoolFlag{
			Name:  "dry-run",
			Usage: "Only show what would be done without actually vacuuming",
		},
		cli.BoolFlag{
			Name:  "wal-checkpoint",
			Usage: "Also perform WAL checkpoint (TRUNCATE) before vacuum",
		},
		cli.BoolFlag{
			Name:  "force-truncate-wal",
			Usage: "Force truncate WAL file by temporarily switching to DELETE journal mode (enabled by default when WAL exists)",
		},
		cli.BoolFlag{
			Name:  "no-force-truncate-wal",
			Usage: "Disable automatic force truncate WAL optimization",
		},
		cli.BoolFlag{
			Name:  "skip-default",
			Usage: "Skip default databases (project, profile, ssa) when using --all",
		},
		cli.IntFlag{
			Name:  "busy-timeout",
			Usage: "SQLite busy timeout in milliseconds (default 5000ms)",
			Value: 5000,
		},
	},
	Action: func(c *cli.Context) error {
		dbFiles := c.StringSlice("db-file")
		vacuumAll := c.Bool("all")
		dryRun := c.Bool("dry-run")
		walCheckpoint := c.Bool("wal-checkpoint")
		forceTruncateWAL := c.Bool("force-truncate-wal")
		noForceTruncateWAL := c.Bool("no-force-truncate-wal")
		skipDefault := c.Bool("skip-default")
		busyTimeout := c.Int("busy-timeout")

		// Auto-detect mode: will be set per-database based on WAL existence
		autoDetectForceTruncate := !forceTruncateWAL && !noForceTruncateWAL

		// force-truncate-wal implies wal-checkpoint
		if forceTruncateWAL {
			walCheckpoint = true
		}

		var databasesToVacuum []string

		// If custom db files are specified, add them
		if len(dbFiles) > 0 {
			for _, dbFile := range dbFiles {
				if utils.GetFirstExistedFile(dbFile) != "" {
					databasesToVacuum = append(databasesToVacuum, dbFile)
				} else {
					log.Warnf("database file not found: %s", dbFile)
				}
			}
		}

		// If --all is specified, scan for all .db files
		if vacuumAll {
			baseDir := consts.GetDefaultYakitBaseDir()
			log.Infof("scanning for .db files in: %s", baseDir)

			err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil // skip errors
				}
				if info.IsDir() {
					// skip temp directories
					if info.Name() == "temp" || strings.HasPrefix(info.Name(), ".") {
						return filepath.SkipDir
					}
					return nil
				}
				if strings.HasSuffix(strings.ToLower(info.Name()), ".db") {
					// skip WAL and SHM files
					if strings.HasSuffix(info.Name(), "-wal") || strings.HasSuffix(info.Name(), "-shm") {
						return nil
					}
					databasesToVacuum = append(databasesToVacuum, path)
				}
				return nil
			})
			if err != nil {
				log.Warnf("error scanning directory: %v", err)
			}
		}

		// If no custom files and not --all, use default databases
		if len(dbFiles) == 0 && !vacuumAll {
			baseDir := consts.GetDefaultYakitBaseDir()
			defaultDBs := []string{
				consts.GetDefaultYakitProjectDatabase(baseDir),
				consts.GetDefaultYakitPluginDatabase(baseDir),
			}
			// Add SSA database
			_, ssaDBPath := consts.GetSSADataBaseInfo()
			defaultDBs = append(defaultDBs, ssaDBPath)

			for _, dbPath := range defaultDBs {
				if utils.GetFirstExistedFile(dbPath) != "" {
					databasesToVacuum = append(databasesToVacuum, dbPath)
				}
			}
		}

		// Remove duplicates
		databasesToVacuum = utils.RemoveRepeatStringSlice(databasesToVacuum)

		if len(databasesToVacuum) == 0 {
			fmt.Println("No databases found to vacuum.")
			return nil
		}

		// Filter out default databases if skipDefault is set
		if skipDefault && vacuumAll {
			baseDir := consts.GetDefaultYakitBaseDir()
			defaultDBs := map[string]bool{
				consts.GetDefaultYakitProjectDatabase(baseDir): true,
				consts.GetDefaultYakitPluginDatabase(baseDir):  true,
			}
			_, ssaDBPath := consts.GetSSADataBaseInfo()
			defaultDBs[ssaDBPath] = true

			var filtered []string
			for _, db := range databasesToVacuum {
				if !defaultDBs[db] {
					filtered = append(filtered, db)
				}
			}
			databasesToVacuum = filtered
		}

		fmt.Printf("\n╔══════════════════════════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║                         SQLite Database Vacuum Utility                       ║\n")
		fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════╣\n")
		fmt.Printf("║  Databases to process: %-54d║\n", len(databasesToVacuum))
		if dryRun {
			fmt.Printf("║  Mode: DRY RUN (no changes will be made)                                    ║\n")
		}
		fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════╝\n\n")

		var totalSavedBytes int64
		var successCount, failCount int

		for i, dbPath := range databasesToVacuum {
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			fmt.Printf("[%d/%d] Processing: %s\n", i+1, len(databasesToVacuum), dbPath)
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

			saved, err := vacuumDatabase(dbPath, dryRun, walCheckpoint, forceTruncateWAL, autoDetectForceTruncate, busyTimeout)
			if err != nil {
				log.Errorf("failed to vacuum %s: %v", dbPath, err)
				failCount++
				fmt.Printf("  [FAILED] Status: FAILED - %v\n\n", err)
			} else {
				totalSavedBytes += saved
				successCount++
				fmt.Printf("  [OK] Status: SUCCESS\n\n")
			}
		}

		// Summary
		fmt.Printf("\n╔══════════════════════════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║                                   Summary                                    ║\n")
		fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════╣\n")
		fmt.Printf("║  Total databases processed: %-50d║\n", len(databasesToVacuum))
		fmt.Printf("║  Successful: %-65d║\n", successCount)
		fmt.Printf("║  Failed: %-69d║\n", failCount)
		fmt.Printf("║  Total space saved: %-58s║\n", utils.ByteSize(uint64(totalSavedBytes)))
		fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════╝\n")

		return nil
	},
}

// vacuumDatabase performs vacuum on a single SQLite database file
// Returns the number of bytes saved
func vacuumDatabase(dbPath string, dryRun bool, walCheckpoint bool, forceTruncateWAL bool, autoDetectForceTruncate bool, busyTimeout int) (int64, error) {
	// Get file info before vacuum
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		return 0, fmt.Errorf("cannot stat database file: %v", err)
	}
	sizeBefore := dbInfo.Size()

	// Check for WAL file
	walPath := dbPath + "-wal"
	shmPath := dbPath + "-shm"
	var walSizeBefore int64
	walExists := false

	if walInfo, err := os.Stat(walPath); err == nil {
		walSizeBefore = walInfo.Size()
		walExists = true
	}

	// Get database info
	dbInfo2, journalMode, freelistCount, pageSize, autoVacuum, err := getSQLiteInfo(dbPath)
	if err != nil {
		log.Warnf("cannot get database info: %v", err)
	}

	// Auto-detect: enable force truncate WAL if WAL file exists and is significant (> 1MB)
	if autoDetectForceTruncate && walExists && walSizeBefore > 1024*1024 && strings.ToUpper(journalMode) == "WAL" {
		forceTruncateWAL = true
		walCheckpoint = true
		fmt.Printf("  [AUTO] Detected large WAL file (%s), enabling force truncate optimization\n", utils.ByteSize(uint64(walSizeBefore)))
	}

	fmt.Printf("  Database File: %s\n", filepath.Base(dbPath))
	fmt.Printf("     Path: %s\n", dbPath)
	fmt.Printf("     Size before: %s (%d bytes)\n", utils.ByteSize(uint64(sizeBefore)), sizeBefore)
	if walExists {
		fmt.Printf("     WAL file size: %s (%d bytes)\n", utils.ByteSize(uint64(walSizeBefore)), walSizeBefore)
	}
	fmt.Printf("     Journal mode: %s\n", journalMode)
	fmt.Printf("     Page size: %d bytes\n", pageSize)
	fmt.Printf("     Auto vacuum: %s\n", autoVacuum)
	fmt.Printf("     Free pages: %d\n", freelistCount)
	if dbInfo2 != "" {
		fmt.Printf("     SQLite version: %s\n", dbInfo2)
	}

	if dryRun {
		fmt.Printf("     [DRY RUN] Would vacuum this database\n")
		if forceTruncateWAL && strings.ToUpper(journalMode) == "WAL" {
			fmt.Printf("     [DRY RUN] Would force truncate WAL by switching to DELETE mode\n")
		}
		return 0, nil
	}

	// For force truncate WAL, we use a special approach:
	// 1. Switch to DELETE journal mode (this checkpoints and removes WAL)
	// 2. Perform VACUUM
	// 3. Switch back to WAL mode
	if forceTruncateWAL && strings.ToUpper(journalMode) == "WAL" {
		saved, err := vacuumWithForceTruncateWAL(dbPath, walPath, shmPath, sizeBefore, walSizeBefore, busyTimeout)
		if err != nil {
			// Check if it's a database locked error
			if strings.Contains(err.Error(), "database is locked") {
				fmt.Printf("     [WARN] Database is locked (Yakit/GoLand may be running), falling back to regular vacuum...\n")
				fmt.Printf("     [TIP] Close Yakit/GoLand database tools for best results, or use --no-force-truncate-wal\n")
				// Fall back to regular vacuum
				walCheckpoint = true
				forceTruncateWAL = false
			} else {
				return saved, err
			}
		} else {
			return saved, nil
		}
	}

	// Open database for vacuum with busy timeout
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_busy_timeout=%d", dbPath, busyTimeout))
	if err != nil {
		return 0, fmt.Errorf("cannot open database: %v", err)
	}
	defer db.Close()

	// Set busy timeout
	_, err = db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d;", busyTimeout))
	if err != nil {
		log.Warnf("failed to set busy_timeout: %v", err)
	}

	// Perform WAL checkpoint if requested and in WAL mode
	if walCheckpoint && strings.ToUpper(journalMode) == "WAL" {
		fmt.Printf("     Performing WAL checkpoint (TRUNCATE)...\n")
		_, err = db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
		if err != nil {
			if strings.Contains(err.Error(), "database is locked") {
				fmt.Printf("     [WARN] WAL checkpoint skipped (database locked by other process)\n")
			} else {
				log.Warnf("WAL checkpoint failed: %v", err)
			}
		} else {
			fmt.Printf("     WAL checkpoint completed\n")
		}
	}

	// Perform VACUUM
	fmt.Printf("     Performing VACUUM...\n")
	_, err = db.Exec("VACUUM;")
	if err != nil {
		if strings.Contains(err.Error(), "database is locked") {
			return 0, fmt.Errorf("VACUUM failed: database is locked. Please close Yakit and try again")
		}
		return 0, fmt.Errorf("VACUUM failed: %v", err)
	}

	// Perform another checkpoint after vacuum to truncate the new WAL
	if walCheckpoint && strings.ToUpper(journalMode) == "WAL" {
		fmt.Printf("     Performing post-vacuum WAL checkpoint (TRUNCATE)...\n")
		_, err = db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
		if err != nil {
			if strings.Contains(err.Error(), "database is locked") {
				fmt.Printf("     [WARN] Post-vacuum WAL checkpoint skipped (database locked)\n")
			} else {
				log.Warnf("Post-vacuum WAL checkpoint failed: %v", err)
			}
		} else {
			fmt.Printf("     Post-vacuum WAL checkpoint completed\n")
		}
	}

	// Close the database to ensure changes are flushed
	db.Close()

	// Get file info after vacuum
	dbInfoAfter, err := os.Stat(dbPath)
	if err != nil {
		return 0, fmt.Errorf("cannot stat database file after vacuum: %v", err)
	}
	sizeAfter := dbInfoAfter.Size()

	// Check WAL after
	var walSizeAfter int64
	if walInfo, err := os.Stat(walPath); err == nil {
		walSizeAfter = walInfo.Size()
	}

	// Calculate savings
	dbSaved := sizeBefore - sizeAfter
	walSaved := walSizeBefore - walSizeAfter
	totalSaved := dbSaved + walSaved

	fmt.Printf("     Size after: %s (%d bytes)\n", utils.ByteSize(uint64(sizeAfter)), sizeAfter)
	if walExists || walSizeAfter > 0 {
		fmt.Printf("     WAL file size after: %s (%d bytes)\n", utils.ByteSize(uint64(walSizeAfter)), walSizeAfter)
	}

	if totalSaved > 0 {
		fmt.Printf("     [SAVED] Space saved: %s (DB: %s, WAL: %s)\n",
			utils.ByteSize(uint64(totalSaved)),
			utils.ByteSize(uint64(dbSaved)),
			utils.ByteSize(uint64(walSaved)))
	} else if totalSaved < 0 {
		fmt.Printf("     [WARN] Size increased by: %s (this can happen with small databases)\n",
			utils.ByteSize(uint64(-totalSaved)))
	} else {
		fmt.Printf("     [INFO] No size change\n")
	}

	// Clean up SHM file if exists and WAL is gone
	if walSizeAfter == 0 {
		if _, err := os.Stat(shmPath); err == nil {
			os.Remove(shmPath)
		}
		if _, err := os.Stat(walPath); err == nil {
			os.Remove(walPath)
		}
	}

	return totalSaved, nil
}

// vacuumWithForceTruncateWAL performs vacuum with forced WAL truncation
// by temporarily switching to DELETE journal mode
func vacuumWithForceTruncateWAL(dbPath, walPath, shmPath string, sizeBefore, walSizeBefore int64, busyTimeout int) (int64, error) {
	fmt.Printf("     [FORCE] Truncate WAL mode: switching to DELETE journal mode...\n")

	// Open database with exclusive locking and busy timeout
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_busy_timeout=%d&_locking_mode=EXCLUSIVE", dbPath, busyTimeout))
	if err != nil {
		return 0, fmt.Errorf("cannot open database: %v", err)
	}

	// Set busy timeout
	_, err = db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d;", busyTimeout))
	if err != nil {
		log.Warnf("failed to set busy_timeout: %v", err)
	}

	// First, checkpoint all WAL content
	fmt.Printf("     Performing final WAL checkpoint (TRUNCATE)...\n")
	_, err = db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	if err != nil {
		if strings.Contains(err.Error(), "database is locked") {
			db.Close()
			return 0, fmt.Errorf("database is locked - please close Yakit first")
		}
		log.Warnf("WAL checkpoint failed: %v", err)
	}

	// Switch to DELETE mode - this will checkpoint and remove WAL
	fmt.Printf("     Switching to DELETE journal mode...\n")
	var newMode string
	err = db.QueryRow("PRAGMA journal_mode=DELETE;").Scan(&newMode)
	if err != nil {
		db.Close()
		if strings.Contains(err.Error(), "database is locked") {
			return 0, fmt.Errorf("database is locked - please close Yakit first")
		}
		return 0, fmt.Errorf("failed to switch to DELETE mode: %v", err)
	}
	fmt.Printf("     Journal mode switched to: %s\n", newMode)

	// Perform VACUUM in DELETE mode
	fmt.Printf("     Performing VACUUM in DELETE mode...\n")
	_, err = db.Exec("VACUUM;")
	if err != nil {
		db.Close()
		return 0, fmt.Errorf("VACUUM failed: %v", err)
	}

	// Switch back to WAL mode
	fmt.Printf("     Switching back to WAL journal mode...\n")
	err = db.QueryRow("PRAGMA journal_mode=WAL;").Scan(&newMode)
	if err != nil {
		log.Warnf("failed to switch back to WAL mode: %v", err)
	} else {
		fmt.Printf("     Journal mode restored to: %s\n", newMode)
	}

	// Close the database
	db.Close()

	// Manually remove WAL and SHM files if they still exist
	if _, err := os.Stat(walPath); err == nil {
		if err := os.Remove(walPath); err != nil {
			log.Warnf("failed to remove WAL file: %v", err)
		} else {
			fmt.Printf("     Removed WAL file\n")
		}
	}
	if _, err := os.Stat(shmPath); err == nil {
		if err := os.Remove(shmPath); err != nil {
			log.Warnf("failed to remove SHM file: %v", err)
		} else {
			fmt.Printf("     Removed SHM file\n")
		}
	}

	// Get file info after vacuum
	dbInfoAfter, err := os.Stat(dbPath)
	if err != nil {
		return 0, fmt.Errorf("cannot stat database file after vacuum: %v", err)
	}
	sizeAfter := dbInfoAfter.Size()

	// Check WAL after (should be 0 or non-existent)
	var walSizeAfter int64
	if walInfo, err := os.Stat(walPath); err == nil {
		walSizeAfter = walInfo.Size()
	}

	// Calculate savings
	dbSaved := sizeBefore - sizeAfter
	walSaved := walSizeBefore - walSizeAfter
	totalSaved := dbSaved + walSaved

	fmt.Printf("     Size after: %s (%d bytes)\n", utils.ByteSize(uint64(sizeAfter)), sizeAfter)
	if walSizeAfter > 0 {
		fmt.Printf("     WAL file size after: %s (%d bytes)\n", utils.ByteSize(uint64(walSizeAfter)), walSizeAfter)
	} else {
		fmt.Printf("     WAL file: removed\n")
	}

	if totalSaved > 0 {
		fmt.Printf("     [SAVED] Space saved: %s (DB: %s, WAL: %s)\n",
			utils.ByteSize(uint64(totalSaved)),
			utils.ByteSize(uint64(dbSaved)),
			utils.ByteSize(uint64(walSaved)))
	} else if totalSaved < 0 {
		fmt.Printf("     [WARN] Size increased by: %s (this can happen with small databases)\n",
			utils.ByteSize(uint64(-totalSaved)))
	} else {
		fmt.Printf("     [INFO] No size change\n")
	}

	return totalSaved, nil
}

// getSQLiteInfo retrieves SQLite database information
func getSQLiteInfo(dbPath string) (sqliteVersion, journalMode string, freelistCount, pageSize int, autoVacuum string, err error) {
	db, err := sql.Open("sqlite3", dbPath+"?mode=ro")
	if err != nil {
		return "", "", 0, 0, "", err
	}
	defer db.Close()

	// Get SQLite version
	row := db.QueryRow("SELECT sqlite_version();")
	row.Scan(&sqliteVersion)

	// Get journal mode
	row = db.QueryRow("PRAGMA journal_mode;")
	row.Scan(&journalMode)

	// Get freelist count
	row = db.QueryRow("PRAGMA freelist_count;")
	row.Scan(&freelistCount)

	// Get page size
	row = db.QueryRow("PRAGMA page_size;")
	row.Scan(&pageSize)

	// Get auto_vacuum
	var autoVacuumInt int
	row = db.QueryRow("PRAGMA auto_vacuum;")
	row.Scan(&autoVacuumInt)
	switch autoVacuumInt {
	case 0:
		autoVacuum = "NONE"
	case 1:
		autoVacuum = "FULL"
	case 2:
		autoVacuum = "INCREMENTAL"
	default:
		autoVacuum = fmt.Sprintf("UNKNOWN(%d)", autoVacuumInt)
	}

	return sqliteVersion, journalMode, freelistCount, pageSize, autoVacuum, nil
}
