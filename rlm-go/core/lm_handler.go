package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/alexzhang13/rlm-go/clients"
	"github.com/alexzhang13/rlm-go/protocol"
	"github.com/alexzhang13/rlm-go/types"
)

type LMHandler struct {
	defaultClient      clients.Client
	otherBackendClient clients.Client
	clients            map[string]clients.Client
	host               string
	port               int
	listener           net.Listener
	wg                 sync.WaitGroup
	batchMaxConcurrent int
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
	h.clients[modelName] = client
}

func (h *LMHandler) GetClient(model string, depth int) clients.Client {
	if model != "" {
		if client, ok := h.clients[model]; ok {
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
	return h.GetClient(model, 0).Completion(context.Background(), prompt, model)
}

func (h *LMHandler) GetUsageSummary() types.UsageSummary {
	merged := map[string]types.ModelUsageSummary{}
	for _, client := range h.clients {
		for model, usage := range client.GetUsageSummary().ModelUsageSummaries {
			merged[model] = usage
		}
	}
	if h.otherBackendClient != nil {
		for model, usage := range h.otherBackendClient.GetUsageSummary().ModelUsageSummaries {
			merged[model] = usage
		}
	}
	return types.UsageSummary{ModelUsageSummaries: merged}
}

func (h *LMHandler) handleConnection(conn net.Conn) {
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
	_ = protocol.SocketSend(conn, response)
}

func (h *LMHandler) handleSingle(request protocol.LMRequest) protocol.LMResponse {
	client := h.GetClient(request.Model, request.Depth)
	start := time.Now()
	content, err := client.Completion(context.Background(), request.Prompt, request.Model)
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
