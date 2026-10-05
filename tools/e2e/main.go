// Command e2e drives the real pipeline over the real workbooks in raw_data and
// then audits the exported file for fidelity. It is the acceptance check for the
// whole backend:
//
//	go run ./tools/e2e -in raw_data/mps_data -out /tmp/clear-e2e
//	go run ./tools/e2e -in raw_data/mps_data -out /tmp/clear-e2e -clean
//
// The work itself lives in tools/e2e/harness, which the tests in that package
// also call, so the acceptance path and the automated one cannot drift apart.
package main

import (
	"flag"
	"fmt"
	"os"

	"project_clear/tools/e2e/harness"
)

func main() {
	in := flag.String("in", "raw_data/mps_data", "folder of source workbooks")
	out := flag.String("out", "/tmp/clear-e2e", "scratch folder for the database and export")
	clean := flag.Bool("clean", false, "use the clean rebuild export engine")
	keep := flag.Bool("keep", false, "keep the scratch folder from a previous run")
	flag.Parse()

	err := harness.Run(harness.Options{
		In:     *in,
		Out:    *out,
		Clean:  *clean,
		Keep:   *keep,
		Report: os.Stdout,
	})
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
		os.Exit(1)
	}
}
