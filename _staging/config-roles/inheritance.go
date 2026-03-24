package roles

import (
	"fmt"
	"sort"
	"strings"
)

const systemPromptSeparator = "\n\n---\n\n"

// ResolveInheritance resolves extends for all roles, detects cycles and missing parents,
// and returns a map of name -> ResolvedRole with merged fields. Resolution order is deterministic.
func ResolveInheritance(roles map[string]Role) (map[string]ResolvedRole, error) {
	if len(roles) == 0 {
		return nil, nil
	}
	// Topological order: resolve parents before children
	order, err := topologicalOrder(roles)
	if err != nil {
		return nil, err
	}
	resolved := make(map[string]ResolvedRole, len(roles))
	for _, name := range order {
		r, ok := roles[name]
		if !ok {
			continue
		}
		res, err := resolveOne(name, r, roles, resolved)
		if err != nil {
			return nil, err
		}
		resolved[name] = res
	}
	return resolved, nil
}

// topologicalOrder returns role names in an order such that every parent is before any child.
// Detects cycles and missing parents.
func topologicalOrder(roles map[string]Role) ([]string, error) {
	// Collect all names and validate parents exist
	names := make([]string, 0, len(roles))
	for name := range roles {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		r := roles[name]
		for _, p := range r.Extends {
			if _, ok := roles[p]; !ok {
				return nil, fmt.Errorf("role %q extends unknown role %q", name, p)
			}
		}
	}

	// Kahn-style topological sort: in-degree = number of parents (Extends); process roots first
	inDegree := make(map[string]int)
	for name, r := range roles {
		inDegree[name] = len(r.Extends)
	}

	var order []string
	queue := make([]string, 0)
	for name, d := range inDegree {
		if d == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)

	seen := make(map[string]bool)
	for len(queue) > 0 {
		// Deterministic: take smallest name
		sort.Strings(queue)
		name := queue[0]
		queue = queue[1:]
		if seen[name] {
			continue
		}
		seen[name] = true
		order = append(order, name)

		// Find all roles that extend this one (depend on it) and reduce their in-degree
		for other, r := range roles {
			if seen[other] {
				continue
			}
			for _, p := range r.Extends {
				if p == name {
					inDegree[other]--
					if inDegree[other] == 0 {
						queue = append(queue, other)
					}
					break
				}
			}
		}
	}

	if len(order) != len(roles) {
		// Cycle: some nodes never got inDegree 0
		var cycle []string
		for name := range roles {
			if !seen[name] {
				cycle = append(cycle, name)
			}
		}
		sort.Strings(cycle)
		return nil, fmt.Errorf("circular role inheritance involving: %s", strings.Join(cycle, ", "))
	}

	return order, nil
}

func resolveOne(name string, r Role, roles map[string]Role, resolved map[string]ResolvedRole) (ResolvedRole, error) {
	out := ResolvedRole{
		Name:        name,
		Description: r.Description,
		Enabled:     true,
		Priority:    0,
		Weight:      1.0,
		Mode:        "",
		Exclusive:   false,
		Matching:    MatchingConfig{},
	}
	if r.Enabled != nil {
		out.Enabled = *r.Enabled
	}

	// Merge parents first (order: first parent, then next, ...)
	for _, pName := range r.Extends {
		p, ok := resolved[pName]
		if !ok {
			return ResolvedRole{}, fmt.Errorf("role %q: parent %q not resolved", name, pName)
		}
		mergeParent(&out, p)
	}

	// Apply child (self) on top: overrides and appends
	applyChild(&out, r)
	return out, nil
}

func mergeParent(out *ResolvedRole, p ResolvedRole) {
	// SystemPrompt: parents in order (first parent, then next), then child; so append parent to out
	if p.SystemPrompt != "" {
		if out.SystemPrompt != "" {
			out.SystemPrompt = out.SystemPrompt + systemPromptSeparator + p.SystemPrompt
		} else {
			out.SystemPrompt = p.SystemPrompt
		}
	}

	// Matching: signals and negative signals append
	out.Matching.Signals = append(out.Matching.Signals, p.Matching.Signals...)
	out.Matching.NegativeSignals = append(out.Matching.NegativeSignals, p.Matching.NegativeSignals...)
	// Imports: merge and deduplicate
	importSet := make(map[string]bool)
	for _, imp := range out.Matching.Imports {
		importSet[imp] = true
	}
	for _, imp := range p.Matching.Imports {
		importSet[imp] = true
	}
	out.Matching.Imports = nil
	for imp := range importSet {
		out.Matching.Imports = append(out.Matching.Imports, imp)
	}
	sort.Strings(out.Matching.Imports)
	// Threshold: child overrides if defined; else keep (we set in applyChild from r)
	if p.Matching.Threshold > 0 && out.Matching.Threshold == 0 {
		out.Matching.Threshold = p.Matching.Threshold
	}

	// Priority: inherit highest parent if child not set (applyChild will set from r)
	if p.Priority > out.Priority {
		out.Priority = p.Priority
	}
	// Weight: inherit from parent if child not set
	if out.Weight == 0 {
		out.Weight = p.Weight
	}
	if out.Mode == "" {
		out.Mode = p.Mode
	}
	// Exclusive: if parent true -> child inherits true; child may override to false in applyChild
	if p.Exclusive {
		out.Exclusive = true
	}
}

func applyChild(out *ResolvedRole, r Role) {
	// SystemPrompt: child appended last
	if r.SystemPrompt != "" {
		if out.SystemPrompt != "" {
			out.SystemPrompt = out.SystemPrompt + systemPromptSeparator + r.SystemPrompt
		} else {
			out.SystemPrompt = r.SystemPrompt
		}
	}

	// Matching: child signals/negative_signals/imports appended
	out.Matching.Signals = append(out.Matching.Signals, r.Matching.Signals...)
	out.Matching.NegativeSignals = append(out.Matching.NegativeSignals, r.Matching.NegativeSignals...)
	importSet := make(map[string]bool)
	for _, imp := range out.Matching.Imports {
		importSet[imp] = true
	}
	for _, imp := range r.Matching.Imports {
		importSet[imp] = true
	}
	out.Matching.Imports = nil
	for imp := range importSet {
		out.Matching.Imports = append(out.Matching.Imports, imp)
	}
	sort.Strings(out.Matching.Imports)

	// Threshold: child overrides if defined
	if r.Matching.Threshold > 0 {
		out.Matching.Threshold = r.Matching.Threshold
	}
	// Priority, Weight, Mode, Exclusive: child overrides if defined
	if r.Priority != nil {
		out.Priority = *r.Priority
	}
	if r.Weight != nil {
		out.Weight = *r.Weight
	}
	if r.Mode != nil {
		out.Mode = *r.Mode
	}
	if r.Exclusive != nil {
		out.Exclusive = *r.Exclusive
	}
}
