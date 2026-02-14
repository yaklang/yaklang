package cvequeryops

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	// import aiforge to register liteforge callback
	_ "github.com/yaklang/yaklang/common/aiforge"
)

// CVEAICompleteConfig holds configuration for CVE AI completion
type CVEAICompleteConfig struct {
	Concurrent int   // Number of concurrent workers
	TestLimit  int   // Limit number of CVEs to process (0 = no limit)
	aiOpts     []any // AI options to pass to LiteForge
}

// CVEAICompleteOption is a function type for configuring CVE AI completion
type CVEAICompleteOption func(*CVEAICompleteConfig)

// WithCVEAIConcurrent sets the number of concurrent workers for AI completion
func WithCVEAIConcurrent(n int) CVEAICompleteOption {
	return func(c *CVEAICompleteConfig) {
		if n > 0 {
			c.Concurrent = n
		}
	}
}

// WithCVETestLimit sets the maximum number of CVEs to process (for testing)
func WithCVETestLimit(n int) CVEAICompleteOption {
	return func(c *CVEAICompleteConfig) {
		if n > 0 {
			c.TestLimit = n
		}
	}
}

// cveTranslationTask represents a single CVE translation task
type cveTranslationTask struct {
	cve    *cveresources.CVE
	prompt string
	index  int
	total  int
}

// cveTranslationResult represents the result of a translation task
type cveTranslationResult struct {
	cve     *cveresources.CVE
	success bool
	err     error
}

// CVEAICompleteFields uses AI to complete missing CVE fields like translations
// Usage:
//   - cve.AICompleteFields() - use default settings
//   - cve.AICompleteFields(ai.type("openai")) - specify AI type
//   - cve.AICompleteFields(cve.aiConcurrent(10)) - use 10 concurrent workers
//   - cve.AICompleteFields(cve.testLimit(5)) - only process 5 CVEs for testing
//   - cve.AICompleteFields(cve.aiConcurrent(10), cve.testLimit(5), ai.type("openai"))
func CVEAICompleteFields(opts ...any) error {
	// Parse options
	config := &CVEAICompleteConfig{
		Concurrent: 5, // Default: parallel processing with 5 concurrent workers
		TestLimit:  0, // Default: no limit
	}

	for _, opt := range opts {
		switch v := opt.(type) {
		case CVEAICompleteOption:
			v(config)
		default:
			// Pass other options (like ai.type, ai.model) to LiteForge
			config.aiOpts = append(config.aiOpts, opt)
		}
	}

	db := consts.GetGormCVEDatabase()
	if db == nil {
		return utils.Errorf("cannot get CVE database")
	}

	// Count total CVEs to process
	var totalCount int
	if err := db.Model(&cveresources.CVE{}).Count(&totalCount).Error; err != nil {
		return utils.Errorf("count CVE entries failed: %v", err)
	}
	log.Infof("found %d CVE entries in database", totalCount)

	if totalCount == 0 {
		return utils.Errorf("no CVE entries found in database, please run cve.Download() and cve.LoadCVE() first")
	}

	// Collect CVEs that need translation
	var cvesToProcess []*cveresources.CVE
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	skippedCount := 0
	for cve := range cveresources.YieldCVEs(db, ctx) {
		// Skip if already has Chinese translations
		if cve.TitleZh != "" && cve.DescriptionMainZh != "" {
			skippedCount++
			continue
		}
		cvesToProcess = append(cvesToProcess, cve)

		// Check test limit
		if config.TestLimit > 0 && len(cvesToProcess) >= config.TestLimit {
			log.Infof("test limit reached: %d CVEs", config.TestLimit)
			break
		}
	}

	needProcess := len(cvesToProcess)
	log.Infof("need to process %d CVEs, skipped %d (already translated)", needProcess, skippedCount)

	if needProcess == 0 {
		log.Infof("no CVEs need translation")
		return nil
	}

	// Adjust concurrent workers
	concurrent := config.Concurrent
	if concurrent > needProcess {
		concurrent = needProcess
	}
	log.Infof("using %d concurrent workers", concurrent)

	// Create task and result channels
	taskChan := make(chan *cveTranslationTask, needProcess)
	resultChan := make(chan *cveTranslationResult, needProcess)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for task := range taskChan {
				result := processCVETranslation(ctx, db, task, config.aiOpts)
				resultChan <- result
			}
		}(i)
	}

	// Send tasks
	go func() {
		for i, cve := range cvesToProcess {
			prompt := generateCVETranslationPrompt(cve)
			taskChan <- &cveTranslationTask{
				cve:    cve,
				prompt: prompt,
				index:  i + 1,
				total:  needProcess,
			}
		}
		close(taskChan)
	}()

	// Wait for all workers to finish and close result channel
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	successCount := 0
	errorCount := 0
	for result := range resultChan {
		if result.success {
			successCount++
		} else {
			errorCount++
		}
	}

	log.Infof("AI completion finished: %d total, %d success, %d skipped, %d errors",
		needProcess, successCount, skippedCount, errorCount)
	return nil
}

// processCVETranslation processes a single CVE translation task
func processCVETranslation(ctx context.Context, db *gorm.DB, task *cveTranslationTask, aiOpts []any) *cveTranslationResult {
	cve := task.cve
	log.Infof("processing %s (%d/%d): %s/%s", cve.CVE, task.index, task.total, cve.Vendor, cve.Product)

	result, err := invokeCVETranslationLiteForge(ctx, task.prompt, aiOpts...)
	if err != nil {
		log.Warnf("LiteForge failed for %s: %v", cve.CVE, err)
		return &cveTranslationResult{cve: cve, success: false, err: err}
	}

	// Extract fields from result
	if result != nil {
		if titleZh := result.GetString("title_zh"); titleZh != "" {
			cve.TitleZh = titleZh
		}
		if descZh := result.GetString("description_zh"); descZh != "" {
			cve.DescriptionMainZh = descZh
		}
		if solution := result.GetString("solution"); solution != "" && cve.Solution == "" {
			cve.Solution = solution
		}
	}

	// Validate that we got at least a title translation
	if cve.TitleZh == "" {
		log.Warnf("failed to extract Chinese title for %s", cve.CVE)
		return &cveTranslationResult{cve: cve, success: false, err: utils.Errorf("no title_zh extracted")}
	}

	// Save updated CVE to database using Table to bypass BeforeSave hook
	if err := db.Table("cves").Where("cve = ?", cve.CVE).Updates(map[string]interface{}{
		"title_zh":            cve.TitleZh,
		"description_main_zh": cve.DescriptionMainZh,
		"solution":            cve.Solution,
	}).Error; err != nil {
		log.Warnf("save %s failed: %v", cve.CVE, err)
		return &cveTranslationResult{cve: cve, success: false, err: err}
	}

	// Print translation result for user to verify quality
	log.Infof("%s completed:", cve.CVE)
	log.Infof("  [%s]: %s", cve.TitleZh, truncateString(cve.DescriptionMainZh, 100))
	if cve.Solution != "" {
		log.Infof("  [Solution]: %s", truncateString(cve.Solution, 100))
	}

	return &cveTranslationResult{cve: cve, success: true}
}

// invokeCVETranslationLiteForge calls AI using LiteForge with structured output schema
func invokeCVETranslationLiteForge(ctx context.Context, prompt string, opts ...any) (*aicommon.ForgeResult, error) {
	// Build LiteForge options with output schema
	var liteforgeOpts []any

	// Set context
	liteforgeOpts = append(liteforgeOpts, aicommon.WithContext(ctx))

	// Set output schema using aicommon.WithLiteForgeOutputSchemaFromAIToolOptions
	liteforgeOpts = append(liteforgeOpts, aicommon.WithLiteForgeOutputSchemaFromAIToolOptions(
		aitool.WithStringParam("title_zh",
			aitool.WithParam_Description("A concise Chinese title for this CVE vulnerability"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam("description_zh",
			aitool.WithParam_Description("Chinese translation of the CVE description"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam("solution",
			aitool.WithParam_Description("Brief solution or mitigation for this vulnerability in Chinese"),
		),
	))

	// Add user-provided options (like ai.type, ai.model, etc.)
	liteforgeOpts = append(liteforgeOpts, opts...)

	// Execute LiteForge
	result, err := aicommon.InvokeLiteForge(prompt, liteforgeOpts...)
	if err != nil {
		return nil, utils.Errorf("invoke liteforge failed: %v", err)
	}

	return result, nil
}

// generateCVETranslationPrompt generates a prompt for AI to translate CVE fields
func generateCVETranslationPrompt(cve *cveresources.CVE) string {
	var prompt strings.Builder

	prompt.WriteString("You are a security expert and translator. ")
	prompt.WriteString("Please translate the following CVE vulnerability information to Chinese. ")
	prompt.WriteString("Also provide a brief solution or mitigation for this vulnerability.\n\n")

	prompt.WriteString(fmt.Sprintf("CVE ID: %s\n", cve.CVE))
	if cve.CWE != "" {
		prompt.WriteString(fmt.Sprintf("CWE: %s\n", cve.CWE))
	}
	prompt.WriteString(fmt.Sprintf("Description: %s\n", cve.DescriptionMain))
	if cve.Severity != "" {
		prompt.WriteString(fmt.Sprintf("Severity: %s\n", cve.Severity))
	}
	if cve.Vendor != "" {
		prompt.WriteString(fmt.Sprintf("Vendor: %s\n", cve.Vendor))
	}
	if cve.Product != "" {
		prompt.WriteString(fmt.Sprintf("Product: %s\n", cve.Product))
	}
	if cve.BaseCVSSv2Score > 0 {
		prompt.WriteString(fmt.Sprintf("CVSS Score: %.1f\n", cve.BaseCVSSv2Score))
	}

	prompt.WriteString("\nPlease provide:\n")
	prompt.WriteString("1. title_zh: A concise Chinese title for this vulnerability (e.g., 'Apache Log4j 远程代码执行漏洞')\n")
	prompt.WriteString("2. description_zh: Chinese translation of the description\n")
	prompt.WriteString("3. solution: Brief solution or mitigation in Chinese\n")

	return prompt.String()
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ExportCVE exports all CVE entries to a JSONL file
// Each line is a JSON object representing a CVE entry
func ExportCVE(filename string) error {
	db := consts.GetGormCVEDatabase()
	if db == nil {
		return utils.Errorf("cannot get CVE database")
	}

	fp, err := os.Create(filename)
	if err != nil {
		return utils.Errorf("create file failed: %v", err)
	}
	defer fp.Close()

	ctx := context.Background()
	count := 0

	for cve := range cveresources.YieldCVEs(db, ctx) {
		data, err := json.Marshal(cve)
		if err != nil {
			log.Errorf("marshal %s failed: %v", cve.CVE, err)
			continue
		}
		fp.Write(data)
		fp.Write([]byte{'\n'})
		count++
	}

	log.Infof("exported %d CVE entries to %s", count, filename)
	return nil
}

// ImportCVE imports CVE entries from a JSONL file
// Each line should be a JSON object representing a CVE entry
func ImportCVE(filename string) error {
	db := consts.GetGormCVEDatabase()
	if db == nil {
		return utils.Errorf("cannot get CVE database")
	}

	// Auto migrate CVE table
	if err := db.AutoMigrate(&cveresources.CVE{}).Error; err != nil {
		return utils.Errorf("auto migrate CVE table failed: %v", err)
	}

	fp, err := os.Open(filename)
	if err != nil {
		return utils.Errorf("open file failed: %v", err)
	}
	defer fp.Close()

	scanner := bufio.NewReader(fp)
	count := 0
	errorCount := 0

	for {
		line, err := utils.BufioReadLine(scanner)
		if err != nil {
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return utils.Errorf("read line failed: %v", err)
		}

		if len(line) == 0 {
			continue
		}

		var cve cveresources.CVE
		if err := json.Unmarshal(line, &cve); err != nil {
			log.Errorf("unmarshal CVE failed: %v", err)
			errorCount++
			continue
		}

		// Save to database
		if err := db.Save(&cve).Error; err != nil {
			log.Errorf("save %s failed: %v", cve.CVE, err)
			errorCount++
			continue
		}
		count++
	}

	log.Infof("imported %d CVE entries from %s, %d errors", count, filename, errorCount)
	return nil
}
