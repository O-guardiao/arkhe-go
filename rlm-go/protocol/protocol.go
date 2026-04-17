package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/O-guardiao/arkhe-go/rlm-go/types"
)

type LMRequest struct {
	Prompt  any    `json:"prompt,omitempty"`
	Prompts []any  `json:"prompts,omitempty"`
	Model   string `json:"model,omitempty"`
	Depth   int    `json:"depth"`
}

func (r LMRequest) IsBatched() bool {
	return len(r.Prompts) > 0
}

type LMResponse struct {
	Error           string                    `json:"error,omitempty"`
	ChatCompletion  *types.RLMChatCompletion  `json:"chat_completion,omitempty"`
	ChatCompletions []types.RLMChatCompletion `json:"chat_completions,omitempty"`
}

func (r LMResponse) Success() bool {
	return r.Error == ""
}

func SuccessResponse(chatCompletion types.RLMChatCompletion) LMResponse {
	return LMResponse{ChatCompletion: &chatCompletion}
}

func BatchedSuccessResponse(chatCompletions []types.RLMChatCompletion) LMResponse {
	return LMResponse{ChatCompletions: chatCompletions}
}

func ErrorResponse(err string) LMResponse {
	return LMResponse{Error: err}
}

func SocketSend(conn net.Conn, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err = conn.Write(payload)
	return err
}

// maxPayloadSize limits SocketRecv allocations to prevent OOM from malicious headers.
const maxPayloadSize = 64 * 1024 * 1024 // 64 MB

func SocketRecv(conn net.Conn) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(header)
	if length > maxPayloadSize {
		return nil, fmt.Errorf("payload size %d exceeds maximum allowed %d bytes", length, maxPayloadSize)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func SocketRequest(address string, payload any, timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	if err := SocketSend(conn, payload); err != nil {
		return nil, err
	}
	return SocketRecv(conn)
}

func SendLMRequest(address string, request LMRequest, timeout time.Duration) LMResponse {
	payload, err := SocketRequest(address, request, timeout)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("request failed: %v", err))
	}
	var response LMResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return ErrorResponse(fmt.Sprintf("request decode failed: %v", err))
	}
	return response
}

func SendLMRequestBatched(address string, prompts []any, model string, timeout time.Duration, depth int) []LMResponse {
	response := SendLMRequest(address, LMRequest{
		Prompts: prompts,
		Model:   model,
		Depth:   depth,
	}, timeout)
	if !response.Success() {
		out := make([]LMResponse, len(prompts))
		for i := range out {
			out[i] = ErrorResponse(response.Error)
		}
		return out
	}
	if len(response.ChatCompletions) == 0 {
		out := make([]LMResponse, len(prompts))
		for i := range out {
			out[i] = ErrorResponse("no completions returned")
		}
		return out
	}
	out := make([]LMResponse, 0, len(response.ChatCompletions))
	for _, completion := range response.ChatCompletions {
		c := completion
		out = append(out, SuccessResponse(c))
	}
	return out
}
