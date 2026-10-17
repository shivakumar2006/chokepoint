// Chokepoint MCP server (stdio, JSON-RPC 2.0, newline-delimited).
// Tools: list_identities, enumerate_paths, min_cut, simulate_fix, apply_fix.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"chokepoint/core"
)

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
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

func obj(props map[string]interface{}, required ...string) map[string]interface{} {
	if required == nil {
		required = []string{}
	}
	return map[string]interface{}{"type": "object", "properties": props, "required": required}
}

var strProp = map[string]interface{}{"type": "string"}
var idsProp = map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"},
	"description": "permission edge ids, exactly as returned by enumerate_paths / min_cut"}

var tools = []tool{
	{"list_identities", "List users in the IAM graph and which resources are crown jewels. Start here.", obj(map[string]interface{}{})},
	{"enumerate_paths", "Multi-hop graph traversal: every attack path from an identity to any crown jewel, with the permission edge ids each path depends on.",
		obj(map[string]interface{}{"source": strProp}, "source")},
	{"min_cut", "Graph algorithm: smallest set of permission edges whose removal blocks ALL attack paths from the identity while keeping every legit access working. Returns ranked options with collateral and rejected options.",
		obj(map[string]interface{}{"source": strProp, "max_k": map[string]interface{}{"type": "integer"}}, "source")},
	{"simulate_fix", "Counterfactual: what happens if these permission edges are removed (paths killed, crown jewels still reachable, legit access broken, permissions lost). Does NOT change anything.",
		obj(map[string]interface{}{"source": strProp, "remove_ids": idsProp}, "source", "remove_ids")},
	{"apply_fix", "ACTION: remove the permission edges from the graph, write out/remediation.sh with the matching IAM commands, then re-run the attack path query as verification. Only call after simulate_fix shows attacker_contained=true and legit_access_broken is empty.",
		obj(map[string]interface{}{"source": strProp, "remove_ids": idsProp}, "source", "remove_ids")},
}

func addr() string {
	if v := os.Getenv("FALKOR_ADDR"); v != "" {
		return v
	}
	return "falkor://localhost:6379"
}

func graphName() string {
	if v := os.Getenv("FALKOR_GRAPH"); v != "" {
		return v
	}
	return "iam"
}

func call(name string, raw json.RawMessage) (interface{}, error) {
	g, err := core.Connect(addr(), graphName())
	if err != nil {
		return nil, err
	}
	x, err := core.Load(g)
	if err != nil {
		return nil, err
	}
	var a struct {
		Source    string   `json:"source"`
		MaxK      int      `json:"max_k"`
		RemoveIDs []string `json:"remove_ids"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, fmt.Errorf("bad arguments: %v", err)
		}
	}
	needSource := func() error {
		if a.Source == "" {
			return fmt.Errorf("source is required")
		}
		for _, u := range x.Users {
			if u == a.Source {
				return nil
			}
		}
		return fmt.Errorf("unknown user %q (known: %v)", a.Source, x.Users)
	}
	switch name {
	case "list_identities":
		crown := []string{}
		for c := range x.Crown {
			crown = append(crown, c)
		}
		return map[string]interface{}{"users": x.Users, "crown_jewels": crown}, nil
	case "enumerate_paths":
		if err := needSource(); err != nil {
			return nil, err
		}
		s := core.NewSession(x, a.Source, 12)
		return map[string]interface{}{"source": a.Source, "attack_paths_total": len(s.Paths), "paths": s.PathViews()}, nil
	case "min_cut":
		if err := needSource(); err != nil {
			return nil, err
		}
		if a.MaxK <= 0 || a.MaxK > 4 {
			a.MaxK = 3
		}
		return core.NewSession(x, a.Source, 12).MinCut(a.MaxK), nil
	case "simulate_fix":
		if err := needSource(); err != nil {
			return nil, err
		}
		if len(a.RemoveIDs) == 0 {
			return nil, fmt.Errorf("remove_ids is empty")
		}
		return core.NewSession(x, a.Source, 12).Eval(a.RemoveIDs), nil
	case "apply_fix":
		if err := needSource(); err != nil {
			return nil, err
		}
		return core.Apply(g, a.Source, a.RemoveIDs)
	}
	return nil, fmt.Errorf("unknown tool %q", name)
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<24)
	out := json.NewEncoder(os.Stdout)

	send := func(r response) {
		r.JSONRPC = "2.0"
		if err := out.Encode(r); err != nil {
			fmt.Fprintln(os.Stderr, "write:", err)
		}
	}

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintln(os.Stderr, "bad json:", err)
			continue
		}
		isNotification := len(req.ID) == 0
		switch req.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.ProtocolVersion == "" {
				p.ProtocolVersion = "2024-11-05"
			}
			send(response{ID: req.ID, Result: map[string]interface{}{
				"protocolVersion": p.ProtocolVersion,
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":      map[string]interface{}{"name": "chokepoint", "version": "0.1.0"},
			}})
		case "ping":
			send(response{ID: req.ID, Result: map[string]interface{}{}})
		case "tools/list":
			send(response{ID: req.ID, Result: map[string]interface{}{"tools": tools}})
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &p); err != nil {
				send(response{ID: req.ID, Error: &rpcError{Code: -32602, Message: err.Error()}})
				continue
			}
			fmt.Fprintf(os.Stderr, "[tool] %s %s\n", p.Name, string(p.Arguments))
			res, err := call(p.Name, p.Arguments)
			if err != nil {
				send(response{ID: req.ID, Result: map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": "error: " + err.Error()}},
					"isError": true,
				}})
				continue
			}
			b, _ := json.MarshalIndent(res, "", "  ")
			send(response{ID: req.ID, Result: map[string]interface{}{
				"content": []map[string]interface{}{{"type": "text", "text": string(b)}},
			}})
		default:
			if !isNotification {
				send(response{ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found: " + req.Method}})
			}
		}
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "scanner error:", err)
	}
}

