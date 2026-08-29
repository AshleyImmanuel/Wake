package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleToolCall_UnknownTool(t *testing.T) {
	svc := &mockTaskService{}
	srv := NewServer(svc, ".")
	ctx := context.Background()

	_, err := srv.handleToolCall(ctx, "unknown_tool", map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
}

func TestHandleToolCall_Status(t *testing.T) {
	svc := &mockTaskService{}
	srv := NewServer(svc, ".")
	ctx := context.Background()

	res, err := srv.handleToolCall(ctx, "wake_status", map[string]interface{}{"dir": "."})
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Len(t, res.Content, 1)

	text, ok := res.Content[0].(TextContent)
	require.True(t, ok)
	assert.Equal(t, "text", text.Type)
}

func TestHandleToolCall_Checkpoint(t *testing.T) {
	svc := &mockTaskService{}
	srv := NewServer(svc, ".")
	ctx := context.Background()

	res, err := srv.handleToolCall(ctx, "wake_checkpoint", map[string]interface{}{"dir": ".", "message": "test"})
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Len(t, res.Content, 1)
}

func TestHandleToolCall_History(t *testing.T) {
	svc := &mockTaskService{}
	srv := NewServer(svc, ".")
	ctx := context.Background()

	res, err := srv.handleToolCall(ctx, "wake_history", map[string]interface{}{"taskId": "123", "limit": 10.0})
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Len(t, res.Content, 1)
}

func TestHandleToolCall_Resume(t *testing.T) {
	svc := &mockTaskService{}
	srv := NewServer(svc, ".")
	ctx := context.Background()

	res, err := srv.handleToolCall(ctx, "wake_resume", map[string]interface{}{"taskId": "123"})
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Len(t, res.Content, 1)
}

func TestServer_ToolsCall_Success(t *testing.T) {
	svc := &mockTaskService{}
	srv := NewServer(svc, ".")

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      rawMsg(`"1"`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"wake_status", "arguments":{"dir":"."}}`),
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
	assert.NotNil(t, resp.Result)
}
