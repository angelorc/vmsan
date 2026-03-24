package deploy

import (
	"fmt"
	"strings"

	"github.com/angelorc/vmsan/hostd/internal/config"
)

// DeployGroup represents a set of services that can be deployed in parallel.
type DeployGroup struct {
	Services []string `json:"services"`
	Level    int      `json:"level"`
}

// DependencyGraph holds the topological ordering of services.
type DependencyGraph struct {
	Groups       []DeployGroup `json:"groups"`
	Order        []string      `json:"order"`         // flattened topological order
	ReverseOrder []string      `json:"reverseOrder"`  // for shutdown (reverse)
}

// BuildDependencyGraph constructs a dependency graph using Kahn's algorithm
// and returns deployment groups (services that can deploy in parallel).
func BuildDependencyGraph(services map[string]config.ServiceConfig, accessories map[string]config.AccessoryConfig) (*DependencyGraph, error) {
	// Check for cycles first using DFS
	if err := checkCycles(services, accessories); err != nil {
		return nil, err
	}

	// Build adjacency list and in-degree map
	inDegree := make(map[string]int)
	dependents := make(map[string][]string) // node -> nodes that depend on it

	// Initialize all nodes
	for name := range services {
		inDegree[name] = 0
	}
	for name := range accessories {
		inDegree[name] = 0
	}

	// Count edges
	for name, svc := range services {
		for _, dep := range svc.DependsOn {
			if _, ok := inDegree[dep]; ok {
				inDegree[name]++
				dependents[dep] = append(dependents[dep], name)
			}
		}
	}

	// Kahn's algorithm with level grouping
	var groups []DeployGroup
	var order []string
	level := 0

	for len(inDegree) > 0 {
		// Find all nodes with in-degree 0
		var ready []string
		for name, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, name)
			}
		}

		if len(ready) == 0 {
			// Should not happen if cycle check passed
			break
		}

		groups = append(groups, DeployGroup{
			Services: ready,
			Level:    level,
		})
		order = append(order, ready...)

		// Remove ready nodes and update in-degrees
		for _, name := range ready {
			delete(inDegree, name)
			for _, dependent := range dependents[name] {
				if _, ok := inDegree[dependent]; ok {
					inDegree[dependent]--
				}
			}
		}
		level++
	}

	// Build reverse order for shutdown
	reverse := make([]string, len(order))
	for i, name := range order {
		reverse[len(order)-1-i] = name
	}

	return &DependencyGraph{
		Groups:       groups,
		Order:        order,
		ReverseOrder: reverse,
	}, nil
}

// checkCycles uses DFS three-color marking to detect circular dependencies.
func checkCycles(services map[string]config.ServiceConfig, accessories map[string]config.AccessoryConfig) error {
	const (
		white = 0
		gray  = 1
		black = 2
	)

	color := make(map[string]int)
	for name := range services {
		color[name] = white
	}
	for name := range accessories {
		color[name] = white
	}

	var cyclePath []string

	var dfs func(node string) bool
	dfs = func(node string) bool {
		color[node] = gray
		cyclePath = append(cyclePath, node)

		if svc, ok := services[node]; ok {
			for _, dep := range svc.DependsOn {
				if _, exists := color[dep]; !exists {
					continue
				}
				switch color[dep] {
				case white:
					if dfs(dep) {
						return true
					}
				case gray:
					// Found cycle — extract it
					cyclePath = append(cyclePath, dep)
					return true
				}
			}
		}

		color[node] = black
		cyclePath = cyclePath[:len(cyclePath)-1]
		return false
	}

	for name := range color {
		if color[name] == white {
			cyclePath = nil
			if dfs(name) {
				return fmt.Errorf("circular dependency: %s", strings.Join(cyclePath, " -> "))
			}
		}
	}

	return nil
}
