package domain

import "testing"

func TestBlockingSemanticsMatchExcludeConstraint(t *testing.T) {
	// The DB EXCLUDE constraint blocks confirmed, in_progress and completed.
	if !AssignmentConfirmed.Blocking() || !AssignmentInProgress.Blocking() || !AssignmentCompleted.Blocking() {
		t.Fatalf("confirmed, in_progress and completed must block")
	}
	if AssignmentCancelled.Blocking() || AssignmentReplaced.Blocking() {
		t.Fatalf("cancelled and replaced must not block")
	}
}

func TestOrderTransitions(t *testing.T) {
	cases := []struct {
		from, to OrderStatus
		want     bool
	}{
		{OrderPending, OrderAssigned, true},
		{OrderPending, OrderCompleted, false},
		{OrderAssigned, OrderInProgress, true},
		{OrderInProgress, OrderCompleted, true},
		{OrderInProgress, OrderAssigned, false},
		{OrderCompleted, OrderPending, false},
		{OrderIncident, OrderPending, true},
	}
	for _, c := range cases {
		if got := ValidOrderTransition(c.from, c.to); got != c.want {
			t.Fatalf("%s -> %s: got %v, want %v", c.from, c.to, got, c.want)
		}
	}
}
