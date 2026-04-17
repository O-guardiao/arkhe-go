package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/O-guardiao/arkhe-go/rlm-go/clients"
	"github.com/O-guardiao/arkhe-go/rlm-go/protocol"
	"github.com/O-guardiao/arkhe-go/rlm-go/types"
)

type LMHandler struct {
	defaultClient      clients.Client
	otherBackendClient clients.Client
	host               string
	port               int
	listener           net.Listener
	wg                 sync.WaitGroup
	batchMaxConcurrent int

	clientsMu sync.RWMutex
	clients   map[string]clients.Client
}

func NewLMHandler(
	client clients.Client,
	host string,
	otherBackendClient clients.Client,
	batchMaxConcurrent int,
) *LMHandler {
	if host == "" {
		host = "127.0.0.1"
	}
	if batchMaxConcurrent <= 0 {
		batchMaxConcurrent = 16
	}
	handler := &LMHandler{
		defaultClient:      client,
		otherBackendClient: otherBackendClient,
		clients:            map[string]clients.Client{},
		host:               host,
		batchMaxConcurrent: batchMaxConcurrent,
	}
	handler.RegisterClient(client.ModelName(), client)
	return handler
}

func (h *LMHandler) RegisterClient(modelName string, client clients.Client) {
	if modelName == "" || client == nil {
		return
	}
	h.clientsMu.Lock()
	h.clients[modelName] = client
	h.clientsMu.Unlock()
}

func (h *LMHandler) GetClient(model string, depth int) clients.Client {
	if model != "" {
		h.clientsMu.RLock()
		client, ok := h.clients[model]
		h.clientsMu.RUnlock()
		if ok {
			return client
		}
	}
	if depth == 1 && h.otherBackendClient != nil {
		return h.otherBackendClient
	}
	return h.defaultClient
}

func (h *LMHandler) Start() (string, error) {
	if h.listener != nil {
		return h.Address(), nil
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(h.host, "0"))
	if err != nil {
		return "", err
	}
	h.listener = listener
	if addr, ok := listener.Addr().(*net.TCPAddr); ok {
		h.port = addr.Port
	}

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			h.wg.Add(1)
			go func(connection net.Conn) {
				defer h.wg.Done()
				defer connection.Close()
				h.handleConnection(connection)
			}(conn)
		}
	}()
	return h.Address(), nil
}

func (h *LMHandler) Stop() {
	if h.listener == nil {
		return
	}
	_ = h.listener.Close()
	h.listener = nil
	h.wg.Wait()
}

func (h *LMHandler) Address() string {
	return net.JoinHostPort(h.host, fmt.Sprintf("%d", h.port))
}

func (h *LMHandler) Completion(prompt any, model string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	return h.GetClient(model, 0).Completion(ctx, prompt, model)
}

func (h *LMHandler) GetUsageSummary() types.UsageSummary {
	merged := map[string]types.ModelUsageSummary{}
	h.clientsMu.RLock()
	for _, client := range h.clients {
		for model, usage := range client.GetUsageSummary().ModelUsageSummaries {
			if existing, ok := merged[model]; ok {
				existing.TotalCalls += usage.TotalCalls
				existing.TotalInputTokens += usage.TotalInputTokens
				existing.TotalOutputTokens += usage.TotalOutputTokens
				if usage.TotalCost != nil {
					if existing.TotalCost == nil {
						cost := *usage.TotalCost
						existing.TotalCost = &cost
					} else {
						*existing.TotalCost += *usage.TotalCost
					}
				}
				merged[model] = existing
			} else {
				merged[model] = usage
			}
		}
	}
	h.clientsMu.RUnlock()
	if h.otherBackendClient != nil {
		for model, usage := range h.otherBackendClient.GetUsageSummary().ModelUsageSummaries {
			if existing, ok := merged[model]; ok {
				existing.TotalCalls += usage.TotalCalls
				existing.TotalInputTokens += usage.TotalInputTokens
				existing.TotalOutputTokens += usage.TotalOutputTokens
				if usage.TotalCost != nil {
					if existing.TotalCost == nil {
						cost := *usage.TotalCost
						existing.TotalCost = &cost
					} else {
						*existing.TotalCost += *usage.TotalCost
					}
				}
				merged[model] = existing
			} else {
				merged[model] = usage
			}
		}
	}
	return types.UsageSummary{ModelUsageSummaries: merged}
}

func (h *LMHandler) handleConnection(conn net.Conn) {
	// Set a read/write deadline to prevent goroutine leaks from stalled clients.
	_ = conn.SetDeadline(time.Now().Add(600 * time.Second))

	payload, err := protocol.SocketRecv(conn)
	if err != nil {
		_ = protocol.SocketSend(conn, protocol.ErrorResponse(err.Error()))
		return
	}

	var request protocol.LMRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		_ = protocol.SocketSend(conn, protocol.ErrorResponse(err.Error()))
		return
	}

	var response protocol.LMResponse
	if request.IsBatched() {
		response = h.handleBatched(request)
	} else if request.Prompt != nil {
		response = h.handleSingle(request)
	} else {
		response = protocol.ErrorResponse("missing prompt or prompts")
	}
	// Deadline refresh before writing response.
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	_ = protocol.SocketSend(conn, response)
}

func (h *LMHandler) handleSingle(request protocol.LMRequest) protocol.LMResponse {
	client := h.GetClient(request.Model, request.Depth)
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	content, err := client.Completion(ctx, request.Prompt, request.Model)
	if err != nil {
		return protocol.ErrorResponse(err.Error())
	}
	rootModel := request.Model
	if rootModel == "" {
		rootModel = client.ModelName()
	}
	completion := types.RLMChatCompletion{
		RootModel:     rootModel,
		Prompt:        request.Prompt,
		Response:      content,
		UsageSummary:  types.UsageSummary{ModelUsageSummaries: map[string]types.ModelUsageSummary{rootModel: client.GetLastUsage()}},
		ExecutionTime: time.Since(start).Seconds(),
	}
	return protocol.SuccessResponse(completion)
}

func (h *LMHandler) handleBatched(request protocol.LMRequest) protocol.LMResponse {
	client := h.GetClient(request.Model, request.Depth)
	start := time.Now()

	maxConcurrent := h.batchMaxConcurrent
	if maxConcurrent > len(request.Prompts) {
		maxConcurrent = len(request.Prompts)
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	type result struct {
		index   int
		content string
		err     error
	}

	jobs := make(chan int)
	results := make(chan result, len(request.Prompts))
	var wg sync.WaitGroup
	for worker := 0; worker < maxConcurrent; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				content, err := client.Completion(context.Background(), request.Prompts[index], request.Model)
				results <- result{index: index, content: content, err: err}
			}
		}()
	}
	for index := range request.Prompts {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	close(results)

	items := make([]result, len(request.Prompts))
	for item := range results {
		if item.err != nil {
			return protocol.ErrorResponse(item.err.Error())
		}
		items[item.index] = item
	}

	rootModel := request.Model
	if rootModel == "" {
		rootModel = client.ModelName()
	}
	usageSummary := types.UsageSummary{ModelUsageSummaries: map[string]types.ModelUsageSummary{
		rootModel: client.GetLastUsage(),
	}}
	totalTime := time.Since(start).Seconds()
	completions := make([]types.RLMChatCompletion, 0, len(items))
	perPromptTime := totalTime / float64(len(items))
	for index, item := range items {
		completions = append(completions, types.RLMChatCompletion{
			RootModel:     rootModel,
			Prompt:        request.Prompts[index],
			Response:      item.content,
			UsageSummary:  usageSummary,
			ExecutionTime: perPromptTime,
		})
	}
	return protocol.BatchedSuccessResponse(completions)
}
