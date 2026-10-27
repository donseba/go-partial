package partial

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
	"text/template/parse"

	"github.com/donseba/go-partial/internal/templateutil"
)

// TemplateProblem is a problem CheckTemplates finds in a template file: one
// a render of the file would fail with.
type TemplateProblem struct {
	// File is the template's path in the partial's file system, such as
	// "regions/menu.gohtml".
	File string
	// Layer is, for a file system made with Overlay, the index of the layer
	// the file comes from, the first layer 0, as the overlay serves it;
	// 0 for any other file system.
	Layer int
	// Line and Column are where the problem is in the file, counted from 1;
	// Column is 0 when the parser does not say.
	Line   int
	Column int
	// Func is the function the template calls that the partial does not
	// have, such as "newsletterForm"; empty for other problems.
	Func string
	// Template is the template a {{template}} names that no file defines;
	// empty for other problems.
	Template string
	// Suggestion is the defined function or template whose name is closest
	// to Func or Template, such as "newsletterForm" for "newsleterForm";
	// empty when none is close.
	Suggestion string
	// Message says what is wrong, without the position, such as
	// `function "newsletterForm" not defined`.
	Message string
}

// Position is the problem's position: the file, its line and, when known,
// its column, such as "layout.gohtml:69:5".
func (p TemplateProblem) Position() string {
	position := p.File + ":" + strconv.Itoa(p.Line)
	if p.Column > 0 {
		position += ":" + strconv.Itoa(p.Column)
	}

	return position
}

// Error is the position and the message, with the suggestion when there is
// one: `layout.gohtml:69:5: function "newsleterForm" not defined; did you
// mean "newsletterForm"?`.
func (p TemplateProblem) Error() string {
	message := p.Position() + ": " + p.Message
	if p.Suggestion != "" {
		message += fmt.Sprintf("; did you mean %q?", p.Suggestion)
	}

	return message
}

// CheckTemplates parses every template file of the partial's file system,
// in every folder, with the functions its renders have: those registered
// with SetFunc on the partial and its ancestors, go-partial's own helpers,
// those the render stages of the partial and its ancestors give (see Use)
// and the typed roots the files declare. It reports every problem, not
// only the first: a file that does not parse, every call of a function the
// partial does not have, and every {{template}} that names a template no
// file defines, each with its file, line and column, and for an Overlay
// the layer the file comes from. Run it when the application starts, so a
// template that would fail its renders is known before the first request:
//
//	problems, err := root.CheckTemplates()
//	for _, problem := range problems {
//		log.Print(problem)
//	}
//
// patterns choose the files by their base name, as path.Match does, such
// as "*.tmpl"; without, every file named *.gohtml is a template. Templates
// render with the templates of other files, such as those of SetIncludes,
// so a template named in any checked file counts as defined. The error is
// that of reading the file system; the problems are not errors.
func (p *Partial) CheckTemplates(patterns ...string) ([]TemplateProblem, error) {
	if p == nil {
		return nil, errors.New("partial is not initialized")
	}

	if len(patterns) == 0 {
		patterns = []string{"*.gohtml"}
	}

	fsys := p.getFS()

	files, err := checkedFiles(fsys, patterns)
	if err != nil {
		return nil, err
	}

	sources, err := readSources(fsys, files)
	if err != nil {
		return nil, err
	}

	funcs := p.checkFuncs(sources)
	trees, defined, problems := parseFilesForCheck(files, sources)

	for _, name := range files {
		set, ok := trees[name]
		if !ok {
			continue
		}

		problems = append(problems, checkTrees(name, sources[name], set, funcs, defined)...)
	}

	layerOf := overlayLayerOf(fsys)
	for i := range problems {
		problems[i].Layer = layerOf(problems[i].File)
	}

	slices.SortStableFunc(problems, compareProblems)

	return problems, nil
}

// readSources reads the files of fsys, by name.
func readSources(fsys fs.FS, files []string) (map[string]string, error) {
	sources := make(map[string]string, len(files))
	for _, name := range files {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}

		sources[name] = string(data)
	}

	return sources, nil
}

// parseFilesForCheck parses the files without looking up their functions:
// trees holds the templates of each file that parses, by file, and defined
// every template name a render of them knows; problems are the files that
// do not parse.
func parseFilesForCheck(
	files []string,
	sources map[string]string,
) (trees map[string]map[string]*parse.Tree, defined map[string]bool, problems []TemplateProblem) {
	trees = map[string]map[string]*parse.Tree{}
	defined = map[string]bool{}

	for _, name := range files {
		set, problem := parseForCheck(name, sources[name])
		if problem != nil {
			problems = append(problems, *problem)

			continue
		}

		trees[name] = set
		for _, alias := range fileTemplateNames(name) {
			defined[alias] = true
		}

		for defineName := range set {
			defined[defineName] = true
		}
	}

	return trees, defined, problems
}

// compareProblems orders problems by file, line and column.
func compareProblems(a, b TemplateProblem) int {
	if a.File != b.File {
		return strings.Compare(a.File, b.File)
	}

	if a.Line != b.Line {
		return a.Line - b.Line
	}

	return a.Column - b.Column
}

// checkedFiles are the files of fsys whose base name matches one of
// patterns, sorted.
func checkedFiles(fsys fs.FS, patterns []string) ([]string, error) {
	var files []string

	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		for _, pattern := range patterns {
			matched, err := path.Match(pattern, path.Base(name))
			if err != nil {
				return fmt.Errorf("template pattern %q: %w", pattern, err)
			}

			if matched {
				files = append(files, name)

				break
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.Sort(files)

	return files, nil
}

// checkFuncs are the names of the functions the partial's templates can
// call: its registered functions and its ancestors', go-partial's helpers,
// those of the render stages, the typed roots the sources declare and
// those of text/template itself.
func (p *Partial) checkFuncs(sources map[string]string) map[string]bool {
	funcs := templateutil.MergeFuncMaps(p.getStaticFuncMap(), placeholderRequestFuncMap())
	addStageStandIns(funcs, p.getRenderStages())

	names := make(map[string]bool, len(funcs))
	for name := range funcs {
		names[name] = true
	}

	for _, src := range sources {
		declarations, _ := templateutil.Declarations(src)
		for _, declaration := range declarations {
			names[declaration.Name] = true
		}
	}

	for _, name := range builtinFuncNames {
		names[name] = true
	}

	return names
}

// builtinFuncNames are the functions of text/template itself.
var builtinFuncNames = []string{
	"and", "call", "eq", "ge", "gt", "html", "index", "js", "le", "len", "lt", "ne",
	"not", "or", "print", "printf", "println", "slice", "urlquery",
}

// parseForCheck parses the file name without looking up its functions,
// and returns its templates by name, or the problem when it does not
// parse.
func parseForCheck(name, src string) (map[string]*parse.Tree, *TemplateProblem) {
	tree := parse.New(name)
	tree.Mode = parse.SkipFuncCheck

	set := map[string]*parse.Tree{}

	_, err := tree.Parse(src, "{{", "}}", set)
	if err == nil {
		return set, nil
	}

	problem := TemplateProblem{File: name, Line: 1, Message: err.Error()}

	// The parser's errors read "template: <name>:<line>: <message>".
	rest, ok := strings.CutPrefix(err.Error(), "template: "+name+":")
	if ok {
		line, message, found := strings.Cut(rest, ": ")
		if number, err := strconv.Atoi(line); found && err == nil {
			problem.Line = number
			problem.Message = message
		}
	}

	if strings.HasPrefix(problem.Message, "unexpected EOF") {
		problem.Message += ": an {{if}}, {{range}}, {{with}}, {{block}} or {{define}} lacks its {{end}}"
	}

	return set, &problem
}

// fileTemplateNames are the names a file's template has in a render: its
// base name, as template.ParseFS names it, and its path with and without a
// leading slash (see templateutil.AddPathAliases).
func fileTemplateNames(name string) []string {
	return append([]string{templateutil.PathBase(name)}, templateutil.PathAliases(name)...)
}

// checkTrees reports the calls of unknown functions and templates in the
// templates of the file name.
func checkTrees(name, src string, set map[string]*parse.Tree, funcs, defined map[string]bool) []TemplateProblem {
	var problems []TemplateProblem

	report := func(pos parse.Pos, problem TemplateProblem) {
		problem.File = name
		problem.Line, problem.Column = lineAndColumn(src, int(pos))
		problems = append(problems, problem)
	}

	visit := func(node parse.Node) {
		switch node := node.(type) {
		case *parse.IdentifierNode:
			if funcs[node.Ident] {
				return
			}

			report(node.Pos, TemplateProblem{
				Func:       node.Ident,
				Suggestion: closestName(node.Ident, funcs),
				Message:    fmt.Sprintf("function %q not defined", node.Ident),
			})
		case *parse.TemplateNode:
			if defined[node.Name] {
				return
			}

			report(node.Pos, TemplateProblem{
				Template:   node.Name,
				Suggestion: closestName(node.Name, defined),
				Message:    fmt.Sprintf("no such template %q", node.Name),
			})
		}
	}

	// A file's own template and those it defines share their source; each
	// is walked once, in the order of the source.
	trees := make([]*parse.Tree, 0, len(set))
	for _, tree := range set {
		if tree != nil && tree.Root != nil {
			trees = append(trees, tree)
		}
	}

	slices.SortFunc(trees, func(a, b *parse.Tree) int {
		return int(a.Root.Pos) - int(b.Root.Pos)
	})

	for _, tree := range trees {
		walkNodes(tree.Root, visit)
	}

	return problems
}

// walkNodes calls visit for node and every node below it.
func walkNodes(node parse.Node, visit func(parse.Node)) {
	if node == nil {
		return
	}

	visit(node)

	switch node := node.(type) {
	case *parse.ListNode:
		if node == nil {
			return
		}

		for _, child := range node.Nodes {
			walkNodes(child, visit)
		}
	case *parse.ActionNode:
		walkPipe(node.Pipe, visit)
	case *parse.IfNode:
		walkBranch(&node.BranchNode, visit)
	case *parse.RangeNode:
		walkBranch(&node.BranchNode, visit)
	case *parse.WithNode:
		walkBranch(&node.BranchNode, visit)
	case *parse.TemplateNode:
		walkPipe(node.Pipe, visit)
	case *parse.PipeNode:
		walkPipe(node, visit)
	case *parse.CommandNode:
		for _, arg := range node.Args {
			walkNodes(arg, visit)
		}
	case *parse.ChainNode:
		walkNodes(node.Node, visit)
	}
}

func walkBranch(branch *parse.BranchNode, visit func(parse.Node)) {
	walkPipe(branch.Pipe, visit)
	walkNodes(branch.List, visit)

	if branch.ElseList != nil {
		walkNodes(branch.ElseList, visit)
	}
}

func walkPipe(pipe *parse.PipeNode, visit func(parse.Node)) {
	if pipe == nil {
		return
	}

	for _, command := range pipe.Cmds {
		walkNodes(command, visit)
	}
}

// lineAndColumn are the line and column of the byte offset pos in src,
// counted from 1.
func lineAndColumn(src string, pos int) (int, int) {
	pos = min(max(pos, 0), len(src))
	before := src[:pos]
	line := 1 + strings.Count(before, "\n")
	column := pos - strings.LastIndex(before, "\n")

	return line, column
}

// closestName is the name in names closest to name by edit distance, when
// it is close enough to be a typing mistake: at most two edits, or a third
// of name's length for a longer one. Of equally close names the first in
// sorted order is chosen.
func closestName(name string, names map[string]bool) string {
	best := ""
	bestDistance := max(2, len(name)/3) + 1

	for _, candidate := range slices.Sorted(maps.Keys(names)) {
		if candidate == name {
			continue
		}

		distance := editDistance(strings.ToLower(name), strings.ToLower(candidate))
		if distance < bestDistance {
			best = candidate
			bestDistance = distance
		}
	}

	return best
}

// editDistance is the Levenshtein distance between a and b, in bytes.
func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)

	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current[0] = i

		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}

		previous, current = current, previous
	}

	return previous[len(b)]
}

// overlayLayerOf returns, for an Overlay, the index of the layer that
// serves a file; for any other file system, 0.
func overlayLayerOf(fsys fs.FS) func(name string) int {
	overlay, ok := fsys.(overlayFS)
	if !ok {
		return func(string) int {
			return 0
		}
	}

	return func(name string) int {
		for i, layer := range overlay.layers {
			if _, err := fs.Stat(layer, name); err == nil {
				return i
			}
		}

		return 0
	}
}
