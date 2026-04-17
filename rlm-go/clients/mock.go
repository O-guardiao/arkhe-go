package clients

import (
	"context"
	"fmt"
	"sync"

	"github.com/O-guardiao/arkhe-go/rlm-go/types"
)

type MockClient struct {
	BaseClient
	mu         sync.Mutex
	responses  []string
	responseFn func(prompt any) string
	callCount  int
}

func NewMockClient(modelName string, responses []string, responseFn func(prompt any) string) *MockClient {
	return &MockClient{
		BaseClient: NewBaseClient(modelName, 0),
		responses:  append([]string(nil), responses...),
		responseFn: responseFn,
	}
}

func (m *MockClient) Completion(_ context.Context, prompt any, model string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++

	if model == "" {
		model = m.ModelName()
	}
	if len(m.responses) > 0 {
		response := m.responses[0]
		m.responses = m.responses[1:]
		m.TrackUsage(model, 10, 10, nil)
		return response, nil
	}
	if m.responseFn != nil {
		response := m.responseFn(prompt)
		m.TrackUsage(model, 10, 10, nil)
		return response, nil
	}
	m.TrackUsage(model, 10, 10, nil)
	return fmt.Sprintf("Mock response to: %v", prompt), nil
}

func (m *MockClient) GetUsageSummary() types.UsageSummary {
	return m.BaseClient.GetUsageSummary()
}

func (m *MockClient) GetLastUsage() types.ModelUsageSummary {
	return m.BaseClient.GetLastUsage()
}
