package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/FalkorDB/falkordb-go/v2"
)

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

// Derived edges: (principal)-[:ESCALATES {id, rule, via:[source edge ids]}]->(target)
// `via` lists the real permission edges that make the escalation possible.
// The min-cut step cuts those source edges, never the derived edge itself.
var rules = []struct {
	name  string
	query string
}{
	{
		// iam:PassRole on role R + ec2:RunInstances  => launch instance with R, i.e. effectively become R
		// Two separate MATCH clauses on purpose: inside ONE match Cypher forces relationships to be distinct,
		// which would wrongly require two different HAS_POLICY edges when one policy grants both permissions.
		name: "passrole_runinstances",
		query: `
MATCH (p)-[h1:HAS_POLICY]->(:Policy)-[a:ALLOWS_ON_ROLE]->(r:Role)
WHERE a.action IN ['iam:PassRole', 'iam:*'] AND p.name <> r.name
MATCH (p)-[h2:HAS_POLICY]->(:Policy)-[b:ALLOWS]->(:Resource)
WHERE b.action IN ['ec2:RunInstances', 'ec2:*']
CREATE (p)-[:ESCALATES {id: 'd-' + p.name + '-' + a.id + '-' + b.id, rule: 'passrole_runinstances', via: [h1.id, a.id, h2.id, b.id]}]->(r)`,
	},
	{
		// iam:CreatePolicyVersion on a policy attached to yourself => rewrite it to *:* => every crown jewel
		name: "createpolicyversion",
		query: `
MATCH (p)-[hx:HAS_POLICY]->(x:Policy)
MATCH (p)-[h1:HAS_POLICY]->(:Policy)-[a:ALLOWS_ON_POLICY]->(x)
WHERE a.action IN ['iam:CreatePolicyVersion', 'iam:*']
MATCH (c:Resource {crown_jewel: true})
CREATE (p)-[:ESCALATES {id: 'd-' + p.name + '-' + a.id + '-' + c.name, rule: 'createpolicyversion', via: [hx.id, h1.id, a.id]}]->(c)`,
	},
}

func main() {
	addr := flag.String("addr", "falkor://localhost:6379", "FalkorDB URL")
	graphName := flag.String("graph", "iam", "graph name")
	flag.Parse()

	db, err := falkordb.FromURL(*addr)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	g := db.SelectGraph(*graphName)

	// idempotent: drop old derived edges, rebuild
	q(g, "MATCH ()-[e:ESCALATES]->() DELETE e", nil)
	for _, r := range rules {
		q(g, r.query, nil)
	}

	fmt.Println("== derived ESCALATES edges ==")
	res := q(g, "MATCH (a)-[e:ESCALATES]->(b) RETURN a.name, e.rule, b.name, e.via ORDER BY a.name, e.rule, b.name", nil)
	for res.Next() {
		r := res.Record()
		fmt.Printf("  %v --[%v]--> %v   via %v\n", val(r, 0), val(r, 1), val(r, 2), val(r, 3))
	}

	fmt.Println("\n== attack paths to crown jewels (per user) ==")
	users := q(g, "MATCH (u:User) RETURN u.name ORDER BY u.name", nil)
	var names []string
	for users.Next() {
		names = append(names, fmt.Sprint(val(users.Record(), 0)))
	}
	for _, u := range names {
		pr := q(g, `
MATCH p=(u:User {name: $u})-[:MEMBER_OF|HAS_POLICY|CAN_ASSUME|ALLOWS|ESCALATES*1..10]->(r:Resource {crown_jewel: true})
RETURN [n IN nodes(p) | n.name] AS names,
       [e IN relationships(p) | type(e)] AS types,
       [e IN relationships(p) | e.id] AS ids`,
			map[string]interface{}{"u": u})
		count := 0
		var lines []string
		for pr.Next() {
			rec := pr.Record()
			count++
			lines = append(lines, fmt.Sprintf("    #%d %v\n        types=%v\n        ids=%v", count, val(rec, 0), val(rec, 1), val(rec, 2)))
		}
		fmt.Printf("  %s: %d path(s)\n", u, count)
		for _, l := range lines {
			fmt.Println(l)
		}
	}
}
