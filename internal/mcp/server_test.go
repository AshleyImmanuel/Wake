package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AshleyImmanuel/Wake/internal/events"
	"github.com/AshleyImmanuel/Wake/internal/reconcile"
	"github.com/AshleyImmanuel/Wake/internal/service"
	"github.com/AshleyImmanuel/Wake/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTaskService struct {
	author string
}

func (m *mockTaskService) CreateCheckpoint(ctx context.Context, req service.CheckpointRequest) (*state.Checkpoint, error) {
	return &state.Checkpoint{}, nil
}

func (m *mockTaskService) GetStatus(ctx context.Context, req service.StatusRequest) (*reconcile.ReconciliationResult, error) {
	return &reconcile.ReconciliationResult{}, nil
}

func (m *mockTaskService) GetHistory(ctx context.Context, taskID string, limit int) ([]events.Event, error) {
	return []events.Event{}, nil
}

func (m *mockTaskService) ResumeTask(ctx context.Context, taskID string) (*service.ResumePacket, error) {
	return &service.ResumePacket{}, nil
}

func (m *mockTaskService) UpdateObjective(ctx context.Context, taskID string, objective string) error {
	return nil
}

func (m *mockTaskService) RecordEvent(ctx context.Context, taskID string, eventType events.EventType, payload map[string]interface{}) (*events.Event, error) {
	return &events.Event{}, nil
}

func (m *mockTaskService) InitWorkspace(ctx context.Context, dir string) error {
	return nil
}

func (m *mockTaskService) SetAuthor(author string) {
	m.author = author
}

func rawMsg(s string) *json.RawMessage {
	r := json.RawMessage(s)
	return &r
}

func TestServer_Initialize(t *testing.T) {
	svc := &mockTaskService{}
	srv := NewServer(svc, ".")

	initReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      rawMsg(`"1"`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"clientInfo":{"name":"test-client"}}`),
	}

	reqBytes, err := json.Marshal(initReq)
	require.NoError(t, err)

	reqBytes = append(reqBytes, '\n')
	reader := bytes.NewReader(reqBytes)
	writer := &bytes.Buffer{}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = srv.Serve(ctx, reader, writer)
	assert.NoError(t, err) // scanner will hit EOF and exit gracefully

	// Check that author was set
	assert.Equal(t, "test-client", svc.author)

	// Check response
	var resp JSONRPCResponse
	err = json.Unmarshal(writer.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)
}

func TestServer_InvalidRPC(t *testing.T) {
	svc := &mockTaskService{}
	srv := NewServer(svc, ".")

	initReq := JSONRPCRequest{
		JSONRPC: "1.0", // Invalid RPC version
		ID:      rawMsg(`"1"`),
		Method:  "initialize",
	}

	reqBytes, _ := json.Marshal(initReq)
	reqBytes = append(reqBytes, '\n')
	reader := bytes.NewReader(reqBytes)
	writer := &bytes.Buffer{}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := srv.Serve(ctx, reader, writer)
	assert.NoError(t, err)

	var resp JSONRPCResponse
	err = json.Unmarshal(writer.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, -32600, resp.Error.Code)
}

func TestServer_ToolsList(t *testing.T) {
	svc := &mockTaskService{}
	srv := NewServer(svc, ".")

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      rawMsg(`"2"`),
		Method:  "tools/list",
	}

	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')
	reader := bytes.NewReader(reqBytes)
	writer := &bytes.Buffer{}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_ = srv.Serve(ctx, reader, writer)

	var resp JSONRPCResponse
	err := json.Unmarshal(writer.Bytes(), &resp)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
}

func TestServer_ResourcesList(t *testing.T) {
	svc := &mockTaskService{}
	srv := NewServer(svc, ".")

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      rawMsg(`"3"`),
		Method:  "resources/list",
	}

	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')
	reader := bytes.NewReader(reqBytes)
	writer := &bytes.Buffer{}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_ = srv.Serve(ctx, reader, writer)

	var resp JSONRPCResponse
	err := json.Unmarshal(writer.Bytes(), &resp)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
}

func TestServer_PromptsList(t *testing.T) {
	svc := &mockTaskService{}
	srv := NewServer(svc, ".")

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      rawMsg(`"4"`),
		Method:  "prompts/list",
	}

	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')
	reader := bytes.NewReader(reqBytes)
	writer := &bytes.Buffer{}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_ = srv.Serve(ctx, reader, writer)

	var resp JSONRPCResponse
	err := json.Unmarshal(writer.Bytes(), &resp)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
}
