package main

import "testing"

func TestPlanBatch(t *testing.T) {
	a := &Repo{Name: "a"}
	b := &Repo{Name: "b", Status: Status{Files: []string{"x"}}}
	c := &Repo{Name: "c"}
	d := &Repo{Name: "d", Status: Status{Files: []string{"y", "z"}}}
	clean, dirty := planBatch([]*Repo{a, b, c, d})
	if len(clean) != 2 || clean[0] != a || clean[1] != c {
		t.Errorf("clean = %v", clean)
	}
	if len(dirty) != 2 || dirty[0] != b || dirty[1] != d {
		t.Errorf("dirty = %v", dirty)
	}
}
