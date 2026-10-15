// Package mcp is the Quests MCP server: JSON-RPC 2.0 over stdio, tools on top of the HTTP API.
// The protocol subset (initialize, ping, tools/list, tools/call) is small enough to keep in-tree
// instead of pulling a framework in.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"sync"
)

const protocolVersion = "2025-06-18"

// Tool is one callable: its name, a one-line contract, a JSON Schema for the arguments and the handler.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Run         func(ctx context.Context, args json.RawMessage) (any, error)
}

// Server dispatches JSON-RPC messages to registered tools.
type Server struct {
	name, version, instructions string
	tools                       map[string]Tool
	mu                          sync.Mutex // serializes writes to the output
}

func NewServer(name, version, instructions string) *Server {
	return &Server{name: name, version: version, instructions: instructions, tools: map[string]Tool{}}
}

// Add registers tools; a duplicate name is a programming error.
func (s *Server) Add(tools ...Tool) {
	for _, t := range tools {
		if _, dup := s.tools[t.Name]; dup {
			panic("mcp: duplicate tool " + t.Name)
		}
		s.tools[t.Name] = t
	}
}

// Names lists the registered tools in alphabetical order.
func (s *Server) Names() []string {
	out := make([]string, 0, len(s.tools))
	for n := range s.tools {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads newline-delimited JSON-RPC messages from in until EOF or ctx is done.
// Calls run one at a time: an agent waits for each answer, and a switch_workspace must
// take effect before the call that follows it.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.write(out, response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			continue
		}
		if resp, ok := s.handle(ctx, req); ok {
			s.write(out, resp)
		}
	}
	return sc.Err()
}

func (s *Server) write(out io.Writer, resp response) {
	b, err := json.Marshal(resp)
	if err != nil {
		b, _ = json.Marshal(response{JSONRPC: "2.0", ID: resp.ID, Error: &rpcError{-32603, err.Error()}})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = out.Write(append(b, '\n'))
}

func (s *Server) handle(ctx context.Context, req request) (response, bool) {
	isNotification := len(req.ID) == 0
	reply := func(result any) (response, bool) {
		return response{JSONRPC: "2.0", ID: req.ID, Result: result}, !isNotification
	}
	fail := func(code int, msg string) (response, bool) {
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{code, msg}}, !isNotification
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		ver := protocolVersion
		if p.ProtocolVersion != "" {
			ver = p.ProtocolVersion // the client's revision: the subset we speak is the same in all of them
		}
		return reply(map[string]any{
			"protocolVersion": ver,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
			"instructions":    s.instructions,
		})
	case "ping":
		return reply(map[string]any{})
	case "tools/list":
		list := make([]map[string]any, 0, len(s.tools))
		for _, n := range s.Names() {
			t := s.tools[n]
			list = append(list, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.Schema})
		}
		return reply(map[string]any{"tools": list})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return fail(-32602, "invalid params")
		}
		t, ok := s.tools[p.Name]
		if !ok {
			return fail(-32602, fmt.Sprintf("unknown tool %q", p.Name))
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		return reply(s.call(ctx, t, p.Arguments))
	default:
		if isNotification {
			return response{}, false // notifications/initialized, notifications/cancelled, …
		}
		return fail(-32601, "method not found: "+req.Method)
	}
}

// call turns a handler outcome into an MCP tool result; a failure is a result with isError, not a protocol error,
// so the agent sees the message and can correct the call.
func (s *Server) call(ctx context.Context, t Tool, args json.RawMessage) map[string]any {
	res, err := t.Run(ctx, args)
	if err != nil {
		return map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": err.Error()}}}
	}
	var text string
	switch v := res.(type) {
	case string:
		text = v
	default:
		b, merr := json.Marshal(v)
		if merr != nil {
			return map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": merr.Error()}}}
		}
		text = string(b)
	}
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
}
