package orchestrator

import (
	"testing"
)

func TestTaskGraph_TopoSort_Linear(t *testing.T) {
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "a", Description: "first"},
			{ID: "b", Description: "second", DependsOn: []string{"a"}},
			{ID: "c", Description: "third", DependsOn: []string{"b"}},
		},
	}

	waves, err := g.TopoSort()
	if err != nil {
		t.Fatalf("TopoSort failed: %v", err)
	}

	if len(waves) != 3 {
		t.Fatalf("expected 3 waves, got %d", len(waves))
	}
	if waves[0][0].ID != "a" {
		t.Errorf("wave 0: expected a, got %s", waves[0][0].ID)
	}
	if waves[1][0].ID != "b" {
		t.Errorf("wave 1: expected b, got %s", waves[1][0].ID)
	}
	if waves[2][0].ID != "c" {
		t.Errorf("wave 2: expected c, got %s", waves[2][0].ID)
	}
}

func TestTaskGraph_TopoSort_Parallel(t *testing.T) {
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "a", Description: "independent 1"},
			{ID: "b", Description: "independent 2"},
			{ID: "c", Description: "depends on both", DependsOn: []string{"a", "b"}},
		},
	}

	waves, err := g.TopoSort()
	if err != nil {
		t.Fatalf("TopoSort failed: %v", err)
	}

	if len(waves) != 2 {
		t.Fatalf("expected 2 waves, got %d", len(waves))
	}
	if len(waves[0]) != 2 {
		t.Errorf("wave 0: expected 2 tasks, got %d", len(waves[0]))
	}
	// Verify deterministic ordering within wave (follows g.Tasks input order)
	if waves[0][0].ID != "a" {
		t.Errorf("wave 0[0]: expected a, got %s", waves[0][0].ID)
	}
	if waves[0][1].ID != "b" {
		t.Errorf("wave 0[1]: expected b, got %s", waves[0][1].ID)
	}
	if len(waves[1]) != 1 {
		t.Errorf("wave 1: expected 1 task, got %d", len(waves[1]))
	}
}

func TestTaskGraph_TopoSort_AllParallel(t *testing.T) {
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "a", Description: "task a"},
			{ID: "b", Description: "task b"},
			{ID: "c", Description: "task c"},
		},
	}

	waves, err := g.TopoSort()
	if err != nil {
		t.Fatalf("TopoSort failed: %v", err)
	}

	if len(waves) != 1 {
		t.Fatalf("expected 1 wave (all parallel), got %d", len(waves))
	}
	if len(waves[0]) != 3 {
		t.Errorf("wave 0: expected 3 tasks, got %d", len(waves[0]))
	}
}

func TestTaskGraph_TopoSort_Cycle(t *testing.T) {
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "a", DependsOn: []string{"b"}},
			{ID: "b", DependsOn: []string{"a"}},
		},
	}

	_, err := g.TopoSort()
	if err == nil {
		t.Fatal("expected error for cyclic dependency")
	}
}

func TestTaskGraph_Validate_OK(t *testing.T) {
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "a", Description: "first"},
			{ID: "b", Description: "second", DependsOn: []string{"a"}},
		},
	}

	if err := g.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestTaskGraph_Validate_DuplicateID(t *testing.T) {
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "a", Description: "first"},
			{ID: "a", Description: "duplicate"},
		},
	}

	if err := g.Validate(); err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestTaskGraph_Validate_EmptyID(t *testing.T) {
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "", Description: "no id"},
		},
	}

	if err := g.Validate(); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestTaskGraph_Validate_UnknownDep(t *testing.T) {
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "a", Description: "first", DependsOn: []string{"nonexistent"}},
		},
	}

	if err := g.Validate(); err == nil {
		t.Fatal("expected error for unknown dependency")
	}
}

func TestTaskGraph_Validate_Cycle(t *testing.T) {
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "a", DependsOn: []string{"c"}},
			{ID: "b", DependsOn: []string{"a"}},
			{ID: "c", DependsOn: []string{"b"}},
		},
	}

	if err := g.Validate(); err == nil {
		t.Fatal("expected error for cyclic dependency")
	}
}

func TestTaskGraph_AssignAgents(t *testing.T) {
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "a"},
			{ID: "b", AgentName: "frontend-dev"},
			{ID: "c"},
			{ID: "d"},
		},
	}

	g.AssignAgents([]string{"backend-dev", "fullstack"})

	// "b" should keep its explicit assignment
	if g.Tasks[1].AgentName != "frontend-dev" {
		t.Errorf("task b: expected frontend-dev, got %s", g.Tasks[1].AgentName)
	}

	// Unassigned tasks should get round-robin assignments
	if g.Tasks[0].AgentName != "backend-dev" {
		t.Errorf("task a: expected backend-dev, got %s", g.Tasks[0].AgentName)
	}
	if g.Tasks[2].AgentName != "fullstack" {
		t.Errorf("task c: expected fullstack, got %s", g.Tasks[2].AgentName)
	}
	if g.Tasks[3].AgentName != "backend-dev" {
		t.Errorf("task d: expected backend-dev, got %s", g.Tasks[3].AgentName)
	}
}

func TestTaskGraph_AssignAgents_EmptyList(t *testing.T) {
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "a"},
		},
	}

	// Should not panic with empty agent list
	g.AssignAgents(nil)

	if g.Tasks[0].AgentName != "" {
		t.Errorf("expected empty agent name, got %s", g.Tasks[0].AgentName)
	}
}

func TestTaskGraph_TopoSort_SingleTask(t *testing.T) {
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "only"},
		},
	}

	waves, err := g.TopoSort()
	if err != nil {
		t.Fatalf("TopoSort failed: %v", err)
	}
	if len(waves) != 1 || len(waves[0]) != 1 {
		t.Errorf("expected 1 wave with 1 task, got %d waves", len(waves))
	}
}

func TestTaskGraph_TopoSort_Diamond(t *testing.T) {
	// Diamond: a → b, a → c, b+c → d
	g := &TaskGraph{
		Tasks: []TaskItem{
			{ID: "a"},
			{ID: "b", DependsOn: []string{"a"}},
			{ID: "c", DependsOn: []string{"a"}},
			{ID: "d", DependsOn: []string{"b", "c"}},
		},
	}

	waves, err := g.TopoSort()
	if err != nil {
		t.Fatalf("TopoSort failed: %v", err)
	}

	if len(waves) != 3 {
		t.Fatalf("expected 3 waves, got %d", len(waves))
	}
	if len(waves[0]) != 1 || waves[0][0].ID != "a" {
		t.Errorf("wave 0: expected [a], got %v", waves[0])
	}
	if len(waves[1]) != 2 {
		t.Errorf("wave 1: expected 2 tasks (b,c), got %d", len(waves[1]))
	}
	if len(waves[2]) != 1 || waves[2][0].ID != "d" {
		t.Errorf("wave 2: expected [d], got %v", waves[2])
	}
}
