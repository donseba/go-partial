package partial

import "sort"

type Node struct {
	ID    string
	Depth int
	Nodes []*Node
}

// Tree returns the tree of partials.
func Tree(p *Partial) *Node {
	return tree(p, 0)
}

func tree(p *Partial, depth int) *Node {
	var out = &Node{ID: p.id, Depth: depth}

	childIDs := make([]string, 0, len(p.children))
	for id := range p.children {
		childIDs = append(childIDs, id)
	}
	sort.Strings(childIDs)

	for _, id := range childIDs {
		out.Nodes = append(out.Nodes, tree(p.children[id], depth+1))
	}

	return out
}
