package core_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/O-guardiao/arkhe-go/rlm-go/clients"
	"github.com/O-guardiao/arkhe-go/rlm-go/core"
	"github.com/O-guardiao/arkhe-go/rlm-go/protocol"
)

func TestLMHandlerSingleRequest(t *testing.T) {
	mock := clients.NewMockClient("mock-model", []string{"hello back"}, nil)
	handler := core.NewLMHandler(mock, "127.0.0.1", nil, 4)
	address, err := handler.Start()
	if err != nil {
		t.Fatalf("start handler: %v", err)
	}
	defer handler.Stop()

	response := protocol.SendLMRequest(address, protocol.LMRequest{Prompt: "hello"}, 5*time.Second)
	if !response.Success() {
		t.Fatalf("expected success, got error: %s", response.Error)
	}
	if response.ChatCompletion == nil || response.ChatCompletion.Response != "hello back" {
		t.Fatalf("unexpected chat completion: %#v", response.ChatCompletion)
	}
}

func TestLMHandlerBatchedRequest(t *testing.T) {
	// Use a prompt-based response function instead of a FIFO queue so that
	// the result is deterministic regardless of goroutine scheduling order.
	mock := clients.NewMockClient("mock-model", nil, func(prompt any) string {
		return "r-" + fmt.Sprint(prompt)
	})
	handler := core.NewLMHandler(mock, "127.0.0.1", nil, 2)
	address, err := handler.Start()
	if err != nil {
		t.Fatalf("start handler: %v", err)
	}
	defer handler.Stop()

	responses := protocol.SendLMRequestBatched(address, []any{"p0", "p1", "p2"}, "", 5*time.Second, 0)
	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}
	for i, response := range responses {
		if !response.Success() {
			t.Fatalf("response %d failed: %s", i, response.Error)
		}
		expected := "r-p" + string(rune('0'+i))
		if response.ChatCompletion == nil || response.ChatCompletion.Response != expected {
			t.Fatalf("unexpected response %d: got %q, want %q", i, response.ChatCompletion.Response, expected)
		}
	}
}
