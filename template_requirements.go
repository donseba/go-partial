package partial

import (
	"io/fs"
	"sort"
	"text/template/parse"
)

var builtinFuncs = map[string]bool{
	"and": true, "call": true, "html": true, "index": true,
	"slice": true, "js": true, "len": true, "not": true,
	"or": true, "print": true, "printf": true, "println": true,
	"urlquery": true,

	"eq": true, "ge": true, "gt": true,
	"le": true, "lt": true, "ne": true,
}

func RequiredFuncs(name, src string) ([]string, error) {
	tree := parse.New(name)
	tree.Mode = parse.SkipFuncCheck

	treeSet := map[string]*parse.Tree{}
	parsed, err := tree.Parse(src, "{{", "}}", treeSet)
	if err != nil {
		return nil, err
	}

	found := map[string]bool{}
	walk(parsed.Root, found)
	for _, parsedTree := range treeSet {
		walk(parsedTree.Root, found)
	}

	var funcs []string
	for fn := range found {
		if !builtinFuncs[fn] {
			funcs = append(funcs, fn)
		}
	}

	sort.Strings(funcs)
	return funcs, nil
}

func requiredFuncsFromFS(fsys fs.FS, names []string) (map[string]struct{}, error) {
	found := make(map[string]struct{})
	for _, name := range names {
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		funcs, err := RequiredFuncs(name, string(content))
		if err != nil {
			return nil, err
		}
		for _, fn := range funcs {
			found[fn] = struct{}{}
		}
	}
	return found, nil
}

func walk(n parse.Node, found map[string]bool) {
	if n == nil {
		return
	}

	switch x := n.(type) {
	case *parse.ListNode:
		if x == nil {
			return
		}
		for _, node := range x.Nodes {
			walk(node, found)
		}

	case *parse.ActionNode:
		if x == nil {
			return
		}
		walk(x.Pipe, found)

	case *parse.IfNode:
		if x == nil {
			return
		}
		walk(x.Pipe, found)
		walk(x.List, found)
		walk(x.ElseList, found)

	case *parse.RangeNode:
		if x == nil {
			return
		}
		walk(x.Pipe, found)
		walk(x.List, found)
		walk(x.ElseList, found)

	case *parse.WithNode:
		if x == nil {
			return
		}
		walk(x.Pipe, found)
		walk(x.List, found)
		walk(x.ElseList, found)

	case *parse.TemplateNode:
		if x == nil {
			return
		}
		walk(x.Pipe, found)

	case *parse.PipeNode:
		if x == nil {
			return
		}
		for _, cmd := range x.Cmds {
			walk(cmd, found)
		}

	case *parse.CommandNode:
		if x == nil {
			return
		}
		for _, arg := range x.Args {
			walk(arg, found)
		}

	case *parse.ChainNode:
		if x == nil {
			return
		}
		walk(x.Node, found)

	case *parse.IdentifierNode:
		if x == nil {
			return
		}
		found[x.Ident] = true
	}
}
