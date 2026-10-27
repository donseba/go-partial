package partial

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/donseba/go-partial/internal/templateutil"
)

// SetIncludes adds the templates that match patterns, such as
// "partials/*.gohtml", to every render of the partial and of the partials
// rendered under it: its children and content, the partials {{ partial }}
// renders and those that name it with SetParent. Their templates can call
// the templates these files define, as if each partial listed the files
// itself. Patterns are those of fs.Glob, resolved against the file system
// of the partial being rendered; a partial's patterns come after those of
// its ancestors, and a file is parsed once, also when several patterns or
// the partial's own templates name it. With the template cache (see
// UseTemplateCache) a pattern is resolved once per file system; without,
// at every render, so new files show.
//
//	root := partial.New("layout.gohtml").
//		SetFileSystem(theme).
//		SetIncludes("partials/*.gohtml")
//
// SetIncludes replaces the partial's own patterns; no patterns removes
// them.
func (p *Partial) SetIncludes(patterns ...string) *Partial {
	if p == nil {
		return nil
	}

	cleaned := cleanPatterns(patterns)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.includes = cleaned

	return p
}

// SetChildIncludes is SetIncludes for the partials rendered under the
// partial only, not for the partial itself: a root can give the content,
// regions and fragments of every page their shared templates, such as a
// theme's "partials/*.gohtml", without adding them to the parse of its
// layout.
//
//	root := partial.New("layout.gohtml").
//		SetFileSystem(theme).
//		SetChildIncludes("partials/*.gohtml")
//
// The patterns of descendants come after those of SetIncludes on the same
// partial. SetChildIncludes replaces the partial's own child patterns; no
// patterns removes them.
func (p *Partial) SetChildIncludes(patterns ...string) *Partial {
	if p == nil {
		return nil
	}

	cleaned := cleanPatterns(patterns)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.childIncludes = nil
	if len(cleaned) > 0 {
		p.childIncludes = &cleaned
	}

	return p
}

// cleanPatterns trims patterns and leaves out empty and repeated ones.
func cleanPatterns(patterns []string) []string {
	var cleaned []string

	for _, pattern := range patterns {
		pattern = trimPattern(pattern)
		if pattern == "" || slices.Contains(cleaned, pattern) {
			continue
		}

		cleaned = append(cleaned, pattern)
	}

	return cleaned
}

// trimPattern trims a pattern or file name of SetIncludes, Glob and the
// lookups of files: without surrounding space and leading slashes, as io/fs
// names files.
func trimPattern(pattern string) string {
	return strings.TrimLeft(strings.TrimSpace(pattern), "/")
}

// cleanFileName is the name of a file in the partial's file system as the
// lookups of files take it: trimmed and cleaned, "." for an empty one.
func cleanFileName(name string) string {
	return path.Clean(trimPattern(name))
}

// getIncludePatterns returns the include patterns of the partial's
// ancestors, with their child patterns, and its own, the root's first. The
// slice may be shared: do not change it.
func (p *Partial) getIncludePatterns() []string {
	var buffer [8][]string

	levels := buffer[:0]
	size := 0

	for current := p; current != nil; {
		current.mu.RLock()
		own := current.includes

		var children []string
		if current.childIncludes != nil {
			children = *current.childIncludes
		}

		parent := current.parent
		current.mu.RUnlock()

		// The levels are collected from the partial up and read back from
		// the root down: an ancestor's child patterns go first, so they
		// come after its own.
		if current != p && len(children) > 0 {
			levels = append(levels, children)
			size += len(children)
		}

		if len(own) > 0 {
			levels = append(levels, own)
			size += len(own)
		}

		current = parent
	}

	switch len(levels) {
	case 0:
		return nil
	case 1:
		return slices.Clip(levels[0])
	}

	patterns := make([]string, 0, size)

	for i := len(levels) - 1; i >= 0; i-- {
		for _, pattern := range levels[i] {
			if !slices.Contains(patterns, pattern) {
				patterns = append(patterns, pattern)
			}
		}
	}

	return patterns
}

// renderTemplates returns the templates a render of the partial parses: its
// template tree, then the files its include patterns match, each once.
func (p *Partial) renderTemplates(cached bool) ([]string, error) {
	tree := p.templateTree()

	patterns := p.getIncludePatterns()
	if len(patterns) == 0 {
		return tree, nil
	}

	scanner := p.scannerFor(cached)

	var buffer [8][]string

	groups := buffer[:0]
	size := len(tree)

	for _, pattern := range patterns {
		matches, err := scanner.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("error resolving includes %q: %w", pattern, err)
		}

		groups = append(groups, matches)
		size += len(matches)
	}

	if size == len(tree) {
		return tree, nil
	}

	templates := make([]string, 0, size)
	templates = append(templates, tree...)

	for _, matches := range groups {
		for _, match := range matches {
			if !slices.Contains(templates, match) {
				templates = append(templates, match)
			}
		}
	}

	return templates, nil
}

// Glob returns the template files pattern matches in the partial's file
// system, as SetIncludes resolves its patterns: with the template cache
// (see UseTemplateCache) the matches are remembered per file system;
// without, the file system is asked every time, so new files show.
// Directories are left out. The slice may be shared: do not change it.
//
//	files, err := root.Glob("regions/*.gohtml")
func (p *Partial) Glob(pattern string) ([]string, error) {
	if p == nil {
		return nil, errors.New("partial is not initialized")
	}

	return p.scanner().Glob(trimPattern(pattern))
}

// FirstExisting returns the first of candidates that is a file in the
// partial's file system, such as the template of a page's own type before
// the default one:
//
//	file, err := root.FirstExisting("pages/"+key+".gohtml", "pages/"+kind+".gohtml", "pages/page.gohtml")
//
// With the template cache (see UseTemplateCache) the answer for each
// candidate is remembered per file system, also when it is missing;
// without, the file system is asked every time. It returns an error that
// wraps fs.ErrNotExist when no candidate exists, or the first error other
// than a missing file.
func (p *Partial) FirstExisting(candidates ...string) (string, error) {
	if p == nil {
		return "", errors.New("partial is not initialized")
	}

	name, found, err := firstExisting(p.scanner(), candidates)
	if err != nil || found {
		return name, err
	}

	return "", noneExist(candidates)
}

// firstExisting returns the first of candidates that scanner finds; found
// is false when it finds none.
func firstExisting(scanner templateutil.Scanner, candidates []string) (name string, found bool, err error) {
	for _, candidate := range candidates {
		name := cleanFileName(candidate)
		if name == "." {
			continue
		}

		exists, err := scanner.Exists(name)
		if err != nil {
			return "", false, err
		}

		if exists {
			return name, true, nil
		}
	}

	return "", false, nil
}

// noneExist is the error of FirstExisting and FirstExistingIn when no
// candidate exists.
func noneExist(candidates []string) error {
	return fmt.Errorf("none of %s: %w", strings.Join(candidates, ", "), fs.ErrNotExist)
}

// Defines reports whether the template file defines a template called
// name with {{ define }}, such as the "html" part of an e-mail that a
// partial renders with SetEntry, without parsing it for a render: the
// scan of the file is the one renders use, kept with the template cache
// (see UseTemplateCache) and made again without. The file is read from
// the partial's file system; a missing file or one that does not parse is
// an error.
//
//	if ok, _ := root.Defines("mail/welcome.gohtml", "html"); ok {
//		html, err = partial.RenderSelf(ctx, nil, mail.SetEntry("html"))
//	}
func (p *Partial) Defines(file string, name string) (bool, error) {
	if p == nil {
		return false, errors.New("partial is not initialized")
	}

	scan, err := p.scanner().File(cleanFileName(file))
	if err != nil {
		return false, err
	}

	if scan.ParseErr != nil {
		return false, scan.ParseErr
	}

	return slices.Contains(scan.Defined, name), nil
}

// HasEntry reports whether the partial's first template defines its entry
// (see SetEntry); without an entry, whether it has a template.
func (p *Partial) HasEntry() (bool, error) {
	if p == nil {
		return false, errors.New("partial is not initialized")
	}

	p.mu.RLock()
	templates := p.templates
	entry := p.entry
	p.mu.RUnlock()

	if len(templates) == 0 {
		return false, nil
	}

	if entry == "" {
		return p.scanner().Exists(templates[0])
	}

	return p.Defines(templates[0], entry)
}

// FirstExistingIn returns the first of candidates found in layers, trying
// every candidate in a layer before the next layer: over a theme and a
// package's defaults, the theme's general template wins over the default
// specific one, where FirstExisting on Overlay(theme, defaults) would pick
// the latter.
//
//	file, err := root.FirstExistingIn(nil, "pages/"+key+".gohtml", "pages/"+kind+".gohtml")
//
// Without layers, those of the partial's file system are used when it is
// an Overlay, and the file system itself otherwise; nil layers are skipped.
// The answers are remembered as FirstExisting remembers them. The name
// returned is that of the file in the layer, which the overlay serves.
func (p *Partial) FirstExistingIn(layers []fs.FS, candidates ...string) (string, error) {
	if p == nil {
		return "", errors.New("partial is not initialized")
	}

	if len(layers) == 0 {
		layers = layersOf(p.getFS())
	}

	cached := p.usesCache()
	store := p.getTemplateStore()

	for _, layer := range layers {
		if layer == nil {
			continue
		}

		name, found, err := firstExisting(store.Scanner(layer, layerCacheKey(layer, cached)), candidates)
		if err != nil || found {
			return name, err
		}
	}

	return "", noneExist(candidates)
}

// layerCacheKey is the key under which the store remembers the answers of
// a layer of FirstExistingIn: none without the template cache or for a
// file system that cannot be told apart from others of its type.
func layerCacheKey(layer fs.FS, cached bool) string {
	if !cached {
		return ""
	}

	id, unique := fileSystemKey(layer)
	if !unique {
		return ""
	}

	return id
}
