package kernel

import (
	"fmt"
	"slices"

	"go.edgescale.dev/kernel/internal"
)

// validateAndSort validates the dependency graph and returns a topological sort order.
// It checks for missing dependencies and circular dependencies.
func (k *Kernel) validateAndSort() ([]string, error) {
	ids := make(map[string]bool, len(k.manifests))
	for id := range k.manifests {
		ids[id] = true
	}

	// Check for missing dependencies.
	for id, m := range k.manifests {
		for _, dep := range m.DependsOn {
			if !ids[dep] {
				return nil, fmt.Errorf("module %q depends on %q, which is not registered", id, dep)
			}
		}
	}

	// Topological sort using Kahn's algorithm.
	return internal.TopoSort(k.manifests)
}

// reverseDepOrder returns modules in reverse topological order (for shutdown).
func (k *Kernel) reverseDepOrder() []string {
	if len(k.depOrder) == 0 {
		return nil
	}
	reversed := make([]string, len(k.depOrder))
	for i, id := range k.depOrder {
		reversed[len(k.depOrder)-1-i] = id
	}
	return reversed
}

// dependents returns all transitive dependents of a given module ID.
// Used for cascade-disable warnings in admin panels.
func (k *Kernel) dependents(moduleID string) []string {
	// Build reverse dependency map: module → modules that depend on it.
	reverseDeps := make(map[string][]string)
	for id, m := range k.manifests {
		for _, dep := range m.DependsOn {
			reverseDeps[dep] = append(reverseDeps[dep], id)
		}
	}

	// BFS to find all transitive dependents.
	visited := make(map[string]bool)
	queue := []string{moduleID}
	var result []string

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		for _, dep := range reverseDeps[node] {
			if !visited[dep] {
				visited[dep] = true
				result = append(result, dep)
				queue = append(queue, dep)
			}
		}
	}

	slices.Sort(result)
	return result
}
