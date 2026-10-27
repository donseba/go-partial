package templateutil

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"strings"
	"sync"
	"text/template/parse"
)

// FileScan is what one template file declares and uses: the templates it
// defines and calls, the functions it calls, and its typed root declarations.
type FileScan struct {
	Defined    []string
	Referenced []string
	Required   []string
	Contracts  []ContractDeclaration
	// Dot is the type the file declares for its dot with @dot, or "".
	Dot string
	// ParseErr is set when the file does not parse; Defined, Referenced and
	// Required are empty then.
	ParseErr error

	// roots are the file's typed root declarations by root name, made once;
	// see RootContracts.
	rootsOnce sync.Once
	roots     map[string]RootContract
	rootsErr  error
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
	contracts, dot := Declarations(src)
	scan := &FileScan{Contracts: contracts, Dot: dot}

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

// Declarations returns the typed root declarations of a template source and
// the type of its first @dot declaration, or "".
func Declarations(src string) ([]ContractDeclaration, string) {
	text := contractScanText(src)

	dot := ""
	if match := dotPattern.FindStringSubmatch(text); match != nil {
		dot = NormalizeContractType(strings.TrimSpace(match[1]))
	}

	return contractDeclarations(text), dot
}

func contractDeclarations(text string) []ContractDeclaration {
	var declarations []ContractDeclaration

	for _, match := range typedRootPattern.FindAllStringSubmatch(text, -1) {
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
// identifies the file system in the process, each file is scanned once, and
// whether a file exists and what a pattern matches asked once, and kept in
// the store, for template trees whose files do not change.
type Scanner struct {
	fsys fs.FS
	// scans, files and matches are those the store keeps for the file
	// system's key, nil without one.
	scans   *sync.Map
	files   *sync.Map
	matches *sync.Map
}

// Scanner returns a scanner for fsys. An empty key, or a nil store, scans
// files on every call.
func (store *Store) Scanner(fsys fs.FS, key string) Scanner {
	scanner := Scanner{fsys: fsys}
	if store == nil || key == "" {
		return scanner
	}

	scanner.scans = keyedMap(&store.scans, key)
	scanner.files = keyedMap(&store.files, key)
	scanner.matches = keyedMap(&store.matches, key)

	return scanner
}

// keyedMap returns the map maps keeps for key.
func keyedMap(maps *sync.Map, key string) *sync.Map {
	if found, ok := maps.Load(key); ok {
		return found.(*sync.Map)
	}

	found, _ := maps.LoadOrStore(key, &sync.Map{})

	return found.(*sync.Map)
}

// File returns the scan of the template file name.
func (s Scanner) File(name string) (*FileScan, error) {
	if s.scans != nil {
		if cached, ok := s.scans.Load(name); ok {
			return cached.(*FileScan), nil
		}
	}

	content, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		return nil, err
	}
	scan := ScanTemplate(name, string(content))

	if s.scans != nil {
		cached, _ := s.scans.LoadOrStore(name, scan)
		return cached.(*FileScan), nil
	}
	return scan, nil
}

// IsFile reports whether name is a file. With a key, the answer is
// remembered; see Exists.
func (s Scanner) IsFile(name string) bool {
	exists, _ := s.Exists(name)
	return exists
}

// Exists reports whether name is a file. With a key, the answer is
// remembered, also when the file is missing; errors other than a missing
// file are returned, and asked again next time.
func (s Scanner) Exists(name string) (bool, error) {
	if s.files != nil {
		if found, ok := s.files.Load(name); ok {
			return found.(bool), nil
		}
	}

	info, err := fs.Stat(s.fsys, name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}

	exists := err == nil && !info.IsDir()
	if s.files != nil {
		s.files.Store(name, exists)
	}

	return exists, nil
}

// Glob returns the files pattern matches, as fs.Glob, without the
// directories that match. With a key, the matches are remembered. The slice
// may be shared: do not change it.
func (s Scanner) Glob(pattern string) ([]string, error) {
	if s.matches != nil {
		if found, ok := s.matches.Load(pattern); ok {
			return found.([]string), nil
		}
	}

	matches, err := fs.Glob(s.fsys, pattern)
	if err != nil {
		return nil, err
	}

	// Directories that match are not templates.
	files := matches[:0]
	for _, match := range matches {
		info, err := fs.Stat(s.fsys, match)
		if err == nil && info.IsDir() {
			continue
		}

		files = append(files, match)
	}

	if s.matches != nil {
		found, _ := s.matches.LoadOrStore(pattern, slices.Clip(files))
		return found.([]string), nil
	}

	return files, nil
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
//
// The map may be shared: do not change it.
func (s Scanner) RootContracts(names []string) (map[string]RootContract, error) {
	if len(names) == 1 {
		scan, err := s.File(names[0])
		if err != nil {
			return nil, err
		}

		return scan.rootContracts()
	}

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

// rootContracts returns the file's typed root declarations by root name,
// made at the first call.
func (scan *FileScan) rootContracts() (map[string]RootContract, error) {
	scan.rootsOnce.Do(func() {
		roots := make(map[string]RootContract, len(scan.Contracts))
		if err := addContracts(roots, scan.Contracts); err != nil {
			scan.rootsErr = err
			return
		}

		scan.roots = roots
	})

	return scan.roots, scan.rootsErr
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
