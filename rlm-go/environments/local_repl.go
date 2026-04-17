package environments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/O-guardiao/arkhe-go/rlm-go/protocol"
	"github.com/O-guardiao/arkhe-go/rlm-go/types"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

type LocalREPL struct {
	config                Config
	lmHandlerAddress      string
	subcallFn             SubcallFn
	originalCWD           string
	tempDir               string
	cleanupTempDir        bool
	contextCount          int
	historyCount          int
	compaction            bool
	maxConcurrentSubcalls int
	customTools           map[string]any
	customSubTools        map[string]any
	writeContextFiles     bool

	mu                sync.Mutex
	interpreter       *interp.Interpreter
	stdoutProxy       *bufferProxy
	stderrProxy       *bufferProxy
	pendingLLMCalls   []types.RLMChatCompletion
	lastFinalAnswer   *string
	locals            map[string]any
	trackedNames      map[string]struct{}
	compactionHistory []any
}

type bufferProxy struct {
	mu     sync.Mutex
	target io.Writer
}

func (w *bufferProxy) SetTarget(target io.Writer) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.target = target
}

func (w *bufferProxy) Write(p []byte) (int, error) {
	w.mu.Lock()
	target := w.target
	w.mu.Unlock()
	if target == nil {
		return len(p), nil
	}
	return target.Write(p)
}

func NewLocalREPL(config Config) (*LocalREPL, error) {
	if config.MaxConcurrentSubcalls <= 0 {
		config.MaxConcurrentSubcalls = 4
	}
	if config.CustomSubTools == nil {
		config.CustomSubTools = config.CustomTools
	}
	if err := ValidateCustomTools(config.CustomTools); err != nil {
		return nil, err
	}

	workingDir := filepath.Clean(strings.TrimSpace(config.WorkingDir))
	cleanupTempDir := false
	if workingDir == "." || workingDir == "" {
		var err error
		workingDir, err = os.MkdirTemp("", "rlm_go_repl_*")
		if err != nil {
			return nil, err
		}
		cleanupTempDir = true
	} else {
		info, err := os.Stat(workingDir)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("working dir %q is not a directory", workingDir)
		}
	}

	writeContextFiles := config.WriteContextFiles
	if cleanupTempDir {
		writeContextFiles = true
	}

	repl := &LocalREPL{
		config:                config,
		lmHandlerAddress:      config.LMHandlerAddress,
		subcallFn:             config.SubcallFn,
		originalCWD:           mustGetwd(),
		tempDir:               workingDir,
		cleanupTempDir:        cleanupTempDir,
		compaction:            config.Compaction,
		maxConcurrentSubcalls: config.MaxConcurrentSubcalls,
		customTools:           config.CustomTools,
		customSubTools:        config.CustomSubTools,
		writeContextFiles:     writeContextFiles,
		stdoutProxy:           &bufferProxy{},
		stderrProxy:           &bufferProxy{},
		locals:                map[string]any{},
		trackedNames:          map[string]struct{}{},
	}
	if err := repl.Setup(); err != nil {
		_ = repl.Cleanup()
		return nil, err
	}
	if repl.compaction {
		repl.compactionHistory = []any{}
		_, _ = repl.declareValue("history", repl.compactionHistory)
	}
	if config.ContextPayload != nil {
		if err := repl.LoadContext(config.ContextPayload); err != nil {
			_ = repl.Cleanup()
			return nil, err
		}
	}
	if config.SetupCode != "" {
		repl.ExecuteCode(config.SetupCode)
	}
	return repl, nil
}

func (r *LocalREPL) Setup() error {
	r.interpreter = interp.New(interp.Options{
		Stdout: r.stdoutProxy,
		Stderr: r.stderrProxy,
	})
	r.interpreter.Use(stdlib.Symbols)

	exports := interp.Exports{
		"rlmbridge/rlmbridge": {
			"RLMFinalVar":              reflect.ValueOf(func(value any) string { return r.finalVar(value) }),
			"RLMShowVars":              reflect.ValueOf(func() string { return r.showVars() }),
			"RLMQuery":                 reflect.ValueOf(func(prompt string, model string) string { return r.llmQuery(prompt, model) }),
			"RLMQueryBatched":          reflect.ValueOf(func(prompts []string, model string) []string { return r.llmQueryBatched(prompts, model) }),
			"RLMRecursiveQuery":        reflect.ValueOf(func(prompt string, model string) string { return r.rlmQuery(prompt, model) }),
			"RLMRecursiveQueryBatched": reflect.ValueOf(func(prompts []string, model string) []string { return r.rlmQueryBatched(prompts, model) }),
			"RLMPrint":                 reflect.ValueOf(func(args ...any) { fmt.Fprintln(r.stdoutProxy, args...) }),
			"RLMDecodeJSON":            reflect.ValueOf(func(raw string) any { return mustDecodeJSON(raw) }),
		},
	}
	if err := r.interpreter.Use(exports); err != nil {
		return err
	}
	if _, err := r.interpreter.Eval(`import . "rlmbridge/rlmbridge"`); err != nil {
		return err
	}
	if _, err := r.interpreter.Eval(`
var FINAL_VAR = RLMFinalVar
var SHOW_VARS = RLMShowVars
var llm_query = RLMQuery
var llm_query_batched = RLMQueryBatched
var rlm_query = RLMRecursiveQuery
var rlm_query_batched = RLMRecursiveQueryBatched
var print = RLMPrint
`); err != nil {
		return err
	}

	for _, name := range []string{
		"FINAL_VAR", "SHOW_VARS", "llm_query", "llm_query_batched", "rlm_query", "rlm_query_batched", "print",
	} {
		r.trackedNames[name] = struct{}{}
	}
	if err := r.installCustomTools(); err != nil {
		return err
	}
	return r.refreshLocals()
}

func (r *LocalREPL) installCustomTools() error {
	if len(r.customTools) == 0 {
		return nil
	}
	exports := interp.Exports{
		"rlmcustom/rlmcustom": {},
	}
	declarations := []string{`import . "rlmcustom/rlmcustom"`}
	for name, entry := range r.customTools {
		value := ExtractToolValue(entry)
		exportName := customExportName(name)
		exports["rlmcustom/rlmcustom"][exportName] = reflect.ValueOf(value)
		if _, ok := value.(func()); ok {
			declarations = append(declarations, fmt.Sprintf("var %s = %s", name, exportName))
			r.trackedNames[name] = struct{}{}
			continue
		}
		declarations = append(declarations, fmt.Sprintf("var %s = %s", name, exportName))
		r.trackedNames[name] = struct{}{}
	}
	if err := r.interpreter.Use(exports); err != nil {
		return err
	}
	for _, declaration := range declarations {
		if _, err := r.interpreter.Eval(declaration); err != nil {
			return err
		}
	}
	return r.refreshLocals()
}

func customExportName(name string) string {
	name = regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(name, "_")
	if name == "" {
		return "ToolValue"
	}
	return "Tool_" + strings.ToUpper(name[:1]) + name[1:]
}

func (r *LocalREPL) LoadContext(contextPayload any) error {
	_, err := r.AddContext(contextPayload, intPtr(0))
	return err
}

func (r *LocalREPL) AddContext(contextPayload any, contextIndex *int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	index := r.contextCount
	if contextIndex != nil {
		index = *contextIndex
	}
	if err := r.writeContextFile(contextPayload, index); err != nil {
		return 0, err
	}

	varName := fmt.Sprintf("context_%d", index)
	if _, err := r.declareValueLocked(varName, contextPayload); err != nil {
		return 0, err
	}
	if index == 0 {
		if _, err := r.declareValueLocked("context", contextPayload); err != nil {
			return 0, err
		}
	}
	if index >= r.contextCount {
		r.contextCount = index + 1
	}
	return index, r.refreshLocalsLocked()
}

func (r *LocalREPL) AddHistory(messageHistory []map[string]any, historyIndex *int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	index := r.historyCount
	if historyIndex != nil {
		index = *historyIndex
	}
	deepCopy, err := deepCopyMessages(messageHistory)
	if err != nil {
		return 0, err
	}

	varName := fmt.Sprintf("history_%d", index)
	if _, err := r.declareValueLocked(varName, deepCopy); err != nil {
		return 0, err
	}
	if index == 0 && !r.compaction {
		if _, err := r.declareValueLocked("history", deepCopy); err != nil {
			return 0, err
		}
	}
	if index >= r.historyCount {
		r.historyCount = index + 1
	}
	return index, r.refreshLocalsLocked()
}

func (r *LocalREPL) AppendCompactionEntry(entry any) {
	if !r.compaction {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.compactionHistory = append(r.compactionHistory, entry)
	_, _ = r.setValueLocked("history", goLiteralOrJSON(r.compactionHistory), false)
	_ = r.refreshLocalsLocked()
}

func (r *LocalREPL) UpdateHandlerAddress(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lmHandlerAddress = address
}

func (r *LocalREPL) GetContextCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.contextCount
}

func (r *LocalREPL) GetHistoryCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.historyCount
}

func (r *LocalREPL) ExecuteCode(code string) types.REPLResult {
	start := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.pendingLLMCalls = nil
	r.lastFinalAnswer = nil
	definedNames := extractDefinedNames(code)

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	r.stdoutProxy.SetTarget(stdoutBuf)
	r.stderrProxy.SetTarget(stderrBuf)
	defer func() {
		r.stdoutProxy.SetTarget(nil)
		r.stderrProxy.SetTarget(nil)
	}()

	restoreCWD, err := chdir(r.tempDir)
	if err == nil {
		defer restoreCWD()
	}

	if _, evalErr := r.interpreter.Eval(code); evalErr != nil {
		if stderrBuf.Len() > 0 {
			stderrBuf.WriteString("\n")
		}
		stderrBuf.WriteString(evalErr.Error())
	}

	for _, name := range definedNames {
		r.trackedNames[name] = struct{}{}
	}
	r.restoreScaffoldLocked()
	_ = r.refreshLocalsLocked()

	localsCopy := map[string]any{}
	for key, value := range r.locals {
		localsCopy[key] = types.SerializeValue(value)
	}
	rlmCalls := append([]types.RLMChatCompletion(nil), r.pendingLLMCalls...)
	return types.REPLResult{
		Stdout:        stdoutBuf.String(),
		Stderr:        stderrBuf.String(),
		Locals:        localsCopy,
		ExecutionTime: time.Since(start).Seconds(),
		RLMCalls:      rlmCalls,
		FinalAnswer:   r.lastFinalAnswer,
	}
}

func (r *LocalREPL) ResolveFinalValue(name string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.locals[name]
	if !ok {
		return "", false
	}
	return stringifyValue(value), true
}

func (r *LocalREPL) Cleanup() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.locals = map[string]any{}
	r.trackedNames = map[string]struct{}{}
	if r.cleanupTempDir && r.tempDir != "" {
		_ = os.RemoveAll(r.tempDir)
	}
	return nil
}

func (r *LocalREPL) finalVar(variableName any) string {
	switch value := variableName.(type) {
	case string:
		trimmed := strings.TrimSpace(strings.Trim(value, `"'`))
		if localValue, ok := r.locals[trimmed]; ok {
			answer := stringifyValue(localValue)
			r.lastFinalAnswer = &answer
			return answer
		}
		available := r.availableVariableNames()
		if len(available) > 0 {
			return fmt.Sprintf(
				"Error: Variable '%s' not found. Available variables: %v. You must create and assign a variable BEFORE calling FINAL_VAR on it.",
				trimmed,
				available,
			)
		}
		return fmt.Sprintf(
			"Error: Variable '%s' not found. No variables have been created yet. You must create and assign a variable in a REPL block BEFORE calling FINAL_VAR on it.",
			trimmed,
		)
	default:
		answer := stringifyValue(value)
		r.lastFinalAnswer = &answer
		return answer
	}
}

func (r *LocalREPL) showVars() string {
	available := map[string]string{}
	for _, name := range r.availableVariableNames() {
		if value, ok := r.locals[name]; ok {
			available[name] = fmt.Sprintf("%T", value)
		}
	}
	if len(available) == 0 {
		return "No variables created yet. Use ```repl``` blocks to create variables."
	}
	return fmt.Sprintf("Available variables: %v", available)
}

func (r *LocalREPL) llmQuery(prompt string, model string) string {
	if r.lmHandlerAddress == "" {
		return "Error: No LM handler configured"
	}
	request := protocol.LMRequest{
		Prompt: prompt,
		Model:  emptyToNone(model),
		Depth:  r.config.Depth,
	}
	response := protocol.SendLMRequest(r.lmHandlerAddress, request, 300*time.Second)
	if !response.Success() || response.ChatCompletion == nil {
		if response.Error == "" {
			return "Error: no chat completion returned"
		}
		return "Error: " + response.Error
	}
	r.pendingLLMCalls = append(r.pendingLLMCalls, *response.ChatCompletion)
	return response.ChatCompletion.Response
}

func (r *LocalREPL) llmQueryBatched(prompts []string, model string) []string {
	if r.lmHandlerAddress == "" {
		out := make([]string, len(prompts))
		for i := range out {
			out[i] = "Error: No LM handler configured"
		}
		return out
	}
	inputs := make([]any, 0, len(prompts))
	for _, prompt := range prompts {
		inputs = append(inputs, prompt)
	}
	responses := protocol.SendLMRequestBatched(r.lmHandlerAddress, inputs, emptyToNone(model), 300*time.Second, r.config.Depth)
	out := make([]string, 0, len(responses))
	for _, response := range responses {
		if !response.Success() || response.ChatCompletion == nil {
			if response.Error == "" {
				out = append(out, "Error: no chat completion returned")
			} else {
				out = append(out, "Error: "+response.Error)
			}
			continue
		}
		r.pendingLLMCalls = append(r.pendingLLMCalls, *response.ChatCompletion)
		out = append(out, response.ChatCompletion.Response)
	}
	return out
}

func (r *LocalREPL) rlmQuery(prompt string, model string) string {
	if r.subcallFn == nil {
		return r.llmQuery(prompt, model)
	}
	completion, err := r.subcallFn(prompt, emptyToNone(model))
	if err != nil {
		return "Error: RLM query failed - " + err.Error()
	}
	r.pendingLLMCalls = append(r.pendingLLMCalls, completion)
	return completion.Response
}

func (r *LocalREPL) rlmQueryBatched(prompts []string, model string) []string {
	if r.subcallFn == nil {
		return r.llmQueryBatched(prompts, model)
	}
	if len(prompts) <= 1 {
		out := make([]string, 0, len(prompts))
		for _, prompt := range prompts {
			completion, err := r.subcallFn(prompt, emptyToNone(model))
			if err != nil {
				out = append(out, "Error: RLM query failed - "+err.Error())
				continue
			}
			r.pendingLLMCalls = append(r.pendingLLMCalls, completion)
			out = append(out, completion.Response)
		}
		return out
	}

	maxWorkers := r.maxConcurrentSubcalls
	if maxWorkers > len(prompts) {
		maxWorkers = len(prompts)
	}
	type result struct {
		index      int
		response   string
		completion *types.RLMChatCompletion
	}
	jobs := make(chan int)
	results := make(chan result, len(prompts))
	var wg sync.WaitGroup
	for worker := 0; worker < maxWorkers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				completion, err := r.subcallFn(prompts[index], emptyToNone(model))
				if err != nil {
					results <- result{index: index, response: "Error: RLM query failed - " + err.Error()}
					continue
				}
				completionCopy := completion
				results <- result{index: index, response: completion.Response, completion: &completionCopy}
			}
		}()
	}
	for index := range prompts {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	close(results)

	out := make([]string, len(prompts))
	ordered := make([]*types.RLMChatCompletion, len(prompts))
	for item := range results {
		out[item.index] = item.response
		ordered[item.index] = item.completion
	}
	for _, completion := range ordered {
		if completion != nil {
			r.pendingLLMCalls = append(r.pendingLLMCalls, *completion)
		}
	}
	return out
}

func (r *LocalREPL) writeContextFile(payload any, index int) error {
	if !r.writeContextFiles {
		return nil
	}
	switch value := payload.(type) {
	case string:
		return os.WriteFile(filepath.Join(r.tempDir, fmt.Sprintf("context_%d.txt", index)), []byte(value), 0o644)
	default:
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(r.tempDir, fmt.Sprintf("context_%d.json", index)), raw, 0o644)
	}
}

func (r *LocalREPL) declareValue(name string, value any) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.declareValueLocked(name, value)
}

func (r *LocalREPL) declareValueLocked(name string, value any) (string, error) {
	literal := goLiteralOrJSON(value)
	code := fmt.Sprintf("var %s = %s", name, literal)
	if _, exists := r.trackedNames[name]; exists {
		code = fmt.Sprintf("%s = %s", name, literal)
	}
	if _, err := r.interpreter.Eval(code); err != nil {
		return literal, err
	}
	r.trackedNames[name] = struct{}{}
	return literal, nil
}

func (r *LocalREPL) setValueLocked(name string, expression string, declareIfMissing bool) (string, error) {
	code := fmt.Sprintf("%s = %s", name, expression)
	if declareIfMissing {
		if _, ok := r.trackedNames[name]; !ok {
			code = fmt.Sprintf("var %s = %s", name, expression)
		}
	}
	if _, err := r.interpreter.Eval(code); err != nil {
		return expression, err
	}
	r.trackedNames[name] = struct{}{}
	return expression, nil
}

func (r *LocalREPL) restoreScaffoldLocked() {
	_, _ = r.setValueLocked("FINAL_VAR", "RLMFinalVar", false)
	_, _ = r.setValueLocked("SHOW_VARS", "RLMShowVars", false)
	_, _ = r.setValueLocked("llm_query", "RLMQuery", false)
	_, _ = r.setValueLocked("llm_query_batched", "RLMQueryBatched", false)
	_, _ = r.setValueLocked("rlm_query", "RLMRecursiveQuery", false)
	_, _ = r.setValueLocked("rlm_query_batched", "RLMRecursiveQueryBatched", false)
	_, _ = r.setValueLocked("print", "RLMPrint", false)
	if _, ok := r.trackedNames["context_0"]; ok {
		_, _ = r.setValueLocked("context", "context_0", true)
	}
	if r.compaction {
		_, _ = r.setValueLocked("history", goLiteralOrJSON(r.compactionHistory), true)
	} else if _, ok := r.trackedNames["history_0"]; ok {
		_, _ = r.setValueLocked("history", "history_0", true)
	}
}

func (r *LocalREPL) refreshLocals() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.refreshLocalsLocked()
}

func (r *LocalREPL) refreshLocalsLocked() error {
	keys := make([]string, 0, len(r.trackedNames))
	for key := range r.trackedNames {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, err := r.interpreter.Eval(key)
		if err != nil {
			continue
		}
		r.locals[key] = value.Interface()
	}
	return nil
}

func (r *LocalREPL) availableVariableNames() []string {
	names := []string{}
	for name := range r.locals {
		if strings.HasPrefix(name, "_") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func extractDefinedNames(code string) []string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^\s*var\s+([A-Za-z_][A-Za-z0-9_]*)`),
		regexp.MustCompile(`(?m)^\s*const\s+([A-Za-z_][A-Za-z0-9_]*)`),
		regexp.MustCompile(`(?m)^\s*func\s+([A-Za-z_][A-Za-z0-9_]*)`),
		regexp.MustCompile(`(?m)\b([A-Za-z_][A-Za-z0-9_]*)\s*:=`),
	}
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		for _, match := range pattern.FindAllStringSubmatch(code, -1) {
			if len(match) > 1 {
				seen[match[1]] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func deepCopyMessages(messageHistory []map[string]any) ([]map[string]any, error) {
	raw, err := json.Marshal(messageHistory)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	return out, json.Unmarshal(raw, &out)
}

func mustDecodeJSON(raw string) any {
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return raw
	}
	return out
}

func stringifyValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	}
	rv := reflect.ValueOf(value)
	if rv.IsValid() {
		switch rv.Kind() {
		case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct:
			raw, err := json.Marshal(value)
			if err == nil {
				return string(raw)
			}
		}
	}
	return fmt.Sprintf("%v", value)
}

func goLiteralOrJSON(value any) string {
	if literal, err := goLiteral(value); err == nil {
		return literal
	}
	raw, _ := json.Marshal(value)
	return fmt.Sprintf(`RLMDecodeJSON(%q)`, string(raw))
}

func goLiteral(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "any(nil)", nil
	case string:
		return fmt.Sprintf("%q", typed), nil
	case bool:
		if typed {
			return "true", nil
		}
		return "false", nil
	case int:
		return fmt.Sprintf("%d", typed), nil
	case int64:
		return fmt.Sprintf("%d", typed), nil
	case float64:
		return fmt.Sprintf("%v", typed), nil
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			items = append(items, fmt.Sprintf("%q", item))
		}
		return "[]string{" + strings.Join(items, ", ") + "}", nil
	case []map[string]any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			literal, err := goLiteral(item)
			if err != nil {
				return "", err
			}
			items = append(items, literal)
		}
		return "[]map[string]any{" + strings.Join(items, ", ") + "}", nil
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			literal, err := goLiteral(item)
			if err != nil {
				return "", err
			}
			items = append(items, literal)
		}
		return "[]any{" + strings.Join(items, ", ") + "}", nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		items := make([]string, 0, len(keys))
		for _, key := range keys {
			literal, err := goLiteral(typed[key])
			if err != nil {
				return "", err
			}
			items = append(items, fmt.Sprintf("%q: %s", key, literal))
		}
		return "map[string]any{" + strings.Join(items, ", ") + "}", nil
	default:
		rv := reflect.ValueOf(value)
		if !rv.IsValid() {
			return "any(nil)", nil
		}
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			items := make([]string, 0, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				literal, err := goLiteral(rv.Index(i).Interface())
				if err != nil {
					return "", err
				}
				items = append(items, literal)
			}
			return "[]any{" + strings.Join(items, ", ") + "}", nil
		case reflect.Map:
			iter := rv.MapRange()
			items := []string{}
			for iter.Next() {
				literal, err := goLiteral(iter.Value().Interface())
				if err != nil {
					return "", err
				}
				items = append(items, fmt.Sprintf("%q: %s", fmt.Sprint(iter.Key().Interface()), literal))
			}
			sort.Strings(items)
			return "map[string]any{" + strings.Join(items, ", ") + "}", nil
		}
		return "", fmt.Errorf("unsupported literal type: %T", value)
	}
}

func mustGetwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func chdir(target string) (func(), error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(target); err != nil {
		return nil, err
	}
	return func() {
		_ = os.Chdir(cwd)
	}, nil
}

func intPtr(value int) *int {
	return &value
}

func emptyToNone(value string) string {
	return strings.TrimSpace(value)
}

var _ PersistentEnvironment = (*LocalREPL)(nil)
var _ = context.Background
