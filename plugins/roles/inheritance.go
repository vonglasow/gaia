package roles

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveInheritance resolves extends for all roles, detects cycles and missing parents.
func ResolveInheritance(roles []Role) (map[string]ResolvedRole, error) {
	roleMap := make(map[string]Role, len(roles))
	for _, role := range roles {
		roleMap[role.Name] = role
	}
	if len(roleMap) == 0 {
		return map[string]ResolvedRole{}, nil
	}

	order, err := topologicalOrder(roleMap)
	if err != nil {
		return nil, err
	}
	resolved := make(map[string]ResolvedRole, len(roleMap))
	for _, name := range order {
		role := roleMap[name]
		res, err := resolveOne(role, roleMap, resolved)
		if err != nil {
			return nil, err
		}
		resolved[name] = res
	}
	return resolved, nil
}

func resolveOne(role Role, roleMap map[string]Role, resolved map[string]ResolvedRole) (ResolvedRole, error) {
	out := ResolvedRole{
		Name:         role.Name,
		Description:  role.Description,
		Priority:     role.Priority,
		Exclusive:    role.Exclusive,
		SystemPrompt: strings.TrimSpace(role.SystemPrompt),
		Providers:    cloneProviders(role.Providers),
		Models:       cloneModels(role.Models),
	}
	for _, parentName := range role.Extends {
		parentName = strings.TrimSpace(parentName)
		if parentName == "" {
			continue
		}
		parent, ok := resolved[parentName]
		if !ok {
			if _, exists := roleMap[parentName]; exists {
				return ResolvedRole{}, fmt.Errorf("role %q: parent %q not resolved", role.Name, parentName)
			}
			return ResolvedRole{}, fmt.Errorf("role %q extends unknown role %q", role.Name, parentName)
		}
		if out.SystemPrompt == "" {
			out.SystemPrompt = parent.SystemPrompt
		} else if parent.SystemPrompt != "" {
			out.SystemPrompt = parent.SystemPrompt + "\n\n" + out.SystemPrompt
		}
		if out.Description == "" {
			out.Description = parent.Description
		}
		if out.Priority == 0 {
			out.Priority = parent.Priority
		}
		out.Providers = mergeProviders(parent.Providers, out.Providers)
		out.Models = mergeModels(parent.Models, out.Models)
	}
	return out, nil
}

func cloneProviders(in map[string]ProviderOverride) map[string]ProviderOverride {
	if len(in) == 0 {
		return map[string]ProviderOverride{}
	}
	out := make(map[string]ProviderOverride, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneModels(in map[string]ModelOverride) map[string]ModelOverride {
	if len(in) == 0 {
		return map[string]ModelOverride{}
	}
	out := make(map[string]ModelOverride, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeProviders(parent, child map[string]ProviderOverride) map[string]ProviderOverride {
	out := cloneProviders(parent)
	for k, v := range child {
		out[k] = v
	}
	return out
}

func mergeModels(parent, child map[string]ModelOverride) map[string]ModelOverride {
	out := cloneModels(parent)
	for k, v := range child {
		out[k] = v
	}
	return out
}

func topologicalOrder(roles map[string]Role) ([]string, error) {
	inDegree := make(map[string]int, len(roles))
	graph := make(map[string][]string, len(roles))
	for name := range roles {
		inDegree[name] = 0
	}
	for name, role := range roles {
		for _, parent := range role.Extends {
			parent = strings.TrimSpace(parent)
			if parent == "" {
				continue
			}
			if _, ok := roles[parent]; !ok {
				return nil, fmt.Errorf("role %q extends unknown role %q", name, parent)
			}
			graph[parent] = append(graph[parent], name)
			inDegree[name]++
		}
	}

	queue := make([]string, 0)
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)

	order := make([]string, 0, len(roles))
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		for _, child := range graph[n] {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
		sort.Strings(queue)
	}
	if len(order) != len(roles) {
		cycle := make([]string, 0)
		for name, deg := range inDegree {
			if deg > 0 {
				cycle = append(cycle, name)
			}
		}
		sort.Strings(cycle)
		return nil, fmt.Errorf("circular role inheritance involving: %s", strings.Join(cycle, ", "))
	}
	return order, nil
}
