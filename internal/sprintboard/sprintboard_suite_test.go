//go:build integration

package sprintboard_test

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// Run the Ginkgo suite via go test's TestMain. The standard
// `ginkgo bootstrap` template is preserved verbatim to match every
// other Helixon core service per L0 rule 42 (integration-test-gating).
func TestSprintboard(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "sprintboard-mcp internal/sprintboard package")
}

// _ keeps imports referenced even before the first spec is written
// (RED state). Once specs below compile, this alias can be dropped —
// keep it as documentation that RED state compiles.
var (
	_ = sprintboard.NewStore
	_ = ginkgo.Describe
	_ = gomega.Expect
)
