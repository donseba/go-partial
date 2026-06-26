package partial

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"text/template"
)

type (
	TemplateModel struct {
		Name  string
		Value any
	}

	templateContract struct {
		Models map[string]contractModel
	}

	contractModel struct {
		Name string
		Type string
	}

	templateAnalysis struct {
		RequiredFuncs map[string]struct{}
		Contract      templateContract
	}
)

var contractModelPattern = regexp.MustCompile(`(?m)^\s*@model\s+([A-Za-z][A-Za-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_./\[\]*-]*)\s*$`)

func Model(name string, value any) TemplateModel {
	return TemplateModel{Name: name, Value: value}
}

func analyzeTemplatesFromFS(fsys fs.FS, names []string) (templateAnalysis, error) {
	analysis := templateAnalysis{
		RequiredFuncs: make(map[string]struct{}),
		Contract: templateContract{
			Models: make(map[string]contractModel),
		},
	}

	for _, name := range names {
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return templateAnalysis{}, err
		}

		funcs, err := RequiredFuncs(name, string(content))
		if err != nil {
			return templateAnalysis{}, err
		}
		for _, fn := range funcs {
			analysis.RequiredFuncs[fn] = struct{}{}
		}

		contract := parseTemplateContract(string(content))
		for modelName, model := range contract.Models {
			if !validContractModelName(modelName) {
				return templateAnalysis{}, fmt.Errorf("template model %q uses a reserved template function name", modelName)
			}
			if existing, ok := analysis.Contract.Models[modelName]; ok && existing.Type != model.Type {
				return templateAnalysis{}, fmt.Errorf("template model %q declared as both %q and %q", modelName, existing.Type, model.Type)
			}
			analysis.Contract.Models[modelName] = model
		}
	}

	return analysis, nil
}

func parseTemplateContract(src string) templateContract {
	contract := templateContract{Models: make(map[string]contractModel)}
	for _, match := range contractModelPattern.FindAllStringSubmatch(src, -1) {
		name := match[1]
		contract.Models[name] = contractModel{
			Name: name,
			Type: match[2],
		}
	}
	return contract
}

func (c templateContract) modelFuncNames() []string {
	names := make([]string, 0, len(c.Models))
	for name := range c.Models {
		names = append(names, contractModelFuncName(name))
	}
	sort.Strings(names)
	return names
}

func (c templateContract) validateModels(models map[string]any) error {
	for name, model := range c.Models {
		value, ok := models[name]
		if !ok || value == nil {
			return fmt.Errorf("template model %q (%s) is required", name, model.Type)
		}
	}
	return nil
}

func contractModelFuncName(name string) string {
	return name
}

func contractPlaceholderFuncMap(contract templateContract) template.FuncMap {
	funcs := make(template.FuncMap, len(contract.Models))
	for _, name := range contract.modelFuncNames() {
		funcs[name] = func() any { return nil }
	}
	return funcs
}

func contractFuncMap(contract templateContract, models map[string]any) template.FuncMap {
	funcs := make(template.FuncMap, len(contract.Models))
	for modelName := range contract.Models {
		name := modelName
		funcName := contractModelFuncName(name)
		funcs[funcName] = func() any {
			return models[name]
		}
	}
	return funcs
}

func validContractModelName(name string) bool {
	return name != "" && !isProtectedFunctionName(name)
}
