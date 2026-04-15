package types

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type ModelUsageSummary struct {
	TotalCalls        int      `json:"total_calls"`
	TotalInputTokens  int      `json:"total_input_tokens"`
	TotalOutputTokens int      `json:"total_output_tokens"`
	TotalCost         *float64 `json:"total_cost,omitempty"`
}

type UsageSummary struct {
	ModelUsageSummaries map[string]ModelUsageSummary `json:"model_usage_summaries"`
}

func (u UsageSummary) TotalInputTokens() int {
	total := 0
	for _, summary := range u.ModelUsageSummaries {
		total += summary.TotalInputTokens
	}
	return total
}

func (u UsageSummary) TotalOutputTokens() int {
	total := 0
	for _, summary := range u.ModelUsageSummaries {
		total += summary.TotalOutputTokens
	}
	return total
}

func (u UsageSummary) TotalCostValue() *float64 {
	total := 0.0
	hasAny := false
	for _, summary := range u.ModelUsageSummaries {
		if summary.TotalCost != nil {
			total += *summary.TotalCost
			hasAny = true
		}
	}
	if !hasAny {
		return nil
	}
	return &total
}

type RLMChatCompletion struct {
	RootModel     string         `json:"root_model"`
	Prompt        any            `json:"prompt"`
	Response      string         `json:"response"`
	UsageSummary  UsageSummary   `json:"usage_summary"`
	ExecutionTime float64        `json:"execution_time"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type REPLResult struct {
	Stdout        string              `json:"stdout"`
	Stderr        string              `json:"stderr"`
	Locals        map[string]any      `json:"locals"`
	ExecutionTime float64             `json:"execution_time"`
	RLMCalls      []RLMChatCompletion `json:"rlm_calls"`
	FinalAnswer   *string             `json:"final_answer,omitempty"`
}

type CodeBlock struct {
	Code   string     `json:"code"`
	Result REPLResult `json:"result"`
}

type RLMIteration struct {
	Prompt        any         `json:"prompt"`
	Response      string      `json:"response"`
	CodeBlocks    []CodeBlock `json:"code_blocks"`
	FinalAnswer   *string     `json:"final_answer,omitempty"`
	IterationTime float64     `json:"iteration_time,omitempty"`
}

type RLMMetadata struct {
	RootModel         string         `json:"root_model"`
	MaxDepth          int            `json:"max_depth"`
	MaxIterations     int            `json:"max_iterations"`
	Backend           string         `json:"backend"`
	BackendKwargs     map[string]any `json:"backend_kwargs"`
	EnvironmentType   string         `json:"environment_type"`
	EnvironmentKwargs map[string]any `json:"environment_kwargs"`
	OtherBackends     []string       `json:"other_backends,omitempty"`
}

type QueryMetadata struct {
	ContextLengths     []int  `json:"context_lengths"`
	ContextTotalLength int    `json:"context_total_length"`
	ContextType        string `json:"context_type"`
}

func NewQueryMetadata(prompt any) (QueryMetadata, error) {
	meta := QueryMetadata{}
	switch value := prompt.(type) {
	case string:
		meta.ContextLengths = []int{len(value)}
		meta.ContextType = "str"
	case map[string]any:
		meta.ContextType = "dict"
		for _, chunk := range value {
			meta.ContextLengths = append(meta.ContextLengths, serializedLength(chunk))
		}
	case []string:
		meta.ContextType = "list"
		for _, chunk := range value {
			meta.ContextLengths = append(meta.ContextLengths, len(chunk))
		}
	case []map[string]any:
		meta.ContextType = "list"
		if len(value) == 0 {
			meta.ContextLengths = []int{0}
			break
		}
		for _, chunk := range value {
			if content, ok := chunk["content"]; ok {
				meta.ContextLengths = append(meta.ContextLengths, len(fmt.Sprint(content)))
			} else {
				meta.ContextLengths = append(meta.ContextLengths, serializedLength(chunk))
			}
		}
	case []any:
		meta.ContextType = "list"
		if len(value) == 0 {
			meta.ContextLengths = []int{0}
			break
		}
		for _, chunk := range value {
			meta.ContextLengths = append(meta.ContextLengths, serializedLength(chunk))
		}
	default:
		return QueryMetadata{}, fmt.Errorf("invalid prompt type: %T", prompt)
	}

	for _, length := range meta.ContextLengths {
		meta.ContextTotalLength += length
	}
	return meta, nil
}

func serializedLength(value any) int {
	switch typed := value.(type) {
	case string:
		return len(typed)
	case fmt.Stringer:
		return len(typed.String())
	default:
		raw, err := json.Marshal(value)
		if err == nil {
			return len(raw)
		}
		return len(fmt.Sprintf("%v", value))
	}
}

func SerializeValue(value any) any {
	switch typed := value.(type) {
	case nil, string, bool, int, int32, int64, float32, float64:
		return typed
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return nil
	}

	switch rv.Kind() {
	case reflect.Func:
		return fmt.Sprintf("<func %s>", runtimeName(value))
	case reflect.Map:
		out := map[string]any{}
		iter := rv.MapRange()
		for iter.Next() {
			out[fmt.Sprint(iter.Key().Interface())] = SerializeValue(iter.Value().Interface())
		}
		return out
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = SerializeValue(rv.Index(i).Interface())
		}
		return out
	case reflect.Struct:
		raw, err := json.Marshal(value)
		if err == nil {
			var decoded any
			if json.Unmarshal(raw, &decoded) == nil {
				return decoded
			}
		}
	}

	return fmt.Sprintf("%v", value)
}

func runtimeName(value any) string {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || rv.Kind() != reflect.Func {
		return reflect.TypeOf(value).String()
	}
	return rv.Type().String()
}
