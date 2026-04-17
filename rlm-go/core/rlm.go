package core

import (
	"context"
	"fmt"
	"time"

	"github.com/O-guardiao/arkhe-go/rlm-go/clients"
	"github.com/O-guardiao/arkhe-go/rlm-go/environments"
	"github.com/O-guardiao/arkhe-go/rlm-go/logger"
	"github.com/O-guardiao/arkhe-go/rlm-go/types"
	"github.com/O-guardiao/arkhe-go/rlm-go/utils"
)

type Config struct {
	Backend                string
	BackendKwargs          map[string]any
	ClientFactory          ClientFactory
	Environment            string
	EnvironmentKwargs      map[string]any
	Depth                  int
	MaxDepth               int
	MaxIterations          int
	MaxBudget              float64
	MaxTimeout             time.Duration
	MaxTokens              int
	MaxErrors              int
	CustomSystemPrompt     string
	OtherBackends          []string
	OtherBackendKwargs     []map[string]any
	Logger                 *logger.RLMLogger
	Persistent             bool
	CustomTools            map[string]any
	CustomSubTools         map[string]any
	Compaction             bool
	CompactionThresholdPct float64
	MaxConcurrentSubcalls  int
	OnSubcallStart         func(depth int, model string, promptPreview string)
	OnSubcallComplete      func(depth int, model string, duration float64, err string)
	OnIterationStart       func(depth int, iteration int)
	OnIterationComplete    func(depth int, iteration int, duration float64)
}

type ClientFactory func(backend string, backendKwargs map[string]any) (clients.Client, error)

type RLM struct {
	config            Config
	systemPrompt      string
	cumulativeCost    float64
	consecutiveErrors int
	lastError         string
	bestPartialAnswer string
	completionStart   time.Time
	persistentEnv     environments.PersistentEnvironment
}

func New(config Config) (*RLM, error) {
	if config.Backend == "" {
		config.Backend = "openai"
	}
	if config.Environment == "" {
		config.Environment = "local"
	}
	if config.MaxDepth <= 0 {
		config.MaxDepth = 1
	}
	if config.MaxIterations <= 0 {
		config.MaxIterations = 30
	}
	if config.CompactionThresholdPct <= 0 {
		config.CompactionThresholdPct = 0.85
	}
	if config.MaxConcurrentSubcalls <= 0 {
		config.MaxConcurrentSubcalls = 4
	}
	if config.CustomSubTools == nil {
		config.CustomSubTools = config.CustomTools
	}
	if config.Persistent && config.Environment != "local" {
		return nil, fmt.Errorf("persistent mode is only supported for local environment in the Go runtime")
	}

	rlm := &RLM{
		config:       config,
		systemPrompt: utils.RLMSystemPrompt,
	}
	if config.CustomSystemPrompt != "" {
		rlm.systemPrompt = config.CustomSystemPrompt
	}

	if config.Logger != nil {
		config.Logger.LogMetadata(types.RLMMetadata{
			RootModel:         rlm.rootModel(),
			MaxDepth:          config.MaxDepth,
			MaxIterations:     config.MaxIterations,
			Backend:           config.Backend,
			BackendKwargs:     cloneMap(config.BackendKwargs),
			EnvironmentType:   config.Environment,
			EnvironmentKwargs: cloneMap(config.EnvironmentKwargs),
			OtherBackends:     append([]string(nil), config.OtherBackends...),
		})
	}
	return rlm, nil
}

func (r *RLM) Completion(prompt any, rootPrompt string) (types.RLMChatCompletion, error) {
	start := time.Now()
	r.completionStart = start
	r.consecutiveErrors = 0
	r.lastError = ""
	r.bestPartialAnswer = ""

	if r.config.Depth >= r.config.MaxDepth {
		return r.fallbackAnswer(prompt)
	}

	if r.config.Logger != nil {
		r.config.Logger.ClearIterations()
	}

	handler, environment, cleanup, err := r.spawnCompletionContext(prompt)
	if err != nil {
		return types.RLMChatCompletion{}, err
	}
	defer cleanup()

	queryMetadata, err := types.NewQueryMetadata(prompt)
	if err != nil {
		return types.RLMChatCompletion{}, err
	}
	messageHistory := utils.BuildRLMSystemPrompt(r.systemPrompt, queryMetadata, r.config.CustomTools)
	if r.config.Compaction {
		messageHistory[0]["content"] = fmt.Sprint(messageHistory[0]["content"]) + "\n\nThe full conversation history is also available in the REPL variable `history`."
	}

	compactionCount := 0
	for i := 0; i < r.config.MaxIterations; i++ {
		if r.config.OnIterationStart != nil {
			r.config.OnIterationStart(r.config.Depth, i)
		}
		if err := r.checkTimeout(i, start); err != nil {
			return types.RLMChatCompletion{}, err
		}
		if r.config.Compaction {
			currentTokens, thresholdTokens, _ := r.getCompactionStatus(messageHistory)
			if currentTokens >= thresholdTokens {
				compactionCount++
				messageHistory, err = r.compactHistory(handler, environment, messageHistory, compactionCount)
				if err != nil {
					return types.RLMChatCompletion{}, err
				}
			}
		}

		contextCount := 1
		historyCount := 0
		if persistent, ok := environment.(environments.PersistentEnvironment); ok {
			contextCount = persistent.GetContextCount()
			historyCount = persistent.GetHistoryCount()
		}
		currentPrompt := append([]map[string]any{}, messageHistory...)
		currentPrompt = append(currentPrompt, utils.BuildUserPrompt(rootPrompt, i, contextCount, historyCount))

		iterationStart := time.Now()
		iteration, err := r.completionTurn(currentPrompt, handler, environment)
		if err != nil {
			return types.RLMChatCompletion{}, err
		}

		if err := r.checkIterationLimits(iteration, i, handler); err != nil {
			return types.RLMChatCompletion{}, err
		}

		var finalAnswer string
		finalFound := false
		for _, block := range iteration.CodeBlocks {
			if block.Result.FinalAnswer != nil {
				finalAnswer = *block.Result.FinalAnswer
				finalFound = true
				break
			}
		}
		if !finalFound {
			if resolver, ok := environment.(utils.FinalAnswerResolver); ok {
				if resolved, ok := utils.FindFinalAnswer(iteration.Response, resolver); ok {
					finalAnswer = resolved
					finalFound = true
				}
			} else if resolved, ok := utils.FindFinalAnswer(iteration.Response, nil); ok {
				finalAnswer = resolved
				finalFound = true
			}
		}
		if finalFound {
			iteration.FinalAnswer = &finalAnswer
		}

		if iteration.Response != "" {
			r.bestPartialAnswer = iteration.Response
		}
		if r.config.Logger != nil {
			r.config.Logger.Log(iteration)
		}
		if r.config.OnIterationComplete != nil {
			r.config.OnIterationComplete(r.config.Depth, i, time.Since(iterationStart).Seconds())
		}

		if finalFound {
			usage := handler.GetUsageSummary()
			if r.config.Persistent {
				if persistent, ok := environment.(environments.PersistentEnvironment); ok {
					_, _ = persistent.AddHistory(messageHistory, nil)
				}
			}
			return types.RLMChatCompletion{
				RootModel:     r.rootModel(),
				Prompt:        prompt,
				Response:      finalAnswer,
				UsageSummary:  usage,
				ExecutionTime: time.Since(start).Seconds(),
				Metadata:      getTrajectory(r.config.Logger),
			}, nil
		}

		newMessages := utils.FormatIteration(iteration, 20_000)
		messageHistory = append(messageHistory, newMessages...)
		if local, ok := environment.(*environments.LocalREPL); ok && r.config.Compaction {
			local.AppendCompactionEntry(newMessages)
		}
	}

	finalAnswer, err := r.defaultAnswer(messageHistory, handler)
	if err != nil {
		return types.RLMChatCompletion{}, err
	}
	if r.config.Persistent {
		if persistent, ok := environment.(environments.PersistentEnvironment); ok {
			_, _ = persistent.AddHistory(messageHistory, nil)
		}
	}
	return types.RLMChatCompletion{
		RootModel:     r.rootModel(),
		Prompt:        prompt,
		Response:      finalAnswer,
		UsageSummary:  handler.GetUsageSummary(),
		ExecutionTime: time.Since(start).Seconds(),
		Metadata:      getTrajectory(r.config.Logger),
	}, nil
}

func (r *RLM) Close() error {
	if r.persistentEnv != nil {
		return r.persistentEnv.Cleanup()
	}
	return nil
}

func (r *RLM) spawnCompletionContext(prompt any) (*LMHandler, environments.Environment, func(), error) {
	client, err := r.createClient(r.config.Backend, r.config.BackendKwargs)
	if err != nil {
		return nil, nil, nil, err
	}

	var otherClient clients.Client
	if len(r.config.OtherBackends) > 0 && len(r.config.OtherBackendKwargs) > 0 {
		otherClient, err = r.createClient(r.config.OtherBackends[0], r.config.OtherBackendKwargs[0])
		if err != nil {
			return nil, nil, nil, err
		}
	}

	handler := NewLMHandler(client, "127.0.0.1", otherClient, 16)
	if len(r.config.OtherBackends) > 0 {
		for index, backend := range r.config.OtherBackends {
			if index >= len(r.config.OtherBackendKwargs) {
				break
			}
			registered, err := r.createClient(backend, r.config.OtherBackendKwargs[index])
			if err != nil {
				return nil, nil, nil, err
			}
			handler.RegisterClient(registered.ModelName(), registered)
		}
	}

	address, err := handler.Start()
	if err != nil {
		return nil, nil, nil, err
	}

	var environment environments.Environment
	if r.config.Persistent && r.persistentEnv != nil {
		r.persistentEnv.UpdateHandlerAddress(address)
		if _, err := r.persistentEnv.AddContext(prompt, nil); err != nil {
			handler.Stop()
			return nil, nil, nil, err
		}
		environment = r.persistentEnv
	} else {
		envConfig := environments.Config{
			LMHandlerAddress:      address,
			ContextPayload:        prompt,
			Depth:                 r.config.Depth + 1,
			SubcallFn:             nil,
			CustomTools:           r.config.CustomTools,
			CustomSubTools:        r.config.CustomSubTools,
			Compaction:            r.config.Compaction,
			MaxConcurrentSubcalls: r.config.MaxConcurrentSubcalls,
			WorkingDir:            stringValue(r.config.EnvironmentKwargs, "working_dir"),
			WriteContextFiles:     boolValue(r.config.EnvironmentKwargs, "write_context_files", false),
		}
		if r.config.Environment == "local" && r.config.MaxDepth > 1 {
			envConfig.SubcallFn = func(prompt string, model string) (types.RLMChatCompletion, error) {
				return r.subcall(prompt, model)
			}
		}
		environment, err = environments.NewEnvironment(r.config.Environment, envConfig)
		if err != nil {
			handler.Stop()
			return nil, nil, nil, err
		}
		if r.config.Persistent {
			if persistent, ok := environment.(environments.PersistentEnvironment); ok {
				r.persistentEnv = persistent
			}
		}
	}

	cleanup := func() {
		handler.Stop()
		if !r.config.Persistent && environment != nil {
			_ = environment.Cleanup()
		}
	}
	return handler, environment, cleanup, nil
}

func (r *RLM) completionTurn(
	prompt []map[string]any,
	handler *LMHandler,
	environment environments.Environment,
) (types.RLMIteration, error) {
	start := time.Now()
	response, err := handler.Completion(prompt, "")
	if err != nil {
		return types.RLMIteration{}, err
	}
	codeBlocks := utils.FindCodeBlocks(response)
	results := make([]types.CodeBlock, 0, len(codeBlocks))
	for _, codeBlock := range codeBlocks {
		results = append(results, types.CodeBlock{
			Code:   codeBlock,
			Result: environment.ExecuteCode(codeBlock),
		})
	}
	return types.RLMIteration{
		Prompt:        prompt,
		Response:      response,
		CodeBlocks:    results,
		IterationTime: time.Since(start).Seconds(),
	}, nil
}

func (r *RLM) defaultAnswer(messageHistory []map[string]any, handler *LMHandler) (string, error) {
	currentPrompt := append([]map[string]any{}, messageHistory...)
	currentPrompt = append(currentPrompt, map[string]any{
		"role":    "assistant",
		"content": "Please provide a final answer to the user's question based on the information provided.",
	})
	response, err := handler.Completion(currentPrompt, "")
	if err != nil {
		return "", err
	}
	if r.config.Logger != nil {
		r.config.Logger.Log(types.RLMIteration{
			Prompt:      currentPrompt,
			Response:    response,
			FinalAnswer: &response,
			CodeBlocks:  nil,
		})
	}
	return response, nil
}

func (r *RLM) fallbackAnswer(prompt any) (types.RLMChatCompletion, error) {
	client, err := r.createClient(r.config.Backend, r.config.BackendKwargs)
	if err != nil {
		return types.RLMChatCompletion{}, err
	}
	start := time.Now()
	response, err := client.Completion(context.Background(), prompt, "")
	if err != nil {
		return types.RLMChatCompletion{}, err
	}
	return types.RLMChatCompletion{
		RootModel:     r.rootModel(),
		Prompt:        prompt,
		Response:      response,
		UsageSummary:  types.UsageSummary{ModelUsageSummaries: map[string]types.ModelUsageSummary{r.rootModel(): client.GetLastUsage()}},
		ExecutionTime: time.Since(start).Seconds(),
	}, nil
}

func (r *RLM) subcall(prompt string, model string) (types.RLMChatCompletion, error) {
	nextDepth := r.config.Depth + 1
	childBackendKwargs := cloneMap(r.config.BackendKwargs)
	if model != "" {
		childBackendKwargs["model_name"] = model
	}
	resolvedModel := model
	if resolvedModel == "" {
		resolvedModel = fmt.Sprint(childBackendKwargs["model_name"])
		if resolvedModel == "" {
			resolvedModel = r.rootModel()
		}
	}

	if nextDepth >= r.config.MaxDepth {
		var client clients.Client
		var err error
		if len(r.config.OtherBackends) > 0 && len(r.config.OtherBackendKwargs) > 0 {
			client, err = r.createClient(r.config.OtherBackends[0], r.config.OtherBackendKwargs[0])
		} else {
			client, err = r.createClient(r.config.Backend, childBackendKwargs)
		}
		if err != nil {
			return types.RLMChatCompletion{}, err
		}
		start := time.Now()
		response, err := client.Completion(context.Background(), prompt, model)
		if err != nil {
			return types.RLMChatCompletion{
				RootModel:     resolvedModel,
				Prompt:        prompt,
				Response:      "Error: LM query failed at max depth - " + err.Error(),
				UsageSummary:  types.UsageSummary{ModelUsageSummaries: map[string]types.ModelUsageSummary{}},
				ExecutionTime: time.Since(start).Seconds(),
			}, nil
		}
		return types.RLMChatCompletion{
			RootModel:     resolvedModel,
			Prompt:        prompt,
			Response:      response,
			UsageSummary:  types.UsageSummary{ModelUsageSummaries: map[string]types.ModelUsageSummary{resolvedModel: client.GetLastUsage()}},
			ExecutionTime: time.Since(start).Seconds(),
		}, nil
	}

	remainingBudget := 0.0
	if r.config.MaxBudget > 0 {
		remainingBudget = r.config.MaxBudget - r.cumulativeCost
		if remainingBudget <= 0 {
			return types.RLMChatCompletion{
				RootModel:     resolvedModel,
				Prompt:        prompt,
				Response:      fmt.Sprintf("Error: Budget exhausted (spent $%.6f of $%.6f)", r.cumulativeCost, r.config.MaxBudget),
				UsageSummary:  types.UsageSummary{ModelUsageSummaries: map[string]types.ModelUsageSummary{}},
				ExecutionTime: 0,
			}, nil
		}
	}
	remainingTimeout := time.Duration(0)
	if r.config.MaxTimeout > 0 {
		elapsed := time.Since(r.completionStart)
		remainingTimeout = r.config.MaxTimeout - elapsed
		if remainingTimeout <= 0 {
			return types.RLMChatCompletion{
				RootModel:     resolvedModel,
				Prompt:        prompt,
				Response:      fmt.Sprintf("Error: Timeout exhausted (%.1fs of %.1fs)", elapsed.Seconds(), r.config.MaxTimeout.Seconds()),
				UsageSummary:  types.UsageSummary{ModelUsageSummaries: map[string]types.ModelUsageSummary{}},
				ExecutionTime: 0,
			}, nil
		}
	}

	promptPreview := prompt
	if len(promptPreview) > 80 {
		promptPreview = promptPreview[:80]
	}
	if r.config.OnSubcallStart != nil {
		r.config.OnSubcallStart(nextDepth, resolvedModel, promptPreview)
	}

	childLogger := (*logger.RLMLogger)(nil)
	if r.config.Logger != nil {
		childLogger, _ = logger.New("", "")
	}

	childConfig := r.config
	childConfig.Depth = nextDepth
	childConfig.BackendKwargs = childBackendKwargs
	childConfig.Logger = childLogger
	childConfig.CustomTools = r.config.CustomSubTools
	childConfig.CustomSubTools = r.config.CustomSubTools
	if remainingBudget > 0 {
		childConfig.MaxBudget = remainingBudget
	}
	if remainingTimeout > 0 {
		childConfig.MaxTimeout = remainingTimeout
	}

	child, err := New(childConfig)
	if err != nil {
		return types.RLMChatCompletion{}, err
	}
	start := time.Now()
	result, runErr := child.Completion(prompt, "")
	if closeErr := child.Close(); closeErr != nil && runErr == nil {
		runErr = closeErr
	}
	if result.UsageSummary.TotalCostValue() != nil {
		r.cumulativeCost += *result.UsageSummary.TotalCostValue()
	}
	if r.config.OnSubcallComplete != nil {
		errString := ""
		if runErr != nil {
			errString = runErr.Error()
		}
		r.config.OnSubcallComplete(nextDepth, resolvedModel, time.Since(start).Seconds(), errString)
	}
	if runErr != nil {
		return types.RLMChatCompletion{
			RootModel:     resolvedModel,
			Prompt:        prompt,
			Response:      "Error: Child RLM completion failed - " + runErr.Error(),
			UsageSummary:  types.UsageSummary{ModelUsageSummaries: map[string]types.ModelUsageSummary{}},
			ExecutionTime: time.Since(start).Seconds(),
		}, nil
	}
	return result, nil
}

func (r *RLM) checkTimeout(iteration int, start time.Time) error {
	if r.config.MaxTimeout <= 0 {
		return nil
	}
	elapsed := time.Since(start)
	if elapsed > r.config.MaxTimeout {
		return utils.TimeoutExceededError{
			Elapsed:       elapsed.Seconds(),
			Timeout:       r.config.MaxTimeout.Seconds(),
			PartialAnswer: r.bestPartialAnswer,
			Msg: fmt.Sprintf(
				"timeout exceeded after iteration %d: %.1fs of %.1fs limit",
				iteration,
				elapsed.Seconds(),
				r.config.MaxTimeout.Seconds(),
			),
		}
	}
	return nil
}

func (r *RLM) checkIterationLimits(iteration types.RLMIteration, iterationNum int, handler *LMHandler) error {
	iterationHadError := false
	for _, codeBlock := range iteration.CodeBlocks {
		if codeBlock.Result.Stderr != "" {
			iterationHadError = true
			r.lastError = codeBlock.Result.Stderr
			break
		}
	}
	if iterationHadError {
		r.consecutiveErrors++
	} else {
		r.consecutiveErrors = 0
	}
	if r.config.MaxErrors > 0 && r.consecutiveErrors >= r.config.MaxErrors {
		return utils.ErrorThresholdExceededError{
			ErrorCount:    r.consecutiveErrors,
			Threshold:     r.config.MaxErrors,
			LastError:     r.lastError,
			PartialAnswer: r.bestPartialAnswer,
			Msg:           fmt.Sprintf("error threshold exceeded: %d consecutive errors (limit: %d)", r.consecutiveErrors, r.config.MaxErrors),
		}
	}

	currentUsage := handler.GetUsageSummary()
	if cost := currentUsage.TotalCostValue(); cost != nil {
		r.cumulativeCost = *cost
		if r.config.MaxBudget > 0 && r.cumulativeCost > r.config.MaxBudget {
			return utils.BudgetExceededError{
				Spent:  r.cumulativeCost,
				Budget: r.config.MaxBudget,
				Msg:    fmt.Sprintf("budget exceeded after iteration %d: spent $%.6f of $%.6f", iterationNum+1, r.cumulativeCost, r.config.MaxBudget),
			}
		}
	}
	if r.config.MaxTokens > 0 {
		totalTokens := currentUsage.TotalInputTokens() + currentUsage.TotalOutputTokens()
		if totalTokens > r.config.MaxTokens {
			return utils.TokenLimitExceededError{
				TokensUsed:    totalTokens,
				TokenLimit:    r.config.MaxTokens,
				PartialAnswer: r.bestPartialAnswer,
				Msg:           fmt.Sprintf("token limit exceeded after iteration %d: %d of %d tokens", iterationNum+1, totalTokens, r.config.MaxTokens),
			}
		}
	}
	return nil
}

func (r *RLM) getCompactionStatus(messageHistory []map[string]any) (int, int, int) {
	modelName := r.rootModel()
	maxTokens := utils.GetContextLimit(modelName)
	currentTokens := utils.CountTokens(messageHistory, modelName)
	thresholdTokens := int(float64(maxTokens) * r.config.CompactionThresholdPct)
	return currentTokens, thresholdTokens, maxTokens
}

func (r *RLM) compactHistory(
	handler *LMHandler,
	environment environments.Environment,
	messageHistory []map[string]any,
	compactionCount int,
) ([]map[string]any, error) {
	summaryPrompt := append([]map[string]any{}, messageHistory...)
	summaryPrompt = append(summaryPrompt, map[string]any{
		"role":    "user",
		"content": "Summarize your progress so far. Include completed work, remaining work, concrete intermediate results, and the next action. Be concise but preserve exact values.",
	})
	summary, err := handler.Completion(summaryPrompt, "")
	if err != nil {
		return nil, err
	}
	if local, ok := environment.(*environments.LocalREPL); ok {
		local.AppendCompactionEntry(map[string]any{"type": "summary", "content": summary})
	}
	return append(messageHistory[:2], []map[string]any{
		{"role": "assistant", "content": summary},
		{
			"role": "user",
			"content": fmt.Sprintf(
				"Your conversation has been compacted %d time(s). Continue from the summary above. Do not repeat finished work. Check SHOW_VARS() and history when needed. Your next action:",
				compactionCount,
			),
		},
	}...), nil
}

func (r *RLM) rootModel() string {
	if value, ok := r.config.BackendKwargs["model_name"]; ok {
		if text := fmt.Sprint(value); text != "" {
			return text
		}
	}
	return "unknown"
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := map[string]any{}
	for key, value := range input {
		out[key] = value
	}
	return out
}

func (r *RLM) createClient(backend string, backendKwargs map[string]any) (clients.Client, error) {
	clonedKwargs := cloneMap(backendKwargs)
	if r.config.ClientFactory != nil {
		return r.config.ClientFactory(backend, clonedKwargs)
	}
	return clients.NewClient(backend, clonedKwargs)
}

func stringValue(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, ok := input[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func boolValue(input map[string]any, key string, fallback bool) bool {
	if input == nil {
		return fallback
	}
	value, ok := input[key]
	if !ok {
		return fallback
	}
	typed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return typed
}

func getTrajectory(logger *logger.RLMLogger) map[string]any {
	if logger == nil {
		return nil
	}
	return logger.GetTrajectory()
}
