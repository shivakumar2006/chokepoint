// Package core holds all graph logic (paths, min-cut, simulate, remediation) so the CLI,
// the MCP server and the agent share exactly one implementation.
package core

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/FalkorDB/falkordb-go/v2"
)

// ---------- model ----------

type Edge struct {
	ID     string   `json:"id"`
	Type   string   `json:"type"`
	From   string   `json:"from"`
	To     string   `json:"to"`
	Action string   `json:"action,omitempty"`
	Via    []string `json:"via,omitempty"` // ESCALATES only: real edges it depends on (ALL required)
}

type Graph struct {
	Edges []Edge
	Users []string
	Crown map[string]bool

	adj   map[string][]int // walkable edges
	perms map[string][]int // permission statements by owning policy
	kind  map[string]string
	byID  map[string]int
}

var walkable = map[string]bool{
	"MEMBER_OF": true, "HAS_POLICY": true, "CAN_ASSUME": true, "ALLOWS": true, "ESCALATES": true,
}

// ---------- falkordb ----------

func Connect(addr, name string) (*falkordb.Graph, error) {
	db, err := falkordb.FromURL(addr)
	if err != nil {
		return nil, err
	}
	return db.SelectGraph(name), nil
}

func val(r *falkordb.Record, i int) interface{} {
	v, _ := r.GetByIndex(i)
	return v
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// Load reads the whole (small) graph into memory. Cypher stays the source of truth;
// call Load again after any mutation.
func Load(g *falkordb.Graph) (*Graph, error) {
	x := &Graph{
		Crown: map[string]bool{}, adj: map[string][]int{}, perms: map[string][]int{},
		kind: map[string]string{}, byID: map[string]int{},
	}
	res, err := g.Query("MATCH (a)-[e]->(b) RETURN a.name, type(e), b.name, e.id, e.action, e.via", nil, nil)
	if err != nil {
		return nil, err
	}
	for res.Next() {
		r := res.Record()
		e := Edge{From: str(val(r, 0)), Type: str(val(r, 1)), To: str(val(r, 2)), ID: str(val(r, 3)), Action: str(val(r, 4))}
		if arr, ok := val(r, 5).([]interface{}); ok {
			for _, v := range arr {
				e.Via = append(e.Via, str(v))
			}
		}
		idx := len(x.Edges)
		x.Edges = append(x.Edges, e)
		x.byID[e.ID] = idx
		if walkable[e.Type] {
			x.adj[e.From] = append(x.adj[e.From], idx)
		}
		switch e.Type {
		case "ALLOWS", "ALLOWS_ON_ROLE", "ALLOWS_ON_POLICY":
			x.perms[e.From] = append(x.perms[e.From], idx)
		}
	}
	ns, err := g.Query("MATCH (n) RETURN n.name, labels(n)[0], n.crown_jewel ORDER BY n.name", nil, nil)
	if err != nil {
		return nil, err
	}
	for ns.Next() {
		r := ns.Record()
		name, label := str(val(r, 0)), str(val(r, 1))
		x.kind[name] = label
		if label == "User" {
			x.Users = append(x.Users, name)
		}
		if b, ok := val(r, 2).(bool); ok && b {
			x.Crown[name] = true
		}
	}
	return x, nil
}

// ApplyFix really mutates the graph: removes the permission edges and every derived
// escalation edge that depended on them. Re-running Load + AttackPaths afterwards is the verification.
func ApplyFix(g *falkordb.Graph, ids []string) error {
	for _, id := range ids {
		p := map[string]interface{}{"id": id}
		if _, err := g.Query("MATCH ()-[e {id: $id}]->() DELETE e", p, nil); err != nil {
			return err
		}
		if _, err := g.Query("MATCH ()-[d:ESCALATES]->() WHERE $id IN d.via DELETE d", p, nil); err != nil {
			return err
		}
	}
	return nil
}

// ---------- reachability ----------

func (x *Graph) alive(e Edge, removed map[string]bool, noEsc bool) bool {
	if noEsc && e.Type == "ESCALATES" {
		return false
	}
	if removed[e.ID] {
		return false
	}
	for _, v := range e.Via {
		if removed[v] {
			return false
		}
	}
	return true
}

func (x *Graph) reach(src string, removed map[string]bool, noEsc bool) map[string]bool {
	seen := map[string]bool{src: true}
	queue := []string{src}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, ei := range x.adj[n] {
			e := x.Edges[ei]
			if seen[e.To] || !x.alive(e, removed, noEsc) {
				continue
			}
			seen[e.To] = true
			queue = append(queue, e.To)
		}
	}
	return seen
}

// effPerms = permission statements an identity legitimately holds (escalation ignored)
func (x *Graph) effPerms(src string, removed map[string]bool) map[string]bool {
	out := map[string]bool{}
	for n := range x.reach(src, removed, true) {
		for _, ei := range x.perms[n] {
			if id := x.Edges[ei].ID; !removed[id] {
				out[id] = true
			}
		}
	}
	return out
}

func (x *Graph) Describe(id string) string {
	ei, ok := x.byID[id]
	if !ok {
		return id
	}
	e := x.Edges[ei]
	if e.Action != "" {
		return fmt.Sprintf("%s -[%s %s]-> %s", e.From, e.Type, e.Action, e.To)
	}
	return fmt.Sprintf("%s -[%s]-> %s", e.From, e.Type, e.To)
}

// ---------- attack paths ----------

type PathView struct {
	Target        string   `json:"target"`
	Nodes         []string `json:"nodes"`
	EdgeTypes     []string `json:"edge_types"`
	PermissionIDs []string `json:"permission_ids"`
}

type Session struct {
	G          *Graph
	Source     string
	Paths      [][]int
	idSets     []map[string]bool
	Candidates []string
	basePerm   map[string]map[string]bool
	legit      [][2]string
}

func NewSession(x *Graph, source string, maxDepth int) *Session {
	s := &Session{G: x, Source: source, basePerm: map[string]map[string]bool{}}
	visited := map[string]bool{source: true}
	var path []int
	var dfs func(n string)
	dfs = func(n string) {
		if len(path) >= maxDepth {
			return
		}
		for _, ei := range x.adj[n] {
			e := x.Edges[ei]
			if visited[e.To] {
				continue
			}
			path = append(path, ei)
			if x.Crown[e.To] {
				s.Paths = append(s.Paths, append([]int(nil), path...))
			} else {
				visited[e.To] = true
				dfs(e.To)
				visited[e.To] = false
			}
			path = path[:len(path)-1]
		}
	}
	dfs(source)

	cand := map[string]bool{}
	for _, p := range s.Paths {
		ids := map[string]bool{}
		for _, ei := range p {
			e := x.Edges[ei]
			if e.Type == "ESCALATES" {
				for _, v := range e.Via {
					ids[v] = true
				}
			} else {
				ids[e.ID] = true
			}
		}
		s.idSets = append(s.idSets, ids)
		for id := range ids {
			cand[id] = true
		}
	}
	for id := range cand {
		s.Candidates = append(s.Candidates, id)
	}
	sort.Strings(s.Candidates)

	none := map[string]bool{}
	for _, u := range x.Users {
		s.basePerm[u] = x.effPerms(u, none)
	}
	for _, e := range x.Edges {
		if e.Type == "LEGIT_ACCESS" && x.reach(e.From, none, false)[e.To] {
			s.legit = append(s.legit, [2]string{e.From, e.To})
		}
	}
	return s
}

func (s *Session) PathViews() []PathView {
	out := []PathView{}
	for i, p := range s.Paths {
		v := PathView{Nodes: []string{s.Source}}
		for _, ei := range p {
			e := s.G.Edges[ei]
			v.Nodes = append(v.Nodes, e.To)
			v.EdgeTypes = append(v.EdgeTypes, e.Type)
		}
		v.Target = v.Nodes[len(v.Nodes)-1]
		for id := range s.idSets[i] {
			v.PermissionIDs = append(v.PermissionIDs, id)
		}
		sort.Strings(v.PermissionIDs)
		out = append(out, v)
	}
	return out
}

// ---------- counterfactual ----------

type Impact struct {
	RemoveIDs         []string `json:"remove_ids"`
	Remove            []string `json:"remove"`
	AttackerContained bool     `json:"attacker_contained"`
	PathsKilled       int      `json:"attack_paths_killed"`
	PathsTotal        int      `json:"attack_paths_total"`
	StillReachable    []string `json:"still_reachable_crown_jewels"`
	LegitBroken       []string `json:"legit_access_broken"`
	Collateral        []string `json:"collateral_permissions_lost"`
}

// Eval = simulate_fix: what happens if exactly these permission edges are removed.
func (s *Session) Eval(ids []string) Impact {
	x := s.G
	removed := map[string]bool{}
	for _, id := range ids {
		removed[id] = true
	}
	im := Impact{RemoveIDs: append([]string(nil), ids...), PathsTotal: len(s.Paths), AttackerContained: true,
		StillReachable: []string{}, LegitBroken: []string{}, Collateral: []string{}}
	for _, id := range ids {
		im.Remove = append(im.Remove, x.Describe(id))
	}
	r := x.reach(s.Source, removed, false)
	for c := range x.Crown {
		if r[c] {
			im.AttackerContained = false
			im.StillReachable = append(im.StillReachable, c)
		}
	}
	sort.Strings(im.StillReachable)
	for _, l := range s.legit {
		if !x.reach(l[0], removed, false)[l[1]] {
			im.LegitBroken = append(im.LegitBroken, l[0]+" -> "+l[1])
		}
	}
	for _, u := range x.Users {
		after := x.effPerms(u, removed)
		for id := range s.basePerm[u] {
			if !after[id] {
				im.Collateral = append(im.Collateral, u+": "+x.Describe(id))
			}
		}
	}
	sort.Strings(im.Collateral)
	for i := range s.Paths {
		for _, id := range ids {
			if s.idSets[i][id] {
				im.PathsKilled++
				break
			}
		}
	}
	return im
}

// ---------- minimum cut ----------

type MinCutResult struct {
	Source     string   `json:"source"`
	PathsTotal int      `json:"attack_paths_total"`
	Candidates int      `json:"candidate_permission_edges"`
	Singles    []Impact `json:"best_single_fixes"`
	Size       int      `json:"min_cut_size"`
	Options    []Impact `json:"min_cut_options"`
	Rejected   []Impact `json:"rejected_because_legit_breaks"`
	Note       string   `json:"note"`
}

func combos(items []string, k int, f func([]string)) {
	var rec func(start int, cur []string)
	rec = func(start int, cur []string) {
		if len(cur) == k {
			f(cur)
			return
		}
		for i := start; i < len(items); i++ {
			rec(i+1, append(cur[:len(cur):len(cur)], items[i]))
		}
	}
	rec(0, nil)
}

// MinCut finds the smallest set of real permission edges whose removal contains the attacker
// while every legit access keeps working. Derived escalation edges are ANDs of permissions, which
// plain max-flow cannot model, so this is an exact search over candidate edges (fine up to ~30).
func (s *Session) MinCut(maxK int) MinCutResult {
	res := MinCutResult{Source: s.Source, PathsTotal: len(s.Paths), Candidates: len(s.Candidates),
		Singles: []Impact{}, Options: []Impact{}, Rejected: []Impact{}}
	if len(s.Paths) == 0 {
		res.Note = "no attack paths from this identity"
		return res
	}
	var singles []Impact
	for _, id := range s.Candidates {
		singles = append(singles, s.Eval([]string{id}))
	}
	sort.SliceStable(singles, func(i, j int) bool { return singles[i].PathsKilled > singles[j].PathsKilled })
	for i, im := range singles {
		if i >= 8 {
			break
		}
		res.Singles = append(res.Singles, im)
	}

	for k := 1; k <= maxK; k++ {
		var ok, bad []Impact
		combos(s.Candidates, k, func(S []string) {
			im := s.Eval(S)
			if !im.AttackerContained {
				return
			}
			if len(im.LegitBroken) > 0 {
				bad = append(bad, im)
			} else {
				ok = append(ok, im)
			}
		})
		if len(ok) == 0 {
			continue
		}
		sort.SliceStable(ok, func(i, j int) bool {
			if len(ok[i].Collateral) != len(ok[j].Collateral) {
				return len(ok[i].Collateral) < len(ok[j].Collateral)
			}
			return strings.Join(ok[i].RemoveIDs, ",") < strings.Join(ok[j].RemoveIDs, ",")
		})
		res.Size = k
		for i, im := range ok {
			if i < 5 {
				res.Options = append(res.Options, im)
			}
		}
		for i, im := range bad {
			if i < 3 {
				res.Rejected = append(res.Rejected, im)
			}
		}
		res.Note = "exact minimum; ranked by permissions legit users would lose"
		return res
	}
	res.Note = fmt.Sprintf("no legit-safe cut of size <= %d", maxK)
	return res
}

// ---------- remediation (the 'act' part) ----------

// Remediation turns removed permission edges into concrete, reviewable IAM changes.
func (x *Graph) Remediation(ids []string) []string {
	var out []string
	for _, id := range ids {
		ei, ok := x.byID[id]
		if !ok {
			continue
		}
		e := x.Edges[ei]
		arn := "arn:aws:iam::ACCOUNT_ID:policy/" + e.To
		switch e.Type {
		case "MEMBER_OF":
			out = append(out, fmt.Sprintf("aws iam remove-user-from-group --user-name %s --group-name %s", e.From, e.To))
		case "HAS_POLICY":
			switch x.kind[e.From] {
			case "Group":
				out = append(out, fmt.Sprintf("aws iam detach-group-policy --group-name %s --policy-arn %s", e.From, arn))
			case "Role":
				out = append(out, fmt.Sprintf("aws iam detach-role-policy --role-name %s --policy-arn %s", e.From, arn))
			default:
				out = append(out, fmt.Sprintf("aws iam detach-user-policy --user-name %s --policy-arn %s", e.From, arn))
			}
		case "CAN_ASSUME":
			out = append(out, fmt.Sprintf("# remove %s from the trust policy of role %s  (aws iam update-assume-role-policy)", e.From, e.To))
		default: // ALLOWS, ALLOWS_ON_ROLE, ALLOWS_ON_POLICY
			out = append(out, fmt.Sprintf("# edit policy %s: remove action %s on %s, then create a new policy version (aws iam create-policy-version --set-as-default)", e.From, e.Action, e.To))
		}
	}
	return out
}

// ---------- small accessors for the web UI ----------

func (x *Graph) Kind(name string) string { return x.kind[name] }

func (x *Graph) NodeNames() []string {
	out := make([]string, 0, len(x.kind))
	for n := range x.kind {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ---------- apply (the "act" step, shared by UI, MCP and agent) ----------

type ApplyResult struct {
	Applied             []string `json:"applied"`
	PathsBefore         int      `json:"attack_paths_before"`
	PathsAfter          int      `json:"attack_paths_after"`
	Verified            bool     `json:"verified_from_graph"`
	RemediationFile     string   `json:"remediation_file"`
	RemediationCommands []string `json:"remediation_commands"`
	LegitBroken         []string `json:"legit_access_broken"`
	Collateral          []string `json:"collateral_permissions_lost"`
}

// Apply refuses unsafe fixes, mutates the graph, writes out/remediation.sh and re-queries the graph to verify.
func Apply(g *falkordb.Graph, source string, ids []string) (*ApplyResult, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("no permission ids given")
	}
	x, err := Load(g)
	if err != nil {
		return nil, err
	}
	before := NewSession(x, source, 12)
	im := before.Eval(ids)
	if !im.AttackerContained || len(im.LegitBroken) > 0 {
		return nil, fmt.Errorf("refusing to apply: attacker_contained=%v legit_broken=%v", im.AttackerContained, im.LegitBroken)
	}
	cmds := x.Remediation(ids)
	if err := ApplyFix(g, ids); err != nil {
		return nil, err
	}
	x2, err := Load(g)
	if err != nil {
		return nil, err
	}
	after := NewSession(x2, source, 12)
	file := "out/remediation.sh"
	script := "#!/bin/sh\n# generated by Chokepoint " + time.Now().Format(time.RFC3339) + "\n" + strings.Join(cmds, "\n") + "\n"
	_ = os.MkdirAll("out", 0o755)
	if err := os.WriteFile(file, []byte(script), 0o755); err != nil {
		file = "(could not write file: " + err.Error() + ")"
	}
	return &ApplyResult{
		Applied: im.Remove, PathsBefore: len(before.Paths), PathsAfter: len(after.Paths),
		Verified: len(after.Paths) == 0, RemediationFile: file, RemediationCommands: cmds,
		LegitBroken: im.LegitBroken, Collateral: im.Collateral,
	}, nil
}
