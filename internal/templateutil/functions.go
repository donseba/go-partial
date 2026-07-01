package templateutil

import (
	"html/template"
	"maps"
	"slices"
	"sort"
	"strings"
)

func Names(funcs template.FuncMap) map[string]struct{} {
	names := make(map[string]struct{}, len(funcs))
	for name := range funcs {
		names[name] = struct{}{}
	}
	return names
}

func MergeFuncMaps(funcMaps ...template.FuncMap) template.FuncMap {
	size := 0
	for _, funcMap := range funcMaps {
		size += len(funcMap)
	}
	funcs := make(template.FuncMap, size)
	for _, funcMap := range funcMaps {
		maps.Copy(funcs, funcMap)
	}
	return funcs
}

func FunctionNameSignature(funcs template.FuncMap) string {
	names := make([]string, 0, len(funcs))
	for name := range funcs {
		names = append(names, name)
	}
	return FunctionNameSignatureFromNames(names)
}

func FunctionNameSignatureFromSet(names map[string]struct{}) string {
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	return FunctionNameSignatureFromNames(out)
}

func MergeFunctionSignatures(signatures ...string) string {
	var names []string
	for _, signature := range signatures {
		for _, name := range strings.Split(signature, ";") {
			if name == "" {
				continue
			}
			names = append(names, name)
		}
	}
	return FunctionNameSignatureFromNames(names)
}

func FunctionNameSignatureFromNames(names []string) string {
	if len(names) == 0 {
		return ""
	}

	names = slices.Clone(names)
	sort.Strings(names)
	names = slices.Compact(names)

	var builder strings.Builder
	for _, name := range names {
		if name == "" {
			continue
		}
		builder.WriteString(name)
		builder.WriteString(";")
	}
	return builder.String()
}
