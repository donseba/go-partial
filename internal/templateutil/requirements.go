package templateutil

import (
	"fmt"
	"html/template"
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
	typedRootPattern       = regexp.MustCompile(`(?m)^\s*@([A-Za-z_][A-Za-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s+([^\s]+)`)
	templateCommentPattern = regexp.MustCompile(`(?s)\{\{/\*(.*?)\*/\}\}`)
)

type RootContract struct {
	Annotation string
	Type       string
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

func ReferencedTemplates(name, src string) ([]string, error) {
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

func DefinedTemplates(name, src string) ([]string, error) {
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

func NormalizeContractType(typeName string) string {
	lastSlash := strings.LastIndex(typeName, "/")
	lastDot := strings.LastIndex(typeName, ".")
	if lastSlash > lastDot {
		return typeName[:lastSlash] + "." + typeName[lastSlash+1:]
	}
	return typeName
}

func PathBase(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' || name[i] == '\\' {
			return name[i+1:]
		}
	}
	return name
}

func PathAliases(name string) []string {
	trimmed := strings.TrimLeft(name, `/\`)
	if trimmed == "" {
		return nil
	}
	aliases := []string{trimmed}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return aliases
	}
	return append(aliases, "/"+trimmed)
}

func AddPathAliases(tmpl *template.Template, names []string) error {
	if tmpl == nil {
		return nil
	}
	for _, name := range names {
		base := PathBase(name)
		if name == "" || name == base || tmpl.Lookup(base) == nil {
			continue
		}
		for _, alias := range PathAliases(name) {
			if tmpl.Lookup(alias) != nil {
				continue
			}
			if _, err := tmpl.New(alias).Parse(fmt.Sprintf(`{{ template %q . }}`, base)); err != nil {
				return err
			}
		}
	}
	return nil
}

func reservedContractAnnotation(name string) bool {
	switch name {
	case "dot", "func", "gen":
		return true
	default:
		return false
	}
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
		if x != nil {
			walk(x.Pipe, found)
		}
	case *parse.IfNode:
		if x != nil {
			walk(x.Pipe, found)
			walk(x.List, found)
			walk(x.ElseList, found)
		}
	case *parse.RangeNode:
		if x != nil {
			walk(x.Pipe, found)
			walk(x.List, found)
			walk(x.ElseList, found)
		}
	case *parse.WithNode:
		if x != nil {
			walk(x.Pipe, found)
			walk(x.List, found)
			walk(x.ElseList, found)
		}
	case *parse.TemplateNode:
		if x != nil {
			walk(x.Pipe, found)
		}
	case *parse.PipeNode:
		if x != nil {
			for _, cmd := range x.Cmds {
				walk(cmd, found)
			}
		}
	case *parse.CommandNode:
		if x != nil {
			for _, arg := range x.Args {
				walk(arg, found)
			}
		}
	case *parse.ChainNode:
		if x != nil {
			walk(x.Node, found)
		}
	case *parse.IdentifierNode:
		if x != nil {
			found[x.Ident] = true
		}
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
