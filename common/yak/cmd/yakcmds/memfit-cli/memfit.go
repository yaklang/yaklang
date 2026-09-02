package memfitcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const memfitWorkerEnvironment = "YAK_MEMFIT_WORKER"

var Command = &cli.Command{
	Name:  "memfit",
	Usage: "Run the local Memfit AI agent in an isolated worker process",
	UsageText: "yak memfit [options] [prompt]\n\n" +
		"Examples:\n" +
		"  yak memfit \"你好？你是什么模型？\"\n" +
		"  yak memfit --ai-type openai --api-key $OPENAI_API_KEY --model gpt-4.1\n" +
		"  yak memfit --print \"summarize this directory\"",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "ai-type",
			Usage: "AI provider override; empty uses the configured Yaklang model",
		},
		cli.StringFlag{
			Name:   "api-key",
			Usage:  "AI API key override (prefer the YAK_AI_API_KEY environment variable)",
			EnvVar: "YAK_AI_API_KEY",
		},
		cli.StringFlag{
			Name:  "model",
			Usage: "AI model override",
		},
		cli.StringFlag{
			Name:  "base-url",
			Usage: "AI API base URL override",
		},
		cli.StringFlag{
			Name:  "workdir,C",
			Usage: "agent working directory (default: current directory)",
		},
		cli.StringFlag{
			Name:  "language,lang",
			Value: "zh",
			Usage: "response language preference",
		},
		cli.StringFlag{
			Name:  "review",
			Value: "yolo",
			Usage: "tool review policy: yolo, ai, or manual (default: yolo)",
		},
		cli.IntFlag{
			Name:  "max-iterations",
			Value: 50,
			Usage: "maximum ReAct iterations per task",
		},
		cli.IntFlag{
			Name:  "timeout",
			Value: 0,
			Usage: "maximum worker lifetime in seconds (0 disables the limit)",
		},
		cli.StringFlag{
			Name:  "session",
			Usage: "persistent session ID (empty creates a new session)",
		},
		cli.BoolFlag{
			Name:  "print,p",
			Usage: "print one response and exit; also implied when output is not a terminal",
		},
		cli.BoolFlag{
			Name:  "plain",
			Usage: "disable ANSI styling and interactive TUI",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "show worker diagnostic logs and detailed events",
		},
	},
	Action: runMemfitCLI,
}

// WorkerCommand is registered outside cliGroup so its Hidden bit is not
// cleared by the legacy command grouping helper.
var WorkerCommand = &cli.Command{
	Name:   "memfit-worker",
	Hidden: true,
	Action: runMemfitWorkerCLI,
}

func runMemfitCLI(c *cli.Context) error {
	workdir := strings.TrimSpace(c.String("workdir"))
	if workdir == "" {
		var err error
		workdir, err = os.Getwd()
		if err != nil {
			return utils.Wrap(err, "resolve current working directory")
		}
	}
	absWorkdir, err := filepath.Abs(workdir)
	if err != nil {
		return utils.Wrap(err, "resolve memfit working directory")
	}
	stat, err := os.Stat(absWorkdir)
	if err != nil {
		return utils.Wrap(err, "stat memfit working directory")
	}
	if !stat.IsDir() {
		return utils.Errorf("memfit workdir is not a directory: %s", absWorkdir)
	}

	config := memfitStartConfig{
		AIType:        strings.TrimSpace(c.String("ai-type")),
		APIKey:        c.String("api-key"),
		Model:         strings.TrimSpace(c.String("model")),
		BaseURL:       strings.TrimSpace(c.String("base-url")),
		Workdir:       absWorkdir,
		Language:      strings.TrimSpace(c.String("language")),
		ReviewPolicy:  strings.ToLower(strings.TrimSpace(c.String("review"))),
		MaxIterations: c.Int("max-iterations"),
		Timeout:       c.Int("timeout"),
		SessionID:     strings.TrimSpace(c.String("session")),
		Debug:         c.Bool("debug"),
	}
	if err := validateMemfitStartConfig(config); err != nil {
		return err
	}

	query := strings.TrimSpace(strings.Join(c.Args(), " "))
	interactive := memfitCanUseTUI() && !c.Bool("print") && !c.Bool("plain")
	if query == "" && !interactive {
		data, readErr := io.ReadAll(io.LimitReader(os.Stdin, 16<<20))
		if readErr != nil {
			return utils.Wrap(readErr, "read memfit prompt from stdin")
		}
		query = strings.TrimSpace(string(data))
	}
	if query == "" && !interactive {
		return utils.Error("memfit prompt is required in print/plain mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := startMemfitProcessClient(ctx, config)
	if err != nil {
		return err
	}
	defer client.Close()

	if interactive {
		return runMemfitTUI(ctx, client, config, query)
	}
	return runMemfitPlain(ctx, client, config, query)
}

func runMemfitWorkerCLI(_ *cli.Context) error {
	if os.Getenv(memfitWorkerEnvironment) != "1" {
		return utils.Error("memfit worker is an internal command")
	}
	return runMemfitWorker(os.Stdin, os.Stdout)
}

func runMemfitWorker(input io.Reader, output io.Writer) error {
	protocol := newMemfitProtocolWriter(output)
	scanner := newMemfitFrameScanner(input)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return utils.Wrap(err, "read memfit start frame")
		}
		return io.EOF
	}
	first, err := decodeMemfitEnvelope(scanner.Bytes())
	if err != nil {
		_ = protocol.send("error", "", memfitStatus{Message: err.Error()})
		return err
	}
	if first.Type != "start" {
		err = fmt.Errorf("first memfit worker frame must be start, got %q", first.Type)
		_ = protocol.send("error", first.ID, memfitStatus{Message: err.Error()})
		return err
	}
	config, err := decodeMemfitPayload[memfitStartConfig](first)
	if err != nil {
		_ = protocol.send("error", first.ID, memfitStatus{Message: err.Error()})
		return err
	}
	if err := validateMemfitStartConfig(config); err != nil {
		_ = protocol.send("error", first.ID, memfitStatus{Message: err.Error()})
		return err
	}

	if config.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}

	var cancel context.CancelFunc
	ctx := context.Background()
	if config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(config.Timeout)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	options, err := buildMemfitEngineOptions(ctx, config, protocol)
	if err != nil {
		_ = protocol.send("error", first.ID, memfitStatus{Message: err.Error()})
		return err
	}
	engine, err := aiengine.NewAIEngine(options...)
	if err != nil {
		_ = protocol.send("error", first.ID, memfitStatus{Message: redactMemfitSecret(err.Error(), config.APIKey)})
		return utils.Wrap(err, "create memfit AI engine")
	}
	defer engine.Close()

	if err := protocol.send("ready", first.ID, memfitStatus{Message: "worker ready"}); err != nil {
		return err
	}

	frames := make(chan []byte)
	scanDone := make(chan error, 1)
	go func() {
		defer close(frames)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			select {
			case frames <- line:
			case <-ctx.Done():
				scanDone <- ctx.Err()
				return
			}
		}
		scanDone <- scanner.Err()
	}()

	for {
		select {
		case <-ctx.Done():
			message := fmt.Sprintf("memfit worker stopped: %v", ctx.Err())
			_ = protocol.send("error", "", memfitStatus{Message: message})
			return ctx.Err()
		case line, ok := <-frames:
			if !ok {
				if scanErr := <-scanDone; scanErr != nil && scanErr != context.Canceled {
					return utils.Wrap(scanErr, "read memfit protocol")
				}
				return nil
			}
			envelope, decodeErr := decodeMemfitEnvelope(line)
			if errorsIsNotProtocolFrame(decodeErr) {
				continue
			}
			if decodeErr != nil {
				_ = protocol.send("error", envelope.ID, memfitStatus{Message: decodeErr.Error()})
				continue
			}
			stop, handleErr := handleMemfitWorkerEnvelope(engine, protocol, envelope)
			if handleErr != nil {
				_ = protocol.send("error", envelope.ID, memfitStatus{Message: redactMemfitSecret(handleErr.Error(), config.APIKey)})
			}
			if stop {
				return handleErr
			}
		}
	}
}

func handleMemfitWorkerEnvelope(
	engine *aiengine.AIEngine,
	protocol *memfitProtocolWriter,
	envelope memfitEnvelope,
) (bool, error) {
	var err error
	switch envelope.Type {
	case "input":
		payload, decodeErr := decodeMemfitPayload[memfitInput](envelope)
		if decodeErr != nil || strings.TrimSpace(payload.Text) == "" {
			if decodeErr == nil {
				decodeErr = utils.Error("empty memfit input")
			}
			return false, decodeErr
		}
		err = engine.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: payload.Text})
		if err == nil {
			err = protocol.send("accepted", envelope.ID, memfitStatus{Message: "input accepted"})
		}
	case "interactive":
		payload, decodeErr := decodeMemfitPayload[memfitInput](envelope)
		if decodeErr != nil {
			err = decodeErr
		} else {
			err = engine.SendInputEvent(&ypb.AIInputEvent{
				IsInteractiveMessage: true,
				InteractiveId:        envelope.ID,
				InteractiveJSONInput: payload.Text,
			})
		}
	case "cancel":
		err = engine.SendInputEvent(&ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      aireact.SYNC_TYPE_REACT_CANCEL_CURRENT_TASK,
			SyncJsonInput: `{"reason":"cancelled from memfit TUI"}`,
		})
		if err == nil {
			err = protocol.send("status", envelope.ID, memfitStatus{Message: "cancellation requested"})
		}
	case "review":
		payload, decodeErr := decodeMemfitPayload[memfitReviewUpdate](envelope)
		if decodeErr != nil {
			err = decodeErr
		} else if validateErr := validateMemfitReviewPolicy(payload.Policy); validateErr != nil {
			err = validateErr
		} else {
			for _, event := range memfitReviewHotpatchEvents(payload.Policy) {
				err = engine.SendInputEvent(event)
				if err != nil {
					break
				}
			}
			if err == nil {
				err = protocol.send("status", envelope.ID, memfitStatus{Message: "review policy updated"})
			}
		}
	case "shutdown":
		return true, protocol.send("bye", envelope.ID, memfitStatus{Message: "worker stopped"})
	default:
		err = fmt.Errorf("unsupported memfit frame type %q", envelope.Type)
	}
	return false, err
}

func validateMemfitReviewPolicy(policy string) error {
	return validateMemfitStartConfig(memfitStartConfig{ReviewPolicy: policy})
}

func memfitReviewHotpatchEvents(policy string) []*ypb.AIInputEvent {
	return []*ypb.AIInputEvent{
		{
			IsConfigHotpatch: true,
			HotpatchType:     aicommon.HotPatchType_AgreePolicy,
			Params:           &ypb.AIStartParams{ReviewPolicy: policy},
		},
		{
			IsConfigHotpatch: true,
			HotpatchType:     aicommon.HotPatchType_AllowRequireForUserInteract,
			Params: &ypb.AIStartParams{
				DisallowRequireForUserPrompt: policy == "yolo",
			},
		},
	}
}

func errorsIsNotProtocolFrame(err error) bool {
	return err == errMemfitNotProtocolFrame
}

func buildMemfitEngineOptions(
	ctx context.Context,
	config memfitStartConfig,
	protocol *memfitProtocolWriter,
) ([]aiengine.AIEngineConfigOption, error) {
	options := []aiengine.AIEngineConfigOption{
		aiengine.WithContext(ctx),
		aiengine.WithWorkdir(config.Workdir),
		aiengine.WithLanguage(config.Language),
		aiengine.WithMaxIteration(config.MaxIterations),
		aiengine.WithReviewPolicy(config.ReviewPolicy),
		aiengine.WithAllowUserInteract(config.ReviewPolicy != "yolo"),
		aiengine.WithOnEvent(func(_ aicommon.AIEngineOperator, event *schema.AiOutputEvent) {
			forwardMemfitWorkerEvent(protocol, config.APIKey, event)
		}),
	}
	if config.SessionID != "" {
		options = append(options, aiengine.WithSessionID(config.SessionID))
	}
	if config.Debug {
		options = append(options, aiengine.WithDebugMode(true))
	}

	if config.AIType != "" || config.APIKey != "" || config.Model != "" || config.BaseURL != "" {
		aiType := config.AIType
		if aiType == "" {
			aiType = configuredMemfitProviderType()
		}
		if aiType == "" {
			return nil, utils.Error("--ai-type is required because no configured intelligent provider was found")
		}
		var aiOptions []aispec.AIConfigOption
		if config.APIKey != "" {
			aiOptions = append(aiOptions, aispec.WithAPIKey(config.APIKey))
		}
		if config.Model != "" {
			aiOptions = append(aiOptions, aispec.WithModel(config.Model))
		}
		if config.BaseURL != "" {
			aiOptions = append(aiOptions, aispec.WithBaseURL(config.BaseURL))
		}
		options = append(options, aiengine.WithAIConfig(aiType, aiOptions...))
	}
	return options, nil
}

func configuredMemfitProviderType() string {
	config := consts.GetTieredAIConfig()
	if config == nil {
		return ""
	}
	for _, model := range config.IntelligentConfigs {
		if model == nil || model.GetProvider() == nil {
			continue
		}
		if providerType := strings.TrimSpace(model.GetProvider().GetType()); providerType != "" {
			return providerType
		}
	}
	return ""
}

func forwardMemfitWorkerEvent(protocol *memfitProtocolWriter, apiKey string, event *schema.AiOutputEvent) {
	if event == nil {
		return
	}
	wireEvent := memfitWorkerEvent{
		Type:          string(event.Type),
		NodeID:        event.NodeId,
		TaskID:        event.TaskId,
		CallToolID:    event.CallToolID,
		IsSystem:      event.IsSystem,
		IsStream:      event.IsStream,
		IsReason:      event.IsReason,
		IsJSON:        event.IsJson,
		StreamDelta:   redactMemfitSecret(string(event.StreamDelta), apiKey),
		Content:       redactMemfitSecret(string(event.Content), apiKey),
		AIService:     event.AIService,
		AIModel:       event.AIModelName,
		ModelVerbose:  event.AIModelVerboseName,
		ContentType:   event.ContentType,
		VizSource:     event.VizSource,
		Timestamp:     event.Timestamp,
		DisableMarkup: event.DisableMarkdown,
	}
	_ = protocol.send("event", event.EventUUID, wireEvent)

	if event.Type != schema.EVENT_TYPE_STRUCTURED || event.NodeId != "react_task_status_changed" {
		return
	}
	var statusData map[string]any
	if json.Unmarshal(event.Content, &statusData) != nil {
		return
	}
	status := utils.InterfaceToString(statusData["react_task_now_status"])
	switch status {
	case "completed", "failed", "cancelled", "aborted", "skipped":
		_ = protocol.send("turn_done", "", memfitStatus{
			TaskID: utils.InterfaceToString(statusData["react_task_id"]),
			Status: status,
		})
	}
}
