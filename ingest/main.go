package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/FalkorDB/falkordb-go/v2"
)

// ---------- input schema ----------

type Statement struct {
	Actions   []string `json:"actions"`
	Resources []string `json:"resources"`
}
type Policy struct {
	Name       string      `json:"name"`
	Statements []Statement `json:"statements"`
}
type Group struct {
	Name     string   `json:"name"`
	Policies []string `json:"policies"`
}
type User struct {
	Name     string   `json:"name"`
	Groups   []string `json:"groups"`
	Policies []string `json:"policies"`
}
type Role struct {
	Name     string   `json:"name"`
	Trust    []string `json:"trust"`
	Policies []string `json:"policies"`
}
type Resource struct {
	Name       string `json:"name"`
	CrownJewel bool   `json:"crown_jewel"`
}
type IAM struct {
	Resources []Resource `json:"resources"`
	Policies  []Policy   `json:"policies"`
	Groups    []Group    `json:"groups"`
	Users     []User     `json:"users"`
	Roles     []Role     `json:"roles"`
}
type Legit struct {
	Principal string `json:"principal"`
	Resource  string `json:"resource"`
}

// ---------- falkordb helper (only place that touches the client API) ----------

func q(g *falkordb.Graph, query string, params map[string]interface{}) *falkordb.QueryResult {
	res, err := g.Query(query, params, nil)
	if err != nil {
		log.Fatalf("query failed: %v\n%s", err, query)
	}
	return res
}

// val unwraps GetByIndex, which returns (value, error) in falkordb-go v2.
func val(r *falkordb.Record, i int) interface{} {
	v, err := r.GetByIndex(i)
	if err != nil {
		log.Fatalf("record index %d: %v", i, err)
	}
	return v
}

// ---------- ingest ----------

var edgeCounter int

func nextID() string {
	edgeCounter++
	return fmt.Sprintf("e%d", edgeCounter)
}

// edge creates (from)-[rel {id, action?}]->(to). Labels can't be parameterised in Cypher,
// so they come from a fixed internal set via Sprintf; names/props are real params.
func edge(g *falkordb.Graph, fromLabel, from, rel, toLabel, to, action string) {
	params := map[string]interface{}{"from": from, "to": to, "id": nextID()}
	props := "id: $id"
	if action != "" {
		params["action"] = action
		props += ", action: $action"
	}
	query := fmt.Sprintf(
		"MATCH (a:%s {name: $from}), (b:%s {name: $to}) CREATE (a)-[:%s {%s}]->(b)",
		fromLabel, toLabel, rel, props)
	q(g, query, params)
}

func node(g *falkordb.Graph, label, name string, extra string, params map[string]interface{}) {
	if params == nil {
		params = map[string]interface{}{}
	}
	params["name"] = name
	query := fmt.Sprintf("MERGE (n:%s {name: $name})", label)
	if extra != "" {
		query += " SET " + extra
	}
	q(g, query, params)
}

func readJSON(path string, v interface{}) {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		log.Fatalf("parse %s: %v", path, err)
	}
}

func main() {
	addr := flag.String("addr", "falkor://localhost:6379", "FalkorDB URL")
	graphName := flag.String("graph", "iam", "graph name")
	iamPath := flag.String("iam", "data/iam.json", "IAM json")
	legitPath := flag.String("legit", "data/legit.json", "legit access json")
	flag.Parse()

	var iam IAM
	var legit []Legit
	readJSON(*iamPath, &iam)
	readJSON(*legitPath, &legit)

	db, err := falkordb.FromURL(*addr)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	g := db.SelectGraph(*graphName)

	// fresh start every run
	q(g, "MATCH (n) DETACH DELETE n", nil)

	// name -> label (names must be unique across kinds)
	kind := map[string]string{}
	register := func(name, label string) {
		if prev, ok := kind[name]; ok {
			log.Fatalf("duplicate name %q (%s and %s); names must be unique across kinds", name, prev, label)
		}
		kind[name] = label
	}

	// 1) nodes
	for _, r := range iam.Resources {
		register(r.Name, "Resource")
		node(g, "Resource", r.Name, "n.crown_jewel = $cj", map[string]interface{}{"cj": r.CrownJewel})
	}
	for _, p := range iam.Policies {
		register(p.Name, "Policy")
		node(g, "Policy", p.Name, "", nil)
	}
	for _, x := range iam.Groups {
		register(x.Name, "Group")
		node(g, "Group", x.Name, "", nil)
	}
	for _, x := range iam.Users {
		register(x.Name, "User")
		node(g, "User", x.Name, "", nil)
	}
	for _, x := range iam.Roles {
		register(x.Name, "Role")
		node(g, "Role", x.Name, "", nil)
	}

	mustKind := func(name, want string) {
		got, ok := kind[name]
		if !ok {
			log.Fatalf("unknown name %q in input", name)
		}
		if want != "" && got != want {
			log.Fatalf("%q is a %s, expected %s", name, got, want)
		}
	}

	// 2) membership + policy attachment + trust
	for _, u := range iam.Users {
		for _, grp := range u.Groups {
			mustKind(grp, "Group")
			edge(g, "User", u.Name, "MEMBER_OF", "Group", grp, "")
		}
		for _, p := range u.Policies {
			mustKind(p, "Policy")
			edge(g, "User", u.Name, "HAS_POLICY", "Policy", p, "")
		}
	}
	for _, grp := range iam.Groups {
		for _, p := range grp.Policies {
			mustKind(p, "Policy")
			edge(g, "Group", grp.Name, "HAS_POLICY", "Policy", p, "")
		}
	}
	for _, r := range iam.Roles {
		for _, p := range r.Policies {
			mustKind(p, "Policy")
			edge(g, "Role", r.Name, "HAS_POLICY", "Policy", p, "")
		}
		for _, who := range r.Trust {
			mustKind(who, "")
			edge(g, kind[who], who, "CAN_ASSUME", "Role", r.Name, "")
		}
	}

	// 3) permissions. Edge type depends on what the permission points at:
	//    Resource -> ALLOWS (directly traversable)
	//    Role     -> ALLOWS_ON_ROLE   (e.g. iam:PassRole; NOT an access path by itself)
	//    Policy   -> ALLOWS_ON_POLICY (e.g. iam:CreatePolicyVersion; NOT an access path by itself)
	// The privesc rules (step 3) turn the latter two into derived edges.
	for _, p := range iam.Policies {
		for _, st := range p.Statements {
			for _, res := range st.Resources {
				for _, act := range st.Actions {
					switch kind[res] {
					case "Resource":
						edge(g, "Policy", p.Name, "ALLOWS", "Resource", res, act)
					case "Role":
						edge(g, "Policy", p.Name, "ALLOWS_ON_ROLE", "Role", res, act)
					case "Policy":
						edge(g, "Policy", p.Name, "ALLOWS_ON_POLICY", "Policy", res, act)
					default:
						log.Fatalf("policy %s targets unknown %q", p.Name, res)
					}
				}
			}
		}
	}

	// 4) legit access (ground truth, becomes infinite-capacity in min-cut)
	for _, l := range legit {
		mustKind(l.Principal, "")
		mustKind(l.Resource, "Resource")
		edge(g, kind[l.Principal], l.Principal, "LEGIT_ACCESS", "Resource", l.Resource, "")
	}

	// 5) sanity output
	fmt.Println("== node counts ==")
	res := q(g, "MATCH (n) RETURN labels(n)[0] AS label, count(n) AS c ORDER BY label", nil)
	for res.Next() {
		r := res.Record()
		fmt.Printf("  %v: %v\n", val(r, 0), val(r, 1))
	}
	fmt.Println("== edge counts ==")
	res = q(g, "MATCH ()-[e]->() RETURN type(e) AS t, count(e) AS c ORDER BY t", nil)
	for res.Next() {
		r := res.Record()
		fmt.Printf("  %v: %v\n", val(r, 0), val(r, 1))
	}

	fmt.Println("== direct reachability per user (no privesc yet) ==")
	for _, u := range iam.Users {
		res = q(g,
			"MATCH (u:User {name: $u})-[:MEMBER_OF|HAS_POLICY|CAN_ASSUME|ALLOWS*1..8]->(r:Resource) RETURN DISTINCT r.name ORDER BY r.name",
			map[string]interface{}{"u": u.Name})
		var names []string
		for res.Next() {
			names = append(names, fmt.Sprint(val(res.Record(), 0)))
		}
		fmt.Printf("  %-14s -> %v\n", u.Name, names)
	}
	fmt.Println("done. open http://localhost:3000 and run: MATCH (n) RETURN n LIMIT 100")
}
