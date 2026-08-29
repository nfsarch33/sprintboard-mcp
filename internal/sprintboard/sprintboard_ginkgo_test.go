//go:build integration

package sprintboard_test

import (
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// ClaimHistory is a new API introduced by q10b-3. The Ginkgo spec is
// RED on main (no implementation), GREEN once the method is added to
// sprintboard.Store.
//
// TDD contract (L0 rule 42):
//   - RED: this spec fails to compile on main because ClaimHistory
//     does not exist.
//   - GREEN: after the implementation lands, this spec passes.
//
// Spec design rationale: ClaimHistory returns the audit log of every
// claim attempt against a ticket (success + conflict). Order is
// chronological (oldest first) so audit tooling can render a
// timeline. Returns an empty slice (not nil) when the ticket has no
// claims yet.
var _ = ginkgo.Describe("sprintboard.Store ClaimHistory", func() {
	var (
		store *sprintboard.Store
		dir   string
	)

	ginkgo.BeforeEach(func() {
		dir = ginkgo.GinkgoT().TempDir()
		dbPath := filepath.Join(dir, "test.db")
		s, err := sprintboard.NewStore(dbPath)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		store = s
		// Seed a sprint + ticket so ClaimTicket can succeed.
		err = store.CreateSprint(sprintboard.Sprint{ID: "S1", Name: "test"})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		err = store.CreateTicket(sprintboard.Ticket{
			ID:       "T1",
			SprintID: "S1",
			Title:    "task",
			Status:   sprintboard.StatusReady,
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.When("the ticket has never been claimed", func() {
		ginkgo.It("returns an empty slice (not nil)", func() {
			history := store.ClaimHistory("T1")
			gomega.Expect(history).NotTo(gomega.BeNil())
			gomega.Expect(history).To(gomega.BeEmpty())
		})
	})

	ginkgo.When("the ticket has been claimed once", func() {
		ginkgo.It("returns one entry with the claimer", func() {
			_, err := store.ClaimTicket("T1", "cursor-parent")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			history := store.ClaimHistory("T1")
			gomega.Expect(history).To(gomega.HaveLen(1))
			gomega.Expect(history[0].ClaimedBy).To(gomega.Equal("cursor-parent"))
			gomega.Expect(history[0].TicketID).To(gomega.Equal("T1"))
		})
	})

	ginkgo.When("the ticket has been claimed and released", func() {
		ginkgo.It("returns the audit timeline in chronological order", func() {
			_, err := store.ClaimTicket("T1", "agent-a")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			_, err = store.ClaimTicket("T1", "agent-b")
			// agent-b will hit a conflict; the conflict itself is recorded
			// in the history.
			history := store.ClaimHistory("T1")
			gomega.Expect(len(history)).To(gomega.BeNumerically(">=", 1))
			// First entry must be the original successful claim by agent-a.
			gomega.Expect(history[0].ClaimedBy).To(gomega.Equal("agent-a"))
		})
	})
})
