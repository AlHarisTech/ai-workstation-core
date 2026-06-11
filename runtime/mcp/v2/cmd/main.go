package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/v2"
)

type jsonrpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolInput struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "--adapter" {
		runStdio()
		return
	}

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: mcpgateway <request.json>")
		fmt.Fprintln(os.Stderr, "       mcpgateway --test")
		fmt.Fprintln(os.Stderr, "       mcpgateway --adapter <name>")
		os.Exit(1)
	}

	if os.Args[1] == "--test" {
		runTest()
		return
	}

	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	var req mcpv2.MCPRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing request: %v\n", err)
		os.Exit(1)
	}

	gw := mcpv2.NewGateway()
	resp := gw.Process(&req)

	out, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Println(string(out))
}

func runStdio() {
	adapterName := ""
	for i, a := range os.Args {
		if a == "--adapter" && i+1 < len(os.Args) {
			adapterName = os.Args[i+1]
			break
		}
	}
	if adapterName == "" {
		fmt.Fprintln(os.Stderr, "error: --adapter requires a name")
		os.Exit(1)
	}

	gw := mcpv2.NewGateway()
	adapter := gw.Server(adapterName)
	if adapter == nil {
		fmt.Fprintf(os.Stderr, "error: unknown adapter %q\n", adapterName)
		os.Exit(1)
	}

	schema := json.RawMessage(`{"type":"object","properties":{"payload":{"type":"object"}},"required":["payload"]}`)
	tools := []tool{{
		Name:        adapterName,
		Description: fmt.Sprintf("%s MCP adapter", adapterName),
		InputSchema: schema,
	}}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var msg jsonrpcMsg
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		switch msg.Method {
		case "initialize":
			resp := jsonrpcMsg{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result: mustJSON(map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities": map[string]any{
						"tools": map[string]any{},
					},
					"serverInfo": map[string]any{
						"name":    adapterName,
						"version": "1.0.0",
					},
				}),
			}
			writeJSON(resp)

		case "tools/list":
			resp := jsonrpcMsg{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result: mustJSON(map[string]any{
					"tools": tools,
				}),
			}
			writeJSON(resp)

		case "tools/call":
			var input toolInput
			if err := json.Unmarshal(msg.Params, &input); err != nil {
				writeJSON(jsonrpcMsg{
					JSONRPC: "2.0",
					ID:      msg.ID,
					Error:   &jsonrpcError{Code: -32602, Message: "invalid params"},
				})
				continue
			}

			payload := input.Arguments
			action := ""
			if p, ok := payload["action"]; ok {
				if s, ok := p.(string); ok {
					action = s
				}
			}
			if p, ok := payload["payload"]; ok {
				if pm, ok := p.(map[string]any); ok {
					payload = pm
				}
			}
			if action == "" {
				action = input.Name
			}

			result, err := adapter.Execute(adapterName+"."+action, payload, mcpv2.MCPContext{})
			if err != nil {
				writeJSON(jsonrpcMsg{
					JSONRPC: "2.0",
					ID:      msg.ID,
					Error:   &jsonrpcError{Code: -32000, Message: err.Error()},
				})
				continue
			}

			writeJSON(jsonrpcMsg{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result: mustJSON(map[string]any{
					"content": []map[string]any{
						{"type": "json", "json": result},
					},
				}),
			})

		default:
			// notifications (no id) are silently ignored
			if msg.ID != nil {
				writeJSON(jsonrpcMsg{
					JSONRPC: "2.0",
					ID:      msg.ID,
					Error:   &jsonrpcError{Code: -32601, Message: "method not found: " + msg.Method},
				})
			}
		}
	}
}

func writeJSON(v any) {
	out, _ := json.Marshal(v)
	fmt.Println(string(out))
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func runTest() {
	gw := mcpv2.NewGateway()

	req := &mcpv2.MCPRequest{
		ID:        "test-001",
		Timestamp: "2026-06-10T20:00:00Z",
		Source:    "opencode",
		Type:      "mcp.request",
		Action: mcpv2.MCPAction{
			Type:      mcpv2.ActionGit,
			Operation: "status",
			Version:   "1.0",
		},
		Context: mcpv2.MCPContext{
			TenantID:  "t1",
			SessionID: "s1",
			TraceID:   "tr1",
		},
		Auth:   mcpv2.MCPAuth{Type: "bearer", Scope: []string{"read"}},
		Policy: mcpv2.MCPPolicy{Allow: []string{"git.*"}},
		Meta: mcpv2.MCPMeta{
			Priority: "high",
			Timeout:  30000,
			Retry:    2,
			TraceID:  "demo-trace",
			SpanID:   "demo-span",
		},
	}
	req.Context.Workspace.Path = "/demo"

	fmt.Println("=== MCP Gateway v1.1 ===")
	fmt.Println("Request:")
	reqJSON, _ := json.MarshalIndent(req, "  ", "  ")
	fmt.Println("  " + string(reqJSON))

	resp := gw.Process(req)

	fmt.Println("\nResponse:")
	respJSON, _ := json.MarshalIndent(resp, "  ", "  ")
	fmt.Println("  " + string(respJSON))
	fmt.Println("\nStatus:", resp.Status)
	if resp.Status == "success" {
		fmt.Println("✓ Gateway pipeline completed successfully")
	} else {
		fmt.Println("✗ Gateway error:", resp.Error.Message)
	}
}
