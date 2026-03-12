package keainspect

import (
	"fmt"
	"sort"
	"strings"
)

type builderPropertySpec struct {
	ImportPath   string
	ImportedName string
}

var supportedBuilderProperties = map[string]builderPropertySpec{
	"props":           {ImportPath: "kea", ImportedName: "props"},
	"key":             {ImportPath: "kea", ImportedName: "key"},
	"path":            {ImportPath: "kea", ImportedName: "path"},
	"connect":         {ImportPath: "kea", ImportedName: "connect"},
	"actions":         {ImportPath: "kea", ImportedName: "actions"},
	"defaults":        {ImportPath: "kea", ImportedName: "defaults"},
	"loaders":         {ImportPath: "kea-loaders", ImportedName: "loaders"},
	"forms":           {ImportPath: "kea-forms", ImportedName: "forms"},
	"subscriptions":   {ImportPath: "kea-subscriptions", ImportedName: "subscriptions"},
	"windowValues":    {ImportPath: "kea-window-values", ImportedName: "windowValues"},
	"reducers":        {ImportPath: "kea", ImportedName: "reducers"},
	"selectors":       {ImportPath: "kea", ImportedName: "selectors"},
	"sharedListeners": {ImportPath: "kea", ImportedName: "sharedListeners"},
	"thunks":          {ImportPath: "kea-thunk", ImportedName: "thunks"},
	"listeners":       {ImportPath: "kea", ImportedName: "listeners"},
	"start":           {ImportPath: "kea-saga", ImportedName: "saga"},
	"stop":            {ImportPath: "kea-saga", ImportedName: "cancelled"},
	"saga":            {ImportPath: "kea-saga", ImportedName: "saga"},
	"workers":         {ImportPath: "kea-saga", ImportedName: "workers"},
	"takeEvery":       {ImportPath: "kea-saga", ImportedName: "takeEvery"},
	"takeLatest":      {ImportPath: "kea-saga", ImportedName: "takeLatest"},
	"actionToUrl":     {ImportPath: "kea-router", ImportedName: "actionToUrl"},
	"urlToAction":     {ImportPath: "kea-router", ImportedName: "urlToAction"},
	"events":          {ImportPath: "kea", ImportedName: "events"},
}

type builderImportPlanner struct {
	source           string
	importsByLocal   map[string]importedValueCandidate
	localsByKey      map[string]string
	namespacesByPath map[string]string
	decls            []namedImportDecl
	neededByPath     map[string]map[string]string
}

func newBuilderImportPlanner(source, file string) *builderImportPlanner {
	namespacesByPath := map[string]string{}
	for alias, importPath := range parseNamespaceValueImports(source) {
		if alias == "" || importPath == "" {
			continue
		}
		if _, exists := namespacesByPath[importPath]; !exists {
			namespacesByPath[importPath] = alias
		}
	}
	planner := &builderImportPlanner{
		source:           source,
		importsByLocal:   parseNamedValueImports(source),
		localsByKey:      map[string]string{},
		namespacesByPath: namespacesByPath,
		decls:            findNamedImportDecls(source, file),
		neededByPath:     map[string]map[string]string{},
	}
	for localName, candidate := range planner.importsByLocal {
		key := builderImportKey(candidate.Path, candidate.ImportedName)
		if candidate.Path == "" || candidate.ImportedName == "" || localName == "" {
			continue
		}
		if _, exists := planner.localsByKey[key]; !exists {
			planner.localsByKey[key] = localName
		}
	}
	return planner
}

func builderImportKey(importPath, importedName string) string {
	return importPath + "\x00" + importedName
}

func (p *builderImportPlanner) require(importPath, importedName string) string {
	key := builderImportKey(importPath, importedName)
	if localName, ok := p.localsByKey[key]; ok && localName != "" {
		return localName
	}
	if namespaceAlias := p.namespacesByPath[importPath]; namespaceAlias != "" {
		return namespaceAlias + "." + importedName
	}

	localName := importedName
	if !p.localNameAvailable(localName) {
		localName = p.nextAvailableLocalName(importedName)
	}
	p.importsByLocal[localName] = importedValueCandidate{
		Path:         importPath,
		ImportedName: importedName,
	}
	p.localsByKey[key] = localName
	if p.neededByPath[importPath] == nil {
		p.neededByPath[importPath] = map[string]string{}
	}
	p.neededByPath[importPath][localName] = importedName
	return localName
}

func (p *builderImportPlanner) localNameAvailable(localName string) bool {
	_, exists := p.importsByLocal[localName]
	return !exists
}

func (p *builderImportPlanner) nextAvailableLocalName(base string) string {
	candidates := []string{base, base + "Builder"}
	for _, candidate := range candidates {
		if p.localNameAvailable(candidate) {
			return candidate
		}
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%sBuilder%d", base, index)
		if p.localNameAvailable(candidate) {
			return candidate
		}
	}
}

func (p *builderImportPlanner) edits() ([]textEdit, error) {
	if len(p.neededByPath) == 0 {
		return nil, nil
	}

	paths := make([]string, 0, len(p.neededByPath))
	for importPath := range p.neededByPath {
		paths = append(paths, importPath)
	}
	sort.Strings(paths)

	var edits []textEdit
	var insertedLines []string
	for _, importPath := range paths {
		specifiers := renderBuilderImportSpecifiers(p.neededByPath[importPath])
		if len(specifiers) == 0 {
			continue
		}
		if decl, ok := p.runtimeDeclForPath(importPath); ok {
			merged := mergeImportSpecifierLists(splitImportSpecifierList(decl.SpecifiersText), specifiers)
			edits = append(edits, textEdit{
				Start:       decl.Start,
				End:         decl.End,
				Replacement: replacementForImportDecl(p.source, decl, merged),
			})
			continue
		}
		insertedLines = append(insertedLines, fmt.Sprintf("import { %s } from '%s'\n", strings.Join(specifiers, ", "), importPath))
	}

	if len(insertedLines) > 0 {
		insertPos, err := importInsertionPoint(p.source)
		if err != nil {
			return nil, err
		}
		for _, decl := range p.decls {
			if decl.Start < insertPos && decl.End > insertPos {
				insertPos = decl.End
			}
		}
		edits = append(edits, textEdit{
			Start:       insertPos,
			End:         insertPos,
			Replacement: strings.Join(insertedLines, ""),
		})
	}
	return edits, nil
}

func (p *builderImportPlanner) runtimeDeclForPath(importPath string) (namedImportDecl, bool) {
	for _, decl := range p.decls {
		if decl.Path == importPath && !decl.IsTypeOnly {
			return decl, true
		}
	}
	return namedImportDecl{}, false
}

func renderBuilderImportSpecifiers(specifiers map[string]string) []string {
	rendered := make([]string, 0, len(specifiers))
	for localName, importedName := range specifiers {
		if localName == importedName {
			rendered = append(rendered, importedName)
			continue
		}
		rendered = append(rendered, importedName+" as "+localName)
	}
	sort.Strings(rendered)
	return rendered
}

func mergeImportSpecifierLists(existing, additions []string) []string {
	seen := map[string]bool{}
	merged := make([]string, 0, len(existing)+len(additions))
	for _, specifier := range existing {
		specifier = strings.TrimSpace(specifier)
		if specifier == "" || seen[specifier] {
			continue
		}
		seen[specifier] = true
		merged = append(merged, specifier)
	}
	for _, specifier := range additions {
		specifier = strings.TrimSpace(specifier)
		if specifier == "" || seen[specifier] {
			continue
		}
		seen[specifier] = true
		merged = append(merged, specifier)
	}
	return merged
}

func replacementForImportDecl(source string, decl namedImportDecl, specifiers []string) string {
	trailing := "\n"
	if decl.Start >= 0 && decl.End <= len(source) && decl.End > decl.Start {
		raw := source[decl.Start:decl.End]
		trimmed := strings.TrimRight(raw, "\r\n")
		if len(trimmed) < len(raw) {
			trailing = raw[len(trimmed):]
		}
	}
	if decl.DefaultImport != "" {
		return fmt.Sprintf("import %s, { %s } from '%s'%s", decl.DefaultImport, strings.Join(specifiers, ", "), decl.Path, trailing)
	}
	return fmt.Sprintf("import { %s } from '%s'%s", strings.Join(specifiers, ", "), decl.Path, trailing)
}

func countConvertibleLogics(sourceLogics []SourceLogic) int {
	count := 0
	for _, logic := range sourceLogics {
		if logic.InputKind == "object" {
			count++
		}
	}
	return count
}

func convertFileToBuilders(source string, file parsedFile, options AppOptions) (string, int, []string, error) {
	sourceLogics, err := FindLogics(source)
	if err != nil {
		return "", 0, nil, err
	}

	planner := newBuilderImportPlanner(source, file.File)
	replacements := make([]textEdit, 0, len(sourceLogics))
	var warnings []string
	converted := 0

	for index, sourceLogic := range sourceLogics {
		if sourceLogic.InputKind != "object" {
			continue
		}

		parsedLogic := ParsedLogic{Name: sourceLogic.Name}
		if index < len(file.Logics) {
			parsedLogic = file.Logics[index]
		}

		replacement, unsupported, err := buildBuilderLogicReplacement(source, sourceLogic, parsedLogic, options, planner)
		if err != nil {
			return "", 0, nil, err
		}
		replacements = append(replacements, textEdit{
			Start:       sourceLogic.ObjectStart,
			End:         sourceLogic.ObjectEnd + 1,
			Replacement: replacement,
		})
		converted++

		if len(unsupported) > 0 {
			sort.Strings(unsupported)
			logicName := firstNonEmpty(parsedLogic.Name, sourceLogic.Name, "logic")
			warnings = append(
				warnings,
				fmt.Sprintf("❗ Logic %q converted unsupported keys (%s) to builders without imports", logicName, strings.Join(unsupported, ", ")),
			)
		}
	}

	importEdits, err := planner.edits()
	if err != nil {
		return "", 0, nil, err
	}

	allEdits := make([]textEdit, 0, len(replacements)+len(importEdits))
	allEdits = append(allEdits, replacements...)
	allEdits = append(allEdits, importEdits...)
	updatedSource, err := applyTextEdits(source, allEdits)
	if err != nil {
		return "", 0, nil, err
	}
	return updatedSource, converted, warnings, nil
}

func buildBuilderLogicReplacement(
	source string,
	sourceLogic SourceLogic,
	parsedLogic ParsedLogic,
	options AppOptions,
	planner *builderImportPlanner,
) (string, []string, error) {
	entries := make([]string, 0, len(sourceLogic.Properties)+1)
	var unsupported []string

	if options.WritePaths && !hasSourceProperty(sourceLogic.Properties, "path") && len(parsedLogic.Path) > 0 {
		localName := planner.require("kea", "path")
		entries = append(entries, fmt.Sprintf("%s(%s)", localName, formatPathSegments(parsedLogic.Path)))
	}

	for _, property := range sourceLogic.Properties {
		valueEnd := trimExpressionEnd(source, property.ValueEnd)
		if valueEnd <= property.ValueStart {
			continue
		}
		valueText := strings.TrimSpace(source[property.ValueStart:valueEnd])
		if valueText == "" {
			continue
		}

		callName := property.Name
		if spec, ok := supportedBuilderProperties[property.Name]; ok {
			callName = planner.require(spec.ImportPath, spec.ImportedName)
		} else {
			unsupported = append(unsupported, property.Name)
		}
		entries = append(entries, fmt.Sprintf("%s(%s)", callName, valueText))
	}

	return renderBuilderArrayLiteral(source, sourceLogic, entries), unsupported, nil
}

func renderBuilderArrayLiteral(source string, sourceLogic SourceLogic, entries []string) string {
	if len(entries) == 0 {
		return "[]"
	}

	childIndent := containerChildIndent(source, sourceLogic.ObjectStart, sourceLogic.ObjectEnd)
	parentIndent := lineIndent(source, sourceLogic.ObjectStart)
	var builder strings.Builder
	builder.WriteString("[\n")
	for index, entry := range entries {
		builder.WriteString(childIndent)
		builder.WriteString(entry)
		if index < len(entries)-1 {
			builder.WriteString(",")
		}
		builder.WriteString("\n")
	}
	builder.WriteString(parentIndent)
	builder.WriteString("]")
	return builder.String()
}
