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
	"unicode"

	"kea-typegen/rewrite/internal/tsgoapi"
)

var (
	optionalUndefinedUnionPattern = regexp.MustCompile(`(\?\s*:\s*[^;,\n{}]+?)\s*\|\s*undefined\b`)
	unbracedMemberTypePattern     = regexp.MustCompile(`\b[$A-Za-z_][\w$]*\??\s*:`)
	selectorInputTupleHintPattern = regexp.MustCompile(`=>\s*\[\s*(?:\.{3}|[^\]])`)
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

type ParsedFunction struct {
	Name         string `json:"name"`
	FunctionType string `json:"functionType"`
}

type sourceObjectEntry struct {
	Name  string
	Value string
}

type selectorDependency struct {
	Name string
	Type string
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
	Name                   string                 `json:"name"`
	TypeName               string                 `json:"typeName"`
	File                   string                 `json:"file"`
	InputKind              string                 `json:"inputKind"`
	Path                   []string               `json:"path,omitempty"`
	PathString             string                 `json:"pathString,omitempty"`
	PropsType              string                 `json:"propsType,omitempty"`
	KeyType                string                 `json:"keyType,omitempty"`
	Actions                []ParsedAction         `json:"actions,omitempty"`
	Reducers               []ParsedField          `json:"reducers,omitempty"`
	Selectors              []ParsedField          `json:"selectors,omitempty"`
	Listeners              []ParsedListener       `json:"listeners,omitempty"`
	SharedListeners        []ParsedSharedListener `json:"sharedListeners,omitempty"`
	Events                 []string               `json:"events,omitempty"`
	CustomType             string                 `json:"customType,omitempty"`
	ExtraInputForm         string                 `json:"extraInputForm,omitempty"`
	InternalSelectorTypes  []ParsedFunction       `json:"internalSelectorTypes,omitempty"`
	InternalReducerActions []ParsedAction         `json:"internalReducerActions,omitempty"`
	Imports                []TypeImport           `json:"imports,omitempty"`
}

type buildState struct {
	binaryPath       string
	projectDir       string
	configFile       string
	timeout          time.Duration
	parsedByFile     map[string][]ParsedLogic
	building         map[string]bool
	apiClient        *tsgoapi.Client
	apiSnapshot      string
	config           *tsgoapi.ConfigResponse
	primaryProjectID string
	projectByFile    map[string]string
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
		if property, hasProperty := properties["props"]; hasProperty && (parsed.PropsType == "" || isAnyLikeType(parsed.PropsType)) {
			if recovered := normalizeSourceTypeText(sourceExpressionTypeText(source, sourcePropertyText(source, property))); recovered != "" && !isAnyLikeType(recovered) {
				parsed.PropsType = recovered
			}
		}
	}
	if section, ok := sections["key"]; ok {
		parsed.KeyType = preferredTypeText(section)
		if property, hasProperty := properties["key"]; hasProperty && (parsed.KeyType == "" || isAnyLikeType(parsed.KeyType)) {
			if probed := normalizeSourceTypeText(sourceCallbackReturnTypeFromTypeProbe(source, report.File, property, state)); probed != "" &&
				!isAnyLikeType(probed) &&
				!typeTextContainsStandaloneToken(probed, "any") &&
				!typeTextContainsStandaloneToken(probed, "unknown") {
				parsed.KeyType = probed
			} else if inferred := sourceArrowReturnTypeText(source, sourcePropertyText(source, property)); inferred != "" {
				parsed.KeyType = inferred
			} else if recovered := sourceKeyTypeFromSource(source, report.File, property, parsed.PropsType, state); recovered != "" {
				parsed.KeyType = recovered
			}
		}
	}
	if section, ok := sections["actions"]; ok {
		parsed.Actions = parseActionsWithSource(section, source, properties["actions"], logicReport.InputKind, report.File, state)
	}
	if section, ok := sections["defaults"]; ok {
		parsed.Reducers = mergeParsedFields(parsed.Reducers, parseDefaultFieldsWithSource(section, source, properties["defaults"])...)
	}
	if section, ok := sections["reducers"]; ok {
		parsed.Reducers = mergeParsedFields(parsed.Reducers, parseReducersWithSource(section, source, properties["reducers"], report.File, state)...)
	}
	if section, ok := sections["windowValues"]; ok {
		parsed.Reducers = mergeParsedFields(parsed.Reducers, parseWindowValues(section)...)
	}
	if section, ok := sections["form"]; ok {
		formActions, formReducers, extraInputForm := parseFormPluginSection(section)
		parsed.Actions = mergeParsedActions(parsed.Actions, formActions...)
		parsed.Reducers = mergeParsedFields(parsed.Reducers, formReducers...)
		if parsed.ExtraInputForm == "" {
			parsed.ExtraInputForm = extraInputForm
		}
	}
	if section, ok := sections["loaders"]; ok {
		loaderActions, loaderReducers := parseLoadersWithSource(section, source, properties["loaders"], report.File, state)
		parsed.Actions = mergeParsedActionsPreferExisting(parsed.Actions, loaderActions...)
		parsed.Reducers = mergeParsedFields(parsed.Reducers, loaderReducers...)
	}
	if section, ok := sections["lazyLoaders"]; ok {
		loaderActions, loaderReducers := parseLazyLoadersWithSource(section, source, properties["lazyLoaders"], report.File, state)
		parsed.Actions = mergeParsedActionsPreferExisting(parsed.Actions, loaderActions...)
		parsed.Reducers = mergeParsedFields(parsed.Reducers, loaderReducers...)
	}
	if section, ok := sections["selectors"]; ok {
		parsed.Selectors = parseSelectorsWithSource(section, parsed, source, properties["selectors"], report.File, state)
		parsed.InternalSelectorTypes = parseInternalSelectorTypesWithSource(section, parsed, source, properties["selectors"], report.File, state)
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
	if _, hasReducers := sections["reducers"]; hasReducers || sections["listeners"].Name != "" {
		parsed.InternalReducerActions = parseInternalReducerActions(source, report.File, properties["reducers"], properties["listeners"], state)
	}
	if section, ok := sections["reducers"]; ok {
		parsed.Actions = refineActionPayloadTypes(parsed.Actions, collectReducerActionPayloadHints(section))
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
	return parseActionsWithSource(section, "", SourceProperty{}, "", "", nil)
}

func parseActionsWithSource(
	section SectionReport,
	source string,
	property SourceProperty,
	inputKind string,
	file string,
	state *buildState,
) []ParsedAction {
	sourceMembers := sectionSourceProperties(source, property)
	actions := make([]ParsedAction, 0, len(section.Members))
	for _, member := range section.Members {
		if nested, ok := sourceMembers[member.Name]; ok {
			if strings.TrimSpace(sourcePropertyText(source, nested)) == "true" {
				actions = append(actions, ParsedAction{
					Name:         member.Name,
					FunctionType: "() => { value: true }",
					PayloadType:  "{ value: true }",
				})
				continue
			}
		}
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
				parameters = normalizeFunctionParametersText(parameters)
				if explicitReturn != "" {
					payloadType = explicitReturn
				}
				if actionPayloadNeedsSourceRecovery(payloadType) && explicitReturn == "" {
					if inferred := sourceActionPayloadTypeFromSource(source, expression); inferred != "" {
						payloadType = inferred
					}
					if sourceActionHasDefaultedParameters(expression) {
						if probed := sourceArrowReturnTypeFromTypeProbe(source, file, nested, state); isUsableActionPayloadProbeType(probed) {
							payloadType = probed
						}
					}
				}
				if payloadType == "" {
					if _, returnType, ok := splitFunctionType(functionType); ok {
						payloadType = returnType
					}
				}
				if payloadType != "" {
					if memberParameters, _, ok := splitFunctionType(functionType); ok && !sourceActionParametersArePreferred(parameters) {
						parameters = memberParameters
					}
					functionType = parameters + " => " + payloadType
				}
			}
		}
		payloadType = normalizeActionPayloadType(payloadType)
		if inputKind == "object" {
			payloadType = stripNullableActionPayloadProperties(payloadType)
		}
		actions = append(actions, ParsedAction{
			Name:         member.Name,
			FunctionType: functionType,
			PayloadType:  payloadType,
		})
	}
	return actions
}

func typeTextNeedsSourceRecovery(typeText string) bool {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if text == "" || isAnyLikeType(text) {
		return true
	}
	return typeTextContainsStandaloneToken(text, "any") || typeTextContainsStandaloneToken(text, "unknown")
}

func sourceActionParametersArePreferred(parameters string) bool {
	parts, ok := splitFunctionParameterParts(parameters)
	if !ok {
		return false
	}
	for _, part := range parts {
		part = normalizeParameterDeclarationText(part)
		if part == "" {
			continue
		}
		typeText := parameterTypeText(part)
		if typeText == "" || typeTextNeedsSourceRecovery(typeText) {
			return false
		}
	}
	return true
}

func parameterTypeText(part string) string {
	_, typeText, ok := splitTopLevelProperty(part)
	if !ok {
		return ""
	}
	return normalizeSourceTypeText(typeText)
}

func sourceActionHasDefaultedParameters(expression string) bool {
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return false
	}
	parts, ok := splitFunctionParameterParts(info.Parameters)
	if !ok {
		return false
	}
	for _, part := range parts {
		if index := findTopLevelParameterDefault(strings.TrimSpace(part)); index != -1 {
			return true
		}
	}
	return false
}

func isUsableActionPayloadProbeType(typeText string) bool {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	return strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}")
}

func actionPayloadNeedsSourceRecovery(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	switch {
	case text == "":
		return true
	case strings.Contains(text, "..."):
		return true
	case isAnyLikeType(text):
		return true
	case typeTextContainsStandaloneToken(text, "any"):
		return true
	case typeTextContainsStandaloneToken(text, "unknown"):
		return true
	case isGenericIndexSignatureType(text):
		return true
	default:
		return false
	}
}

func parseReducers(section SectionReport) []ParsedField {
	return parseReducersWithSource(section, "", SourceProperty{}, "", nil)
}

func parseReducersWithSource(section SectionReport, source string, property SourceProperty, file string, state *buildState) []ParsedField {
	sourceMembers := sectionSourceProperties(source, property)
	sourceEntries := sourceEntriesByName(sectionSourceEntries(source, property))
	reducers := make([]ParsedField, 0, len(section.Members))
	for _, member := range section.Members {
		stateType := "any"
		if nested, ok := sourceMembers[member.Name]; ok {
			if hinted := sourceReducerStateTypeWithContext(source, file, sourcePropertyText(source, nested), state); hinted != "" {
				stateType = hinted
			}
		} else if entry, ok := sourceEntries[member.Name]; ok {
			if hinted := sourceReducerStateTypeWithContext(source, file, entry.Value, state); hinted != "" {
				stateType = hinted
			}
		}
		if parsed, ok := parseReducerStateType(member.TypeString); ok {
			stateType = preferredReducerStateType(stateType, parsed)
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

func parseFormPluginSection(section SectionReport) ([]ParsedAction, []ParsedField, string) {
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
	}, formType
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
		return normalizeSourceTypeTextWithOptions(printed, false)
	}
	return raw
}

func sourceExpressionTypeText(source, expression string) string {
	text := trimLeadingSourceTrivia(expression)
	if text == "" {
		return ""
	}
	if asserted := sourceAssertedType(text); asserted != "" {
		return normalizeSourceTypeText(asserted)
	}
	if objectType := sourceObjectLiteralTypeText(source, text); objectType != "" {
		return objectType
	}
	if arrayType := sourceArrayLiteralTypeText(source, text); arrayType != "" {
		return arrayType
	}
	if literalType := sourceLiteralTypeText(text); literalType != "" {
		return literalType
	}
	if newType := sourceNewExpressionTypeText(text); newType != "" {
		return newType
	}
	if identifier, ok := sourceIdentifierExpression(text); ok {
		inferredFromInitializer := ""
		if initializer := findLocalValueInitializer(source, identifier); initializer != "" {
			if inferred := sourceExpressionTypeText(source, initializer); inferred != "" {
				inferredFromInitializer = inferred
				if !isAnyLikeType(inferred) {
					return inferred
				}
			}
		}
		if declared := findLocalValueDeclaredType(source, identifier); declared != "" {
			return declared
		}
		if inferredFromInitializer != "" {
			return inferredFromInitializer
		}
		if expanded := expandLocalSourceTypeText(source, identifier); expanded != "" {
			return expanded
		}
		if len(identifier) > 0 && unicode.IsUpper(rune(identifier[0])) {
			return identifier
		}
		return "any"
	}
	return ""
}

func sourceNewExpressionTypeText(expression string) string {
	text := unwrapWrappedExpression(expression)
	if !strings.HasPrefix(text, "new ") {
		return ""
	}

	text = strings.TrimSpace(text[len("new "):])
	qualified, next, ok := sourceQualifiedIdentifierExpression(text)
	if !ok {
		return ""
	}

	rest := strings.TrimSpace(text[next:])
	if strings.HasPrefix(rest, "<") {
		end, err := findMatching(rest, 0, '<', '>')
		if err != nil {
			return ""
		}
		rest = strings.TrimSpace(rest[end+1:])
	}
	if rest != "" && !strings.HasPrefix(rest, "(") {
		return ""
	}
	return qualified
}

func sourceQualifiedIdentifierExpression(expression string) (string, int, bool) {
	text := strings.TrimSpace(expression)
	if text == "" || !isIdentifierStart(text[0]) {
		return "", 0, false
	}

	end := 1
	for end < len(text) {
		switch {
		case isIdentifierPart(text[end]):
			end++
		case text[end] == '.':
			if end+1 >= len(text) || !isIdentifierStart(text[end+1]) {
				return "", 0, false
			}
			end++
		default:
			return text[:end], end, true
		}
	}
	return text[:end], end, true
}

func sourceArrayLiteralTypeText(source, expression string) string {
	text := strings.TrimSpace(expression)
	if len(text) < 2 || text[0] != '[' {
		return ""
	}

	end, err := findMatching(text, 0, '[', ']')
	if err != nil || trimExpressionEnd(text, end+1) != len(text) {
		return ""
	}

	parts, err := splitTopLevelList(text[1:end])
	if err != nil {
		return ""
	}
	if len(parts) == 0 {
		return "any[]"
	}

	elementTypes := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.HasPrefix(part, "...") {
			spreadType := strings.TrimSpace(sourceExpressionTypeText(source, part[3:]))
			switch {
			case strings.HasSuffix(spreadType, "[]"):
				part = strings.TrimSpace(spreadType[:len(spreadType)-2])
			case strings.HasPrefix(spreadType, "Array<") && strings.HasSuffix(spreadType, ">"):
				part = strings.TrimSpace(spreadType[len("Array<") : len(spreadType)-1])
			case strings.HasPrefix(spreadType, "ReadonlyArray<") && strings.HasSuffix(spreadType, ">"):
				part = strings.TrimSpace(spreadType[len("ReadonlyArray<") : len(spreadType)-1])
			default:
				return "any[]"
			}
		} else {
			part = strings.TrimSpace(sourceExpressionTypeText(source, part))
		}
		if part == "" {
			return "any[]"
		}
		part = normalizeInferredTypeText(part)
		if !seen[part] {
			seen[part] = true
			elementTypes = append(elementTypes, part)
		}
	}

	if len(elementTypes) == 0 {
		return "any[]"
	}
	if len(elementTypes) == 1 {
		return arrayTypeText(elementTypes[0])
	}
	return arrayTypeText(strings.Join(elementTypes, " | "))
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
			if expanded := expandLocalSourceTypeText(source, spreadType); expanded != "" {
				spreadType = normalizeSourceTypeText(expanded)
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
			if shorthand, shorthandOK := sourceIdentifierExpression(text); shorthandOK {
				valueType := sourceExpressionTypeText(source, shorthand)
				if valueType != "" {
					properties[shorthand] = widenSourceObjectPropertyType(valueType)
				}
			}
			continue
		}
		valueType := sourceExpressionTypeText(source, value)
		if valueType == "" {
			continue
		}
		properties[name] = widenSourceObjectPropertyType(valueType)
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

func widenSourceObjectPropertyType(typeText string) string {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	switch {
	case isBooleanLiteralType(text):
		return "boolean"
	case isQuotedString(text):
		return "string"
	case isNumericLiteralType(text):
		return "number"
	default:
		return text
	}
}

func isNumericLiteralType(expression string) bool {
	text := strings.TrimSpace(expression)
	if text == "" {
		return false
	}
	hasDigit := false
	for i := 0; i < len(text); i++ {
		char := text[i]
		if (char < '0' || char > '9') && char != '.' && char != '_' && char != '-' {
			return false
		}
		if char >= '0' && char <= '9' {
			hasDigit = true
		}
	}
	return hasDigit
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
	declared := findLocalTypeAliasText(source, text)
	if declared == "" {
		return ""
	}
	if strings.HasPrefix(strings.TrimSpace(declared), "{") && strings.HasSuffix(strings.TrimSpace(declared), "}") {
		return normalizeInlineObjectTypeText(declared)
	}
	return normalizeSourceTypeText(declared)
}

func normalizeInlineObjectTypeText(typeText string) string {
	text := strings.TrimSpace(typeText)
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return normalizeSourceTypeText(text)
	}
	body := strings.TrimSpace(text[1 : len(text)-1])
	if body == "" {
		return "{}"
	}
	if strings.Contains(body, ";") {
		return normalizeSourceTypeText(text)
	}
	members, err := splitTopLevelTypeMembers(body)
	if err != nil || len(members) <= 1 {
		return normalizeSourceTypeText(text)
	}
	parts := make([]string, 0, len(members))
	for _, member := range members {
		member = strings.TrimSpace(strings.TrimSuffix(member, ";"))
		if member == "" {
			continue
		}
		parts = append(parts, member)
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(parts, "; ") + "; }"
}

func findLocalTypeAliasText(source, name string) string {
	interfacePattern := regexp.MustCompile(`(?m)^\s*(?:export\s+)?interface\s+` + regexp.QuoteMeta(name) + `\b[^{]*\{`)
	if match := interfacePattern.FindStringIndex(source); match != nil {
		header := source[match[0]:match[1]]
		braceOffset := strings.LastIndex(header, "{")
		if braceOffset != -1 {
			start := match[0] + braceOffset
			end, err := findMatching(source, start, '{', '}')
			if err == nil {
				return source[start : end+1]
			}
		}
	}

	typePattern := regexp.MustCompile(`(?m)^\s*(?:export\s+)?type\s+` + regexp.QuoteMeta(name) + `\s*=`)
	if match := typePattern.FindStringIndex(source); match != nil {
		start := skipTrivia(source, match[1])
		if start < len(source) && source[start] == '{' {
			end, err := findMatching(source, start, '{', '}')
			if err == nil {
				return source[start : end+1]
			}
		}
		end, err := findStatementExpressionEnd(source, start)
		if err == nil && end > start {
			return strings.TrimSpace(strings.TrimSuffix(source[start:end], ";"))
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

func findLocalValueDeclaredType(source, name string) string {
	valuePattern := regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(name) + `\b`)
	match := valuePattern.FindStringIndex(source)
	if match == nil {
		return ""
	}

	start := skipTrivia(source, match[1])
	colon := -1
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0
	for i := start; i < len(source); i++ {
		switch source[i] {
		case '\'':
			end, err := skipQuoted(source, i, '\'')
			if err != nil {
				return ""
			}
			i = end
		case '"':
			end, err := skipQuoted(source, i, '"')
			if err != nil {
				return ""
			}
			i = end
		case '`':
			end, err := skipTemplate(source, i)
			if err != nil {
				return ""
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
					return ""
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
		case ':':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				colon = i
			}
		case '=':
			if i+1 < len(source) && source[i+1] == '>' {
				i++
				continue
			}
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				if colon == -1 || colon > i {
					return ""
				}
				typeStart := skipTrivia(source, colon+1)
				if typeStart >= i {
					return ""
				}
				return normalizeSourceTypeText(source[typeStart:i])
			}
		case '\n':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 && colon == -1 {
				return ""
			}
		}
	}
	return ""
}

func findLocalFunctionReturnType(source, name string) string {
	functionPattern := regexp.MustCompile(`(?m)^\s*(?:export\s+)?function\s+` + regexp.QuoteMeta(name) + `\b`)
	match := functionPattern.FindStringIndex(source)
	if match == nil {
		return ""
	}

	start := skipTrivia(source, match[1])
	if start < len(source) && source[start] == '<' {
		end, err := findMatching(source, start, '<', '>')
		if err != nil {
			return ""
		}
		start = skipTrivia(source, end+1)
	}
	if start >= len(source) || source[start] != '(' {
		return ""
	}
	end, err := findMatching(source, start, '(', ')')
	if err != nil {
		return ""
	}
	start = skipTrivia(source, end+1)
	if start >= len(source) || source[start] != ':' {
		return ""
	}
	start = skipTrivia(source, start+1)
	end = findFunctionReturnTypeEnd(source, start)
	if end <= start {
		return ""
	}
	return normalizeSourceTypeText(source[start:end])
}

func findFunctionReturnTypeEnd(source string, start int) int {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0

	for i := start; i < len(source); i++ {
		switch source[i] {
		case '\'':
			end, err := skipQuoted(source, i, '\'')
			if err != nil {
				return -1
			}
			i = end
		case '"':
			end, err := skipQuoted(source, i, '"')
			if err != nil {
				return -1
			}
			i = end
		case '`':
			end, err := skipTemplate(source, i)
			if err != nil {
				return -1
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
					return -1
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
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				prefix := strings.TrimSpace(source[start:i])
				if prefix != "" && !strings.HasSuffix(prefix, "|") && !strings.HasSuffix(prefix, "&") {
					return i
				}
			}
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
		}
	}
	return -1
}

func sourceImportedValueInitializer(source, file, identifier string, state *buildState) (string, string, string, bool) {
	if state == nil || file == "" || identifier == "" {
		return "", "", "", false
	}

	if candidate, ok := parseNamedValueImports(source)[identifier]; ok {
		if candidate.ImportedName == "" || candidate.ImportedName == "default" {
			return "", "", "", false
		}
		resolvedFile, ok := resolveImportFile(file, candidate.Path, state)
		if !ok {
			return "", "", "", false
		}
		importedSourceBytes, err := os.ReadFile(resolvedFile)
		if err != nil {
			return "", "", "", false
		}
		importedSource := string(importedSourceBytes)
		if initializer := findLocalValueInitializer(importedSource, candidate.ImportedName); initializer != "" {
			return importedSource, resolvedFile, initializer, true
		}
	}

	return "", "", "", false
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
				if shouldTerminateStatementAtNewline(source, i) {
					return i, nil
				}
			}
		}
	}
	return len(source), nil
}

func shouldTerminateStatementAtNewline(source string, index int) bool {
	previous := previousNonWhitespaceByte(source, index)
	switch previous {
	case 0, '=', '(', '[', '{', ',', ':', '?', '.', '+', '-', '*', '/', '%', '|', '&', '!':
		return false
	case '>':
		previousIndex := previousNonWhitespaceIndex(source, index)
		if previousIndex > 0 && previousNonWhitespaceByte(source, previousIndex) == '=' {
			return false
		}
	}

	next := nextNonWhitespaceByte(source, index+1)
	switch next {
	case 0, ')', ']', '}', ';', ',', '.', '+', '-', '*', '/', '%', '|', '&', ':', '?':
		return false
	}

	return true
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
	return parseLoadersWithSource(section, "", SourceProperty{}, "", nil)
}

func parseLoadersWithSource(section SectionReport, source string, property SourceProperty, file string, state *buildState) ([]ParsedAction, []ParsedField) {
	var actions []ParsedAction
	var reducers []ParsedField
	sourceMembers := sectionSourceProperties(source, property)
	sourceEntries := sectionSourceEntries(source, property)
	entryByName := sourceEntriesByName(sourceEntries)
	memberByName := map[string]MemberReport{}
	orderedNames := make([]string, 0, len(section.Members)+len(sourceEntries))
	seenNames := map[string]bool{}

	for _, member := range section.Members {
		memberByName[member.Name] = member
		if !seenNames[member.Name] {
			seenNames[member.Name] = true
			orderedNames = append(orderedNames, member.Name)
		}
	}
	for _, entry := range sourceEntries {
		if !seenNames[entry.Name] {
			seenNames[entry.Name] = true
			orderedNames = append(orderedNames, entry.Name)
		}
	}

	for _, name := range orderedNames {
		member, hasMember := memberByName[name]
		defaultType := ""
		properties := map[string]string{}
		ok := false

		if hasMember {
			memberType := strings.TrimSpace(member.TypeString)
			if memberType == "" {
				memberType = preferredMemberTypeText(member)
			}
			if parsedDefault, parsedProperties, parsed := parseLoaderMemberType(memberType); parsed {
				defaultType = parsedDefault
				properties = parsedProperties
				ok = true
			}
		}

		if nested, okNested := sourceMembers[name]; okNested {
			if sourceDefault, sourceProperties, sourceOK := sourceLoaderMemberTypeFromProperty(source, nested, file, state); sourceOK {
				if strings.TrimSpace(sourceDefault) != "" {
					defaultType = sourceDefault
				}
				if len(sourceProperties) > 0 && (len(properties) == 0 || !hasLoaderActionProperties(properties)) {
					properties = sourceProperties
				}
				ok = true
			} else if hinted := sourceLoaderDefaultType(source, nested); hinted != "" {
				defaultType = hinted
			}
		} else if entry, okEntry := entryByName[name]; okEntry {
			if sourceDefault, sourceProperties, sourceOK := sourceLoaderMemberTypeFromExpression(source, entry.Value); sourceOK {
				if strings.TrimSpace(sourceDefault) != "" {
					defaultType = sourceDefault
				}
				if len(sourceProperties) > 0 && (len(properties) == 0 || !hasLoaderActionProperties(properties)) {
					properties = sourceProperties
				}
				ok = true
			}
		}
		if !ok {
			continue
		}
		defaultType = widenLiteralReducerStateType(normalizeInferredTypeText(defaultType))
		if defaultType == "" {
			defaultType = "any"
		}

		reducers = append(reducers,
			ParsedField{Name: name, Type: defaultType},
			ParsedField{Name: name + "Loading", Type: "boolean"},
		)

		actions = append(actions, parseLoaderActions(name, properties, "__default", defaultType)...)
	}

	return actions, reducers
}

func parseLazyLoaders(section SectionReport) ([]ParsedAction, []ParsedField) {
	return parseLazyLoadersWithSource(section, "", SourceProperty{}, "", nil)
}

func parseLazyLoadersWithSource(section SectionReport, source string, property SourceProperty, file string, state *buildState) ([]ParsedAction, []ParsedField) {
	var actions []ParsedAction
	var reducers []ParsedField
	sourceMembers := sectionSourceProperties(source, property)
	sourceEntries := sectionSourceEntries(source, property)
	entryByName := sourceEntriesByName(sourceEntries)
	memberByName := map[string]MemberReport{}
	orderedNames := make([]string, 0, len(section.Members)+len(sourceEntries))
	seenNames := map[string]bool{}

	for _, member := range section.Members {
		memberByName[member.Name] = member
		if !seenNames[member.Name] {
			seenNames[member.Name] = true
			orderedNames = append(orderedNames, member.Name)
		}
	}
	for _, entry := range sourceEntries {
		if !seenNames[entry.Name] {
			seenNames[entry.Name] = true
			orderedNames = append(orderedNames, entry.Name)
		}
	}

	for _, name := range orderedNames {
		member, hasMember := memberByName[name]
		defaultType := ""
		properties := map[string]string{}
		ok := false

		if hasMember {
			memberType := strings.TrimSpace(member.TypeString)
			if memberType == "" {
				memberType = preferredMemberTypeText(member)
			}
			if parsedDefault, parsedProperties, parsed := parseLazyLoaderMemberType(memberType); parsed {
				defaultType = parsedDefault
				properties = parsedProperties
				ok = true
			}
		}

		if nested, okNested := sourceMembers[name]; okNested {
			if sourceDefault, sourceProperties, sourceOK := sourceLoaderMemberTypeFromProperty(source, nested, file, state); sourceOK {
				if strings.TrimSpace(sourceDefault) != "" {
					defaultType = sourceDefault
				}
				if len(sourceProperties) > 0 && (len(properties) == 0 || !hasLoaderActionProperties(properties)) {
					properties = sourceProperties
				}
				ok = true
			} else if hinted := sourceLoaderDefaultType(source, nested); hinted != "" {
				defaultType = hinted
			}
		} else if entry, okEntry := entryByName[name]; okEntry {
			if sourceDefault, sourceProperties, sourceOK := sourceLoaderMemberTypeFromExpression(source, entry.Value); sourceOK {
				if strings.TrimSpace(sourceDefault) != "" {
					defaultType = sourceDefault
				}
				if len(sourceProperties) > 0 && (len(properties) == 0 || !hasLoaderActionProperties(properties)) {
					properties = sourceProperties
				}
				ok = true
			}
		}
		if !ok {
			continue
		}
		defaultType = widenLiteralReducerStateType(normalizeInferredTypeText(defaultType))
		if defaultType == "" {
			defaultType = "any"
		}

		reducers = append(reducers,
			ParsedField{Name: name, Type: defaultType},
			ParsedField{Name: name + "Loading", Type: "boolean"},
		)

		actions = append(actions, parseLoaderActions(name, properties, "__default", defaultType)...)
	}

	return actions, reducers
}

func parseLoaderMemberType(typeText string) (string, map[string]string, bool) {
	text := strings.TrimSpace(typeText)
	if text == "" {
		return "", nil, false
	}

	if properties, ok := parseObjectTypeMembers(text); ok {
		return strings.TrimSpace(properties["__default"]), properties, true
	}

	if len(text) < 2 || text[0] != '[' || text[len(text)-1] != ']' {
		return "", nil, false
	}

	parts, err := splitTopLevelList(text[1 : len(text)-1])
	if err != nil || len(parts) < 2 {
		return "", nil, false
	}

	properties, ok := parseObjectTypeMembers(strings.TrimSpace(parts[1]))
	if !ok {
		return "", nil, false
	}

	return strings.TrimSpace(parts[0]), properties, true
}

func sourceLoaderMemberTypeFromExpression(source, expression string) (string, map[string]string, bool) {
	text := strings.TrimSpace(expression)
	if text == "" {
		return "", nil, false
	}

	if properties, ok := sourceLoaderObjectProperties(source, text, ""); ok {
		return strings.TrimSpace(properties["__default"]), properties, true
	}

	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(text, 0, len(text))
	if err != nil || !ok {
		return "", nil, false
	}
	parts, err := splitTopLevelList(text[arrayStart+1 : arrayEnd])
	if err != nil || len(parts) < 2 {
		return "", nil, false
	}

	defaultType := normalizeInferredTypeText(sourceExpressionTypeText(source, strings.TrimSpace(parts[0])))
	properties, ok := sourceLoaderObjectProperties(source, strings.TrimSpace(parts[1]), defaultType)
	if !ok {
		return "", nil, false
	}
	properties["__default"] = defaultType
	return defaultType, properties, true
}

func sourceLoaderMemberTypeFromProperty(source string, property SourceProperty, file string, state *buildState) (string, map[string]string, bool) {
	if source == "" || property.ValueEnd <= property.ValueStart {
		return "", nil, false
	}

	if properties, ok := sourceLoaderObjectPropertiesFromRange(source, property.ValueStart, property.ValueEnd, file, state, ""); ok {
		return strings.TrimSpace(properties["__default"]), properties, true
	}

	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(source, property.ValueStart, property.ValueEnd)
	if err != nil || !ok {
		return "", nil, false
	}
	parts, err := splitTopLevelSourceSegments(source, arrayStart+1, arrayEnd)
	if err != nil || len(parts) < 2 {
		return "", nil, false
	}

	defaultType := normalizeInferredTypeText(sourceExpressionTypeText(source, parts[0].Text))
	properties, ok := sourceLoaderObjectPropertiesFromRange(source, parts[1].Start, parts[1].End, file, state, defaultType)
	if !ok {
		return "", nil, false
	}
	properties["__default"] = defaultType
	return defaultType, properties, true
}

func sourceLoaderObjectProperties(source, expression, defaultTypeHint string) (map[string]string, bool) {
	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(expression, 0, len(expression))
	if err != nil || !ok {
		return nil, false
	}

	properties, err := parseTopLevelProperties(expression, objectStart, objectEnd)
	if err != nil {
		return nil, false
	}

	result := map[string]string{}
	for _, nested := range properties {
		value := sourcePropertyText(expression, nested)
		if nested.Name == "__default" {
			result[nested.Name] = normalizeInferredTypeText(sourceExpressionTypeText(source, value))
			continue
		}
		if functionType := sourceArrowFunctionTypeText(source, value); functionType != "" {
			result[nested.Name] = functionType
			continue
		}
		if functionType := sourceLoaderArrowFunctionTypeTextWithFallback(source, value, defaultTypeHint); functionType != "" {
			result[nested.Name] = functionType
		}
	}

	if len(result) == 0 {
		return nil, false
	}
	return result, true
}

func sourceLoaderObjectPropertiesFromRange(source string, valueStart, valueEnd int, file string, state *buildState, defaultTypeHint string) (map[string]string, bool) {
	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(source, valueStart, valueEnd)
	if err != nil || !ok {
		return nil, false
	}

	properties, err := parseTopLevelProperties(source, objectStart, objectEnd)
	if err != nil {
		return nil, false
	}

	result := map[string]string{}
	for _, nested := range properties {
		value := sourcePropertyText(source, nested)
		if nested.Name == "__default" {
			result[nested.Name] = normalizeInferredTypeText(sourceExpressionTypeText(source, value))
			continue
		}
		if functionType := sourceArrowFunctionTypeTextFromRange(source, file, nested, state); functionType != "" {
			result[nested.Name] = functionType
			continue
		}
		if functionType := sourceLoaderArrowFunctionTypeTextWithFallback(source, value, defaultTypeHint); functionType != "" {
			result[nested.Name] = functionType
		}
	}

	if len(result) == 0 {
		return nil, false
	}
	return result, true
}

func sourceLoaderArrowFunctionTypeTextWithFallback(source, expression, defaultTypeHint string) string {
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return ""
	}

	returnType := normalizeInferredTypeText(strings.TrimSpace(defaultTypeHint))
	if returnType == "" {
		return ""
	}
	if info.Async {
		returnType = promiseTypeText(returnType)
	}
	return info.Parameters + " => " + returnType
}

func hasLoaderActionProperties(properties map[string]string) bool {
	for name := range properties {
		if name != "__default" {
			return true
		}
	}
	return false
}

func parseLoaderActions(loaderName string, properties map[string]string, skipProperty, defaultType string) []ParsedAction {
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
		successType := preferredLoaderSuccessType(returnType, defaultType)
		if successType == "" {
			successType = unwrapPromiseType(returnType)
		}

		parameters, basePayload := loaderActionParameters(parameters)
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
				FunctionType: fmt.Sprintf("(%s: %s, payload?: %s) => { %s: %s; payload?: %s }", loaderName, successType, basePayload, loaderName, successType, basePayload),
				PayloadType:  fmt.Sprintf("{ %s: %s; payload?: %s }", loaderName, successType, basePayload),
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

func preferredLoaderSuccessType(returnType, defaultType string) string {
	resolved := normalizeInferredTypeText(strings.TrimSpace(unwrapPromiseType(returnType)))
	fallback := normalizeInferredTypeText(strings.TrimSpace(defaultType))
	switch resolved {
	case "", "any", "unknown", "PromiseConstructor":
		return fallback
	default:
		return resolved
	}
}

func loaderActionParameters(parameters string) (string, string) {
	parsed, ok := parseFunctionParameters(parameters)
	if !ok {
		return parameters, firstParameterType(parameters)
	}

	kept := make([]string, 0, len(parsed))
	firstType := ""
	for _, parameter := range parsed {
		if isLoaderHelperParameter(parameter) {
			continue
		}
		kept = append(kept, parameter.Text)
		if firstType == "" {
			firstType = parameter.Type
		}
	}

	if len(kept) == 0 {
		return "()", ""
	}
	return "(" + strings.Join(kept, ", ") + ")", firstType
}

func isLoaderHelperParameter(parameter parsedParameter) bool {
	if name, ok := sourceParameterName(parameter.Text); ok {
		switch name {
		case "breakpoint":
			return true
		}
	}

	typeText := normalizeSourceTypeText(parameter.Type)
	return typeTextContainsStandaloneToken(typeText, "BreakPointFunction") ||
		typeTextContainsStandaloneToken(typeText, "BreakpointFunction")
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
	return parseSelectorsWithSource(section, ParsedLogic{}, "", SourceProperty{}, "", nil)
}

func parseSelectorsWithSource(
	section SectionReport,
	logic ParsedLogic,
	source string,
	property SourceProperty,
	file string,
	state *buildState,
) []ParsedField {
	sourceMembers := sectionSourceProperties(source, property)
	sourceEntries := canonicalizeSourceObjectEntries(source, file, sectionSourceEntries(source, property), state)
	entryByName := sourceEntriesByName(sourceEntries)
	memberByName := map[string]MemberReport{}
	orderedNames := make([]string, 0, len(section.Members)+len(sourceEntries))
	seenNames := map[string]bool{}

	for _, member := range section.Members {
		member.Name = canonicalSourceObjectMemberName(source, file, member.Name, state)
		memberByName[member.Name] = member
		if !seenNames[member.Name] {
			seenNames[member.Name] = true
			orderedNames = append(orderedNames, member.Name)
		}
	}
	for _, entry := range sourceEntries {
		if !seenNames[entry.Name] {
			seenNames[entry.Name] = true
			orderedNames = append(orderedNames, entry.Name)
		}
	}

	selectors := make([]ParsedField, 0, len(orderedNames))
	for _, name := range orderedNames {
		member, hasMember := memberByName[name]
		selectorType := ""
		parsedMemberReturnType := ""
		if hasMember {
			if returnType, ok := parseSelectorReturnType(member.TypeString); ok && !isAnyLikeType(returnType) {
				parsedMemberReturnType = returnType
			}
		}
		if nested, ok := sourceMembers[name]; ok {
			if explicitReturn := sourceSelectorReturnType(sourcePropertyText(source, nested)); explicitReturn != "" {
				selectorType = explicitReturn
			}
		} else if entry, ok := entryByName[name]; ok {
			if explicitReturn := sourceSelectorReturnType(entry.Value); explicitReturn != "" {
				selectorType = explicitReturn
			}
		}
		if selectorType == "" && hasMember {
			if returnType := strings.TrimSpace(preferredMemberReturnTypeText(member)); returnType != "" && !strings.Contains(returnType, "...") && !isAnyLikeType(returnType) && !selectorReturnTypeConflicts(returnType, parsedMemberReturnType) {
				selectorType = returnType
			}
		}
		if selectorType == "" && parsedMemberReturnType != "" {
			selectorType = parsedMemberReturnType
		}
		if selectorTypeNeedsSourceRecovery(selectorType) && source != "" {
			if nested, ok := sourceMembers[name]; ok {
				if inferred := sourceSelectorInferredType(logic, source, file, sourcePropertyText(source, nested), state); inferred != "" {
					selectorType = inferred
				}
				if selectorTypeNeedsSourceRecovery(selectorType) {
					if probed := sourceSelectorReturnTypeFromTypeProbe(source, file, nested, state); probed != "" {
						selectorType = normalizeRecoveredSelectorType(source, file, probed)
					}
				}
			} else if entry, ok := entryByName[name]; ok {
				if inferred := sourceSelectorInferredType(logic, source, file, entry.Value, state); inferred != "" {
					selectorType = inferred
				}
			}
		}
		if selectorType == "" && hasMember {
			if returnType := strings.TrimSpace(preferredMemberReturnTypeText(member)); returnType != "" && !strings.Contains(returnType, "...") {
				selectorType = returnType
			}
		}
		if selectorType == "" && hasMember {
			if returnType, ok := parseSelectorReturnType(member.TypeString); ok {
				selectorType = returnType
			}
		}
		if selectorType == "" && hasMember {
			selectorType = "any"
		}
		if selectorType == "" {
			continue
		}
		selectors = append(selectors, ParsedField{Name: name, Type: selectorType})
	}
	return selectors
}

func parseInternalSelectorTypesWithSource(
	section SectionReport,
	logic ParsedLogic,
	source string,
	property SourceProperty,
	file string,
	state *buildState,
) []ParsedFunction {
	sourceMembers := sectionSourceProperties(source, property)
	sourceEntries := canonicalizeSourceObjectEntries(source, file, sectionSourceEntries(source, property), state)
	entryByName := sourceEntriesByName(sourceEntries)
	memberByName := map[string]MemberReport{}
	orderedNames := make([]string, 0, len(section.Members)+len(sourceEntries))
	seenNames := map[string]bool{}

	for _, member := range section.Members {
		member.Name = canonicalSourceObjectMemberName(source, file, member.Name, state)
		memberByName[member.Name] = member
		if !seenNames[member.Name] {
			seenNames[member.Name] = true
			orderedNames = append(orderedNames, member.Name)
		}
	}
	for _, entry := range sourceEntries {
		if !seenNames[entry.Name] {
			seenNames[entry.Name] = true
			orderedNames = append(orderedNames, entry.Name)
		}
	}

	helpers := make([]ParsedFunction, 0, len(orderedNames))
	for _, name := range orderedNames {
		member, hasMember := memberByName[name]
		selectorExpression := ""
		if nested, ok := sourceMembers[name]; ok {
			selectorExpression = sourcePropertyText(source, nested)
		} else if entry, ok := entryByName[name]; ok {
			selectorExpression = entry.Value
		}
		if hasMember && logic.InputKind == "builders" && !selectorMemberSupportsInternalHelper(member) && !selectorSourceSupportsInternalHelper(selectorExpression) {
			continue
		}

		functionType := ""
		if nested, ok := sourceMembers[name]; ok {
			functionType = sourceInternalSelectorFunctionType(logic, source, file, sourcePropertyText(source, nested), state)
		} else if entry, ok := entryByName[name]; ok {
			functionType = sourceInternalSelectorFunctionType(logic, source, file, entry.Value, state)
		}
		if functionType == "" {
			if hasMember {
				functionType = selectorFunctionTypeFromMember(member)
			}
		}
		parameters, _, ok := splitFunctionType(functionType)
		if !ok {
			continue
		}
		params, ok := parseFunctionParameters(parameters)
		if !ok || len(params) == 0 {
			continue
		}
		if logic.InputKind == "builders" && internalSelectorFunctionTypeIsUninformative(functionType, params) {
			continue
		}
		helpers = append(helpers, ParsedFunction{Name: name, FunctionType: functionType})
	}
	return helpers
}

func internalSelectorFunctionTypeIsUninformative(functionType string, params []parsedParameter) bool {
	_, returnType, ok := splitFunctionType(functionType)
	if !ok || !isAnyLikeSelectorHelperType(returnType) {
		return false
	}
	for _, param := range params {
		if !isAnyLikeSelectorHelperType(param.Type) {
			return false
		}
	}
	return true
}

func isAnyLikeSelectorHelperType(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	switch text {
	case "", "any", "unknown", "any[]", "unknown[]":
		return true
	default:
		return false
	}
}

func selectorSourceSupportsInternalHelper(expression string) bool {
	inputFunction := firstTopLevelArrayElement(strings.TrimSpace(expression))
	if inputFunction == "" {
		return false
	}
	info, ok := parseSourceArrowInfo(inputFunction)
	if !ok {
		return false
	}

	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
	}
	if body == "" {
		return false
	}

	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(body, 0, len(body))
	if err != nil || !ok {
		return false
	}
	parts, err := splitTopLevelList(body[arrayStart+1 : arrayEnd])
	if err != nil || len(parts) == 0 {
		return false
	}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return false
		}
		if _, ok := parseSourceArrowInfo(part); ok {
			continue
		}
		parameters, _, ok := splitFunctionType(part)
		if !ok || strings.TrimSpace(parameters) == "" {
			return false
		}
	}
	return true
}

func selectorMemberSupportsInternalHelper(member MemberReport) bool {
	for _, text := range []string{
		strings.TrimSpace(member.TypeString),
		strings.TrimSpace(member.PrintedTypeNode),
	} {
		if text == "" {
			continue
		}
		if selectorMemberTypeTextSupportsInternalHelper(text) {
			return true
		}
	}
	return false
}

func selectorMemberTypeTextSupportsInternalHelper(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if strings.HasPrefix(text, "[") {
		return true
	}
	return selectorInputTupleHintPattern.MatchString(text)
}

func selectorFunctionTypeFromMember(member MemberReport) string {
	text := strings.TrimSpace(member.TypeString)
	if element := lastTopLevelArrayElement(text); element != "" {
		if parameters, returnType, ok := splitFunctionType(element); ok && strings.TrimSpace(firstParameterText(parameters)) != "" {
			return parameters + " => " + normalizeInferredTypeText(returnType)
		}
	}
	if parameters, returnType, ok := splitFunctionType(text); ok && strings.TrimSpace(firstParameterText(parameters)) != "" {
		return parameters + " => " + normalizeInferredTypeText(returnType)
	}
	return ""
}

func sourceInternalSelectorFunctionType(logic ParsedLogic, source, file, expression string, state *buildState) string {
	selectorExpression := strings.TrimSpace(expression)
	if selectorExpression == "" {
		return ""
	}
	if element := sourceSelectorProjectorElement(selectorExpression); element != "" {
		selectorExpression = element
	}
	info, ok := parseSourceArrowInfo(selectorExpression)
	if !ok {
		return ""
	}
	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	dependencyNames := sourceSelectorDependencyNames(expression)
	parameterNames := internalHelperParameterNames(dependencyNames, info.ParameterNames)
	parameterCount := len(parameterNames)
	if parameterCount == 0 || len(dependencyTypes) < parameterCount {
		return ""
	}
	returnType := info.ExplicitReturn
	if returnType == "" {
		returnType = sourceSelectorInferredType(logic, source, file, expression, state)
	}
	if returnType == "" {
		return ""
	}

	parameters := make([]string, 0, parameterCount)
	for index, name := range parameterNames {
		parameterType := normalizeInternalHelperParameterType(dependencyTypes[index])
		if parameterType == "" {
			return ""
		}
		parameters = append(parameters, fmt.Sprintf("%s: %s", name, parameterType))
	}
	return "(" + strings.Join(parameters, ", ") + ") => " + returnType
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
	expression := sourcePropertyText(source, property)
	if expression == "" {
		return nil
	}
	if info, ok := parseSourceArrowInfo(expression); ok {
		body := strings.TrimSpace(info.Body)
		if info.BlockBody {
			body = singleReturnExpression(body)
		}
		if entries := sourceObjectEntriesFromExpression(body); len(entries) > 0 {
			return entries
		}
	}
	if entries := sourceObjectEntriesFromExpression(expression); len(entries) > 0 {
		return entries
	}
	return nil
}

func sourceEntriesByName(entries []sourceObjectEntry) map[string]sourceObjectEntry {
	if len(entries) == 0 {
		return nil
	}
	result := make(map[string]sourceObjectEntry, len(entries))
	for _, entry := range entries {
		result[entry.Name] = entry
	}
	return result
}

func canonicalizeSourceObjectEntries(
	source, file string,
	entries []sourceObjectEntry,
	state *buildState,
) []sourceObjectEntry {
	if len(entries) == 0 {
		return nil
	}
	result := make([]sourceObjectEntry, 0, len(entries))
	for _, entry := range entries {
		entry.Name = canonicalSourceObjectMemberName(source, file, entry.Name, state)
		result = append(result, entry)
	}
	return result
}

func canonicalSourceObjectMemberName(source, file, name string, state *buildState) string {
	if resolved := resolveComputedSourceObjectKeyName(source, file, name, state); resolved != "" {
		return resolved
	}
	return name
}

func resolveComputedSourceObjectKeyName(source, file, name string, state *buildState) string {
	text := strings.TrimSpace(name)
	if len(text) < 3 || text[0] != '[' || text[len(text)-1] != ']' {
		return ""
	}
	return sourceStringLiteralValueWithContext(source, file, text[1:len(text)-1], state)
}

func sourceStringLiteralValueWithContext(source, file, expression string, state *buildState) string {
	text := trimLeadingSourceTrivia(expression)
	if text == "" {
		return ""
	}
	for {
		trimmed := strings.TrimSpace(text)
		switch {
		case strings.HasSuffix(trimmed, " as const"):
			text = strings.TrimSpace(strings.TrimSuffix(trimmed, " as const"))
		default:
			text = unwrapWrappedExpression(trimmed)
			goto normalized
		}
	}

normalized:
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	switch text[0] {
	case '\'', '"':
		end, err := skipQuoted(text, 0, text[0])
		if err == nil && end == len(text)-1 {
			return text[1:end]
		}
	case '`':
		end, err := skipTemplate(text, 0)
		if err == nil && end == len(text)-1 && !strings.Contains(text, "${") {
			return text[1:end]
		}
	}

	identifier, ok := sourceIdentifierExpression(text)
	if !ok {
		return ""
	}
	if initializer := findLocalValueInitializer(source, identifier); initializer != "" {
		if literal := sourceStringLiteralValueWithContext(source, file, initializer, state); literal != "" {
			return literal
		}
	}
	if importedSource, importedFile, initializer, ok := sourceImportedValueInitializer(source, file, identifier, state); ok {
		if literal := sourceStringLiteralValueWithContext(importedSource, importedFile, initializer, state); literal != "" {
			return literal
		}
	}
	return ""
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

func sourceActionPayloadTypeFromSource(source, expression string) string {
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return ""
	}
	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
	}
	if body == "" {
		return ""
	}
	hints := sourceParameterTypeHints(source, normalizeFunctionParametersText(info.Parameters))
	return sourceObjectLiteralTypeTextWithHints(source, body, hints)
}

func sourceParameterTypeHints(source, parameters string) map[string]string {
	parsed, ok := parseFunctionParameters(parameters)
	if !ok || len(parsed) == 0 {
		return nil
	}
	hints := map[string]string{}
	for _, parameter := range parsed {
		name, ok := sourceParameterName(parameter.Text)
		if !ok || parameter.Type == "" {
			continue
		}
		typeText := normalizeSourceTypeText(parameter.Type)
		if expanded := expandLocalSourceTypeText(source, typeText); expanded != "" {
			typeText = normalizeSourceTypeText(expanded)
		}
		hints[name] = typeText
	}
	return hints
}

func sourceParameterTypeHintsWithDefault(source, parameters, defaultType string) map[string]string {
	hints := sourceParameterTypeHints(source, parameters)
	defaultType = normalizeSourceTypeText(strings.TrimSpace(defaultType))
	if defaultType == "" || isAnyLikeType(defaultType) {
		return hints
	}

	text := strings.TrimSpace(parameters)
	if len(text) < 2 || text[0] != '(' || text[len(text)-1] != ')' {
		return hints
	}
	parts, err := splitTopLevelList(text[1 : len(text)-1])
	if err != nil || len(parts) != 1 {
		return hints
	}
	parameterText := strings.TrimSpace(parts[0])
	if parameterText == "" {
		return hints
	}
	if strings.HasPrefix(parameterText, "...") {
		parameterText = strings.TrimSpace(parameterText[3:])
	}
	if index := strings.Index(parameterText, "="); index != -1 {
		parameterText = strings.TrimSpace(parameterText[:index])
	}
	if index := strings.Index(parameterText, ":"); index != -1 {
		parameterText = strings.TrimSpace(parameterText[:index])
	}

	if name, ok := sourceParameterName(parameterText); ok {
		if hints == nil {
			hints = map[string]string{}
		}
		if _, exists := hints[name]; !exists {
			hints[name] = defaultType
		}
		return hints
	}

	destructured := sourceDestructuredParameterTypeHints(source, parameterText, defaultType)
	if len(destructured) == 0 {
		return hints
	}
	if hints == nil {
		hints = map[string]string{}
	}
	for name, typeText := range destructured {
		if _, exists := hints[name]; !exists {
			hints[name] = typeText
		}
	}
	return hints
}

func sourceDestructuredParameterTypeHints(source, parameterText, defaultType string) map[string]string {
	text := strings.TrimSpace(parameterText)
	if text == "" || text[0] != '{' {
		return nil
	}
	end, err := findMatching(text, 0, '{', '}')
	if err != nil || trimExpressionEnd(text, end+1) != len(text) {
		return nil
	}

	expandedType := defaultType
	if expanded := expandLocalSourceTypeText(source, defaultType); expanded != "" {
		expandedType = normalizeSourceTypeText(expanded)
	}
	members, ok := parseObjectTypeMembers(expandedType)
	if !ok || len(members) == 0 {
		return nil
	}

	parts, err := splitTopLevelList(text[1:end])
	if err != nil {
		return nil
	}

	hints := map[string]string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if index := strings.Index(part, "="); index != -1 {
			part = strings.TrimSpace(part[:index])
		}

		sourceName := part
		localName := part
		if index := strings.Index(part, ":"); index != -1 {
			sourceName = strings.TrimSpace(part[:index])
			localName = strings.TrimSpace(part[index+1:])
			if defaultIndex := strings.Index(localName, "="); defaultIndex != -1 {
				localName = strings.TrimSpace(localName[:defaultIndex])
			}
		}

		name, ok := sourceParameterName(localName)
		if !ok {
			continue
		}
		typeText, ok := members[sourceName]
		if !ok {
			continue
		}
		hints[name] = normalizeSourceTypeText(typeText)
	}
	if len(hints) == 0 {
		return nil
	}
	return hints
}

func sourceObjectLiteralTypeTextWithHints(source, expression string, hints map[string]string) string {
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
			spreadType := sourceExpressionTypeTextWithHints(source, strings.TrimSpace(text[3:]), hints)
			if spreadType == "" {
				continue
			}
			if expanded := expandLocalSourceTypeText(source, spreadType); expanded != "" {
				spreadType = normalizeSourceTypeText(expanded)
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
			if shorthand, shorthandOK := sourceIdentifierExpression(text); shorthandOK {
				valueType := sourceExpressionTypeTextWithHints(source, shorthand, hints)
				if valueType != "" {
					properties[shorthand] = widenSourceObjectPropertyType(valueType)
				}
			}
			continue
		}
		valueType := sourceExpressionTypeTextWithHints(source, value, hints)
		if valueType == "" {
			continue
		}
		properties[name] = widenSourceObjectPropertyType(valueType)
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

func sourceExpressionTypeTextWithHints(source, expression string, hints map[string]string) string {
	text := trimLeadingSourceTrivia(expression)
	if text == "" {
		return ""
	}
	if objectType := sourceObjectLiteralTypeTextWithHints(source, text, hints); objectType != "" {
		return objectType
	}
	if logicalType := sourceLogicalExpressionTypeTextWithHints(source, text, hints); logicalType != "" {
		return logicalType
	}
	if memberType := sourceMemberAccessTypeTextWithHints(source, text, hints); memberType != "" {
		return memberType
	}
	if identifier, ok := sourceIdentifierExpression(text); ok && hints != nil {
		if hinted := strings.TrimSpace(hints[identifier]); hinted != "" {
			if expanded := expandLocalSourceTypeText(source, hinted); expanded != "" {
				return normalizeSourceTypeText(expanded)
			}
			return normalizeSourceTypeText(hinted)
		}
	}
	return sourceExpressionTypeText(source, expression)
}

func sourceExpressionTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
	text := trimLeadingSourceTrivia(expression)
	if text == "" {
		return ""
	}
	if functionType := sourceArrowFunctionTypeTextWithContext(source, file, text, state); functionType != "" {
		return functionType
	}
	if callType := sourceCallExpressionTypeTextWithContext(source, file, text, hints, state); callType != "" {
		return callType
	}
	if objectType := sourceObjectLiteralTypeTextWithHints(source, text, hints); objectType != "" {
		return objectType
	}
	if logicalType := sourceLogicalExpressionTypeTextWithHints(source, text, hints); logicalType != "" {
		return logicalType
	}
	if memberType := sourceMemberAccessTypeTextWithHints(source, text, hints); memberType != "" {
		return memberType
	}
	if identifier, ok := sourceIdentifierExpression(text); ok {
		if hinted := strings.TrimSpace(hints[identifier]); hinted != "" {
			if expanded := expandLocalSourceTypeText(source, hinted); expanded != "" {
				return normalizeSourceTypeText(expanded)
			}
			return normalizeSourceTypeText(hinted)
		}
		inferredFromInitializer := ""
		if initializer := findLocalValueInitializer(source, identifier); initializer != "" {
			if inferred := sourceExpressionTypeTextWithContext(source, file, initializer, nil, state); inferred != "" {
				inferredFromInitializer = inferred
				if !isAnyLikeType(inferred) {
					return inferred
				}
			}
		}
		if declared := findLocalValueDeclaredType(source, identifier); declared != "" {
			return declared
		}
		if inferredFromInitializer != "" {
			return inferredFromInitializer
		}
		if importedSource, importedFile, initializer, ok := sourceImportedValueInitializer(source, file, identifier, state); ok {
			if inferred := sourceExpressionTypeTextWithContext(importedSource, importedFile, initializer, nil, state); inferred != "" {
				return inferred
			}
		}
		if expanded := expandLocalSourceTypeText(source, identifier); expanded != "" {
			return expanded
		}
		if len(identifier) > 0 && unicode.IsUpper(rune(identifier[0])) {
			return identifier
		}
		return "any"
	}
	return sourceExpressionTypeTextWithHints(source, expression, hints)
}

func sourceArrowFunctionTypeTextWithContext(source, file, expression string, state *buildState) string {
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return ""
	}
	returnType := info.ExplicitReturn
	if returnType == "" {
		body := strings.TrimSpace(info.Body)
		if info.BlockBody {
			body = singleReturnExpression(body)
			if body == "" {
				body = blockReturnExpression(info.Body)
			}
		}
		if body != "" {
			if inferred := sourceExpressionTypeTextWithContext(source, file, body, nil, state); inferred != "" {
				returnType = normalizeInferredTypeText(inferred)
			}
		}
	}
	if returnType == "" {
		return ""
	}
	if info.Async {
		returnType = promiseTypeText(returnType)
	}
	return info.Parameters + " => " + returnType
}

func sourceCallExpressionTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	callee, ok := stripTrailingCallExpression(text)
	if !ok {
		return ""
	}
	calleeType := sourceExpressionTypeTextWithContext(source, file, callee, hints, state)
	if !isFunctionLikeTypeText(calleeType) {
		return ""
	}
	returnType, ok := parseFunctionReturnType(calleeType)
	if !ok {
		return ""
	}
	return normalizeInferredTypeText(returnType)
}

func sourceLogicalExpressionTypeTextWithHints(source, expression string, hints map[string]string) string {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" {
		return ""
	}
	for _, operator := range []string{"??", "||"} {
		index := findLastTopLevelOperator(text, operator)
		if index == -1 {
			continue
		}
		left := sourceExpressionTypeTextWithHints(source, strings.TrimSpace(text[:index]), hints)
		right := sourceExpressionTypeTextWithHints(source, strings.TrimSpace(text[index+len(operator):]), hints)
		if merged := mergeLogicalOperandTypes(left, right, operator); merged != "" {
			return merged
		}
	}
	return ""
}

func mergeLogicalOperandTypes(left, right, operator string) string {
	left = normalizeSourceTypeText(strings.TrimSpace(left))
	right = normalizeSourceTypeText(strings.TrimSpace(right))

	switch operator {
	case "??":
		left = normalizeInternalHelperParameterType(left)
	case "||":
		left = normalizeInternalHelperParameterType(left)
	}

	switch {
	case left == "":
		return right
	case right == "":
		return left
	case left == right:
		return left
	}

	if right == "string" || isQuotedString(right) {
		if left == "string" || strings.Contains(left, "string") {
			return "string"
		}
	}
	if right == "number" || isNumericLiteralType(right) {
		if left == "number" || strings.Contains(left, "number") {
			return "number"
		}
	}

	return normalizeSourceTypeText(left + " | " + right)
}

func sourceMemberAccessTypeTextWithHints(source, expression string, hints map[string]string) string {
	if len(hints) == 0 {
		return ""
	}
	segments, _, end, ok := parseMemberExpressionSegments(expression, 0, len(expression))
	if !ok || end != len(expression) || len(segments) < 2 {
		return ""
	}
	rootType := strings.TrimSpace(hints[segments[0]])
	if rootType == "" {
		return ""
	}
	return sourceMemberPathTypeText(source, rootType, segments[1:])
}

func sourceMemberPathTypeText(source, typeText string, path []string) string {
	current := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if current == "" {
		return ""
	}
	if expanded := expandLocalSourceTypeText(source, current); expanded != "" {
		current = normalizeSourceTypeText(expanded)
	}
	for _, segment := range path {
		members, ok := parseObjectTypeMembers(current)
		if !ok {
			return ""
		}
		next, ok := members[segment]
		if !ok {
			return ""
		}
		current = normalizeSourceTypeText(strings.TrimSpace(next))
		if expanded := expandLocalSourceTypeText(source, current); expanded != "" {
			current = normalizeSourceTypeText(expanded)
		}
	}
	return current
}

func sourceReducerStateType(expression string) string {
	return sourceReducerStateTypeWithContext(expression, "", expression, nil)
}

func sourceReducerStateTypeWithContext(source, file, expression string, state *buildState) string {
	text := strings.TrimSpace(expression)
	if text == "" {
		return ""
	}

	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(text, 0, len(text))
	if err == nil && ok {
		parts, err := splitTopLevelList(text[arrayStart+1 : arrayEnd])
		if err == nil && len(parts) > 0 {
			stateType := sourceReducerLiteralStateTypeWithContext(source, file, strings.TrimSpace(parts[0]), state)
			if len(parts) > 1 {
				if widened := sourceWidenReducerStateTypeFromHandlers(stateType, strings.TrimSpace(parts[len(parts)-1])); widened != "" {
					stateType = widened
				}
			}
			if stateType != "" {
				return normalizeInferredTypeText(stateType)
			}
		}
		return ""
	}

	return sourceReducerLiteralStateTypeWithContext(source, file, text, state)
}

func sourceLoaderDefaultType(source string, property SourceProperty) string {
	expression := sourcePropertyText(source, property)
	if expression == "" {
		return ""
	}

	if objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(expression, 0, len(expression)); err == nil && ok {
		properties, err := parseTopLevelProperties(expression, objectStart, objectEnd)
		if err == nil {
			for _, nested := range properties {
				if nested.Name != "__default" {
					continue
				}
				return normalizeInferredTypeText(sourceExpressionTypeText(source, sourcePropertyText(expression, nested)))
			}
		}
	}

	if element := firstTopLevelArrayElement(expression); element != "" {
		return normalizeInferredTypeText(sourceExpressionTypeText(source, element))
	}

	return ""
}

func sourceSelectorReturnType(expression string) string {
	if element := sourceSelectorProjectorElement(expression); element != "" {
		if _, returnType, ok := parseSourceArrowSignature(element); ok && returnType != "" {
			return returnType
		}
	}
	if _, returnType, ok := parseSourceArrowSignature(expression); ok && returnType != "" {
		return returnType
	}
	return ""
}

func sourceReducerLiteralStateType(expression string) string {
	return sourceReducerLiteralStateTypeWithContext(expression, "", expression, nil)
}

func sourceReducerLiteralStateTypeWithContext(source, file, expression string, state *buildState) string {
	expression = trimLeadingSourceTrivia(expression)
	if hinted := sourceAssertedType(expression); hinted != "" {
		return normalizeInferredTypeText(hinted)
	}
	if source != "" {
		text := trimLeadingSourceTrivia(unwrapWrappedExpression(expression))
		if callee, ok := stripTrailingCallExpression(text); ok {
			if identifier, ok := sourceIdentifierExpression(callee); ok {
				if returnType := findLocalFunctionReturnType(source, identifier); returnType != "" {
					return normalizeInferredTypeText(widenLiteralReducerStateType(returnType))
				}
			}
		}
		if inferred := sourceExpressionTypeTextWithContext(source, file, expression, nil, state); inferred != "" && !isAnyLikeType(inferred) {
			return normalizeInferredTypeText(widenLiteralReducerStateType(inferred))
		}
	}
	if inferred := sourceExpressionTypeText(expression, expression); inferred != "" {
		return normalizeInferredTypeText(widenLiteralReducerStateType(inferred))
	}
	return ""
}

func sourceWidenReducerStateTypeFromHandlers(stateType, expression string) string {
	originalStateType := normalizeInferredTypeText(strings.TrimSpace(stateType))
	switch {
	case originalStateType == "":
		return ""
	case originalStateType == "boolean":
		return "boolean"
	case !isBooleanLiteralType(originalStateType) && !isQuotedString(originalStateType) && !isNumericLiteralType(originalStateType):
		return originalStateType
	}
	stateType = widenLiteralReducerStateType(originalStateType)

	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(expression, 0, len(expression))
	if err != nil || !ok {
		return stateType
	}
	properties, err := parseTopLevelProperties(expression, objectStart, objectEnd)
	if err != nil {
		return stateType
	}

	for _, nested := range properties {
		returnType := strings.TrimSpace(sourceArrowReturnTypeText(expression, sourcePropertyText(expression, nested)))
		switch widenLiteralReducerStateType(returnType) {
		case "boolean":
			return "boolean"
		case "string":
			return "string"
		case "number":
			return "number"
		}
	}

	return stateType
}

func widenLiteralReducerStateType(typeText string) string {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	switch {
	case text == "":
		return ""
	case isBooleanLiteralType(text):
		return "boolean"
	case isQuotedString(text):
		return "string"
	case isNumericLiteralType(text):
		return "number"
	default:
		return text
	}
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

func sourceSelectorArrayParts(expression string) []string {
	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(expression, 0, len(expression))
	if err != nil || !ok {
		return nil
	}
	parts, err := splitTopLevelList(expression[arrayStart+1 : arrayEnd])
	if err != nil || len(parts) == 0 {
		return nil
	}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}

func sourceSelectorProjectorElement(expression string) string {
	parts := sourceSelectorArrayParts(expression)
	if len(parts) < 2 {
		return ""
	}
	index := sourceSelectorProjectorIndex(parts)
	if index < 0 || index >= len(parts) {
		return ""
	}
	return parts[index]
}

func sourceSelectorDependencyElements(expression string) []string {
	parts := sourceSelectorArrayParts(expression)
	if len(parts) < 2 {
		return nil
	}
	index := sourceSelectorProjectorIndex(parts)
	if index <= 0 {
		return nil
	}
	return parts[:index]
}

func sourceSelectorProjectorIndex(parts []string) int {
	for index := len(parts) - 1; index >= 0; index-- {
		if sourceSelectorProjectorCandidate(parts[index]) {
			return index
		}
	}
	return len(parts) - 1
}

func sourceSelectorProjectorCandidate(expression string) bool {
	text := strings.TrimSpace(expression)
	if text == "" {
		return false
	}
	if _, ok := parseSourceArrowInfo(text); ok {
		return true
	}
	return strings.HasPrefix(text, "function")
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

type sourceArrowInfo struct {
	Parameters     string
	ExplicitReturn string
	Body           string
	BlockBody      bool
	Async          bool
	ParameterNames []string
}

func parseSourceArrowInfo(expression string) (sourceArrowInfo, bool) {
	text := unwrapWrappedExpression(expression)
	if text == "" {
		return sourceArrowInfo{}, false
	}
	arrowIndex, ok, err := findTopLevelArrow(text, 0, len(text))
	if err != nil || !ok {
		return sourceArrowInfo{}, false
	}

	head := strings.TrimSpace(text[:arrowIndex])
	async := strings.HasPrefix(head, "async ")
	if async {
		head = strings.TrimSpace(head[len("async "):])
	}
	if head == "" {
		return sourceArrowInfo{}, false
	}
	if strings.HasPrefix(head, "<") {
		end, err := findMatching(head, 0, '<', '>')
		if err == nil && end+1 < len(head) {
			head = strings.TrimSpace(head[end+1:])
		}
	}
	if head == "" {
		return sourceArrowInfo{}, false
	}

	parameters := ""
	explicitReturn := ""
	if head[0] != '(' {
		parameters = normalizeSourceTypeText("(" + head + ")")
	} else {
		end, err := findMatching(head, 0, '(', ')')
		if err != nil {
			return sourceArrowInfo{}, false
		}
		parameters = normalizeSourceTypeText(head[:end+1])
		explicitReturn = strings.TrimSpace(head[end+1:])
		if strings.HasPrefix(explicitReturn, ":") {
			explicitReturn = normalizeSourceTypeText(explicitReturn[1:])
		} else {
			explicitReturn = ""
		}
	}

	body := strings.TrimSpace(text[arrowIndex+2:])
	return sourceArrowInfo{
		Parameters:     parameters,
		ExplicitReturn: explicitReturn,
		Body:           body,
		BlockBody:      strings.HasPrefix(body, "{"),
		Async:          async,
		ParameterNames: sourceParameterNames(parameters),
	}, true
}

func sourceArrowFunctionTypeText(source, expression string) string {
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return ""
	}
	returnType := info.ExplicitReturn
	if returnType == "" {
		returnType = sourceArrowReturnTypeText(source, expression)
	}
	if returnType == "" {
		return ""
	}
	return info.Parameters + " => " + returnType
}

func sourceArrowReturnTypeText(source, expression string) string {
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return ""
	}
	if info.ExplicitReturn != "" {
		return info.ExplicitReturn
	}
	if returnType := sourceReturnExpressionType(source, info.Body, info.BlockBody, info.ParameterNames, nil, info.Async); returnType != "" {
		return returnType
	}
	hints := sourceParameterTypeHints(source, info.Parameters)
	if len(hints) == 0 {
		return ""
	}

	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
		if body == "" {
			body = blockReturnExpression(info.Body)
		}
	}
	if body == "" {
		return ""
	}
	if hinted := sourceExpressionTypeTextWithHints(source, body, hints); hinted != "" {
		hinted = normalizeInferredTypeText(hinted)
		if info.Async {
			return promiseTypeText(hinted)
		}
		return hinted
	}
	return ""
}

func sourceKeyTypeFromProps(source string, property SourceProperty, propsType string) string {
	return sourceKeyTypeFromSource(source, "", property, propsType, nil)
}

func sourceKeyTypeFromSource(source, file string, property SourceProperty, propsType string, state *buildState) string {
	expression := sourcePropertyText(source, property)
	if expression == "" {
		return ""
	}
	if propsType != "" && !isAnyLikeType(propsType) {
		if info, ok := parseSourceArrowInfo(expression); ok {
			body := strings.TrimSpace(info.Body)
			if info.BlockBody {
				body = singleReturnExpression(body)
				if body == "" {
					body = blockReturnExpression(info.Body)
				}
			}
			if body != "" {
				hints := sourceParameterTypeHintsWithDefault(source, info.Parameters, propsType)
				if inferred := sourceExpressionTypeTextWithContext(source, file, body, hints, state); inferred != "" {
					return normalizeInferredTypeText(inferred)
				}
			}
		}
	}
	if inferred := sourceExpressionTypeTextWithContext(source, file, expression, nil, state); inferred != "" {
		if isFunctionLikeTypeText(inferred) {
			if returnType, ok := parseFunctionReturnType(inferred); ok {
				return normalizeInferredTypeText(returnType)
			}
		}
	}
	return ""
}

func sourceSelectorInferredType(logic ParsedLogic, source, file, expression string, state *buildState) string {
	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	if element := sourceSelectorProjectorElement(expression); element != "" {
		if info, ok := parseSourceArrowInfo(element); ok {
			if returnType := sourceReturnExpressionType(source, info.Body, info.BlockBody, info.ParameterNames, dependencyTypes, info.Async); returnType != "" {
				return normalizeRecoveredSelectorType(source, file, returnType)
			}
		}
	}
	if info, ok := parseSourceArrowInfo(expression); ok {
		if returnType := sourceReturnExpressionType(source, info.Body, info.BlockBody, info.ParameterNames, dependencyTypes, info.Async); returnType != "" {
			return normalizeRecoveredSelectorType(source, file, returnType)
		}
	}
	return ""
}

func normalizeRecoveredSelectorType(source, file, typeText string) string {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return ""
	}
	if expanded := expandImportedTypeAliasText(source, file, text); expanded != "" {
		return normalizeInferredTypeText(expanded)
	}
	return text
}

func expandImportedTypeAliasText(source, file, typeText string) string {
	if file == "" {
		return ""
	}
	identifier, ok := sourceIdentifierExpression(typeText)
	if !ok {
		return ""
	}

	candidate, ok := parseNamedValueImports(source)[identifier]
	if !ok {
		var resolved importCandidate
		resolved, ok = resolveRelativeExportCandidate(file, parseRelativeImportPaths(source), identifier, map[string]map[string]bool{})
		if !ok {
			return ""
		}
		candidate = importedValueCandidate{Path: resolved.Path, ImportedName: identifier}
	}
	if !strings.HasPrefix(candidate.Path, ".") {
		return ""
	}

	resolvedFile, ok := resolveLocalImportFile(file, candidate.Path)
	if !ok {
		return ""
	}
	content, err := os.ReadFile(resolvedFile)
	if err != nil {
		return ""
	}
	return expandLocalSourceTypeText(string(content), candidate.ImportedName)
}

func sourceSelectorDependencyTypes(logic ParsedLogic, source, file, expression string, state *buildState) []string {
	var dependencyTypes []string
	for _, part := range sourceSelectorDependencyElements(expression) {
		dependencyTypes = append(dependencyTypes, sourceSelectorDependencyTypesFromElement(logic, source, file, part, state)...)
	}
	return dependencyTypes
}

func sourceSelectorDependencyNames(expression string) []string {
	var dependencyNames []string
	for _, part := range sourceSelectorDependencyElements(expression) {
		dependencyNames = append(dependencyNames, sourceSelectorDependencyNamesFromElement(part)...)
	}
	return dependencyNames
}

func sourceSelectorDependencyNamesFromElement(expression string) []string {
	if info, ok := parseSourceArrowInfo(expression); ok {
		body := info.Body
		if info.BlockBody {
			body = singleReturnExpression(body)
		}
		if dependencyNames := sourceDependencyNamesFromReturnedArray(body); len(dependencyNames) > 0 {
			return dependencyNames
		}
		if _, fieldName, ok := parseSelectorReference(body); ok {
			return []string{fieldName}
		}
	}

	if dependencyNames := sourceDependencyNamesFromReturnedArray(expression); len(dependencyNames) > 0 {
		return dependencyNames
	}
	if _, fieldName, ok := parseSelectorReference(expression); ok {
		return []string{fieldName}
	}
	return nil
}

func sourceDependencyNamesFromReturnedArray(expression string) []string {
	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(expression, 0, len(expression))
	if err != nil || !ok {
		return nil
	}
	parts, err := splitTopLevelList(expression[arrayStart+1 : arrayEnd])
	if err != nil || len(parts) == 0 {
		return nil
	}

	names := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if _, fieldName, ok := parseSelectorReference(part); ok {
			names = append(names, fieldName)
			continue
		}
		names = append(names, "")
	}
	return names
}

func internalHelperParameterNames(dependencyNames, fallbackNames []string) []string {
	names := append([]string(nil), dependencyNames...)
	if len(names) == 0 {
		names = append(names, fallbackNames...)
	}
	if len(names) == 0 {
		return nil
	}

	seen := map[string]int{}
	resolved := make([]string, 0, len(names))
	for _, name := range names {
		baseName := strings.TrimSpace(name)
		if baseName == "" {
			baseName = "arg"
		}
		seen[baseName]++
		if seen[baseName] > 1 {
			resolved = append(resolved, fmt.Sprintf("%s%d", baseName, seen[baseName]))
			continue
		}
		resolved = append(resolved, baseName)
	}
	return resolved
}

func sourceSelectorDependencyTypesFromElement(logic ParsedLogic, source, file, expression string, state *buildState) []string {
	if info, ok := parseSourceArrowInfo(expression); ok {
		body := info.Body
		if info.BlockBody {
			body = singleReturnExpression(body)
		}
		if dependencyTypes := sourceDependencyTypesFromReturnedArray(logic, source, file, body, state); len(dependencyTypes) > 0 {
			return dependencyTypes
		}
		if dependencyType := resolveSelectorDependencyType(logic, source, file, body, state); dependencyType != "" {
			return []string{dependencyType}
		}
		if returnType := sourceArrowReturnTypeText(source, body); returnType != "" {
			return []string{returnType}
		}
		if returnType := sourceExpressionTypeText(source, body); returnType != "" {
			return []string{normalizeInferredTypeText(returnType)}
		}
	}

	if dependencyTypes := sourceDependencyTypesFromReturnedArray(logic, source, file, expression, state); len(dependencyTypes) > 0 {
		return dependencyTypes
	}
	if dependencyType := resolveSelectorDependencyType(logic, source, file, expression, state); dependencyType != "" {
		return []string{dependencyType}
	}
	if returnType := sourceArrowReturnTypeText(source, expression); returnType != "" {
		return []string{returnType}
	}
	return nil
}

func sourceDependencyTypesFromReturnedArray(logic ParsedLogic, source, file, expression string, state *buildState) []string {
	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(expression, 0, len(expression))
	if err != nil || !ok {
		return nil
	}
	parts, err := splitTopLevelList(expression[arrayStart+1 : arrayEnd])
	if err != nil || len(parts) == 0 {
		return nil
	}

	dependencyTypes := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if dependencyType := resolveSelectorDependencyType(logic, source, file, part, state); dependencyType != "" {
			dependencyTypes = append(dependencyTypes, dependencyType)
			continue
		}
		if returnType := sourceArrowReturnTypeText(source, part); returnType != "" {
			dependencyTypes = append(dependencyTypes, returnType)
			continue
		}
		if returnType := sourceExpressionTypeText(source, part); returnType != "" {
			dependencyTypes = append(dependencyTypes, normalizeInferredTypeText(returnType))
		}
	}
	return dependencyTypes
}

func resolveSelectorDependencyType(logic ParsedLogic, source, file, expression string, state *buildState) string {
	targetExpr, fieldName, ok := parseSelectorReference(expression)
	if !ok {
		return ""
	}

	if targetExpr == "selectors" || targetExpr == "values" {
		if field, found := findParsedField(mergeParsedFields(logic.Reducers, logic.Selectors...), fieldName); found {
			return field.Type
		}
		return ""
	}
	if isSimpleIdentifier(targetExpr) {
		if field, found := findParsedField(mergeParsedFields(logic.Reducers, logic.Selectors...), fieldName); found {
			return field.Type
		}
	}

	targetLogic, ok := resolveConnectedLogic(source, file, targetExpr, state)
	if !ok {
		return ""
	}
	if field, found := findParsedField(mergeParsedFields(targetLogic.Reducers, targetLogic.Selectors...), fieldName); found {
		return field.Type
	}
	return ""
}

func parseSelectorReference(expression string) (string, string, bool) {
	text := strings.TrimSpace(expression)
	for _, prefix := range []string{"selectors.", "values."} {
		if strings.HasPrefix(text, prefix) {
			fieldName := strings.TrimSpace(text[len(prefix):])
			if fieldName == "" {
				return "", "", false
			}
			for i := 0; i < len(fieldName); i++ {
				if !isIdentifierPart(fieldName[i]) {
					return "", "", false
				}
			}
			return strings.TrimSuffix(prefix, "."), fieldName, true
		}
	}
	for _, marker := range []string{".selectors.", ".values."} {
		index := strings.LastIndex(text, marker)
		if index == -1 {
			continue
		}
		targetExpr := strings.TrimSpace(text[:index])
		fieldName := strings.TrimSpace(text[index+len(marker):])
		if targetExpr == "" || fieldName == "" {
			continue
		}
		for i := 0; i < len(fieldName); i++ {
			if !isIdentifierPart(fieldName[i]) {
				return "", "", false
			}
		}
		return targetExpr, fieldName, true
	}
	if index := strings.LastIndex(text, "."); index != -1 {
		targetExpr := strings.TrimSpace(text[:index])
		fieldName := strings.TrimSpace(text[index+1:])
		if targetExpr != "" && fieldName != "" && isSimpleIdentifier(fieldName) && isSimpleIdentifier(targetExpr) {
			return targetExpr, fieldName, true
		}
	}
	return "", "", false
}

func sourceReturnExpressionType(source, body string, blockBody bool, parameterNames, dependencyTypes []string, async bool) string {
	expression := strings.TrimSpace(body)
	if blockBody {
		expression = singleReturnExpression(body)
		if expression == "" {
			expression = blockReturnExpression(body)
		}
	}
	if expression == "" {
		return ""
	}

	if dependencyType := sourceDependencyTypeForReturnExpression(source, expression, parameterNames, dependencyTypes); dependencyType != "" {
		if async {
			return promiseTypeText(dependencyType)
		}
		return normalizeInferredTypeText(dependencyType)
	}
	if derivedType := sourceCommonReturnExpressionType(expression); derivedType != "" {
		if async {
			return promiseTypeText(derivedType)
		}
		return derivedType
	}

	if inferred := sourceExpressionTypeText(source, expression); inferred != "" {
		inferred = normalizeInferredTypeText(inferred)
		if async {
			return promiseTypeText(inferred)
		}
		return inferred
	}

	return ""
}

func blockReturnExpression(body string) string {
	text := strings.TrimSpace(body)
	if len(text) < 2 || text[0] != '{' {
		return ""
	}
	end, err := findMatching(text, 0, '{', '}')
	if err != nil || trimExpressionEnd(text, end+1) != len(text) {
		return ""
	}
	inner := strings.TrimSpace(text[1:end])
	if inner == "" {
		return ""
	}
	returnIndex := lastTopLevelKeyword(inner, "return")
	if returnIndex == -1 {
		return ""
	}
	expression := strings.TrimSpace(inner[returnIndex+len("return"):])
	expression = strings.TrimSpace(strings.TrimSuffix(expression, ";"))
	if expression == "" {
		return ""
	}
	if identifier, ok := sourceIdentifierExpression(expression); ok {
		if initializer := findLocalValueInitializer(inner[:returnIndex], identifier); initializer != "" {
			return initializer
		}
	}
	return expression
}

func sourceCommonReturnExpressionType(expression string) string {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" {
		return ""
	}
	if len(text) >= 2 && text[0] == '`' && text[len(text)-1] == '`' {
		return "string"
	}
	for _, marker := range []string{
		".join(",
		".toUpperCase(",
		".toLowerCase(",
		".trim(",
		".charAt(",
		".slice(",
		".substring(",
		".replace(",
		".padStart(",
		".padEnd(",
		".repeat(",
		".normalize(",
		".concat(",
		".toString(",
	} {
		if strings.Contains(text, marker) {
			return "string"
		}
	}
	return ""
}

func findLastTopLevelOperator(source, operator string) int {
	if operator == "" {
		return -1
	}

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
		}

		if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 && strings.HasPrefix(source[i:], operator) {
			last = i
			i += len(operator) - 1
		}
	}
	return last
}

func sourceDependencyTypeForReturnExpression(source, expression string, parameterNames, dependencyTypes []string) string {
	text := strings.TrimSpace(expression)
	if len(parameterNames) == 0 || len(dependencyTypes) == 0 {
		return ""
	}

	for index, name := range parameterNames {
		if name == "" || index >= len(dependencyTypes) {
			continue
		}
		if text == name || isSortedCopyOfParameter(text, name) {
			return publicSelectorDependencyType(source, dependencyTypes[index])
		}
	}
	return ""
}

func publicSelectorDependencyType(source, typeText string) string {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return ""
	}
	if expanded := expandLocalSourceTypeText(source, text); expanded != "" {
		return normalizeInferredTypeText(expanded)
	}
	return text
}

func singleReturnExpression(body string) string {
	text := strings.TrimSpace(body)
	if len(text) < 2 || text[0] != '{' {
		return strings.TrimSpace(body)
	}
	end, err := findMatching(text, 0, '{', '}')
	if err != nil || trimExpressionEnd(text, end+1) != len(text) {
		return ""
	}
	inner := strings.TrimSpace(text[1:end])
	if !strings.HasPrefix(inner, "return ") {
		return ""
	}
	inner = strings.TrimSpace(inner[len("return "):])
	inner = strings.TrimSuffix(inner, ";")
	return strings.TrimSpace(inner)
}

func isSortedCopyOfParameter(expression, parameterName string) bool {
	text := strings.Join(strings.Fields(strings.TrimSpace(expression)), "")
	pattern := "[..." + parameterName + "].sort("
	return strings.HasPrefix(text, pattern)
}

func sourceParameterNames(parameters string) []string {
	text := strings.TrimSpace(parameters)
	if len(text) < 2 || text[0] != '(' || text[len(text)-1] != ')' {
		return nil
	}
	parts, err := splitTopLevelList(text[1 : len(text)-1])
	if err != nil || len(parts) == 0 {
		return nil
	}
	names := make([]string, 0, len(parts))
	for _, parameter := range parts {
		if name, ok := sourceParameterName(parameter); ok {
			names = append(names, name)
		} else {
			names = append(names, "")
		}
	}
	return names
}

func sourceParameterName(text string) (string, bool) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "...")
	if index := strings.Index(text, "="); index != -1 {
		text = strings.TrimSpace(text[:index])
	}
	if index := strings.Index(text, ":"); index != -1 {
		text = strings.TrimSpace(text[:index])
	}
	if text == "" || !isIdentifierStart(text[0]) {
		return "", false
	}
	for i := 1; i < len(text); i++ {
		if !isIdentifierPart(text[i]) {
			return "", false
		}
	}
	return text, true
}

func promiseTypeText(typeText string) string {
	typeText = strings.TrimSpace(typeText)
	if typeText == "" || strings.HasPrefix(typeText, "Promise<") {
		return typeText
	}
	return "Promise<" + typeText + ">"
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

func trimLeadingSourceTrivia(expression string) string {
	text := strings.TrimSpace(expression)
	if text == "" {
		return ""
	}
	start := skipTrivia(text, 0)
	if start >= len(text) {
		return ""
	}
	return strings.TrimSpace(text[start:])
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
	return normalizeSourceTypeTextWithOptions(text, true)
}

func normalizeSourceTypeTextWithOptions(text string, collapseOptionalUndefined bool) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = normalizeTypeLiteralText(text)
	text = strings.Join(strings.Fields(text), " ")
	text = strings.ReplaceAll(text, "( ", "(")
	text = strings.ReplaceAll(text, " )", ")")
	text = strings.ReplaceAll(text, ",)", ")")
	if collapseOptionalUndefined {
		text = optionalUndefinedUnionPattern.ReplaceAllString(text, "$1")
	}
	return text
}

func normalizeTypeLiteralText(text string) string {
	var builder strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			builder.WriteByte(text[i])
			continue
		}

		end, err := findMatching(text, i, '{', '}')
		if err != nil {
			builder.WriteByte(text[i])
			continue
		}

		inner := normalizeTypeLiteralText(text[i+1 : end])
		normalized, ok := normalizeTypeLiteralBody(inner)
		if !ok {
			builder.WriteByte(text[i])
			continue
		}

		builder.WriteString("{")
		if normalized != "" {
			builder.WriteString(" ")
			builder.WriteString(normalized)
			builder.WriteString(" ")
		}
		builder.WriteString("}")
		i = end
	}
	return builder.String()
}

func normalizeTypeLiteralBody(text string) (string, bool) {
	body := strings.TrimSpace(text)
	if body == "" {
		return "", true
	}

	entries, err := splitTopLevelTypeMembers(body)
	if err != nil {
		return "", false
	}

	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if name, value, ok := splitTopLevelTypeMember(entry); ok {
			parts = append(parts, strings.TrimSpace(name)+": "+normalizeSourceTypeText(value))
			continue
		}
		parts = append(parts, normalizeSourceTypeText(entry))
	}
	if len(parts) == 0 {
		return "", true
	}
	return strings.Join(parts, "; ") + ";", true
}

func splitTopLevelTypeMember(entry string) (string, string, bool) {
	index := findTopLevelDelimiter(entry, ':')
	if index == -1 {
		return "", "", false
	}
	name := strings.TrimSpace(entry[:index])
	value := strings.TrimSpace(entry[index+1:])
	if name == "" || value == "" {
		return "", "", false
	}
	return name, value, true
}

func findTopLevelDelimiter(text string, delimiter byte) int {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0

	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\'':
			end, err := skipQuoted(text, i, '\'')
			if err != nil {
				return -1
			}
			i = end
		case '"':
			end, err := skipQuoted(text, i, '"')
			if err != nil {
				return -1
			}
			i = end
		case '`':
			end, err := skipTemplate(text, i)
			if err != nil {
				return -1
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
			if shouldOpenAngle(text, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		default:
			if text[i] == delimiter && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				return i
			}
		}
	}

	return -1
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
	sourceEntries := sectionSourceEntries(source, property)
	listeners := make([]ParsedListener, 0, len(section.Members)+len(sourceEntries)+len(resolvedExternal))
	var imports []TypeImport
	memberByName := map[string]MemberReport{}
	for _, member := range section.Members {
		memberByName[member.Name] = member
	}

	seenNames := map[string]bool{}
	appendListener := func(name string, member *MemberReport, allowGenericFallback bool) {
		name = strings.TrimSpace(name)
		if name == "" || seenNames[name] {
			return
		}

		if action, ok := findParsedAction(logic.Actions, name); ok {
			payloadType := fallbackType(action.PayloadType, "any")
			listeners = append(listeners, ParsedListener{
				Name:        name,
				PayloadType: payloadType,
				ActionType:  fmt.Sprintf("{ type: %s; payload: %s }", quoteString(actionTypeString(logic, action.Name)), payloadType),
			})
			seenNames[name] = true
			return
		}

		if resolved, ok := resolvedExternal[name]; ok {
			listeners = append(listeners, ParsedListener{
				Name:        name,
				PayloadType: resolved.PayloadType,
				ActionType:  resolved.ActionType,
			})
			imports = mergeTypeImports(imports, resolved.Imports)
			seenNames[name] = true
			return
		}

		if !allowGenericFallback || member == nil {
			return
		}

		payloadType := "any"
		if payload := listenerPayloadTypeFromMember(*member); payload != "" {
			payloadType = payload
		}
		listeners = append(listeners, ParsedListener{
			Name:        name,
			PayloadType: payloadType,
			ActionType:  "{ type: string; payload: any }",
		})
		seenNames[name] = true
	}

	if len(sourceEntries) > 0 {
		for _, entry := range sourceEntries {
			name := entry.Name
			if _, typeName, _, ok := resolveActionReferenceFromSourceKey(source, file, entry.Name, state); ok {
				name = typeName
			}
			if member, ok := memberByName[name]; ok {
				appendListener(name, &member, false)
			} else {
				appendListener(name, nil, false)
			}
		}
		return listeners, imports
	}

	for _, member := range section.Members {
		member := member
		appendListener(member.Name, &member, true)
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
	imports := filterImportsByTypeTexts(targetLogic.Imports, []string{
		action.FunctionType,
		action.PayloadType,
	})
	if targetLogic.File != "" && file != "" {
		imports = rebaseTypeImports(imports, targetLogic.File, file)
	}
	return action, actionTypeString(targetLogic, action.Name), imports, true
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
	return normalizeInferredTypeText(firstParameterType(parameters))
}

func parseSharedListeners(section SectionReport) []ParsedSharedListener {
	listeners := make([]ParsedSharedListener, 0, len(section.Members))
	for _, member := range section.Members {
		parameters, _, ok := splitFunctionType(member.TypeString)
		if !ok {
			continue
		}
		payloadType := normalizeInferredTypeText(firstParameterType(parameters))
		if isAnyLikeType(payloadType) {
			continue
		}
		listeners = append(listeners, ParsedSharedListener{
			Name:        member.Name,
			PayloadType: payloadType,
			ActionType:  normalizeSourceTypeText(fmt.Sprintf("{ type: string; payload: %s }", payloadType)),
		})
	}
	return listeners
}

func parseInternalReducerActions(source, file string, reducersProperty, listenersProperty SourceProperty, state *buildState) []ParsedAction {
	actionsByName := map[string]ParsedAction{}

	appendAction := func(action ParsedAction, actionType string) {
		action.Name = actionType
		actionsByName[actionType] = action
	}

	for _, keyText := range collectReducerActionReferenceKeys(source, reducersProperty) {
		action, actionType, _, ok := resolveActionReferenceFromSourceKey(source, file, keyText, state)
		if !ok {
			continue
		}
		appendAction(action, actionType)
	}
	for _, entry := range sectionSourceEntries(source, listenersProperty) {
		action, actionType, _, ok := resolveActionReferenceFromSourceKey(source, file, entry.Name, state)
		if !ok {
			continue
		}
		appendAction(action, actionType)
	}

	if len(actionsByName) == 0 {
		return nil
	}

	names := make([]string, 0, len(actionsByName))
	for name := range actionsByName {
		names = append(names, name)
	}
	sort.Strings(names)

	actions := make([]ParsedAction, 0, len(names))
	for _, name := range names {
		actions = append(actions, actionsByName[name])
	}
	return actions
}

func collectReducerActionReferenceKeys(source string, property SourceProperty) []string {
	if source == "" || property.ValueEnd <= property.ValueStart {
		return nil
	}
	entriesByName := sourceEntriesByName(sectionSourceEntries(source, property))
	if len(entriesByName) == 0 {
		return nil
	}

	seen := map[string]bool{}
	var keys []string
	for _, entry := range entriesByName {
		handlersText := reducerHandlersExpression(entry.Value)
		if handlersText == "" {
			continue
		}
		for _, entry := range sourceObjectEntriesFromExpression(handlersText) {
			if !strings.Contains(entry.Name, ".actionTypes.") || seen[entry.Name] {
				continue
			}
			seen[entry.Name] = true
			keys = append(keys, entry.Name)
		}
	}
	return keys
}

func reducerHandlersExpression(expression string) string {
	text := strings.TrimSpace(expression)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "[") {
		arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(text, 0, len(text))
		if err != nil || !ok {
			return ""
		}
		parts, err := splitTopLevelList(text[arrayStart+1 : arrayEnd])
		if err != nil || len(parts) < 2 {
			return ""
		}
		return strings.TrimSpace(parts[1])
	}
	if strings.HasPrefix(text, "{") {
		return text
	}
	return ""
}

func sourceObjectEntriesFromExpression(expression string) []sourceObjectEntry {
	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(expression, 0, len(expression))
	if err != nil || !ok {
		return nil
	}
	segments, err := splitTopLevelSourceSegments(expression, objectStart+1, objectEnd)
	if err != nil {
		return nil
	}
	entries := make([]sourceObjectEntry, 0, len(segments))
	for _, segment := range segments {
		name, value, ok := splitTopLevelProperty(segment.Text)
		if !ok {
			continue
		}
		entries = append(entries, sourceObjectEntry{Name: name, Value: value})
	}
	return entries
}

func collectReducerActionPayloadHints(section SectionReport) map[string]string {
	hints := map[string]string{}
	for _, member := range section.Members {
		handlersText := reducerHandlersType(member.TypeString)
		if handlersText == "" {
			continue
		}
		handlers, ok := parseObjectTypeMembers(handlersText)
		if !ok {
			continue
		}
		for name, handlerType := range handlers {
			parameters, _, ok := splitFunctionType(handlerType)
			if !ok {
				continue
			}
			payloadType := nthParameterType(parameters, 1)
			if payloadType == "" {
				continue
			}
			payloadType = normalizeInferredTypeText(payloadType)
			if shouldRefineActionPayloadType(hints[name], payloadType) {
				hints[name] = payloadType
			}
		}
	}
	return hints
}

func reducerHandlersType(typeText string) string {
	text := strings.TrimSpace(typeText)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "[") {
		inner := strings.TrimSpace(text[1:])
		if strings.HasSuffix(inner, "]") {
			inner = strings.TrimSpace(inner[:len(inner)-1])
		}
		parts, err := splitTopLevelList(inner)
		if err != nil || len(parts) < 2 {
			return ""
		}
		return strings.TrimSpace(parts[1])
	}
	if strings.HasPrefix(text, "{") {
		return text
	}
	return ""
}

func nthParameterType(parameters string, index int) string {
	params, ok := parseFunctionParameters(parameters)
	if !ok || index < 0 || index >= len(params) {
		return ""
	}
	return params[index].Type
}

func refineActionPayloadTypes(actions []ParsedAction, hints map[string]string) []ParsedAction {
	if len(actions) == 0 || len(hints) == 0 {
		return actions
	}
	refined := append([]ParsedAction(nil), actions...)
	for index, action := range refined {
		hinted, ok := hints[action.Name]
		if !ok || !shouldRefineActionPayloadType(action.PayloadType, hinted) {
			continue
		}
		refined[index].PayloadType = hinted
	}
	return refined
}

func shouldRefineActionPayloadType(current, hinted string) bool {
	current = strings.TrimSpace(current)
	hinted = strings.TrimSpace(hinted)
	if hinted == "" {
		return false
	}
	if current == "" {
		return true
	}
	if current == hinted {
		return false
	}
	if isAnyLikeType(current) || strings.Contains(current, "...") || isGenericIndexSignatureType(current) {
		return true
	}
	if strings.Contains(current, "| null") && !strings.Contains(hinted, "null") {
		return true
	}
	if strings.Contains(current, "| undefined") && !strings.Contains(hinted, "undefined") {
		return true
	}
	return false
}

func isGenericIndexSignatureType(typeText string) bool {
	text := normalizeInferredTypeText(typeText)
	return strings.Contains(text, "[x: string]: any") || strings.Contains(text, "[x:string]: any")
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

func mergeParsedActionsPreferExisting(existing []ParsedAction, extra ...ParsedAction) []ParsedAction {
	indexByName := make(map[string]int, len(existing))
	merged := append([]ParsedAction(nil), existing...)
	for index, action := range merged {
		indexByName[action.Name] = index
	}
	for _, action := range extra {
		if _, ok := indexByName[action.Name]; ok {
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
	s.primaryProjectID = ""
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
	if len(snapshot.Projects) == 1 {
		if project := tsgoapi.PickProject(snapshot.Projects, s.configFile); project != nil {
			s.primaryProjectID = project.ID
		}
	}
	if s.projectByFile == nil {
		s.projectByFile = map[string]string{}
	}
	return nil
}

func (s *buildState) projectIDForFile(file string) (string, error) {
	if s != nil && s.primaryProjectID != "" {
		return s.primaryProjectID, nil
	}
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
	report := &Report{
		BinaryPath: s.binaryPath,
		ProjectDir: s.projectDir,
		ConfigFile: s.configFile,
		File:       file,
	}
	if !strings.Contains(source, "kea") {
		return report, source, nil
	}
	if len(logics) == 0 {
		return report, source, nil
	}

	if err := s.ensureAPIClient(); err != nil {
		return nil, "", err
	}
	projectID, err := s.projectIDForFile(file)
	if err != nil {
		return nil, "", err
	}

	report.Snapshot = s.apiSnapshot
	report.Config = s.config
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
					if hasLocalLogic {
						if parsedAction, found := findParsedAction(targetLogic.Actions, name.SourceName); found && shouldPreferParsedConnectedAction(action, parsedAction) {
							action = parsedAction
							action.Name = name.LocalName
						}
					}
					if hasListenerSection {
						if member, found := findMemberReport(listenerSection.Members, name.LocalName); found {
							if payload := listenerPayloadTypeFromMember(member); payload != "" && !strings.Contains(payload, "...") && shouldRefineActionPayloadType(action.PayloadType, payload) {
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
				imports = mergeTypeImports(imports, rebaseTypeImports(filterImportsByTypeTexts(targetLogic.Imports, addedTypeTexts), targetLogic.File, file))
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
					if hasLocalLogic {
						if field, found := findParsedField(targetFields, name.SourceName); found && shouldPreferParsedConnectedField(member.TypeString, field.Type) {
							copied := field
							copied.Name = name.LocalName
							parsed.Selectors = mergeParsedFields(parsed.Selectors, copied)
							addedTypeTexts = append(addedTypeTexts, copied.Type)
							continue
						}
					}
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
				imports = mergeTypeImports(imports, rebaseTypeImports(filterImportsByTypeTexts(targetLogic.Imports, addedTypeTexts), targetLogic.File, file))
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
		if logic, ok := resolveImportedConnectedLogic(file, candidate.Path, packageConnectedLogicName(target, candidate.ImportedName), state, target.LogicName, candidate.ImportedName, target.BaseAlias); ok {
			return logic, true
		}
	}

	if candidate, ok := parseDefaultValueImports(source)[target.BaseAlias]; ok {
		if logic, ok := resolveImportedConnectedLogic(file, candidate.Path, packageConnectedLogicName(target, target.BaseAlias), state, target.LogicName, target.BaseAlias); ok {
			return logic, true
		}
	}

	if importPath, ok := parseNamespaceValueImports(source)[target.BaseAlias]; ok {
		if logic, ok := resolveImportedConnectedLogic(file, importPath, target.LogicName, state, target.LogicName); ok {
			return logic, true
		}
	}

	return ParsedLogic{}, false
}

func resolveImportedConnectedLogic(file, importPath, packageLogicName string, state *buildState, names ...string) (ParsedLogic, bool) {
	if resolvedFile, ok := resolveImportFile(file, importPath, state); ok {
		logics, err := state.loadLogics(resolvedFile)
		if err != nil {
			return ParsedLogic{}, false
		}
		return findConnectedLogic(logics, names...)
	}

	if strings.TrimSpace(packageLogicName) == "" {
		return ParsedLogic{}, false
	}
	if logic, ok := loadPackageTypegenLogic(file, importPath, packageLogicName); ok {
		return logic, true
	}
	return ParsedLogic{}, false
}

func packageConnectedLogicName(target connectedTargetReference, fallback string) string {
	if strings.TrimSpace(target.LogicName) != "" {
		return strings.TrimSpace(target.LogicName)
	}
	return strings.TrimSpace(fallback)
}

func shouldPreferParsedConnectedAction(symbolAction, parsedAction ParsedAction) bool {
	return shouldRefineActionPayloadType(symbolAction.PayloadType, parsedAction.PayloadType) ||
		isGenericIndexSignatureType(symbolAction.PayloadType) ||
		strings.Contains(symbolAction.FunctionType, "[x: string]: any")
}

func shouldPreferParsedConnectedField(symbolType, parsedType string) bool {
	return connectedFieldTypeQuality(parsedType) > connectedFieldTypeQuality(symbolType)
}

func connectedFieldTypeQuality(typeText string) int {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return 0
	}
	score := 1
	if !isAnyLikeType(text) {
		score += 3
	}
	if !typeTextContainsStandaloneToken(text, "any") {
		score += 2
	}
	if !typeTextContainsStandaloneToken(text, "unknown") {
		score += 2
	}
	if !isGenericIndexSignatureType(text) {
		score++
	}
	if !strings.Contains(text, "...") {
		score++
	}
	return score
}

func loadPackageTypegenLogic(file, importPath, logicName string) (ParsedLogic, bool) {
	logicName = strings.TrimSpace(logicName)
	if logicName == "" {
		return ParsedLogic{}, false
	}
	rootDir, rootName, _, ok := resolvePackageRoot(file, importPath)
	if !ok {
		return ParsedLogic{}, false
	}
	typeName := logicName + "Type"

	typeFile, source, ok := findPackageLogicTypeFile(rootDir, typeName)
	if !ok {
		return ParsedLogic{}, false
	}
	logic, ok := parseGeneratedLogicTypeFile(typeFile, source, logicName, typeName)
	if !ok {
		return ParsedLogic{}, false
	}
	logic.Imports = packageQualifiedImports(
		rootDir,
		rootName,
		typeFile,
		mergeTypeImports(logic.Imports, collectTypeImports(source, typeFile, logic)),
	)
	return logic, true
}

func packageQualifiedImports(rootDir, rootName, typeFile string, imports []TypeImport) []TypeImport {
	if len(imports) == 0 {
		return nil
	}
	qualified := make([]TypeImport, 0, len(imports))
	for _, item := range imports {
		path := item.Path
		if strings.HasPrefix(path, ".") {
			resolvedFile, ok := resolveLocalImportFile(typeFile, path)
			if ok {
				if importPath := packageModuleImportPath(rootName, rootDir, resolvedFile); importPath != "" {
					path = importPath
				}
			}
		}
		qualified = append(qualified, TypeImport{Path: path, Names: append([]string(nil), item.Names...)})
	}
	return qualified
}

func rebaseTypeImports(imports []TypeImport, fromFile, toFile string) []TypeImport {
	if len(imports) == 0 {
		return nil
	}

	fromFile = filepath.Clean(fromFile)
	toFile = filepath.Clean(toFile)
	if fromFile == "" || toFile == "" || fromFile == toFile {
		return imports
	}

	rebased := make([]TypeImport, 0, len(imports))
	for _, item := range imports {
		path := item.Path
		if strings.HasPrefix(path, ".") {
			if resolvedFile, ok := resolveLocalImportFile(fromFile, path); ok {
				if importPath, ok := relativeImportPath(toFile, resolvedFile); ok {
					path = importPath
				}
			}
		}
		rebased = append(rebased, TypeImport{Path: path, Names: append([]string(nil), item.Names...)})
	}
	return rebased
}

func relativeImportPath(fromFile, toFile string) (string, bool) {
	if fromFile == "" || toFile == "" {
		return "", false
	}

	relative, err := filepath.Rel(filepath.Dir(fromFile), toFile)
	if err != nil {
		return "", false
	}
	relative = filepath.ToSlash(relative)
	relative = strings.TrimSuffix(relative, filepath.Ext(relative))
	if !strings.HasPrefix(relative, ".") {
		relative = "./" + relative
	}
	return relative, true
}

func findPackageLogicTypeFile(rootDir, typeName string) (string, string, bool) {
	var (
		foundFile   string
		foundSource string
	)
	_ = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".d.ts") {
			return nil
		}
		sourceBytes, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		source := string(sourceBytes)
		if _, ok := findGeneratedLogicInterfaceBody(source, typeName); !ok {
			return nil
		}
		foundFile = path
		foundSource = source
		return filepath.SkipAll
	})
	return foundFile, foundSource, foundFile != ""
}

func parseGeneratedLogicTypeFile(file, source, logicName, typeName string) (ParsedLogic, bool) {
	body, ok := findGeneratedLogicInterfaceBody(source, typeName)
	if !ok {
		return ParsedLogic{}, false
	}
	properties, err := splitTopLevelTypeMembers(body)
	if err != nil {
		return ParsedLogic{}, false
	}
	propertyMap := map[string]string{}
	for _, property := range properties {
		name, value, ok := splitTopLevelProperty(property)
		if !ok {
			continue
		}
		propertyMap[name] = strings.TrimSpace(value)
	}

	logic := ParsedLogic{
		Name:       logicName,
		TypeName:   typeName,
		File:       file,
		PathString: "",
	}
	if pathText, ok := propertyMap["pathString"]; ok {
		logic.PathString = strings.Trim(unquoteString(pathText), "")
	}
	if actionCreatorsText, ok := propertyMap["actionCreators"]; ok {
		logic.Actions = parseGeneratedActionCreatorsType(actionCreatorsText)
	}
	if selectorsText, ok := propertyMap["selectors"]; ok {
		logic.Selectors = parseGeneratedFieldTypes(selectorsText)
	}
	if valuesText, ok := propertyMap["values"]; ok {
		logic.Selectors = mergeParsedFields(logic.Selectors, parseGeneratedFieldTypes(valuesText)...)
	}
	return logic, true
}

func findGeneratedLogicInterfaceBody(source, typeName string) (string, bool) {
	pattern := regexp.MustCompile(`export\s+interface\s+` + regexp.QuoteMeta(typeName) + `\s+extends\s+Logic\s*\{`)
	location := pattern.FindStringIndex(source)
	if location == nil {
		return "", false
	}
	braceIndex := strings.Index(source[location[0]:], "{")
	if braceIndex == -1 {
		return "", false
	}
	start := location[0] + braceIndex
	end, err := findMatching(source, start, '{', '}')
	if err != nil || end <= start {
		return "", false
	}
	return strings.TrimSpace(source[start+1 : end]), true
}

func parseGeneratedActionCreatorsType(typeText string) []ParsedAction {
	properties, ok := parseObjectTypeMembers(typeText)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)

	actions := make([]ParsedAction, 0, len(names))
	for _, name := range names {
		functionType := strings.TrimSpace(properties[name])
		parameters, returnType, ok := splitFunctionType(functionType)
		if !ok {
			continue
		}
		returnMembers, ok := parseObjectTypeMembers(returnType)
		if !ok {
			continue
		}
		payloadType := strings.TrimSpace(returnMembers["payload"])
		if payloadType == "" {
			payloadType = "any"
		}
		actions = append(actions, ParsedAction{
			Name:         name,
			FunctionType: parameters + " => " + payloadType,
			PayloadType:  payloadType,
		})
	}
	return actions
}

func parseGeneratedFieldTypes(typeText string) []ParsedField {
	properties, ok := parseObjectTypeMembers(typeText)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)

	fields := make([]ParsedField, 0, len(names))
	for _, name := range names {
		fieldType := strings.TrimSpace(properties[name])
		if parameters, returnType, ok := splitFunctionType(fieldType); ok && strings.Contains(parameters, "state: any") {
			fieldType = strings.TrimSpace(returnType)
		}
		fields = append(fields, ParsedField{Name: name, Type: fieldType})
	}
	return fields
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
		if stripped, ok := stripTrailingNonNullAssertion(text); ok {
			text = stripped
			changed = true
		}
		if stripped, ok := stripTopLevelTypeAssertionSuffix(text); ok {
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

	parts, ok := parseConnectedTargetSegments(text)
	if !ok || len(parts) == 0 {
		return connectedTargetReference{}, false
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
				if strings.HasSuffix(prefix, "?.") {
					prefix = strings.TrimSpace(prefix[:len(prefix)-2])
				}
				return prefix, true
			}
		}
	}
	return text, false
}

func stripTrailingNonNullAssertion(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasSuffix(text, "!") {
		return text, false
	}
	stripped := strings.TrimSpace(text[:len(text)-1])
	if stripped == "" {
		return text, false
	}
	return stripped, true
}

func stripTopLevelTypeAssertionSuffix(text string) (string, bool) {
	for _, keyword := range []string{" satisfies ", " as "} {
		index := findLastTopLevelKeyword(text, keyword)
		if index <= 0 {
			continue
		}
		return strings.TrimSpace(text[:index]), true
	}
	return text, false
}

func findLastTopLevelKeyword(text, keyword string) int {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0
	lastIndex := -1

	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\'':
			end, err := skipQuoted(text, i, '\'')
			if err != nil {
				return lastIndex
			}
			i = end
		case '"':
			end, err := skipQuoted(text, i, '"')
			if err != nil {
				return lastIndex
			}
			i = end
		case '`':
			end, err := skipTemplate(text, i)
			if err != nil {
				return lastIndex
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
			if shouldOpenAngle(text, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		}

		if parenDepth != 0 || bracketDepth != 0 || braceDepth != 0 || angleDepth != 0 {
			continue
		}
		if strings.HasPrefix(text[i:], keyword) {
			lastIndex = i
		}
	}

	return lastIndex
}

func parseConnectedTargetSegments(text string) ([]string, bool) {
	index := skipWhitespaceForward(text, 0)
	segment, next, ok := readConnectedTargetIdentifier(text, index)
	if !ok {
		return nil, false
	}

	segments := []string{segment}
	index = next
	for {
		index = skipWhitespaceForward(text, index)
		if index >= len(text) {
			break
		}

		switch {
		case strings.HasPrefix(text[index:], "?.["):
			index += 3
			segment, next, ok = readConnectedTargetBracketProperty(text, index)
		case text[index] == '[':
			index++
			segment, next, ok = readConnectedTargetBracketProperty(text, index)
		case strings.HasPrefix(text[index:], "?."):
			index += 2
			index = skipWhitespaceForward(text, index)
			segment, next, ok = readConnectedTargetIdentifier(text, index)
		case text[index] == '.':
			index++
			index = skipWhitespaceForward(text, index)
			segment, next, ok = readConnectedTargetIdentifier(text, index)
		default:
			return nil, false
		}
		if !ok {
			return nil, false
		}
		segments = append(segments, segment)
		index = next
	}

	return segments, len(segments) > 0
}

func readConnectedTargetIdentifier(text string, start int) (string, int, bool) {
	if start >= len(text) || !isIdentifierStart(text[start]) {
		return "", start, false
	}
	end := start + 1
	for end < len(text) && isIdentifierPart(text[end]) {
		end++
	}
	return text[start:end], end, true
}

func readConnectedTargetBracketProperty(text string, start int) (string, int, bool) {
	start = skipWhitespaceForward(text, start)
	if start >= len(text) || !isQuote(text[start]) {
		return "", start, false
	}

	var (
		end int
		err error
	)
	if text[start] == '`' {
		end, err = skipTemplate(text, start)
	} else {
		end, err = skipQuoted(text, start, text[start])
	}
	if err != nil {
		return "", start, false
	}

	quoted := text[start : end+1]
	if text[start] == '`' && strings.Contains(quoted, "${") {
		return "", start, false
	}

	index := skipWhitespaceForward(text, end+1)
	if index >= len(text) || text[index] != ']' {
		return "", start, false
	}

	name := strings.TrimSpace(unquoteString(quoted))
	if name == "" {
		return "", start, false
	}
	return name, index + 1, true
}

func skipWhitespaceForward(text string, start int) int {
	for start < len(text) && unicode.IsSpace(rune(text[start])) {
		start++
	}
	return start
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
		return normalizeInferredTypeText(text), true
	}

	inner := strings.TrimSpace(text[1:])
	if strings.HasSuffix(inner, "]") {
		inner = strings.TrimSpace(inner[:len(inner)-1])
	}
	parts, err := splitTopLevelList(inner)
	if err != nil || len(parts) == 0 {
		return "", false
	}

	stateType := normalizeInferredTypeText(strings.TrimSpace(parts[0]))
	if len(parts) > 1 {
		if widened := widenReducerStateTypeFromHandlers(stateType, strings.TrimSpace(parts[1])); widened != "" {
			stateType = widened
		}
	}
	return stateType, true
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

	if returnType, ok := parseFunctionReturnType(text); ok {
		return returnType, true
	}
	return parseSelectorFunctionArrayReturnType(text)
}

type selectorFunctionReturnCandidate struct {
	Type           string
	ParameterCount int
}

func parseFunctionReturnType(typeText string) (string, bool) {
	text := strings.TrimSpace(unwrapWrappedExpression(typeText))
	if text == "" {
		return "", false
	}
	_, returnType, ok := splitFunctionType(text)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(returnType), true
}

func parseSelectorFunctionArrayReturnType(typeText string) (string, bool) {
	text := strings.TrimSpace(typeText)
	if !strings.HasSuffix(text, "[]") {
		return "", false
	}

	elementType := strings.TrimSpace(text[:len(text)-2])
	if elementType == "" {
		return "", false
	}
	elementType = strings.TrimSpace(unwrapWrappedExpression(elementType))
	if elementType == "" {
		return "", false
	}

	parts, err := splitTopLevelUnion(elementType)
	if err != nil || len(parts) == 0 {
		parts = []string{elementType}
	}

	candidates := make([]selectorFunctionReturnCandidate, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(unwrapWrappedExpression(part))
		if part == "" {
			continue
		}
		parameters, returnType, ok := splitFunctionType(part)
		if !ok {
			continue
		}
		candidates = append(candidates, selectorFunctionReturnCandidate{
			Type:           normalizeInferredTypeText(strings.TrimSpace(returnType)),
			ParameterCount: functionParameterCount(parameters),
		})
	}
	if len(candidates) == 0 {
		return "", false
	}
	return pickSelectorFunctionReturnType(candidates), true
}

func pickSelectorFunctionReturnType(candidates []selectorFunctionReturnCandidate) string {
	best := candidates[0]
	bestLoose := isLooselyTypedType(best.Type)
	bestArrayDepth := typeArrayDepth(best.Type)

	for _, candidate := range candidates[1:] {
		candidateLoose := isLooselyTypedType(candidate.Type)
		candidateArrayDepth := typeArrayDepth(candidate.Type)

		switch {
		case bestLoose != candidateLoose:
			if !candidateLoose {
				best = candidate
				bestLoose = candidateLoose
				bestArrayDepth = candidateArrayDepth
			}
		case bestArrayDepth != candidateArrayDepth:
			if candidateArrayDepth < bestArrayDepth {
				best = candidate
				bestLoose = candidateLoose
				bestArrayDepth = candidateArrayDepth
			}
		case best.ParameterCount != candidate.ParameterCount:
			if candidate.ParameterCount > best.ParameterCount {
				best = candidate
				bestLoose = candidateLoose
				bestArrayDepth = candidateArrayDepth
			}
		default:
			best = candidate
			bestLoose = candidateLoose
			bestArrayDepth = candidateArrayDepth
		}
	}

	return best.Type
}

func functionParameterCount(parameters string) int {
	text := strings.TrimSpace(parameters)
	if text == "()" {
		return 0
	}
	parsed, ok := parseFunctionParameters(parameters)
	if !ok {
		return 0
	}
	return len(parsed)
}

func typeArrayDepth(typeText string) int {
	text := strings.TrimSpace(typeText)
	depth := 0
	for {
		text = strings.TrimSpace(unwrapWrappedExpression(text))
		switch {
		case strings.HasSuffix(text, "[]"):
			depth++
			text = strings.TrimSpace(text[:len(text)-2])
		case strings.HasPrefix(text, "Array<") && strings.HasSuffix(text, ">"):
			depth++
			text = strings.TrimSpace(text[len("Array<") : len(text)-1])
		case strings.HasPrefix(text, "ReadonlyArray<") && strings.HasSuffix(text, ">"):
			depth++
			text = strings.TrimSpace(text[len("ReadonlyArray<") : len(text)-1])
		default:
			return depth
		}
	}
}

func selectorReturnTypeNeedsRecovery(member MemberReport) bool {
	if strings.TrimSpace(member.TypeString) == "" && strings.TrimSpace(member.ReturnTypeString) == "" {
		return false
	}
	if selectorTypeNeedsSourceRecovery(strings.TrimSpace(member.ReturnTypeString)) {
		return true
	}
	if returnType := strings.TrimSpace(member.ReturnTypeString); returnType != "" && !strings.Contains(returnType, "...") && !isAnyLikeType(returnType) {
		return false
	}
	if returnType, ok := parseSelectorReturnType(member.TypeString); ok && !isAnyLikeType(returnType) {
		return false
	}
	return true
}

func selectorTypeNeedsSourceRecovery(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	switch {
	case text == "":
		return true
	case isLooselyTypedType(text):
		return true
	case text == "{}" || text == "{ }":
		return true
	case strings.HasPrefix(text, "typeof "):
		return true
	default:
		return false
	}
}

func selectorReturnTypeConflicts(reported, parsed string) bool {
	reported = normalizeInferredTypeText(strings.TrimSpace(reported))
	parsed = normalizeInferredTypeText(strings.TrimSpace(parsed))
	if reported == "" || parsed == "" {
		return false
	}
	return reported != parsed
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
	name, value, ok := splitTopLevelPropertyRaw(entry)
	if !ok {
		return "", "", false
	}
	name = strings.TrimSuffix(name, "?")
	name = strings.Trim(name, `"'`)
	if name == "" || value == "" {
		return "", "", false
	}
	return name, value, true
}

func splitTopLevelPropertyRaw(entry string) (string, string, bool) {
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

func splitFunctionParameterParts(parameters string) ([]string, bool) {
	text := strings.TrimSpace(parameters)
	if len(text) < 2 || text[0] != '(' || text[len(text)-1] != ')' {
		return nil, false
	}

	parts, err := splitTopLevelList(text[1 : len(text)-1])
	if err != nil {
		return nil, false
	}
	return parts, true
}

func normalizeFunctionParametersText(parameters string) string {
	parts, ok := splitFunctionParameterParts(parameters)
	if !ok {
		return normalizeSourceTypeText(parameters)
	}

	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := normalizeParameterDeclarationText(part); item != "" {
			normalized = append(normalized, item)
		}
	}
	return "(" + strings.Join(normalized, ", ") + ")"
}

func normalizeParameterDeclarationText(part string) string {
	text := strings.TrimSpace(part)
	if text == "" {
		return ""
	}

	baseText, hadDefault := stripTopLevelParameterDefault(text)
	text = normalizeSourceTypeText(baseText)
	if hadDefault {
		text = addOptionalMarkerToParameterText(text)
	}
	return text
}

func stripTopLevelParameterDefault(text string) (string, bool) {
	index := findTopLevelParameterDefault(text)
	if index == -1 {
		return text, false
	}
	return strings.TrimSpace(text[:index]), true
}

func findTopLevelParameterDefault(text string) int {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0

	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\'':
			end, err := skipQuoted(text, i, '\'')
			if err != nil {
				return -1
			}
			i = end
		case '"':
			end, err := skipQuoted(text, i, '"')
			if err != nil {
				return -1
			}
			i = end
		case '`':
			end, err := skipTemplate(text, i)
			if err != nil {
				return -1
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
			if shouldOpenAngle(text, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '=':
			if i+1 < len(text) && text[i+1] == '>' {
				continue
			}
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				return i
			}
		}
	}

	return -1
}

func addOptionalMarkerToParameterText(text string) string {
	index := findTopLevelDelimiter(text, ':')
	if index == -1 {
		return text
	}

	name := strings.TrimSpace(text[:index])
	if name == "" || strings.HasSuffix(name, "?") {
		return text
	}
	last := name[len(name)-1]
	switch last {
	case ')', '}', ']':
		return text
	}
	return name + "?: " + strings.TrimSpace(text[index+1:])
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

func isLooselyTypedType(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	switch text {
	case "", "any", "unknown", "any[]", "readonly any[]", "Array<any>", "ReadonlyArray<any>":
		return true
	default:
		return false
	}
}

func isBooleanLiteralType(typeText string) bool {
	text := strings.TrimSpace(typeText)
	return text == "true" || text == "false"
}

func preferredReducerStateType(current, parsed string) string {
	current = normalizeInferredTypeText(current)
	parsed = normalizeInferredTypeText(parsed)

	switch {
	case parsed == "":
		return current
	case current == "":
		return parsed
	case isAnyLikeType(current):
		return parsed
	case current == parsed:
		return current
	case isBooleanLiteralType(current) && parsed == "boolean":
		return parsed
	default:
		return current
	}
}

func widenReducerStateTypeFromHandlers(stateType, handlersText string) string {
	if !isBooleanLiteralType(stateType) {
		return ""
	}

	properties, ok := parseObjectTypeMembers(handlersText)
	if !ok {
		return ""
	}

	hasTrue := stateType == "true"
	hasFalse := stateType == "false"
	for _, propertyType := range properties {
		_, returnType, ok := splitFunctionType(propertyType)
		if !ok {
			continue
		}

		switch strings.TrimSpace(returnType) {
		case "boolean":
			return "boolean"
		case "true":
			hasTrue = true
		case "false":
			hasFalse = true
		}
	}

	if hasTrue && hasFalse {
		return "boolean"
	}
	return ""
}

func normalizeInferredTypeText(typeText string) string {
	text := normalizeSourceTypeText(typeText)
	switch text {
	case "never[]":
		return "any[]"
	case "readonly never[]":
		return "readonly any[]"
	case "Array<never>":
		return "Array<any>"
	case "ReadonlyArray<never>":
		return "ReadonlyArray<any>"
	default:
		return text
	}
}

func normalizeActionPayloadType(typeText string) string {
	return normalizeInferredTypeText(typeText)
}

func stripNullableActionPayloadProperties(typeText string) string {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return text
	}

	body := strings.TrimSpace(text[1 : len(text)-1])
	if body == "" {
		return text
	}

	entries, err := splitTopLevelTypeMembers(body)
	if err != nil {
		return text
	}

	parts := make([]string, 0, len(entries))
	changed := false
	for _, entry := range entries {
		rawName, value, ok := splitTopLevelPropertyRaw(entry)
		if !ok {
			return text
		}
		normalized := normalizeInternalHelperParameterType(value)
		if normalized != strings.TrimSpace(value) {
			changed = true
		}
		parts = append(parts, fmt.Sprintf("%s: %s", rawName, normalized))
	}
	if !changed {
		return text
	}
	return "{ " + strings.Join(parts, "; ") + "; }"
}

func normalizeInternalHelperParameterType(typeText string) string {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return ""
	}
	unwrapped := unwrapWrappedExpression(text)
	if unwrapped != "" {
		text = unwrapped
	}
	parts, err := splitTopLevelUnion(text)
	if err != nil || len(parts) == 0 {
		return text
	}
	filtered := make([]string, 0, len(parts))
	removedNullable := false
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case "", "null", "undefined":
			if part != "" {
				removedNullable = true
			}
			continue
		}
		filtered = append(filtered, part)
	}
	if !removedNullable || len(filtered) == 0 {
		return text
	}
	return normalizeInferredTypeText(strings.Join(filtered, " | "))
}

func arrayTypeText(elementType string) string {
	elementType = strings.TrimSpace(elementType)
	if elementType == "" {
		return "any[]"
	}
	if strings.Contains(elementType, "|") {
		return "(" + elementType + ")[]"
	}
	return elementType + "[]"
}

func sourceArrowFunctionTypeTextFromRange(source, file string, property SourceProperty, state *buildState) string {
	expression := sourcePropertyText(source, property)
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return ""
	}

	returnType := info.ExplicitReturn
	if returnType == "" {
		returnType = sourceReturnExpressionType(source, info.Body, info.BlockBody, info.ParameterNames, nil, info.Async)
	}
	if state != nil && file != "" && shouldProbeSourceArrowReturnType(returnType, info.ExplicitReturn) {
		if probed := sourceArrowReturnTypeFromTypeProbe(source, file, property, state); probed != "" {
			returnType = probed
		}
	}
	if returnType == "" {
		return ""
	}
	return info.Parameters + " => " + returnType
}

func shouldProbeSourceArrowReturnType(returnType, explicitReturn string) bool {
	if strings.TrimSpace(explicitReturn) != "" {
		return false
	}
	text := normalizeSourceTypeText(returnType)
	return text == "" || isAnyLikeType(text) || typeTextContainsStandaloneToken(text, "any") || typeTextContainsStandaloneToken(text, "unknown")
}

func sourceArrowReturnTypeFromTypeProbe(source, file string, property SourceProperty, state *buildState) string {
	return sourceArrowReturnTypeFromTypeProbeRange(source, file, property, property.ValueStart, property.ValueEnd, state)
}

func sourceCallbackReturnTypeFromTypeProbe(source, file string, property SourceProperty, state *buildState) string {
	if state == nil || file == "" {
		return ""
	}
	if err := state.ensureAPIClient(); err != nil {
		return ""
	}
	projectID, err := state.projectIDForFile(file)
	if err != nil {
		return ""
	}

	end := trimExpressionEnd(source, property.ValueEnd)
	if end <= property.ValueStart {
		return ""
	}

	var fallback string
	for _, position := range callbackTypeProbePositions(source, property.ValueStart, end) {
		returnType := normalizeSourceTypeText(callbackReturnTypeAtPositionString(
			context.Background(),
			state.apiClient,
			state.timeout,
			state.apiSnapshot,
			projectID,
			file,
			position,
		))
		if returnType == "" {
			continue
		}
		if !isUsableCallbackReturnType(returnType) {
			continue
		}
		if !isAnyLikeType(returnType) && !typeTextContainsStandaloneToken(returnType, "any") && !typeTextContainsStandaloneToken(returnType, "unknown") {
			return returnType
		}
		if fallback == "" {
			fallback = returnType
		}
	}
	return fallback
}

func sourceSelectorReturnTypeFromTypeProbe(source, file string, property SourceProperty, state *buildState) string {
	start := property.ValueStart
	end := property.ValueEnd
	probeProperty := property
	if lastStart, lastEnd, ok, err := FindLastTopLevelArrayElement(source, property.ValueStart, property.ValueEnd); err == nil && ok {
		start = lastStart
		end = lastEnd
		probeProperty.NameStart = lastStart
		probeProperty.ValueStart = lastStart
		probeProperty.ValueEnd = lastEnd
	}
	return sourceArrowReturnTypeFromTypeProbeRange(source, file, probeProperty, start, end, state)
}

func sourceArrowReturnTypeFromTypeProbeRange(source, file string, property SourceProperty, valueStart, valueEnd int, state *buildState) string {
	if state == nil || file == "" {
		return ""
	}
	if err := state.ensureAPIClient(); err != nil {
		return ""
	}
	projectID, err := state.projectIDForFile(file)
	if err != nil {
		return ""
	}

	var fallback string
	if typeText := normalizeSourceTypeText(sourceArrowReturnTypeFromSignatureProbe(file, property, projectID, state)); typeText != "" {
		if !isAnyLikeType(typeText) && !typeTextContainsStandaloneToken(typeText, "any") && !typeTextContainsStandaloneToken(typeText, "unknown") {
			return typeText
		}
		fallback = typeText
	}

	probePosition, ok, err := FindArrowFunctionReturnProbe(source, valueStart, valueEnd)
	if err != nil || !ok {
		return ""
	}

	probeEnd, err := findPropertyEnd(source, probePosition, valueEnd)
	if err != nil {
		probeEnd = valueEnd
	}
	probeEnd = trimExpressionEnd(source, probeEnd)

	for _, position := range selectorTypeProbePositions(source, probePosition, probeEnd) {
		typeText := normalizeSourceTypeText(typeAtPositionString(
			context.Background(),
			state.apiClient,
			state.timeout,
			state.apiSnapshot,
			projectID,
			file,
			position,
		))
		if typeText == "" {
			continue
		}
		if !isAnyLikeType(typeText) && !typeTextContainsStandaloneToken(typeText, "any") && !typeTextContainsStandaloneToken(typeText, "unknown") && isCompatibleProbeReturnType(typeText, source, probePosition, probeEnd) {
			return typeText
		}
		if fallback == "" {
			fallback = typeText
		}
	}
	return fallback
}

func sourceArrowReturnTypeFromSignatureProbe(file string, property SourceProperty, projectID string, state *buildState) string {
	if state == nil || state.apiClient == nil {
		return ""
	}

	for _, position := range []int{property.NameStart, property.ValueStart} {
		if position <= 0 {
			continue
		}

		functionType, err := state.apiClient.GetTypeAtPosition(
			tsgoapi.WithTimeout(context.Background(), state.timeout),
			state.apiSnapshot,
			projectID,
			file,
			position,
		)
		if err != nil || functionType == nil {
			continue
		}

		signatures, err := state.apiClient.GetSignaturesOfType(
			tsgoapi.WithTimeout(context.Background(), state.timeout),
			state.apiSnapshot,
			projectID,
			functionType.ID,
		)
		if err != nil || len(signatures) == 0 {
			continue
		}

		returnType, err := state.apiClient.GetReturnTypeOfSignature(
			tsgoapi.WithTimeout(context.Background(), state.timeout),
			state.apiSnapshot,
			projectID,
			signatures[0].ID,
		)
		if err != nil || returnType == nil {
			continue
		}

		typeText := normalizeSourceTypeText(safeTypeString(
			context.Background(),
			state.apiClient,
			state.timeout,
			state.apiSnapshot,
			projectID,
			returnType.ID,
		))
		if typeText == "" || isAnyLikeType(typeText) || typeTextContainsStandaloneToken(typeText, "any") || typeTextContainsStandaloneToken(typeText, "unknown") {
			continue
		}
		return typeText
	}
	return ""
}

func callbackReturnTypeAtPositionString(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	file string,
	position int,
) string {
	typ, err := client.GetTypeAtPosition(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, file, position)
	if err != nil || typ == nil {
		return ""
	}

	signatures, err := client.GetSignaturesOfType(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, typ.ID)
	if err == nil && len(signatures) > 0 {
		returnType, err := client.GetReturnTypeOfSignature(
			tsgoapi.WithTimeout(ctx, timeout),
			snapshot,
			projectID,
			signatures[0].ID,
		)
		if err == nil && returnType != nil {
			if text := normalizeSourceTypeText(safeTypeString(ctx, client, timeout, snapshot, projectID, returnType.ID)); text != "" {
				return text
			}
		}
	}

	typeText := normalizeSourceTypeText(safeTypeString(ctx, client, timeout, snapshot, projectID, typ.ID))
	if typeText == "" {
		return ""
	}
	if isFunctionLikeTypeText(typeText) {
		if returnType, ok := parseFunctionReturnType(typeText); ok {
			return normalizeSourceTypeText(returnType)
		}
	}
	return ""
}

func isFunctionLikeTypeText(typeText string) bool {
	text := strings.TrimSpace(unwrapWrappedExpression(typeText))
	if text == "" {
		return false
	}
	if text[0] == '(' || text[0] == '<' {
		return true
	}
	if strings.HasPrefix(text, "new (") || strings.HasPrefix(text, "new<") {
		return true
	}
	return false
}

func isUsableCallbackReturnType(typeText string) bool {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return false
	}
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		return true
	}
	return !unbracedMemberTypePattern.MatchString(text)
}

func isCompatibleProbeReturnType(typeText, source string, probeStart, probeEnd int) bool {
	typeText = normalizeSourceTypeText(typeText)
	if typeText == "" {
		return false
	}
	expression := strings.TrimSpace(source[probeStart:probeEnd])
	if expression == "" {
		return true
	}
	if objectStart, _, ok, err := FindInspectableObjectLiteral(expression, 0, len(expression)); err == nil && ok && objectStart == 0 {
		return strings.HasPrefix(typeText, "{")
	}
	return true
}

func typeTextContainsStandaloneToken(typeText, token string) bool {
	text := strings.TrimSpace(typeText)
	if text == "" || token == "" {
		return false
	}

	for index := 0; ; {
		next := strings.Index(text[index:], token)
		if next == -1 {
			return false
		}
		next += index
		beforeOK := next == 0 || !isIdentifierPart(text[next-1])
		afterIndex := next + len(token)
		afterOK := afterIndex >= len(text) || !isIdentifierPart(text[afterIndex])
		if beforeOK && afterOK {
			return true
		}
		index = next + len(token)
		if index >= len(text) {
			return false
		}
	}
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
	for _, selector := range logic.InternalSelectorTypes {
		texts = append(texts, selector.FunctionType)
	}
	for _, action := range logic.InternalReducerActions {
		texts = append(texts, action.FunctionType, action.PayloadType)
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

	if rootDir, ok := resolvePackageRootFromDir(filepath.Dir(file), rootName); ok {
		return rootDir, rootName, subpath, true
	}
	if cwd, err := os.Getwd(); err == nil {
		if rootDir, ok := resolvePackageRootFromDir(cwd, rootName); ok {
			return rootDir, rootName, subpath, true
		}
	}
	return "", "", "", false
}

func resolvePackageRootFromDir(dir, rootName string) (string, bool) {
	for {
		candidate := filepath.Join(dir, "node_modules", filepath.FromSlash(rootName))
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return filepath.Clean(candidate), true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
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
	relative = strings.TrimSuffix(relative, ".js")
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

func resolveImportFile(file, importPath string, state *buildState) (string, bool) {
	if resolvedFile, ok := resolveLocalImportFile(file, importPath); ok {
		return resolvedFile, true
	}
	return resolveConfiguredImportFile(importPath, state)
}

func resolveConfiguredImportFile(importPath string, state *buildState) (string, bool) {
	if state == nil || state.configFile == "" || importPath == "" {
		return "", false
	}
	if state.config == nil {
		if err := state.ensureAPIClient(); err != nil {
			return "", false
		}
	}

	baseURL := "."
	if rawBaseURL, ok := state.config.Options["baseUrl"].(string); ok && strings.TrimSpace(rawBaseURL) != "" {
		baseURL = strings.TrimSpace(rawBaseURL)
	}
	configDir := filepath.Dir(state.configFile)
	rootPath := filepath.FromSlash(baseURL)
	root := filepath.Clean(filepath.Join(configDir, rootPath))
	if filepath.IsAbs(rootPath) {
		root = filepath.Clean(rootPath)
	}

	for _, candidate := range configuredImportCandidates(importPath, root, state.config.Options["paths"]) {
		if resolvedFile, ok := resolveAbsoluteImportBase(candidate); ok {
			return resolvedFile, true
		}
	}

	if strings.HasPrefix(importPath, "/") {
		return resolveAbsoluteImportBase(importPath)
	}
	return resolveAbsoluteImportBase(filepath.Join(root, filepath.FromSlash(importPath)))
}

func configuredImportCandidates(importPath, root string, rawPaths any) []string {
	pathMap := map[string]any{}
	switch value := rawPaths.(type) {
	case map[string]any:
		pathMap = value
	case map[string][]string:
		for key, targets := range value {
			pathMap[key] = targets
		}
	default:
		return nil
	}
	if len(pathMap) == 0 {
		return nil
	}

	keys := make([]string, 0, len(pathMap))
	for key := range pathMap {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})

	var candidates []string
	seen := map[string]bool{}
	for _, pattern := range keys {
		matches, replacement := matchPathPattern(pattern, importPath)
		if !matches {
			continue
		}
		for _, target := range pathPatternTargets(pathMap[pattern]) {
			if strings.TrimSpace(target) == "" {
				continue
			}
			target = strings.ReplaceAll(target, "*", replacement)
			candidate := filepath.Clean(filepath.Join(root, filepath.FromSlash(target)))
			if seen[candidate] {
				continue
			}
			seen[candidate] = true
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func pathPatternTargets(raw any) []string {
	switch value := raw.(type) {
	case []any:
		targets := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				targets = append(targets, text)
			}
		}
		return targets
	case []string:
		return append([]string(nil), value...)
	default:
		return nil
	}
}

func matchPathPattern(pattern, importPath string) (bool, string) {
	if pattern == importPath {
		return true, ""
	}
	star := strings.Index(pattern, "*")
	if star == -1 {
		return false, ""
	}
	prefix := pattern[:star]
	suffix := pattern[star+1:]
	if !strings.HasPrefix(importPath, prefix) || !strings.HasSuffix(importPath, suffix) {
		return false, ""
	}
	if len(importPath) < len(prefix)+len(suffix) {
		return false, ""
	}
	return true, importPath[len(prefix) : len(importPath)-len(suffix)]
}

func resolveAbsoluteImportBase(basePath string) (string, bool) {
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

func previousNonWhitespaceIndex(text string, index int) int {
	for index > 0 {
		index--
		if text[index] == ' ' || text[index] == '\t' || text[index] == '\n' || text[index] == '\r' {
			continue
		}
		return index
	}
	return -1
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
