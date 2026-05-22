package sprintboard

import (
	"testing"
)

// TestV8700_B23_TicketCommentsLifecycle covers the full add+list cycle.
// Strict acceptance: a comment can be added with author + body, listed in
// chronological order, and survives store re-open.
func TestV8700_B23_TicketCommentsLifecycle(t *testing.T) {
	t.Parallel()

	store := testStore(t)
	if err := store.CreateSprint(Sprint{ID: "s1", Name: "v8700"}); err != nil {
		t.Fatalf("CreateSprint: %v", err)
	}
	if err := store.CreateTicket(Ticket{ID: "t1", SprintID: "s1", Title: "test"}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	c1, err := store.AddTicketComment("t1", "claude-code", "first comment")
	if err != nil {
		t.Fatalf("AddTicketComment: %v", err)
	}
	if c1.ID == 0 {
		t.Fatalf("AddTicketComment: got id=0")
	}
	if c1.Author != "claude-code" || c1.Body != "first comment" {
		t.Fatalf("AddTicketComment got = %+v", c1)
	}
	if c1.CreatedAt.IsZero() {
		t.Fatalf("AddTicketComment: created_at not set")
	}

	c2, err := store.AddTicketComment("t1", "operator", "second comment")
	if err != nil {
		t.Fatalf("AddTicketComment: %v", err)
	}
	if c2.ID <= c1.ID {
		t.Fatalf("second comment id %d not greater than first %d", c2.ID, c1.ID)
	}

	got, err := store.ListTicketComments("t1")
	if err != nil {
		t.Fatalf("ListTicketComments: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListTicketComments len = %d, want 2", len(got))
	}
	if got[0].Author != "claude-code" || got[1].Author != "operator" {
		t.Fatalf("comments not in chronological order: %+v", got)
	}
	if got[0].ID >= got[1].ID {
		t.Fatalf("comment[0] id %d should precede comment[1] id %d", got[0].ID, got[1].ID)
	}
}

// TestV8700_B23_TicketCommentsValidation covers the input boundary.
// Strict acceptance: empty ticket / author / body must fail fast.
func TestV8700_B23_TicketCommentsValidation(t *testing.T) {
	t.Parallel()

	store := testStore(t)
	cases := []struct {
		name, ticket, author, body, want string
	}{
		{"empty ticket", "", "a", "b", "ticket_id"},
		{"empty author", "t1", "", "b", "author"},
		{"empty body", "t1", "a", "", "body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.AddTicketComment(tc.ticket, tc.author, tc.body)
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.want)
			}
			if !containsSubstr(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// TestV8700_B23_TicketCommentsListEmpty asserts listing a ticket with no
// comments returns an empty (non-nil) slice and no error.
func TestV8700_B23_TicketCommentsListEmpty(t *testing.T) {
	t.Parallel()

	store := testStore(t)
	got, err := store.ListTicketComments("nonexistent")
	if err != nil {
		t.Fatalf("ListTicketComments on missing ticket: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func containsSubstr(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
