// Chokepoint API server (JSON + CORS) for the React console.
//
//	GET  /api/health
//	GET  /api/graph?source=intern        nodes, edges, attack paths (proof chains)
//	GET  /api/mincut?source=intern       min-cut options + single-fix counterfactuals
//	GET  /api/simulate?source=&ids=e2,e7 what-if impact of removing permission edges
//	POST /api/apply   {source, ids}      apply a safe fix, verify from the graph, write out/remediation.sh
//	GET  /api/remediation                contents of out/remediation.sh
//	GET  /api/trace                      agent reasoning log (out/trace.jsonl)
//	GET  /api/status                     is the agent running
//	POST /api/run     {source, task?}    start the agent (auto-approve) in the background
//	POST /api/reset                      re-seed the demo graph
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/FalkorDB/falkordb-go/v2"

	"chokepoint/core"
)

var (
	graph   *falkordb.Graph
	mu      sync.Mutex
	running bool
)

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func sourceOf(r *http.Request) string {
	if s := r.URL.Query().Get("source"); s != "" {
		return s
	}
	return "intern"
}

func session(r *http.Request) (*core.Graph, *core.Session, error) {
	x, err := core.Load(graph)
	if err != nil {
		return nil, nil, err
	}
	src := sourceOf(r)
	known := false
	for _, u := range x.Users {
		if u == src {
			known = true
		}
	}
	if !known {
		return nil, nil, fmt.Errorf("unknown user %q (known: %v)", src, x.Users)
	}
	return x, core.NewSession(x, src, 12), nil
}

// ---------- CORS ----------

// mode "dev"  : any http://localhost:* or http://127.0.0.1:* origin
// mode "*"    : any origin
// otherwise   : comma separated exact origins, e.g. https://chokepoint.vercel.app,http://localhost:5173
func cors(mode string, next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range strings.Split(mode, ",") {
		allowed[strings.TrimSpace(o)] = true
	}
	ok := func(origin string) bool {
		switch {
		case origin == "":
			return false
		case mode == "*":
			return true
		case mode == "dev":
			return strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:")
		default:
			return allowed[origin]
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); ok(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.RequestURI())
		next.ServeHTTP(w, r)
	})
}

// ---------- main ----------

func main() {
	addr := flag.String("addr", "falkor://localhost:6379", "FalkorDB URL")
	name := flag.String("graph", "iam", "graph name")
	listen := flag.String("listen", ":8080", "http listen address")
	corsMode := flag.String("cors", "dev", `CORS: "dev" (localhost any port), "*" or comma separated origins`)
	flag.Parse()

	var err error
	if graph, err = core.Connect(*addr, *name); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		x, err := core.Load(graph)
		if err != nil {
			fail(w, 503, err)
			return
		}
		writeJSON(w, 200, map[string]interface{}{"ok": true, "users": len(x.Users), "edges": len(x.Edges)})
	})

	mux.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
		x, s, err := session(r)
		if err != nil {
			fail(w, 500, err)
			return
		}
		type node struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Crown bool   `json:"crown"`
		}
		nodes := []node{}
		for _, n := range x.NodeNames() {
			nodes = append(nodes, node{n, x.Kind(n), x.Crown[n]})
		}
		edges := []core.Edge{}
		for _, e := range x.Edges {
			if e.Type != "LEGIT_ACCESS" {
				edges = append(edges, e)
			}
		}
		crown := []string{}
		for c := range x.Crown {
			crown = append(crown, c)
		}
		writeJSON(w, 200, map[string]interface{}{
			"source": s.Source, "nodes": nodes, "edges": edges, "paths": s.PathViews(),
			"users": x.Users, "crown_jewels": crown,
		})
	})

	mux.HandleFunc("/api/mincut", func(w http.ResponseWriter, r *http.Request) {
		_, s, err := session(r)
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, 200, s.MinCut(3))
	})

	mux.HandleFunc("/api/simulate", func(w http.ResponseWriter, r *http.Request) {
		_, s, err := session(r)
		if err != nil {
			fail(w, 500, err)
			return
		}
		ids := []string{}
		for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
		writeJSON(w, 200, s.Eval(ids))
	})

	mux.HandleFunc("/api/apply", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			fail(w, 405, fmt.Errorf("POST only"))
			return
		}
		var body struct {
			Source string   `json:"source"`
			IDs    []string `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Source == "" || len(body.IDs) == 0 {
			fail(w, 400, fmt.Errorf("body must be {\"source\": \"...\", \"ids\": [\"e2\"]}"))
			return
		}
		res, err := core.Apply(graph, body.Source, body.IDs)
		if err != nil {
			fail(w, 422, err)
			return
		}
		writeJSON(w, 200, res)
	})

	mux.HandleFunc("/api/remediation", func(w http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile("out/remediation.sh")
		if err != nil {
			writeJSON(w, 200, map[string]string{"script": ""})
			return
		}
		writeJSON(w, 200, map[string]string{"script": string(b)})
	})

	mux.HandleFunc("/api/trace", func(w http.ResponseWriter, r *http.Request) {
		out := []json.RawMessage{}
		if b, err := os.ReadFile("out/trace.jsonl"); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if strings.TrimSpace(line) != "" && json.Valid([]byte(line)) {
					out = append(out, json.RawMessage(line))
				}
			}
		}
		writeJSON(w, 200, out)
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		writeJSON(w, 200, map[string]bool{"running": running})
	})

	mux.HandleFunc("/api/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			fail(w, 405, fmt.Errorf("POST only"))
			return
		}
		mu.Lock()
		busy := running
		mu.Unlock()
		if busy {
			fail(w, 409, fmt.Errorf("agent is running, wait for it to finish"))
			return
		}
		var log1 []byte
		for _, step := range [][]string{{"go", "run", "./ingest"}, {"go", "run", "./rules"}} {
			out, err := exec.Command(step[0], step[1:]...).CombinedOutput()
			log1 = append(log1, out...)
			if err != nil {
				fail(w, 500, fmt.Errorf("%v: %s", err, tail(string(out), 400)))
				return
			}
		}
		_ = os.Remove("out/trace.jsonl")
		writeJSON(w, 200, map[string]string{"ok": "graph re-seeded", "log": tail(string(log1), 500)})
	})

	mux.HandleFunc("/api/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			fail(w, 405, fmt.Errorf("POST only"))
			return
		}
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			fail(w, 400, fmt.Errorf("ANTHROPIC_API_KEY is not set in the terminal that runs the Go server"))
			return
		}
		var body struct {
			Source string `json:"source"`
			Task   string `json:"task"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Source == "" {
			body.Source = "intern"
		}
		if body.Task == "" {
			body.Task = fmt.Sprintf("Investigate the attack paths from the user '%s' to our crown jewels and fix them safely.", body.Source)
		}
		mu.Lock()
		if running {
			mu.Unlock()
			fail(w, 409, fmt.Errorf("agent already running"))
			return
		}
		running = true
		mu.Unlock()
		go func() {
			out, err := exec.Command("go", "run", "./agent", "-yes", "-task", body.Task).CombinedOutput()
			if err != nil {
				log.Printf("agent failed: %v\n%s", err, tail(string(out), 800))
			}
			mu.Lock()
			running = false
			mu.Unlock()
		}()
		writeJSON(w, 202, map[string]string{"ok": "agent started"})
	})

	log.Printf("Chokepoint API on http://localhost%s  (cors=%s)", *listen, *corsMode)
	log.Fatal(http.ListenAndServe(*listen, logging(cors(*corsMode, mux))))
}
