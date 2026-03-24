package deploy

import (
	"strings"
	"testing"

	"github.com/angelorc/vmsan/hostd/internal/config"
)

func TestBuildDependencyGraph_Linear(t *testing.T) {
	services := map[string]config.ServiceConfig{
		"a": {Start: "start-a", DependsOn: []string{"b"}},
		"b": {Start: "start-b", DependsOn: []string{"c"}},
		"c": {Start: "start-c"},
	}

	g, err := BuildDependencyGraph(services, nil)
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error: %v", err)
	}

	if len(g.Groups) != 3 {
		t.Fatalf("len(Groups) = %d, want 3", len(g.Groups))
	}

	// c must be in the first group
	if !containsService(g.Groups[0].Services, "c") {
		t.Errorf("first group should contain 'c', got %v", g.Groups[0].Services)
	}
	// b in the second
	if !containsService(g.Groups[1].Services, "b") {
		t.Errorf("second group should contain 'b', got %v", g.Groups[1].Services)
	}
	// a in the third
	if !containsService(g.Groups[2].Services, "a") {
		t.Errorf("third group should contain 'a', got %v", g.Groups[2].Services)
	}

	// Verify Order: c before b before a
	posC := indexOf(g.Order, "c")
	posB := indexOf(g.Order, "b")
	posA := indexOf(g.Order, "a")
	if posC >= posB || posB >= posA {
		t.Errorf("Order should be c < b < a, got %v", g.Order)
	}
}

func TestBuildDependencyGraph_Parallel(t *testing.T) {
	services := map[string]config.ServiceConfig{
		"a": {Start: "start-a"},
		"b": {Start: "start-b"},
	}

	g, err := BuildDependencyGraph(services, nil)
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error: %v", err)
	}

	if len(g.Groups) != 1 {
		t.Fatalf("len(Groups) = %d, want 1", len(g.Groups))
	}
	if len(g.Groups[0].Services) != 2 {
		t.Errorf("first group should have 2 services, got %d", len(g.Groups[0].Services))
	}
	if !containsService(g.Groups[0].Services, "a") || !containsService(g.Groups[0].Services, "b") {
		t.Errorf("first group should contain a and b, got %v", g.Groups[0].Services)
	}
}

func TestBuildDependencyGraph_Diamond(t *testing.T) {
	services := map[string]config.ServiceConfig{
		"a": {Start: "start-a", DependsOn: []string{"b", "c"}},
		"b": {Start: "start-b", DependsOn: []string{"d"}},
		"c": {Start: "start-c", DependsOn: []string{"d"}},
		"d": {Start: "start-d"},
	}

	g, err := BuildDependencyGraph(services, nil)
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error: %v", err)
	}

	if len(g.Groups) != 3 {
		t.Fatalf("len(Groups) = %d, want 3", len(g.Groups))
	}

	// d in first group
	if !containsService(g.Groups[0].Services, "d") {
		t.Errorf("first group should contain 'd', got %v", g.Groups[0].Services)
	}
	if len(g.Groups[0].Services) != 1 {
		t.Errorf("first group should have 1 service, got %d", len(g.Groups[0].Services))
	}

	// b and c in second group (parallel)
	if !containsService(g.Groups[1].Services, "b") || !containsService(g.Groups[1].Services, "c") {
		t.Errorf("second group should contain b and c, got %v", g.Groups[1].Services)
	}

	// a in third group
	if !containsService(g.Groups[2].Services, "a") {
		t.Errorf("third group should contain 'a', got %v", g.Groups[2].Services)
	}
}

func TestBuildDependencyGraph_Circular(t *testing.T) {
	services := map[string]config.ServiceConfig{
		"a": {Start: "start-a", DependsOn: []string{"b"}},
		"b": {Start: "start-b", DependsOn: []string{"a"}},
	}

	_, err := BuildDependencyGraph(services, nil)
	if err == nil {
		t.Fatal("expected error for circular dependency, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("error = %q, want it to contain 'circular'", err.Error())
	}
}

func TestBuildDependencyGraph_Empty(t *testing.T) {
	g, err := BuildDependencyGraph(nil, nil)
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error: %v", err)
	}

	if len(g.Groups) != 0 {
		t.Errorf("len(Groups) = %d, want 0", len(g.Groups))
	}
	if len(g.Order) != 0 {
		t.Errorf("len(Order) = %d, want 0", len(g.Order))
	}
	if len(g.ReverseOrder) != 0 {
		t.Errorf("len(ReverseOrder) = %d, want 0", len(g.ReverseOrder))
	}
}

func TestBuildDependencyGraph_WithAccessories(t *testing.T) {
	services := map[string]config.ServiceConfig{
		"web": {Start: "npm start", DependsOn: []string{"db"}},
	}
	accessories := map[string]config.AccessoryConfig{
		"db": {Type: "postgres"},
	}

	g, err := BuildDependencyGraph(services, accessories)
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error: %v", err)
	}

	if len(g.Groups) != 2 {
		t.Fatalf("len(Groups) = %d, want 2", len(g.Groups))
	}

	// db (accessory) should be in the first group
	if !containsService(g.Groups[0].Services, "db") {
		t.Errorf("first group should contain 'db', got %v", g.Groups[0].Services)
	}
	// web should be in the second group
	if !containsService(g.Groups[1].Services, "web") {
		t.Errorf("second group should contain 'web', got %v", g.Groups[1].Services)
	}
}

func TestBuildDependencyGraph_ReverseOrder(t *testing.T) {
	services := map[string]config.ServiceConfig{
		"a": {Start: "start-a", DependsOn: []string{"b"}},
		"b": {Start: "start-b", DependsOn: []string{"c"}},
		"c": {Start: "start-c"},
	}

	g, err := BuildDependencyGraph(services, nil)
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error: %v", err)
	}

	if len(g.Order) != len(g.ReverseOrder) {
		t.Fatalf("len(Order)=%d != len(ReverseOrder)=%d", len(g.Order), len(g.ReverseOrder))
	}

	// ReverseOrder should be the exact reverse of Order
	n := len(g.Order)
	for i := 0; i < n; i++ {
		if g.Order[i] != g.ReverseOrder[n-1-i] {
			t.Errorf("Order[%d]=%q != ReverseOrder[%d]=%q", i, g.Order[i], n-1-i, g.ReverseOrder[n-1-i])
		}
	}
}

// --- helpers ---

func containsService(services []string, name string) bool {
	for _, s := range services {
		if s == name {
			return true
		}
	}
	return false
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}
