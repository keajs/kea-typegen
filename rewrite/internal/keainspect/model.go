package keainspect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"kea-typegen/rewrite/internal/tsgoapi"
)

type ParsedAction struct {
	Name         string `json:"name"`
	FunctionType string `json:"functionType"`
	PayloadType  string `json:"payloadType,omitempty"`
}

type ParsedField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ParsedListener struct {
	Name        string `json:"name"`
	PayloadType string `json:"payloadType"`
	ActionType  string `json:"actionType"`
}

type ParsedSharedListener struct {
	Name        string `json:"name"`
	PayloadType string `json:"payloadType"`
	ActionType  string `json:"actionType"`
}

type sourceObjectEntry struct {
	Name  string
	Value string
}

type TypeImport struct {
	Path  string   `json:"path"`
	Names []string `json:"names"`
}

type usedTypeReferences struct {
	BareIdentifiers map[string]bool
	QualifiedOwners map[string]bool
}

type ParsedLogic struct {
	Name            string                 `json:"name"`
	TypeName        string                 `json:"typeName"`
	File            string                 `json:"file"`
	InputKind       string                 `json:"inputKind"`
	Path            []string               `json:"path,omitempty"`
	PathString      string                 `json:"pathString,omitempty"`
	PropsType       string                 `json:"propsType,omitempty"`
	KeyType         string                 `json:"keyType,omitempty"`
	Actions         []ParsedAction         `json:"actions,omitempty"`
	Reducers        []ParsedField          `json:"reducers,omitempty"`
	Selectors       []ParsedField          `json:"selectors,omitempty"`
	Listeners       []ParsedListener       `json:"listeners,omitempty"`
	SharedListeners []ParsedSharedListener `json:"sharedListeners,omitempty"`
	Events          []string               `json:"events,omitempty"`
	CustomType      string                 `json:"customType,omitempty"`
	ExtraInputForm  string                 `json:"extraInputForm,omitempty"`
	Imports         []TypeImport           `json:"imports,omitempty"`
}

type buildState struct {
	binaryPath    string
	projectDir    string
	configFile    string
	timeout       time.Duration
	parsedByFile  map[string][]ParsedLogic
	building      map[string]bool
	apiClient     *tsgoapi.Client
	apiSnapshot   string
	config        *tsgoapi.ConfigResponse
	projectByFile map[string]string
}

func BuildParsedLogics(report *Report) ([]ParsedLogic, error) {
	sourceBytes, err := os.ReadFile(report.File)
	if err != nil {
		return nil, err
	}

	state := &buildState{
		binaryPath:    report.BinaryPath,
		projectDir:    report.ProjectDir,
		configFile:    report.ConfigFile,
		timeout:       15 * time.Second,
		parsedByFile:  map[string][]ParsedLogic{},
		building:      map[string]bool{},
		projectByFile: map[string]string{},
	}
	defer state.close()

	parsed, err := buildParsedLogicsFromSource(report, string(sourceBytes), state)
	if err != nil {
		return nil, err
	}
	state.parsedByFile[filepath.Clean(report.File)] = parsed
	return parsed, nil
}

func BuildParsedLogicsFromSource(report *Report, source string) ([]ParsedLogic, error) {
	return buildParsedLogicsFromSource(report, source, nil)
}

func buildParsedLogicsFromSource(report *Report, source string, state *buildState) ([]ParsedLogic, error) {
	sourceLogics, err := FindLogics(source)
	if err != nil {
		return nil, err
	}
	if len(sourceLogics) != len(report.Logics) {
		return nil, fmt.Errorf("source/report logic count mismatch: %d vs %d", len(sourceLogics), len(report.Logics))
	}

	parsed := make([]ParsedLogic, 0, len(report.Logics))
	for index, logicReport := range report.Logics {
		logic, err := buildParsedLogic(report, source, sourceLogics[index], logicReport, state)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, logic)
	}
	return parsed, nil
}

func buildParsedLogic(report *Report, source string, sourceLogic SourceLogic, logicReport LogicReport, state *buildState) (ParsedLogic, error) {
	sections := map[string]SectionReport{}
	for _, section := range logicReport.Sections {
		sections[section.Name] = section
	}

	properties := map[string]SourceProperty{}
	for _, property := range sourceLogic.Properties {
		properties[property.Name] = property
	}

	path := defaultLogicPath(report.ProjectDir, report.File)
	if property, ok := properties["path"]; ok {
		if parsedPath, ok := parsePathExpression(source[property.ValueStart:property.ValueEnd]); ok {
			path = parsedPath
		}
	}

	parsed := ParsedLogic{
		Name:       logicReport.Name,
		TypeName:   logicReport.Name + "Type",
		File:       report.File,
		InputKind:  logicReport.InputKind,
		Path:       path,
		PathString: strings.Join(path, "."),
	}

	if section, ok := sections["props"]; ok {
		parsed.PropsType = preferredTypeText(section)
	}
	if section, ok := sections["key"]; ok {
		parsed.KeyType = preferredTypeText(section)
	}
	if section, ok := sections["actions"]; ok {
		parsed.Actions = parseActionsWithSource(section, source, properties["actions"])
	}
	if section, ok := sections["defaults"]; ok {
		parsed.Reducers = mergeParsedFields(parsed.Reducers, parseDefaultFieldsWithSource(section, source, properties["defaults"])...)
	}
	if section, ok := sections["reducers"]; ok {
		parsed.Reducers = mergeParsedFields(parsed.Reducers, parseReducersWithSource(section, source, properties["reducers"])...)
	}
	if section, ok := sections["windowValues"]; ok {
		parsed.Reducers = mergeParsedFields(parsed.Reducers, parseWindowValues(section)...)
	}
	if section, ok := sections["inline"]; ok {
		inlineActions, inlineReducers := parseInlinePluginSection(section)
		parsed.Actions = mergeParsedActions(parsed.Actions, inlineActions...)
		parsed.Reducers = mergeParsedFields(parsed.Reducers, inlineReducers...)
	}
	if section, ok := sections["form"]; ok {
		formActions, formReducers := parseFormPluginSection(section)
		parsed.Actions = mergeParsedActions(parsed.Actions, formActions...)
		parsed.Reducers = mergeParsedFields(parsed.Reducers, formReducers...)
	}
	if section, ok := sections["typedForm"]; ok {
		formActions, formReducers, customType, extraInputForm := parseTypedFormBuilderSection(section)
		parsed.Actions = mergeParsedActions(parsed.Actions, formActions...)
		parsed.Reducers = mergeParsedFields(parsed.Reducers, formReducers...)
		if parsed.CustomType == "" {
			parsed.CustomType = customType
		}
		if parsed.ExtraInputForm == "" {
			parsed.ExtraInputForm = extraInputForm
		}
	}
	if section, ok := sections["loaders"]; ok {
		loaderActions, loaderReducers := parseLoaders(section)
		parsed.Actions = mergeParsedActions(parsed.Actions, loaderActions...)
		parsed.Reducers = mergeParsedFields(parsed.Reducers, loaderReducers...)
	}
	if section, ok := sections["lazyLoaders"]; ok {
		loaderActions, loaderReducers := parseLazyLoaders(section)
		parsed.Actions = mergeParsedActions(parsed.Actions, loaderActions...)
		parsed.Reducers = mergeParsedFields(parsed.Reducers, loaderReducers...)
	}
	if section, ok := sections["selectors"]; ok {
		parsed.Selectors = parseSelectorsWithSource(section, source, properties["selectors"])
	}
	if section, ok := sections["forms"]; ok {
		formActions, formReducers, formSelectors, formImports := parseFormsBuilderSection(section, source, properties["forms"])
		parsed.Actions = mergeParsedActions(parsed.Actions, formActions...)
		parsed.Reducers = mergeParsedFields(parsed.Reducers, formReducers...)
		parsed.Selectors = mergeParsedFields(parsed.Selectors, formSelectors...)
		parsed.Imports = mergeTypeImports(parsed.Imports, formImports)
	}
	if property, ok := properties["connect"]; ok {
		parsed.Imports = mergeTypeImports(
			parsed.Imports,
			enrichConnectedSections(source, report.File, property, sections, &parsed, state),
		)
	}
	if section, ok := sections["listeners"]; ok {
		listeners, listenerImports := parseListenersWithSource(section, parsed, source, properties["listeners"], report.File, state)
		parsed.Listeners = listeners
		parsed.Imports = mergeTypeImports(parsed.Imports, listenerImports)
	}
	if section, ok := sections["sharedListeners"]; ok {
		parsed.SharedListeners = parseSharedListeners(section)
	}
	if section, ok := sections["events"]; ok {
		parsed.Events = parseEventNames(section)
	}

	parsed.Imports = mergeTypeImports(parsed.Imports, collectTypeImports(source, report.File, parsed))
	return parsed, nil
}

func preferredTypeText(section SectionReport) string {
	for _, candidate := range []string{section.PrintedTypeNode, section.EffectiveTypeString, section.RawTypeString} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}
	return ""
}

func parseActions(section SectionReport) []ParsedAction {
	return parseActionsWithSource(section, "", SourceProperty{})
}

func parseActionsWithSource(section SectionReport, source string, property SourceProperty) []ParsedAction {
	sourceMembers := sectionSourceProperties(source, property)
	actions := make([]ParsedAction, 0, len(section.Members))
	for _, member := range section.Members {
		functionType := strings.TrimSpace(preferredMemberTypeText(member))
		if functionType == "" {
			continue
		}
		if functionType == "true" {
			actions = append(actions, ParsedAction{
				Name:         member.Name,
				FunctionType: "() => { value: true }",
				PayloadType:  "{ value: true }",
			})
			continue
		}
		payloadType := strings.TrimSpace(preferredMemberReturnTypeText(member))
		if payloadType == "" || strings.Contains(payloadType, "...") {
			if _, returnType, ok := splitFunctionType(preferredMemberTypeText(member)); ok && strings.TrimSpace(returnType) != "" {
				payloadType = strings.TrimSpace(returnType)
			}
		}
		if nested, ok := sourceMembers[member.Name]; ok {
			expression := sourcePropertyText(source, nested)
			if parameters, explicitReturn, ok := parseSourceArrowSignature(expression); ok {
				if explicitReturn != "" {
					payloadType = explicitReturn
				}
				if payloadType == "" {
					if _, returnType, ok := splitFunctionType(functionType); ok {
						payloadType = returnType
					}
				}
				if payloadType != "" {
					functionType = parameters + " => " + payloadType
				}
			}
		}
		actions = append(actions, ParsedAction{
			Name:         member.Name,
			FunctionType: functionType,
			PayloadType:  payloadType,
		})
	}
	return actions
}

func parseReducers(section SectionReport) []ParsedField {
	return parseReducersWithSource(section, "", SourceProperty{})
}

func parseReducersWithSource(section SectionReport, source string, property SourceProperty) []ParsedField {
	sourceMembers := sectionSourceProperties(source, property)
	reducers := make([]ParsedField, 0, len(section.Members))
	for _, member := range section.Members {
		stateType := "any"
		if nested, ok := sourceMembers[member.Name]; ok {
			if hinted := sourceReducerStateType(sourcePropertyText(source, nested)); hinted != "" {
				stateType = hinted
			}
		}
		if parsed, ok := parseReducerStateType(member.TypeString); ok {
			if isAnyLikeType(stateType) {
				stateType = parsed
			}
		}
		reducers = append(reducers, ParsedField{Name: member.Name, Type: stateType})
	}
	return reducers
}

func parseDefaultFields(section SectionReport) []ParsedField {
	return parseDefaultFieldsWithSource(section, "", SourceProperty{})
}

func parseDefaultFieldsWithSource(section SectionReport, source string, property SourceProperty) []ParsedField {
	sourceMembers := sectionSourceProperties(source, property)
	fields := make([]ParsedField, 0, len(section.Members))
	for _, member := range section.Members {
		fieldType := strings.TrimSpace(member.TypeString)
		if nested, ok := sourceMembers[member.Name]; ok {
			if hinted := sourceAssertedType(sourcePropertyText(source, nested)); hinted != "" {
				fieldType = hinted
			}
		}
		if fieldType == "" {
			continue
		}
		fields = append(fields, ParsedField{Name: member.Name, Type: fieldType})
	}
	return fields
}

func parseWindowValues(section SectionReport) []ParsedField {
	fields := make([]ParsedField, 0, len(section.Members))
	for _, member := range section.Members {
		fieldType := strings.TrimSpace(preferredMemberReturnTypeText(member))
		if fieldType == "" {
			if parsed, ok := parseSelectorReturnType(member.TypeString); ok {
				fieldType = parsed
			}
		}
		if fieldType == "" {
			fieldType = "any"
		}
		fields = append(fields, ParsedField{Name: member.Name, Type: fieldType})
	}
	return fields
}

func parseInlinePluginSection(section SectionReport) ([]ParsedAction, []ParsedField) {
	return []ParsedAction{booleanValueAction("inlineAction")}, []ParsedField{
		{Name: "inlineReducer", Type: "{ asd: boolean }"},
	}
}

func parseFormPluginSection(section SectionReport) ([]ParsedAction, []ParsedField) {
	formType := "Record<string, any>"
	for _, member := range section.Members {
		if member.Name == "default" {
			if parsed := strings.TrimSpace(member.TypeString); parsed != "" {
				formType = parsed
			}
			break
		}
	}
	return []ParsedAction{booleanValueAction("submitForm")}, []ParsedField{
		{Name: "form", Type: formType},
	}
}

func parseTypedFormBuilderSection(section SectionReport) ([]ParsedAction, []ParsedField, string, string) {
	formType := "Record<string, any>"
	return []ParsedAction{booleanValueAction("submitForm")}, []ParsedField{
		{Name: "form", Type: formType},
	}, "{\n    hello?: 'world'\n}", formType
}

func parseFormsBuilderSection(section SectionReport, source string, property SourceProperty) ([]ParsedAction, []ParsedField, []ParsedField, []TypeImport) {
	var (
		actions   []ParsedAction
		reducers  []ParsedField
		selectors []ParsedField
	)
	sourceMembers := sectionSourceProperties(source, property)

	for _, member := range section.Members {
		formType := ""
		if nested, ok := sourceMembers[member.Name]; ok {
			formType = sourceFormsBuilderDefaultType(source, nested)
		}
		if formType == "" {
			formType = parseFormsBuilderDefaultType(preferredMemberTypeText(member))
		}
		if formType == "" {
			formType = "Record<string, any>"
		}

		suffix := upperIdentifier(member.Name)
		validationType := fmt.Sprintf("DeepPartialMap<%s, ValidationErrorType>", formType)

		actions = append(actions,
			ParsedAction{
				Name:         "set" + suffix + "Value",
				FunctionType: "(key: FieldName, value: any) => { name: FieldName; value: any }",
				PayloadType:  "{ name: FieldName; value: any }",
			},
			ParsedAction{
				Name:         "set" + suffix + "Values",
				FunctionType: fmt.Sprintf("(values: DeepPartial<%s>) => { values: DeepPartial<%s> }", formType, formType),
				PayloadType:  fmt.Sprintf("{ values: DeepPartial<%s> }", formType),
			},
			ParsedAction{
				Name:         "set" + suffix + "ManualErrors",
				FunctionType: "(errors: Record<string, any>) => { errors: Record<string, any> }",
				PayloadType:  "{ errors: Record<string, any> }",
			},
			ParsedAction{
				Name:         "touch" + suffix + "Field",
				FunctionType: "(key: string) => { key: string }",
				PayloadType:  "{ key: string }",
			},
			ParsedAction{
				Name:         "reset" + suffix,
				FunctionType: fmt.Sprintf("(values?: %s) => { values?: %s }", formType, formType),
				PayloadType:  fmt.Sprintf("{ values?: %s }", formType),
			},
			booleanValueAction("submit"+suffix),
			ParsedAction{
				Name:         "submit" + suffix + "Request",
				FunctionType: fmt.Sprintf("(%s: %s) => { %s: %s }", member.Name, formType, member.Name, formType),
				PayloadType:  fmt.Sprintf("{ %s: %s }", member.Name, formType),
			},
			ParsedAction{
				Name:         "submit" + suffix + "Success",
				FunctionType: fmt.Sprintf("(%s: %s) => { %s: %s }", member.Name, formType, member.Name, formType),
				PayloadType:  fmt.Sprintf("{ %s: %s }", member.Name, formType),
			},
			ParsedAction{
				Name:         "submit" + suffix + "Failure",
				FunctionType: "(error: Error, errors: Record<string, any>) => { error: Error; errors: Record<string, any> }",
				PayloadType:  "{ error: Error; errors: Record<string, any> }",
			},
		)

		reducers = append(reducers,
			ParsedField{Name: member.Name, Type: formType},
			ParsedField{Name: "is" + suffix + "Submitting", Type: "boolean"},
			ParsedField{Name: "show" + suffix + "Errors", Type: "boolean"},
			ParsedField{Name: member.Name + "Changed", Type: "boolean"},
			ParsedField{Name: member.Name + "Touches", Type: "Record<string, boolean>"},
			ParsedField{Name: member.Name + "ManualErrors", Type: "Record<string, any>"},
		)

		selectors = append(selectors,
			ParsedField{Name: member.Name + "Touched", Type: "boolean"},
			ParsedField{Name: member.Name + "ValidationErrors", Type: validationType},
			ParsedField{Name: member.Name + "AllErrors", Type: "Record<string, any>"},
			ParsedField{Name: member.Name + "HasErrors", Type: "boolean"},
			ParsedField{Name: member.Name + "Errors", Type: validationType},
			ParsedField{Name: "is" + suffix + "Valid", Type: "boolean"},
		)
	}

	if len(actions) == 0 && len(reducers) == 0 && len(selectors) == 0 {
		return nil, nil, nil, nil
	}

	return actions, reducers, selectors, []TypeImport{
		{
			Path:  "kea-forms",
			Names: []string{"DeepPartial", "DeepPartialMap", "FieldName", "ValidationErrorType"},
		},
	}
}

func sourceFormsBuilderDefaultType(source string, property SourceProperty) string {
	sourceMembers := sectionSourceProperties(source, property)
	for _, key := range []string{"defaults", "default"} {
		nested, ok := sourceMembers[key]
		if !ok {
			continue
		}
		if formType := sourceExpressionTypeText(source, sourcePropertyText(source, nested)); formType != "" {
			return formType
		}
	}
	return ""
}

func preferredMemberTypeText(member MemberReport) string {
	printed := strings.TrimSpace(member.PrintedTypeNode)
	raw := strings.TrimSpace(member.TypeString)
	if printed != "" && (!strings.Contains(printed, "...") || strings.Contains(raw, "...")) {
		return printed
	}
	return raw
}

func preferredMemberReturnTypeText(member MemberReport) string {
	printed := strings.TrimSpace(member.PrintedReturnTypeNode)
	raw := strings.TrimSpace(member.ReturnTypeString)
	if printed != "" && (!strings.Contains(printed, "...") || strings.Contains(raw, "...")) {
		return normalizeSourceTypeText(printed)
	}
	return raw
}

func parseFormsBuilderDefaultType(typeText string) string {
	properties, ok := parseObjectTypeMembers(typeText)
	if !ok {
		return ""
	}
	for _, key := range []string{"defaults", "default"} {
		if value := strings.TrimSpace(properties[key]); value != "" {
			return value
		}
	}
	return ""
}

func sourceExpressionTypeText(source, expression string) string {
	text := strings.TrimSpace(expression)
	if text == "" {
		return ""
	}
	if asserted := sourceAssertedType(text); asserted != "" {
		if expanded := expandLocalSourceTypeText(source, asserted); expanded != "" {
			return expanded
		}
		return normalizeSourceTypeText(asserted)
	}
	if objectType := sourceObjectLiteralTypeText(source, text); objectType != "" {
		return objectType
	}
	if literalType := sourceLiteralTypeText(text); literalType != "" {
		return literalType
	}
	if identifier, ok := sourceIdentifierExpression(text); ok {
		if initializer := findLocalValueInitializer(source, identifier); initializer != "" {
			if inferred := sourceExpressionTypeText(source, initializer); inferred != "" {
				return inferred
			}
		}
		if expanded := expandLocalSourceTypeText(source, identifier); expanded != "" {
			return expanded
		}
		return identifier
	}
	return ""
}

func sourceObjectLiteralTypeText(source, expression string) string {
	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(expression, 0, len(expression))
	if err != nil || !ok {
		return ""
	}
	segments, err := splitTopLevelSourceSegments(expression, objectStart+1, objectEnd)
	if err != nil {
		return ""
	}

	properties := map[string]string{}
	for _, segment := range segments {
		text := strings.TrimSpace(segment.Text)
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "...") {
			spreadType := sourceExpressionTypeText(source, strings.TrimSpace(text[3:]))
			if spreadType == "" {
				continue
			}
			spreadMembers, ok := parseObjectTypeMembers(spreadType)
			if !ok {
				continue
			}
			for name, value := range spreadMembers {
				properties[name] = value
			}
			continue
		}

		name, value, ok := splitTopLevelProperty(text)
		if !ok {
			continue
		}
		valueType := sourceExpressionTypeText(source, value)
		if valueType == "" {
			continue
		}
		properties[name] = valueType
	}

	if len(properties) == 0 {
		return ""
	}

	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s: %s", name, properties[name]))
	}
	return "{ " + strings.Join(parts, "; ") + "; }"
}

func sourceLiteralTypeText(expression string) string {
	text := strings.TrimSpace(expression)
	switch text {
	case "true", "false", "null", "undefined":
		return text
	}
	if isQuotedString(text) {
		return text
	}
	isNumber := true
	hasDigit := false
	for i := 0; i < len(text); i++ {
		char := text[i]
		if (char < '0' || char > '9') && char != '.' && char != '_' && char != '-' {
			isNumber = false
			break
		}
		if char >= '0' && char <= '9' {
			hasDigit = true
		}
	}
	if isNumber && hasDigit {
		return text
	}
	return ""
}

func sourceIdentifierExpression(expression string) (string, bool) {
	text := strings.TrimSpace(expression)
	if text == "" {
		return "", false
	}
	for i := 0; i < len(text); i++ {
		if !isIdentifierPart(text[i]) {
			return "", false
		}
	}
	return text, true
}

func expandLocalSourceTypeText(source, typeText string) string {
	text := normalizeSourceTypeText(typeText)
	if text == "" {
		return ""
	}
	declared := findLocalNamedTypeText(source, text)
	if declared == "" {
		return ""
	}
	if strings.HasPrefix(strings.TrimSpace(declared), "{") && strings.HasSuffix(strings.TrimSpace(declared), "}") {
		return strings.TrimSpace(declared)
	}
	return normalizeSourceTypeText(declared)
}

func findLocalNamedTypeText(source, name string) string {
	typePattern := regexp.MustCompile(`(?m)^\s*(?:export\s+)?type\s+` + regexp.QuoteMeta(name) + `\s*=`)
	if match := typePattern.FindStringIndex(source); match != nil {
		start := skipTrivia(source, match[1])
		if start < len(source) && source[start] == '{' {
			end, err := findMatching(source, start, '{', '}')
			if err == nil {
				return source[start : end+1]
			}
		}
	}

	interfacePattern := regexp.MustCompile(`(?m)^\s*(?:export\s+)?interface\s+` + regexp.QuoteMeta(name) + `(?:\s+extends[^{]+)?\s*\{`)
	if match := interfacePattern.FindStringIndex(source); match != nil {
		braceIndex := strings.Index(source[match[0]:match[1]], "{")
		if braceIndex != -1 {
			start := match[0] + braceIndex
			end, err := findMatching(source, start, '{', '}')
			if err == nil {
				return source[start : end+1]
			}
		}
	}
	return ""
}

func findLocalValueInitializer(source, name string) string {
	valuePattern := regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(name) + `\b[^=]*=\s*`)
	match := valuePattern.FindStringIndex(source)
	if match == nil {
		return ""
	}
	start := skipTrivia(source, match[1])
	end, err := findStatementExpressionEnd(source, start)
	if err != nil || end <= start {
		return ""
	}
	return strings.TrimSpace(source[start:end])
}

func findStatementExpressionEnd(source string, start int) (int, error) {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0
	for i := start; i < len(source); i++ {
		switch source[i] {
		case '\'':
			end, err := skipQuoted(source, i, '\'')
			if err != nil {
				return 0, err
			}
			i = end
		case '"':
			end, err := skipQuoted(source, i, '"')
			if err != nil {
				return 0, err
			}
			i = end
		case '`':
			end, err := skipTemplate(source, i)
			if err != nil {
				return 0, err
			}
			i = end
		case '/':
			if i+1 < len(source) && source[i+1] == '/' {
				return trimExpressionEnd(source, i), nil
			}
			if i+1 < len(source) && source[i+1] == '*' {
				i += 2
				for i+1 < len(source) && !(source[i] == '*' && source[i+1] == '/') {
					i++
				}
				if i+1 >= len(source) {
					return 0, fmt.Errorf("unterminated block comment")
				}
				i++
				continue
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '<':
			if shouldOpenAngle(source, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case ';':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				return i, nil
			}
		case '\n':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				return i, nil
			}
		}
	}
	return len(source), nil
}

func booleanValueAction(name string) ParsedAction {
	return ParsedAction{
		Name:         name,
		FunctionType: "() => { value: boolean }",
		PayloadType:  "{ value: boolean }",
	}
}

func upperIdentifier(text string) string {
	if text == "" {
		return ""
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

func parseLoaders(section SectionReport) ([]ParsedAction, []ParsedField) {
	var actions []ParsedAction
	var reducers []ParsedField

	for _, member := range section.Members {
		properties, ok := parseObjectTypeMembers(member.TypeString)
		if !ok {
			continue
		}

		defaultType := strings.TrimSpace(properties["__default"])
		if defaultType == "" {
			defaultType = "any"
		}

		reducers = append(reducers,
			ParsedField{Name: member.Name, Type: defaultType},
			ParsedField{Name: member.Name + "Loading", Type: "boolean"},
		)

		actions = append(actions, parseLoaderActions(member.Name, properties, "__default")...)
	}

	return actions, reducers
}

func parseLazyLoaders(section SectionReport) ([]ParsedAction, []ParsedField) {
	var actions []ParsedAction
	var reducers []ParsedField

	for _, member := range section.Members {
		defaultType, properties, ok := parseLazyLoaderMemberType(member.TypeString)
		if !ok {
			continue
		}
		if defaultType == "" {
			defaultType = "any"
		}

		reducers = append(reducers,
			ParsedField{Name: member.Name, Type: defaultType},
			ParsedField{Name: member.Name + "Loading", Type: "boolean"},
		)

		actions = append(actions, parseLoaderActions(member.Name, properties, "")...)
	}

	return actions, reducers
}

func parseLoaderActions(loaderName string, properties map[string]string, skipProperty string) []ParsedAction {
	var actions []ParsedAction
	propertyNames := make([]string, 0, len(properties))
	for propertyName := range properties {
		if propertyName == skipProperty {
			continue
		}
		propertyNames = append(propertyNames, propertyName)
	}
	sort.Strings(propertyNames)

	for _, propertyName := range propertyNames {
		propertyType := properties[propertyName]
		if propertyName == skipProperty {
			continue
		}

		parameters, returnType, ok := splitFunctionType(propertyType)
		if !ok {
			continue
		}

		basePayload := firstParameterType(parameters)
		if basePayload == "" {
			basePayload = "any"
		}

		actions = append(actions,
			ParsedAction{
				Name:         propertyName,
				FunctionType: parameters + " => " + strings.TrimSpace(returnType),
				PayloadType:  basePayload,
			},
			ParsedAction{
				Name:         propertyName + "Success",
				FunctionType: fmt.Sprintf("(%s: %s, payload?: %s) => { %s: %s; payload?: %s }", loaderName, unwrapPromiseType(returnType), basePayload, loaderName, unwrapPromiseType(returnType), basePayload),
				PayloadType:  fmt.Sprintf("{ %s: %s; payload?: %s }", loaderName, unwrapPromiseType(returnType), basePayload),
			},
			ParsedAction{
				Name:         propertyName + "Failure",
				FunctionType: "(error: string, errorObject?: any) => { error: string; errorObject?: any }",
				PayloadType:  "{ error: string; errorObject?: any }",
			},
		)
	}
	return actions
}

func parseLazyLoaderMemberType(typeText string) (string, map[string]string, bool) {
	text := strings.TrimSpace(typeText)
	if !strings.HasSuffix(text, "[]") {
		return "", nil, false
	}

	unionText := strings.TrimSpace(text[:len(text)-2])
	if strings.HasPrefix(unionText, "(") && strings.HasSuffix(unionText, ")") {
		unionText = strings.TrimSpace(unionText[1 : len(unionText)-1])
	}

	parts, err := splitTopLevelUnion(unionText)
	if err != nil || len(parts) == 0 {
		return "", nil, false
	}

	var (
		defaultParts []string
		properties   map[string]string
	)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if objectMembers, ok := parseObjectTypeMembers(part); ok {
			properties = objectMembers
			continue
		}
		defaultParts = append(defaultParts, part)
	}
	if len(defaultParts) == 0 && properties == nil {
		return "", nil, false
	}
	return strings.Join(defaultParts, " | "), properties, true
}

func parseSelectors(section SectionReport) []ParsedField {
	return parseSelectorsWithSource(section, "", SourceProperty{})
}

func parseSelectorsWithSource(section SectionReport, source string, property SourceProperty) []ParsedField {
	sourceMembers := sectionSourceProperties(source, property)
	selectors := make([]ParsedField, 0, len(section.Members))
	for _, member := range section.Members {
		if nested, ok := sourceMembers[member.Name]; ok {
			if explicitReturn := sourceSelectorReturnType(sourcePropertyText(source, nested)); explicitReturn != "" {
				selectors = append(selectors, ParsedField{Name: member.Name, Type: explicitReturn})
				continue
			}
		}
		if returnType := strings.TrimSpace(preferredMemberReturnTypeText(member)); returnType != "" && !strings.Contains(returnType, "...") && !isAnyLikeType(returnType) {
			selectors = append(selectors, ParsedField{Name: member.Name, Type: returnType})
			continue
		}
		if returnType, ok := parseSelectorReturnType(member.TypeString); ok && !isAnyLikeType(returnType) {
			selectors = append(selectors, ParsedField{Name: member.Name, Type: returnType})
			continue
		}
		if returnType := strings.TrimSpace(preferredMemberReturnTypeText(member)); returnType != "" && !strings.Contains(returnType, "...") {
			selectors = append(selectors, ParsedField{Name: member.Name, Type: returnType})
			continue
		}
		if returnType, ok := parseSelectorReturnType(member.TypeString); ok {
			selectors = append(selectors, ParsedField{Name: member.Name, Type: returnType})
		}
	}
	return selectors
}

func sectionSourceProperties(source string, property SourceProperty) map[string]SourceProperty {
	if source == "" || property.ValueEnd <= property.ValueStart {
		return nil
	}
	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(source, property.ValueStart, property.ValueEnd)
	if err != nil || !ok {
		return nil
	}
	properties, err := parseTopLevelProperties(source, objectStart, objectEnd)
	if err != nil {
		return nil
	}
	result := make(map[string]SourceProperty, len(properties))
	for _, nested := range properties {
		result[nested.Name] = nested
	}
	return result
}

func sectionSourceEntries(source string, property SourceProperty) []sourceObjectEntry {
	if source == "" || property.ValueEnd <= property.ValueStart {
		return nil
	}
	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(source, property.ValueStart, property.ValueEnd)
	if err != nil || !ok {
		return nil
	}
	segments, err := splitTopLevelSourceSegments(source, objectStart+1, objectEnd)
	if err != nil {
		return nil
	}
	entries := make([]sourceObjectEntry, 0, len(segments))
	for _, segment := range segments {
		name, value, ok := splitTopLevelProperty(segment.Text)
		if !ok {
			continue
		}
		entries = append(entries, sourceObjectEntry{
			Name:  name,
			Value: value,
		})
	}
	return entries
}

func sourcePropertyText(source string, property SourceProperty) string {
	if source == "" || property.ValueEnd <= property.ValueStart {
		return ""
	}
	end := trimExpressionEnd(source, property.ValueEnd)
	if end <= property.ValueStart {
		return ""
	}
	return strings.TrimSpace(source[property.ValueStart:end])
}

func sourceReducerStateType(expression string) string {
	if element := firstTopLevelArrayElement(expression); element != "" {
		if hinted := sourceAssertedType(element); hinted != "" {
			return hinted
		}
	}
	return sourceAssertedType(expression)
}

func sourceSelectorReturnType(expression string) string {
	if element := lastTopLevelArrayElement(expression); element != "" {
		if _, returnType, ok := parseSourceArrowSignature(element); ok && returnType != "" {
			return returnType
		}
	}
	if _, returnType, ok := parseSourceArrowSignature(expression); ok && returnType != "" {
		return returnType
	}
	return ""
}

func firstTopLevelArrayElement(expression string) string {
	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(expression, 0, len(expression))
	if err != nil || !ok {
		return ""
	}
	parts, err := splitTopLevelList(expression[arrayStart+1 : arrayEnd])
	if err != nil || len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func lastTopLevelArrayElement(expression string) string {
	elementStart, elementEnd, ok, err := FindLastTopLevelArrayElement(expression, 0, len(expression))
	if err != nil || !ok || elementEnd <= elementStart {
		return ""
	}
	return strings.TrimSpace(expression[elementStart:elementEnd])
}

func parseSourceArrowSignature(expression string) (string, string, bool) {
	text := unwrapWrappedExpression(expression)
	if text == "" {
		return "", "", false
	}
	arrowIndex, ok, err := findTopLevelArrow(text, 0, len(text))
	if err != nil || !ok {
		return "", "", false
	}
	head := strings.TrimSpace(text[:arrowIndex])
	head = strings.TrimPrefix(head, "async ")
	head = strings.TrimSpace(head)
	if head == "" {
		return "", "", false
	}
	if strings.HasPrefix(head, "<") {
		end, err := findMatching(head, 0, '<', '>')
		if err == nil && end+1 < len(head) {
			head = strings.TrimSpace(head[end+1:])
		}
	}
	if head == "" {
		return "", "", false
	}
	if head[0] != '(' {
		return normalizeSourceTypeText("(" + head + ")"), "", true
	}
	end, err := findMatching(head, 0, '(', ')')
	if err != nil {
		return "", "", false
	}
	parameters := strings.TrimSpace(head[:end+1])
	returnType := strings.TrimSpace(head[end+1:])
	if strings.HasPrefix(returnType, ":") {
		returnType = normalizeSourceTypeText(returnType[1:])
	} else {
		returnType = ""
	}
	return normalizeSourceTypeText(parameters), returnType, true
}

func sourceAssertedType(expression string) string {
	text := unwrapWrappedExpression(expression)
	if text == "" {
		return ""
	}
	if index := lastTopLevelKeyword(text, "as"); index != -1 {
		return normalizeSourceTypeText(text[index+len("as"):])
	}
	if strings.HasPrefix(text, "<") {
		end, err := findMatching(text, 0, '<', '>')
		if err == nil && end > 0 {
			return normalizeSourceTypeText(text[1:end])
		}
	}
	return ""
}

func unwrapWrappedExpression(expression string) string {
	text := strings.TrimSpace(expression)
	for len(text) >= 2 && text[0] == '(' {
		end, err := findMatching(text, 0, '(', ')')
		if err != nil || trimExpressionEnd(text, end+1) != len(text) {
			break
		}
		text = strings.TrimSpace(text[1:end])
	}
	return text
}

func lastTopLevelKeyword(source, keyword string) int {
	last := -1
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0

	for i := 0; i < len(source); i++ {
		switch source[i] {
		case '\'':
			end, err := skipQuoted(source, i, '\'')
			if err != nil {
				return last
			}
			i = end
		case '"':
			end, err := skipQuoted(source, i, '"')
			if err != nil {
				return last
			}
			i = end
		case '`':
			end, err := skipTemplate(source, i)
			if err != nil {
				return last
			}
			i = end
		case '/':
			if i+1 < len(source) && source[i+1] == '/' {
				i += 2
				for i < len(source) && source[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < len(source) && source[i+1] == '*' {
				i += 2
				for i+1 < len(source) && !(source[i] == '*' && source[i+1] == '/') {
					i++
				}
				if i+1 >= len(source) {
					return last
				}
				i++
				continue
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '<':
			if shouldOpenAngle(source, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		default:
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 && matchesIdentifierAt(source, i, keyword) {
				last = i
				i += len(keyword) - 1
			}
		}
	}

	return last
}

func normalizeSourceTypeText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	text = strings.ReplaceAll(text, "( ", "(")
	text = strings.ReplaceAll(text, " )", ")")
	text = strings.ReplaceAll(text, ",)", ")")
	return text
}

func parseListeners(section SectionReport, logic ParsedLogic) []ParsedListener {
	listeners, _ := parseListenersWithSource(section, logic, "", SourceProperty{}, logic.File, nil)
	return listeners
}

func parseListenersWithSource(
	section SectionReport,
	logic ParsedLogic,
	source string,
	property SourceProperty,
	file string,
	state *buildState,
) ([]ParsedListener, []TypeImport) {
	resolvedExternal := resolveListenerActionReferences(source, file, property, state)
	listeners := make([]ParsedListener, 0, len(section.Members))
	var imports []TypeImport
	for _, member := range section.Members {
		payloadType := "any"
		actionType := "{ type: string; payload: any }"

		if action, ok := findParsedAction(logic.Actions, member.Name); ok {
			payloadType = fallbackType(action.PayloadType, "any")
			actionType = fmt.Sprintf("{ type: %s; payload: %s }", quoteString(actionTypeString(logic, action.Name)), payloadType)
		} else if resolved, ok := resolvedExternal[member.Name]; ok {
			payloadType = resolved.PayloadType
			actionType = resolved.ActionType
			imports = mergeTypeImports(imports, resolved.Imports)
		} else if payload := listenerPayloadTypeFromMember(member); payload != "" {
			payloadType = payload
		}

		listeners = append(listeners, ParsedListener{
			Name:        member.Name,
			PayloadType: payloadType,
			ActionType:  actionType,
		})
	}
	return listeners, imports
}

type resolvedListenerAction struct {
	PayloadType string
	ActionType  string
	Imports     []TypeImport
}

func resolveListenerActionReferences(source, file string, property SourceProperty, state *buildState) map[string]resolvedListenerAction {
	entries := sectionSourceEntries(source, property)
	if len(entries) == 0 {
		return nil
	}

	resolved := map[string]resolvedListenerAction{}
	for _, entry := range entries {
		action, typeName, imports, ok := resolveActionReferenceFromSourceKey(source, file, entry.Name, state)
		if !ok {
			continue
		}
		payloadType := fallbackType(action.PayloadType, "any")
		resolved[typeName] = resolvedListenerAction{
			PayloadType: payloadType,
			ActionType:  fmt.Sprintf("{ type: %s; payload: %s }", quoteString(typeName), payloadType),
			Imports:     imports,
		}
	}
	return resolved
}

func resolveActionReferenceFromSourceKey(source, file, keyText string, state *buildState) (ParsedAction, string, []TypeImport, bool) {
	targetExpr, actionName, ok := parseActionTypeReference(keyText)
	if !ok {
		return ParsedAction{}, "", nil, false
	}
	targetLogic, ok := resolveConnectedLogic(source, file, targetExpr, state)
	if !ok {
		return ParsedAction{}, "", nil, false
	}
	action, ok := findParsedAction(targetLogic.Actions, actionName)
	if !ok {
		return ParsedAction{}, "", nil, false
	}
	return action, actionTypeString(targetLogic, action.Name), filterImportsByTypeTexts(targetLogic.Imports, []string{
		action.FunctionType,
		action.PayloadType,
	}), true
}

func parseActionTypeReference(keyText string) (string, string, bool) {
	text := strings.TrimSpace(keyText)
	if len(text) < 2 || text[0] != '[' || text[len(text)-1] != ']' {
		return "", "", false
	}
	text = strings.TrimSpace(text[1 : len(text)-1])
	const marker = ".actionTypes."
	index := strings.LastIndex(text, marker)
	if index == -1 {
		return "", "", false
	}
	targetExpr := strings.TrimSpace(text[:index])
	actionName := strings.TrimSpace(text[index+len(marker):])
	if targetExpr == "" || actionName == "" {
		return "", "", false
	}
	for i := 0; i < len(actionName); i++ {
		if !isIdentifierPart(actionName[i]) {
			return "", "", false
		}
	}
	return targetExpr, actionName, true
}

func listenerPayloadTypeFromMember(member MemberReport) string {
	parameters, _, ok := splitFunctionType(member.TypeString)
	if !ok {
		return ""
	}
	return strings.TrimSpace(firstParameterType(parameters))
}

func parseSharedListeners(section SectionReport) []ParsedSharedListener {
	listeners := make([]ParsedSharedListener, 0, len(section.Members))
	for _, member := range section.Members {
		payloadType := "any"
		parameters, _, ok := splitFunctionType(member.TypeString)
		if ok {
			if first := firstParameterType(parameters); first != "" {
				payloadType = first
			}
		}
		listeners = append(listeners, ParsedSharedListener{
			Name:        member.Name,
			PayloadType: payloadType,
			ActionType:  fmt.Sprintf("{ type: string; payload: %s }", payloadType),
		})
	}
	return listeners
}

func parseEventNames(section SectionReport) []string {
	names := make([]string, 0, len(section.Members))
	for _, member := range section.Members {
		names = append(names, member.Name)
	}
	sort.Strings(names)
	return names
}

func mergeParsedFields(existing []ParsedField, extra ...ParsedField) []ParsedField {
	indexByName := make(map[string]int, len(existing))
	merged := append([]ParsedField(nil), existing...)
	for index, field := range merged {
		indexByName[field.Name] = index
	}
	for _, field := range extra {
		if index, ok := indexByName[field.Name]; ok {
			if !isAnyLikeType(field.Type) {
				merged[index] = field
			}
			continue
		}
		indexByName[field.Name] = len(merged)
		merged = append(merged, field)
	}
	return merged
}

func findParsedAction(actions []ParsedAction, name string) (ParsedAction, bool) {
	for _, action := range actions {
		if action.Name == name {
			return action, true
		}
	}
	return ParsedAction{}, false
}

func mergeParsedActions(existing []ParsedAction, extra ...ParsedAction) []ParsedAction {
	indexByName := make(map[string]int, len(existing))
	merged := append([]ParsedAction(nil), existing...)
	for index, action := range merged {
		indexByName[action.Name] = index
	}
	for _, action := range extra {
		if index, ok := indexByName[action.Name]; ok {
			merged[index] = action
			continue
		}
		indexByName[action.Name] = len(merged)
		merged = append(merged, action)
	}
	return merged
}

func (s *buildState) close() {
	if s == nil || s.apiClient == nil {
		return
	}
	_ = s.apiClient.Close()
	s.apiClient = nil
	s.apiSnapshot = ""
	s.config = nil
	s.projectByFile = nil
}

func (s *buildState) ensureAPIClient() error {
	if s == nil {
		return fmt.Errorf("build state is nil")
	}
	if s.apiClient != nil {
		return nil
	}
	if s.binaryPath == "" || s.projectDir == "" || s.configFile == "" {
		return fmt.Errorf("build state is not configured for symbol-backed inspection")
	}

	client, err := tsgoapi.Start(s.projectDir, s.binaryPath)
	if err != nil {
		return err
	}
	initializeCtx := context.Background()
	if _, err := client.Initialize(tsgoapi.WithTimeout(initializeCtx, s.timeout)); err != nil {
		_ = client.Close()
		return err
	}
	config, err := client.ParseConfigFile(tsgoapi.WithTimeout(initializeCtx, s.timeout), s.configFile)
	if err != nil {
		_ = client.Close()
		return err
	}
	snapshot, err := client.UpdateSnapshot(tsgoapi.WithTimeout(initializeCtx, s.timeout), s.configFile)
	if err != nil {
		_ = client.Close()
		return err
	}

	s.apiClient = client
	s.apiSnapshot = snapshot.Snapshot
	s.config = config
	if s.projectByFile == nil {
		s.projectByFile = map[string]string{}
	}
	return nil
}

func (s *buildState) projectIDForFile(file string) (string, error) {
	if err := s.ensureAPIClient(); err != nil {
		return "", err
	}
	file = filepath.Clean(file)
	if projectID := s.projectByFile[file]; projectID != "" {
		return projectID, nil
	}

	project, err := s.apiClient.GetDefaultProjectForFile(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		s.apiSnapshot,
		file,
	)
	if err != nil {
		return "", err
	}
	if project == nil || project.ID == "" {
		return "", fmt.Errorf("no default project returned for %s", file)
	}
	s.projectByFile[file] = project.ID
	return project.ID, nil
}

func (s *buildState) loadLogics(file string) ([]ParsedLogic, error) {
	if s == nil || s.binaryPath == "" || s.projectDir == "" || s.configFile == "" {
		return nil, fmt.Errorf("build state is not configured for recursive inspection")
	}

	file = filepath.Clean(file)
	if parsed, ok := s.parsedByFile[file]; ok {
		return parsed, nil
	}
	if s.building[file] {
		return nil, fmt.Errorf("cyclic logic inspection for %s", file)
	}

	s.building[file] = true
	defer delete(s.building, file)

	report, source, err := s.inspectFile(file)
	if err != nil {
		return nil, err
	}

	parsed, err := buildParsedLogicsFromSource(report, source, s)
	if err != nil {
		return nil, err
	}
	s.parsedByFile[file] = parsed
	return parsed, nil
}

func (s *buildState) configFileNames() ([]string, error) {
	if err := s.ensureAPIClient(); err != nil {
		return nil, err
	}
	if s.config == nil {
		return nil, fmt.Errorf("build state has no parsed config")
	}

	root := filepath.Dir(s.configFile)
	files := make([]string, 0, len(s.config.FileNames))
	for _, name := range s.config.FileNames {
		if name == "" {
			continue
		}
		if filepath.IsAbs(name) {
			files = append(files, filepath.Clean(name))
			continue
		}
		files = append(files, filepath.Clean(filepath.Join(root, name)))
	}
	return files, nil
}

func (s *buildState) compilerTypes() []string {
	if s == nil || s.config == nil {
		return nil
	}
	rawTypes, ok := s.config.Options["types"]
	if !ok {
		return nil
	}
	values, ok := rawTypes.([]any)
	if !ok {
		return nil
	}
	types := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if ok && strings.TrimSpace(text) != "" {
			types = append(types, strings.TrimSpace(text))
		}
	}
	return types
}

func (s *buildState) inspectFile(file string) (*Report, string, error) {
	if s == nil {
		return nil, "", fmt.Errorf("build state is nil")
	}
	if err := s.ensureAPIClient(); err != nil {
		return nil, "", err
	}

	file = filepath.Clean(file)
	sourceBytes, err := os.ReadFile(file)
	if err != nil {
		return nil, "", err
	}
	source := string(sourceBytes)

	logics, err := FindLogics(source)
	if err != nil {
		return nil, "", err
	}
	projectID, err := s.projectIDForFile(file)
	if err != nil {
		return nil, "", err
	}

	report := &Report{
		BinaryPath: s.binaryPath,
		ProjectDir: s.projectDir,
		ConfigFile: s.configFile,
		File:       file,
		Snapshot:   s.apiSnapshot,
		Config:     s.config,
	}
	for _, logic := range logics {
		report.Logics = append(report.Logics, inspectLogic(
			context.Background(),
			s.apiClient,
			s.timeout,
			s.apiSnapshot,
			projectID,
			file,
			source,
			logic,
		))
	}
	return report, source, nil
}

type connectedName struct {
	SourceName string
	LocalName  string
}

type connectedReference struct {
	Kind        string
	TargetExpr  string
	TargetStart int
	TargetEnd   int
	Names       []connectedName
}

type connectedTargetReference struct {
	BaseAlias string
	LogicName string
}

type sourceSegment struct {
	Text  string
	Start int
	End   int
}

type importedValueCandidate struct {
	Path         string
	ImportedName string
}

func enrichConnectedSections(source, file string, property SourceProperty, sections map[string]SectionReport, parsed *ParsedLogic, state *buildState) []TypeImport {
	references, err := parseConnectedReferences(source, property)
	if err != nil {
		return nil
	}

	var imports []TypeImport
	for _, reference := range references {
		targetLogic, hasLocalLogic := resolveConnectedLogic(source, file, reference.TargetExpr, state)

		switch reference.Kind {
		case "actions":
			var addedTypeTexts []string
			actionMembers, _ := resolveConnectedSectionMembersBySymbol(source, file, reference, "actions", state)
			actionCreatorMembers, _ := resolveConnectedSectionMembersBySymbol(source, file, reference, "actionCreators", state)
			listenerSection, hasListenerSection := sections["listeners"]
			for _, name := range reference.Names {
				if action, ok := synthesizeConnectedActionFromSymbols(name, actionMembers, actionCreatorMembers); ok {
					if hasListenerSection {
						if member, found := findMemberReport(listenerSection.Members, name.LocalName); found {
							if payload := listenerPayloadTypeFromMember(member); payload != "" && !strings.Contains(payload, "...") && (action.PayloadType == "" || strings.Contains(action.PayloadType, "...")) {
								action.PayloadType = payload
							}
						}
					}
					parsed.Actions = mergeParsedActions(parsed.Actions, action)
					addedTypeTexts = append(addedTypeTexts, action.FunctionType, action.PayloadType)
					continue
				}
				if hasLocalLogic {
					action, found := findParsedAction(targetLogic.Actions, name.SourceName)
					if !found {
						continue
					}
					copied := action
					copied.Name = name.LocalName
					parsed.Actions = mergeParsedActions(parsed.Actions, copied)
					addedTypeTexts = append(addedTypeTexts, copied.FunctionType, copied.PayloadType)
				}
			}
			if hasLocalLogic {
				imports = mergeTypeImports(imports, filterImportsByTypeTexts(targetLogic.Imports, addedTypeTexts))
			}
			if len(addedTypeTexts) > 0 {
				continue
			}

			if !hasListenerSection {
				continue
			}
			for _, name := range reference.Names {
				member, found := findMemberReport(listenerSection.Members, name.LocalName)
				if !found {
					continue
				}
				action, ok := synthesizeConnectedActionFromListener(name.LocalName, member)
				if !ok {
					continue
				}
				parsed.Actions = mergeParsedActions(parsed.Actions, action)
			}

		case "values", "props":
			var addedTypeTexts []string
			valueMembers, _ := resolveConnectedSectionMembersBySymbol(source, file, reference, reference.Kind, state)
			if reference.Kind == "props" && len(valueMembers) == 0 {
				valueMembers, _ = resolveConnectedSectionMembersBySymbol(source, file, reference, "values", state)
			}
			targetFields := mergeParsedFields(targetLogic.Reducers, targetLogic.Selectors...)
			for _, name := range reference.Names {
				if member, found := findMemberReport(valueMembers, name.SourceName); found && strings.TrimSpace(member.TypeString) != "" {
					parsed.Selectors = mergeParsedFields(parsed.Selectors, ParsedField{
						Name: name.LocalName,
						Type: strings.TrimSpace(member.TypeString),
					})
					addedTypeTexts = append(addedTypeTexts, member.TypeString)
					continue
				}
				if hasLocalLogic {
					field, found := findParsedField(targetFields, name.SourceName)
					if !found {
						continue
					}
					copied := field
					copied.Name = name.LocalName
					parsed.Selectors = mergeParsedFields(parsed.Selectors, copied)
					addedTypeTexts = append(addedTypeTexts, copied.Type)
				}
			}
			if hasLocalLogic {
				imports = mergeTypeImports(imports, filterImportsByTypeTexts(targetLogic.Imports, addedTypeTexts))
			}
		}
	}

	return imports
}

func parseConnectedReferences(source string, property SourceProperty) ([]connectedReference, error) {
	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(source, property.ValueStart, property.ValueEnd)
	if err != nil || !ok {
		return nil, err
	}

	properties, err := parseTopLevelProperties(source, objectStart, objectEnd)
	if err != nil {
		return nil, err
	}

	var references []connectedReference
	for _, nested := range properties {
		if nested.Name != "actions" && nested.Name != "values" && nested.Name != "props" {
			continue
		}

		arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(source, nested.ValueStart, nested.ValueEnd)
		if err != nil || !ok {
			continue
		}

		elements, err := splitTopLevelSourceSegments(source, arrayStart+1, arrayEnd)
		if err != nil {
			return nil, err
		}
		for index := 0; index+1 < len(elements); index += 2 {
			names := parseConnectedNames(elements[index+1].Text)
			if len(names) == 0 {
				continue
			}
			references = append(references, connectedReference{
				Kind:        nested.Name,
				TargetExpr:  strings.TrimSpace(elements[index].Text),
				TargetStart: elements[index].Start,
				TargetEnd:   elements[index].End,
				Names:       names,
			})
		}
	}

	return references, nil
}

func parseConnectedNames(expression string) []connectedName {
	text := strings.TrimSpace(expression)
	for {
		if len(text) < 2 || text[0] != '(' {
			break
		}
		end, err := findMatching(text, 0, '(', ')')
		if err != nil || end != len(text)-1 {
			break
		}
		text = strings.TrimSpace(text[1:end])
	}
	if len(text) < 2 || text[0] != '[' || text[len(text)-1] != ']' {
		return nil
	}

	parts, err := splitTopLevelList(text[1 : len(text)-1])
	if err != nil {
		return nil
	}

	names := make([]connectedName, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !isQuotedString(part) {
			continue
		}
		value := unquoteString(part)
		sourceName := value
		localName := value
		if strings.Contains(value, " as ") {
			pieces := strings.SplitN(value, " as ", 2)
			sourceName = strings.TrimSpace(pieces[0])
			localName = strings.TrimSpace(pieces[1])
		}
		if sourceName == "" || localName == "" {
			continue
		}
		names = append(names, connectedName{SourceName: sourceName, LocalName: localName})
	}
	return names
}

func resolveConnectedLogic(source, file, expression string, state *buildState) (ParsedLogic, bool) {
	if state == nil {
		return ParsedLogic{}, false
	}

	target, ok := parseConnectedTargetReference(expression)
	if !ok {
		return ParsedLogic{}, false
	}

	if candidate, ok := parseNamedValueImports(source)[target.BaseAlias]; ok {
		resolvedFile, ok := resolveLocalImportFile(file, candidate.Path)
		if !ok {
			return ParsedLogic{}, false
		}

		logics, err := state.loadLogics(resolvedFile)
		if err != nil {
			return ParsedLogic{}, false
		}
		return findConnectedLogic(logics, target.LogicName, candidate.ImportedName, target.BaseAlias)
	}

	if candidate, ok := parseDefaultValueImports(source)[target.BaseAlias]; ok {
		resolvedFile, ok := resolveLocalImportFile(file, candidate.Path)
		if !ok {
			return ParsedLogic{}, false
		}

		logics, err := state.loadLogics(resolvedFile)
		if err != nil {
			return ParsedLogic{}, false
		}
		return findConnectedLogic(logics, target.LogicName, target.BaseAlias)
	}

	if importPath, ok := parseNamespaceValueImports(source)[target.BaseAlias]; ok {
		resolvedFile, ok := resolveLocalImportFile(file, importPath)
		if !ok {
			return ParsedLogic{}, false
		}

		logics, err := state.loadLogics(resolvedFile)
		if err != nil {
			return ParsedLogic{}, false
		}
		return findConnectedLogic(logics, target.LogicName)
	}

	return ParsedLogic{}, false
}

func resolveConnectedSectionMembersBySymbol(
	source string,
	file string,
	reference connectedReference,
	sectionName string,
	state *buildState,
) ([]MemberReport, bool) {
	if state == nil || sectionName == "" {
		return nil, false
	}
	if err := state.ensureAPIClient(); err != nil {
		return nil, false
	}
	projectID, err := state.projectIDForFile(file)
	if err != nil {
		return nil, false
	}

	for _, position := range connectedTargetProbePositions(source, reference) {
		targetType, err := state.apiClient.GetTypeAtPosition(
			tsgoapi.WithTimeout(context.Background(), state.timeout),
			state.apiSnapshot,
			projectID,
			file,
			position,
		)
		if err != nil || targetType == nil {
			continue
		}

		sectionType, ok := connectedPropertyType(state.apiClient, state.apiSnapshot, projectID, state.timeout, targetType.ID, sectionName)
		if !ok {
			continue
		}
		members, ok := connectedTypeMembers(state.apiClient, state.apiSnapshot, projectID, state.timeout, sectionType.ID)
		if ok {
			return members, true
		}
	}

	return nil, false
}

func connectedTargetProbePositions(source string, reference connectedReference) []int {
	positions := make([]int, 0, 2)
	appendPosition := func(position int) {
		if position < reference.TargetStart || position >= reference.TargetEnd {
			return
		}
		for _, existing := range positions {
			if existing == position {
				return
			}
		}
		positions = append(positions, position)
	}

	if target, ok := parseConnectedTargetReference(reference.TargetExpr); ok {
		if position, ok := findIdentifierPosition(source, reference.TargetStart, reference.TargetEnd, target.BaseAlias); ok {
			appendPosition(position)
		}
		if target.LogicName != "" && target.LogicName != target.BaseAlias {
			if position, ok := findLastIdentifierPosition(source, reference.TargetStart, reference.TargetEnd, target.LogicName); ok {
				appendPosition(position)
			}
		}
	}
	if tail := trimExpressionEnd(source, reference.TargetEnd); tail > reference.TargetStart {
		if position, ok := findLastIdentifierPosition(source, reference.TargetStart, tail, ""); ok {
			appendPosition(position)
		} else {
			appendPosition(tail - 1)
		}
	}
	return positions
}

func findIdentifierPosition(source string, start, end int, name string) (int, bool) {
	for i := start; i+len(name) <= end; i++ {
		if matchesIdentifierAt(source, i, name) {
			return i, true
		}
	}
	return 0, false
}

func findLastIdentifierPosition(source string, start, end int, name string) (int, bool) {
	if start < 0 {
		start = 0
	}
	if end > len(source) {
		end = len(source)
	}
	if start >= end {
		return 0, false
	}

	for i := end - 1; i >= start; i-- {
		if !isIdentifierPart(source[i]) {
			continue
		}
		identifierEnd := i + 1
		identifierStart := i
		for identifierStart > start && isIdentifierPart(source[identifierStart-1]) {
			identifierStart--
		}
		identifier := source[identifierStart:identifierEnd]
		if name == "" || identifier == name {
			return identifierStart, true
		}
		i = identifierStart
	}
	return 0, false
}

func connectedPropertyType(
	client *tsgoapi.Client,
	snapshot string,
	projectID string,
	timeout time.Duration,
	typeID string,
	propertyName string,
) (*tsgoapi.TypeResponse, bool) {
	properties, err := client.GetPropertiesOfType(tsgoapi.WithTimeout(context.Background(), timeout), snapshot, projectID, typeID)
	if err != nil {
		return nil, false
	}
	for _, property := range properties {
		if property == nil || property.Name != propertyName {
			continue
		}
		propertyType, err := client.GetTypeOfSymbol(tsgoapi.WithTimeout(context.Background(), timeout), snapshot, projectID, property.ID)
		if err == nil && propertyType != nil {
			return propertyType, true
		}
	}
	return nil, false
}

func connectedTypeMembers(
	client *tsgoapi.Client,
	snapshot string,
	projectID string,
	timeout time.Duration,
	typeID string,
) ([]MemberReport, bool) {
	properties, err := client.GetPropertiesOfType(tsgoapi.WithTimeout(context.Background(), timeout), snapshot, projectID, typeID)
	if err != nil || len(properties) == 0 {
		return nil, false
	}

	members := make([]MemberReport, 0, len(properties))
	for _, property := range properties {
		if property == nil || property.Name == "" {
			continue
		}
		member := MemberReport{Name: property.Name}
		propertyType, err := client.GetTypeOfSymbol(tsgoapi.WithTimeout(context.Background(), timeout), snapshot, projectID, property.ID)
		if err == nil && propertyType != nil {
			member.TypeString = safeTypeString(context.Background(), client, timeout, snapshot, projectID, propertyType.ID)
			if printed, err := client.PrintTypeNode(tsgoapi.WithTimeout(context.Background(), timeout), snapshot, projectID, propertyType.ID); err == nil {
				member.PrintedTypeNode = printed
			}
			signatures, err := client.GetSignaturesOfType(tsgoapi.WithTimeout(context.Background(), timeout), snapshot, projectID, propertyType.ID)
			if err == nil && len(signatures) > 0 {
				member.SignatureCount = len(signatures)
				returnType, err := client.GetReturnTypeOfSignature(
					tsgoapi.WithTimeout(context.Background(), timeout),
					snapshot,
					projectID,
					signatures[0].ID,
				)
				if err == nil && returnType != nil {
					member.ReturnTypeString = safeTypeString(context.Background(), client, timeout, snapshot, projectID, returnType.ID)
					if printed, err := client.PrintTypeNode(tsgoapi.WithTimeout(context.Background(), timeout), snapshot, projectID, returnType.ID); err == nil {
						member.PrintedReturnTypeNode = printed
					}
				}
			}
		}
		members = append(members, member)
	}
	sort.Slice(members, func(i, j int) bool {
		return members[i].Name < members[j].Name
	})
	return members, len(members) > 0
}

func synthesizeConnectedActionFromSymbols(name connectedName, actionMembers, actionCreatorMembers []MemberReport) (ParsedAction, bool) {
	actionMember, hasActionMember := findMemberReport(actionMembers, name.SourceName)
	creatorMember, hasCreatorMember := findMemberReport(actionCreatorMembers, name.SourceName)
	if !hasActionMember && !hasCreatorMember {
		return ParsedAction{}, false
	}

	functionType := strings.TrimSpace(actionMember.TypeString)
	if functionType == "" {
		functionType = strings.TrimSpace(creatorMember.TypeString)
	}
	if functionType == "" {
		return ParsedAction{}, false
	}

	payloadType := connectedActionPayloadType(creatorMember)
	if payloadType == "" {
		payloadType = "any"
	}

	return ParsedAction{
		Name:         name.LocalName,
		FunctionType: functionType,
		PayloadType:  payloadType,
	}, true
}

func connectedActionPayloadType(member MemberReport) string {
	if member.Name == "" {
		return ""
	}
	returnType := strings.TrimSpace(preferredMemberReturnTypeText(member))
	if returnType == "" || strings.Contains(returnType, "...") {
		_, returnType, _ = splitFunctionType(preferredMemberTypeText(member))
	}
	if returnType == "" {
		return ""
	}
	properties, ok := parseObjectTypeMembers(returnType)
	if !ok {
		return ""
	}
	return strings.TrimSpace(properties["payload"])
}

func parseConnectedTargetReference(expression string) (connectedTargetReference, bool) {
	text := strings.TrimSpace(expression)
	for {
		changed := false
		if len(text) >= 2 && text[0] == '(' {
			end, err := findMatching(text, 0, '(', ')')
			if err == nil && end == len(text)-1 {
				text = strings.TrimSpace(text[1:end])
				changed = true
			}
		}
		if stripped, ok := stripTrailingCallExpression(text); ok {
			text = stripped
			changed = true
		}
		if !changed {
			break
		}
	}
	if text == "" {
		return connectedTargetReference{}, false
	}

	parts := strings.Split(text, ".")
	if len(parts) == 0 {
		return connectedTargetReference{}, false
	}
	for _, part := range parts {
		if part == "" {
			return connectedTargetReference{}, false
		}
		for i := 0; i < len(part); i++ {
			if !isIdentifierPart(part[i]) {
				return connectedTargetReference{}, false
			}
		}
	}

	target := connectedTargetReference{
		BaseAlias: parts[0],
		LogicName: parts[len(parts)-1],
	}
	if target.LogicName == "" {
		target.LogicName = target.BaseAlias
	}
	return target, true
}

func stripTrailingCallExpression(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasSuffix(text, ")") {
		return text, false
	}

	depth := 0
	for i := len(text) - 1; i >= 0; i-- {
		switch text[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				prefix := strings.TrimSpace(text[:i])
				if prefix == "" {
					return text, false
				}
				return prefix, true
			}
		}
	}
	return text, false
}

func findConnectedLogic(logics []ParsedLogic, names ...string) (ParsedLogic, bool) {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		for _, logic := range logics {
			if logic.Name == name {
				return logic, true
			}
		}
	}
	if len(logics) == 1 {
		return logics[0], true
	}
	return ParsedLogic{}, false
}

func synthesizeConnectedActionFromListener(name string, member MemberReport) (ParsedAction, bool) {
	parameters, _, ok := splitFunctionType(member.TypeString)
	if !ok {
		return ParsedAction{}, false
	}

	firstParameter := strings.TrimSpace(firstParameterText(parameters))
	payloadType := strings.TrimSpace(firstParameterType(parameters))
	if payloadType == "" {
		payloadType = "any"
	}

	functionType := "() => " + payloadType
	if firstParameter != "" {
		functionType = "(" + firstParameter + ") => " + payloadType
	}

	return ParsedAction{
		Name:         name,
		FunctionType: functionType,
		PayloadType:  payloadType,
	}, true
}

func findMemberReport(members []MemberReport, name string) (MemberReport, bool) {
	for _, member := range members {
		if member.Name == name {
			return member, true
		}
	}
	return MemberReport{}, false
}

func findParsedField(fields []ParsedField, name string) (ParsedField, bool) {
	for _, field := range fields {
		if field.Name == name {
			return field, true
		}
	}
	return ParsedField{}, false
}

func parseReducerStateType(typeText string) (string, bool) {
	text := strings.TrimSpace(typeText)
	if text == "" {
		return "", false
	}
	if !strings.HasPrefix(text, "[") {
		if strings.HasPrefix(text, "{") {
			return "any", true
		}
		return text, true
	}

	inner := strings.TrimSpace(text[1:])
	if strings.HasSuffix(inner, "]") {
		inner = strings.TrimSpace(inner[:len(inner)-1])
	}
	parts, err := splitTopLevelList(inner)
	if err != nil || len(parts) == 0 {
		return "", false
	}
	return strings.TrimSpace(parts[0]), true
}

func parseSelectorReturnType(typeText string) (string, bool) {
	text := strings.TrimSpace(typeText)
	if text == "" || strings.Contains(text, "...") {
		return "", false
	}
	if strings.HasPrefix(text, "[") {
		inner := strings.TrimSpace(text[1:])
		if strings.HasSuffix(inner, "]") {
			inner = strings.TrimSpace(inner[:len(inner)-1])
		}
		parts, err := splitTopLevelList(inner)
		if err != nil || len(parts) == 0 {
			return "", false
		}
		text = strings.TrimSpace(parts[len(parts)-1])
	}

	_, returnType, ok := splitFunctionType(text)
	if !ok {
		return "", false
	}
	return returnType, true
}

func selectorReturnTypeNeedsRecovery(member MemberReport) bool {
	if strings.TrimSpace(member.TypeString) == "" && strings.TrimSpace(member.ReturnTypeString) == "" {
		return false
	}
	if returnType := strings.TrimSpace(member.ReturnTypeString); returnType != "" && !strings.Contains(returnType, "...") && !isAnyLikeType(returnType) {
		return false
	}
	if returnType, ok := parseSelectorReturnType(member.TypeString); ok && !isAnyLikeType(returnType) {
		return false
	}
	return true
}

func parseObjectTypeMembers(typeText string) (map[string]string, bool) {
	text := strings.TrimSpace(typeText)
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return nil, false
	}

	body := strings.TrimSpace(text[1 : len(text)-1])
	if body == "" {
		return map[string]string{}, true
	}

	entries, err := splitTopLevelTypeMembers(body)
	if err != nil {
		return nil, false
	}

	properties := make(map[string]string, len(entries))
	for _, entry := range entries {
		name, value, ok := splitTopLevelProperty(entry)
		if !ok {
			continue
		}
		properties[name] = value
	}
	return properties, true
}

func splitFunctionType(typeText string) (string, string, bool) {
	text := strings.TrimSpace(typeText)
	arrowIndex, ok, err := findTopLevelArrow(text, 0, len(text))
	if err != nil || !ok {
		return "", "", false
	}
	parameters := strings.TrimSpace(text[:arrowIndex])
	returnType := strings.TrimSpace(text[arrowIndex+2:])
	if parameters == "" || returnType == "" {
		return "", "", false
	}
	return parameters, returnType, true
}

func splitTopLevelTypeMembers(source string) ([]string, error) {
	var members []string
	start := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0

	for i := 0; i < len(source); i++ {
		switch source[i] {
		case '\'':
			end, err := skipQuoted(source, i, '\'')
			if err != nil {
				return nil, err
			}
			i = end
		case '"':
			end, err := skipQuoted(source, i, '"')
			if err != nil {
				return nil, err
			}
			i = end
		case '`':
			end, err := skipTemplate(source, i)
			if err != nil {
				return nil, err
			}
			i = end
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '<':
			if shouldOpenAngle(source, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case ';':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				member := strings.TrimSpace(source[start:i])
				if member != "" {
					members = append(members, member)
				}
				start = i + 1
			}
		case '\n':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				member := strings.TrimSpace(source[start:i])
				if member != "" {
					members = append(members, member)
				}
				start = i + 1
			}
		}
	}

	last := strings.TrimSpace(source[start:])
	if last != "" {
		members = append(members, last)
	}
	return members, nil
}

func splitTopLevelProperty(entry string) (string, string, bool) {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0

	for i := 0; i < len(entry); i++ {
		switch entry[i] {
		case '\'':
			end, err := skipQuoted(entry, i, '\'')
			if err != nil {
				return "", "", false
			}
			i = end
		case '"':
			end, err := skipQuoted(entry, i, '"')
			if err != nil {
				return "", "", false
			}
			i = end
		case '`':
			end, err := skipTemplate(entry, i)
			if err != nil {
				return "", "", false
			}
			i = end
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '<':
			if shouldOpenAngle(entry, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case ':':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				name := strings.TrimSpace(entry[:i])
				value := strings.TrimSpace(entry[i+1:])
				name = strings.TrimSuffix(name, "?")
				name = strings.Trim(name, `"'`)
				if name == "" || value == "" {
					return "", "", false
				}
				return name, value, true
			}
		}
	}

	return "", "", false
}

func firstParameterType(parameters string) string {
	params, ok := parseFunctionParameters(parameters)
	if !ok || len(params) == 0 {
		return ""
	}
	return params[0].Type
}

func firstParameterText(parameters string) string {
	params, ok := parseFunctionParameters(parameters)
	if !ok || len(params) == 0 {
		return ""
	}
	return params[0].Text
}

type parsedParameter struct {
	Text string
	Type string
}

func parseFunctionParameters(parameters string) ([]parsedParameter, bool) {
	text := strings.TrimSpace(parameters)
	if len(text) < 2 || text[0] != '(' || text[len(text)-1] != ')' {
		return nil, false
	}

	parts, err := splitTopLevelList(text[1 : len(text)-1])
	if err != nil {
		return nil, false
	}
	params := make([]parsedParameter, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		_, typeText, ok := splitTopLevelProperty(part)
		if !ok {
			continue
		}
		params = append(params, parsedParameter{Text: part, Type: typeText})
	}
	return params, true
}

func unwrapPromiseType(typeText string) string {
	text := strings.TrimSpace(typeText)
	if !strings.HasPrefix(text, "Promise<") || !strings.HasSuffix(text, ">") {
		return text
	}
	inner := strings.TrimSpace(text[len("Promise<") : len(text)-1])
	if inner == "" {
		return text
	}
	return inner
}

func isAnyLikeType(typeText string) bool {
	text := strings.TrimSpace(typeText)
	return text == "" || text == "any" || text == "unknown"
}

func parsePathExpression(expression string) ([]string, bool) {
	text := strings.TrimSpace(expression[:trimExpressionEnd(expression, len(expression))])
	if text == "" {
		return nil, false
	}

	for {
		text = strings.TrimSpace(text)
		if len(text) < 2 || text[0] != '(' {
			break
		}
		end, err := findMatching(text, 0, '(', ')')
		if err != nil || end != len(text)-1 {
			break
		}
		text = text[1:end]
	}

	if arrowIndex, ok, err := findTopLevelArrow(text, 0, len(text)); err == nil && ok {
		text = strings.TrimSpace(text[arrowIndex+2:])
		for {
			if len(text) >= 2 && text[0] == '(' {
				end, err := findMatching(text, 0, '(', ')')
				if err != nil || end != len(text)-1 {
					break
				}
				text = strings.TrimSpace(text[1:end])
				continue
			}
			break
		}
	}

	if text == "" || text[0] != '[' {
		return nil, false
	}
	end, err := findMatching(text, 0, '[', ']')
	if err != nil || end != len(text)-1 {
		return nil, false
	}

	parts, err := splitTopLevelList(text[1:end])
	if err != nil {
		return nil, false
	}

	path := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch {
		case isQuotedString(part):
			path = append(path, unquoteString(part))
		case part != "":
			path = append(path, "*")
		}
	}
	return path, len(path) > 0
}

func splitTopLevelList(source string) ([]string, error) {
	var parts []string
	start := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0

	for i := 0; i < len(source); i++ {
		switch source[i] {
		case '\'':
			end, err := skipQuoted(source, i, '\'')
			if err != nil {
				return nil, err
			}
			i = end
		case '"':
			end, err := skipQuoted(source, i, '"')
			if err != nil {
				return nil, err
			}
			i = end
		case '`':
			end, err := skipTemplate(source, i)
			if err != nil {
				return nil, err
			}
			i = end
		case '/':
			if i+1 < len(source) && source[i+1] == '/' {
				i += 2
				for i < len(source) && source[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < len(source) && source[i+1] == '*' {
				i += 2
				for i+1 < len(source) && !(source[i] == '*' && source[i+1] == '/') {
					i++
				}
				if i+1 >= len(source) {
					return nil, fmt.Errorf("unterminated block comment")
				}
				i++
				continue
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '<':
			if shouldOpenAngle(source, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case ',':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				parts = append(parts, strings.TrimSpace(source[start:i]))
				start = i + 1
			}
		}
	}

	last := strings.TrimSpace(source[start:])
	if last != "" {
		parts = append(parts, last)
	}
	return parts, nil
}

func splitTopLevelSourceSegments(source string, start, end int) ([]sourceSegment, error) {
	var parts []sourceSegment
	segmentStart := start
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0

	appendSegment := func(rawStart, rawEnd int) {
		trimmedStart := skipTrivia(source, rawStart)
		trimmedEnd := trimExpressionEnd(source, rawEnd)
		if trimmedEnd <= trimmedStart {
			return
		}
		parts = append(parts, sourceSegment{
			Text:  source[trimmedStart:trimmedEnd],
			Start: trimmedStart,
			End:   trimmedEnd,
		})
	}

	for i := start; i < end; i++ {
		switch source[i] {
		case '\'':
			skip, err := skipQuoted(source, i, '\'')
			if err != nil {
				return nil, err
			}
			i = skip
		case '"':
			skip, err := skipQuoted(source, i, '"')
			if err != nil {
				return nil, err
			}
			i = skip
		case '`':
			skip, err := skipTemplate(source, i)
			if err != nil {
				return nil, err
			}
			i = skip
		case '/':
			if i+1 < end && source[i+1] == '/' {
				i += 2
				for i < end && source[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < end && source[i+1] == '*' {
				i += 2
				for i+1 < end && !(source[i] == '*' && source[i+1] == '/') {
					i++
				}
				if i+1 >= end {
					return nil, fmt.Errorf("unterminated block comment")
				}
				i++
				continue
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '<':
			if shouldOpenAngle(source, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case ',':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				appendSegment(segmentStart, i)
				segmentStart = i + 1
			}
		}
	}

	appendSegment(segmentStart, end)
	return parts, nil
}

func splitTopLevelUnion(source string) ([]string, error) {
	var parts []string
	start := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0

	for i := 0; i < len(source); i++ {
		switch source[i] {
		case '\'':
			end, err := skipQuoted(source, i, '\'')
			if err != nil {
				return nil, err
			}
			i = end
		case '"':
			end, err := skipQuoted(source, i, '"')
			if err != nil {
				return nil, err
			}
			i = end
		case '`':
			end, err := skipTemplate(source, i)
			if err != nil {
				return nil, err
			}
			i = end
		case '/':
			if i+1 < len(source) && source[i+1] == '/' {
				i += 2
				for i < len(source) && source[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < len(source) && source[i+1] == '*' {
				i += 2
				for i+1 < len(source) && !(source[i] == '*' && source[i+1] == '/') {
					i++
				}
				if i+1 >= len(source) {
					return nil, fmt.Errorf("unterminated block comment")
				}
				i++
				continue
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '<':
			if shouldOpenAngle(source, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '|':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				parts = append(parts, strings.TrimSpace(source[start:i]))
				start = i + 1
			}
		}
	}

	last := strings.TrimSpace(source[start:])
	if last != "" {
		parts = append(parts, last)
	}
	return parts, nil
}

func defaultLogicPath(projectDir, file string) []string {
	relative, err := filepath.Rel(projectDir, file)
	if err != nil {
		relative = filepath.Base(file)
	}
	relative = filepath.ToSlash(relative)
	relative = strings.TrimPrefix(relative, "./")
	relative = strings.TrimSuffix(relative, filepath.Ext(relative))
	if relative == "" {
		return []string{"logic"}
	}
	return strings.Split(relative, "/")
}

func isQuotedString(text string) bool {
	return len(text) >= 2 && ((text[0] == '\'' && text[len(text)-1] == '\'') || (text[0] == '"' && text[len(text)-1] == '"') || (text[0] == '`' && text[len(text)-1] == '`'))
}

func unquoteString(text string) string {
	if len(text) < 2 {
		return text
	}
	if text[0] == '`' && text[len(text)-1] == '`' {
		return text[1 : len(text)-1]
	}
	quoted := text
	if text[0] == '\'' {
		quoted = `"` + strings.ReplaceAll(strings.ReplaceAll(text[1:len(text)-1], `\`, `\\`), `"`, `\"`) + `"`
	}
	unquoted, err := strconv.Unquote(quoted)
	if err != nil {
		return text[1 : len(text)-1]
	}
	return unquoted
}

type importCandidate struct {
	Path string
	Name string
}

func mergeTypeImports(groups ...[]TypeImport) []TypeImport {
	grouped := map[string]map[string]bool{}
	for _, imports := range groups {
		for _, item := range imports {
			if grouped[item.Path] == nil {
				grouped[item.Path] = map[string]bool{}
			}
			for _, name := range item.Names {
				grouped[item.Path][name] = true
			}
		}
	}

	paths := make([]string, 0, len(grouped))
	for path := range grouped {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	imports := make([]TypeImport, 0, len(paths))
	for _, path := range paths {
		names := make([]string, 0, len(grouped[path]))
		for name := range grouped[path] {
			names = append(names, name)
		}
		sort.Strings(names)
		imports = append(imports, TypeImport{Path: path, Names: names})
	}
	return imports
}

func collectTypeImports(source, file string, logic ParsedLogic) []TypeImport {
	importsByAlias := map[string]importCandidate{}
	for alias, candidate := range parseNamedImports(source) {
		importsByAlias[alias] = candidate
	}
	for alias, candidate := range parseDefaultImports(source) {
		importsByAlias[alias] = candidate
	}
	for alias, candidate := range parseNamespaceImports(source) {
		importsByAlias[alias] = candidate
	}
	for name, candidate := range parseLocalExportedTypes(file, source) {
		importsByAlias[name] = candidate
	}
	relativeImportPaths := parseRelativeImportPaths(source)
	packageImportPaths := parsePackageImportPaths(source)
	exportCache := map[string]map[string]bool{}
	packageExportCache := map[string]map[string]importCandidate{}
	coveredIdentifiers := existingImportReferenceNames(logic.Imports)
	usedReferences := collectUsedTypeReferences(collectUsedTypeTexts(logic))

	grouped := map[string]map[string]bool{}
	for identifier := range usedReferences.QualifiedOwners {
		if coveredIdentifiers[identifier] {
			continue
		}
		candidate, ok := importsByAlias[identifier]
		if !ok || candidate.Path == "" || candidate.Name == "" {
			continue
		}
		if grouped[candidate.Path] == nil {
			grouped[candidate.Path] = map[string]bool{}
		}
		grouped[candidate.Path][candidate.Name] = true
	}
	for identifier := range usedReferences.BareIdentifiers {
		if coveredIdentifiers[identifier] {
			continue
		}
		candidate, ok := importsByAlias[identifier]
		if !ok && len(relativeImportPaths) > 0 {
			candidate, ok = resolveRelativeExportCandidate(file, relativeImportPaths, identifier, exportCache)
		}
		if !ok && len(packageImportPaths) > 0 {
			candidate, ok = resolvePackageExportCandidate(file, packageImportPaths, identifier, packageExportCache)
		}
		if !ok || candidate.Path == "" || candidate.Name == "" {
			continue
		}
		if grouped[candidate.Path] == nil {
			grouped[candidate.Path] = map[string]bool{}
		}
		grouped[candidate.Path][candidate.Name] = true
	}

	paths := make([]string, 0, len(grouped))
	for path := range grouped {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	imports := make([]TypeImport, 0, len(paths))
	for _, path := range paths {
		names := make([]string, 0, len(grouped[path]))
		for name := range grouped[path] {
			names = append(names, name)
		}
		sort.Strings(names)
		imports = append(imports, TypeImport{Path: path, Names: names})
	}
	return imports
}

func existingImportReferenceNames(imports []TypeImport) map[string]bool {
	covered := map[string]bool{}
	for _, item := range imports {
		for _, name := range item.Names {
			referenceName := importReferenceName(name)
			if referenceName != "" {
				covered[referenceName] = true
			}
		}
	}
	return covered
}

func collectUsedTypeTexts(logic ParsedLogic) []string {
	var texts []string
	if logic.PropsType != "" {
		texts = append(texts, logic.PropsType)
	}
	if logic.KeyType != "" {
		texts = append(texts, logic.KeyType)
	}
	for _, action := range logic.Actions {
		texts = append(texts, action.FunctionType, action.PayloadType)
	}
	for _, reducer := range logic.Reducers {
		texts = append(texts, reducer.Type)
	}
	for _, selector := range logic.Selectors {
		texts = append(texts, selector.Type)
	}
	for _, listener := range logic.Listeners {
		texts = append(texts, listener.PayloadType, listener.ActionType)
	}
	for _, listener := range logic.SharedListeners {
		texts = append(texts, listener.PayloadType, listener.ActionType)
	}
	if logic.CustomType != "" {
		texts = append(texts, logic.CustomType)
	}
	if logic.ExtraInputForm != "" {
		texts = append(texts, logic.ExtraInputForm)
	}
	return texts
}

func parseNamedImports(source string) map[string]importCandidate {
	result := map[string]importCandidate{}
	matches := importClausePattern.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		for localName, candidate := range parseImportSpecifiers(match[1], match[2]) {
			importName := candidate.ImportedName
			if localName != candidate.ImportedName {
				importName = candidate.ImportedName + " as " + localName
			}
			result[localName] = importCandidate{Path: candidate.Path, Name: importName}
		}
	}
	mixedMatches := importDefaultNamedPattern.FindAllStringSubmatch(source, -1)
	for _, match := range mixedMatches {
		for localName, candidate := range parseImportSpecifiers(match[2], match[3]) {
			importName := candidate.ImportedName
			if localName != candidate.ImportedName {
				importName = candidate.ImportedName + " as " + localName
			}
			result[localName] = importCandidate{Path: candidate.Path, Name: importName}
		}
	}
	return result
}

func parseNamedValueImports(source string) map[string]importedValueCandidate {
	result := map[string]importedValueCandidate{}
	matches := importClausePattern.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		for localName, candidate := range parseImportSpecifiers(match[1], match[2]) {
			result[localName] = candidate
		}
	}
	mixedMatches := importDefaultNamedPattern.FindAllStringSubmatch(source, -1)
	for _, match := range mixedMatches {
		for localName, candidate := range parseImportSpecifiers(match[2], match[3]) {
			result[localName] = candidate
		}
	}
	return result
}

func parseDefaultImports(source string) map[string]importCandidate {
	result := map[string]importCandidate{}
	for _, match := range importDefaultPattern.FindAllStringSubmatch(source, -1) {
		alias := strings.TrimSpace(match[1])
		importPath := strings.TrimSpace(match[2])
		if alias == "" || importPath == "" {
			continue
		}
		result[alias] = importCandidate{Path: importPath, Name: "default as " + alias}
	}
	for _, match := range importDefaultNamedPattern.FindAllStringSubmatch(source, -1) {
		alias := strings.TrimSpace(match[1])
		importPath := strings.TrimSpace(match[3])
		if alias == "" || importPath == "" {
			continue
		}
		result[alias] = importCandidate{Path: importPath, Name: "default as " + alias}
	}
	return result
}

func parseDefaultValueImports(source string) map[string]importedValueCandidate {
	result := map[string]importedValueCandidate{}
	for _, match := range importDefaultPattern.FindAllStringSubmatch(source, -1) {
		alias := strings.TrimSpace(match[1])
		importPath := strings.TrimSpace(match[2])
		if alias == "" || importPath == "" {
			continue
		}
		result[alias] = importedValueCandidate{Path: importPath, ImportedName: "default"}
	}
	for _, match := range importDefaultNamedPattern.FindAllStringSubmatch(source, -1) {
		alias := strings.TrimSpace(match[1])
		importPath := strings.TrimSpace(match[3])
		if alias == "" || importPath == "" {
			continue
		}
		result[alias] = importedValueCandidate{Path: importPath, ImportedName: "default"}
	}
	return result
}

func parseNamespaceImports(source string) map[string]importCandidate {
	result := map[string]importCandidate{}
	matches := importNamespacePattern.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		alias := strings.TrimSpace(match[1])
		importPath := strings.TrimSpace(match[2])
		if alias == "" || importPath == "" {
			continue
		}
		result[alias] = importCandidate{Path: importPath, Name: "* as " + alias}
	}
	return result
}

func parseNamespaceValueImports(source string) map[string]string {
	result := map[string]string{}
	matches := importNamespacePattern.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		alias := strings.TrimSpace(match[1])
		importPath := strings.TrimSpace(match[2])
		if alias == "" || importPath == "" {
			continue
		}
		result[alias] = importPath
	}
	return result
}

func parseImportSpecifiers(specifiersText, importPath string) map[string]importedValueCandidate {
	result := map[string]importedValueCandidate{}
	specifiers := strings.Split(specifiersText, ",")
	for _, specifier := range specifiers {
		specifier = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(specifier), "type "))
		if specifier == "" {
			continue
		}
		parts := strings.Split(specifier, " as ")
		switch len(parts) {
		case 1:
			name := strings.TrimSpace(parts[0])
			result[name] = importedValueCandidate{Path: importPath, ImportedName: name}
		case 2:
			original := strings.TrimSpace(parts[0])
			alias := strings.TrimSpace(parts[1])
			result[alias] = importedValueCandidate{Path: importPath, ImportedName: original}
		}
	}
	return result
}

func parseLocalExportedTypes(file, source string) map[string]importCandidate {
	result := map[string]importCandidate{}
	importPath := sourceImportPath(file)
	for name := range exportedTypeNames(file, source, map[string]map[string]bool{}) {
		result[name] = importCandidate{Path: importPath, Name: name}
	}
	return result
}

func parseRelativeImportPaths(source string) []string {
	seen := map[string]bool{}
	var paths []string
	matches := importClausePattern.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		importPath := strings.TrimSpace(match[2])
		appendImportPath(&paths, seen, importPath, true)
	}
	mixedMatches := importDefaultNamedPattern.FindAllStringSubmatch(source, -1)
	for _, match := range mixedMatches {
		importPath := strings.TrimSpace(match[3])
		appendImportPath(&paths, seen, importPath, true)
	}
	defaultMatches := importDefaultPattern.FindAllStringSubmatch(source, -1)
	for _, match := range defaultMatches {
		importPath := strings.TrimSpace(match[2])
		appendImportPath(&paths, seen, importPath, true)
	}
	namespaceMatches := importNamespacePattern.FindAllStringSubmatch(source, -1)
	for _, match := range namespaceMatches {
		importPath := strings.TrimSpace(match[2])
		appendImportPath(&paths, seen, importPath, true)
	}
	return paths
}

func parsePackageImportPaths(source string) []string {
	seen := map[string]bool{}
	var paths []string
	matches := importClausePattern.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		importPath := strings.TrimSpace(match[2])
		appendImportPath(&paths, seen, importPath, false)
	}
	mixedMatches := importDefaultNamedPattern.FindAllStringSubmatch(source, -1)
	for _, match := range mixedMatches {
		importPath := strings.TrimSpace(match[3])
		appendImportPath(&paths, seen, importPath, false)
	}
	defaultMatches := importDefaultPattern.FindAllStringSubmatch(source, -1)
	for _, match := range defaultMatches {
		importPath := strings.TrimSpace(match[2])
		appendImportPath(&paths, seen, importPath, false)
	}
	namespaceMatches := importNamespacePattern.FindAllStringSubmatch(source, -1)
	for _, match := range namespaceMatches {
		importPath := strings.TrimSpace(match[2])
		appendImportPath(&paths, seen, importPath, false)
	}
	return paths
}

func appendImportPath(target *[]string, seen map[string]bool, importPath string, relative bool) {
	if importPath == "" {
		return
	}
	if relative {
		if !strings.HasPrefix(importPath, ".") {
			return
		}
	} else if strings.HasPrefix(importPath, ".") {
		return
	}
	if seen[importPath] {
		return
	}
	seen[importPath] = true
	*target = append(*target, importPath)
}

func resolveRelativeExportCandidate(file string, importPaths []string, identifier string, cache map[string]map[string]bool) (importCandidate, bool) {
	matches := make([]importCandidate, 0, 1)
	for _, importPath := range importPaths {
		resolvedFile, ok := resolveLocalImportFile(file, importPath)
		if !ok {
			continue
		}
		if !exportedTypeNames(resolvedFile, "", cache)[identifier] {
			continue
		}
		matches = append(matches, importCandidate{Path: importPath, Name: identifier})
		if len(matches) > 1 {
			return importCandidate{}, false
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return importCandidate{}, false
}

func resolvePackageExportCandidate(file string, importPaths []string, identifier string, cache map[string]map[string]importCandidate) (importCandidate, bool) {
	matches := make([]importCandidate, 0, 1)
	seen := map[string]bool{}
	for _, importPath := range importPaths {
		candidates, ok := cache[importPath]
		if !ok {
			candidates = packageExportCandidates(file, importPath)
			cache[importPath] = candidates
		}
		candidate, ok := candidates[identifier]
		if !ok {
			continue
		}
		key := candidate.Path + "::" + candidate.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		matches = append(matches, candidate)
		if len(matches) > 1 {
			return importCandidate{}, false
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return importCandidate{}, false
}

func resolvePackageRoot(file string, importPath string) (string, string, string, bool) {
	rootName, subpath := splitPackageImportPath(importPath)
	if rootName == "" {
		return "", "", "", false
	}

	dir := filepath.Dir(file)
	for {
		candidate := filepath.Join(dir, "node_modules", filepath.FromSlash(rootName))
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return filepath.Clean(candidate), rootName, subpath, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", "", "", false
}

func splitPackageImportPath(importPath string) (string, string) {
	parts := strings.Split(importPath, "/")
	if len(parts) == 0 {
		return "", ""
	}
	if strings.HasPrefix(importPath, "@") {
		if len(parts) < 2 {
			return "", ""
		}
		root := parts[0] + "/" + parts[1]
		return root, strings.Join(parts[2:], "/")
	}
	return parts[0], strings.Join(parts[1:], "/")
}

func resolvePackageTypesEntryFile(rootDir, subpath string) (string, bool) {
	if subpath != "" {
		return resolvePackageModuleFile(rootDir, subpath)
	}

	packageJSONPath := filepath.Join(rootDir, "package.json")
	if packageJSONBytes, err := os.ReadFile(packageJSONPath); err == nil {
		var packageJSON struct {
			Types   string `json:"types"`
			Typings string `json:"typings"`
		}
		if err := json.Unmarshal(packageJSONBytes, &packageJSON); err == nil {
			for _, candidate := range []string{packageJSON.Types, packageJSON.Typings} {
				if candidate == "" {
					continue
				}
				if resolved, ok := resolvePackageModuleFile(rootDir, candidate); ok {
					return resolved, true
				}
			}
		}
	}

	return resolvePackageModuleFile(rootDir, "index")
}

func resolvePackageModuleFile(rootDir, modulePath string) (string, bool) {
	modulePath = strings.TrimSuffix(modulePath, ".d.ts")
	modulePath = strings.TrimSuffix(modulePath, ".ts")
	basePath := filepath.Join(rootDir, filepath.FromSlash(modulePath))
	candidates := []string{
		basePath,
		basePath + ".d.ts",
		basePath + ".ts",
		filepath.Join(basePath, "index.d.ts"),
		filepath.Join(basePath, "index.ts"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return filepath.Clean(candidate), true
		}
	}
	return "", false
}

func packageModuleImportPath(rootName, rootDir, file string) string {
	relative, err := filepath.Rel(rootDir, file)
	if err != nil {
		return ""
	}
	relative = filepath.ToSlash(relative)
	relative = strings.TrimSuffix(relative, ".d.ts")
	relative = strings.TrimSuffix(relative, ".ts")
	if relative == "" || relative == "index" {
		return rootName
	}
	relative = strings.TrimSuffix(relative, "/index")
	if relative == "" {
		return rootName
	}
	return rootName + "/" + relative
}

func packageExportCandidates(file string, importPath string) map[string]importCandidate {
	rootDir, rootName, subpath, ok := resolvePackageRoot(file, importPath)
	if !ok {
		return map[string]importCandidate{}
	}

	exportCache := map[string]map[string]bool{}
	candidates := map[string]importCandidate{}

	if entryFile, ok := resolvePackageTypesEntryFile(rootDir, subpath); ok {
		for name := range exportedTypeNames(entryFile, "", exportCache) {
			candidates[name] = importCandidate{Path: importPath, Name: name}
		}
	}

	_ = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".d.ts") {
			return nil
		}
		modulePath := packageModuleImportPath(rootName, rootDir, path)
		if modulePath == "" {
			return nil
		}
		for name := range exportedTypeNames(path, "", exportCache) {
			if _, exists := candidates[name]; !exists {
				candidates[name] = importCandidate{Path: modulePath, Name: name}
			}
		}
		return nil
	})

	return candidates
}

func exportedTypeNames(file, source string, cache map[string]map[string]bool) map[string]bool {
	file = filepath.Clean(file)
	if cached, ok := cache[file]; ok {
		return cached
	}

	exports := map[string]bool{}
	cache[file] = exports

	if source == "" {
		sourceBytes, err := os.ReadFile(file)
		if err != nil {
			return exports
		}
		source = string(sourceBytes)
	}

	for _, match := range exportedTypePattern.FindAllStringSubmatch(source, -1) {
		exports[match[1]] = true
	}
	for name := range parseExportedNamespaceMembers(source) {
		exports[name] = true
	}
	for _, reexport := range parseReexportClauses(source) {
		if reexport.ExportAll {
			if !strings.HasPrefix(reexport.Path, ".") {
				continue
			}
			resolvedFile, ok := resolveLocalImportFile(file, reexport.Path)
			if !ok {
				continue
			}
			for name := range exportedTypeNames(resolvedFile, "", cache) {
				exports[name] = true
			}
			continue
		}
		for name := range reexport.Names {
			exports[name] = true
		}
	}

	return exports
}

func parseExportedNamespaceMembers(source string) map[string]bool {
	result := map[string]bool{}
	matches := exportedNamespacePattern.FindAllStringSubmatchIndex(source, -1)
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		name := source[match[2]:match[3]]
		result[name] = true

		header := source[match[0]:match[1]]
		braceOffset := strings.LastIndex(header, "{")
		if braceOffset == -1 {
			continue
		}
		braceStart := match[0] + braceOffset
		braceEnd, err := findMatching(source, braceStart, '{', '}')
		if err != nil {
			continue
		}
		body := source[braceStart+1 : braceEnd]
		for _, member := range exportedNamespaceMemberPattern.FindAllStringSubmatch(body, -1) {
			result[member[1]] = true
		}
	}
	return result
}

type reexportClause struct {
	Path      string
	Names     map[string]bool
	ExportAll bool
}

func parseReexportClauses(source string) []reexportClause {
	var clauses []reexportClause

	for _, match := range exportAllPattern.FindAllStringSubmatch(source, -1) {
		clauses = append(clauses, reexportClause{
			Path:      strings.TrimSpace(match[1]),
			ExportAll: true,
		})
	}

	for _, match := range exportClauseWithFromPattern.FindAllStringSubmatch(source, -1) {
		names := map[string]bool{}
		for _, specifier := range strings.Split(match[1], ",") {
			specifier = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(specifier), "type "))
			if specifier == "" {
				continue
			}
			parts := strings.Split(specifier, " as ")
			name := ""
			switch len(parts) {
			case 1:
				name = strings.TrimSpace(parts[0])
			case 2:
				name = strings.TrimSpace(parts[1])
			}
			if name != "" {
				names[name] = true
			}
		}
		if len(names) == 0 {
			continue
		}
		clauses = append(clauses, reexportClause{
			Path:  strings.TrimSpace(match[2]),
			Names: names,
		})
	}

	return clauses
}

func sourceImportPath(file string) string {
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	if base == "" {
		return "."
	}
	return "./" + base
}

func resolveLocalImportFile(file, importPath string) (string, bool) {
	if !strings.HasPrefix(importPath, ".") {
		return "", false
	}

	basePath := filepath.Join(filepath.Dir(file), importPath)
	candidates := []string{
		basePath,
		basePath + ".ts",
		basePath + ".tsx",
		basePath + ".js",
		basePath + ".jsx",
		filepath.Join(basePath, "index.ts"),
		filepath.Join(basePath, "index.tsx"),
		filepath.Join(basePath, "index.js"),
		filepath.Join(basePath, "index.jsx"),
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return filepath.Clean(candidate), true
		}
	}
	return "", false
}

func filterImportsByTypeTexts(imports []TypeImport, typeTexts []string) []TypeImport {
	used := usedImportReferenceNames(typeTexts)
	if len(used) == 0 {
		return nil
	}

	filtered := make([]TypeImport, 0, len(imports))
	for _, item := range imports {
		var names []string
		for _, name := range item.Names {
			if used[importReferenceName(name)] {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			filtered = append(filtered, TypeImport{Path: item.Path, Names: names})
		}
	}
	return filtered
}

func importReferenceName(name string) string {
	if strings.HasPrefix(name, "* as ") {
		return strings.TrimSpace(strings.TrimPrefix(name, "* as "))
	}
	if strings.Contains(name, " as ") {
		parts := strings.SplitN(name, " as ", 2)
		return strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(name)
}

func collectUsedTypeReferences(typeTexts []string) usedTypeReferences {
	references := usedTypeReferences{
		BareIdentifiers: map[string]bool{},
		QualifiedOwners: map[string]bool{},
	}
	for _, typeText := range typeTexts {
		collectUsedTypeReferencesInto(typeText, references)
	}
	return references
}

func shouldImportIdentifier(identifier string) bool {
	if identifier == "" {
		return false
	}
	if identifier[0] < 'A' || identifier[0] > 'Z' {
		return false
	}
	_, blocked := builtinTypeNames[identifier]
	return !blocked
}

func collectUsedTypeReferencesInto(typeText string, references usedTypeReferences) {
	for i := 0; i < len(typeText); {
		if !isIdentifierStart(typeText[i]) {
			i++
			continue
		}
		start := i
		i++
		for i < len(typeText) && isIdentifierPart(typeText[i]) {
			i++
		}
		identifier := typeText[start:i]
		previous := previousNonWhitespaceByte(typeText, start)
		next := nextNonWhitespaceByte(typeText, i)
		if previous == '.' {
			continue
		}
		if next == '.' {
			references.QualifiedOwners[identifier] = true
			continue
		}
		if shouldImportIdentifier(identifier) {
			references.BareIdentifiers[identifier] = true
		}
	}
}

func usedImportReferenceNames(typeTexts []string) map[string]bool {
	references := collectUsedTypeReferences(typeTexts)
	used := map[string]bool{}
	for identifier := range references.BareIdentifiers {
		used[identifier] = true
	}
	for identifier := range references.QualifiedOwners {
		used[identifier] = true
	}
	return used
}

func previousNonWhitespaceByte(text string, index int) byte {
	for index > 0 {
		index--
		if text[index] == ' ' || text[index] == '\t' || text[index] == '\n' || text[index] == '\r' {
			continue
		}
		return text[index]
	}
	return 0
}

func nextNonWhitespaceByte(text string, index int) byte {
	for index < len(text) {
		if text[index] == ' ' || text[index] == '\t' || text[index] == '\n' || text[index] == '\r' {
			index++
			continue
		}
		return text[index]
	}
	return 0
}

var importClausePattern = regexp.MustCompile(`(?m)^\s*import(?:\s+type)?\s*\{([^}]*)\}\s*from\s*['"]([^'"]+)['"]`)

var importDefaultNamedPattern = regexp.MustCompile(`(?ms)^\s*import(?:\s+type)?\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*,\s*\{([^}]*)\}\s*from\s*['"]([^'"]+)['"]`)

var importDefaultPattern = regexp.MustCompile(`(?m)^\s*import(?:\s+type)?\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*from\s*['"]([^'"]+)['"]`)

var importNamespacePattern = regexp.MustCompile(`(?m)^\s*import(?:\s+type)?\s+\*\s+as\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*from\s*['"]([^'"]+)['"]`)

var exportedTypePattern = regexp.MustCompile(`(?m)^\s*export\s+(?:declare\s+)?(?:interface|type|class|enum)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)

var exportedNamespacePattern = regexp.MustCompile(`(?m)^\s*export\s+(?:declare\s+)?namespace\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\{`)

var exportedNamespaceMemberPattern = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:declare\s+)?(?:interface|type|class|enum)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)

var exportClauseWithFromPattern = regexp.MustCompile(`(?m)^\s*export(?:\s+type)?\s*\{([^}]*)\}\s*from\s*['"]([^'"]+)['"]`)

var exportAllPattern = regexp.MustCompile(`(?m)^\s*export\s*\*\s*from\s*['"]([^'"]+)['"]`)

var builtinTypeNames = map[string]struct{}{
	"Array":         {},
	"Awaited":       {},
	"Boolean":       {},
	"Date":          {},
	"Error":         {},
	"Exclude":       {},
	"Extract":       {},
	"InstanceType":  {},
	"Map":           {},
	"NonNullable":   {},
	"Number":        {},
	"Omit":          {},
	"Partial":       {},
	"Pick":          {},
	"Promise":       {},
	"Readonly":      {},
	"ReadonlyArray": {},
	"Record":        {},
	"RegExp":        {},
	"Required":      {},
	"ReturnType":    {},
	"Set":           {},
	"String":        {},
	"ThisType":      {},
	"URL":           {},
	"Uppercase":     {},
	"Lowercase":     {},
	"Capitalize":    {},
	"Uncapitalize":  {},
}
