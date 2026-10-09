package sim

import (
	"testing"
)

func TestEvaluationDeterministic(t *testing.T) {
	a := Run()
	b := Run()

	if a.Base.Assigned != b.Base.Assigned || a.Proposed.Assigned != b.Proposed.Assigned {
		t.Fatalf("evaluation is not deterministic: %+v vs %+v", a.Base, b.Base)
	}
	if a.Base.TotalDistance != b.Base.TotalDistance || a.Proposed.TotalDistance != b.Proposed.TotalDistance {
		t.Fatalf("distance not deterministic")
	}
	for name, h := range a.Proposed.HoursAfter {
		if b.Proposed.HoursAfter[name] != h {
			t.Fatalf("hours for %s not deterministic: %v vs %v", name, h, b.Proposed.HoursAfter[name])
		}
	}
}

func TestNoConflictsUnderEitherMethod(t *testing.T) {
	ev := Run()
	if ev.Base.Conflicts != 0 {
		t.Fatalf("base method produced %d schedule conflicts", ev.Base.Conflicts)
	}
	if ev.Proposed.Conflicts != 0 {
		t.Fatalf("proposed method produced %d schedule conflicts", ev.Proposed.Conflicts)
	}
}

func TestOrderWithUnknownSkillStaysUnassigned(t *testing.T) {
	ev := Run()
	hasOrder := func(ids []int64, id int64) bool {
		for _, v := range ids {
			if v == id {
				return true
			}
		}
		return false
	}
	if !hasOrder(ev.Base.UnassignedOrderIDs, 10) {
		t.Fatalf("order 10 (unknown skill) must be unassigned under base method")
	}
	if !hasOrder(ev.Proposed.UnassignedOrderIDs, 10) {
		t.Fatalf("order 10 (unknown skill) must be unassigned under proposed method")
	}
}

func TestSaturatedWindowStaysUnassigned(t *testing.T) {
	// Order 5 (coax, 08:00-12:00) only fits Epsilon, whose entire window is busy.
	ev := Run()
	hasOrder := func(ids []int64, id int64) bool {
		for _, v := range ids {
			if v == id {
				return true
			}
		}
		return false
	}
	if !hasOrder(ev.Base.UnassignedOrderIDs, 5) || !hasOrder(ev.Proposed.UnassignedOrderIDs, 5) {
		t.Fatalf("order 5 must be unassigned under both methods (saturated window)")
	}
}

func TestBothMethodsUseTheSameDataset(t *testing.T) {
	ev := Run()
	if ev.Base.Assigned+ev.Base.Unassigned != ev.OrdersTotal {
		t.Fatalf("base: assigned+unassigned != total")
	}
	if ev.Proposed.Assigned+ev.Proposed.Unassigned != ev.OrdersTotal {
		t.Fatalf("proposed: assigned+unassigned != total")
	}
	if ev.OrdersTotal != 12 {
		t.Fatalf("expected 12 orders in the dataset, got %d", ev.OrdersTotal)
	}
}

func TestHoursConservation(t *testing.T) {
	// Initial hours + hours of assigned orders must equal final hours.
	ev := Run()
	for _, m := range []struct {
		name string
		res  MethodResult
	}{{"base", ev.Base}, {"proposed", ev.Proposed}} {
		var before, after float64
		for _, h := range m.res.HoursBefore {
			before += h
		}
		for _, h := range m.res.HoursAfter {
			after += h
		}
		assignedMin := 0
		for _, a := range m.res.Assignments {
			assignedMin += a.DurationMin
		}
		want := before + float64(assignedMin)/60.0
		if after != want {
			t.Fatalf("%s: hours not conserved: before=%.2f assigned=%.2fh after=%.2f (want %.2f)",
				m.name, before, float64(assignedMin)/60.0, after, want)
		}
		if m.res.Assigned != len(m.res.Assignments) {
			t.Fatalf("%s: assigned count mismatch", m.name)
		}
	}
}
