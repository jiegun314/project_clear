package harness

import (
	"bytes"
	"os"
	"testing"
)

// The department workbooks under raw_data are real business data (suppliers,
// factories, SKUs, weekly demand), so they are deliberately not in version
// control. On a machine that has them this is the full acceptance check; anywhere
// else it skips, because the synthetic test above already covers the contract.
//
// It takes a couple of minutes, so `go test -short` skips it too.
func TestAcceptanceRunOnTheRealWorkbooks(t *testing.T) {
	const in = "../../../raw_data/mps_data"
	if testing.Short() {
		t.Skip("skipping the acceptance run over the real workbooks in short mode")
	}
	if _, err := os.Stat(in); err != nil {
		t.Skipf("no real workbooks at %s; the synthetic test covers the contract", in)
	}

	for _, mode := range []struct {
		name  string
		clean bool
	}{{"template", false}, {"clean", true}} {
		t.Run(mode.name, func(t *testing.T) {
			var report bytes.Buffer
			err := Run(Options{
				In:     in,
				Out:    t.TempDir(),
				Clean:  mode.clean,
				Report: &report,
			})
			if err != nil {
				t.Fatalf("acceptance run failed: %v\n--- report ---\n%s", err, report.String())
			}
			if !bytes.Contains(report.Bytes(), []byte("全部检查通过")) {
				t.Errorf("the run did not report success:\n%s", report.String())
			}
		})
	}
}
