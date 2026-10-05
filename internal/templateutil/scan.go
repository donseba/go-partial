package templateutil

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"text/template/parse"
)

// FileScan is what one template file declares and uses: the templates it
// defines and calls, the functions it calls, and its typed root declarations.
type FileScan struct {
	Defined    []string
	Referenced []string
	Required   []string
	Contracts  []ContractDeclaration
	// ParseErr is set when the file does not parse; Defined, Referenced and
	// Required are empty then.
	ParseErr error
}

// ContractDeclaration is one typed root declaration, such as
// "@model Site github.com/acme/site.Site".
type ContractDeclaration struct {
	Annotation string
	Name       string
	Type       string
}

// ScanTemplate reads what the template source declares and uses, parsing it
// once.
func ScanTemplate(name, src string) *FileScan {
	scan := &FileScan{Contracts: contractDeclarations(src)}

	tree := parse.New(name)
	tree.Mode = parse.SkipFuncCheck

	treeSet := map[string]*parse.Tree{}
	parsed, err := tree.Parse(src, "{{", "}}", treeSet)
	if err != nil {
		scan.ParseErr = err
		return scan
	}

	defined := map[string]bool{parsed.Name: true}
	refs := map[string]bool{}
	funcs := map[string]bool{}
	walk(parsed.Root, funcs)
	walkTemplateRefs(parsed.Root, refs)
	for definedName, definedTree := range treeSet {
		defined[definedName] = true
		walk(definedTree.Root, funcs)
		walkTemplateRefs(definedTree.Root, refs)
	}
	for fn := range funcs {
		if builtinFuncs[fn] {
			delete(funcs, fn)
		}
	}

	scan.Defined = sortedNames(defined)
	scan.Referenced = sortedNames(refs)
	scan.Required = sortedNames(funcs)
	return scan
}

func contractDeclarations(src string) []ContractDeclaration {
	var declarations []ContractDeclaration
	for _, match := range typedRootPattern.FindAllStringSubmatch(contractScanText(src), -1) {
		annotation := strings.TrimSpace(match[1])
		if reservedContractAnnotation(annotation) {
			continue
		}
		declarations = append(declarations, ContractDeclaration{
			Annotation: annotation,
			Name:       strings.TrimSpace(match[2]),
			Type:       NormalizeContractType(strings.TrimSpace(match[3])),
		})
	}
	return declarations
}

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Scanner reads template files from one file system. With a key, which
// identifies the file system, each file is scanned once and kept in the
// store, for template trees whose files do not change.
type Scanner struct {
	fsys  fs.FS
	key   string
	store *Store
}

// Scanner returns a scanner for fsys. An empty key scans files on every call.
func (store *Store) Scanner(fsys fs.FS, key string) Scanner {
	return Scanner{fsys: fsys, key: key, store: store}
}

// File returns the scan of the template file name.
func (s Scanner) File(name string) (*FileScan, error) {
	cacheKey := s.key + "\x00" + name
	if s.store != nil && s.key != "" {
		if cached, ok := s.store.scans.Load(cacheKey); ok {
			return cached.(*FileScan), nil
		}
	}

	content, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		return nil, err
	}
	scan := ScanTemplate(name, string(content))

	if s.store != nil && s.key != "" {
		cached, _ := s.store.scans.LoadOrStore(cacheKey, scan)
		return cached.(*FileScan), nil
	}
	return scan, nil
}

// IsFile reports whether name is a file. With a key, files found are
// remembered.
func (s Scanner) IsFile(name string) bool {
	cacheKey := s.key + "\x00" + name
	if s.store != nil && s.key != "" {
		if _, ok := s.store.files.Load(cacheKey); ok {
			return true
		}
	}

	info, err := fs.Stat(s.fsys, name)
	if err != nil || info.IsDir() {
		return false
	}

	if s.store != nil && s.key != "" {
		s.store.files.Store(cacheKey, struct{}{})
	}
	return true
}

// RequiredFuncs returns the functions the files call.
func (s Scanner) RequiredFuncs(names []string) (map[string]struct{}, error) {
	found := make(map[string]struct{})
	for _, name := range names {
		scan, err := s.File(name)
		if err != nil {
			return nil, err
		}
		if scan.ParseErr != nil {
			return nil, scan.ParseErr
		}
		for _, fn := range scan.Required {
			found[fn] = struct{}{}
		}
	}
	return found, nil
}

// ReferencedTemplates returns the templates the files call. Files that cannot
// be read or parsed are skipped; parsing them for rendering reports why.
func (s Scanner) ReferencedTemplates(names []string) map[string]struct{} {
	found := make(map[string]struct{})
	for _, name := range names {
		scan, err := s.File(name)
		if err != nil || scan.ParseErr != nil {
			continue
		}
		for _, ref := range scan.Referenced {
			found[ref] = struct{}{}
		}
	}
	return found
}

// DefinedTemplates returns the names the files can be called by: their paths,
// base names and aliases, and the templates they define.
func (s Scanner) DefinedTemplates(names []string) map[string]struct{} {
	found := make(map[string]struct{})
	for _, name := range names {
		found[name] = struct{}{}
		found[PathBase(name)] = struct{}{}
		for _, alias := range PathAliases(name) {
			found[alias] = struct{}{}
		}

		scan, err := s.File(name)
		if err != nil || scan.ParseErr != nil {
			continue
		}
		for _, definedName := range scan.Defined {
			found[definedName] = struct{}{}
		}
	}
	return found
}

// RootContracts returns the typed root declarations of the files by root
// name. A name declared with two types is an error.
func (s Scanner) RootContracts(names []string) (map[string]RootContract, error) {
	contracts := make(map[string]RootContract)
	for _, name := range names {
		scan, err := s.File(name)
		if err != nil {
			return nil, err
		}
		if err := addContracts(contracts, scan.Contracts); err != nil {
			return nil, err
		}
	}
	return contracts, nil
}

func addContracts(contracts map[string]RootContract, declarations []ContractDeclaration) error {
	for _, declaration := range declarations {
		if previous, exists := contracts[declaration.Name]; exists && previous.Type != declaration.Type {
			return fmt.Errorf("@%s %s is declared as both %s and %s", declaration.Annotation, declaration.Name, previous.Type, declaration.Type)
		}
		contracts[declaration.Name] = RootContract{
			Annotation: declaration.Annotation,
			Type:       declaration.Type,
		}
	}
	return nil
}
