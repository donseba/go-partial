package partial

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

	for _, child := range p.children {
		out.Nodes = append(out.Nodes, tree(child, depth+1))
	}

	return out
}
