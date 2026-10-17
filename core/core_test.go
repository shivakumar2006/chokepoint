package core

import (
	"strings"
	"testing"
)

// tiny hand-built graph: same shape as the seed (intern + dev-alice in developers, DevPolicy self-escalation)
func testGraph() *Graph {
	edges := []Edge{
		{ID: "e2", Type: "MEMBER_OF", From: "intern", To: "developers"},
		{ID: "e3", Type: "MEMBER_OF", From: "dev-alice", To: "developers"},
		{ID: "e7", Type: "HAS_POLICY", From: "developers", To: "DevPolicy"},
		{ID: "e25", Type: "ALLOWS_ON_POLICY", From: "DevPolicy", To: "DevPolicy", Action: "iam:CreatePolicyVersion"},
		{ID: "e30", Type: "ALLOWS", From: "DevPolicy", To: "ci-artifacts", Action: "s3:GetObject"},
		{ID: "d1", Type: "ESCALATES", From: "developers", To: "prod-db", Via: []string{"e7", "e7", "e25"}},
		{ID: "e40", Type: "CAN_ASSUME", From: "developers", To: "ci-role"},
		{ID: "e41", Type: "HAS_POLICY", From: "ci-role", To: "CIPolicy"},
		{ID: "e42", Type: "ALLOWS", From: "CIPolicy", To: "ci-artifacts", Action: "s3:*"},
		{ID: "l1", Type: "LEGIT_ACCESS", From: "dev-alice", To: "ci-artifacts"},
	}
	x := &Graph{Edges: edges, Users: []string{"dev-alice", "intern"}, Crown: map[string]bool{"prod-db": true},
		adj: map[string][]int{}, perms: map[string][]int{}, kind: map[string]string{}, byID: map[string]int{}}
	for i, e := range edges {
		x.byID[e.ID] = i
		if walkable[e.Type] {
			x.adj[e.From] = append(x.adj[e.From], i)
		}
		switch e.Type {
		case "ALLOWS", "ALLOWS_ON_ROLE", "ALLOWS_ON_POLICY":
			x.perms[e.From] = append(x.perms[e.From], i)
		}
	}
	return x
}

func TestPathsAndMinCut(t *testing.T) {
	s := NewSession(testGraph(), "intern", 12)
	if len(s.Paths) != 1 {
		t.Fatalf("want 1 attack path, got %d", len(s.Paths))
	}
	res := s.MinCut(3)
	if res.Size != 1 {
		t.Fatalf("want min cut size 1, got %d (%s)", res.Size, res.Note)
	}
	found := map[string]bool{}
	for _, o := range res.Options {
		found[strings.Join(o.RemoveIDs, ",")] = true
		if !o.AttackerContained || len(o.LegitBroken) > 0 {
			t.Fatalf("invalid option returned: %+v", o)
		}
	}
	for _, want := range []string{"e2", "e7", "e25"} {
		if !found[want] {
			t.Fatalf("expected option %s, got %v", want, found)
		}
	}
	// cutting the intern's membership must cost fewer permissions than detaching DevPolicy for everyone
	if len(res.Options[0].RemoveIDs) != 1 || res.Options[0].RemoveIDs[0] != "e2" {
		t.Fatalf("expected e2 ranked first (least collateral), got %v", res.Options[0].RemoveIDs)
	}
}

func TestLegitIsProtected(t *testing.T) {
	x := testGraph()
	// remove the only other route for alice to ci-artifacts: now detaching DevPolicy would break legit access
	var kept []Edge
	for _, e := range x.Edges {
		if e.ID != "e40" {
			kept = append(kept, e)
		}
	}
	x.Edges = kept
	x.adj, x.perms, x.byID = map[string][]int{}, map[string][]int{}, map[string]int{}
	for i, e := range x.Edges {
		x.byID[e.ID] = i
		if walkable[e.Type] {
			x.adj[e.From] = append(x.adj[e.From], i)
		}
		switch e.Type {
		case "ALLOWS", "ALLOWS_ON_ROLE", "ALLOWS_ON_POLICY":
			x.perms[e.From] = append(x.perms[e.From], i)
		}
	}
	s := NewSession(x, "intern", 12)
	im := s.Eval([]string{"e7"})
	if !im.AttackerContained {
		t.Fatal("e7 should contain the attacker")
	}
	if len(im.LegitBroken) == 0 {
		t.Fatal("e7 should break dev-alice -> ci-artifacts when no other route exists")
	}
}

func TestRemediationCommands(t *testing.T) {
	x := testGraph()
	x.kind = map[string]string{"intern": "User", "developers": "Group"}
	cmds := x.Remediation([]string{"e2", "e7"})
	if len(cmds) != 2 || !strings.Contains(cmds[0], "remove-user-from-group --user-name intern --group-name developers") ||
		!strings.Contains(cmds[1], "detach-group-policy --group-name developers") {
		t.Fatalf("unexpected commands: %v", cmds)
	}
}
