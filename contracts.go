package partial

import (
	"fmt"
	"html/template"
	"reflect"
	"strings"

	"github.com/donseba/go-partial/internal/templateutil"
)

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

func registerRootContracts(tmpl *template.Template, contracts map[string]templateutil.RootContract, bindings []contractInformation) error {
	funcs := make(template.FuncMap, len(contracts))
	for name, contract := range contracts {
		value, err := resolveContractValue(name, contract, bindings)
		if err != nil {
			return err
		}
		captured := value
		funcs[name] = func() any {
			return captured
		}
	}
	tmpl.Funcs(funcs)
	return nil
}

func resolveContractValue(name string, contract templateutil.RootContract, bindings []contractInformation) (any, error) {
	for _, binding := range bindings {
		if binding.Kind != "" && binding.Kind != contractRoot {
			continue
		}
		if binding.Annotation != "" && binding.Annotation != contract.Annotation {
			continue
		}
		if binding.Name != name {
			continue
		}
		if !contractValueMatchesType(contract.Type, binding.Value) {
			return nil, fmt.Errorf("register contracts: @%s %s expects %s, got %s", contract.Annotation, name, contract.Type, contractValueTypeName(binding.Value))
		}
		return binding.Value, nil
	}

	var matches []any
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
		if contractValueMatchesType(contract.Type, binding.Value) {
			matches = append(matches, binding.Value)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("register contracts: @%s %s has no matching value for %s", contract.Annotation, name, contract.Type)
	default:
		return nil, fmt.Errorf("register contracts: @%s %s has multiple matching values for %s; bind it by name", contract.Annotation, name, contract.Type)
	}
}

func contractValueMatchesType(contractType string, value any) bool {
	valueType := contractValueTypeName(value)
	if valueType == "" {
		return false
	}
	contractType = templateutil.NormalizeContractType(contractType)
	if valueType == contractType {
		return true
	}
	return strings.HasPrefix(valueType, "main.") && shortContractTypeName(valueType) == shortContractTypeName(contractType)
}

func contractValueTypeName(value any) string {
	if value == nil {
		return ""
	}
	typ := reflect.TypeOf(value)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Name() == "" || typ.PkgPath() == "" {
		return fmt.Sprintf("%T", value)
	}
	return typ.PkgPath() + "." + typ.Name()
}

func shortContractTypeName(typeName string) string {
	lastDot := strings.LastIndex(typeName, ".")
	if lastDot < 0 || lastDot == len(typeName)-1 {
		return typeName
	}
	return typeName[lastDot+1:]
}
