package cvequeryops

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/antchfx/xmlquery"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/ziputil"

	// import aiforge to register liteforge callback
	_ "github.com/yaklang/yaklang/common/aiforge"
)

// DefaultCWEURL is the default URL to download CWE data from MITRE
const DefaultCWEURL = "https://cwe.mitre.org/data/xml/cwec_latest.xml.zip"

// DefaultFallbackProxy is the default proxy to use when direct download fails
const DefaultFallbackProxy = "http://127.0.0.1:7890"

// CWEUpdateConfig holds configuration for CWE update operation
type CWEUpdateConfig struct {
	URL   string
	Proxy string
}

// CWEUpdateOption is a function type for configuring CWE update
type CWEUpdateOption func(*CWEUpdateConfig)

// WithCWEProxy sets the proxy for CWE download
func WithCWEProxy(proxy string) CWEUpdateOption {
	return func(c *CWEUpdateConfig) {
		c.Proxy = proxy
	}
}

// WithCWEURL sets the URL for CWE download
func WithCWEURL(url string) CWEUpdateOption {
	return func(c *CWEUpdateConfig) {
		c.URL = url
	}
}

// CWEUpdate downloads and updates the CWE database
// Usage: cwe.Update() or cwe.Update(cwe.proxy("http://127.0.0.1:8080"), cwe.url("https://custom-url.com/cwe.zip"))
func CWEUpdate(opts ...CWEUpdateOption) error {
	config := &CWEUpdateConfig{
		URL: DefaultCWEURL,
	}
	for _, opt := range opts {
		opt(config)
	}

	log.Infof("start to download CWE data from: %s", config.URL)

	// Download CWE zip file with retry and fallback proxy
	zipPath, err := downloadCWEWithRetry(config)
	if err != nil {
		return utils.Errorf("download CWE data failed: %v", err)
	}
	log.Infof("CWE data downloaded to: %s", zipPath)

	// Verify the downloaded file exists and has content
	fileInfo, err := os.Stat(zipPath)
	if err != nil {
		return utils.Errorf("cannot stat downloaded file: %v", err)
	}
	if fileInfo.Size() == 0 {
		return utils.Errorf("downloaded file is empty")
	}
	log.Infof("downloaded file size: %d bytes", fileInfo.Size())

	// Parse CWE data
	cwes, err := LoadCWE(zipPath)
	if err != nil {
		return utils.Errorf("load CWE data failed: %v", err)
	}
	if len(cwes) == 0 {
		return utils.Errorf("no CWE entries found in the downloaded data")
	}
	log.Infof("parsed %d CWE entries", len(cwes))

	// Save to database
	db := consts.GetGormCVEDatabase()
	if db == nil {
		return utils.Errorf("cannot get CVE database")
	}

	// Auto migrate CWE table
	if err := db.AutoMigrate(&cveresources.CWE{}).Error; err != nil {
		return utils.Errorf("auto migrate CWE table failed: %v", err)
	}

	SaveCWE(db, cwes)
	log.Infof("CWE database updated successfully with %d entries", len(cwes))

	return nil
}

// downloadCWEWithRetry attempts to download CWE data with retry and fallback proxy
func downloadCWEWithRetry(config *CWEUpdateConfig) (string, error) {
	var lastErr error

	// First attempt with user-specified proxy (or no proxy)
	zipPath, err := downloadCWEWithConfig(config)
	if err == nil {
		return zipPath, nil
	}
	lastErr = err
	log.Warnf("first download attempt failed: %v", err)

	// If no proxy was specified, try with fallback proxy
	if config.Proxy == "" {
		log.Infof("trying with fallback proxy: %s", DefaultFallbackProxy)
		fallbackConfig := &CWEUpdateConfig{
			URL:   config.URL,
			Proxy: DefaultFallbackProxy,
		}
		zipPath, err = downloadCWEWithConfig(fallbackConfig)
		if err == nil {
			return zipPath, nil
		}
		lastErr = err
		log.Warnf("fallback proxy download attempt failed: %v", err)
	}

	return "", utils.Errorf("all download attempts failed, last error: %v", lastErr)
}

// downloadCWEWithConfig downloads CWE data with the given configuration
func downloadCWEWithConfig(config *CWEUpdateConfig) (string, error) {
	fp, err := consts.TempFile("cwe-latest-*.zip")
	if err != nil {
		return "", utils.Errorf("create temp file failed: %v", err)
	}
	filePath := fp.Name()

	var downloadErr error
	var bytesWritten int64
	var pocOpts []poc.PocConfigOption

	pocOpts = append(pocOpts,
		poc.WithSave(false),
		poc.WithNoBodyBuffer(true),
		poc.WithTimeout(120), // 120 seconds timeout for large file
		poc.WithRetryTimes(3),
		poc.WithBodyStreamReaderHandler(func(header []byte, bodyReader io.ReadCloser) {
			defer func() {
				if r := recover(); r != nil {
					downloadErr = utils.Errorf("panic during download: %v", r)
					log.Errorf("panic during CWE download: %v", r)
				}
				bodyReader.Close()
			}()

			n, copyErr := io.Copy(fp, bodyReader)
			bytesWritten = n
			if copyErr != nil {
				downloadErr = copyErr
				log.Errorf("copy CWE data failed: %v", copyErr)
			}
		}),
	)

	if config.Proxy != "" {
		log.Infof("using proxy: %s", config.Proxy)
		pocOpts = append(pocOpts, poc.WithProxy(config.Proxy))
	}

	_, _, err = poc.DoGET(config.URL, pocOpts...)

	// Close the file before checking for errors
	fp.Close()

	if err != nil {
		// Clean up the temp file on error
		os.Remove(filePath)
		return "", utils.Errorf("HTTP request failed: %v", err)
	}

	if downloadErr != nil {
		os.Remove(filePath)
		return "", utils.Errorf("download stream failed: %v", downloadErr)
	}

	if bytesWritten == 0 {
		os.Remove(filePath)
		return "", utils.Errorf("no data received from server")
	}

	log.Infof("downloaded %d bytes to %s", bytesWritten, filePath)
	return filePath, nil
}

// CWEAICompleteConfig holds configuration for AI completion
type CWEAICompleteConfig struct {
	Concurrent int   // Number of concurrent workers
	TestLimit  int   // Limit number of CWEs to process (0 = no limit)
	aiOpts     []any // AI options to pass to LiteForge
}

// CWEAICompleteOption is a function type for configuring AI completion
type CWEAICompleteOption func(*CWEAICompleteConfig)

// WithAIConcurrent sets the number of concurrent workers for AI completion
func WithAIConcurrent(n int) CWEAICompleteOption {
	return func(c *CWEAICompleteConfig) {
		if n > 0 {
			c.Concurrent = n
		}
	}
}

// WithTestLimit sets the maximum number of CWEs to process (for testing)
func WithTestLimit(n int) CWEAICompleteOption {
	return func(c *CWEAICompleteConfig) {
		if n > 0 {
			c.TestLimit = n
		}
	}
}

// cweTranslationTask represents a single CWE translation task
type cweTranslationTask struct {
	cwe    *cveresources.CWE
	prompt string
	index  int
	total  int
}

// cweTranslationResult represents the result of a translation task
type cweTranslationResult struct {
	cwe     *cveresources.CWE
	success bool
	err     error
}

// AICompleteFields uses AI to complete missing CWE fields like translations
// Usage:
//   - cwe.AICompleteFields() - use default settings
//   - cwe.AICompleteFields(ai.type("openai")) - specify AI type
//   - cwe.AICompleteFields(cwe.aiConcurrent(10)) - use 10 concurrent workers
//   - cwe.AICompleteFields(cwe.testLimit(5)) - only process 5 CWEs for testing
//   - cwe.AICompleteFields(cwe.aiConcurrent(10), cwe.testLimit(5), ai.type("openai"))
func AICompleteFields(opts ...any) error {
	// Parse options
	config := &CWEAICompleteConfig{
		Concurrent: 5, // Default: sequential processing
		TestLimit:  0, // Default: no limit
	}

	for _, opt := range opts {
		switch v := opt.(type) {
		case CWEAICompleteOption:
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

	// Count total CWEs to process
	var totalCount int
	if err := db.Model(&cveresources.CWE{}).Count(&totalCount).Error; err != nil {
		return utils.Errorf("count CWE entries failed: %v", err)
	}
	log.Infof("found %d CWE entries in database", totalCount)

	if totalCount == 0 {
		return utils.Errorf("no CWE entries found in database, please run cwe.Update() first")
	}

	// Collect CWEs that need translation
	var cwesToProcess []*cveresources.CWE
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	skippedCount := 0
	for cwe := range cveresources.YieldCWEs(db, ctx) {
		// Skip if already has Chinese translations
		if cwe.NameZh != "" && cwe.DescriptionZh != "" {
			skippedCount++
			continue
		}
		cwesToProcess = append(cwesToProcess, cwe)

		// Check test limit
		if config.TestLimit > 0 && len(cwesToProcess) >= config.TestLimit {
			log.Infof("test limit reached: %d CWEs", config.TestLimit)
			break
		}
	}

	needProcess := len(cwesToProcess)
	log.Infof("need to process %d CWEs, skipped %d (already translated)", needProcess, skippedCount)

	if needProcess == 0 {
		log.Infof("no CWEs need translation")
		return nil
	}

	// Adjust concurrent workers
	concurrent := config.Concurrent
	if concurrent > needProcess {
		concurrent = needProcess
	}
	log.Infof("using %d concurrent workers", concurrent)

	// Create task and result channels
	taskChan := make(chan *cweTranslationTask, needProcess)
	resultChan := make(chan *cweTranslationResult, needProcess)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for task := range taskChan {
				result := processCWETranslation(ctx, db, task, config.aiOpts)
				resultChan <- result
			}
		}(i)
	}

	// Send tasks
	go func() {
		for i, cwe := range cwesToProcess {
			prompt := generateCWETranslationPrompt(cwe)
			taskChan <- &cweTranslationTask{
				cwe:    cwe,
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

// processCWETranslation processes a single CWE translation task
func processCWETranslation(ctx context.Context, db *gorm.DB, task *cweTranslationTask, aiOpts []any) *cweTranslationResult {
	cwe := task.cwe
	log.Infof("processing CWE-%s (%d/%d): %s", cwe.IdStr, task.index, task.total, cwe.Name)

	result, err := invokeCWETranslationLiteForge(ctx, task.prompt, aiOpts...)
	if err != nil {
		log.Warnf("LiteForge failed for CWE-%s: %v", cwe.IdStr, err)
		return &cweTranslationResult{cwe: cwe, success: false, err: err}
	}

	// Extract fields from result
	if result != nil {
		if nameZh := result.GetString("name_zh"); nameZh != "" {
			cwe.NameZh = nameZh
		}
		if descZh := result.GetString("description_zh"); descZh != "" {
			cwe.DescriptionZh = descZh
		}
		if extDescZh := result.GetString("extended_description_zh"); extDescZh != "" {
			cwe.ExtendedDescriptionZh = extDescZh
		}
		if solution := result.GetString("solution"); solution != "" && cwe.CWESolution == "" {
			cwe.CWESolution = solution
		}
	}

	// Validate that we got at least a name translation
	if cwe.NameZh == "" {
		log.Warnf("failed to extract Chinese name for CWE-%s", cwe.IdStr)
		return &cweTranslationResult{cwe: cwe, success: false, err: utils.Errorf("no name_zh extracted")}
	}

	// Save updated CWE to database using Table to bypass BeforeSave hook
	if err := db.Table("cwes").Where("id_str = ?", cwe.IdStr).Updates(map[string]interface{}{
		"name_zh":                 cwe.NameZh,
		"description_zh":          cwe.DescriptionZh,
		"extended_description_zh": cwe.ExtendedDescriptionZh,
		"cwe_solution":            cwe.CWESolution,
	}).Error; err != nil {
		log.Warnf("save CWE-%s failed: %v", cwe.IdStr, err)
		return &cweTranslationResult{cwe: cwe, success: false, err: err}
	}

	// Print translation result for user to verify quality
	log.Infof("CWE-%s completed:", cwe.IdStr)
	log.Infof("  [%s]: %s", cwe.NameZh, cwe.DescriptionZh)
	if cwe.CWESolution != "" {
		log.Infof("  [Solution]: %s", cwe.CWESolution)
	}

	return &cweTranslationResult{cwe: cwe, success: true}
}

// invokeCWETranslationLiteForge calls AI using LiteForge with structured output schema
func invokeCWETranslationLiteForge(ctx context.Context, prompt string, opts ...any) (*aicommon.ForgeResult, error) {
	// Build LiteForge options with output schema
	var liteforgeOpts []any

	// Set context
	liteforgeOpts = append(liteforgeOpts, aicommon.WithContext(ctx))

	// Set output schema using aicommon.WithLiteForgeOutputSchemaFromAIToolOptions
	liteforgeOpts = append(liteforgeOpts, aicommon.WithLiteForgeOutputSchemaFromAIToolOptions(
		aitool.WithStringParam("name_zh",
			aitool.WithParam_Description("Chinese translation of the CWE name"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam("description_zh",
			aitool.WithParam_Description("Chinese translation of the CWE description"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam("extended_description_zh",
			aitool.WithParam_Description("Chinese translation of the extended description (if any)"),
		),
		aitool.WithStringParam("solution",
			aitool.WithParam_Description("Brief solution or mitigation for this weakness in Chinese"),
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

// generateCWETranslationPrompt generates a prompt for AI to translate CWE fields
func generateCWETranslationPrompt(cwe *cveresources.CWE) string {
	var prompt strings.Builder

	prompt.WriteString("You are a security expert and translator. ")
	prompt.WriteString("Please translate the following CWE (Common Weakness Enumeration) information to Chinese. ")
	prompt.WriteString("Also provide a brief solution or mitigation for this weakness.\n\n")

	prompt.WriteString(fmt.Sprintf("CWE ID: %s\n", cwe.CWEString()))
	prompt.WriteString(fmt.Sprintf("Name: %s\n", cwe.Name))
	prompt.WriteString(fmt.Sprintf("Description: %s\n", cwe.Description))

	if cwe.ExtendedDescription != "" {
		prompt.WriteString(fmt.Sprintf("Extended Description: %s\n", cwe.ExtendedDescription))
	}

	prompt.WriteString("\nPlease provide:\n")
	prompt.WriteString("1. name_zh: Chinese translation of the name\n")
	prompt.WriteString("2. description_zh: Chinese translation of the description\n")
	prompt.WriteString("3. extended_description_zh: Chinese translation of extended description (if any)\n")
	prompt.WriteString("4. solution: Brief solution or mitigation in Chinese\n")

	return prompt.String()
}

// ListAllCWE returns a channel that yields all CWE entries from the database
func ListAllCWE() chan *cveresources.CWE {
	db := consts.GetGormCVEDatabase()
	if db == nil {
		log.Error("cannot found CVE database")
		ch := make(chan *cveresources.CWE)
		close(ch)
		return ch
	}
	return cveresources.YieldCWEs(db, context.Background())
}

func DownloadCWE() (string, error) {
	fp, err := consts.TempFile("cwe-latest-*.zip")
	if err != nil {
		return "", err
	}
	defer fp.Close()

	// 使用流式处理下载 CWE zip 文件，避免大文件占用内存
	var downloadErr error
	// https://cwe.mitre.org/data/xml/cwec_latest.xml.zip
	_, _, err = poc.DoGET(DefaultCWEURL,
		poc.WithSave(false),        // 禁用 HTTP 流保存到数据库
		poc.WithNoBodyBuffer(true), // 禁用响应体缓冲
		poc.WithBodyStreamReaderHandler(func(header []byte, bodyReader io.ReadCloser) {
			defer bodyReader.Close()

			// 流式复制到临时文件
			_, copyErr := io.Copy(fp, bodyReader)
			if copyErr != nil {
				downloadErr = copyErr
				log.Errorf("copy cwe data failed: %v", copyErr)
			}
		}))

	if err != nil {
		log.Errorf("download mitre cwe failed: %s", err)
		return "", err
	}

	if downloadErr != nil {
		log.Errorf("save mitre cwe failed: %s", downloadErr)
		return "", downloadErr
	}

	return fp.Name(), nil
}

func SaveCWE(db *gorm.DB, cwes []*cveresources.CWE) {
	for _, i := range cwes {
		// log.Infof("start save cwe: %v", i.CWEString())
		if d := db.Model(&cveresources.CWE{}).Save(i); d.Error != nil {
			log.Errorf("save error: %s", d.Error)
		}
	}
}

func LoadCWE(cweXMLPath string) ([]*cveresources.CWE, error) {
	extracted := filepath.Join(consts.GetDefaultYakitBaseTempDir(), "cwe")
	err := ziputil.DeCompress(cweXMLPath, extracted)
	if err != nil {
		return nil, err
	}

	var targetPath string
	infos, err := utils.ReadDir(extracted)
	if err != nil {
		return nil, utils.Errorf("read extracted directory failed: %v", err)
	}
	for _, i := range infos {
		if i.IsDir {
			continue
		}
		matched, _ := regexp.MatchString(`cwec_(.*?)\.xml`, i.Name)
		if matched {
			targetPath = i.Path
			break
		}
	}
	if targetPath == "" {
		return nil, utils.Errorf("Target Path: %v is not existed or un-zip failed", cweXMLPath)
	}

	raw, err := ioutil.ReadFile(targetPath)
	if err != nil {
		return nil, err
	}
	node, err := xmlquery.Parse(bytes.NewBuffer(raw))
	if err != nil {
		return nil, err
	}

	var cwes []*cveresources.CWE
	xmlquery.FindEach(node, `//Weaknesses/Weakness`, func(i int, cweInstance *xmlquery.Node) {
		cwe := &cveresources.CWE{}
		cwe.IdStr = cweInstance.SelectAttr("ID")
		cwe.Id, _ = strconv.Atoi(cwe.IdStr)
		cwe.Name = cweInstance.SelectAttr("Name")
		cwe.Abstraction = cweInstance.SelectAttr("Abstraction")
		cwe.Status = cweInstance.SelectAttr("Status")

		if ret := xmlquery.FindOne(cweInstance, `//Description`); ret != nil {
			cwe.Description = ret.InnerText()
		}
		var descEx []string
		xmlquery.FindEach(cweInstance, `//Extended_Description`, func(i int, node *xmlquery.Node) {
			descEx = append(descEx, node.InnerText())
		})
		xmlquery.FindEach(cweInstance, `//Extended_Description/p`, func(i int, node *xmlquery.Node) {
			descEx = append(descEx, node.InnerText())
		})
		cwe.ExtendedDescription = strings.Join(descEx, "\n")
		cwe.ExtendedDescription = strings.TrimSpace(cwe.ExtendedDescription)

		var inferTo []string
		var siblings []string
		var requires []string
		var parent []string
		xmlquery.FindEach(cweInstance, `//Related_Weaknesses/Related_Weakness`, func(i int, node *xmlquery.Node) {
			idStr := strings.TrimSpace(node.SelectAttr(`CWE_ID`))
			id, _ := strconv.Atoi(idStr)
			if id <= 0 {
				return
			}
			switch ret := strings.ToLower(node.SelectAttr("Nature")); ret {
			case "childof":
				if !utils.StringArrayContains(parent, idStr) {
					parent = append(parent, idStr)
				}
			case "peerof", "canalsobe":
				if !utils.StringArrayContains(siblings, idStr) {
					siblings = append(siblings, idStr)
				}
			case "canprecede":
				if !utils.StringArrayContains(inferTo, idStr) {
					inferTo = append(inferTo, idStr)
				}
			case "requires", "startswith":
				if !utils.StringArrayContains(requires, idStr) {
					requires = append(requires, idStr)
				}
			default:
				log.Infof("unhandled relation")
				return
			}
		})
		cwe.InferTo = strings.Join(inferTo, ",")
		cwe.Siblings = strings.Join(siblings, ",")
		cwe.Requires = strings.Join(requires, ",")
		cwe.Parent = strings.Join(parent, ",")

		var langs []string
		xmlquery.FindEach(cweInstance, `//Applicable_Platforms/Language`, func(i int, node *xmlquery.Node) {
			if a := node.SelectAttr("Name"); a != "" {
				langs = append(langs, a)
			}
		})
		cwe.RelativeLanguage = strings.Join(langs, ",")
		var cves []string
		xmlquery.FindEach(cweInstance, `//Observed_Examples/Observed_Example/Reference`, func(i int, node *xmlquery.Node) {
			if ret := strings.TrimSpace(node.InnerText()); ret != "" {
				cves = append(cves, ret)
			}
		})
		cwe.CVEExamples = strings.Join(cves, ",")
		var capec []string
		xmlquery.FindEach(cweInstance, `//Related_Attack_Patterns/Related_Attack_Pattern`, func(i int, node *xmlquery.Node) {
			if ret := node.SelectAttr("CAPEC_ID"); ret != "" {
				id, _ := strconv.Atoi(ret)
				if id > 0 {
					capec = append(capec, ret)
				}
			}
		})
		cwe.CAPECVectors = strings.Join(capec, ",")
		cwes = append(cwes, cwe)
	})
	return cwes, nil
}
