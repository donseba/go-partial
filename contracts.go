package partial

import (
	"errors"
	"fmt"
	"html/template"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/donseba/go-partial/internal/templateutil"
)

// Declaration is one typed declaration in a template comment, such as
// "@model Page github.com/acme/app.Page" or "@dot github.com/acme/app.Row".
// A @dot declaration has the annotation "dot" and no name.
type Declaration struct {
	Annotation string
	Name       string
	// Type is the declared Go type as package path and name, also when it
	// was written as path/Name.
	Type string
}

// ParseDeclarations returns the typed declarations of template sources, in
// the order they appear: the typed roots go-partial binds with SetModel and
// SetContract, and the @dot types. A root declared twice with one type is
// listed once; a root declared with two types is an error, as it is when
// the templates render.
func ParseDeclarations(sources ...string) ([]Declaration, error) {
	var declarations []Declaration

	roots := make(map[string]string)
	dots := make(map[string]bool)

	for _, source := range sources {
		contracts, dot := templateutil.Declarations(source)

		if dot != "" && !dots[dot] {
			dots[dot] = true
			declarations = append(declarations, Declaration{Annotation: "dot", Type: dot})
		}

		for _, contract := range contracts {
			if previous, ok := roots[contract.Name]; ok {
				if previous != contract.Type {
					return nil, fmt.Errorf(
						"@%s %s is declared as both %s and %s",
						contract.Annotation,
						contract.Name,
						previous,
						contract.Type,
					)
				}

				continue
			}

			roots[contract.Name] = contract.Type
			declarations = append(declarations, Declaration{
				Annotation: contract.Annotation,
				Name:       contract.Name,
				Type:       contract.Type,
			})
		}
	}

	return declarations, nil
}

// CheckContracts reports whether the partial can render with the values it
// has, without rendering it: every typed root its templates declare, such
// as @model Page, has exactly one matching value among those set on the
// partial and its parents, and its dot is of the type the first template
// declares with @dot. A @dot of any or interface{} accepts every dot. It
// returns the first problem found, or nil.
//
// Use it to decide whether a partial suits a request before committing to
// it, for example in a target resolver; see Runtime.CheckContracts for a
// partial that is not attached to the tree yet.
func (p *Partial) CheckContracts() error {
	if p == nil {
		return errors.New("partial is not initialized")
	}

	p.mu.RLock()
	templates := p.templates
	p.mu.RUnlock()

	if len(templates) == 0 {
		return errors.New("no templates provided for rendering")
	}

	cached := p.usesCache()
	scanner := p.scannerFor(cached)

	templateTree, err := p.renderTemplates(cached)
	if err != nil {
		return err
	}

	contracts, err := scanner.RootContracts(templateTree)
	if err != nil {
		return fmt.Errorf("error scanning template contracts: %w", err)
	}

	if err := validateRootContracts(contracts); err != nil {
		return err
	}

	bindings := p.getContracts()
	for _, name := range slices.Sorted(maps.Keys(contracts)) {
		if _, err := resolveContractValue(name, contracts[name], bindings); err != nil {
			return err
		}
	}

	scan, err := scanner.File(templates[0])
	if err != nil {
		return fmt.Errorf("error scanning template contracts: %w", err)
	}

	if scan.Dot == "" {
		return nil
	}

	dot, lazy, hasDot := p.getDot()
	if lazy != nil {
		// A dot computed when the partial renders is checked by its type,
		// when that is known; see DotFuncOf.
		if lazy.typ == nil || dotTypeMatches(scan.Dot, lazy.typ) {
			return nil
		}

		return fmt.Errorf("check contracts: @dot expects %s, got %s", scan.Dot, fullTypeName(lazy.typ))
	}

	if !dotMatchesType(scan.Dot, dot, hasDot) {
		return fmt.Errorf("check contracts: @dot expects %s, got %s", scan.Dot, describeValueType(dot, hasDot))
	}

	return nil
}

// dotTypeMatches reports whether values of typ, also behind a pointer, have
// the type a template declares with @dot.
func dotTypeMatches(declared string, typ reflect.Type) bool {
	declared = templateutil.NormalizeContractType(strings.TrimLeft(declared, "*"))
	if declared == "any" || declared == "interface{}" {
		return true
	}

	return contractTypeMatches(declared, typ)
}

// dotMatchesType reports whether a dot, also behind a pointer, has the type
// a template declares with @dot.
func dotMatchesType(declared string, value any, hasValue bool) bool {
	declared = templateutil.NormalizeContractType(strings.TrimLeft(declared, "*"))
	if declared == "any" || declared == "interface{}" {
		return true
	}

	if !hasValue || value == nil {
		return false
	}

	return contractValueMatchesType(declared, value)
}

func describeValueType(value any, hasValue bool) string {
	if !hasValue || value == nil {
		return "no dot"
	}

	return fullTypeName(reflect.TypeOf(value))
}

func validateRootContracts(contracts map[string]templateutil.RootContract) error {
	for name := range contracts {
		if _, protected := coreFunctionNames[name]; protected {
			return fmt.Errorf("register contracts: %s conflicts with a go-partial template helper", name)
		}
	}
	return nil
}

func placeholderRootFuncMap(contracts map[string]templateutil.RootContract) template.FuncMap {
	funcs := make(template.FuncMap, len(contracts))
	for name := range contracts {
		funcs[name] = func() any {
			return nil
		}
	}
	return funcs
}

// checkRootContracts reports the first typed root that finds no value among
// bindings.
func checkRootContracts(contracts map[string]templateutil.RootContract, bindings []contractInformation) error {
	for name, contract := range contracts {
		if _, err := resolveContractValue(name, contract, bindings); err != nil {
			return err
		}
	}

	return nil
}

func registerRootContracts(
	state *RenderContext,
	tmpl *template.Template,
	contracts map[string]templateutil.RootContract,
	bindings []contractInformation,
) error {
	funcs := make(template.FuncMap, len(contracts))
	for name, contract := range contracts {
		value, err := resolveContractValue(name, contract, bindings)
		if err != nil {
			return err
		}
		funcs[name] = rootValueFunc(state, value)
	}
	tmpl.Funcs(funcs)
	return nil
}

// rootValueFunc returns the function a template calls for a typed root
// with value, as resolveContractValue found it: a lazy value is computed
// when it is called.
func rootValueFunc(state *RenderContext, value any) any {
	if lazy, ok := value.(*lazyDot); ok {
		return lazyValueFunc(state, lazy)
	}

	return func() any {
		return value
	}
}

// bindingMatches reports whether the value of binding has the type a
// template declares: a lazy value by the type of the values it computes.
func bindingMatches(contractType string, binding contractInformation) bool {
	if lazy, ok := binding.Value.(*lazyDot); ok {
		return lazy.typ != nil && contractTypeMatches(contractType, lazy.typ)
	}

	return contractValueMatchesType(contractType, binding.Value)
}

// bindingTypeName names the type of the value of binding, for errors.
func bindingTypeName(binding contractInformation) string {
	if lazy, ok := binding.Value.(*lazyDot); ok {
		return fullTypeName(lazy.typ)
	}

	return contractValueTypeName(binding.Value)
}

// resolveContractValue finds the value for a declared root. bindings run from
// the root partial to the rendered one; a value bound by name wins, the
// nearest one first, over values matched by type.
func resolveContractValue(name string, contract templateutil.RootContract, bindings []contractInformation) (any, error) {
	for i := len(bindings) - 1; i >= 0; i-- {
		binding := bindings[i]
		if binding.Kind != "" && binding.Kind != contractRoot {
			continue
		}
		if binding.Annotation != "" && binding.Annotation != contract.Annotation {
			continue
		}
		if binding.Name != name {
			continue
		}

		if !bindingMatches(contract.Type, binding) {
			return nil, fmt.Errorf(
				"register contracts: @%s %s expects %s, got %s",
				contract.Annotation,
				name,
				contract.Type,
				bindingTypeName(binding),
			)
		}
		return binding.Value, nil
	}

	var match any
	matches := 0
	for _, binding := range bindings {
		if binding.Kind != "" && binding.Kind != contractRoot {
			continue
		}
		if binding.Name != "" {
			continue
		}
		if binding.Annotation != "" && binding.Annotation != contract.Annotation {
			continue
		}

		if bindingMatches(contract.Type, binding) {
			match = binding.Value
			matches++
		}
	}

	switch matches {
	case 1:
		return match, nil
	case 0:
		return nil, fmt.Errorf("register contracts: @%s %s has no matching value for %s", contract.Annotation, name, contract.Type)
	default:
		return nil, fmt.Errorf("register contracts: @%s %s has multiple matching values for %s; bind it by name", contract.Annotation, name, contract.Type)
	}
}

func contractValueMatchesType(contractType string, value any) bool {
	if value == nil {
		return false
	}

	return contractTypeMatches(contractType, reflect.TypeOf(value))
}

// contractTypeMatches reports whether values of valueType match the
// declared contractType.
func contractTypeMatches(contractType string, valueType reflect.Type) bool {
	valueName := contractTypeName(valueType)
	if valueName == "" {
		return false
	}
	contractType = templateutil.NormalizeContractType(contractType)
	if valueName == contractType {
		return true
	}

	if strings.HasPrefix(valueName, "main.") && shortContractTypeName(valueName) == shortContractTypeName(contractType) {
		return true
	}

	// Unnamed types, such as []github.com/acme/app.Row, are written with
	// the package paths of their elements.
	typ := valueType
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	return typ.Name() == "" && fullTypeName(typ) == contractType
}

// fullTypeName writes a type with the package paths of the named types in
// it, as templates declare them: []github.com/acme/app.Row rather than
// []app.Row.
func fullTypeName(typ reflect.Type) string {
	if typ == nil {
		return "nil"
	}

	if typ.Name() != "" {
		if typ.PkgPath() == "" {
			return typ.Name()
		}

		return typ.PkgPath() + "." + typ.Name()
	}

	switch typ.Kind() {
	case reflect.Pointer:
		return "*" + fullTypeName(typ.Elem())
	case reflect.Slice:
		return "[]" + fullTypeName(typ.Elem())
	case reflect.Array:
		return fmt.Sprintf("[%d]%s", typ.Len(), fullTypeName(typ.Elem()))
	case reflect.Map:
		return "map[" + fullTypeName(typ.Key()) + "]" + fullTypeName(typ.Elem())
	default:
		return typ.String()
	}
}

// contractTypeNames are the names contractValueTypeName gives types, by
// type: a render matches the same few types against its contracts many
// times.
var contractTypeNames sync.Map

func contractValueTypeName(value any) string {
	if value == nil {
		return ""
	}

	return contractTypeName(reflect.TypeOf(value))
}

// contractTypeName is the name contracts give values of valueType.
func contractTypeName(valueType reflect.Type) string {
	if valueType == nil {
		return ""
	}

	if name, ok := contractTypeNames.Load(valueType); ok {
		return name.(string)
	}

	typ := valueType
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	name := typ.PkgPath() + "." + typ.Name()
	if typ.Name() == "" || typ.PkgPath() == "" {
		name = valueType.String()
	}

	contractTypeNames.Store(valueType, name)

	return name
}

func shortContractTypeName(typeName string) string {
	lastDot := strings.LastIndex(typeName, ".")
	if lastDot < 0 || lastDot == len(typeName)-1 {
		return typeName
	}
	return typeName[lastDot+1:]
}
