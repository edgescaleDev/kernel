package internal

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/edgescaleDev/kernel/sdk"
)

// TopoSort performs a topological sort using Kahn's algorithm.
// Returns an error if a cycle is detected.
// Output is deterministic - nodes at the same depth are sorted lexicographically.
func TopoSort(manifests map[string]sdk.Manifest) ([]string, error) {
	// Build adjacency list and in-degree count.
	inDegree := make(map[string]int, len(manifests))
	dependents := make(map[string][]string, len(manifests))

	for id := range manifests {
		inDegree[id] = 0
	}
	for id, m := range manifests {
		for _, dep := range m.DependsOn {
			dependents[dep] = append(dependents[dep], id)
			inDegree[id]++
		}
	}

	// Start with nodes that have no dependencies.
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	slices.Sort(queue) // deterministic order

	var order []string
	for len(queue) > 0 {
		// Pop first element.
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		// Reduce in-degree for dependents.
		deps := dependents[node]
		slices.Sort(deps)
		for _, dep := range deps {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(manifests) {
		// Find the cycle members for a useful error message.
		var cycleMembers []string
		for id, deg := range inDegree {
			if deg > 0 {
				cycleMembers = append(cycleMembers, id)
			}
		}
		slices.Sort(cycleMembers)
		return nil, fmt.Errorf("circular dependency detected among: %v", cycleMembers)
	}

	return order, nil
}

// CollectMigrationFiles returns all .up.sql files from the FS, sorted by name.
func CollectMigrationFiles(migrations fs.FS) ([]string, error) {
	var files []string
	err := fs.WalkDir(migrations, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".up.sql") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(files, func(a, b string) int {
		return strings.Compare(filepath.Base(a), filepath.Base(b))
	})
	return files, nil
}

// CollectDownFiles returns a set of all .down.sql filenames available in the FS.
func CollectDownFiles(migrations fs.FS) (map[string]bool, error) {
	files := make(map[string]bool)
	err := fs.WalkDir(migrations, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".down.sql") {
			files[path] = true
		}
		return nil
	})
	return files, err
}
