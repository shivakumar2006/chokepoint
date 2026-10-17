package main

import (
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/FalkorDB/falkordb-go/v2"
)

// ---------- falkordb helpers ----------

func q(g *falkordb.Graph, query string, params map[string]interface{}) *falkordb.QueryResult {
	res, err := g.Query(query, params, nil)
	if err != nil {
		log.Fatalf("query failed: %v\n%s", err, query)
	}
	return res
}

func val(r *falkordb.Record, i int) interface{} {
	v, err := r.GetByIndex(i)
	if err != nil {
		log.Fatalf("record index %d: %v", i, err)
	}
	return v
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// ---------- in-memory graph ----------

type Edge struct {
	ID, Type, From, To, Action string
	Via                        []string // for ESCALATES: the real edges it depends on (ALL must exist)
}

type G struct {
	edges     []Edge
	adj       map[string][]int // traversable edges only
	crown     map[string]bool
	resources []string // non-crown resources
	users     []string
	perms     map[string][]int // permission edges (ALLOWS / ALLOWS_ON_*) by owning policy
}

// edge types an attacker (or a legit principal) can actually walk along
var traversable = map[string]bool{
	"MEMBER_OF": true, "HAS_POLICY": true, "CAN_ASSUME": true, "ALLOWS": true, "ESCALATES": true,
}

func load(g *falkordb.Graph) *G {
	x := &G{adj: map[string][]int{}, crown: map[string]bool{}, perms: map[string][]int{}}

	res := q(g, "MATCH (a)-[e]->(b) RETURN a.name, type(e), b.name, e.id, e.action, e.via", nil)
	for res.Next() {
		r := res.Record()
		e := Edge{From: str(val(r, 0)), Type: str(val(r, 1)), To: str(val(r, 2)), ID: str(val(r, 3)), Action: str(val(r, 4))}
		if arr, ok := val(r, 5).([]interface{}); ok {
			for _, v := range arr {
				e.Via = append(e.Via, str(v))
			}
		}
		idx := len(x.edges)
		x.edges = append(x.edges, e)
		if traversable[e.Type] {
			x.adj[e.From] = append(x.adj[e.From], idx)
		}
		if e.Type == "ALLOWS" || e.Type == "ALLOWS_ON_ROLE" || e.Type == "ALLOWS_ON_POLICY" {
			x.perms[e.From] = append(x.perms[e.From], idx)
		}
	}

	rs := q(g, "MATCH (r:Resource) RETURN r.name, r.crown_jewel ORDER BY r.name", nil)
	for rs.Next() {
		r := rs.Record()
		name := str(val(r, 0))
		if b, ok := val(r, 1).(bool); ok && b {
			x.crown[name] = true
		} else {
			x.resources = append(x.resources, name)
		}
	}
	us := q(g, "MATCH (u:User) RETURN u.name ORDER BY u.name", nil)
	for us.Next() {
		x.users = append(x.users, str(val(us.Record(), 0)))
	}
	return x
}

func (x *G) alive(e Edge, removed map[string]bool) bool {
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

// reach returns every node reachable from src once the `removed` permission edges are gone.
// A derived ESCALATES edge dies if ANY of its via edges is removed (it is an AND of permissions).
func (x *G) reach(src string, removed map[string]bool) map[string]bool {
	return x.reachOpt(src, removed, false)
}

// reachOpt with noEsc=true ignores privilege-escalation edges = what the identity is *meant* to have.
func (x *G) reachOpt(src string, removed map[string]bool, noEsc bool) map[string]bool {
	seen := map[string]bool{src: true}
	queue := []string{src}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, ei := range x.adj[n] {
			e := x.edges[ei]
			if (noEsc && e.Type == "ESCALATES") || !x.alive(e, removed) || seen[e.To] {
				continue
			}
			seen[e.To] = true
			queue = append(queue, e.To)
		}
	}
	return seen
}

// effPerms = the individual permission statements (edge ids) an identity legitimately holds
// (no escalation), so collateral is counted in permissions, not just in reachable resources.
func (x *G) effPerms(src string, removed map[string]bool) map[string]bool {
	out := map[string]bool{}
	for n := range x.reachOpt(src, removed, true) {
		for _, ei := range x.perms[n] {
			if !removed[x.edges[ei].ID] {
				out[x.edges[ei].ID] = true
			}
		}
	}
	return out
}

func (x *G) attackPaths(src string, maxDepth int) [][]int {
	var out [][]int
	visited := map[string]bool{src: true}
	var path []int
	var dfs func(n string)
	dfs = func(n string) {
		if len(path) >= maxDepth {
			return
		}
		for _, ei := range x.adj[n] {
			e := x.edges[ei]
			if visited[e.To] {
				continue
			}
			path = append(path, ei)
			if x.crown[e.To] {
				out = append(out, append([]int(nil), path...))
			} else {
				visited[e.To] = true
				dfs(e.To)
				visited[e.To] = false
			}
			path = path[:len(path)-1]
		}
	}
	dfs(src)
	return out
}

// pathIDs = every real permission edge id whose removal would kill this path
func (x *G) pathIDs(path []int) map[string]bool {
	ids := map[string]bool{}
	for _, ei := range path {
		e := x.edges[ei]
		if e.Type == "ESCALATES" {
			for _, v := range e.Via {
				ids[v] = true
			}
		} else {
			ids[e.ID] = true
		}
	}
	return ids
}

func (x *G) describe(id string) string {
	for _, e := range x.edges {
		if e.ID == id {
			if e.Action != "" {
				return fmt.Sprintf("%s -[%s %s]-> %s", e.From, e.Type, e.Action, e.To)
			}
			return fmt.Sprintf("%s -[%s]-> %s", e.From, e.Type, e.To)
		}
	}
	return id
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

type Eval struct {
	clean       bool     // attacker can no longer reach any crown jewel
	legitBroken []string // legit access that would stop working
	collateral  []string // non-crown user->resource access lost
	killed      int      // attack paths killed
}

func main() {
	addr := flag.String("addr", "falkor://localhost:6379", "FalkorDB URL")
	graphName := flag.String("graph", "iam", "graph name")
	source := flag.String("source", "intern", "attacker identity")
	maxK := flag.Int("maxk", 4, "max cut size to try")
	flag.Parse()

	db, err := falkordb.FromURL(*addr)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	x := load(db.SelectGraph(*graphName))

	paths := x.attackPaths(*source, 12)
	fmt.Printf("source=%s  attack paths to crown jewels: %d\n", *source, len(paths))
	if len(paths) == 0 {
		fmt.Println("nothing to cut (did you run ./ingest and ./rules?)")
		return
	}

	idSets := make([]map[string]bool, len(paths))
	candSet := map[string]bool{}
	for i, p := range paths {
		idSets[i] = x.pathIDs(p)
		for id := range idSets[i] {
			candSet[id] = true
		}
	}
	var cands []string
	for id := range candSet {
		cands = append(cands, id)
	}
	sort.Strings(cands)
	if len(cands) > 30 {
		fmt.Printf("warning: %d candidate edges, brute force will be slow (fine up to ~30)\n", len(cands))
	}

	// baseline reachability (before any fix)
	none := map[string]bool{}
	basePerm := map[string]map[string]bool{}
	for _, u := range x.users {
		basePerm[u] = x.effPerms(u, none)
	}
	type pair struct{ from, to string }
	var legit []pair // only legit access that works today; it must keep working
	for _, e := range x.edges {
		if e.Type == "LEGIT_ACCESS" && x.reach(e.From, none)[e.To] {
			legit = append(legit, pair{e.From, e.To})
		}
	}

	evaluate := func(S []string) Eval {
		removed := map[string]bool{}
		for _, id := range S {
			removed[id] = true
		}
		ev := Eval{clean: true}
		r := x.reach(*source, removed)
		for c := range x.crown {
			if r[c] {
				ev.clean = false
			}
		}
		for _, l := range legit {
			if !x.reach(l.from, removed)[l.to] {
				ev.legitBroken = append(ev.legitBroken, l.from+" -> "+l.to)
			}
		}
		for _, u := range x.users {
			after := x.effPerms(u, removed)
			for id := range basePerm[u] {
				if !after[id] {
					ev.collateral = append(ev.collateral, u+": "+x.describe(id))
				}
			}
		}
		sort.Strings(ev.collateral)
		for i := range paths {
			for _, id := range S {
				if idSets[i][id] {
					ev.killed++
					break
				}
			}
		}
		return ev
	}

	// ---- single-edge counterfactuals ----
	type single struct {
		id string
		ev Eval
	}
	var singles []single
	for _, id := range cands {
		singles = append(singles, single{id, evaluate([]string{id})})
	}
	sort.Slice(singles, func(i, j int) bool { return singles[i].ev.killed > singles[j].ev.killed })
	fmt.Println("\n== single-permission fixes (top 8 by attack paths killed) ==")
	for i, s := range singles {
		if i >= 8 {
			break
		}
		status := "legit OK"
		if len(s.ev.legitBroken) > 0 {
			status = "BREAKS LEGIT: " + strings.Join(s.ev.legitBroken, ", ")
		}
		fmt.Printf("  kills %d/%d  collateral=%d perms  %s\n      %s\n", s.ev.killed, len(paths), len(s.ev.collateral), status, x.describe(s.id))
	}

	// ---- minimum cut set ----
	type solution struct {
		ids []string
		ev  Eval
	}
	var rejected []solution
	for k := 1; k <= *maxK; k++ {
		var sols []solution
		combos(cands, k, func(S []string) {
			ev := evaluate(S)
			if !ev.clean {
				return
			}
			cp := append([]string(nil), S...)
			if len(ev.legitBroken) > 0 {
				rejected = append(rejected, solution{cp, ev})
				return
			}
			sols = append(sols, solution{cp, ev})
		})
		if len(sols) == 0 {
			continue
		}
		sort.Slice(sols, func(i, j int) bool {
			if len(sols[i].ev.collateral) != len(sols[j].ev.collateral) {
				return len(sols[i].ev.collateral) < len(sols[j].ev.collateral)
			}
			return strings.Join(sols[i].ids, ",") < strings.Join(sols[j].ids, ",")
		})
		fmt.Printf("\n== MINIMUM CUT SET: size %d (%d valid option(s), ranked by collateral) ==\n", k, len(sols))
		for i, s := range sols {
			if i >= 5 {
				break
			}
			fmt.Printf("  Option %c: kills %d/%d attack paths, legit broken: 0, collateral (permissions lost by legit users): %d\n", 'A'+i, s.ev.killed, len(paths), len(s.ev.collateral))
			for _, id := range s.ids {
				fmt.Printf("      remove %s\n", x.describe(id))
			}
			for j, c := range s.ev.collateral {
				if j >= 6 {
					fmt.Printf("      ... and %d more\n", len(s.ev.collateral)-6)
					break
				}
				fmt.Printf("      loses %s\n", c)
			}
		}
		shown := 0
		for _, r := range rejected {
			if len(r.ids) != k || shown >= 3 {
				continue
			}
			shown++
			fmt.Printf("  REJECTED (cuts attack but breaks legit %s):\n", strings.Join(r.ev.legitBroken, ", "))
			for _, id := range r.ids {
				fmt.Printf("      %s\n", x.describe(id))
			}
		}
		return
	}
	fmt.Printf("\nno legit-safe cut of size <= %d exists\n", *maxK)
}
