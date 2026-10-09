// Command evaluate runs the base-vs-proposed comparison on the fixed
// synthetic dataset and prints the JSON report to stdout.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"pu1/backend/internal/sim"
)

func main() {
	ev := sim.Run()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(ev); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
