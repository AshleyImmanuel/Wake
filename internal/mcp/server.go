package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/AshleyImmanuel/Wake/internal/service"
)

type Server struct {
	svc     service.TaskService
	workDir string
	mu      sync.Mutex
}

func NewServer(svc service.TaskService, workDir string) *Server {
	return &Server{
		svc:     svc,
		workDir: workDir,
	}
}

func (s *Server) Serve(ctx context.Context, reader io.Reader, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	out := json.NewEncoder(writer)

	// In Go, bufio.Scanner can handle up to 64KB by default. Let's increase it just in case.
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !scanner.Scan() {
			break
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(out, nil, -32700, "Parse error")
			continue
		}

		s.handleRequest(ctx, out, req)
	}
	return scanner.Err()
}

func (s *Server) sendError(out *json.Encoder, id *json.RawMessage, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = out.Encode(resp)
}

func (s *Server) sendResult(out *json.Encoder, id *json.RawMessage, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = out.Encode(resp)
}

func (s *Server) handleRequest(ctx context.Context, out *json.Encoder, req JSONRPCRequest) {
	if req.JSONRPC != "2.0" {
		s.sendError(out, req.ID, -32600, "Invalid Request")
		return
	}

	switch req.Method {
	case "initialize":
		res := InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: ServerCapabilities{
				Prompts:   map[string]interface{}{},
				Resources: map[string]interface{}{},
				Tools:     map[string]interface{}{},
			},
			ServerInfo: Implementation{
				Name:    "wake-mcp",
				Version: "1.0.0",
			},
		}
		s.sendResult(out, req.ID, res)
	case "initialized":
		// notification, do nothing
	case "tools/list":
		res := ListToolsResult{
			Tools: getTools(),
		}
		s.sendResult(out, req.ID, res)
	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(out, req.ID, -32602, "Invalid params")
			return
		}
		res, err := s.handleToolCall(ctx, params.Name, params.Arguments)
		if err != nil {
			failRes := CallToolResult{
				Content: []interface{}{TextContent{Type: "text", Text: err.Error()}},
				IsError: true,
			}
			s.sendResult(out, req.ID, failRes)
			return
		}
		s.sendResult(out, req.ID, res)
	case "resources/list":
		res := ListResourcesResult{
			Resources: getResources(),
		}
		s.sendResult(out, req.ID, res)
	case "resources/read":
		var params ReadResourceParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(out, req.ID, -32602, "Invalid params")
			return
		}
		res, err := s.handleResourceRead(ctx, params.Uri)
		if err != nil {
			s.sendError(out, req.ID, -32603, err.Error())
			return
		}
		s.sendResult(out, req.ID, res)
	case "prompts/list":
		res := ListPromptsResult{
			Prompts: getPrompts(),
		}
		s.sendResult(out, req.ID, res)
	case "prompts/get":
		var params GetPromptParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(out, req.ID, -32602, "Invalid params")
			return
		}
		res, err := s.handlePromptGet(ctx, params.Name, params.Arguments)
		if err != nil {
			s.sendError(out, req.ID, -32603, err.Error())
			return
		}
		s.sendResult(out, req.ID, res)
	default:
		if req.ID != nil {
			s.sendError(out, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
		}
	}
}
