package partial

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
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

var (
	modelContractPattern   = regexp.MustCompile(`(?m)@model\s+([A-Za-z_][A-Za-z0-9_]*)\s+([^\s]+)`)
	templateCommentPattern = regexp.MustCompile(`(?s)\{\{/\*(.*?)\*/\}\}`)
)

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

func referencedTemplates(name, src string) ([]string, error) {
	tree := parse.New(name)
	tree.Mode = parse.SkipFuncCheck

	treeSet := map[string]*parse.Tree{}
	parsed, err := tree.Parse(src, "{{", "}}", treeSet)
	if err != nil {
		return nil, err
	}

	found := map[string]bool{}
	walkTemplateRefs(parsed.Root, found)
	for _, parsedTree := range treeSet {
		walkTemplateRefs(parsedTree.Root, found)
	}

	var names []string
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func definedTemplates(name, src string) ([]string, error) {
	tree := parse.New(name)
	tree.Mode = parse.SkipFuncCheck

	treeSet := map[string]*parse.Tree{}
	parsed, err := tree.Parse(src, "{{", "}}", treeSet)
	if err != nil {
		return nil, err
	}

	found := map[string]bool{parsed.Name: true}
	for name := range treeSet {
		found[name] = true
	}

	var names []string
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
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

func referencedTemplatesFromFS(fsys fs.FS, names []string) map[string]struct{} {
	found := make(map[string]struct{})
	for _, name := range names {
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			continue
		}
		refs, err := referencedTemplates(name, string(content))
		if err != nil {
			continue
		}
		for _, ref := range refs {
			found[ref] = struct{}{}
		}
	}
	return found
}

func definedTemplatesFromFS(fsys fs.FS, names []string) map[string]struct{} {
	found := make(map[string]struct{})
	for _, name := range names {
		found[name] = struct{}{}
		found[pathBase(name)] = struct{}{}
		for _, alias := range templatePathAliases(name) {
			found[alias] = struct{}{}
		}

		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			continue
		}
		defined, err := definedTemplates(name, string(content))
		if err != nil {
			continue
		}
		for _, definedName := range defined {
			found[definedName] = struct{}{}
		}
	}
	return found
}

func modelContractsFromFS(fsys fs.FS, names []string) (map[string]string, error) {
	contracts := make(map[string]string)
	for _, name := range names {
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		for _, match := range modelContractPattern.FindAllStringSubmatch(contractScanText(string(content)), -1) {
			modelName := strings.TrimSpace(match[1])
			typeName := normalizeContractType(strings.TrimSpace(match[2]))
			if previous, exists := contracts[modelName]; exists && previous != typeName {
				return nil, fmt.Errorf("@model %s is declared as both %s and %s", modelName, previous, typeName)
			}
			contracts[modelName] = typeName
		}
	}
	return contracts, nil
}

func contractScanText(src string) string {
	matches := templateCommentPattern.FindAllStringSubmatch(src, -1)
	if len(matches) == 0 {
		return src
	}
	var out strings.Builder
	for _, match := range matches {
		body := strings.TrimSpace(match[1])
		if body == "" {
			continue
		}
		out.WriteString(body)
		out.WriteByte('\n')
	}
	if out.Len() == 0 {
		return src
	}
	return out.String()
}

func normalizeContractType(typeName string) string {
	lastSlash := strings.LastIndex(typeName, "/")
	lastDot := strings.LastIndex(typeName, ".")
	if lastSlash > lastDot {
		return typeName[:lastSlash] + "." + typeName[lastSlash+1:]
	}
	return typeName
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

func walkTemplateRefs(n parse.Node, found map[string]bool) {
	if n == nil {
		return
	}

	switch x := n.(type) {
	case *parse.ListNode:
		if x == nil {
			return
		}
		for _, node := range x.Nodes {
			walkTemplateRefs(node, found)
		}

	case *parse.ActionNode:
		if x != nil {
			walkTemplateRefs(x.Pipe, found)
		}

	case *parse.IfNode:
		if x != nil {
			walkTemplateRefs(x.Pipe, found)
			walkTemplateRefs(x.List, found)
			walkTemplateRefs(x.ElseList, found)
		}

	case *parse.RangeNode:
		if x != nil {
			walkTemplateRefs(x.Pipe, found)
			walkTemplateRefs(x.List, found)
			walkTemplateRefs(x.ElseList, found)
		}

	case *parse.WithNode:
		if x != nil {
			walkTemplateRefs(x.Pipe, found)
			walkTemplateRefs(x.List, found)
			walkTemplateRefs(x.ElseList, found)
		}

	case *parse.TemplateNode:
		if x != nil {
			found[x.Name] = true
			walkTemplateRefs(x.Pipe, found)
		}

	case *parse.PipeNode:
		if x != nil {
			for _, cmd := range x.Cmds {
				walkTemplateRefs(cmd, found)
			}
		}

	case *parse.CommandNode:
		if x != nil {
			for _, arg := range x.Args {
				walkTemplateRefs(arg, found)
			}
		}

	case *parse.ChainNode:
		if x != nil {
			walkTemplateRefs(x.Node, found)
		}
	}
}

func pathBase(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' || name[i] == '\\' {
			return name[i+1:]
		}
	}
	return name
}
