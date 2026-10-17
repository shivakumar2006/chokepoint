# ⛓ Chokepoint

Find the path. Cut the path. Prove it. An agent that maps cloud IAM as a graph (FalkorDB), finds every attack
path to your crown jewels, computes the smallest set of permissions to remove, checks legit access survives,
applies it after human approval and verifies from the graph.

## Run

```bash
docker compose up -d                 # FalkorDB (6379) + Browser (3000)
go mod tidy
go run ./ingest && go run ./rules    # seed + derive privilege-escalation edges
export ANTHROPIC_API_KEY=sk-ant-...  # only needed for the agent
go run ./server                      # API on :8080 (CORS: any localhost port by default)

cd web && npm install && npm run dev # UI on http://localhost:5173
```

CLI alternatives: `go run ./mincut`, `go run ./agent` (terminal agent with y/N approval), `go run ./mcp` (stdio MCP server).

## API

| Method | Path                                  |                                              |
| ------ | ------------------------------------- | -------------------------------------------- |
| GET    | /api/health                           | graph reachable?                             |
| GET    | /api/graph?source=intern              | nodes, edges, attack paths (proof chains)    |
| GET    | /api/mincut?source=intern             | min-cut options + single-fix counterfactuals |
| GET    | /api/simulate?source=intern&ids=e2,e7 | what-if impact                               |
| POST   | /api/apply `{source, ids}`            | safe apply + verify + `out/remediation.sh`   |
| GET    | /api/remediation                      | generated script                             |
| GET    | /api/trace                            | agent reasoning log                          |
| GET    | /api/status                           | agent running?                               |
| POST   | /api/run `{source, task?}`            | start agent (auto-approve)                   |
| POST   | /api/reset                            | re-seed demo graph                           |

CORS flag: `go run ./server -cors dev` (default, any localhost port) · `-cors '*'` · `-cors https://your.app,http://localhost:5173`.
Point the UI elsewhere with `VITE_API_URL` (see `web/.env.example`).

## Limits (honest)

Explicit-allow statements only (no conditions/SCPs/boundaries) · 2 privilege-escalation patterns ·
"legit access" is a labeled set in `data/legit.json` · exact search sized for tens of candidate permissions ·
remediation is generated as commands, nothing touches a real AWS account.
