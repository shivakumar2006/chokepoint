// CLI view of the minimum cut. All logic lives in ./core (shared with the API, MCP server and agent).
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"chokepoint/core"
)

func printImpact(label string, im core.Impact) {
	fmt.Printf("  %s: kills %d/%d paths, legit broken: %d, perms lost by legit users: %d\n",
		label, im.PathsKilled, im.PathsTotal, len(im.LegitBroken), len(im.Collateral))
	for _, r := range im.Remove {
		fmt.Printf("      remove %s\n", r)
	}
	for i, c := range im.Collateral {
		if i >= 5 {
			fmt.Printf("      ... and %d more\n", len(im.Collateral)-5)
			break
		}
		fmt.Printf("      loses %s\n", c)
	}
}

func main() {
	addr := flag.String("addr", "falkor://localhost:6379", "FalkorDB URL")
	graphName := flag.String("graph", "iam", "graph name")
	source := flag.String("source", "intern", "attacker identity")
	maxK := flag.Int("maxk", 5, "max cut size to try")
	flag.Parse()

	g, err := core.Connect(*addr, *graphName)
	if err != nil {
		log.Fatal(err)
	}
	x, err := core.Load(g)
	if err != nil {
		log.Fatal(err)
	}
	s := core.NewSession(x, *source, 12)
	fmt.Printf("source=%s  attack paths to crown jewels: %d  candidate permission edges: %d\n", *source, len(s.Paths), len(s.Candidates))

	t0 := time.Now()
	res := s.MinCut(*maxK)
	fmt.Printf("min-cut search took %s\n", time.Since(t0).Round(time.Millisecond))

	fmt.Println("\n== best single-permission fixes ==")
	for _, im := range res.Singles {
		status := "legit OK"
		if len(im.LegitBroken) > 0 {
			status = "BREAKS LEGIT: " + strings.Join(im.LegitBroken, ", ")
		}
		fmt.Printf("  kills %d/%d  contained=%v  %s\n      %s\n", im.PathsKilled, im.PathsTotal, im.AttackerContained, status, strings.Join(im.Remove, " + "))
	}

	if res.Size == 0 {
		fmt.Printf("\n%s\n", res.Note)
		return
	}
	fmt.Printf("\n== MINIMUM CUT SET: size %d ==\n  %s\n", res.Size, res.Note)
	for i, o := range res.Options {
		printImpact(fmt.Sprintf("Option %c", 'A'+i), o)
	}
	for _, r := range res.Rejected {
		fmt.Printf("  REJECTED (contains attacker but breaks legit: %s)\n", strings.Join(r.LegitBroken, ", "))
		for _, d := range r.Remove {
			fmt.Printf("      %s\n", d)
		}
	}
}
