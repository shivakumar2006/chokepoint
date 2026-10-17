// Chokepoint agent: Claude decides which MCP tools to call; every tool runs against the FalkorDB graph.
// The agent talks MCP (stdio) to ./mcp as a subprocess and the Anthropic Messages API over plain HTTP.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ---------- minimal MCP stdio client ----------

type mcpClient struct {
	in     io.WriteCloser
	out    *bufio.Reader
	nextID int
}

func (c *mcpClient) send(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = c.in.Write(append(b, '\n'))
	return err
}

func (c *mcpClient) rpc(method string, params interface{}) (json.RawMessage, error) {
	c.nextID++
	id := c.nextID
	if err := c.send(map[string]interface{}{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		line, err := c.out.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("mcp server closed (build error? try `go build ./mcp`): %v", err)
		}
		var resp struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(line, &resp) != nil || string(resp.ID) != strconv.Itoa(id) {
			continue // not our response (or build noise)
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp %s: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (c *mcpClient) callTool(name string, args json.RawMessage) (string, bool, error) {
	var a interface{} = map[string]interface{}{}
	if len(args) > 0 {
		_ = json.Unmarshal(args, &a)
	}
	raw, err := c.rpc("tools/call", map[string]interface{}{"name": name, "arguments": a})
	if err != nil {
		return "", true, err
	}
	var res struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", true, err
	}
	var sb strings.Builder
	for _, c := range res.Content {
		sb.WriteString(c.Text)
	}
	return sb.String(), res.IsError, nil
}

// ---------- anthropic ----------

type block struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	raw   json.RawMessage
}

func callClaude(key, model, system string, tools []map[string]interface{}, messages []interface{}) ([]block, string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model": model, "max_tokens": 2000, "system": system, "tools": tools, "messages": messages,
	})
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	resp, err := (&http.Client{Timeout: 180 * time.Second}).Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("anthropic API %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Content    []json.RawMessage `json:"content"`
		StopReason string            `json:"stop_reason"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, "", err
	}
	var blocks []block
	for _, r := range out.Content {
		var bl block
		_ = json.Unmarshal(r, &bl)
		bl.raw = r
		blocks = append(blocks, bl)
	}
	return blocks, out.StopReason, nil
}

// ---------- trace (reasoning log the UI will read) ----------

var traceFile *os.File

func trace(kind, name string, data interface{}) {
	if traceFile == nil {
		return
	}
	b, _ := json.Marshal(map[string]interface{}{"t": time.Now().Format(time.RFC3339), "type": kind, "name": name, "data": data})
	traceFile.Write(append(b, '\n'))
}

func short(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + " ..."
	}
	return s
}

const systemPrompt = `You are Chokepoint, a cloud IAM security agent. You work ONLY through the tools provided; they query a graph database of IAM users, groups, roles, policies and resources.

Process:
1. list_identities, then enumerate_paths for the identity you were asked about (multi-hop attack paths to crown jewels).
2. min_cut to get the smallest permission set that blocks all paths while legit access keeps working.
3. simulate_fix on the option you prefer. Prefer the option with the least collateral; never choose anything with legit_access_broken. If a different single fix looks tempting, compare it with simulate_fix first and say why you reject it.
4. Only if simulate_fix shows attacker_contained=true and legit_access_broken is empty, call apply_fix. A human may reject it; if so, say so and stop.
5. Finish with a short report: attack paths before/after (use apply_fix's verified numbers), what was changed, what legit permissions were lost, and the reasoning path for the main attack (cite the hops).

Rules: use only numbers returned by tools, never invent any. State limits honestly: the model covers explicit allow statements only (no conditions, SCPs or permission boundaries) and 5 privilege-escalation patterns. Keep the report under 200 words.`

func main() {
	mcpCmd := flag.String("mcp", "go run ./mcp", "command that starts the MCP server")
	model := flag.String("model", "claude-sonnet-5-5", "Anthropic model id")
	task := flag.String("task", "Investigate the attack paths from the user 'intern' to our crown jewels and fix them safely.", "what to ask the agent")
	yes := flag.Bool("yes", false, "auto-approve apply_fix (skip the human approval prompt)")
	listOnly := flag.Bool("list-tools", false, "just print MCP tools and exit (no API key needed)")
	maxTurns := flag.Int("max-turns", 14, "safety limit on agent steps")
	flag.Parse()

	cmd := exec.Command("sh", "-c", *mcpCmd)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if os.Getenv("AGENT_DEBUG") != "" {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		log.Fatalf("start mcp: %v", err)
	}
	defer func() { stdin.Close(); cmd.Process.Kill() }()
	c := &mcpClient{in: stdin, out: bufio.NewReaderSize(stdout, 1<<20)}

	if _, err := c.rpc("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05", "capabilities": map[string]interface{}{},
		"clientInfo": map[string]interface{}{"name": "chokepoint-agent", "version": "0.1.0"},
	}); err != nil {
		log.Fatal(err)
	}
	_ = c.send(map[string]interface{}{"jsonrpc": "2.0", "method": "notifications/initialized"})

	raw, err := c.rpc("tools/list", map[string]interface{}{})
	if err != nil {
		log.Fatal(err)
	}
	var tl struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(raw, &tl)
	var tools []map[string]interface{}
	for _, t := range tl.Tools {
		tools = append(tools, map[string]interface{}{"name": t.Name, "description": t.Description, "input_schema": t.InputSchema})
	}
	if *listOnly {
		for _, t := range tl.Tools {
			fmt.Printf("- %s: %s\n", t.Name, short(t.Description, 100))
		}
		return
	}

	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		log.Fatal("set ANTHROPIC_API_KEY first:  export ANTHROPIC_API_KEY=sk-ant-...")
	}
	_ = os.MkdirAll("out", 0o755)
	traceFile, _ = os.OpenFile("out/trace.jsonl", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)

	fmt.Printf("TASK: %s\n\n", *task)
	trace("task", "", *task)
	messages := []interface{}{map[string]interface{}{"role": "user", "content": *task}}

	for turn := 1; turn <= *maxTurns; turn++ {
		blocks, stop, err := callClaude(key, *model, systemPrompt, tools, messages)
		if err != nil {
			log.Fatal(err)
		}
		var assistant []interface{}
		var results []interface{}
		for _, b := range blocks {
			assistant = append(assistant, b.raw)
			switch b.Type {
			case "text":
				if strings.TrimSpace(b.Text) != "" {
					fmt.Printf("AGENT: %s\n\n", b.Text)
					trace("text", "", b.Text)
				}
			case "tool_use":
				fmt.Printf("-> TOOL %s %s\n", b.Name, short(string(b.Input), 160))
				trace("tool_call", b.Name, json.RawMessage(b.Input))
				var text string
				var isErr bool
				if b.Name == "apply_fix" && !*yes {
					fmt.Printf("\n   HUMAN APPROVAL NEEDED: apply_fix %s\n   Apply this change? [y/N]: ", string(b.Input))
					ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
					ok := strings.HasPrefix(strings.ToLower(strings.TrimSpace(ans)), "y")
					trace("approval", b.Name, ok)
					if !ok {
						text, isErr = "human rejected this change; do not apply anything", true
					}
				}
				if text == "" {
					t, e, err := c.callTool(b.Name, b.Input)
					if err != nil {
						t, e = "tool failed: "+err.Error(), true
					}
					text, isErr = t, e
				}
				fmt.Printf("   <- %s\n\n", short(text, 240))
				trace("tool_result", b.Name, text)
				results = append(results, map[string]interface{}{
					"type": "tool_result", "tool_use_id": b.ID, "content": text, "is_error": isErr,
				})
			}
		}
		messages = append(messages, map[string]interface{}{"role": "assistant", "content": assistant})
		if stop != "tool_use" || len(results) == 0 {
			fmt.Println("(done)")
			return
		}
		messages = append(messages, map[string]interface{}{"role": "user", "content": results})
	}
	fmt.Println("(stopped: max turns reached)")
}
