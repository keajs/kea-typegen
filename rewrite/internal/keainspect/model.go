package keainspect

import (
	"context"
	"encoding/base64"
	"encoding/binary"
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
	optionalUndefinedUnionPattern  = regexp.MustCompile(`(\?\s*:\s*[^;,\n{}]+?)\s*\|\s*undefined\b`)
	simpleIndexedAccessTypePattern = regexp.MustCompile(`([A-Za-z_$][A-Za-z0-9_$.]*)\[(?:'|")([A-Za-z_$][A-Za-z0-9_$]*)(?:'|")\]`)
	unbracedMemberTypePattern      = regexp.MustCompile(`\b[$A-Za-z_][\w$]*\??\s*:`)
	loaderSpreadReturnPattern      = regexp.MustCompile(`\breturn\s+\[[^\]]*\.\.\.\s*values\.`)
	loaderAwaitedJSONLocalPattern  = regexp.MustCompile(`\b(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*await\s+[^\n;]*?\.json\s*\(`)
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
	binaryPath         string
	projectDir         string
	configFile         string
	timeout            time.Duration
	parsedByFile       map[string][]ParsedLogic
	parsedFileByFile   map[string]parsedFileCache
	building           map[string]bool
	apiClient          *tsgoapi.Client
	apiSnapshot        string
	config             *tsgoapi.ConfigResponse
	primaryProjectID   string
	projectByFile      map[string]string
	typeTextByTypeID   map[string]string
	typeTextByPos      map[string]string
	typeTextByLocation map[string]string
	callbackByPos      map[string]string
	signatureByPos     map[string]string
	sourceOffsetByFile map[string]sourceOffsetMap
	sourceNodesByFile  map[string]sourceFileNodeCache
}

type parsedFileCache struct {
	File         string
	Source       string
	SourceLogics []SourceLogic
	Logics       []ParsedLogic
}

type sourceFileNode struct {
	Kind   uint32
	Pos    uint32
	End    uint32
	Parent uint32
}

type sourceFileNodeCache struct {
	Nodes         []sourceFileNode
	CanonicalFile string
	OK            bool
}

const (
	sourceNodeRecordWidth = 7
	sourceNodeRecordBytes = sourceNodeRecordWidth * 4
	sourceFileSyntaxKind  = 307
	sourceInvalidNodeKind = ^uint32(0)
)

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
	sections := map[string][]SectionReport{}
	for _, section := range logicReport.Sections {
		sections[section.Name] = append(sections[section.Name], section)
	}

	properties := map[string][]SourceProperty{}
	for _, property := range sourceLogic.Properties {
		properties[property.Name] = append(properties[property.Name], property)
	}

	path := defaultLogicPath(report.ProjectDir, report.File)
	if property, ok := lastSourceProperty(properties["path"]); ok {
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

	reportedPropsType := ""
	missingLocalLogicTypeImport := hasMissingLocalLogicTypeImport(source, report.File)
	if section, ok := lastSectionReport(sections["props"]); ok {
		reportedPropsType = preferredTypeText(section)
		parsed.PropsType = reportedPropsType
		if property, hasProperty := lastSourceProperty(properties["props"]); hasProperty && propsTypeNeedsSourceRecovery(parsed.PropsType) {
			if recovered := normalizeSourceTypeText(sourceExpressionTypeText(source, sourcePropertyText(source, property))); recovered != "" && !isAnyLikeType(recovered) {
				parsed.PropsType = recovered
			}
		}
	}
	if section, ok := lastSectionReport(sections["key"]); ok {
		parsed.KeyType = preferredTypeText(section)
		if property, hasProperty := lastSourceProperty(properties["key"]); hasProperty && keyTypeNeedsSourceRecovery(parsed.KeyType) {
			expression := normalizeSingleCallbackExpression(sourcePropertyText(source, property))
			keepReportedAny := shouldKeepReportedAnyForBuilderKey(logicReport.InputKind, expression)
			allowRecoveredUntypedKeyFromProps := !isAnyLikeType(reportedPropsType) || missingLocalLogicTypeImport
			if probed := normalizeSourceTypeText(sourceCallbackReturnTypeFromTypeProbe(source, report.File, property, state)); probed != "" &&
				!isAnyLikeType(probed) &&
				!typeTextContainsStandaloneToken(probed, "any") &&
				!typeTextContainsStandaloneToken(probed, "unknown") {
				parsed.KeyType = probed
			}
			if probed := normalizeSourceTypeText(sourceArrowReturnTypeFromLocationProbe(source, report.File, property, state)); shouldPreferRecoveredKeyType(parsed.KeyType, probed) {
				parsed.KeyType = probed
			}
			if inferred := sourceArrowReturnTypeText(source, expression); inferred != "" {
				if shouldPreferRecoveredKeyType(parsed.KeyType, inferred) {
					parsed.KeyType = normalizeSourceTypeText(inferred)
				}
			}
			if recovered := sourceKeyTypeFromSource(source, report.File, property, parsed.PropsType, state); recovered != "" {
				if shouldPreferRecoveredKeyType(parsed.KeyType, recovered) &&
					(!keepReportedAny || shouldUseRecoveredUntypedBuilderKeyType(source, report.File, expression, parsed.PropsType, recovered, allowRecoveredUntypedKeyFromProps, state)) {
					parsed.KeyType = normalizeSourceTypeText(recovered)
				}
			} else if keepReportedAny {
				if recovered := recoverUntypedBuilderKeyType(source, report.File, property, parsed.PropsType, state); recovered != "" {
					if shouldPreferRecoveredKeyType(parsed.KeyType, recovered) {
						parsed.KeyType = normalizeSourceTypeText(recovered)
					}
				}
			}
		}
	}
	for index, section := range sections["actions"] {
		parsed.Actions = mergeParsedActions(parsed.Actions, parseActionsWithSource(section, source, sourcePropertyAt(properties["actions"], index), logicReport.InputKind, report.File, state)...)
	}
	for index, section := range sections["defaults"] {
		parsed.Reducers = mergeParsedFields(
			parsed.Reducers,
			parseDefaultFieldsWithSource(section, source, sourcePropertyAt(properties["defaults"], index), report.File, state)...,
		)
	}
	for index, section := range sections["reducers"] {
		parsed.Reducers = mergeParsedFields(parsed.Reducers, parseReducersWithSource(section, source, sourcePropertyAt(properties["reducers"], index), report.File, state)...)
	}
	for _, section := range sections["windowValues"] {
		parsed.Reducers = mergeParsedFields(parsed.Reducers, parseWindowValues(section)...)
	}
	for _, section := range sections["form"] {
		formActions, formReducers, extraInputForm := parseFormPluginSection(section)
		parsed.Actions = mergeParsedActions(parsed.Actions, formActions...)
		parsed.Reducers = mergeParsedFields(parsed.Reducers, formReducers...)
		if parsed.ExtraInputForm == "" {
			parsed.ExtraInputForm = extraInputForm
		}
	}
	for index, section := range sections["forms"] {
		property := sourcePropertyAt(properties["forms"], index)
		formActions, formReducers, formSelectors, formImports := parseFormsPluginSectionWithSource(
			section,
			source,
			property,
			report.File,
			state,
		)
		parsed.Actions = mergeParsedActionsPreferExisting(parsed.Actions, formActions...)
		parsed.Reducers = mergeParsedFieldsPreferExisting(parsed.Reducers, formReducers...)
		parsed.Selectors = mergeParsedFieldsPreferExisting(parsed.Selectors, formSelectors...)
		parsed.Imports = mergeTypeImports(parsed.Imports, formImports)
	}
	for index, section := range sections["loaders"] {
		loaderActions, loaderReducers := parseLoadersWithSource(section, source, sourcePropertyAt(properties["loaders"], index), report.File, state)
		parsed.Actions = mergeParsedActionsPreferLoaderRecovery(parsed.Actions, loaderActions...)
		parsed.Reducers = mergeParsedFields(parsed.Reducers, loaderReducers...)
	}
	for index, section := range sections["lazyLoaders"] {
		loaderActions, loaderReducers := parseLazyLoadersWithSource(section, source, sourcePropertyAt(properties["lazyLoaders"], index), report.File, state)
		parsed.Actions = mergeParsedActionsPreferLoaderRecovery(parsed.Actions, loaderActions...)
		parsed.Reducers = mergeParsedFields(parsed.Reducers, loaderReducers...)
	}
	for _, property := range properties["connect"] {
		parsed.Imports = mergeTypeImports(
			parsed.Imports,
			enrichConnectedSections(source, report.File, property, sections["listeners"], &parsed, state),
		)
	}
	for index, section := range sections["selectors"] {
		property := sourcePropertyAt(properties["selectors"], index)
		if parityModeSuppressesBuilderCallbackSelectors(logicReport.InputKind, source, property, properties["forms"]) {
			continue
		}
		parsed.Selectors = mergeParsedFields(parsed.Selectors, parseSelectorsWithSource(section, parsed, source, property, report.File, state)...)
		helpers := parseInternalSelectorTypesWithSource(section, parsed, source, property, report.File, state)
		parsed.InternalSelectorTypes = mergeParsedFunctions(parsed.InternalSelectorTypes, helpers...)
		parsed.Selectors = refineSelectorTypesFromInternalHelpers(parsed.Selectors, helpers, missingLocalLogicTypeImport)
		if len(helpers) > 0 {
			// A second selector pass lets later sibling selectors see earlier selector
			// surfaces that only became concrete after helper-driven refinement.
			reparsedSelectors := parseSelectorsWithSource(section, parsed, source, property, report.File, state)
			parsed.Selectors = mergeParsedFields(parsed.Selectors, reparsedSelectors...)
			// A second pass lets earlier helpers see later sibling selectors that only
			// became concrete after their own helper recovery in the first pass.
			refinedHelpers := parseInternalSelectorTypesWithSource(section, parsed, source, property, report.File, state)
			parsed.InternalSelectorTypes = mergeParsedFunctions(parsed.InternalSelectorTypes, refinedHelpers...)
			parsed.Selectors = refineSelectorTypesFromInternalHelpers(parsed.Selectors, parsed.InternalSelectorTypes, missingLocalLogicTypeImport)
		}
	}
	for index, section := range sections["sharedListeners"] {
		parsed.SharedListeners = mergeParsedSharedListeners(
			parsed.SharedListeners,
			parseSharedListenersWithSource(
				section,
				source,
				sourcePropertyAt(properties["sharedListeners"], index),
				properties["listeners"],
				report.File,
				state,
			)...,
		)
	}
	for index, section := range sections["events"] {
		parsed.Events = mergeEventNames(parsed.Events, parseEventNamesWithSource(section, source, sourcePropertyAt(properties["events"], index))...)
	}
	if len(sections["reducers"]) > 0 || len(sections["listeners"]) > 0 {
		parsed.InternalReducerActions = parseInternalReducerActions(
			source,
			report.File,
			logicReport.InputKind,
			properties["reducers"],
			properties["listeners"],
			state,
		)
	}
	reducerPayloadHints := map[string]string{}
	for _, section := range sections["reducers"] {
		for name, payloadType := range collectReducerActionPayloadHints(section) {
			if shouldRefineActionPayloadType(reducerPayloadHints[name], payloadType) {
				reducerPayloadHints[name] = payloadType
			}
		}
	}
	if len(reducerPayloadHints) > 0 {
		parsed.Actions = refineActionPayloadTypes(parsed.Actions, reducerPayloadHints)
	}
	parsed.Actions = refineLoaderSuccessActionPayloadTypes(parsed.Actions)
	for index, section := range sections["listeners"] {
		listeners, listenerImports := parseListenersWithSource(section, parsed, source, sourcePropertyAt(properties["listeners"], index), report.File, state)
		parsed.Listeners = mergeParsedListeners(parsed.Listeners, listeners...)
		parsed.Imports = mergeTypeImports(parsed.Imports, listenerImports)
	}
	parsed = reorderParsedLogicForSourceProperties(source, sourceLogic, sections, properties, parsed, report.File, state)

	parsed.Imports = mergeTypeImports(parsed.Imports, collectTypeImports(source, report.File, parsed, state))
	return parsed, nil
}

func lastSectionReport(sections []SectionReport) (SectionReport, bool) {
	if len(sections) == 0 {
		return SectionReport{}, false
	}
	return sections[len(sections)-1], true
}

func lastSourceProperty(properties []SourceProperty) (SourceProperty, bool) {
	if len(properties) == 0 {
		return SourceProperty{}, false
	}
	return properties[len(properties)-1], true
}

func sourcePropertyAt(properties []SourceProperty, index int) SourceProperty {
	if index < 0 || index >= len(properties) {
		return SourceProperty{}
	}
	return properties[index]
}

func shouldKeepReportedAnyForBuilderKey(inputKind, expression string) bool {
	if inputKind != "builders" {
		return false
	}
	info, ok := parseSourceArrowInfo(expression)
	if !ok || strings.TrimSpace(info.ExplicitReturn) != "" {
		return false
	}
	parts, ok := splitFunctionParameterParts(info.Parameters)
	if !ok || len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if strings.Contains(part, ":") {
			return false
		}
	}
	return true
}

func parityModeEnabled() bool {
	return os.Getenv("KEA_TYPEGEN_PARITY_MODE") == "1"
}

func sectionUsesBuilderCallback(source string, property SourceProperty) bool {
	if source == "" || property.ValueEnd <= property.ValueStart {
		return false
	}
	expression := normalizeSingleCallbackExpression(sourcePropertyText(source, property))
	if expression == "" {
		return false
	}
	if _, ok := parseSourceArrowInfo(expression); ok {
		return true
	}
	return strings.HasPrefix(expression, "function")
}

func anySourcePropertiesUseBuilderCallback(source string, properties []SourceProperty) bool {
	for _, property := range properties {
		if sectionUsesBuilderCallback(source, property) {
			return true
		}
	}
	return false
}

func parityModeSuppressesBuilderCallbackSelectors(inputKind, source string, selectorProperty SourceProperty, formProperties []SourceProperty) bool {
	if !parityModeEnabled() || inputKind != "builders" || !sectionUsesBuilderCallback(source, selectorProperty) {
		return false
	}
	return anySourcePropertiesUseBuilderCallback(source, formProperties)
}

func hasMissingLocalLogicTypeImport(source, file string) bool {
	if strings.TrimSpace(file) == "" {
		return false
	}
	for _, candidate := range parseNamedValueImports(source) {
		importedName := strings.TrimSpace(candidate.ImportedName)
		if !strings.HasSuffix(importedName, "LogicType") {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(candidate.Path), ".") {
			continue
		}
		if _, ok := resolveLocalImportFile(file, candidate.Path); ok {
			continue
		}
		return true
	}
	return false
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
	sourceEntries := sourceEntriesByName(sectionSourceEntries(source, property))
	memberByName := map[string]MemberReport{}
	for _, member := range section.Members {
		memberByName[member.Name] = member
	}
	orderedNames := sourceOrderedSectionMemberNames(source, property, section.Members)

	actions := make([]ParsedAction, 0, len(orderedNames))
	for _, name := range orderedNames {
		var member *MemberReport
		if parsedMember, ok := memberByName[name]; ok {
			member = &parsedMember
		}
		nested := SourceProperty{}
		if property, ok := sourceMembers[name]; ok {
			nested = property
		}
		entryExpression := ""
		if entry, ok := sourceEntries[name]; ok {
			entryExpression = entry.Value
		}
		action, ok := parseSingleActionWithSource(name, member, source, nested, entryExpression, inputKind, file, state)
		if ok {
			actions = append(actions, action)
		}
	}
	return actions
}

func parseSingleActionWithSource(
	name string,
	member *MemberReport,
	source string,
	property SourceProperty,
	entryExpression string,
	inputKind string,
	file string,
	state *buildState,
) (ParsedAction, bool) {
	expression := entryExpression
	if property.ValueEnd > property.ValueStart {
		expression = sourcePropertyText(source, property)
	}
	if strings.TrimSpace(expression) == "true" {
		return ParsedAction{
			Name:         name,
			FunctionType: "() => { value: true }",
			PayloadType:  "{ value: true }",
		}, true
	}

	functionType := ""
	payloadType := ""
	sourceParametersAvailable := false
	preserveSourceUnknownParameters := map[string]bool{}
	if member != nil {
		functionType = strings.TrimSpace(preferredMemberTypeText(*member))
		if functionType == "true" {
			return ParsedAction{
				Name:         name,
				FunctionType: "() => { value: true }",
				PayloadType:  "{ value: true }",
			}, true
		}
		payloadType = strings.TrimSpace(preferredMemberReturnTypeText(*member))
		if payloadType == "" || strings.Contains(payloadType, "...") {
			if _, returnType, ok := splitFunctionType(preferredMemberTypeText(*member)); ok && strings.TrimSpace(returnType) != "" {
				payloadType = strings.TrimSpace(returnType)
			}
		}
	}

	if expression != "" {
		if parameters, explicitReturn, ok := parseSourceArrowSignature(expression); ok {
			sourceParametersAvailable = true
			preserveSourceUnknownParameters = sourceExplicitUnknownParameterNames(parameters)
			indexedAccessOverrides := sourceIndexedAccessActionPayloadOverrides(source, file, expression, parameters, state)
			parameters = normalizeFunctionParametersText(parameters)
			defaultedParameters := sourceDefaultedParameterNames(parameters)
			if explicitReturn != "" {
				payloadType = explicitReturn
			}
			if explicitReturn == "" {
				if inferred := sourceActionPayloadTypeFromSource(source, file, expression, state); inferred != "" {
					payloadType = mergePreferredActionPayloadTypePreservingCurrentMembers(
						payloadType,
						inferred,
						parityModeReportedAnyShorthandPayloadMembers(expression, parameters, payloadType),
					)
				}
				if len(indexedAccessOverrides) > 0 {
					payloadType = mergeIndexedAccessActionPayloadOverrides(payloadType, indexedAccessOverrides)
				}
				if property.ValueEnd > property.ValueStart && sourceActionHasDefaultedParameters(expression) {
					if probed := sourceArrowReturnTypeFromTypeProbe(source, file, property, state); isUsableActionPayloadProbeType(probed) {
						payloadType = mergePreferredActionPayloadTypePreservingMembers(payloadType, probed, defaultedParameters)
					}
				}
			}
			payloadType = normalizeActionPayloadType(payloadType)
			if payloadType == "" {
				if _, returnType, ok := splitFunctionType(functionType); ok {
					payloadType = returnType
				}
			}
			if payloadType == "" {
				payloadType = "any"
			}
			if member != nil {
				if memberParameters, _, ok := splitFunctionType(functionType); ok && !sourceActionParametersArePreferred(parameters) {
					parameters = memberParameters
				}
			}
			functionType = parameters + " => " + payloadType
		}
	}

	payloadType = normalizeActionPayloadType(payloadType)
	if inputKind == "object" {
		payloadType = stripNullableActionPayloadProperties(payloadType)
	}
	if !sourceParametersAvailable {
		functionType = refineActionFunctionType(functionType, payloadType)
	} else {
		functionType = refineActionFunctionTypePreservingSourceUnknowns(functionType, payloadType, preserveSourceUnknownParameters)
	}
	if functionType == "" && payloadType == "" {
		return ParsedAction{}, false
	}
	return ParsedAction{
		Name:         name,
		FunctionType: functionType,
		PayloadType:  payloadType,
	}, true
}

func sourceExplicitUnknownParameterNames(parameters string) map[string]bool {
	parsedParameters, ok := parseFunctionParameters(parameters)
	if !ok || len(parsedParameters) == 0 {
		return nil
	}

	names := map[string]bool{}
	for _, parameter := range parsedParameters {
		if !typeTextContainsUnquotedStandaloneToken(parameter.Type, "unknown") {
			continue
		}
		name, ok := sourceParameterName(parameter.Text)
		if !ok || name == "" {
			continue
		}
		names[name] = true
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

func sourceIndexedAccessActionPayloadOverrides(source, file, expression, parameters string, state *buildState) map[string]string {
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return nil
	}
	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
	}
	if body == "" {
		return nil
	}

	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(body, 0, len(body))
	if err != nil || !ok {
		return nil
	}
	segments, err := splitTopLevelSourceSegments(body, objectStart+1, objectEnd)
	if err != nil {
		return nil
	}

	parsedParameters, ok := parseFunctionParameters(parameters)
	if !ok || len(parsedParameters) == 0 {
		return nil
	}
	indexedAccessParameterTypes := map[string]string{}
	for _, parameter := range parsedParameters {
		name, ok := sourceParameterName(parameter.Text)
		if !ok || name == "" || !strings.Contains(parameter.Type, "[") {
			continue
		}
		expanded := normalizeSourceTypeText(expandSourceParameterHintTypeText(source, file, parameter.Type, state))
		if expanded == "" || !isUsableIndexedAccessActionPayloadOverrideType(expanded) {
			continue
		}
		indexedAccessParameterTypes[name] = expanded
	}
	if len(indexedAccessParameterTypes) == 0 {
		return nil
	}

	overrides := map[string]string{}
	for _, segment := range segments {
		text := strings.TrimSpace(segment.Text)
		if text == "" || strings.HasPrefix(text, "...") {
			continue
		}
		if name, value, ok := splitTopLevelProperty(text); ok {
			identifier, shorthand := sourceIdentifierExpression(strings.TrimSpace(value))
			if !shorthand {
				continue
			}
			override, ok := indexedAccessParameterTypes[identifier]
			if !ok {
				continue
			}
			overrides[name] = override
			continue
		}
		identifier, shorthand := sourceIdentifierExpression(text)
		if !shorthand {
			continue
		}
		override, ok := indexedAccessParameterTypes[identifier]
		if !ok {
			continue
		}
		overrides[identifier] = override
	}
	if len(overrides) == 0 {
		return nil
	}
	return overrides
}

func mergeIndexedAccessActionPayloadOverrides(current string, overrides map[string]string) string {
	if len(overrides) == 0 {
		return current
	}
	members, ok := parseActionPayloadObjectMembers(current)
	if !ok {
		return current
	}

	changed := false
	for name, override := range overrides {
		member, ok := members[name]
		if !ok || member.Type == override {
			continue
		}
		member.Type = override
		members[name] = member
		changed = true
	}
	if !changed {
		return current
	}
	return renderActionPayloadObjectMembers(members)
}

func parityModeReportedAnyShorthandPayloadMembers(expression, parameters, currentPayloadType string) map[string]bool {
	if !parityModeEnabled() {
		return nil
	}
	currentMembers, ok := parseActionPayloadObjectMembers(currentPayloadType)
	if !ok || len(currentMembers) == 0 {
		return nil
	}

	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return nil
	}
	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}

	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(body, 0, len(body))
	if err != nil || !ok {
		return nil
	}
	segments, err := splitTopLevelSourceSegments(body, objectStart+1, objectEnd)
	if err != nil || len(segments) == 0 {
		return nil
	}

	parsedParameters, ok := parseFunctionParameters(parameters)
	if !ok || len(parsedParameters) == 0 {
		return nil
	}
	parameterTypes := make(map[string]string, len(parsedParameters))
	for _, parameter := range parsedParameters {
		name, ok := sourceParameterName(parameter.Text)
		if !ok || name == "" || parameter.Type == "" {
			continue
		}
		parameterTypes[name] = normalizeSourceTypeText(parameter.Type)
	}
	if len(parameterTypes) == 0 {
		return nil
	}

	preserved := map[string]bool{}
	for _, segment := range segments {
		text := strings.TrimSpace(segment.Text)
		if text == "" || strings.HasPrefix(text, "...") {
			continue
		}

		propertyName := ""
		identifier := ""
		if name, value, ok := splitTopLevelProperty(text); ok {
			propertyName = name
			identifier, ok = sourceIdentifierExpression(strings.TrimSpace(value))
			if !ok {
				continue
			}
		} else if shorthand, ok := sourceIdentifierExpression(text); ok {
			propertyName = shorthand
			identifier = shorthand
		} else {
			continue
		}

		currentMember, ok := currentMembers[propertyName]
		if !ok || !isAnyLikeType(normalizeSourceTypeText(currentMember.Type)) {
			continue
		}
		parameterType := normalizeSourceTypeText(parameterTypes[identifier])
		if !actionShorthandPayloadShouldPreserveReportedAny(parameterType) {
			continue
		}
		preserved[propertyName] = true
	}
	if len(preserved) == 0 {
		return nil
	}
	return preserved
}

func actionShorthandPayloadShouldPreserveReportedAny(typeText string) bool {
	typeText = normalizeSourceTypeText(strings.TrimSpace(typeText))
	if typeText == "" || !strings.Contains(typeText, "|") {
		return false
	}
	if typeTextContainsUnquotedStandaloneToken(typeText, "null") || typeTextContainsUnquotedStandaloneToken(typeText, "undefined") {
		return true
	}
	return !isPrimitiveLikeUnionType(typeText) && !isBroadPrimitiveSelectorType(typeText)
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
		if parameterTypeNeedsSourceRecovery(typeText) {
			return false
		}
	}
	return true
}

func parameterTypeNeedsSourceRecovery(typeText string) bool {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return true
	}
	return isAnyLikeType(text) ||
		typeTextContainsUnquotedStandaloneToken(text, "any") ||
		typeTextContainsUnquotedStandaloneToken(text, "unknown") ||
		strings.Contains(text, "...") ||
		isGenericIndexSignatureType(text)
}

func typeTextContainsUnquotedStandaloneToken(typeText, token string) bool {
	text := strings.TrimSpace(typeText)
	if text == "" || token == "" {
		return false
	}

	for index := 0; index < len(text); index++ {
		switch text[index] {
		case '\'':
			end, err := skipQuoted(text, index, '\'')
			if err != nil {
				return false
			}
			index = end
			continue
		case '"':
			end, err := skipQuoted(text, index, '"')
			if err != nil {
				return false
			}
			index = end
			continue
		case '`':
			end, err := skipTemplate(text, index)
			if err != nil {
				return false
			}
			index = end
			continue
		}

		if !strings.HasPrefix(text[index:], token) {
			continue
		}
		beforeOK := index == 0 || !isIdentifierPart(text[index-1])
		afterIndex := index + len(token)
		afterOK := afterIndex >= len(text) || !isIdentifierPart(text[afterIndex])
		if beforeOK && afterOK {
			return true
		}
	}
	return false
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

func shouldPreferCandidateActionPayloadType(current, candidate string) bool {
	current = normalizeActionPayloadType(current)
	candidate = normalizeActionPayloadType(candidate)
	if candidate == "" || current == candidate {
		return false
	}
	if current == "" {
		return true
	}
	if typeTextContainsStandaloneToken(current, "any") && !typeTextContainsStandaloneToken(candidate, "any") {
		return true
	}
	if typeTextContainsStandaloneToken(current, "unknown") && !typeTextContainsStandaloneToken(candidate, "unknown") {
		return true
	}
	if genericSpecializesActionPayloadType(current, candidate) {
		return true
	}
	if !typeTextContainsStandaloneToken(current, "any") && typeTextContainsStandaloneToken(candidate, "any") {
		return false
	}
	if !typeTextContainsStandaloneToken(current, "unknown") && typeTextContainsStandaloneToken(candidate, "unknown") {
		return false
	}
	if shouldRefineActionPayloadType(current, candidate) {
		return true
	}

	currentMembers, currentOK := parseObjectTypeMembers(current)
	candidateMembers, candidateOK := parseObjectTypeMembers(candidate)
	if !currentOK || !candidateOK {
		return false
	}

	improved := false
	for name, candidateType := range candidateMembers {
		currentType, ok := currentMembers[name]
		if !ok {
			improved = true
			continue
		}
		if shouldPreferCandidateActionPayloadMemberType(currentType, candidateType) {
			improved = true
		}
	}
	return improved
}

func shouldPreferCandidateActionPayloadMemberType(current, candidate string) bool {
	current = normalizeActionPayloadType(current)
	candidate = normalizeActionPayloadType(candidate)
	if candidate == "" || current == candidate {
		return false
	}
	if current == "" {
		return true
	}
	if typeTextContainsStandaloneToken(current, "any") && !typeTextContainsStandaloneToken(candidate, "any") {
		return true
	}
	if typeTextContainsStandaloneToken(current, "unknown") && !typeTextContainsStandaloneToken(candidate, "unknown") {
		return true
	}
	if genericSpecializesActionPayloadType(current, candidate) {
		return true
	}
	if !typeTextContainsStandaloneToken(current, "any") && typeTextContainsStandaloneToken(candidate, "any") {
		return false
	}
	if !typeTextContainsStandaloneToken(current, "unknown") && typeTextContainsStandaloneToken(candidate, "unknown") {
		return false
	}
	if shouldRefineActionPayloadType(current, candidate) {
		return true
	}
	if candidateAddsExtraNullishToActionPayloadMember(current, candidate) {
		return actionPayloadNeedsSourceRecovery(current)
	}
	return strings.Contains(candidate, "<"+current+">")
}

func mergePreferredActionPayloadTypePreservingCurrentMembers(current, candidate string, preservedCurrentMembers map[string]bool) string {
	current = normalizeActionPayloadType(current)
	candidate = normalizeActionPayloadType(candidate)
	if len(preservedCurrentMembers) == 0 {
		return mergePreferredActionPayloadType(current, candidate)
	}
	if candidate == "" {
		return current
	}
	if current == "" {
		return candidate
	}

	currentMembers, currentOK := parseActionPayloadObjectMembers(current)
	candidateMembers, candidateOK := parseActionPayloadObjectMembers(candidate)
	if !currentOK || !candidateOK {
		return mergePreferredActionPayloadType(current, candidate)
	}

	merged := make(map[string]actionPayloadObjectMember, len(currentMembers)+len(candidateMembers))
	for name, value := range currentMembers {
		merged[name] = value
	}

	changed := false
	for name, candidateMember := range candidateMembers {
		if preservedCurrentMembers[name] {
			if _, ok := merged[name]; ok {
				continue
			}
		}
		currentMember, ok := merged[name]
		if !ok {
			merged[name] = candidateMember
			changed = true
			continue
		}
		nextMember := currentMember
		if shouldPreferCandidateActionPayloadMemberType(currentMember.Type, candidateMember.Type) {
			nextMember.Type = candidateMember.Type
		}
		if candidateMember.Optional && !currentMember.Optional {
			nextMember.Optional = true
		}
		if nextMember != currentMember {
			merged[name] = nextMember
			changed = true
		}
	}

	if !changed {
		return current
	}
	return renderActionPayloadObjectMembers(merged)
}

func isUsableIndexedAccessActionPayloadOverrideType(typeText string) bool {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if text == "" ||
		isAnyLikeType(text) ||
		typeTextContainsStandaloneToken(text, "any") ||
		typeTextContainsStandaloneToken(text, "unknown") ||
		isGenericIndexSignatureType(text) ||
		strings.Contains(text, "...") {
		return false
	}
	return true
}

func isPrimitiveLikeKeyType(typeText string) bool {
	return isPrimitiveLikeUnionType(typeText)
}

func isPrimitiveLikeUnionType(typeText string) bool {
	parts, err := splitTopLevelUnion(typeText)
	if err != nil || len(parts) == 0 {
		parts = []string{typeText}
	}

	sawPrimitive := false
	for _, part := range parts {
		part = normalizeSourceTypeText(strings.TrimSpace(part))
		switch part {
		case "string", "number", "boolean", "null", "undefined":
			sawPrimitive = true
			continue
		}
		if isQuotedString(part) || isNumericLiteralType(part) || isBooleanLiteralType(part) {
			sawPrimitive = true
			continue
		}
		return false
	}
	return sawPrimitive
}

func isBroadScalarPrimitiveType(typeText string) bool {
	switch normalizeSourceTypeText(strings.TrimSpace(typeText)) {
	case "string", "number", "boolean":
		return true
	default:
		return false
	}
}

func propsTypeNeedsSourceRecovery(typeText string) bool {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if text == "" ||
		isAnyLikeType(text) ||
		typeTextContainsStandaloneToken(text, "any") ||
		typeTextContainsStandaloneToken(text, "unknown") {
		return true
	}
	if strings.HasPrefix(text, "LogicBuilder<") {
		return true
	}
	return isFunctionLikeTypeText(text)
}

func keyTypeNeedsSourceRecovery(typeText string) bool {
	raw := strings.TrimSpace(typeText)
	if raw == "" ||
		strings.Contains(raw, "`") ||
		strings.Contains(raw, "${") ||
		strings.Contains(raw, ";") {
		return true
	}

	text := normalizeSourceTypeText(raw)
	if text == "" ||
		isAnyLikeType(text) ||
		typeTextContainsStandaloneToken(text, "any") ||
		typeTextContainsStandaloneToken(text, "unknown") {
		return true
	}
	return !isPrimitiveLikeKeyType(text)
}

func shouldPreferRecoveredKeyType(current, candidate string) bool {
	current = normalizeSourceTypeText(strings.TrimSpace(current))
	candidate = normalizeSourceTypeText(strings.TrimSpace(candidate))
	if candidate == "" || current == candidate {
		return false
	}
	if current == "" {
		return true
	}

	currentWeak := isAnyLikeType(current) ||
		typeTextContainsStandaloneToken(current, "any") ||
		typeTextContainsStandaloneToken(current, "unknown")
	candidateWeak := isAnyLikeType(candidate) ||
		typeTextContainsStandaloneToken(candidate, "any") ||
		typeTextContainsStandaloneToken(candidate, "unknown")
	if currentWeak != candidateWeak {
		return !candidateWeak
	}

	currentPrimitive := isPrimitiveLikeKeyType(current)
	candidatePrimitive := isPrimitiveLikeKeyType(candidate)
	if currentPrimitive != candidatePrimitive {
		return candidatePrimitive
	}

	return keyTypeNeedsSourceRecovery(current) && !keyTypeNeedsSourceRecovery(candidate)
}

func mergePreferredActionPayloadType(current, candidate string) string {
	return mergePreferredActionPayloadTypePreservingMembers(current, candidate, nil)
}

func mergePreferredActionPayloadTypePreservingMembers(current, candidate string, preservedMembers map[string]bool) string {
	current = normalizeActionPayloadType(current)
	candidate = normalizeActionPayloadType(candidate)
	if candidate == "" {
		return current
	}
	if current == "" {
		return candidate
	}

	currentMembers, currentOK := parseActionPayloadObjectMembers(current)
	candidateMembers, candidateOK := parseActionPayloadObjectMembers(candidate)
	if !currentOK || !candidateOK {
		if shouldPreferCandidateActionPayloadType(current, candidate) {
			return candidate
		}
		return current
	}

	merged := make(map[string]actionPayloadObjectMember, len(currentMembers)+len(candidateMembers))
	for name, value := range currentMembers {
		merged[name] = value
	}

	changed := false
	for name, candidateMember := range candidateMembers {
		currentMember, ok := merged[name]
		if !ok {
			merged[name] = candidateMember
			changed = true
			continue
		}
		if preservedMembers[name] && candidateAddsOnlyUndefinedToActionPayloadMember(currentMember.Type, candidateMember.Type) {
			continue
		}
		nextMember := currentMember
		if shouldPreferCandidateActionPayloadMemberType(currentMember.Type, candidateMember.Type) {
			nextMember.Type = candidateMember.Type
		}
		if candidateMember.Optional && !currentMember.Optional {
			nextMember.Optional = true
		}
		if nextMember != currentMember {
			merged[name] = nextMember
			changed = true
		}
	}

	if !changed {
		return current
	}
	return renderActionPayloadObjectMembers(merged)
}

type actionPayloadObjectMember struct {
	Optional bool
	Type     string
}

func parseActionPayloadObjectMembers(typeText string) (map[string]actionPayloadObjectMember, bool) {
	text := strings.TrimSpace(typeText)
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return nil, false
	}

	body := strings.TrimSpace(text[1 : len(text)-1])
	if body == "" {
		return map[string]actionPayloadObjectMember{}, true
	}

	entries, err := splitTopLevelTypeMembers(body)
	if err != nil {
		return nil, false
	}

	properties := make(map[string]actionPayloadObjectMember, len(entries))
	for _, entry := range entries {
		rawName, value, ok := splitTopLevelPropertyRaw(entry)
		if !ok {
			continue
		}
		name, optional := normalizedActionPayloadObjectMemberName(rawName)
		if name == "" || value == "" {
			continue
		}
		properties[name] = actionPayloadObjectMember{
			Optional: optional,
			Type:     normalizeSelectorOptionalUndefinedTypeText(strings.TrimSpace(value)),
		}
	}
	return properties, true
}

func normalizedActionPayloadObjectMemberName(rawName string) (string, bool) {
	name := strings.TrimSpace(rawName)
	optional := strings.HasSuffix(name, "?")
	name = strings.TrimSuffix(name, "?")
	name = strings.Trim(name, `"'`)
	return name, optional
}

func renderActionPayloadObjectMembers(properties map[string]actionPayloadObjectMember) string {
	if len(properties) == 0 {
		return "{}"
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		member := properties[name]
		optional := ""
		if member.Optional {
			optional = "?"
		}
		parts = append(parts, fmt.Sprintf("%s%s: %s", name, optional, member.Type))
	}
	return "{ " + strings.Join(parts, "; ") + "; }"
}

func candidateAddsOnlyUndefinedToActionPayloadMember(current, candidate string) bool {
	currentMain, currentNullish, ok := actionPayloadMainType(current)
	if !ok {
		return false
	}
	candidateMain, candidateNullish, ok := actionPayloadMainType(candidate)
	if !ok {
		return false
	}
	return currentMain == candidateMain && currentNullish == "" && candidateNullish == "undefined"
}

func candidateAddsExtraNullishToActionPayloadMember(current, candidate string) bool {
	currentMain, currentNullish, ok := actionPayloadMainType(current)
	if !ok {
		return false
	}
	candidateMain, candidateNullish, ok := actionPayloadMainType(candidate)
	if !ok || currentMain != candidateMain || currentNullish == candidateNullish || candidateNullish == "" {
		return false
	}
	currentNullishParts := strings.Split(currentNullish, "|")
	seen := map[string]bool{}
	for _, part := range currentNullishParts {
		part = strings.TrimSpace(part)
		if part != "" {
			seen[part] = true
		}
	}
	for _, part := range strings.Split(candidateNullish, "|") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		return true
	}
	return false
}

func genericSpecializesActionPayloadType(current, candidate string) bool {
	currentMain, currentNullish, ok := actionPayloadMainType(current)
	if !ok {
		return false
	}
	candidateMain, candidateNullish, ok := actionPayloadMainType(candidate)
	if !ok || currentNullish != candidateNullish {
		return false
	}
	return currentMain != candidateMain && strings.HasPrefix(candidateMain, currentMain+"<") && strings.HasSuffix(candidateMain, ">")
}

func actionPayloadMainType(typeText string) (string, string, bool) {
	parts, err := splitTopLevelUnion(normalizeActionPayloadType(typeText))
	if err != nil {
		return "", "", false
	}
	mainParts := make([]string, 0, len(parts))
	nullish := make([]string, 0, 2)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case "", "null", "undefined":
			if part != "" {
				nullish = append(nullish, part)
			}
		default:
			mainParts = append(mainParts, part)
		}
	}
	if len(mainParts) != 1 {
		return "", "", false
	}
	sort.Strings(nullish)
	return mainParts[0], strings.Join(nullish, "|"), true
}

func sourceDefaultedParameterNames(parameters string) map[string]bool {
	parsed, ok := parseFunctionParameters(parameters)
	if !ok || len(parsed) == 0 {
		return nil
	}

	names := map[string]bool{}
	for _, parameter := range parsed {
		if !strings.Contains(parameter.Text, "=") {
			continue
		}
		name, ok := sourceParameterName(parameter.Text)
		if !ok {
			continue
		}
		names[name] = true
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

func parseReducers(section SectionReport) []ParsedField {
	return parseReducersWithSource(section, "", SourceProperty{}, "", nil)
}

func parseReducersWithSource(section SectionReport, source string, property SourceProperty, file string, state *buildState) []ParsedField {
	sourceMembers := sectionSourceProperties(source, property)
	sourceEntries := sourceEntriesByName(sectionSourceEntries(source, property))
	memberByName := map[string]MemberReport{}
	for _, member := range section.Members {
		memberByName[member.Name] = member
	}
	orderedNames := sourceOrderedSectionMemberNames(source, property, section.Members)
	reducers := make([]ParsedField, 0, len(orderedNames))
	for _, name := range orderedNames {
		member, ok := memberByName[name]
		if !ok {
			continue
		}
		stateType := "any"
		if nested, ok := sourceMembers[name]; ok {
			if hinted := sourceReducerStateTypeFromProperty(source, file, nested, state); hinted != "" {
				stateType = hinted
			}
		} else if entry, ok := sourceEntries[name]; ok {
			if hinted := sourceReducerStateTypeWithContext(source, file, entry.Value, state); hinted != "" {
				stateType = hinted
			}
		}
		if parsed, ok := parseReducerStateType(member.TypeString); ok {
			stateType = preferredReducerStateType(stateType, parsed)
		}
		if nested, ok := sourceMembers[name]; ok {
			if asserted := sourceAssertedType(firstTopLevelArrayElement(sourcePropertyText(source, nested))); asserted != "" {
				stateType = preferredAssertedReducerStateType(stateType, asserted)
			}
		}
		reducers = append(reducers, ParsedField{Name: name, Type: stateType})
	}
	return reducers
}

func parseDefaultFields(section SectionReport) []ParsedField {
	return parseDefaultFieldsWithSource(section, "", SourceProperty{}, "", nil)
}

func parseDefaultFieldsWithSource(section SectionReport, source string, property SourceProperty, file string, state *buildState) []ParsedField {
	sourceMembers := sectionSourceProperties(source, property)
	memberByName := map[string]MemberReport{}
	for _, member := range section.Members {
		memberByName[member.Name] = member
	}
	orderedNames := sourceOrderedSectionMemberNames(source, property, section.Members)
	fields := make([]ParsedField, 0, len(orderedNames))
	for _, name := range orderedNames {
		member, ok := memberByName[name]
		if !ok {
			continue
		}
		fieldType := strings.TrimSpace(preferredMemberTypeText(member))
		if nested, ok := sourceMembers[name]; ok {
			expression := sourcePropertyText(source, nested)
			if hinted := sourceAssertedType(expression); hinted != "" {
				fieldType = hinted
			} else if typeTextNeedsSourceRecovery(fieldType) {
				for _, recovered := range []string{
					normalizeSourceTypeText(sourceExpressionTypeTextWithContext(source, file, expression, nil, state)),
					normalizeSourceTypeText(sourceExpressionTypeText(source, expression)),
				} {
					if recovered == "" || typeTextNeedsSourceRecovery(recovered) {
						continue
					}
					fieldType = recovered
					break
				}
			}
		}
		if fieldType == "" {
			continue
		}
		fields = append(fields, ParsedField{Name: name, Type: fieldType})
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

func parseFormsPluginSection(section SectionReport) ([]ParsedAction, []ParsedField, []ParsedField, []TypeImport) {
	return parseFormsPluginSectionWithSource(section, "", SourceProperty{}, "", nil)
}

func parseFormsPluginSectionWithSource(
	section SectionReport,
	source string,
	property SourceProperty,
	file string,
	state *buildState,
) ([]ParsedAction, []ParsedField, []ParsedField, []TypeImport) {
	actions := make([]ParsedAction, 0, len(section.Members)*9)
	reducers := make([]ParsedField, 0, len(section.Members)*6)
	selectors := make([]ParsedField, 0, len(section.Members)*6)
	sourceMembers := sectionSourceProperties(source, property)
	memberByName := map[string]MemberReport{}
	for _, member := range section.Members {
		memberByName[member.Name] = member
	}
	for _, formName := range sourceOrderedSectionMemberNames(source, property, section.Members) {
		member, ok := memberByName[formName]
		if !ok {
			continue
		}
		formName = strings.TrimSpace(formName)
		if formName == "" {
			continue
		}
		formType := formPluginValueType(member)
		if nested, ok := sourceMembers[formName]; ok {
			if recovered := sourceFormsPluginValueType(source, file, nested, state); recovered != "" && !typeTextNeedsSourceRecovery(recovered) {
				formType = recovered
			}
		}
		if !shouldEmitBuilderFormsPluginSurface(formType) {
			continue
		}
		formTitle := upperIdentifier(formName)

		actions = append(actions,
			ParsedAction{
				Name:         "set" + formTitle + "Value",
				FunctionType: "(key: FieldName, value: any) => { name: FieldName; value: any; }",
				PayloadType:  "{ name: FieldName; value: any; }",
			},
			ParsedAction{
				Name:         "set" + formTitle + "Values",
				FunctionType: fmt.Sprintf("(values: DeepPartial<%s>) => { values: DeepPartial<%s>; }", formType, formType),
				PayloadType:  fmt.Sprintf("{ values: DeepPartial<%s>; }", formType),
			},
			ParsedAction{
				Name:         "set" + formTitle + "ManualErrors",
				FunctionType: "(errors: Record<string, any>) => { errors: Record<string, any>; }",
				PayloadType:  "{ errors: Record<string, any>; }",
			},
			ParsedAction{
				Name:         "touch" + formTitle + "Field",
				FunctionType: "(key: string) => { key: string; }",
				PayloadType:  "{ key: string; }",
			},
			ParsedAction{
				Name:         "reset" + formTitle,
				FunctionType: fmt.Sprintf("(values?: %s) => { values?: %s; }", formType, formType),
				PayloadType:  fmt.Sprintf("{ values?: %s; }", formType),
			},
			ParsedAction{
				Name:         "submit" + formTitle,
				FunctionType: "() => { value: boolean }",
				PayloadType:  "{ value: boolean }",
			},
			ParsedAction{
				Name:         "submit" + formTitle + "Request",
				FunctionType: fmt.Sprintf("(%s: %s) => { %s: %s; }", formName, formType, formName, formType),
				PayloadType:  fmt.Sprintf("{ %s: %s; }", formName, formType),
			},
			ParsedAction{
				Name:         "submit" + formTitle + "Success",
				FunctionType: fmt.Sprintf("(%s: %s) => { %s: %s; }", formName, formType, formName, formType),
				PayloadType:  fmt.Sprintf("{ %s: %s; }", formName, formType),
			},
			ParsedAction{
				Name:         "submit" + formTitle + "Failure",
				FunctionType: "(error: Error, errors: Record<string, any>) => { error: Error; errors: Record<string, any>; }",
				PayloadType:  "{ error: Error; errors: Record<string, any>; }",
			},
		)

		reducers = append(reducers,
			ParsedField{Name: formName, Type: formType},
			ParsedField{Name: "is" + formTitle + "Submitting", Type: "boolean"},
			ParsedField{Name: "show" + formTitle + "Errors", Type: "boolean"},
			ParsedField{Name: formName + "Changed", Type: "boolean"},
			ParsedField{Name: formName + "Touches", Type: "Record<string, boolean>"},
			ParsedField{Name: formName + "ManualErrors", Type: "Record<string, any>"},
		)

		selectors = append(selectors,
			ParsedField{Name: formName + "Touched", Type: "boolean"},
			ParsedField{Name: formName + "ValidationErrors", Type: fmt.Sprintf("DeepPartialMap<%s, ValidationErrorType>", formType)},
			ParsedField{Name: formName + "AllErrors", Type: "Record<string, any>"},
			ParsedField{Name: formName + "HasErrors", Type: "boolean"},
			ParsedField{Name: formName + "Errors", Type: fmt.Sprintf("DeepPartialMap<%s, ValidationErrorType>", formType)},
			ParsedField{Name: "is" + formTitle + "Valid", Type: "boolean"},
		)
	}

	if len(actions) == 0 && len(reducers) == 0 && len(selectors) == 0 {
		return nil, nil, nil, nil
	}

	return actions, reducers, selectors, []TypeImport{{
		Path:  "kea-forms",
		Names: []string{"DeepPartial", "DeepPartialMap", "FieldName", "ValidationErrorType"},
	}}
}

func formPluginValueType(member MemberReport) string {
	for _, candidate := range []string{
		strings.TrimSpace(preferredMemberTypeText(member)),
		strings.TrimSpace(member.TypeString),
	} {
		if candidate == "" {
			continue
		}
		properties, ok := parseObjectTypeMembers(candidate)
		if !ok {
			continue
		}
		if defaults := normalizeSourceTypeText(strings.TrimSpace(properties["defaults"])); defaults != "" {
			return defaults
		}
	}
	return "Record<string, any>"
}

func sourceFormsPluginValueType(source, file string, property SourceProperty, state *buildState) string {
	expression := sourcePropertyText(source, property)
	if expression == "" {
		return ""
	}
	entryByName := sourceEntriesByName(sourceObjectEntriesFromExpression(expression))
	if len(entryByName) == 0 {
		return ""
	}
	defaultsEntry, ok := entryByName["defaults"]
	if !ok {
		return ""
	}
	for _, candidate := range []string{
		sourceAssertedType(defaultsEntry.Value),
		normalizeSourceTypeText(sourceObjectLiteralTypeTextWithHintsOptions(source, defaultsEntry.Value, nil, true)),
		normalizeSourceTypeText(sourceExpressionTypeTextWithContext(source, file, defaultsEntry.Value, nil, state)),
		normalizeSourceTypeText(sourceExpressionTypeText(expression, defaultsEntry.Value)),
	} {
		candidate = normalizeSourceTypeText(strings.TrimSpace(candidate))
		if candidate == "" || isAnyLikeType(candidate) || strings.Contains(candidate, "...") {
			continue
		}
		return candidate
	}
	return ""
}

func shouldEmitBuilderFormsPluginSurface(formType string) bool {
	formType = normalizeSourceTypeText(strings.TrimSpace(formType))
	if formType == "" || isAnyLikeType(formType) {
		return false
	}
	if strings.Contains(formType, "...") {
		return false
	}
	if properties, ok := parseObjectTypeMembers(formType); ok {
		return len(properties) > 0
	}
	return true
}

func preferredMemberTypeText(member MemberReport) string {
	return preferredPrintedOrRawTypeText(member.PrintedTypeNode, member.TypeString)
}

func preferredMemberFunctionTypeText(member MemberReport) string {
	return normalizeSourceTypeText(strings.TrimSpace(preferredMemberTypeText(member)))
}

func preferredMemberReturnTypeText(member MemberReport) string {
	text := preferredPrintedOrRawTypeText(member.PrintedReturnTypeNode, member.ReturnTypeString)
	if text == "" {
		return ""
	}
	return normalizeSourceTypeTextWithOptions(text, false)
}

func preferredSelectorInputTupleTypeText(member MemberReport) string {
	return preferredPrintedOrRawTypeText(member.SelectorInputPrintedReturnTypeNode, member.SelectorInputReturnTypeString)
}

func preferredSelectorProjectorTypeText(member MemberReport) string {
	return preferredPrintedOrRawTypeText(member.ProjectorPrintedTypeNode, member.ProjectorTypeString)
}

func preferredSelectorProjectorReturnTypeText(member MemberReport) string {
	text := preferredPrintedOrRawTypeText(member.ProjectorPrintedReturnTypeNode, member.ProjectorReturnTypeString)
	if text == "" {
		return ""
	}
	return normalizeSourceTypeTextWithOptions(text, false)
}

func preferredPrintedOrRawTypeText(printed, raw string) string {
	printed = strings.TrimSpace(printed)
	raw = strings.TrimSpace(raw)
	if printed != "" && (!strings.Contains(printed, "...") || strings.Contains(raw, "...")) {
		return printed
	}
	return raw
}

func selectorReportedMemberReturnTypeText(member MemberReport) string {
	if projectorReturn := strings.TrimSpace(preferredSelectorProjectorReturnTypeText(member)); projectorReturn != "" {
		return projectorReturn
	}
	for _, text := range []string{
		strings.TrimSpace(member.TypeString),
		strings.TrimSpace(member.PrintedTypeNode),
	} {
		if selectorMemberTypeTextSupportsInternalHelper(text) {
			// Tuple-shaped selector members can report the dependency collector signature
			// instead of the computed callback return when the direct projector surface
			// is not available.
			return ""
		}
	}
	return strings.TrimSpace(preferredMemberReturnTypeText(member))
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
	typeArguments := ""
	if strings.HasPrefix(rest, "<") {
		end, err := findMatching(rest, 0, '<', '>')
		if err != nil {
			return ""
		}
		typeArguments = strings.TrimSpace(rest[:end+1])
		rest = strings.TrimSpace(rest[end+1:])
	}
	if rest != "" && !strings.HasPrefix(rest, "(") {
		return ""
	}
	if typeArguments != "" {
		return normalizeSourceTypeText(qualified + typeArguments)
	}
	return qualified
}

func sourceNewExpressionTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
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
	typeArguments := ""
	if strings.HasPrefix(rest, "<") {
		end, err := findMatching(rest, 0, '<', '>')
		if err != nil {
			return ""
		}
		typeArguments = strings.TrimSpace(rest[:end+1])
		rest = strings.TrimSpace(rest[end+1:])
	}
	if rest != "" && !strings.HasPrefix(rest, "(") {
		return ""
	}
	if typeArguments != "" {
		return normalizeSourceTypeText(qualified + typeArguments)
	}
	if rest == "" {
		return qualified
	}

	end, err := findMatching(rest, 0, '(', ')')
	if err != nil || trimExpressionEnd(rest, end+1) != len(rest) {
		return qualified
	}
	argumentsText := strings.TrimSpace(rest[1:end])
	arguments, err := splitTopLevelList(argumentsText)
	if err != nil {
		return qualified
	}

	switch qualified {
	case "Set", "ReadonlySet", "Iterable":
		if elementType := setConstructorElementType(arguments, source, file, hints, state); elementType != "" {
			return normalizeSourceTypeText(fmt.Sprintf("%s<%s>", qualified, elementType))
		}
	case "Map", "ReadonlyMap":
		keyType, valueType, ok := mapConstructorTypes(arguments, source, file, hints, state)
		if ok {
			return normalizeSourceTypeText(fmt.Sprintf("%s<%s, %s>", qualified, keyType, valueType))
		}
	}
	return qualified
}

func setConstructorElementType(arguments []string, source, file string, hints map[string]string, state *buildState) string {
	if len(arguments) == 0 {
		return ""
	}
	collectionType := sourceCollectionExpressionTypeTextWithContext(source, file, strings.TrimSpace(arguments[0]), hints, state)
	return collectionElementType(collectionType)
}

func mapConstructorTypes(arguments []string, source, file string, hints map[string]string, state *buildState) (string, string, bool) {
	if len(arguments) == 0 {
		return "", "", false
	}
	collectionType := sourceCollectionExpressionTypeTextWithContext(source, file, strings.TrimSpace(arguments[0]), hints, state)
	elementType := collectionElementType(collectionType)
	if elementType == "" {
		return "", "", false
	}
	if keyType, valueType, ok := tupleKeyValueTypes(elementType); ok {
		return keyType, valueType, true
	}
	return "", "", false
}

func sourceCollectionExpressionTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
	text := strings.TrimSpace(expression)
	if arrayType := sourceArrayLiteralTypeTextWithContext(source, file, text, hints, state); arrayType != "" {
		return arrayType
	}
	return sourceExpressionTypeTextWithContext(source, file, text, hints, state)
}

func collectionElementType(typeText string) string {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	switch {
	case strings.HasSuffix(text, "[]"):
		return normalizeSourceTypeText(strings.TrimSpace(text[:len(text)-2]))
	case strings.HasPrefix(text, "Array<") && strings.HasSuffix(text, ">"):
		return normalizeSourceTypeText(strings.TrimSpace(text[len("Array<") : len(text)-1]))
	case strings.HasPrefix(text, "ReadonlyArray<") && strings.HasSuffix(text, ">"):
		return normalizeSourceTypeText(strings.TrimSpace(text[len("ReadonlyArray<") : len(text)-1]))
	case strings.HasPrefix(text, "Set<") && strings.HasSuffix(text, ">"):
		return normalizeSourceTypeText(strings.TrimSpace(text[len("Set<") : len(text)-1]))
	case strings.HasPrefix(text, "ReadonlySet<") && strings.HasSuffix(text, ">"):
		return normalizeSourceTypeText(strings.TrimSpace(text[len("ReadonlySet<") : len(text)-1]))
	case strings.HasPrefix(text, "Iterable<") && strings.HasSuffix(text, ">"):
		return normalizeSourceTypeText(strings.TrimSpace(text[len("Iterable<") : len(text)-1]))
	default:
		return ""
	}
}

func tupleKeyValueTypes(typeText string) (string, string, bool) {
	text := strings.TrimSpace(typeText)
	if !strings.HasPrefix(text, "[") || !strings.HasSuffix(text, "]") {
		return "", "", false
	}
	parts, err := splitTopLevelList(text[1 : len(text)-1])
	if err != nil || len(parts) != 2 {
		return "", "", false
	}
	keyType := normalizeSourceTypeText(strings.TrimSpace(parts[0]))
	valueType := normalizeSourceTypeText(strings.TrimSpace(parts[1]))
	if keyType == "" || valueType == "" {
		return "", "", false
	}
	return keyType, valueType, true
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

func sourceArrayLiteralTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
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
			spreadType := strings.TrimSpace(sourceExpressionTypeTextWithContext(source, file, strings.TrimSpace(part[3:]), hints, state))
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
			part = strings.TrimSpace(sourceExpressionTypeTextWithContext(source, file, part, hints, state))
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
	return normalizeSourceObjectPropertyType(typeText, false)
}

func normalizeSourceObjectPropertyType(typeText string, preserveLiteralType bool) string {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if preserveLiteralType && isBooleanLiteralType(text) {
		return text
	}
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
	return expandLocalSourceTypeTextWithSeen(source, typeText, map[string]bool{})
}

func expandLocalSourceTypeTextWithSeen(source, typeText string, seen map[string]bool) string {
	text := normalizeSourceTypeText(typeText)
	if text == "" {
		return ""
	}
	lookup := text
	if identifier, ok := sourceIdentifierExpression(text); ok {
		lookup = identifier
	}
	if specialized := expandLocalDefaultedGenericTypeTextWithSeen(source, lookup, seen); specialized != "" {
		return specialized
	}
	declared := findLocalTypeAliasTextWithSeen(source, lookup, seen)
	if declared == "" {
		return ""
	}
	if strings.HasPrefix(strings.TrimSpace(declared), "{") && strings.HasSuffix(strings.TrimSpace(declared), "}") {
		return normalizeInlineObjectTypeText(declared)
	}
	return normalizeSourceTypeText(declared)
}

func expandLocalDefaultedGenericTypeTextWithSeen(source, name string, seen map[string]bool) string {
	defaults := localTypeParameterDefaults(source, name)
	if len(defaults) == 0 {
		return ""
	}
	declared := findLocalTypeAliasTextWithSeen(source, name, seen)
	if declared == "" {
		return ""
	}

	specialized := declared
	for parameterName, defaultType := range defaults {
		if parameterName == "" || defaultType == "" {
			return ""
		}
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(parameterName) + `\b`)
		specialized = pattern.ReplaceAllString(specialized, defaultType)
	}
	specialized = normalizeSourceTypeText(specialized)
	if specialized == "" || specialized == normalizeSourceTypeText(declared) {
		return ""
	}
	return specialized
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
	return findLocalTypeAliasTextWithSeen(source, name, map[string]bool{})
}

func findLocalTypeAliasTextWithSeen(source, name string, seen map[string]bool) string {
	name = strings.TrimSpace(name)
	if name == "" || seen[name] {
		return ""
	}
	seen[name] = true
	defer delete(seen, name)

	interfacePattern := regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:declare\s+)?interface\s+` + regexp.QuoteMeta(name) + `\b[^{]*\{`)
	if match := interfacePattern.FindStringIndex(source); match != nil {
		header := source[match[0]:match[1]]
		braceOffset := strings.LastIndex(header, "{")
		if braceOffset != -1 {
			start := match[0] + braceOffset
			end, err := findMatching(source, start, '{', '}')
			if err == nil {
				body := source[start : end+1]
				if merged := mergeInterfaceHeritageObjectType(source, header, body, seen); merged != "" {
					return merged
				}
				return body
			}
		}
	}

	typePattern := regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:declare\s+)?type\s+` + regexp.QuoteMeta(name) + `\b`)
	if match := typePattern.FindStringIndex(source); match != nil {
		start := skipTrivia(source, match[1])
		if start < len(source) && source[start] == '<' {
			end, err := findMatching(source, start, '<', '>')
			if err != nil {
				return ""
			}
			start = skipTrivia(source, end+1)
		}
		if start >= len(source) || source[start] != '=' {
			return ""
		}
		start = skipTrivia(source, start+1)
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

func mergeInterfaceHeritageObjectType(source, header, body string, seen map[string]bool) string {
	heritageNames := parseInterfaceHeritageNames(header)
	if len(heritageNames) == 0 {
		return ""
	}

	typeTexts := make([]string, 0, len(heritageNames)+1)
	for _, heritageName := range heritageNames {
		if expanded := expandLocalSourceTypeTextWithSeen(source, heritageName, seen); expanded != "" {
			typeTexts = append(typeTexts, expanded)
		}
	}
	typeTexts = append(typeTexts, body)
	return mergeObjectTypeTexts(typeTexts...)
}

func parseInterfaceHeritageNames(header string) []string {
	text := strings.TrimSpace(header)
	if text == "" {
		return nil
	}
	if brace := strings.LastIndex(text, "{"); brace != -1 {
		text = strings.TrimSpace(text[:brace])
	}
	index := strings.Index(text, "extends")
	if index == -1 {
		return nil
	}
	heritageText := strings.TrimSpace(text[index+len("extends"):])
	if heritageText == "" {
		return nil
	}
	parts, err := splitTopLevelList(heritageText)
	if err != nil {
		return nil
	}

	names := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if identifier, ok := sourceIdentifierExpression(part); ok {
			part = identifier
		}
		names = append(names, part)
	}
	return names
}

func mergeObjectTypeTexts(typeTexts ...string) string {
	merged := map[string]string{}
	order := []string{}
	seen := map[string]bool{}

	for _, typeText := range typeTexts {
		members, ok := parseObjectTypeMembers(normalizeInlineObjectTypeText(typeText))
		if !ok {
			continue
		}
		names := make([]string, 0, len(members))
		for name := range members {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if !seen[name] {
				seen[name] = true
				order = append(order, name)
			}
			merged[name] = normalizeSourceTypeText(strings.TrimSpace(members[name]))
		}
	}
	if len(order) == 0 {
		return ""
	}

	parts := make([]string, 0, len(order))
	for _, name := range order {
		parts = append(parts, fmt.Sprintf("%s: %s", name, merged[name]))
	}
	return "{ " + strings.Join(parts, "; ") + "; }"
}

func findLocalValueInitializer(source, name string) string {
	match, ok := findLocalValueMatch(source, name)
	if !ok {
		return ""
	}
	return strings.TrimSpace(source[match.InitializerStart:match.InitializerEnd])
}

type localValueMatch struct {
	IdentifierStart  int
	InitializerStart int
	InitializerEnd   int
}

func findLocalValueMatch(source, name string) (localValueMatch, bool) {
	valuePattern := regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(name) + `\b[^=]*=\s*`)
	match := valuePattern.FindStringIndex(source)
	if match == nil {
		return localValueMatch{}, false
	}

	identifierOffset := strings.LastIndex(source[match[0]:match[1]], name)
	if identifierOffset == -1 {
		return localValueMatch{}, false
	}
	initializerStart := skipTrivia(source, match[1])
	initializerEnd, err := findStatementExpressionEnd(source, initializerStart)
	if err != nil || initializerEnd <= initializerStart {
		return localValueMatch{}, false
	}
	return localValueMatch{
		IdentifierStart:  match[0] + identifierOffset,
		InitializerStart: initializerStart,
		InitializerEnd:   initializerEnd,
	}, true
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

func sourceFunctionReturnTypeWithContext(source, file, identifier string, state *buildState) string {
	if returnType := findLocalFunctionReturnType(source, identifier); returnType != "" {
		return returnType
	}
	if returnType, ok := sourceImportedFunctionReturnType(source, file, identifier, state); ok {
		return returnType
	}
	return ""
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

func sourceImportedFunctionReturnType(source, file, identifier string, state *buildState) (string, bool) {
	if state == nil || file == "" || identifier == "" {
		return "", false
	}

	candidate, ok := parseNamedValueImports(source)[identifier]
	if !ok || candidate.ImportedName == "" || candidate.ImportedName == "default" {
		return "", false
	}
	resolvedFile, ok := resolveImportFile(file, candidate.Path, state)
	if !ok {
		return "", false
	}
	importedSourceBytes, err := os.ReadFile(resolvedFile)
	if err != nil {
		return "", false
	}
	if returnType := findLocalFunctionReturnType(string(importedSourceBytes), candidate.ImportedName); returnType != "" {
		return returnType, true
	}
	return "", false
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

	for _, member := range section.Members {
		memberByName[member.Name] = member
	}
	orderedNames := sourceOrderedSectionMemberNames(source, property, section.Members)

	for _, name := range orderedNames {
		member, hasMember := memberByName[name]
		defaultType := ""
		properties := map[string]string{}
		ok := false

		if hasMember {
			memberType := strings.TrimSpace(member.TypeString)
			if memberType == "" || strings.Contains(memberType, "...") {
				memberType = strings.TrimSpace(preferredMemberTypeText(member))
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
				if len(sourceProperties) > 0 {
					properties = mergeSourceLoaderProperties(properties, sourceProperties)
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
		if inferred := inferredLoaderDefaultType(defaultType, properties); inferred != "" {
			defaultType = inferred
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

	for _, member := range section.Members {
		memberByName[member.Name] = member
	}
	orderedNames := sourceOrderedSectionMemberNames(source, property, section.Members)

	for _, name := range orderedNames {
		member, hasMember := memberByName[name]
		defaultType := ""
		properties := map[string]string{}
		ok := false

		if hasMember {
			memberType := strings.TrimSpace(member.TypeString)
			if memberType == "" || strings.Contains(memberType, "...") {
				memberType = strings.TrimSpace(preferredMemberTypeText(member))
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
				if len(sourceProperties) > 0 {
					properties = mergeSourceLoaderProperties(properties, sourceProperties)
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
		if inferred := inferredLoaderDefaultType(defaultType, properties); inferred != "" {
			defaultType = inferred
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

	if text[0] == '[' {
		arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(text, 0, len(text))
		if err != nil || !ok || arrayStart != 0 {
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
	if text[0] != '{' {
		return "", nil, false
	}
	properties, ok := sourceLoaderObjectProperties(source, text, "")
	if !ok {
		return "", nil, false
	}
	return strings.TrimSpace(properties["__default"]), properties, true
}

func sourceLoaderMemberTypeFromProperty(source string, property SourceProperty, file string, state *buildState) (string, map[string]string, bool) {
	if source == "" || property.ValueEnd <= property.ValueStart {
		return "", nil, false
	}

	start := skipTrivia(source, property.ValueStart)
	if start >= property.ValueEnd {
		return "", nil, false
	}

	if source[start] == '[' {
		arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(source, property.ValueStart, property.ValueEnd)
		if err != nil || !ok || arrayStart != start {
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
		if state != nil {
			if statelessProperties, statelessOK := sourceLoaderObjectPropertiesFromRange(source, parts[1].Start, parts[1].End, file, nil, defaultType); statelessOK {
				properties = mergeStatelessOpaqueLoaderProperties(properties, statelessProperties)
			}
		}
		properties["__default"] = defaultType
		return defaultType, properties, true
	}
	if source[start] != '{' {
		return "", nil, false
	}
	properties, ok := sourceLoaderObjectPropertiesFromRange(source, property.ValueStart, property.ValueEnd, file, state, "")
	if !ok {
		return "", nil, false
	}
	if state != nil {
		if statelessProperties, statelessOK := sourceLoaderObjectPropertiesFromRange(source, property.ValueStart, property.ValueEnd, file, nil, ""); statelessOK {
			properties = mergeStatelessOpaqueLoaderProperties(properties, statelessProperties)
		}
	}
	return strings.TrimSpace(properties["__default"]), properties, true
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
			result[nested.Name] = refineSourceLoaderFunctionType(functionType, value, defaultTypeHint)
			continue
		}
		if functionType := sourceLoaderArrowFunctionTypeTextWithFallback(source, value, defaultTypeHint); functionType != "" {
			result[nested.Name] = refineSourceLoaderFunctionType(functionType, value, defaultTypeHint)
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
		simpleType := ""
		if functionType := sourceArrowFunctionTypeText(source, value); functionType != "" {
			simpleType = refineSourceLoaderFunctionType(functionType, value, defaultTypeHint)
		}
		rangeType := ""
		if functionType := sourceArrowFunctionTypeTextFromRange(source, file, nested, state); functionType != "" {
			rangeType = refineSourceLoaderFunctionType(functionType, value, defaultTypeHint)
		}
		if preferred := preferredOpaqueAnyArrayLoaderFunctionType(simpleType, rangeType, value); preferred != "" {
			result[nested.Name] = preferred
			continue
		}
		if simpleType != "" && rangeType != "" {
			if shouldPreferSourceLoaderPropertyType(simpleType, rangeType) {
				result[nested.Name] = preferDefaultLoaderFunctionType(rangeType, defaultTypeHint, source, file, state)
			} else {
				result[nested.Name] = preferDefaultLoaderFunctionType(simpleType, defaultTypeHint, source, file, state)
			}
			continue
		}
		if simpleType != "" {
			result[nested.Name] = preferDefaultLoaderFunctionType(simpleType, defaultTypeHint, source, file, state)
			continue
		}
		if rangeType != "" {
			result[nested.Name] = preferDefaultLoaderFunctionType(rangeType, defaultTypeHint, source, file, state)
			continue
		}
		if functionType := sourceLoaderArrowFunctionTypeTextWithFallback(source, value, defaultTypeHint); functionType != "" {
			result[nested.Name] = preferDefaultLoaderFunctionType(
				refineSourceLoaderFunctionType(functionType, value, defaultTypeHint),
				defaultTypeHint,
				source,
				file,
				state,
			)
		}
	}

	if len(result) == 0 {
		return nil, false
	}
	return result, true
}

func preferredOpaqueAnyArrayLoaderFunctionType(simpleType, rangeType, expression string) string {
	simpleOpaque := loaderFunctionTypeUsesSpreadAnyArray(simpleType, expression) || loaderFunctionTypeUsesMappedOpaqueAnyArray(simpleType, expression)
	rangeOpaque := loaderFunctionTypeUsesSpreadAnyArray(rangeType, expression) || loaderFunctionTypeUsesMappedOpaqueAnyArray(rangeType, expression)
	switch {
	case simpleOpaque && rangeOpaque:
		if merged := mergeSourceLoaderPropertyType(simpleType, rangeType); merged != "" {
			return merged
		}
		return simpleType
	case simpleOpaque:
		if rangeType != "" {
			if merged := mergeSourceLoaderPropertyType(rangeType, simpleType); merged != "" {
				return merged
			}
		}
		return simpleType
	case rangeOpaque:
		if simpleType != "" {
			if merged := mergeSourceLoaderPropertyType(simpleType, rangeType); merged != "" {
				return merged
			}
		}
		return rangeType
	default:
		return ""
	}
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

func preferDefaultLoaderFunctionType(functionType, defaultTypeHint, source, file string, state *buildState) string {
	parameters, returnType, ok := splitFunctionType(functionType)
	if !ok {
		return functionType
	}
	preferred := preferredLoaderReturnTypeWithDefaultHint(returnType, defaultTypeHint, source, file, state)
	if preferred == "" || preferred == returnType {
		return functionType
	}
	return parameters + " => " + preferred
}

func preferredLoaderReturnTypeWithDefaultHint(returnType, defaultTypeHint, source, file string, state *buildState) string {
	defaultTypeHint = normalizeInferredTypeText(strings.TrimSpace(defaultTypeHint))
	if defaultTypeHint == "" {
		return returnType
	}

	async := strings.TrimSpace(returnType) != strings.TrimSpace(unwrapPromiseType(returnType))
	mainReturn := normalizeInferredTypeText(strings.TrimSpace(unwrapPromiseType(returnType)))
	if shouldFallbackToLoaderDefaultType(mainReturn, defaultTypeHint) ||
		defaultLoaderTypeImprovesRecoveredReturn(mainReturn, defaultTypeHint, source, file, state) {
		if async {
			return promiseTypeText(defaultTypeHint)
		}
		return defaultTypeHint
	}
	return returnType
}

func defaultLoaderTypeImprovesRecoveredReturn(currentReturn, defaultTypeHint, source, file string, state *buildState) bool {
	currentReturn = normalizeInferredTypeText(strings.TrimSpace(currentReturn))
	defaultTypeHint = normalizeInferredTypeText(strings.TrimSpace(defaultTypeHint))
	if currentReturn == "" || defaultTypeHint == "" || currentReturn == defaultTypeHint {
		return false
	}

	expandedDefault := normalizeSourceTypeText(expandSourceParameterHintTypeText(source, file, defaultTypeHint, state))
	if expandedDefault == "" {
		expandedDefault = defaultTypeHint
	}
	if actionPayloadMembersImproved(currentReturn, expandedDefault) {
		return true
	}
	return isArrayLikeType(expandedDefault) && !isArrayLikeType(currentReturn) && !isAnyLikeType(currentReturn)
}

func refineSourceLoaderFunctionType(functionType, expression, defaultTypeHint string) string {
	parameters, returnType, ok := splitFunctionType(functionType)
	if !ok {
		return functionType
	}
	info, ok := parseSourceArrowInfo(expression)
	if !ok || sourceHasExplicitNullishReturnPath(info.Body) {
		return functionType
	}
	defaultTypeHint = normalizeInferredTypeText(strings.TrimSpace(defaultTypeHint))
	if widened, ok := widenSpreadLoaderArrayReturnType(returnType, info.Async, info.Body); ok {
		return parameters + " => " + widened
	}
	if widened, ok := widenMappedOpaqueLoaderArrayReturnType(returnType, info.Async, info.Body); ok {
		return parameters + " => " + widened
	}
	if normalized, ok := preferDefaultLoaderArrayReturnType(returnType, defaultTypeHint, info.Async, info.Body); ok {
		return parameters + " => " + normalized
	}

	defaultMain, defaultNullish, ok := returnTypeMainType(defaultTypeHint)
	if loaderSingleReturnUsesAwaitedJSONValue(info.Body) {
		preferredDefault := defaultTypeHint
		if ok && defaultNullish != "" {
			preferredDefault = defaultMain
		}
		if preferredDefault != "" {
			if info.Async {
				return parameters + " => " + promiseTypeText(preferredDefault)
			}
			return parameters + " => " + preferredDefault
		}
	}
	if !ok || defaultNullish == "" {
		return functionType
	}
	if loaderSingleReturnUsesAwaitedLocalValue(info.Body) {
		if info.Async {
			return parameters + " => " + promiseTypeText(defaultMain)
		}
		return parameters + " => " + defaultMain
	}
	returnMain, returnNullish, ok := returnTypeMainType(returnType)
	if !ok || returnMain != defaultMain || returnNullish == "" {
		return functionType
	}
	if info.Async {
		return parameters + " => " + promiseTypeText(defaultMain)
	}
	return parameters + " => " + defaultMain
}

func widenSpreadLoaderArrayReturnType(returnType string, async bool, body string) (string, bool) {
	mainReturn := normalizeSourceTypeText(unwrapPromiseType(returnType))
	if !isArrayLikeType(mainReturn) {
		return "", false
	}
	body = strings.TrimSpace(body)
	if body == "" || !loaderSpreadReturnPattern.MatchString(body) {
		return "", false
	}
	if async {
		return promiseTypeText("any[]"), true
	}
	return "any[]", true
}

func widenMappedOpaqueLoaderArrayReturnType(returnType string, async bool, body string) (string, bool) {
	mainReturn := normalizeSourceTypeText(unwrapPromiseType(returnType))
	if !isArrayLikeType(mainReturn) {
		return "", false
	}

	candidates := []string{}
	if sourceHasMultipleReturnPaths(body) {
		for _, candidate := range collectSourceReturnCandidates(body) {
			expression := strings.TrimSpace(candidate.Expression)
			if expression != "" {
				candidates = append(candidates, expression)
			}
		}
	}
	if len(candidates) == 0 {
		expression := strings.TrimSpace(body)
		if strings.HasPrefix(expression, "{") {
			expression = singleReturnExpression(expression)
			if expression == "" {
				expression = blockReturnExpression(body)
			}
		}
		expression = strings.TrimSpace(expression)
		if expression != "" {
			candidates = append(candidates, expression)
		}
	}

	for _, expression := range candidates {
		if !strings.Contains(expression, ".map(") {
			continue
		}
		for _, match := range loaderAwaitedJSONLocalPattern.FindAllStringSubmatch(body, -1) {
			if len(match) < 2 {
				continue
			}
			if !typeTextContainsStandaloneToken(expression, strings.TrimSpace(match[1])) {
				continue
			}
			if async {
				return promiseTypeText("any[]"), true
			}
			return "any[]", true
		}
	}
	return "", false
}

func loaderSingleReturnUsesAwaitedLocalValue(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" || sourceHasMultipleReturnPaths(body) {
		return false
	}

	expression := body
	if strings.HasPrefix(body, "{") {
		expression = singleReturnExpression(body)
		if expression == "" {
			expression = blockReturnExpression(body)
		}
	}
	expression = trimLeadingSourceTrivia(strings.TrimSpace(expression))
	if expression == "" {
		return false
	}
	if strings.HasPrefix(expression, "await ") {
		return true
	}

	identifier, ok := sourceIdentifierExpression(expression)
	if !ok {
		return false
	}
	initializer := trimLeadingSourceTrivia(findLocalValueInitializer(body, identifier))
	return strings.HasPrefix(strings.TrimSpace(initializer), "await ")
}

func loaderSingleReturnUsesAwaitedJSONValue(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" || sourceHasMultipleReturnPaths(body) {
		return false
	}

	expression := body
	if strings.HasPrefix(body, "{") {
		expression = singleReturnExpression(body)
		if expression == "" {
			expression = blockReturnExpression(body)
		}
	}
	expression = trimLeadingSourceTrivia(strings.TrimSpace(expression))
	if expression == "" {
		return false
	}
	if strings.HasPrefix(expression, "await ") {
		return loaderAwaitedJSONExpression(strings.TrimSpace(strings.TrimPrefix(expression, "await ")))
	}

	identifier, ok := sourceIdentifierExpression(expression)
	if !ok {
		return false
	}
	initializer := trimLeadingSourceTrivia(findLocalValueInitializer(body, identifier))
	initializer = strings.TrimSpace(initializer)
	if !strings.HasPrefix(initializer, "await ") {
		return false
	}
	return loaderAwaitedJSONExpression(strings.TrimSpace(strings.TrimPrefix(initializer, "await ")))
}

func loaderAwaitedJSONExpression(expression string) bool {
	expression = trimLeadingSourceTrivia(strings.TrimSpace(expression))
	if expression == "" {
		return false
	}
	return strings.Contains(expression, ".json(")
}

func preferDefaultLoaderArrayReturnType(returnType, defaultTypeHint string, async bool, body string) (string, bool) {
	mainReturn := normalizeSourceTypeText(unwrapPromiseType(returnType))
	defaultType := normalizeSourceTypeText(defaultTypeHint)
	if !isAnyArrayType(mainReturn) || !isArrayLikeType(defaultType) {
		return "", false
	}
	body = strings.TrimSpace(body)
	if body == "" || loaderSpreadReturnPattern.MatchString(body) {
		return "", false
	}
	if async {
		return promiseTypeText(defaultType), true
	}
	return defaultType, true
}

func loaderFunctionTypeUsesSpreadAnyArray(functionType, expression string) bool {
	if functionType == "" {
		return false
	}
	_, returnType, ok := splitFunctionType(functionType)
	if !ok || !isAnyArrayType(unwrapPromiseType(returnType)) {
		return false
	}
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return false
	}
	return loaderSpreadReturnPattern.MatchString(strings.TrimSpace(info.Body))
}

func loaderFunctionTypeUsesMappedOpaqueAnyArray(functionType, expression string) bool {
	if functionType == "" {
		return false
	}
	_, returnType, ok := splitFunctionType(functionType)
	if !ok || !isAnyArrayType(unwrapPromiseType(returnType)) {
		return false
	}
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return false
	}
	_, widened := widenMappedOpaqueLoaderArrayReturnType(returnType, info.Async, info.Body)
	return widened
}

func hasLoaderActionProperties(properties map[string]string) bool {
	for name := range properties {
		if name != "__default" {
			return true
		}
	}
	return false
}

func mergeSourceLoaderProperties(current, source map[string]string) map[string]string {
	if len(source) == 0 {
		return current
	}
	if len(current) == 0 {
		return source
	}

	merged := make(map[string]string, len(current)+len(source))
	for name, typeText := range current {
		merged[name] = typeText
	}
	for name, typeText := range source {
		currentType := strings.TrimSpace(merged[name])
		if currentType == "" {
			merged[name] = typeText
			continue
		}
		if mergedType := mergeSourceLoaderPropertyType(currentType, typeText); mergedType != "" && mergedType != currentType {
			merged[name] = mergedType
		}
	}
	return merged
}

func mergeStatelessOpaqueLoaderProperties(current, source map[string]string) map[string]string {
	if len(source) == 0 {
		return current
	}
	if len(current) == 0 {
		return source
	}

	merged := make(map[string]string, len(current))
	for name, typeText := range current {
		merged[name] = typeText
	}
	for name, typeText := range source {
		currentType := strings.TrimSpace(merged[name])
		if currentType == "" {
			continue
		}
		if mergedType := mergeStatelessOpaqueLoaderPropertyType(currentType, typeText); mergedType != "" && mergedType != currentType {
			merged[name] = mergedType
		}
	}
	return merged
}

func mergeStatelessOpaqueLoaderPropertyType(current, source string) string {
	current = strings.TrimSpace(current)
	source = strings.TrimSpace(source)
	if current == "" || source == "" || current == source {
		return current
	}

	currentParameters, currentReturn, currentOK := splitFunctionType(current)
	sourceParameters, sourceReturn, sourceOK := splitFunctionType(source)
	if !currentOK || !sourceOK {
		return current
	}
	if !isAnyArrayType(unwrapPromiseType(sourceReturn)) {
		return current
	}

	mergedParameters := currentParameters
	if sourceActionParametersArePreferred(sourceParameters) && !sourceActionParametersArePreferred(currentParameters) {
		mergedParameters = sourceParameters
	}
	if !shouldPreferSourceLoaderAnyArrayReturn(currentReturn, sourceReturn) {
		return current
	}
	return mergedParameters + " => " + sourceReturn
}

func mergeSourceLoaderPropertyType(current, source string) string {
	current = strings.TrimSpace(current)
	source = strings.TrimSpace(source)
	if current == "" {
		return source
	}
	if source == "" || current == source {
		return current
	}

	currentParameters, currentReturn, currentOK := splitFunctionType(current)
	sourceParameters, sourceReturn, sourceOK := splitFunctionType(source)
	if currentOK && sourceOK {
		mergedParameters := currentParameters
		if sourceActionParametersArePreferred(sourceParameters) && !sourceActionParametersArePreferred(currentParameters) {
			mergedParameters = sourceParameters
		}

		mergedReturn := currentReturn
		if shouldPreferSourceLoaderAnyArrayReturn(currentReturn, sourceReturn) {
			mergedReturn = sourceReturn
		} else if shouldPreserveReportedLoaderAnyArrayReturn(currentParameters, currentReturn, sourceParameters, sourceReturn) {
			mergedReturn = currentReturn
		} else if shouldPreferSourceLoaderPropertyReturnType(currentReturn, sourceReturn) {
			mergedReturn = sourceReturn
		}
		if mergedParameters != currentParameters || mergedReturn != currentReturn {
			return mergedParameters + " => " + mergedReturn
		}
	}

	if shouldPreferSourceLoaderPropertyType(current, source) {
		return source
	}
	return current
}

func shouldPreferSourceLoaderPropertyType(current, source string) bool {
	current = strings.TrimSpace(current)
	source = strings.TrimSpace(source)
	if source == "" || current == source {
		return false
	}
	if current == "" {
		return true
	}
	if typeTextNeedsSourceRecovery(current) && !typeTextNeedsSourceRecovery(source) {
		return true
	}
	currentParameters, currentReturn, currentOK := splitFunctionType(current)
	sourceParameters, sourceReturn, sourceOK := splitFunctionType(source)
	if !currentOK || !sourceOK {
		return false
	}
	if currentParameters != sourceParameters && sourceActionParametersArePreferred(sourceParameters) && !sourceActionParametersArePreferred(currentParameters) {
		return true
	}

	return shouldPreferSourceLoaderPropertyReturnType(currentReturn, sourceReturn)
}

func shouldPreferSourceLoaderPropertyReturnType(current, source string) bool {
	current = strings.TrimSpace(current)
	source = strings.TrimSpace(source)
	if source == "" || current == source {
		return false
	}
	if current == "" {
		return true
	}
	if typeTextNeedsSourceRecovery(current) && !typeTextNeedsSourceRecovery(source) {
		return true
	}
	if sourceBroadensPureNullishLoaderReturn(current, source) {
		return true
	}

	currentMain, currentNullish, ok := returnTypeMainType(current)
	if !ok {
		return false
	}
	sourceMain, sourceNullish, ok := returnTypeMainType(source)
	if !ok || currentMain != sourceMain {
		return false
	}
	return sourceNullish == "" && currentNullish != ""
}

func sourceBroadensPureNullishLoaderReturn(current, source string) bool {
	current = normalizeSourceTypeText(strings.TrimSpace(unwrapPromiseType(current)))
	source = normalizeSourceTypeText(strings.TrimSpace(unwrapPromiseType(source)))
	if current == "" || source == "" || current == source {
		return false
	}

	currentParts, err := splitTopLevelUnion(current)
	if err != nil {
		currentParts = []string{current}
	}

	requiredNullish := map[string]bool{}
	for _, part := range currentParts {
		part = strings.TrimSpace(part)
		switch part {
		case "null", "undefined":
			requiredNullish[part] = true
		default:
			return false
		}
	}
	if len(requiredNullish) == 0 {
		return false
	}

	sourceParts, err := splitTopLevelUnion(source)
	if err != nil {
		sourceParts = []string{source}
	}

	remainingNullish := map[string]bool{}
	for part := range requiredNullish {
		remainingNullish[part] = true
	}
	sawMain := false
	for _, part := range sourceParts {
		part = strings.TrimSpace(part)
		switch part {
		case "", "null", "undefined":
			delete(remainingNullish, part)
		default:
			sawMain = true
		}
	}
	return sawMain && len(remainingNullish) == 0
}

func shouldPreserveReportedLoaderAnyArrayReturn(currentParameters, currentReturn, sourceParameters, sourceReturn string) bool {
	currentReturn = normalizeSourceTypeText(unwrapPromiseType(currentReturn))
	sourceReturn = normalizeSourceTypeText(unwrapPromiseType(sourceReturn))
	if currentReturn == "" || sourceReturn == "" || currentReturn == sourceReturn {
		return false
	}
	if !isAnyArrayType(currentReturn) || !isArrayLikeType(sourceReturn) || isAnyArrayType(sourceReturn) {
		return false
	}
	return loaderParametersContainRecoverableAnyObject(currentParameters, sourceParameters)
}

func loaderParametersContainRecoverableAnyObject(currentParameters, sourceParameters string) bool {
	currentMembers, ok := parseObjectTypeMembers(firstParameterType(currentParameters))
	if !ok || len(currentMembers) < 2 {
		return false
	}
	sourceMembers, ok := parseObjectTypeMembers(firstParameterType(sourceParameters))
	if !ok || len(sourceMembers) != len(currentMembers) {
		return false
	}

	improved := false
	for name, currentType := range currentMembers {
		sourceType, ok := sourceMembers[name]
		if !ok {
			return false
		}
		currentType = normalizeSourceTypeText(currentType)
		sourceType = normalizeSourceTypeText(sourceType)
		if currentType == sourceType {
			continue
		}
		if !typeTextNeedsSourceRecovery(currentType) || typeTextNeedsSourceRecovery(sourceType) {
			return false
		}
		improved = true
	}
	return improved
}

func isAnyArrayType(typeText string) bool {
	switch normalizeSourceTypeText(strings.TrimSpace(typeText)) {
	case "any[]", "Array<any>", "ReadonlyArray<any>":
		return true
	default:
		return false
	}
}

func isArrayLikeType(typeText string) bool {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	return strings.HasSuffix(text, "[]") ||
		(strings.HasPrefix(text, "Array<") && strings.HasSuffix(text, ">")) ||
		(strings.HasPrefix(text, "ReadonlyArray<") && strings.HasSuffix(text, ">"))
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
	if shouldFallbackToLoaderDefaultType(resolved, fallback) {
		return fallback
	}
	return resolved
}

func shouldFallbackToLoaderDefaultType(resolved, fallback string) bool {
	if fallback == "" {
		return false
	}
	switch resolved {
	case "", "any", "unknown", "Promise", "PromiseConstructor":
		return true
	}
	if isArrayLikeType(fallback) && !isArrayLikeType(resolved) {
		return true
	}
	if isBroadScalarPrimitiveType(resolved) && !isPrimitiveLikeUnionType(fallback) {
		return true
	}
	if widenLiteralReducerStateType(resolved) == fallback {
		return true
	}
	return isQuotedString(resolved) && !isQuotedString(fallback)
}

func shouldPreferSourceLoaderAnyArrayReturn(currentReturn, sourceReturn string) bool {
	currentReturn = normalizeSourceTypeText(unwrapPromiseType(currentReturn))
	sourceReturn = normalizeSourceTypeText(unwrapPromiseType(sourceReturn))
	if currentReturn == "" || sourceReturn == "" || currentReturn == sourceReturn {
		return currentReturn == "" && isAnyArrayType(sourceReturn)
	}
	if !isAnyArrayType(sourceReturn) {
		return false
	}
	if typeTextNeedsSourceRecovery(currentReturn) {
		return true
	}
	if strings.Contains(currentReturn, "...") {
		return true
	}
	return isArrayLikeType(currentReturn) && !isAnyArrayType(currentReturn)
}

func inferredLoaderDefaultType(currentDefault string, properties map[string]string) string {
	currentDefault = normalizeInferredTypeText(strings.TrimSpace(currentDefault))
	if len(properties) == 0 || !loaderDefaultTypeNeedsRecovery(currentDefault) {
		return ""
	}

	var candidate string
	for name, propertyType := range properties {
		if name == "__default" {
			continue
		}
		_, returnType, ok := splitFunctionType(propertyType)
		if !ok {
			continue
		}
		resolved := normalizeInferredTypeText(strings.TrimSpace(preferredLoaderSuccessType(returnType, currentDefault)))
		if resolved == "" || loaderDefaultTypeNeedsRecovery(resolved) {
			continue
		}
		if candidate == "" {
			candidate = resolved
			continue
		}
		if candidate != resolved {
			return ""
		}
	}
	if candidate == "" {
		return ""
	}
	return candidate
}

func loaderDefaultTypeNeedsRecovery(typeText string) bool {
	typeText = normalizeInferredTypeText(strings.TrimSpace(typeText))
	switch typeText {
	case "", "any", "unknown", "{}", "{ }", "any[]", "never[]", "Array<any>", "Array<never>", "ReadonlyArray<any>", "ReadonlyArray<never>":
		return true
	default:
		return false
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
	sourceMembers := canonicalizeSourceProperties(source, file, sectionSourceProperties(source, property), state)
	sourceEntries := canonicalizeSourceObjectEntries(source, file, sectionSourceEntries(source, property), state)
	entryByName := sourceEntriesByName(sourceEntries)
	memberByName := map[string]MemberReport{}
	orderedNames := make([]string, 0, len(section.Members)+len(sourceEntries))
	seenNames := map[string]bool{}
	for _, entry := range sourceEntries {
		if !seenNames[entry.Name] {
			seenNames[entry.Name] = true
			orderedNames = append(orderedNames, entry.Name)
		}
	}
	for _, name := range orderedSourcePropertyNames(sourceMembers) {
		if !seenNames[name] {
			seenNames[name] = true
			orderedNames = append(orderedNames, name)
		}
	}
	for _, member := range section.Members {
		member.Name = canonicalSourceObjectMemberName(source, file, member.Name, state)
		memberByName[member.Name] = member
		if !seenNames[member.Name] {
			seenNames[member.Name] = true
			orderedNames = append(orderedNames, member.Name)
		}
	}

	selectors := make([]ParsedField, 0, len(orderedNames))
	contextLogic := logic
	for _, name := range orderedNames {
		member, hasMember := memberByName[name]
		selectorExpression := ""
		if nested, ok := sourceMembers[name]; ok {
			selectorExpression = sourcePropertyText(source, nested)
		} else if entry, ok := entryByName[name]; ok {
			selectorExpression = entry.Value
		}
		allowSourceRecovery := selectorSourceRecoveryAllowed(contextLogic, selectorExpression)
		selectorType := ""
		hasExplicitSourceReturn := false
		explicitSourceReturnType := ""
		parsedMemberReturnType := ""
		rawParsedMemberReturnType := ""
		preferredMemberReturnType := ""
		if hasMember {
			if returnType, ok := parseSelectorReturnType(member.TypeString); ok {
				rawParsedMemberReturnType = returnType
				if !isAnyLikeType(returnType) {
					parsedMemberReturnType = returnType
				}
			}
			preferredMemberReturnType = strings.TrimSpace(preferredMemberReturnTypeText(member))
			if selectorExpression != "" || source != "" {
				preferredMemberReturnType = selectorReportedMemberReturnTypeText(member)
			}
			if selectorReturnMatchesSectionType(section, preferredMemberReturnType, selectorExpression) {
				preferredMemberReturnType = ""
			}
			if selectorTypeIsBareLiteral(preferredMemberReturnType) && isAnyLikeType(rawParsedMemberReturnType) {
				preferredMemberReturnType = rawParsedMemberReturnType
			} else if selectorTypeIsBareLiteral(preferredMemberReturnType) && selectorMemberHasLooseReportedReturn(member.TypeString) {
				preferredMemberReturnType = "any"
			}
		}
		reportedSelectorType := resolveReportedSelectorType(preferredMemberReturnType, parsedMemberReturnType)
		keepParityReportedSelectorType := hasMember && parityModeKeepsLooseReportedSelectorType(logic, source, file, seenNames, selectorExpression, reportedSelectorType, member.TypeString, state)
		if !keepParityReportedSelectorType && hasMember && parityModeKeepsLooseContextualProjectorReportedSelectorType(selectorExpression, reportedSelectorType, member) {
			keepParityReportedSelectorType = true
		}
		keepParityTupleProjectorSurfaceLoose := hasMember && parityModeTupleProjectorSelectorSurfaceShouldStayLoose(contextLogic, source, file, selectorExpression, member, parsedMemberReturnType, state)
		if keepParityTupleProjectorSurfaceLoose {
			keepParityReportedSelectorType = true
			reportedSelectorType = "any"
			parsedMemberReturnType = ""
		}
		reportedIsConstructor := strings.HasSuffix(normalizeInferredTypeText(strings.TrimSpace(reportedSelectorType)), "Constructor")
		allowConstructorRecovery := !reportedIsConstructor || selectorSourceHasRecoverableConstructor(selectorExpression)
		if logic.InputKind == "builders" &&
			hasMember &&
			selectorExpression == "" &&
			selectorMemberLacksReportedSurface(member, preferredMemberReturnType) {
			continue
		}
		if selectorExpression != "" {
			if explicitReturn := sourceSelectorReturnType(selectorExpression); explicitReturn != "" {
				explicitSourceReturnType = explicitReturn
				selectorType = explicitReturn
				hasExplicitSourceReturn = true
			}
		}
		if hasExplicitSourceReturn && selectorFunctionTypePreservesMoreOptionalUndefined(selectorType, reportedSelectorType) {
			selectorType = reportedSelectorType
		}
		if selectorType == "" && !keepParityReportedSelectorType && allowSourceRecovery && hasMember && selectorExpression != "" &&
			selectorReportedTypeNeedsSourceRecovery(reportedSelectorType, member.TypeString) {
			if functionType := sourceInternalSelectorFunctionType(contextLogic, source, file, selectorExpression, state); functionType != "" {
				if _, returnType, ok := splitFunctionType(functionType); ok {
					selectorType = normalizePublicRecoveredSelectorType(contextLogic, source, file, selectorExpression, reportedSelectorType, returnType, state)
				}
			}
		}
		if selectorType == "" && hasMember && reportedIsConstructor {
			selectorType = reportedSelectorType
		}
		if nested, ok := sourceMembers[name]; ok && !keepParityReportedSelectorType && allowConstructorRecovery {
			if probed := normalizePublicRecoveredSelectorType(contextLogic, source, file, selectorExpression, reportedSelectorType, sourceSelectorReturnTypeFromTypeProbe(source, file, nested, state), state); selectorFunctionTypePreservesMoreOptionalUndefined(selectorType, probed) && !selectorReturnMatchesSectionType(section, probed, selectorExpression) {
				selectorType = normalizeRecoveredSelectorType(source, file, probed)
			}
		}
		if selectorType == "" && hasMember && reportedSelectorType != "" && !selectorTypeLooksLessInformative(reportedSelectorType) {
			selectorType = reportedSelectorType
		}
		if selectorType == "" && parsedMemberReturnType != "" {
			selectorType = parsedMemberReturnType
		}
		if !hasExplicitSourceReturn && source != "" && !keepParityReportedSelectorType && allowConstructorRecovery && allowSourceRecovery {
			if nested, ok := sourceMembers[name]; ok {
				if selectorExpression != "" && selectorTypeNeedsSourceRecoveryFromExpression(selectorType, selectorExpression) {
					if inferred := normalizePublicRecoveredSelectorType(contextLogic, source, file, selectorExpression, reportedSelectorType, sourceSelectorInferredType(contextLogic, source, file, selectorExpression, state), state); shouldRecoverSelectorTypeFromSource(contextLogic, source, file, selectorType, inferred, selectorExpression, state) {
						selectorType = inferred
					}
					if selectorTypeNeedsSourceRecovery(selectorType) {
						if probed := sourceSelectorReturnTypeFromTypeProbe(source, file, nested, state); probed != "" {
							normalized := normalizePublicRecoveredSelectorType(contextLogic, source, file, selectorExpression, reportedSelectorType, probed, state)
							if !selectorReturnMatchesSectionType(section, normalized, selectorExpression) &&
								shouldRecoverSelectorTypeFromSource(contextLogic, source, file, selectorType, normalized, selectorExpression, state) {
								selectorType = normalized
							}
						}
					}
				}
				if selectorExpression != "" && selectorSourceHasMultipleReturnPaths(selectorExpression) && sourceSelectorReturnsFunction(selectorExpression) {
					if inferred := normalizePublicRecoveredSelectorType(contextLogic, source, file, selectorExpression, reportedSelectorType, sourceSelectorInferredType(contextLogic, source, file, selectorExpression, state), state); shouldPreferRecoveredMultiReturnFunctionSelector(selectorType, inferred) {
						selectorType = inferred
					}
				}
				if selectorExpression != "" && selectorSourceHasMultipleReturnPaths(selectorExpression) {
					if probed := sourceSelectorReturnTypeFromTypeProbe(source, file, nested, state); probed != "" {
						normalized := normalizePublicRecoveredSelectorType(contextLogic, source, file, selectorExpression, reportedSelectorType, probed, state)
						if !selectorReturnMatchesSectionType(section, normalized, selectorExpression) &&
							shouldPreferProbedSelectorTypeForMultipleReturns(selectorType, normalized) {
							selectorType = normalized
						}
					}
				}
			} else if selectorExpression != "" && selectorTypeNeedsSourceRecoveryFromExpression(selectorType, selectorExpression) {
				if inferred := normalizePublicRecoveredSelectorType(contextLogic, source, file, selectorExpression, reportedSelectorType, sourceSelectorInferredType(contextLogic, source, file, selectorExpression, state), state); shouldRecoverSelectorTypeFromSource(contextLogic, source, file, selectorType, inferred, selectorExpression, state) {
					selectorType = inferred
				}
			}
		}
		if source != "" && selectorExpression != "" && !keepParityReportedSelectorType && allowConstructorRecovery && allowSourceRecovery {
			if functionType := sourceInternalSelectorFunctionType(contextLogic, source, file, selectorExpression, state); functionType != "" {
				if _, returnType, ok := splitFunctionType(functionType); ok {
					recovered := normalizePublicRecoveredSelectorType(contextLogic, source, file, selectorExpression, reportedSelectorType, returnType, state)
					switch {
					case recovered == "":
					case selectorType == "":
						selectorType = recovered
					case selectorTypeNeedsSourceRecovery(selectorType):
						selectorType = recovered
					case isAnyLikeType(selectorType) || isLooselyTypedType(selectorType):
						selectorType = recovered
					case selectorRecoveredBooleanShouldBeatCurrent(selectorExpression, selectorType, recovered):
						selectorType = recovered
					case selectorReportedTypeShouldYieldToParsed(selectorType, recovered):
						selectorType = recovered
					case shouldPreferInferredSelectorType(source, file, selectorType, recovered):
						selectorType = recovered
					case shouldPreferStrongRecoveredSelectorType(selectorType, recovered):
						selectorType = recovered
					}
				}
			}
		}
		if source != "" && selectorExpression != "" && !keepParityReportedSelectorType && allowConstructorRecovery && allowSourceRecovery {
			if inferred := normalizePublicRecoveredSelectorType(contextLogic, source, file, selectorExpression, reportedSelectorType, sourceSelectorInferredType(contextLogic, source, file, selectorExpression, state), state); shouldPreferStrongRecoveredSelectorType(selectorType, inferred) || selectorRecoveredBooleanShouldBeatCurrent(selectorExpression, selectorType, inferred) {
				selectorType = inferred
			}
		}
		if !hasExplicitSourceReturn &&
			hasMember &&
			reportedSelectorType != "" &&
			selectorRecoveredLiteralShouldYieldToReported(selectorType, reportedSelectorType) {
			selectorType = reportedSelectorType
		}
		if !hasExplicitSourceReturn &&
			hasMember &&
			reportedSelectorType != "" &&
			!selectorTypeLooksLessInformative(reportedSelectorType) &&
			!selectorRecoveredBooleanShouldBeatCurrent(selectorExpression, reportedSelectorType, selectorType) &&
			shouldPreferReportedSelectorType(selectorType, reportedSelectorType) {
			selectorType = reportedSelectorType
		}
		if source != "" && selectorExpression != "" && !keepParityReportedSelectorType && selectorTypeNeedsSourceRecovery(selectorType) && allowConstructorRecovery && allowSourceRecovery {
			if inferred := normalizePublicRecoveredSelectorType(contextLogic, source, file, selectorExpression, reportedSelectorType, sourceSelectorInferredType(contextLogic, source, file, selectorExpression, state), state); inferred != "" &&
				shouldRecoverSelectorTypeFromSource(contextLogic, source, file, selectorType, inferred, selectorExpression, state) {
				selectorType = inferred
			}
		}
		if selectorType == "" && hasMember {
			if preferredMemberReturnType != "" && !strings.Contains(preferredMemberReturnType, "...") {
				selectorType = preferredMemberReturnType
			}
		}
		if selectorType == "" && hasMember && !keepParityTupleProjectorSurfaceLoose {
			if returnType, ok := parseSelectorReturnType(member.TypeString); ok {
				selectorType = returnType
			}
		}
		if selectorType == "" && hasMember {
			selectorType = "any"
		}
		if source != "" && selectorExpression != "" && !keepParityReportedSelectorType && selectorTypeNeedsSourceRecovery(selectorType) && allowConstructorRecovery && allowSourceRecovery {
			if inferred := normalizePublicRecoveredSelectorType(contextLogic, source, file, selectorExpression, reportedSelectorType, sourceSelectorInferredType(contextLogic, source, file, selectorExpression, state), state); inferred != "" &&
				shouldRecoverSelectorTypeFromSource(contextLogic, source, file, selectorType, inferred, selectorExpression, state) {
				selectorType = inferred
			}
		}
		if source != "" && selectorExpression != "" && !keepParityReportedSelectorType && selectorSourceHasMultipleReturnPaths(selectorExpression) && sourceSelectorReturnsFunction(selectorExpression) {
			if inferred := normalizePublicRecoveredSelectorType(contextLogic, source, file, selectorExpression, reportedSelectorType, sourceSelectorInferredType(contextLogic, source, file, selectorExpression, state), state); shouldPreferRecoveredMultiReturnFunctionSelector(selectorType, inferred) {
				selectorType = inferred
			}
		}
		if source != "" && selectorExpression != "" && parityModeSelectorPropertyFallbackShouldStayAny(contextLogic, source, file, selectorExpression, selectorType, state) {
			selectorType = "any"
		}
		if hasExplicitSourceReturn && explicitSourceReturnType != "" {
			selectorType = explicitSourceReturnType
			if selectorFunctionTypePreservesMoreOptionalUndefined(explicitSourceReturnType, reportedSelectorType) {
				selectorType = reportedSelectorType
			}
		}
		if selectorType == "" {
			continue
		}
		if logic.InputKind == "builders" &&
			selectorExpression != "" &&
			!selectorIdentityConcreteDependencyTypeShouldBePreserved(contextLogic, source, file, selectorExpression, selectorType, state) &&
			selectorIdentityPrimitivePropsReturnShouldStayAny(contextLogic, source, file, selectorExpression, selectorType, state) {
			selectorType = "any"
		}
		if selectorExpression == "" {
			if existing, ok := findParsedField(contextLogic.Selectors, name); ok && shouldPreferExistingParsedSelectorType(existing.Type, selectorType) {
				selectorType = existing.Type
			}
		}
		if parityModeReportedStructuredTypeShouldStayLoose(reportedSelectorType, selectorType) ||
			parityModeReportedStructuredTypeShouldBeatOpaqueCandidate(reportedSelectorType, selectorType) {
			selectorType = reportedSelectorType
		}
		selectorType = wrapSelectorFunctionType(normalizeSelectorFunctionTypeOptionalUndefined(selectorType))
		field := ParsedField{Name: name, Type: selectorType}
		selectors = append(selectors, field)
		contextLogic.Selectors = mergeParsedFields(contextLogic.Selectors, field)
	}
	return selectors
}

func selectorReturnMatchesSectionType(section SectionReport, candidate, expression string) bool {
	candidate = normalizeSourceTypeTextWithOptions(strings.TrimSpace(candidate), false)
	if candidate == "" {
		return false
	}
	for _, text := range []string{
		strings.TrimSpace(section.EffectiveTypeString),
		strings.TrimSpace(section.PrintedTypeNode),
		strings.TrimSpace(section.RawTypeString),
	} {
		text = normalizeSourceTypeTextWithOptions(text, false)
		if text != "" && text == candidate {
			return true
		}
	}
	return false
}

func parseInternalSelectorTypesWithSource(
	section SectionReport,
	logic ParsedLogic,
	source string,
	property SourceProperty,
	file string,
	state *buildState,
) []ParsedFunction {
	sourceMembers := canonicalizeSourceProperties(source, file, sectionSourceProperties(source, property), state)
	sourceEntries := canonicalizeSourceObjectEntries(source, file, sectionSourceEntries(source, property), state)
	entryByName := sourceEntriesByName(sourceEntries)
	memberByName := map[string]MemberReport{}
	orderedNames := make([]string, 0, len(section.Members)+len(sourceEntries))
	seenNames := map[string]bool{}
	for _, entry := range sourceEntries {
		if !seenNames[entry.Name] {
			seenNames[entry.Name] = true
			orderedNames = append(orderedNames, entry.Name)
		}
	}
	for _, name := range orderedSourcePropertyNames(sourceMembers) {
		if !seenNames[name] {
			seenNames[name] = true
			orderedNames = append(orderedNames, name)
		}
	}
	for _, member := range section.Members {
		member.Name = canonicalSourceObjectMemberName(source, file, member.Name, state)
		memberByName[member.Name] = member
		if !seenNames[member.Name] {
			seenNames[member.Name] = true
			orderedNames = append(orderedNames, member.Name)
		}
	}

	helpers := make([]ParsedFunction, 0, len(orderedNames))
	allowSourceOnlyBuilderInternalHelpers := logic.InputKind == "builders" && hasMissingLocalLogicTypeImport(source, file)
	allowOpaqueCurrentRefinement := hasMissingLocalLogicTypeImport(source, file)
	contextLogic := logic
	for _, name := range orderedNames {
		member, hasMember := memberByName[name]
		selectorExpression := ""
		if nested, ok := sourceMembers[name]; ok {
			selectorExpression = sourcePropertyText(source, nested)
		} else if entry, ok := entryByName[name]; ok {
			selectorExpression = entry.Value
		}
		if logic.InputKind == "builders" && !allowSourceOnlyBuilderInternalHelpers {
			if !hasMember || !selectorMemberSupportsInternalHelper(member) {
				continue
			}
		}
		functionType := ""
		if hasMember {
			functionType = selectorFunctionTypeFromMember(member)
		}
		reportedFunctionType := functionType
		keepParityReportedSelectorType := false
		if hasMember {
			if field, ok := findParsedField(logic.Selectors, name); ok {
				keepParityReportedSelectorType = parityModeKeepsLooseReportedSelectorType(logic, source, file, seenNames, selectorExpression, field.Type, member.TypeString, state)
				if !keepParityReportedSelectorType && parityModeKeepsLooseContextualProjectorReportedSelectorType(selectorExpression, field.Type, member) {
					keepParityReportedSelectorType = true
				}
				if !keepParityReportedSelectorType &&
					parityModeEnabled() &&
					selectorMemberHasLooseReportedReturn(member.TypeString) &&
					(isAnyLikeType(field.Type) || isLooselyTypedType(field.Type)) {
					keepParityReportedSelectorType = true
				}
			}
		}
		sourceSupportsInternalHelper := false
		if nested, ok := sourceMembers[name]; ok {
			expression := sourcePropertyText(source, nested)
			fallbackReturnType := normalizeInferredTypeText(strings.TrimSpace(member.ReturnTypeString))
			if parityModeEnabled() {
				if field, ok := findParsedField(contextLogic.Selectors, name); ok {
					fieldType := normalizeInferredTypeText(strings.TrimSpace(field.Type))
					if fieldType != "" && (isAnyLikeType(fieldType) || isLooselyTypedType(fieldType)) {
						fallbackReturnType = fieldType
					}
				}
			}
			if _, currentReturnType, ok := splitFunctionType(functionType); ok {
				fallbackReturnType = currentReturnType
			} else if fallbackReturnType == "" {
				if field, ok := findParsedField(contextLogic.Selectors, name); ok {
					fallbackReturnType = normalizeInferredTypeText(strings.TrimSpace(field.Type))
				}
			}
			if recovered := sourceInternalSelectorFunctionTypeWithFallbackReturn(contextLogic, source, file, expression, fallbackReturnType, state); recovered != "" {
				sourceSupportsInternalHelper = true
				recovered = parityModeRecoveredInternalHelperFunctionType(selectorExpression, functionType, recovered, keepParityReportedSelectorType)
				if shouldPreferRecoveredInternalHelperFunctionType(functionType, recovered) ||
					parityModeUnnamedDependencyPlaceholderHelperShouldPreferRecovered(selectorExpression, functionType, recovered) ||
					helperBooleanRecoveryShouldPreferRecovered(selectorExpression, functionType, recovered) ||
					parityModeExplicitlyTypedUnnamedDependencyHelperShouldPreferRecovered(selectorExpression, functionType, recovered) ||
					parityModeOpaqueNullableLookupHelperShouldPreferRecovered(contextLogic, source, file, selectorExpression, functionType, recovered, state) {
					functionType = recovered
				}
			}
			if probed := sourceSelectorProjectorFunctionTypeFromTypeProbe(contextLogic, source, file, nested, state); probed != "" {
				sourceSupportsInternalHelper = true
				probed = parityModeRecoveredInternalHelperFunctionType(selectorExpression, functionType, probed, keepParityReportedSelectorType)
				if shouldPreferRecoveredInternalHelperFunctionType(functionType, probed) ||
					parityModeUnnamedDependencyPlaceholderHelperShouldPreferRecovered(selectorExpression, functionType, probed) ||
					helperBooleanRecoveryShouldPreferRecovered(selectorExpression, functionType, probed) ||
					parityModeExplicitlyTypedUnnamedDependencyHelperShouldPreferRecovered(selectorExpression, functionType, probed) ||
					parityModeOpaqueNullableLookupHelperShouldPreferRecovered(contextLogic, source, file, selectorExpression, functionType, probed, state) {
					functionType = probed
				}
			}
		} else if entry, ok := entryByName[name]; ok {
			if recovered := sourceInternalSelectorFunctionType(contextLogic, source, file, entry.Value, state); recovered != "" {
				recovered = parityModeRecoveredInternalHelperFunctionType(selectorExpression, functionType, recovered, keepParityReportedSelectorType)
				if shouldPreferRecoveredInternalHelperFunctionType(functionType, recovered) ||
					parityModeUnnamedDependencyPlaceholderHelperShouldPreferRecovered(selectorExpression, functionType, recovered) ||
					helperBooleanRecoveryShouldPreferRecovered(selectorExpression, functionType, recovered) ||
					parityModeExplicitlyTypedUnnamedDependencyHelperShouldPreferRecovered(selectorExpression, functionType, recovered) ||
					parityModeOpaqueNullableLookupHelperShouldPreferRecovered(contextLogic, source, file, selectorExpression, functionType, recovered, state) {
					functionType = recovered
				}
				sourceSupportsInternalHelper = true
			}
		}
		if logic.InputKind == "builders" && !sourceSupportsInternalHelper {
			if !hasMember || !selectorMemberSupportsInternalHelper(member) {
				continue
			}
		}
		functionType = canonicalizeInternalHelperFunctionTypeWithSelectorDependencies(contextLogic, source, file, selectorExpression, functionType, state)
		if parityModeReportedStructuredHelperFunctionTypeShouldStayReported(reportedFunctionType, functionType) {
			functionType = reportedFunctionType
		}
		parameters, _, ok := splitFunctionType(functionType)
		if !ok {
			continue
		}
		params, ok := parseFunctionParameters(parameters)
		if !ok || len(params) == 0 {
			continue
		}
		if parityModeObjectFromEntriesBooleanMapHelperShouldBeOmitted(selectorExpression, functionType) {
			continue
		}
		if field, ok := findParsedField(logic.Selectors, name); ok && selectorPublicTypeSuppressesInternalHelper(field.Type) {
			continue
		}
		if logic.InputKind == "builders" && internalSelectorFunctionTypeIsUninformative(functionType, params) {
			keepParityPropsIdentityHelper := false
			keepParityReportedLooseHelper := false
			if parityModeEnabled() {
				if hasMember {
					if field, ok := findParsedField(contextLogic.Selectors, name); ok &&
						selectorMemberHasLooseReportedReturn(member.TypeString) &&
						(isAnyLikeType(field.Type) || isLooselyTypedType(field.Type)) {
						keepParityReportedLooseHelper = true
					}
				}
				if selectorIdentityPropsDependency(selectorExpression) {
					keepParityPropsIdentityHelper = true
				}
			}
			if !keepParityReportedLooseHelper && !keepParityPropsIdentityHelper {
				continue
			}
		}
		if field, ok := findParsedField(contextLogic.Selectors, name); ok && selectorInternalHelperShouldStayAny(contextLogic, source, file, selectorExpression, field.Type, functionType, state) {
			functionType = opaqueInternalHelperFunctionType(functionType)
		}
		helper := ParsedFunction{Name: name, FunctionType: functionType}
		helpers = append(helpers, helper)
		contextLogic.Selectors = refineSelectorTypesFromInternalHelpers(contextLogic.Selectors, []ParsedFunction{helper}, allowOpaqueCurrentRefinement)
	}
	return helpers
}

func canonicalizeInternalHelperFunctionTypeWithSelectorDependencies(logic ParsedLogic, source, file, expression, functionType string, state *buildState) string {
	functionType = normalizeSelectorFunctionTypeOptionalUndefined(normalizeSourceTypeText(strings.TrimSpace(functionType)))
	if functionType == "" || strings.TrimSpace(expression) == "" {
		return functionType
	}

	parameters, returnType, ok := splitFunctionType(functionType)
	if !ok {
		return functionType
	}
	params, ok := parseFunctionParameters(parameters)
	if !ok || len(params) == 0 {
		return functionType
	}

	dependencyNames := sourceSelectorDependencyNamesWithPlaceholders(expression)
	if len(dependencyNames) != len(params) {
		return functionType
	}
	knownFields := mergeParsedFields(logic.Reducers, logic.Selectors...)
	for _, name := range dependencyNames {
		name = strings.TrimSpace(name)
		if name == "" {
			return functionType
		}
		if _, ok := findParsedField(knownFields, name); !ok {
			return functionType
		}
	}
	dependencyNames = internalHelperParameterNames(dependencyNames, nil)

	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	rebuilt := make([]string, 0, len(params))
	for index, param := range params {
		paramType := normalizeInferredTypeText(strings.TrimSpace(param.Type))
		if index < len(dependencyTypes) {
			if dependencyType := normalizeInternalHelperSignatureParameterType(dependencyTypes[index], true); dependencyType != "" &&
				(paramType == "" || isAnyLikeType(paramType) || isLooselyTypedType(paramType)) {
				paramType = dependencyType
			}
		}
		if paramType == "" {
			return functionType
		}
		rebuilt = append(rebuilt, fmt.Sprintf("%s: %s", dependencyNames[index], paramType))
	}
	return "(" + strings.Join(rebuilt, ", ") + ") => " + normalizeSelectorOptionalUndefinedTypeText(strings.TrimSpace(returnType))
}

func selectorInternalHelperShouldStayAny(logic ParsedLogic, source, file, expression, selectorType, functionType string, state *buildState) bool {
	if logic.InputKind != "builders" || !isAnyLikeType(selectorType) {
		return false
	}
	_, returnType, ok := splitFunctionType(functionType)
	if !ok {
		return false
	}
	if parityModeEnabled() && selectorIdentityPropsDependency(expression) && isAnyLikeSelectorHelperType(returnType) {
		return true
	}
	return selectorIdentityPrimitivePropsReturnShouldStayAny(logic, source, file, expression, normalizeInferredTypeText(strings.TrimSpace(returnType)), state)
}

func opaqueInternalHelperFunctionType(functionType string) string {
	parameters, _, ok := splitFunctionType(functionType)
	if !ok {
		return functionType
	}
	params, ok := parseFunctionParameters(parameters)
	if !ok || len(params) == 0 {
		return functionType
	}

	opaque := make([]string, 0, len(params))
	for index, param := range params {
		rawName, _, ok := splitTopLevelPropertyRaw(param.Text)
		if !ok {
			return functionType
		}
		name := strings.TrimSpace(rawName)
		if name == "" || parityModeEnabled() {
			name = opaqueInternalHelperParameterName(index)
		}
		opaque = append(opaque, name+": any")
	}
	return "(" + strings.Join(opaque, ", ") + ") => any"
}

func opaqueInternalHelperParameterName(index int) string {
	if index <= 0 {
		return "arg"
	}
	return fmt.Sprintf("arg%d", index+1)
}

func parityModeRecoveredInternalHelperFunctionType(expression, current, recovered string, keepReportedLoose bool) string {
	recovered = parityModeRecoveredInternalHelperParameterTypes(expression, current, recovered)
	if !keepReportedLoose {
		return recovered
	}
	_, currentReturnType, ok := splitFunctionType(current)
	if !ok {
		return recovered
	}
	recoveredParameters, recoveredReturnType, ok := splitFunctionType(recovered)
	if !ok {
		return recovered
	}
	currentReturnType = normalizeInferredTypeText(strings.TrimSpace(currentReturnType))
	if currentReturnType == "" || (!isAnyLikeType(currentReturnType) && !isLooselyTypedType(currentReturnType)) {
		return recovered
	}
	recoveredReturnType = normalizeInferredTypeText(strings.TrimSpace(recoveredReturnType))
	if isBroadPrimitiveSelectorType(recoveredReturnType) ||
		isPrimitiveLikeUnionType(recoveredReturnType) ||
		selectorTypeIsLiteralUnionOfKind(recoveredReturnType, "string") ||
		selectorTypeIsLiteralUnionOfKind(recoveredReturnType, "number") ||
		selectorTypeIsLiteralUnionOfKind(recoveredReturnType, "boolean") {
		return recovered
	}
	return recoveredParameters + " => " + currentReturnType
}

func parityModeRecoveredInternalHelperParameterTypes(expression, current, recovered string) string {
	if !parityModeEnabled() {
		return recovered
	}

	currentParameters, _, ok := splitFunctionType(current)
	if !ok {
		return recovered
	}
	recoveredParameters, recoveredReturnType, ok := splitFunctionType(recovered)
	if !ok {
		return recovered
	}

	currentParams, ok := parseFunctionParameters(currentParameters)
	if !ok {
		return recovered
	}
	recoveredParams, ok := parseFunctionParameters(recoveredParameters)
	if !ok || len(currentParams) != len(recoveredParams) {
		return recovered
	}

	dependencyNames := sourceSelectorDependencyNames(expression)
	if len(dependencyNames) != len(recoveredParams) {
		return recovered
	}
	dependencyExpressions := sourceSelectorDependencyPartExpressions(expression)

	rebuilt := make([]string, 0, len(recoveredParams))
	changed := false
	for index, param := range recoveredParams {
		rawName, _, ok := splitTopLevelPropertyRaw(param.Text)
		if !ok {
			return recovered
		}

		paramType := normalizeInferredTypeText(strings.TrimSpace(param.Type))
		if strings.TrimSpace(dependencyNames[index]) == "" {
			keepRecoveredUnnamedType := index < len(dependencyExpressions) &&
				sourceSelectorUnnamedDependencyKeepsRecoveredType(dependencyExpressions[index])
			if !keepRecoveredUnnamedType {
				currentType := normalizeInferredTypeText(strings.TrimSpace(currentParams[index].Type))
				if isAnyLikeType(currentType) || isLooselyTypedType(currentType) {
					paramType = currentType
				} else {
					paramType = "any"
				}
				if paramType != normalizeInferredTypeText(strings.TrimSpace(param.Type)) {
					changed = true
				}
			}
		}
		if paramType == "" {
			return recovered
		}

		rebuilt = append(rebuilt, fmt.Sprintf("%s: %s", strings.TrimSpace(rawName), paramType))
	}
	if !changed {
		return recovered
	}
	return "(" + strings.Join(rebuilt, ", ") + ") => " + normalizeInferredTypeText(strings.TrimSpace(recoveredReturnType))
}

func shouldPreferRecoveredInternalHelperFunctionType(current, candidate string) bool {
	current = normalizeSourceTypeTextWithOptions(strings.TrimSpace(current), false)
	candidate = normalizeSourceTypeTextWithOptions(strings.TrimSpace(candidate), false)
	if candidate == "" || candidate == current {
		return false
	}
	if internalHelperComplexRecoveryShouldStayOpaque(current, candidate) {
		return false
	}
	if current == "" {
		return true
	}
	if strings.Contains(current, "...") && !strings.Contains(candidate, "...") {
		return true
	}
	if internalHelperBuiltInNamespaceReturnShouldYieldToRecovered(current, candidate) {
		return true
	}
	if selectorFunctionTypePreservesMoreOptionalUndefined(current, candidate) || selectorFunctionTypePreservesMoreNullable(current, candidate) {
		return true
	}
	return internalHelperFunctionTypeRecoveryScore(candidate) > internalHelperFunctionTypeRecoveryScore(current)
}

func parityModeExplicitlyTypedUnnamedDependencyHelperShouldPreferRecovered(expression, current, candidate string) bool {
	if !parityModeEnabled() || !internalHelperComplexRecoveryShouldStayOpaque(current, candidate) {
		return false
	}

	dependencyNames := sourceSelectorDependencyNamesWithPlaceholders(expression)
	if len(dependencyNames) == 0 {
		return false
	}
	dependencyExpressions := sourceSelectorDependencyPartExpressions(expression)
	sawExplicitlyTypedUnnamedDependency := false
	for index, name := range dependencyNames {
		if strings.TrimSpace(name) != "" {
			continue
		}
		if index >= len(dependencyExpressions) || !sourceSelectorUnnamedDependencyKeepsRecoveredType(dependencyExpressions[index]) {
			return false
		}
		sawExplicitlyTypedUnnamedDependency = true
	}
	if !sawExplicitlyTypedUnnamedDependency {
		return false
	}

	_, candidateReturn, ok := splitFunctionType(candidate)
	if !ok {
		return false
	}
	return recoveredSelectorTypeLooksStrong(candidateReturn)
}

func parityModeUnnamedDependencyPlaceholderHelperShouldPreferRecovered(expression, current, candidate string) bool {
	if !parityModeEnabled() {
		return false
	}

	currentParameters, currentReturn, ok := splitFunctionType(current)
	if !ok {
		return false
	}
	candidateParameters, candidateReturn, ok := splitFunctionType(candidate)
	if !ok {
		return false
	}

	currentParams, ok := parseFunctionParameters(currentParameters)
	if !ok {
		return false
	}
	candidateParams, ok := parseFunctionParameters(candidateParameters)
	if !ok || len(currentParams) != len(candidateParams) {
		return false
	}

	dependencyNames := sourceSelectorDependencyNamesWithPlaceholders(expression)
	if len(dependencyNames) != len(candidateParams) {
		return false
	}
	dependencyExpressions := sourceSelectorDependencyPartExpressions(expression)

	if normalizeInternalHelperParameterType(currentReturn) != normalizeInternalHelperParameterType(candidateReturn) {
		return false
	}

	sawPlaceholderRename := false
	for index := range candidateParams {
		currentName, currentType, ok := splitTopLevelPropertyRaw(currentParams[index].Text)
		if !ok {
			return false
		}
		candidateName, candidateType, ok := splitTopLevelPropertyRaw(candidateParams[index].Text)
		if !ok {
			return false
		}
		if normalizeInternalHelperParameterType(currentType) != normalizeInternalHelperParameterType(candidateType) {
			return false
		}

		if strings.TrimSpace(dependencyNames[index]) != "" {
			continue
		}
		if index >= len(dependencyExpressions) || !sourceSelectorUnnamedDependencyKeepsRecoveredType(dependencyExpressions[index]) {
			continue
		}

		currentName = strings.TrimSpace(currentName)
		candidateName = strings.TrimSpace(candidateName)
		if currentName == candidateName {
			continue
		}
		if candidateName != opaqueInternalHelperParameterName(index) {
			return false
		}
		sawPlaceholderRename = true
	}

	return sawPlaceholderRename
}

func internalHelperBuiltInNamespaceReturnShouldYieldToRecovered(current, candidate string) bool {
	_, currentReturn, ok := splitFunctionType(current)
	if !ok {
		return false
	}
	_, candidateReturn, ok := splitFunctionType(candidate)
	if !ok {
		return false
	}
	currentReturn = normalizeInferredTypeText(strings.TrimSpace(currentReturn))
	candidateReturn = normalizeInferredTypeText(strings.TrimSpace(candidateReturn))
	if currentReturn == "" || candidateReturn == "" || currentReturn == candidateReturn {
		return false
	}
	if !isBuiltInNamespaceLikeType(currentReturn) {
		return false
	}
	return isBroadPrimitiveSelectorType(candidateReturn) ||
		selectorTypeIsLiteralUnionOfKind(candidateReturn, "string") ||
		selectorTypeIsLiteralUnionOfKind(candidateReturn, "number") ||
		selectorTypeIsLiteralUnionOfKind(candidateReturn, "boolean")
}

func internalHelperComplexRecoveryShouldStayOpaque(current, candidate string) bool {
	current = normalizeSourceTypeTextWithOptions(strings.TrimSpace(current), false)
	candidate = normalizeSourceTypeTextWithOptions(strings.TrimSpace(candidate), false)
	if current == "" || candidate == "" {
		return false
	}

	currentParameters, currentReturn, ok := splitFunctionType(current)
	if !ok {
		return false
	}
	currentReturn = normalizeInferredTypeText(strings.TrimSpace(currentReturn))
	if !isAnyLikeType(currentReturn) && !isLooselyTypedType(currentReturn) {
		return false
	}

	currentParams, ok := parseFunctionParameters(currentParameters)
	if !ok || len(currentParams) == 0 {
		return false
	}

	sawOpaqueCurrentParameter := false
	for _, param := range currentParams {
		paramType := normalizeInferredTypeText(strings.TrimSpace(param.Type))
		if paramType == "" || isAnyLikeType(paramType) || isLooselyTypedType(paramType) {
			sawOpaqueCurrentParameter = true
			break
		}
	}
	if !sawOpaqueCurrentParameter {
		return false
	}

	_, candidateReturn, ok := splitFunctionType(candidate)
	if !ok {
		return false
	}
	candidateReturn = normalizeInferredTypeText(strings.TrimSpace(candidateReturn))
	if candidateReturn == "" || !recoveredSelectorTypeLooksStrong(candidateReturn) {
		return false
	}
	if isPrimitiveLikeUnionType(candidateReturn) ||
		isBroadPrimitiveSelectorType(candidateReturn) ||
		selectorTypeIsLiteralUnionOfKind(candidateReturn, "string") ||
		selectorTypeIsLiteralUnionOfKind(candidateReturn, "number") ||
		selectorTypeIsLiteralUnionOfKind(candidateReturn, "boolean") {
		return false
	}
	return true
}

func parityModeOpaqueNullableLookupHelperShouldPreferRecovered(
	logic ParsedLogic,
	source,
	file,
	expression,
	current,
	candidate string,
	state *buildState,
) bool {
	if !parityModeEnabled() || !internalHelperComplexRecoveryShouldStayOpaque(current, candidate) {
		return false
	}

	_, candidateReturn, ok := splitFunctionType(candidate)
	if !ok {
		return false
	}
	return selectorOpaqueNullableLookupRecoveryShouldStayConcrete(logic, source, file, expression, candidateReturn, state)
}

func helperBooleanRecoveryShouldPreferRecovered(expression, current, candidate string) bool {
	currentParameters, currentReturn, ok := splitFunctionType(current)
	if !ok {
		return false
	}
	candidateParameters, candidateReturn, ok := splitFunctionType(candidate)
	if !ok || currentParameters != candidateParameters {
		return false
	}
	return selectorRecoveredBooleanShouldBeatCurrent(expression, currentReturn, candidateReturn)
}

func internalHelperFunctionTypeRecoveryScore(functionType string) int {
	text := normalizeSourceTypeTextWithOptions(strings.TrimSpace(functionType), false)
	if text == "" {
		return -1000
	}

	score := 0
	if strings.Contains(text, "...") {
		score -= 100
	}
	score -= countStandaloneTokenOccurrences(text, "any") * 4
	score -= countStandaloneTokenOccurrences(text, "unknown") * 4
	score += strings.Count(text, "|")

	parameters, returnType, ok := splitFunctionType(text)
	if !ok {
		return score
	}
	if params, ok := parseFunctionParameters(parameters); ok {
		score += len(params)
		score += internalHelperParameterNameRecoveryScore(params)
	}
	if normalizedReturn := normalizeInferredTypeText(strings.TrimSpace(returnType)); normalizedReturn != "" && !isAnyLikeType(normalizedReturn) && !isLooselyTypedType(normalizedReturn) {
		score += 2
	}
	return score
}

func internalHelperParameterNameRecoveryScore(params []parsedParameter) int {
	score := 0
	for _, param := range params {
		if _, ok := sourceParameterName(param.Text); !ok {
			score -= 4
		}
	}
	return score
}

func shouldPreferReportedSelectorType(current, reported string) bool {
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	reported = normalizeInferredTypeText(strings.TrimSpace(reported))
	if current == "" || reported == "" || current == reported {
		return false
	}
	return selectorTypeLooksLessInformative(current) && !selectorTypeLooksLessInformative(reported)
}

func shouldPreferExistingParsedSelectorType(existing, current string) bool {
	existing = normalizeInferredTypeText(strings.TrimSpace(existing))
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	if existing == "" || current == "" || existing == current {
		return false
	}
	if isAnyLikeType(current) || isLooselyTypedType(current) {
		return true
	}
	if selectorTypeIsLiteralDriven(current) && !selectorTypeIsLiteralDriven(existing) {
		return true
	}
	return selectorTypeLooksLessInformative(current) && !selectorTypeLooksLessInformative(existing)
}

func selectorRecoveredLiteralShouldYieldToReported(current, reported string) bool {
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	reported = normalizeInferredTypeText(strings.TrimSpace(reported))
	if current == "" || reported == "" || current == reported {
		return false
	}
	if selectorTypeIsLiteralDriven(current) {
		if isAnyLikeType(reported) || isLooselyTypedType(reported) {
			return true
		}
		switch reported {
		case "string", "number", "boolean":
			return true
		default:
			return !selectorTypeIsLiteralDriven(reported)
		}
	}
	return selectorRecoveredLiteralFunctionShouldYieldToReported(current, reported)
}

func selectorRecoveredLiteralFunctionShouldYieldToReported(current, reported string) bool {
	current = normalizeSelectorFunctionTypeOptionalUndefined(normalizeInferredTypeText(strings.TrimSpace(unwrapWrappedExpression(current))))
	reported = normalizeSelectorFunctionTypeOptionalUndefined(normalizeInferredTypeText(strings.TrimSpace(unwrapWrappedExpression(reported))))

	currentParameters, currentReturn, ok := splitFunctionType(current)
	if !ok {
		return false
	}
	reportedParameters, reportedReturn, ok := splitFunctionType(reported)
	if !ok {
		return false
	}
	if currentParameters != reportedParameters {
		return false
	}

	currentReturn = normalizeInferredTypeText(strings.TrimSpace(currentReturn))
	reportedReturn = normalizeInferredTypeText(strings.TrimSpace(reportedReturn))
	if currentReturn == "" || reportedReturn == "" || currentReturn == reportedReturn {
		return false
	}
	if !selectorTypeIsLiteralDriven(currentReturn) {
		return false
	}
	if isAnyLikeType(reportedReturn) || isLooselyTypedType(reportedReturn) {
		return true
	}
	return !selectorTypeIsLiteralDriven(reportedReturn)
}

func selectorTypeIsBareLiteral(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	return text == "null" ||
		text == "undefined" ||
		isQuotedString(text) ||
		isNumericLiteralType(text) ||
		isBooleanLiteralType(text)
}

func selectorTypeIsLiteralDriven(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return false
	}
	parts, err := splitTopLevelUnion(text)
	if err != nil || len(parts) == 0 {
		return selectorTypeIsBareLiteral(text)
	}
	sawLiteral := false
	for _, part := range parts {
		part = normalizeInferredTypeText(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if !selectorTypeIsBareLiteral(part) {
			return false
		}
		sawLiteral = true
	}
	return sawLiteral
}

func selectorMemberHasLooseReportedReturn(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	return strings.Contains(text, "=> any") || strings.Contains(text, "=> unknown")
}

func parityModeKeepsLooseReportedSelectorType(
	logic ParsedLogic,
	source,
	file string,
	currentSectionNames map[string]bool,
	expression,
	reportedType,
	memberType string,
	state *buildState,
) bool {
	if !parityModeEnabled() {
		return false
	}
	if strings.TrimSpace(expression) == "" || sourceSelectorReturnType(expression) != "" {
		return false
	}
	reportedType = normalizeInferredTypeText(strings.TrimSpace(reportedType))
	if !isAnyLikeType(reportedType) && !isLooselyTypedType(reportedType) {
		return false
	}
	if !selectorMemberHasLooseReportedReturn(memberType) {
		return false
	}
	knownFields := mergeParsedFields(logic.Reducers, logic.Selectors...)
	for _, dependencyType := range sourceSelectorDependencyTypes(logic, source, file, expression, state) {
		dependencyType = normalizeSourceTypeText(strings.TrimSpace(dependencyType))
		if dependencyType == "" {
			continue
		}
		if isAnyLikeType(dependencyType) || isLooselyTypedType(dependencyType) || dependencyType == "null" || dependencyType == "undefined" {
			return true
		}
	}
	for _, dependencyName := range sourceSelectorDependencyNames(expression) {
		dependencyName = strings.TrimSpace(dependencyName)
		if dependencyName == "" {
			continue
		}
		if currentSectionNames[dependencyName] {
			continue
		}
		if _, ok := findParsedField(knownFields, dependencyName); ok {
			continue
		}
		return true
	}
	return false
}

func parityModeTupleProjectorSelectorSurfaceShouldStayLoose(logic ParsedLogic, source, file, expression string, member MemberReport, parsedReturnType string, state *buildState) bool {
	if !parityModeEnabled() || strings.TrimSpace(expression) == "" || sourceSelectorReturnType(expression) != "" {
		return false
	}
	parsedReturnType = normalizeInferredTypeText(strings.TrimSpace(parsedReturnType))
	if parsedReturnType == "" || isAnyLikeType(parsedReturnType) || isLooselyTypedType(parsedReturnType) {
		return false
	}
	if selectorReportedMemberReturnTypeText(member) != "" {
		return false
	}
	if !selectorMemberTypeTextSupportsInternalHelper(preferredMemberTypeText(member)) {
		return false
	}
	return !selectorDependencyTypesLookConcrete(logic, source, file, expression, state)
}

func parityModeSelectorPropertyFallbackShouldStayAny(logic ParsedLogic, source, file, expression, inferred string, state *buildState) bool {
	if !parityModeEnabled() {
		return false
	}
	inferred = normalizeInferredTypeText(strings.TrimSpace(inferred))
	if inferred == "" || isAnyLikeType(inferred) || isLooselyTypedType(inferred) {
		return false
	}
	if isPrimitiveLikeUnionType(inferred) ||
		isBroadPrimitiveSelectorType(inferred) ||
		selectorTypeIsLiteralUnionOfKind(inferred, "string") ||
		selectorTypeIsLiteralUnionOfKind(inferred, "number") ||
		selectorTypeIsLiteralUnionOfKind(inferred, "boolean") {
		return false
	}

	projector := sourceSelectorProjectorElement(expression)
	if projector == "" {
		projector = expression
	}
	info, ok := parseSourceArrowInfo(projector)
	if !ok || strings.TrimSpace(info.ExplicitReturn) != "" {
		return false
	}

	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
		if body == "" {
			body = blockReturnExpression(info.Body)
		}
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}

	left, right, ok := sourceTopLevelLogicalOperands(body)
	if !ok {
		return false
	}
	leftParam, ok := sourceSimplePropertyFallbackParameterIndex(left, info.ParameterNames)
	if !ok {
		return false
	}
	rightParam, ok := sourceSimplePropertyFallbackParameterIndex(right, info.ParameterNames)
	if !ok {
		return false
	}

	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	if leftParam >= len(dependencyTypes) || rightParam >= len(dependencyTypes) {
		dependencyTypes = sourceSelectorDependencyTypesWithPlaceholders(logic, source, file, expression, state)
	}
	if leftParam >= len(dependencyTypes) || rightParam >= len(dependencyTypes) {
		return false
	}
	for _, index := range []int{leftParam, rightParam} {
		typeText := normalizeInferredTypeText(strings.TrimSpace(dependencyTypes[index]))
		if typeText == "" || isAnyLikeType(typeText) || isLooselyTypedType(typeText) || typeText == "null" || typeText == "undefined" {
			return true
		}
	}
	return false
}

func sourceTopLevelLogicalOperands(expression string) (string, string, bool) {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	for _, operator := range []string{"??", "||"} {
		index := findLastTopLevelOperator(text, operator)
		if index == -1 {
			continue
		}
		left := strings.TrimSpace(text[:index])
		right := strings.TrimSpace(text[index+len(operator):])
		if left == "" || right == "" {
			return "", "", false
		}
		return left, right, true
	}
	return "", "", false
}

func sourceTopLevelLogicalChainOperands(expression string) ([]string, bool) {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" {
		return nil, false
	}

	for _, operator := range []string{"??", "||"} {
		if findLastTopLevelOperator(text, operator) == -1 {
			continue
		}

		var operands []string
		var collect func(string) bool
		collect = func(part string) bool {
			part = strings.TrimSpace(unwrapWrappedExpression(part))
			if part == "" {
				return false
			}

			index := findLastTopLevelOperator(part, operator)
			if index == -1 {
				operands = append(operands, part)
				return true
			}

			left := strings.TrimSpace(part[:index])
			right := strings.TrimSpace(part[index+len(operator):])
			if left == "" || right == "" {
				return false
			}
			if !collect(left) {
				return false
			}
			operands = append(operands, right)
			return true
		}

		if collect(text) && len(operands) > 1 {
			return operands, true
		}
	}
	return nil, false
}

func sourceSimplePropertyFallbackParameterIndex(expression string, parameterNames []string) (int, bool) {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" || strings.Contains(text, "(") {
		return -1, false
	}
	for index, name := range parameterNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if strings.HasPrefix(text, name+".") || strings.HasPrefix(text, name+"?.") || strings.HasPrefix(text, name+"[") {
			return index, true
		}
	}
	return -1, false
}

func selectorSourceRecoveryAllowed(logic ParsedLogic, expression string) bool {
	if !selectorSourceUsesPropsDependency(expression) {
		return true
	}
	return logic.PropsType != "" && !isAnyLikeType(logic.PropsType)
}

func shouldRecoverSelectorTypeFromSource(logic ParsedLogic, source, file, current, inferred, expression string, state *buildState) bool {
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	inferred = normalizeInferredTypeText(strings.TrimSpace(inferred))
	if inferred == "" {
		return false
	}
	if !selectorSourceRecoveryAllowed(logic, expression) {
		return false
	}
	if !selectorTypeNeedsSourceRecovery(current) {
		return true
	}
	if !isAnyLikeType(current) && !isLooselyTypedType(current) {
		return true
	}
	if recoveredSelectorTypeLooksStrong(inferred) {
		return true
	}
	if !selectorSourceUsesPropsDependency(expression) {
		return true
	}
	return selectorPropsDependencyTypesLookConcrete(logic, source, file, expression, state)
}

func recoveredSelectorTypeLooksStrong(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if text == "" || isAnyLikeType(text) || isLooselyTypedType(text) || isBroadPrimitiveSelectorType(text) {
		return false
	}
	if _, ok := sourceIdentifierExpression(text); ok {
		return false
	}
	return strings.Contains(text, "<") ||
		strings.Contains(text, "{") ||
		strings.Contains(text, "|") ||
		strings.Contains(text, "[]") ||
		isFunctionLikeTypeText(text)
}

func selectorSourceUsesPropsDependency(expression string) bool {
	dependencies := sourceSelectorDependencyElements(expression)
	for _, dependency := range dependencies {
		if sourceExpressionUsesPropsDependency(dependency) {
			return true
		}
	}
	if len(dependencies) > 0 {
		return false
	}
	return sourceExpressionUsesPropsDependency(expression)
}

func selectorSourceHasRecoverableConstructor(expression string) bool {
	projector := sourceSelectorProjectorElement(expression)
	if projector == "" {
		projector = expression
	}
	info, ok := parseSourceArrowInfo(projector)
	if !ok {
		return false
	}
	if strings.TrimSpace(info.ExplicitReturn) != "" {
		return false
	}
	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
		if body == "" {
			body = blockReturnExpression(info.Body)
		}
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	for _, constructor := range []string{"Map", "Set"} {
		if strings.HasPrefix(body, "new "+constructor+"(") ||
			strings.HasPrefix(body, "new "+constructor+" (") ||
			strings.HasPrefix(body, "new "+constructor+"<") ||
			strings.HasPrefix(body, "new "+constructor+" <") {
			return true
		}
	}
	return false
}

func normalizePublicRecoveredSelectorType(logic ParsedLogic, source, file, expression, reportedType, candidate string, state *buildState) string {
	normalized := normalizeRecoveredSelectorType(source, file, candidate)
	if normalized == "" {
		return ""
	}
	objectFromEntriesMap := selectorObjectFromEntriesPrimitiveMapType(logic, source, file, expression, state)
	if selectorRecoveredTypeAliasShouldBePreserved(expression, candidate) {
		normalized = normalizeSelectorFunctionTypeOptionalUndefined(normalizeInferredTypeText(strings.TrimSpace(candidate)))
	}
	if objectFromEntriesMap != "" && selectorObjectFromEntriesPrimitiveMapShouldOverride(normalized) {
		normalized = objectFromEntriesMap
	}
	if isAnyLikeType(reportedType) && selectorOpaqueComplexRecoveryShouldStayAny(logic, source, file, expression, normalized, state) {
		return "any"
	}
	if isAnyLikeType(reportedType) && selectorIdentityConcreteDependencyTypeShouldBePreserved(logic, source, file, expression, normalized, state) {
		return normalized
	}
	if isAnyLikeType(reportedType) && selectorIdentityPrimitivePropsReturnShouldStayAny(logic, source, file, expression, normalized, state) {
		return "any"
	}
	if selectorIdentityPropsReturnShouldStayAny(expression, normalized) {
		return "any"
	}
	if selectorImportedAliasPropsIdentityShouldStayAny(logic, source, file, expression, reportedType, state) {
		return "any"
	}
	if selectorImportedPrimitiveMemberFallbackShouldStayAny(logic, source, file, expression, reportedType, normalized, state) {
		return "any"
	}
	if isAnyLikeType(reportedType) && selectorPrimitivePropsReturnShouldStayAny(logic, source, file, expression, normalized, state) {
		return "any"
	}
	if parityModeReportedStructuredTypeShouldStayLoose(reportedType, normalized) ||
		parityModeReportedStructuredTypeShouldBeatOpaqueCandidate(reportedType, normalized) {
		return reportedType
	}
	return normalized
}

func parityModeReportedStructuredHelperFunctionTypeShouldStayReported(current, candidate string) bool {
	if !parityModeEnabled() {
		return false
	}
	currentParameters, currentReturn, ok := splitFunctionType(current)
	if !ok {
		return false
	}
	candidateParameters, candidateReturn, ok := splitFunctionType(candidate)
	if !ok {
		return false
	}
	returnShouldStayReported := parityModeReportedStructuredTypeShouldStayLoose(currentReturn, candidateReturn) ||
		parityModeReportedStructuredTypeShouldBeatOpaqueCandidate(currentReturn, candidateReturn)
	if !returnShouldStayReported {
		return false
	}
	sawOpaquePreservation := structuredTypeUsesOpaqueLoosePreservation(currentReturn, candidateReturn) ||
		parityModeReportedStructuredTypeShouldBeatOpaqueCandidate(currentReturn, candidateReturn)
	currentParams, ok := parseFunctionParameters(currentParameters)
	if !ok {
		return false
	}
	candidateParams, ok := parseFunctionParameters(candidateParameters)
	if !ok || len(currentParams) != len(candidateParams) {
		return false
	}
	for index := range currentParams {
		currentType := normalizeInferredTypeText(strings.TrimSpace(currentParams[index].Type))
		candidateType := normalizeInferredTypeText(strings.TrimSpace(candidateParams[index].Type))
		if currentType == candidateType {
			continue
		}
		if isAnyLikeType(currentType) || isLooselyTypedType(currentType) ||
			isAnyLikeType(candidateType) || isLooselyTypedType(candidateType) {
			sawOpaquePreservation = true
			continue
		}
		if structuredTypeCandidateAddsOnlyNullish(currentType, candidateType) ||
			structuredTypeCandidateAddsOnlyNullish(candidateType, currentType) {
			continue
		}
		return false
	}
	return sawOpaquePreservation
}

func parityModeReportedStructuredTypeShouldStayLoose(reported, candidate string) bool {
	if !parityModeEnabled() {
		return false
	}
	reportedMembers, reportedNullish, ok := structuredTypeMembersAndNullish(reported)
	if !ok {
		return false
	}
	candidateMembers, candidateNullish, ok := structuredTypeMembersAndNullish(candidate)
	if !ok {
		return false
	}
	if len(reportedMembers) != len(candidateMembers) {
		return false
	}
	if !sameStringSet(reportedNullish, candidateNullish) {
		return false
	}
	sawLoosePreservation := false
	sawSpecificSurface := false
	for name, reportedType := range reportedMembers {
		candidateType, ok := candidateMembers[name]
		if !ok {
			return false
		}
		reportedType = normalizeInferredTypeText(strings.TrimSpace(reportedType))
		candidateType = normalizeInferredTypeText(strings.TrimSpace(candidateType))
		if !isAnyLikeType(reportedType) && !isLooselyTypedType(reportedType) {
			sawSpecificSurface = true
		}
		if reportedType == candidateType {
			continue
		}
		if isAnyLikeType(reportedType) || isLooselyTypedType(reportedType) {
			sawLoosePreservation = true
			continue
		}
		if structuredTypeCandidateAddsOnlyNullish(reportedType, candidateType) {
			sawLoosePreservation = true
			continue
		}
		return false
	}
	return sawLoosePreservation && sawSpecificSurface
}

func parityModeReportedStructuredTypeShouldBeatOpaqueCandidate(reported, candidate string) bool {
	if !parityModeEnabled() {
		return false
	}
	reportedMembers, reportedNullish, ok := structuredTypeMembersAndNullish(reported)
	if !ok {
		return false
	}
	candidateMembers, candidateNullish, ok := structuredTypeMembersAndNullish(candidate)
	if !ok {
		return false
	}
	if len(reportedMembers) != len(candidateMembers) {
		return false
	}
	if !sameStringSet(reportedNullish, candidateNullish) {
		return false
	}
	sawOpaqueLoss := false
	sawSpecificReported := false
	for name, reportedType := range reportedMembers {
		candidateType, ok := candidateMembers[name]
		if !ok {
			return false
		}
		reportedType = normalizeInferredTypeText(strings.TrimSpace(reportedType))
		candidateType = normalizeInferredTypeText(strings.TrimSpace(candidateType))
		if !isAnyLikeType(reportedType) && !isLooselyTypedType(reportedType) {
			sawSpecificReported = true
		}
		if reportedType == candidateType {
			continue
		}
		if !isAnyLikeType(reportedType) && !isLooselyTypedType(reportedType) &&
			(isAnyLikeType(candidateType) || isLooselyTypedType(candidateType)) {
			sawOpaqueLoss = true
			continue
		}
		return false
	}
	return sawOpaqueLoss && sawSpecificReported
}

func structuredTypeMembersAndNullish(typeText string) (map[string]string, []string, bool) {
	text := normalizeSourceTypeTextWithOptions(strings.TrimSpace(typeText), false)
	if text == "" {
		return nil, nil, false
	}
	parts, err := splitTopLevelUnion(text)
	if err != nil || len(parts) == 0 {
		parts = []string{text}
	}

	nullish := []string{}
	objectType := ""
	for _, part := range parts {
		part = normalizeSourceTypeTextWithOptions(strings.TrimSpace(part), false)
		switch part {
		case "", "null", "undefined":
			if part != "" {
				nullish = append(nullish, part)
			}
		default:
			if objectType != "" {
				return nil, nil, false
			}
			objectType = part
		}
	}
	if objectType == "" {
		return nil, nil, false
	}
	members, ok := parseObjectTypeMembersWithOptionalUndefined(objectType, true)
	if !ok || len(members) == 0 {
		return nil, nil, false
	}
	sort.Strings(nullish)
	return members, nullish, true
}

func structuredTypeUsesOpaqueLoosePreservation(reported, candidate string) bool {
	reportedMembers, _, ok := structuredTypeMembersAndNullish(reported)
	if !ok {
		return false
	}
	candidateMembers, _, ok := structuredTypeMembersAndNullish(candidate)
	if !ok || len(reportedMembers) != len(candidateMembers) {
		return false
	}
	for name, reportedType := range reportedMembers {
		candidateType, ok := candidateMembers[name]
		if !ok {
			return false
		}
		reportedType = normalizeInferredTypeText(strings.TrimSpace(reportedType))
		candidateType = normalizeInferredTypeText(strings.TrimSpace(candidateType))
		if reportedType == candidateType {
			continue
		}
		if isAnyLikeType(reportedType) || isLooselyTypedType(reportedType) {
			return true
		}
	}
	return false
}

func parityModeKeepsLooseContextualProjectorReportedSelectorType(expression, reportedType string, member MemberReport) bool {
	if !parityModeEnabled() || strings.TrimSpace(expression) == "" || sourceSelectorReturnType(expression) != "" {
		return false
	}
	reportedType = normalizeInferredTypeText(strings.TrimSpace(reportedType))
	if !isAnyLikeType(reportedType) && !isLooselyTypedType(reportedType) {
		return false
	}
	if !selectorMemberHasLooseReportedReturn(strings.TrimSpace(member.TypeString)) {
		return false
	}
	return selectorMemberHasLooseReportedReturn(strings.TrimSpace(preferredSelectorProjectorTypeText(member)))
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func structuredTypeCandidateAddsOnlyNullish(current, candidate string) bool {
	currentMain, currentNullish, ok := structuredTypeMainAndNullish(current)
	if !ok {
		return false
	}
	candidateMain, candidateNullish, ok := structuredTypeMainAndNullish(candidate)
	if !ok || currentMain != candidateMain || currentNullish == candidateNullish {
		return false
	}
	currentParts := map[string]bool{}
	for _, part := range strings.Split(currentNullish, "|") {
		part = strings.TrimSpace(part)
		if part != "" {
			currentParts[part] = true
		}
	}
	sawExtra := false
	for _, part := range strings.Split(candidateNullish, "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if currentParts[part] {
			continue
		}
		if part != "null" && part != "undefined" {
			return false
		}
		sawExtra = true
	}
	return sawExtra
}

func structuredTypeMainAndNullish(typeText string) (string, string, bool) {
	text := normalizeSourceTypeTextWithOptions(strings.TrimSpace(typeText), false)
	if text == "" {
		return "", "", false
	}
	parts, err := splitTopLevelUnion(text)
	if err != nil || len(parts) == 0 {
		parts = []string{text}
	}
	mainParts := make([]string, 0, len(parts))
	nullish := make([]string, 0, 2)
	for _, part := range parts {
		part = normalizeSourceTypeTextWithOptions(strings.TrimSpace(part), false)
		switch part {
		case "", "null", "undefined":
			if part != "" {
				nullish = append(nullish, part)
			}
		default:
			mainParts = append(mainParts, part)
		}
	}
	if len(mainParts) != 1 {
		return "", "", false
	}
	sort.Strings(nullish)
	return mainParts[0], strings.Join(nullish, "|"), true
}

func selectorObjectFromEntriesPrimitiveMapType(logic ParsedLogic, source, file, expression string, state *buildState) string {
	body := selectorProjectorReturnBody(expression)
	if body == "" {
		return ""
	}
	text := strings.TrimSpace(unwrapWrappedExpression(body))
	callee, ok := stripTrailingCallExpression(text)
	if !ok || strings.TrimSpace(callee) != "Object.fromEntries" {
		return ""
	}
	arguments, ok := trailingCallArguments(text)
	if !ok || len(arguments) != 1 {
		return ""
	}
	hints := selectorProjectorDependencyHints(logic, source, file, expression, state)
	if inferred := normalizeSourceTypeText(sourceExpressionTypeTextWithContext(source, file, text, hints, state)); inferred != "" {
		if strings.HasPrefix(inferred, "{ [k: ") {
			return inferred
		}
		if keyType, valueType, ok := parseRecordTypeArguments(inferred); ok {
			if !selectorObjectFromEntriesValueShouldUseIndexSignature(valueType) {
				return ""
			}
			keyType = normalizeObjectFromEntriesKeyType(keyType)
			if keyType == "" {
				return ""
			}
			return fmt.Sprintf("{ [k: %s]: %s; }", keyType, valueType)
		}
	}
	collectionType := sourceCollectionExpressionTypeTextWithContext(source, file, strings.TrimSpace(arguments[0]), hints, state)
	elementType := collectionElementType(collectionType)
	keyType, valueType, ok := tupleKeyValueTypes(elementType)
	if !ok {
		return ""
	}
	valueType = normalizeSourceTypeText(strings.TrimSpace(valueType))
	if !selectorObjectFromEntriesValueShouldUseIndexSignature(valueType) {
		return ""
	}
	keyType = normalizeObjectFromEntriesKeyType(keyType)
	if keyType == "" {
		return ""
	}
	return fmt.Sprintf("{ [k: %s]: %s; }", keyType, valueType)
}

func selectorObjectFromEntriesValueShouldUseIndexSignature(valueType string) bool {
	valueType = normalizeSourceTypeText(strings.TrimSpace(valueType))
	if valueType == "" {
		return false
	}
	if valueType == "true" || valueType == "false" {
		return true
	}
	if valueType == "string" || valueType == "number" || valueType == "boolean" {
		return true
	}
	return selectorTypeIsLiteralUnionOfKind(valueType, "string") ||
		selectorTypeIsLiteralUnionOfKind(valueType, "number") ||
		selectorTypeIsLiteralUnionOfKind(valueType, "boolean")
}

func selectorObjectFromEntriesPrimitiveMapShouldOverride(typeText string) bool {
	typeText = normalizeInferredTypeText(strings.TrimSpace(typeText))
	if typeText == "" {
		return false
	}
	if typeText == "ObjectConstructor" {
		return true
	}
	return strings.HasPrefix(typeText, "Record<") || strings.HasPrefix(typeText, "{ [k: ")
}

func selectorProjectorDependencyHints(logic ParsedLogic, source, file, expression string, state *buildState) map[string]string {
	projector := sourceSelectorProjectorElement(expression)
	if projector == "" {
		projector = expression
	}
	info, ok := parseSourceArrowInfo(projector)
	if !ok || len(info.ParameterNames) == 0 {
		return nil
	}

	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	if parityModeEnabled() && len(dependencyTypes) < len(info.ParameterNames) {
		dependencyTypes = sourceSelectorDependencyTypesWithPlaceholders(logic, source, file, expression, state)
	}
	if len(dependencyTypes) < len(info.ParameterNames) {
		return nil
	}

	hints := map[string]string{}
	for index, name := range info.ParameterNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		typeText := normalizeSourceTypeText(strings.TrimSpace(dependencyTypes[index]))
		if typeText == "" {
			continue
		}
		hints[name] = typeText
	}
	if len(hints) == 0 {
		return nil
	}
	return hints
}

func selectorOpaqueComplexRecoveryShouldStayAny(logic ParsedLogic, source, file, expression, typeText string, state *buildState) bool {
	typeText = normalizeInferredTypeText(strings.TrimSpace(typeText))
	if typeText == "" || !recoveredSelectorTypeLooksStrong(typeText) {
		return false
	}
	if isPrimitiveLikeUnionType(typeText) ||
		isBroadPrimitiveSelectorType(typeText) ||
		selectorTypeIsLiteralUnionOfKind(typeText, "string") ||
		selectorTypeIsLiteralUnionOfKind(typeText, "number") ||
		selectorTypeIsLiteralUnionOfKind(typeText, "boolean") {
		return false
	}
	if selectorOpaqueNullableLookupRecoveryShouldStayConcrete(logic, source, file, expression, typeText, state) {
		return false
	}
	return !selectorDependencyTypesLookConcrete(logic, source, file, expression, state)
}

func selectorOpaqueNullableLookupRecoveryShouldStayConcrete(
	logic ParsedLogic,
	source,
	file,
	expression,
	typeText string,
	state *buildState,
) bool {
	if !parityModeEnabled() {
		return false
	}

	typeText = normalizeInferredTypeText(strings.TrimSpace(typeText))
	if typeText == "" ||
		(!strings.Contains(typeText, "| null") &&
			!strings.Contains(typeText, "| undefined") &&
			!strings.Contains(typeText, "null |") &&
			!strings.Contains(typeText, "undefined |")) {
		return false
	}

	body := selectorProjectorReturnBody(expression)
	if body == "" {
		return false
	}
	left, ok := selectorSimpleFallbackAccessExpression(body)
	if !ok || !selectorLookupCallLooksRecoverable(left) {
		return false
	}

	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	if len(dependencyTypes) == 0 {
		return false
	}

	sawOpaqueDependency := false
	sawConcreteNonPrimitiveDependency := false
	for _, dependencyType := range dependencyTypes {
		typeText := normalizeSourceTypeText(strings.TrimSpace(dependencyType))
		if typeText == "" || isAnyLikeType(typeText) || isLooselyTypedType(typeText) {
			sawOpaqueDependency = true
			continue
		}
		if expanded := expandLocalSourceTypeText(source, typeText); expanded != "" {
			typeText = normalizeSourceTypeText(expanded)
		}
		if expanded := expandIndexedAccessesInTypeText(source, file, typeText, state); expanded != "" {
			typeText = normalizeSourceTypeText(expanded)
		}
		if isPrimitiveLikeKeyType(typeText) ||
			isBroadPrimitiveSelectorType(typeText) ||
			selectorTypeIsLiteralUnionOfKind(typeText, "string") ||
			selectorTypeIsLiteralUnionOfKind(typeText, "number") ||
			selectorTypeIsLiteralUnionOfKind(typeText, "boolean") {
			continue
		}
		sawConcreteNonPrimitiveDependency = true
	}
	return sawOpaqueDependency && sawConcreteNonPrimitiveDependency
}

func selectorProjectorReturnBody(expression string) string {
	projector := sourceSelectorProjectorElement(expression)
	if projector == "" {
		projector = expression
	}
	info, ok := parseSourceArrowInfo(projector)
	if !ok {
		return ""
	}
	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
		if body == "" {
			body = blockReturnExpression(info.Body)
		}
	}
	return strings.TrimSpace(body)
}

func selectorLookupCallLooksRecoverable(expression string) bool {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" {
		return false
	}
	for _, fragment := range []string{".find(", "?.find(", ".findLast(", "?.findLast("} {
		if strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}

func selectorRecoveredBooleanShouldBeatCurrent(expression, current, recovered string) bool {
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	recovered = normalizeInferredTypeText(strings.TrimSpace(recovered))
	if current == "" || recovered == "" || current == recovered || !booleanLikeSelectorType(recovered) {
		return false
	}
	if !selectorProjectorReturnLooksBoolean(expression) {
		return false
	}
	if selectorTypeIsLiteralDriven(current) {
		return true
	}
	return !isPrimitiveLikeUnionType(current)
}

func booleanLikeSelectorType(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	return text == "boolean" || selectorTypeIsLiteralUnionOfKind(text, "boolean")
}

func selectorProjectorReturnLooksBoolean(expression string) bool {
	body := selectorProjectorReturnBody(expression)
	if body == "" {
		return false
	}
	return sourceExpressionLooksBoolean(body)
}

func parityModeObjectFromEntriesBooleanMapHelperShouldBeOmitted(expression, functionType string) bool {
	if !parityModeEnabled() {
		return false
	}
	body := selectorProjectorReturnBody(expression)
	if body == "" || !strings.Contains(body, "Object.fromEntries(") {
		return false
	}

	parameters, returnType, ok := splitFunctionType(functionType)
	if !ok {
		return false
	}
	returnType = normalizeInferredTypeText(strings.TrimSpace(returnType))
	if returnType != "Record<string, boolean>" {
		return false
	}

	params, ok := parseFunctionParameters(parameters)
	if !ok || len(params) < 3 {
		return false
	}
	for _, param := range params {
		if strings.Contains(normalizeInferredTypeText(strings.TrimSpace(param.Type)), "DeepPartialMap<") {
			return true
		}
	}
	return false
}

func sourceExpressionLooksBoolean(expression string) bool {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" {
		return false
	}
	if text == "true" || text == "false" ||
		strings.HasPrefix(text, "!") ||
		strings.HasPrefix(text, "Boolean(") ||
		strings.HasPrefix(text, "!!") ||
		strings.Contains(text, ".some(") ||
		strings.Contains(text, "===") ||
		strings.Contains(text, "!==") ||
		strings.Contains(text, ">=") ||
		strings.Contains(text, "<=") ||
		strings.Contains(text, ">") ||
		strings.Contains(text, "<") {
		return true
	}
	return findLastTopLevelOperator(text, "&&") != -1 || findLastTopLevelOperator(text, "||") != -1
}

func selectorIdentityConcreteDependencyTypeShouldBePreserved(logic ParsedLogic, source, file, expression, typeText string, state *buildState) bool {
	typeText = normalizeInferredTypeText(strings.TrimSpace(typeText))
	if typeText == "" || !isPrimitiveLikeKeyType(typeText) {
		return false
	}
	if selectorSourceUsesPropsDependency(expression) && !selectorPropsDependencyUsesExplicitType(expression) {
		return false
	}
	parameterIndex, ok := selectorProjectorIdentityParameterIndex(expression)
	if !ok {
		return false
	}
	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	if parameterIndex < 0 || parameterIndex >= len(dependencyTypes) {
		return false
	}
	dependencyType := normalizeSourceTypeText(strings.TrimSpace(dependencyTypes[parameterIndex]))
	if dependencyType == "" || !isPrimitiveLikeKeyType(dependencyType) {
		return false
	}
	return dependencyType == typeText
}

func selectorPropsDependencyUsesExplicitType(expression string) bool {
	if !selectorSourceUsesPropsDependency(expression) {
		return false
	}

	for _, dependency := range sourceSelectorDependencyPartExpressions(expression) {
		if !sourceExpressionUsesPropsDependency(dependency) {
			continue
		}
		info, ok := parseSourceArrowInfo(dependency)
		if !ok {
			continue
		}
		parts, ok := splitFunctionParameterParts(info.Parameters)
		if !ok || len(parts) < 2 {
			continue
		}
		_, typeText, ok := splitTopLevelProperty(strings.TrimSpace(parts[1]))
		if !ok {
			continue
		}
		typeText = normalizeSourceTypeText(strings.TrimSpace(typeText))
		if typeText != "" && !isAnyLikeType(typeText) && !isLooselyTypedType(typeText) {
			return true
		}
	}
	return false
}

func selectorIdentityPrimitivePropsReturnShouldStayAny(logic ParsedLogic, source, file, expression, typeText string, state *buildState) bool {
	typeText = normalizeInferredTypeText(strings.TrimSpace(typeText))
	if typeText == "" || !isPrimitiveLikeKeyType(typeText) {
		return false
	}
	if sourceSelectorReturnType(expression) != "" {
		return false
	}
	dependencyType := selectorIdentityPrimitivePropsDependencyType(logic, source, file, expression, state)
	return dependencyType != "" && dependencyType == typeText
}

func selectorIdentityPrimitivePropsDependencyType(logic ParsedLogic, source, file, expression string, state *buildState) string {
	if !selectorSourceUsesPropsDependency(expression) {
		return ""
	}
	parameterIndex, ok := selectorProjectorIdentityParameterIndex(expression)
	if !ok {
		return ""
	}
	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	if parameterIndex < 0 || parameterIndex >= len(dependencyTypes) {
		return ""
	}
	dependencyType := normalizeSourceTypeText(strings.TrimSpace(dependencyTypes[parameterIndex]))
	if dependencyType == "" {
		return ""
	}
	if expanded := expandLocalSourceTypeText(source, dependencyType); expanded != "" {
		dependencyType = normalizeSourceTypeText(expanded)
	}
	if expanded := expandIndexedAccessesInTypeText(source, file, dependencyType, state); expanded != "" {
		dependencyType = normalizeSourceTypeText(expanded)
	}
	if !isPrimitiveLikeKeyType(dependencyType) {
		return ""
	}
	return dependencyType
}

func selectorIdentityPropsDependency(expression string) bool {
	if !selectorSourceUsesPropsDependency(expression) {
		return false
	}
	_, ok := selectorProjectorIdentityParameterIndex(expression)
	return ok
}

func selectorImportedAliasPropsIdentityShouldStayAny(logic ParsedLogic, source, file, expression, reportedType string, state *buildState) bool {
	if !isAnyLikeType(reportedType) || !selectorSourceUsesPropsDependency(expression) {
		return false
	}
	parameterIndex, ok := selectorProjectorIdentityParameterIndex(expression)
	if !ok {
		return false
	}
	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	if parameterIndex < 0 || parameterIndex >= len(dependencyTypes) {
		return false
	}
	dependencyType := normalizeSourceTypeText(strings.TrimSpace(dependencyTypes[parameterIndex]))
	if dependencyType == "" {
		return false
	}
	if expanded := expandLocalSourceTypeText(source, dependencyType); expanded != "" {
		dependencyType = normalizeSourceTypeText(expanded)
	}
	_, ok = sourceIdentifierExpression(dependencyType)
	return ok
}

func selectorImportedPrimitiveMemberFallbackShouldStayAny(logic ParsedLogic, source, file, expression, reportedType, typeText string, state *buildState) bool {
	if !isAnyLikeType(reportedType) {
		return false
	}
	typeText = normalizeInferredTypeText(strings.TrimSpace(typeText))
	if typeText == "" ||
		(!isPrimitiveLikeUnionType(typeText) &&
			!selectorTypeIsLiteralUnionOfKind(typeText, "string") &&
			!selectorTypeIsLiteralUnionOfKind(typeText, "number") &&
			!selectorTypeIsLiteralUnionOfKind(typeText, "boolean")) {
		return false
	}

	projector := sourceSelectorProjectorElement(expression)
	if projector == "" {
		projector = expression
	}
	info, ok := parseSourceArrowInfo(projector)
	if !ok {
		return false
	}
	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
		if body == "" {
			body = blockReturnExpression(info.Body)
		}
	}
	left, ok := selectorSimpleFallbackAccessExpression(body)
	if !ok {
		return false
	}
	root, _, ok := parseSourceAccessPath(left)
	if !ok || root == "" {
		return false
	}
	parameterIndex := -1
	for index, name := range info.ParameterNames {
		if strings.TrimSpace(name) == root {
			parameterIndex = index
			break
		}
	}
	if parameterIndex == -1 {
		return false
	}
	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	if parameterIndex >= len(dependencyTypes) {
		return false
	}
	return typeTextContainsImportedAliasReference(source, file, dependencyTypes[parameterIndex], state)
}

func selectorSimpleFallbackAccessExpression(expression string) (string, bool) {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" {
		return "", false
	}
	for _, operator := range []string{"??", "||"} {
		index := findLastTopLevelOperator(text, operator)
		if index == -1 {
			continue
		}
		left := strings.TrimSpace(text[:index])
		right := strings.TrimSpace(text[index+len(operator):])
		if left == "" || right == "" || !sourceFallbackLiteralShouldDefer(sourceLiteralTypeText(right)) {
			continue
		}
		if findLastTopLevelOperator(left, "??") != -1 || findLastTopLevelOperator(left, "||") != -1 {
			return "", false
		}
		return left, true
	}
	return "", false
}

func typeTextContainsImportedAliasReference(source, file, typeText string, state *buildState) bool {
	parts, err := splitTopLevelUnion(normalizeSourceTypeText(strings.TrimSpace(typeText)))
	if err != nil || len(parts) == 0 {
		parts = []string{typeText}
	}
	for _, part := range parts {
		part = normalizeSourceTypeText(strings.TrimSpace(part))
		if part == "" || part == "null" || part == "undefined" {
			continue
		}
		if expanded := expandImportedAliasOrWrappedTypeText(source, file, part, state); expanded != "" {
			return true
		}
	}
	return false
}

func expandImportedAliasOrWrappedTypeText(source, file, typeText string, state *buildState) string {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return ""
	}
	if expanded := expandImportedTypeAliasTextWithContext(source, file, text, state); expanded != "" {
		return expanded
	}
	for _, wrapper := range []string{"Partial", "DeepPartial", "Readonly"} {
		if inner, ok := parseSingleGenericTypeArgument(text, wrapper); ok {
			return expandImportedAliasOrWrappedTypeText(source, file, inner, state)
		}
	}
	return ""
}

func selectorProjectorIdentityParameterIndex(expression string) (int, bool) {
	projector := sourceSelectorProjectorElement(expression)
	if projector == "" {
		projector = expression
	}
	info, ok := parseSourceArrowInfo(projector)
	if !ok {
		return -1, false
	}
	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
		if body == "" {
			body = blockReturnExpression(info.Body)
		}
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return -1, false
	}
	for index, parameterName := range info.ParameterNames {
		if body == strings.TrimSpace(parameterName) {
			return index, true
		}
	}
	return -1, false
}

func selectorPrimitivePropsReturnShouldStayAny(logic ParsedLogic, source, file, expression, typeText string, state *buildState) bool {
	typeText = normalizeInferredTypeText(strings.TrimSpace(typeText))
	if typeText == "" || !selectorSourceUsesPropsDependency(expression) || !isPrimitiveLikeKeyType(typeText) {
		return false
	}

	projector := sourceSelectorProjectorElement(expression)
	if projector == "" {
		projector = expression
	}
	info, ok := parseSourceArrowInfo(projector)
	if !ok {
		return false
	}
	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
		if body == "" {
			body = blockReturnExpression(info.Body)
		}
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	if sourceKeyBodyUsesDeclaredHelper(source, file, body, state) {
		return false
	}
	parameterIndex, ok := selectorLeadingParameterReferenceIndex(body, info.ParameterNames)
	if !ok {
		return false
	}

	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	if parameterIndex >= 0 && parameterIndex < len(dependencyTypes) {
		dependencyType := normalizeSourceTypeText(strings.TrimSpace(dependencyTypes[parameterIndex]))
		if dependencyType != "" {
			if expanded := expandLocalSourceTypeText(source, dependencyType); expanded != "" {
				dependencyType = normalizeSourceTypeText(expanded)
			}
			if expanded := expandIndexedAccessesInTypeText(source, file, dependencyType, state); expanded != "" {
				dependencyType = normalizeSourceTypeText(expanded)
			}
			if isPrimitiveLikeKeyType(dependencyType) {
				return false
			}
		}
	}

	return true
}

func selectorBodyStartsFromParameterReference(body string, parameterNames []string) bool {
	_, ok := selectorLeadingParameterReferenceIndex(body, parameterNames)
	return ok
}

func selectorLeadingParameterReferenceIndex(body string, parameterNames []string) (int, bool) {
	text := trimLeadingSourceTrivia(unwrapWrappedExpression(strings.TrimSpace(body)))
	if text == "" {
		return -1, false
	}
	if callee, ok := stripTrailingCallExpression(text); ok {
		text = trimLeadingSourceTrivia(unwrapWrappedExpression(strings.TrimSpace(callee)))
	}
	for index, parameterName := range parameterNames {
		name := strings.TrimSpace(parameterName)
		if name == "" {
			continue
		}
		if text == name ||
			strings.HasPrefix(text, name+".") ||
			strings.HasPrefix(text, name+"?.") ||
			strings.HasPrefix(text, name+"[") ||
			strings.HasPrefix(text, name+"?.[") {
			return index, true
		}
	}
	return -1, false
}

func sourceExpressionUsesPropsDependency(expression string) bool {
	text := strings.TrimSpace(expression)
	if text == "" {
		return false
	}
	if info, ok := parseSourceArrowInfo(text); ok {
		if len(info.ParameterNames) >= 2 {
			propsName := strings.TrimSpace(info.ParameterNames[1])
			if propsName != "" {
				body := strings.TrimSpace(info.Body)
				if info.BlockBody {
					body = singleReturnExpression(body)
					if body == "" {
						body = blockReturnExpression(info.Body)
					}
				}
				body = strings.TrimSpace(body)
				if body == propsName ||
					strings.Contains(body, propsName+".") ||
					strings.Contains(body, propsName+"?.") ||
					strings.Contains(body, propsName+"[") {
					return true
				}
			}
		}
		body := strings.TrimSpace(info.Body)
		if info.BlockBody {
			body = singleReturnExpression(body)
			if body == "" {
				body = blockReturnExpression(info.Body)
			}
		}
		if body != "" && body != text && sourceExpressionUsesPropsDependency(body) {
			return true
		}
	}
	if parts := sourceSelectorArrayParts(text); len(parts) > 0 {
		for _, part := range parts {
			if sourceExpressionUsesPropsDependency(part) {
				return true
			}
		}
	}
	return false
}

func selectorPropsDependencyTypesLookConcrete(logic ParsedLogic, source, file, expression string, state *buildState) bool {
	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	if len(dependencyTypes) == 0 {
		return false
	}
	for _, dependencyType := range dependencyTypes {
		typeText := normalizeInferredTypeText(strings.TrimSpace(dependencyType))
		if typeText == "" || isAnyLikeType(typeText) || isLooselyTypedType(typeText) {
			return false
		}
		if _, ok := sourceIdentifierExpression(typeText); ok {
			return false
		}
		if isBroadPrimitiveSelectorType(typeText) ||
			selectorTypeIsLiteralUnionOfKind(typeText, "string") ||
			selectorTypeIsLiteralUnionOfKind(typeText, "number") ||
			selectorTypeIsLiteralUnionOfKind(typeText, "boolean") {
			continue
		}
		return false
	}
	return true
}

func selectorDependencyTypesLookConcrete(logic ParsedLogic, source, file, expression string, state *buildState) bool {
	if len(sourceSelectorDependencyNames(expression)) == 0 {
		return true
	}
	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	if len(dependencyTypes) == 0 {
		return false
	}
	sawConcreteDependency := false
	for _, dependencyType := range dependencyTypes {
		typeText := normalizeInferredTypeText(strings.TrimSpace(dependencyType))
		if typeText == "" || isAnyLikeType(typeText) || isLooselyTypedType(typeText) {
			return false
		}
		sawConcreteDependency = true
	}
	return sawConcreteDependency
}

func shouldPreferInferredSelectorType(source, file, current, inferred string) bool {
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	inferred = normalizeInferredTypeText(strings.TrimSpace(inferred))
	if current == "" || inferred == "" || current == inferred {
		return false
	}
	if expanded := expandImportedTypeAliasText(source, file, current); expanded != "" {
		current = normalizeInferredTypeText(expanded)
	} else if expanded := expandLocalSourceTypeText(source, current); expanded != "" {
		current = normalizeInferredTypeText(expanded)
	}
	switch inferred {
	case "string":
		return selectorTypeIsLiteralUnionOfKind(current, "string")
	case "number":
		return selectorTypeIsLiteralUnionOfKind(current, "number")
	case "boolean":
		return selectorTypeIsLiteralUnionOfKind(current, "boolean")
	default:
		return false
	}
}

func shouldPreferStrongRecoveredSelectorType(current, recovered string) bool {
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	recovered = normalizeInferredTypeText(strings.TrimSpace(recovered))
	if current == "" || recovered == "" || current == recovered {
		return false
	}
	if !recoveredSelectorTypeLooksStrong(recovered) {
		return false
	}
	if selectorTypeIsLiteralDriven(current) {
		return true
	}
	if isAnyLikeType(current) || isLooselyTypedType(current) {
		return true
	}
	if isBroadPrimitiveSelectorType(current) {
		return isArrayLikeType(recovered) ||
			strings.HasPrefix(recovered, "Record<") ||
			strings.HasPrefix(recovered, "Map<") ||
			strings.HasPrefix(recovered, "Set<") ||
			strings.Contains(recovered, "{") ||
			strings.Contains(recovered, "| null") ||
			strings.Contains(recovered, "| undefined") ||
			isFunctionLikeTypeText(recovered)
	}
	return false
}

func selectorTypeIsLiteralUnionOfKind(typeText, kind string) bool {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return false
	}

	parts, err := splitTopLevelUnion(text)
	if err != nil || len(parts) == 0 {
		parts = []string{text}
	}

	sawLiteral := false
	for _, part := range parts {
		part = normalizeSourceTypeText(strings.TrimSpace(part))
		switch kind {
		case "string":
			if !isQuotedString(part) {
				return false
			}
		case "number":
			if !isNumericLiteralType(part) {
				return false
			}
		case "boolean":
			if !isBooleanLiteralType(part) {
				return false
			}
		default:
			return false
		}
		sawLiteral = true
	}
	return sawLiteral
}

func sortLiteralUnionMembers(typeText string) string {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return ""
	}

	parts, err := splitTopLevelUnion(text)
	if err != nil || len(parts) < 2 {
		return text
	}

	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = normalizeSourceTypeText(strings.TrimSpace(part))
		if part == "" {
			return text
		}
		if !isQuotedString(part) && !isNumericLiteralType(part) && !isBooleanLiteralType(part) {
			return text
		}
		normalized = append(normalized, part)
	}

	sorted := append([]string(nil), normalized...)
	sort.Strings(sorted)
	for index := range normalized {
		if normalized[index] != sorted[index] {
			return strings.Join(sorted, " | ")
		}
	}
	return text
}

func selectorTypeLooksLessInformative(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if strings.HasSuffix(text, "Constructor") {
		return false
	}
	if selectorTypeNeedsSourceRecovery(text) {
		return true
	}
	switch text {
	case "Array", "Map", "Promise", "ReadonlyArray", "Set":
		return true
	}
	return strings.Contains(text, "...") ||
		strings.Contains(text, "Set<any>") ||
		strings.Contains(text, "Set<unknown>") ||
		strings.Contains(text, "Map<any") ||
		strings.Contains(text, "Map<unknown") ||
		strings.Contains(text, "Promise<any>") ||
		strings.Contains(text, "Promise<unknown>") ||
		strings.Contains(text, "Array<any>") ||
		strings.Contains(text, "Array<unknown>") ||
		strings.Contains(text, "ReadonlyArray<any>") ||
		strings.Contains(text, "ReadonlyArray<unknown>")
}

func refineSelectorTypesFromInternalHelpers(selectors []ParsedField, helpers []ParsedFunction, allowOpaqueCurrentRefinement bool) []ParsedField {
	if len(selectors) == 0 || len(helpers) == 0 {
		return selectors
	}

	refined := append([]ParsedField(nil), selectors...)
	for index, selector := range refined {
		var helper ParsedFunction
		found := false
		for _, candidate := range helpers {
			if candidate.Name == selector.Name {
				helper = candidate
				found = true
				break
			}
		}
		if !found {
			continue
		}
		_, returnType, ok := splitFunctionType(helper.FunctionType)
		if !ok {
			continue
		}
		returnType = wrapSelectorFunctionType(normalizeSelectorFunctionTypeOptionalUndefined(returnType))
		if shouldPreferInternalHelperSelectorReturn(selector.Type, helper.FunctionType, returnType, allowOpaqueCurrentRefinement) {
			refined[index].Type = returnType
		}
	}
	return refined
}

func shouldPreferInternalHelperSelectorReturn(current, helperFunctionType, helperReturn string, allowOpaqueCurrentRefinement bool) bool {
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	helperReturn = normalizeInferredTypeText(strings.TrimSpace(helperReturn))
	if current == "" || helperReturn == "" || current == helperReturn {
		return false
	}
	if selectorLiteralNullishShouldYieldToInternalHelper(current, helperReturn) {
		return true
	}
	if selectorLiteralBooleanShouldYieldToInternalHelper(current, helperReturn) {
		return true
	}
	if allowOpaqueCurrentRefinement && selectorOpaqueCurrentShouldYieldToInternalHelper(current, helperReturn) {
		return true
	}
	if isAnyLikeType(current) || isLooselyTypedType(current) {
		return internalHelperReturnCanRefineOpaqueSelector(helperFunctionType, helperReturn)
	}
	if isAnyLikeType(helperReturn) || isLooselyTypedType(helperReturn) {
		return false
	}
	if selectorTypeMatchesInternalHelperParameter(current, helperFunctionType) {
		return true
	}
	if selectorMalformedObjectArrayShouldYieldToInternalHelper(current, helperReturn) {
		return true
	}
	if (isBroadPrimitiveSelectorType(helperReturn) ||
		selectorTypeIsLiteralUnionOfKind(helperReturn, "string") ||
		selectorTypeIsLiteralUnionOfKind(helperReturn, "number") ||
		selectorTypeIsLiteralUnionOfKind(helperReturn, "boolean")) &&
		!isPrimitiveLikeUnionType(current) {
		return true
	}
	return selectorReportedTypeShouldYieldToParsed(current, helperReturn)
}

func selectorMalformedObjectArrayShouldYieldToInternalHelper(current, helperReturn string) bool {
	currentElementType, ok := parseNormalizedArrayElementType(current)
	if !ok {
		return false
	}
	helperElementType, ok := parseNormalizedArrayElementType(helperReturn)
	if !ok {
		return false
	}

	currentMembers, ok := parseObjectTypeMembersWithOptionalUndefined(currentElementType, true)
	if !ok || len(currentMembers) == 0 {
		return false
	}
	helperMembers, ok := parseObjectTypeMembersWithOptionalUndefined(helperElementType, true)
	if !ok || len(helperMembers) == 0 {
		return false
	}

	conflictingMemberType := false
	for name, currentType := range currentMembers {
		helperType, ok := helperMembers[name]
		if !ok {
			return false
		}
		if normalizeSourceTypeText(strings.TrimSpace(currentType)) != normalizeSourceTypeText(strings.TrimSpace(helperType)) {
			conflictingMemberType = true
		}
	}
	if conflictingMemberType {
		return true
	}
	return len(currentMembers) <= 1 && len(helperMembers) > len(currentMembers)
}

func internalHelperReturnCanRefineOpaqueSelector(functionType, returnType string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(returnType))
	if text == "" || isAnyLikeType(text) || isLooselyTypedType(text) || selectorTypeIsLiteralDriven(text) {
		return false
	}
	parameters, _, ok := splitFunctionType(functionType)
	if !ok {
		return false
	}
	params, ok := parseFunctionParameters(parameters)
	if !ok {
		return false
	}
	if len(params) == 0 {
		return true
	}
	sawInformativeParameter := false
	for _, param := range params {
		paramType := normalizeInferredTypeText(strings.TrimSpace(param.Type))
		if paramType == "" || isAnyLikeType(paramType) || isLooselyTypedType(paramType) {
			if parityModeEnabled() {
				return false
			}
			continue
		}
		sawInformativeParameter = true
		break
	}
	if !sawInformativeParameter {
		return false
	}
	if isPrimitiveLikeUnionType(text) || isBroadPrimitiveSelectorType(text) {
		return true
	}
	return strings.Contains(text, "|") ||
		strings.Contains(text, "{") ||
		strings.Contains(text, "[]") ||
		strings.Contains(text, "<") ||
		isFunctionLikeTypeText(text)
}

func selectorLiteralNullishShouldYieldToInternalHelper(current, helperReturn string) bool {
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	helperReturn = normalizeInferredTypeText(strings.TrimSpace(helperReturn))
	if current == "" || helperReturn == "" || current == helperReturn {
		return false
	}
	if !normalizedUnionContainsType(current, "null") && !normalizedUnionContainsType(current, "undefined") {
		return false
	}
	parts, err := splitTopLevelUnion(current)
	if err != nil || len(parts) == 0 {
		parts = []string{current}
	}
	for _, part := range parts {
		part = normalizeInferredTypeText(strings.TrimSpace(part))
		if part == "" || part == "null" || part == "undefined" {
			continue
		}
		return false
	}
	if normalizedUnionContainsType(helperReturn, "null") || normalizedUnionContainsType(helperReturn, "undefined") {
		return false
	}
	return !isAnyLikeType(helperReturn) && !isLooselyTypedType(helperReturn)
}

func selectorLiteralBooleanShouldYieldToInternalHelper(current, helperReturn string) bool {
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	helperReturn = normalizeInferredTypeText(strings.TrimSpace(helperReturn))
	if current == "" || helperReturn == "" || current == helperReturn {
		return false
	}
	if !booleanLikeSelectorType(helperReturn) {
		return false
	}

	parts, err := splitTopLevelUnion(current)
	if err != nil || len(parts) == 0 {
		parts = []string{current}
	}
	sawBooleanLiteral := false
	for _, part := range parts {
		part = normalizeInferredTypeText(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if !isBooleanLiteralType(part) {
			return false
		}
		sawBooleanLiteral = true
	}
	return sawBooleanLiteral
}

func selectorOpaqueCurrentShouldYieldToInternalHelper(current, helperReturn string) bool {
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	helperReturn = normalizeInferredTypeText(strings.TrimSpace(helperReturn))
	if current == "" || helperReturn == "" || current == helperReturn {
		return false
	}
	if !isAnyLikeType(current) && !isLooselyTypedType(current) {
		return false
	}
	if isAnyLikeType(helperReturn) || isLooselyTypedType(helperReturn) {
		return false
	}
	return isBroadPrimitiveSelectorType(helperReturn) ||
		selectorTypeIsLiteralUnionOfKind(helperReturn, "string") ||
		selectorTypeIsLiteralUnionOfKind(helperReturn, "number") ||
		selectorTypeIsLiteralUnionOfKind(helperReturn, "boolean")
}

func selectorTypeMatchesInternalHelperParameter(typeText, functionType string) bool {
	parameters, _, ok := splitFunctionType(functionType)
	if !ok {
		return false
	}
	params, ok := parseFunctionParameters(parameters)
	if !ok {
		return false
	}
	current := normalizeInferredTypeText(strings.TrimSpace(typeText))
	for _, parameter := range params {
		parameterType := normalizeInferredTypeText(strings.TrimSpace(parameter.Type))
		if parameterType == current {
			return true
		}
	}
	return false
}

func selectorPublicTypeSuppressesInternalHelper(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	return strings.HasSuffix(text, "Constructor")
}

func resolveReportedSelectorType(preferred, parsed string) string {
	preferred = normalizeInferredTypeText(strings.TrimSpace(preferred))
	parsed = normalizeInferredTypeText(strings.TrimSpace(parsed))

	switch {
	case preferred == "":
		return parsed
	case parsed == "":
		return preferred
	case !selectorReturnTypeConflicts(preferred, parsed):
		return preferred
	case selectorTypeLooksLessInformative(preferred) && !selectorTypeLooksLessInformative(parsed):
		return parsed
	case selectorTypeLooksLessInformative(parsed) && !selectorTypeLooksLessInformative(preferred):
		return preferred
	case selectorReportedTypeShouldYieldToParsed(preferred, parsed):
		return parsed
	default:
		return preferred
	}
}

func selectorReportedTypeShouldYieldToParsed(preferred, parsed string) bool {
	if isFunctionLikeTypeText(parsed) && !isFunctionLikeTypeText(preferred) {
		return true
	}
	return isBroadPrimitiveSelectorType(preferred) && !isBroadPrimitiveSelectorType(parsed)
}

func isBroadPrimitiveSelectorType(typeText string) bool {
	switch normalizeInferredTypeText(strings.TrimSpace(typeText)) {
	case "string", "number", "boolean":
		return true
	default:
		return false
	}
}

func internalSelectorFunctionTypeIsUninformative(functionType string, params []parsedParameter) bool {
	_, returnType, ok := splitFunctionType(functionType)
	if !ok {
		return false
	}
	allParamsOpaque := true
	allParamsGeneric := true
	for _, param := range params {
		if !isOpaqueBuilderInternalHelperParameterType(param.Type) {
			allParamsOpaque = false
		}
		name, ok := sourceParameterName(param.Text)
		if !ok || !isGenericInternalHelperParameterName(name) {
			allParamsGeneric = false
		}
	}
	if isAnyLikeSelectorHelperType(returnType) && allParamsOpaque {
		return true
	}
	return allParamsOpaque && allParamsGeneric
}

func isOpaqueBuilderInternalHelperParameterType(typeText string) bool {
	if isAnyLikeSelectorHelperType(typeText) {
		return true
	}
	return isFunctionLikeTypeText(typeText)
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

func isGenericInternalHelperParameterName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "arg" {
		return true
	}
	if strings.HasPrefix(name, "arg") && len(name) > 3 {
		for _, ch := range name[3:] {
			if ch < '0' || ch > '9' {
				return false
			}
		}
		return true
	}
	return false
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
	return strings.HasPrefix(text, "[")
}

func selectorFunctionTypeFromInputTuple(member MemberReport) string {
	returnType := normalizeInferredTypeText(strings.TrimSpace(selectorReportedMemberReturnTypeText(member)))
	if returnType == "" {
		return ""
	}
	if functionType := selectorFunctionTypeFromInputTupleText(strings.TrimSpace(preferredSelectorInputTupleTypeText(member)), returnType); functionType != "" {
		return functionType
	}
	return ""
}

func selectorFunctionTypeFromMember(member MemberReport) string {
	for _, text := range []string{
		strings.TrimSpace(preferredSelectorProjectorTypeText(member)),
		strings.TrimSpace(member.TypeString),
		strings.TrimSpace(member.PrintedTypeNode),
	} {
		if text == "" {
			continue
		}
		if parameters, returnType, ok := splitFunctionType(text); ok && strings.TrimSpace(firstParameterText(parameters)) != "" {
			return parameters + " => " + normalizeInferredTypeText(returnType)
		}
		if element := sourceSelectorProjectorElement(text); element != "" {
			if parameters, returnType, ok := splitFunctionType(element); ok && strings.TrimSpace(firstParameterText(parameters)) != "" {
				return parameters + " => " + normalizeInferredTypeText(returnType)
			}
		}
		if element := lastTopLevelArrayElement(text); element != "" {
			if parameters, returnType, ok := splitFunctionType(element); ok && strings.TrimSpace(firstParameterText(parameters)) != "" {
				return parameters + " => " + normalizeInferredTypeText(returnType)
			}
		}
	}
	return ""
}

func selectorInputTupleTypeTextFromSelectorMember(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	element := firstTopLevelArrayElement(text)
	if element == "" {
		return ""
	}
	_, returnType, ok := splitFunctionType(element)
	if !ok {
		return ""
	}
	return normalizeSourceTypeText(strings.TrimSpace(returnType))
}

func selectorFunctionTypeFromInputTupleText(tupleText, returnType string) string {
	elementTypes := selectorInputTupleElementTypes(tupleText)
	if len(elementTypes) == 0 {
		return ""
	}
	parameters := make([]string, 0, len(elementTypes))
	for index, elementType := range elementTypes {
		parameterType := normalizeInternalHelperSignatureParameterType(elementType, true)
		if parameterType == "" {
			return ""
		}
		name := "arg"
		if index > 0 {
			name = fmt.Sprintf("arg%d", index+1)
		}
		parameters = append(parameters, fmt.Sprintf("%s: %s", name, parameterType))
	}
	return "(" + strings.Join(parameters, ", ") + ") => " + returnType
}

func selectorInputTupleElementTypes(text string) []string {
	text = normalizeSourceTypeText(strings.TrimSpace(text))
	if text == "" {
		return nil
	}
	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(text, 0, len(text))
	if err != nil || !ok || arrayEnd <= arrayStart {
		return nil
	}
	parts, err := splitTopLevelList(text[arrayStart+1 : arrayEnd])
	if err != nil || len(parts) == 0 {
		return nil
	}
	elementTypes := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil
		}
		if _, returnType, ok := splitFunctionType(part); ok {
			normalized := normalizeInferredTypeText(strings.TrimSpace(returnType))
			if normalized == "" {
				return nil
			}
			elementTypes = append(elementTypes, normalized)
			continue
		}
		elementTypes = append(elementTypes, "any")
	}
	return elementTypes
}

func sourceInternalSelectorFunctionType(logic ParsedLogic, source, file, expression string, state *buildState) string {
	return sourceInternalSelectorFunctionTypeWithFallbackReturn(logic, source, file, expression, "", state)
}

func sourceInternalSelectorFunctionTypeWithFallbackReturn(logic ParsedLogic, source, file, expression, fallbackReturnType string, state *buildState) string {
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
	dependencyNames := sourceSelectorDependencyNamesWithPlaceholders(expression)
	dependencyExpressions := sourceSelectorDependencyPartExpressions(expression)
	parameterNames := internalHelperParameterNames(dependencyNames, info.ParameterNames)
	parameterCount := len(parameterNames)
	if parityModeEnabled() && len(dependencyTypes) < parameterCount {
		dependencyTypes = sourceSelectorDependencyTypesWithPlaceholders(logic, source, file, expression, state)
	}
	if parameterCount == 0 || len(dependencyTypes) < parameterCount {
		return ""
	}
	returnType := info.ExplicitReturn
	if returnType == "" {
		inferredReturnType := sourceSelectorInferredType(logic, source, file, expression, state)
		bodyExpression := strings.TrimSpace(info.Body)
		if info.BlockBody {
			bodyExpression = singleReturnExpression(info.Body)
			if bodyExpression == "" {
				bodyExpression = blockReturnExpression(info.Body)
			}
		}
		if concreteReturn := internalHelperDirectFallbackConcreteDependencyReturnType(expression, bodyExpression, dependencyTypes); concreteReturn != "" &&
			(inferredReturnType == "" || isAnyLikeType(inferredReturnType) || isLooselyTypedType(inferredReturnType)) {
			inferredReturnType = concreteReturn
		}
		fallbackReturnType = normalizeInferredTypeText(strings.TrimSpace(fallbackReturnType))
		if internalHelperOpaqueComplexFallbackReturnShouldStayAny(logic, source, file, expression, info.Body, info.BlockBody, fallbackReturnType, inferredReturnType, dependencyTypes, state) ||
			internalHelperOpaqueTemplateStringReturnShouldStayAny(info.Body, info.BlockBody, fallbackReturnType, inferredReturnType, dependencyTypes) {
			returnType = fallbackReturnType
		} else {
			returnType = inferredReturnType
		}
		if returnType == "" {
			returnType = fallbackReturnType
		}
	}
	if returnType == "" {
		return ""
	}

	parameters := make([]string, 0, parameterCount)
	preserveNullableParameterTypes := true
	for index, name := range parameterNames {
		parameterTypeText := dependencyTypes[index]
		if parityModeEnabled() && index < len(dependencyNames) && strings.TrimSpace(dependencyNames[index]) == "" {
			if index >= len(dependencyExpressions) || !sourceSelectorUnnamedDependencyKeepsRecoveredType(dependencyExpressions[index]) {
				parameterTypeText = "any"
			}
		}
		if parityModeEnabled() && strings.TrimSpace(parameterTypeText) == "" {
			parameterTypeText = "any"
		}
		parameterType := normalizeInternalHelperSignatureParameterType(parameterTypeText, preserveNullableParameterTypes)
		if parameterType == "" {
			return ""
		}
		parameters = append(parameters, fmt.Sprintf("%s: %s", name, parameterType))
	}
	return "(" + strings.Join(parameters, ", ") + ") => " + returnType
}

func internalHelperOpaqueComplexFallbackReturnShouldStayAny(
	logic ParsedLogic,
	source,
	file,
	expression,
	body string,
	blockBody bool,
	fallbackReturnType,
	inferredReturnType string,
	dependencyTypes []string,
	state *buildState,
) bool {
	fallbackReturnType = normalizeInferredTypeText(strings.TrimSpace(fallbackReturnType))
	inferredReturnType = normalizeInferredTypeText(strings.TrimSpace(inferredReturnType))
	if (!isAnyLikeType(fallbackReturnType) && !isLooselyTypedType(fallbackReturnType)) || inferredReturnType == "" {
		return false
	}
	if !recoveredSelectorTypeLooksStrong(inferredReturnType) {
		return false
	}
	if isPrimitiveLikeUnionType(inferredReturnType) ||
		isBroadPrimitiveSelectorType(inferredReturnType) ||
		selectorTypeIsLiteralUnionOfKind(inferredReturnType, "string") ||
		selectorTypeIsLiteralUnionOfKind(inferredReturnType, "number") ||
		selectorTypeIsLiteralUnionOfKind(inferredReturnType, "boolean") {
		return false
	}
	if selectorOpaqueNullableLookupRecoveryShouldStayConcrete(logic, source, file, expression, inferredReturnType, state) {
		return false
	}

	bodyExpression := strings.TrimSpace(body)
	if blockBody {
		bodyExpression = singleReturnExpression(body)
		if bodyExpression == "" {
			bodyExpression = blockReturnExpression(body)
		}
	}
	bodyExpression = strings.TrimSpace(bodyExpression)
	if bodyExpression == "" {
		return false
	}
	if findLastTopLevelOperator(bodyExpression, "??") == -1 && findLastTopLevelOperator(bodyExpression, "||") == -1 {
		return false
	}
	if internalHelperDirectFallbackConcreteDependencyReturnShouldBePreserved(expression, bodyExpression, inferredReturnType, dependencyTypes) {
		return false
	}

	for _, dependencyType := range dependencyTypes {
		dependencyType = normalizeInferredTypeText(strings.TrimSpace(dependencyType))
		if dependencyType == "" ||
			isAnyLikeType(dependencyType) ||
			isLooselyTypedType(dependencyType) ||
			dependencyType == "null" ||
			dependencyType == "undefined" {
			return true
		}
	}
	return false
}

func internalHelperDirectFallbackConcreteDependencyReturnShouldBePreserved(
	expression,
	bodyExpression,
	inferredReturnType string,
	dependencyTypes []string,
) bool {
	if !parityModeEnabled() {
		return false
	}

	inferredReturnType = normalizeInferredTypeText(strings.TrimSpace(inferredReturnType))
	if inferredReturnType == "" {
		return false
	}
	return internalHelperDirectFallbackConcreteDependencyReturnType(expression, bodyExpression, dependencyTypes) == inferredReturnType
}

func internalHelperDirectFallbackConcreteDependencyReturnType(expression, bodyExpression string, dependencyTypes []string) string {
	if !parityModeEnabled() {
		return ""
	}

	projector := sourceSelectorProjectorElement(expression)
	if projector == "" {
		projector = expression
	}
	info, ok := parseSourceArrowInfo(projector)
	if !ok || len(info.ParameterNames) == 0 {
		return ""
	}

	operands, ok := sourceTopLevelLogicalChainOperands(bodyExpression)
	if !ok || len(operands) < 2 {
		return ""
	}

	concreteReturnType := ""
	for _, operand := range operands {
		operand = strings.TrimSpace(unwrapWrappedExpression(operand))
		if operand == "" {
			return ""
		}
		if sourceFallbackLiteralShouldDefer(sourceLiteralTypeText(operand)) {
			continue
		}
		parameterIndex := -1
		for index, name := range info.ParameterNames {
			if operand == strings.TrimSpace(name) {
				parameterIndex = index
				break
			}
		}
		if parameterIndex == -1 || parameterIndex >= len(dependencyTypes) {
			return ""
		}

		dependencyType := normalizeSourceTypeText(strings.TrimSpace(dependencyTypes[parameterIndex]))
		if dependencyType == "" || isAnyLikeType(dependencyType) || isLooselyTypedType(dependencyType) {
			continue
		}
		if concreteReturnType == "" {
			concreteReturnType = dependencyType
			continue
		}
		if concreteReturnType != dependencyType {
			return ""
		}
	}
	if !recoveredSelectorTypeLooksStrong(concreteReturnType) {
		return ""
	}
	return concreteReturnType
}

func internalHelperOpaqueTemplateStringReturnShouldStayAny(body string, blockBody bool, fallbackReturnType, inferredReturnType string, dependencyTypes []string) bool {
	fallbackReturnType = normalizeInferredTypeText(strings.TrimSpace(fallbackReturnType))
	inferredReturnType = normalizeInferredTypeText(strings.TrimSpace(inferredReturnType))
	if (!isAnyLikeType(fallbackReturnType) && !isLooselyTypedType(fallbackReturnType)) || inferredReturnType != "string" {
		return false
	}

	expression := strings.TrimSpace(body)
	if blockBody {
		expression = singleReturnExpression(body)
		if expression == "" {
			expression = blockReturnExpression(body)
		}
	}
	expression = strings.TrimSpace(expression)
	if expression == "" || !strings.Contains(expression, "`") {
		return false
	}

	for _, dependencyType := range dependencyTypes {
		dependencyType = normalizeInferredTypeText(strings.TrimSpace(dependencyType))
		if dependencyType == "" ||
			isAnyLikeType(dependencyType) ||
			isLooselyTypedType(dependencyType) ||
			dependencyType == "null" ||
			dependencyType == "undefined" {
			return true
		}
	}
	return false
}

func sourceSelectorDependencyTypesWithPlaceholders(logic ParsedLogic, source, file, expression string, state *buildState) []string {
	var dependencyTypes []string
	for _, part := range sourceSelectorDependencyElements(expression) {
		dependencyTypes = append(dependencyTypes, sourceSelectorDependencyTypesFromElementWithPlaceholders(logic, source, file, part, state)...)
	}
	return dependencyTypes
}

func sourceSelectorDependencyTypesFromElementWithPlaceholders(logic ParsedLogic, source, file, expression string, state *buildState) []string {
	if info, ok := parseSourceArrowInfo(expression); ok {
		body := info.Body
		if info.BlockBody {
			body = singleReturnExpression(body)
		}
		hints := sourceSelectorInputParameterHints(logic, source, info.Parameters)
		if dependencyTypes, ok := sourceDependencyTypesFromReturnedArrayWithPlaceholders(logic, source, file, body, hints, state); ok {
			return dependencyTypes
		}
		if dependencyType := resolveSelectorDependencyType(logic, source, file, body, state); dependencyType != "" {
			return []string{dependencyType}
		}
		if dependencyType := sourceSelectorDependencyCallbackReturnType(logic, source, file, body, state); dependencyType != "" {
			return []string{dependencyType}
		}
		if returnType := sourceArrowReturnTypeText(source, body); returnType != "" {
			return []string{returnType}
		}
		if returnType := sourceExpressionTypeTextWithContext(source, file, body, hints, state); returnType != "" {
			if normalized := normalizeSelectorDependencyType(returnType); normalized != "" {
				return []string{normalized}
			}
		}
		return []string{""}
	}

	if dependencyTypes, ok := sourceDependencyTypesFromReturnedArrayWithPlaceholders(logic, source, file, expression, nil, state); ok {
		return dependencyTypes
	}
	if dependencyType := resolveSelectorDependencyType(logic, source, file, expression, state); dependencyType != "" {
		return []string{dependencyType}
	}
	if dependencyType := sourceSelectorDependencyCallbackReturnType(logic, source, file, expression, state); dependencyType != "" {
		return []string{dependencyType}
	}
	if returnType := sourceArrowReturnTypeText(source, expression); returnType != "" {
		return []string{returnType}
	}
	return []string{""}
}

func sourceDependencyTypesFromReturnedArrayWithPlaceholders(logic ParsedLogic, source, file, expression string, hints map[string]string, state *buildState) ([]string, bool) {
	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(expression, 0, len(expression))
	if err != nil || !ok {
		return nil, false
	}
	parts, err := splitTopLevelList(expression[arrayStart+1 : arrayEnd])
	if err != nil || len(parts) == 0 {
		return nil, false
	}

	dependencyTypes := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			dependencyTypes = append(dependencyTypes, "")
			continue
		}
		if dependencyType := resolveSelectorDependencyType(logic, source, file, part, state); dependencyType != "" {
			dependencyTypes = append(dependencyTypes, dependencyType)
			continue
		}
		if callbackType := sourceSelectorDependencyCallbackReturnType(logic, source, file, part, state); callbackType != "" {
			dependencyTypes = append(dependencyTypes, callbackType)
			continue
		}
		if returnType := sourceArrowReturnTypeText(source, part); returnType != "" {
			dependencyTypes = append(dependencyTypes, returnType)
			continue
		}
		if returnType := sourceExpressionTypeTextWithContext(source, file, part, hints, state); returnType != "" {
			if normalized := normalizeSelectorDependencyType(returnType); normalized != "" {
				dependencyTypes = append(dependencyTypes, normalized)
				continue
			}
		}
		dependencyTypes = append(dependencyTypes, "")
	}
	return dependencyTypes, true
}

func normalizeInternalHelperSignatureParameterType(typeText string, preserveNullable bool) string {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return ""
	}
	if unwrapped := unwrapWrappedExpression(text); unwrapped != "" {
		text = unwrapped
	}
	if preserveNullable {
		if normalizedUnionContainsType(text, "null") || normalizedUnionContainsType(text, "undefined") {
			return text
		}
	}
	return normalizeInternalHelperParameterType(text)
}

func sectionSourceProperties(source string, property SourceProperty) map[string]SourceProperty {
	if source == "" || property.ValueEnd <= property.ValueStart {
		return nil
	}
	expression := sourcePropertyText(source, property)
	if expression == "" {
		return nil
	}
	if argument := singleCallArgumentExpression(expression); argument != "" {
		expression = argument
	}

	parseProperties := func(start, end int) map[string]SourceProperty {
		objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(source, start, end)
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

	if info, ok := parseSourceArrowInfo(expression); ok {
		body := strings.TrimSpace(info.Body)
		if info.BlockBody {
			body = singleReturnExpression(body)
			if body == "" {
				body = blockReturnExpression(info.Body)
			}
		}
		if body != "" {
			bodyOffset := strings.Index(source[property.ValueStart:property.ValueEnd], body)
			if bodyOffset != -1 {
				start := property.ValueStart + bodyOffset
				if properties := parseProperties(start, start+len(body)); len(properties) > 0 {
					return properties
				}
			}
		}
	}

	return parseProperties(property.ValueStart, property.ValueEnd)
}

func FindSectionProperties(source string, property SourceProperty) []SourceProperty {
	properties := sectionSourceProperties(source, property)
	if len(properties) == 0 {
		return nil
	}

	ordered := make([]SourceProperty, 0, len(properties))
	for _, nested := range properties {
		ordered = append(ordered, nested)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].NameStart == ordered[j].NameStart {
			return ordered[i].Name < ordered[j].Name
		}
		return ordered[i].NameStart < ordered[j].NameStart
	})
	return ordered
}

func canonicalizeSourceProperties(
	source, file string,
	properties map[string]SourceProperty,
	state *buildState,
) map[string]SourceProperty {
	if len(properties) == 0 {
		return nil
	}
	result := make(map[string]SourceProperty, len(properties))
	for _, property := range properties {
		property.Name = canonicalSourceObjectMemberName(source, file, property.Name, state)
		result[property.Name] = property
	}
	return result
}

func orderedSourcePropertyNames(properties map[string]SourceProperty) []string {
	if len(properties) == 0 {
		return nil
	}
	ordered := make([]SourceProperty, 0, len(properties))
	for _, property := range properties {
		ordered = append(ordered, property)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].NameStart == ordered[j].NameStart {
			return ordered[i].Name < ordered[j].Name
		}
		return ordered[i].NameStart < ordered[j].NameStart
	})

	names := make([]string, 0, len(ordered))
	for _, property := range ordered {
		names = append(names, property.Name)
	}
	return names
}

func sectionSourceEntries(source string, property SourceProperty) []sourceObjectEntry {
	if source == "" || property.ValueEnd <= property.ValueStart {
		return nil
	}
	expression := sourcePropertyText(source, property)
	if expression == "" {
		return nil
	}
	if argument := singleCallArgumentExpression(expression); argument != "" {
		expression = argument
	}
	if info, ok := parseSourceArrowInfo(expression); ok {
		body := strings.TrimSpace(info.Body)
		if info.BlockBody {
			body = singleReturnExpression(body)
			if body == "" {
				body = blockReturnExpression(info.Body)
			}
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

func sectionSourceMemberNames(source string, property SourceProperty) []string {
	if source == "" || property.ValueEnd <= property.ValueStart {
		return nil
	}
	expression := sourcePropertyText(source, property)
	if expression == "" {
		return nil
	}
	if argument := singleCallArgumentExpression(expression); argument != "" {
		expression = argument
	}
	if info, ok := parseSourceArrowInfo(expression); ok {
		body := strings.TrimSpace(info.Body)
		if info.BlockBody {
			body = singleReturnExpression(body)
			if body == "" {
				body = blockReturnExpression(info.Body)
			}
		}
		if names := sourceObjectMemberNamesFromExpression(body); len(names) > 0 {
			return names
		}
	}
	return sourceObjectMemberNamesFromExpression(expression)
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

func sourceObjectMemberNamesFromExpression(expression string) []string {
	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(expression, 0, len(expression))
	if err != nil || !ok {
		return nil
	}
	segments, err := splitTopLevelSourceSegments(expression, objectStart+1, objectEnd)
	if err != nil {
		return nil
	}

	seen := map[string]bool{}
	names := make([]string, 0, len(segments))
	for _, segment := range segments {
		text := strings.TrimSpace(segment.Text)
		if text == "" || strings.HasPrefix(text, "...") {
			continue
		}
		name, _, _, ok, err := parseTopLevelPropertyName(text, 0, len(text))
		if err != nil || !ok || name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

func singleCallArgumentExpression(expression string) string {
	text := strings.TrimSpace(expression)
	if !strings.HasSuffix(text, ")") {
		return ""
	}

	depth := 0
	for i := len(text) - 1; i >= 0; i-- {
		switch text[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				arguments, err := splitTopLevelList(text[i+1 : len(text)-1])
				if err != nil || len(arguments) != 1 {
					return ""
				}
				return strings.TrimSpace(arguments[0])
			}
		}
	}
	return ""
}

func normalizeSingleCallbackExpression(expression string) string {
	text := strings.TrimSpace(expression)
	if text == "" {
		return ""
	}
	if argument := singleCallArgumentExpression(text); argument != "" {
		argument = strings.TrimSpace(argument)
		if _, ok := parseSourceArrowInfo(argument); ok {
			return argument
		}
		if strings.HasPrefix(argument, "function") {
			return argument
		}
	}
	return text
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

func sourceActionPayloadTypeFromSource(source, file, expression string, state *buildState) string {
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
	if sourceExpressionIsEmptyObjectLiteral(body) {
		return "{}"
	}
	hints := sourceActionParameterTypeHintsWithContext(source, file, info.Parameters, state)
	if objectType := sourceObjectLiteralTypeTextWithHints(source, body, hints); objectType != "" {
		return objectType
	}
	return normalizeActionPayloadType(sourceExpressionTypeTextWithContext(source, file, body, hints, state))
}

func sourceExpressionIsEmptyObjectLiteral(expression string) bool {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" {
		text = strings.TrimSpace(expression)
	}
	if text == "" {
		return false
	}

	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(text, 0, len(text))
	if err != nil || !ok {
		return false
	}
	segments, err := splitTopLevelSourceSegments(text, objectStart+1, objectEnd)
	if err != nil {
		return false
	}
	for _, segment := range segments {
		if strings.TrimSpace(segment.Text) != "" {
			return false
		}
	}
	return true
}

func sourceParameterTypeHints(source, parameters string) map[string]string {
	return sourceParameterTypeHintsWithContext(source, "", parameters, nil)
}

func sourceParameterTypeHintsWithContext(source, file, parameters string, state *buildState) map[string]string {
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
		if expanded := expandSourceParameterHintTypeText(source, file, typeText, state); expanded != "" {
			typeText = normalizeSourceTypeText(expanded)
		}
		if parameterDeclarationIsOptional(parameter.Text) && typeText != "" && !strings.Contains(typeText, "undefined") {
			typeText = normalizeSourceTypeText(typeText + " | undefined")
		}
		hints[name] = typeText
	}
	return hints
}

func sourceActionParameterTypeHintsWithContext(source, file, parameters string, state *buildState) map[string]string {
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
		rawType := normalizeSourceTypeText(parameter.Type)
		typeText := rawType
		if expanded := expandSourceParameterHintTypeText(source, file, rawType, state); expanded != "" {
			expanded = normalizeSourceTypeText(expanded)
			if actionParameterTypeHintShouldUseExpandedType(rawType, expanded) {
				typeText = expanded
			}
		}
		if parameterDeclarationIsOptional(parameter.Text) && typeText != "" && !strings.Contains(typeText, "undefined") {
			typeText = normalizeSourceTypeText(typeText + " | undefined")
		}
		hints[name] = typeText
	}
	return hints
}

func actionParameterTypeHintShouldUseExpandedType(rawType, expandedType string) bool {
	rawType = normalizeSourceTypeText(strings.TrimSpace(rawType))
	expandedType = normalizeSourceTypeText(strings.TrimSpace(expandedType))
	if rawType == "" {
		return false
	}
	if expandedType != "" &&
		expandedType != rawType &&
		strings.HasPrefix(expandedType, rawType+"<") &&
		strings.HasSuffix(expandedType, ">") {
		return true
	}
	if parameterTypeNeedsSourceRecovery(rawType) {
		return true
	}
	return simpleIndexedAccessTypePattern.MatchString(rawType)
}

func expandSourceParameterHintTypeText(source, file, typeText string, state *buildState) string {
	indexedExpanded := false
	if expanded := expandIndexedAccessesInTypeText(source, file, typeText, state); expanded != "" {
		typeText = expanded
		indexedExpanded = true
	}
	if specialized := specializeLocalTypeTextWithDefaultArguments(source, typeText); specialized != "" {
		return specialized
	}
	if expanded := expandLocalSourceTypeText(source, typeText); expanded != "" {
		return expanded
	}
	if specialized := specializeImportedTypeTextWithDefaultArguments(source, file, typeText, state); specialized != "" {
		return specialized
	}
	if indexedExpanded {
		return typeText
	}
	return ""
}

func expandIndexedAccessesInTypeText(source, file, typeText string, state *buildState) string {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return ""
	}

	changed := false
	expanded := simpleIndexedAccessTypePattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := simpleIndexedAccessTypePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		resolved := indexedAccessPropertyType(source, file, parts[1], parts[2], state)
		if resolved == "" {
			return match
		}
		changed = true
		return resolved
	})
	if !changed {
		return ""
	}
	return normalizeSourceTypeText(expanded)
}

func indexedAccessPropertyType(source, file, baseType, propertyName string, state *buildState) string {
	baseTypeText := resolveIndexedAccessBaseType(source, file, baseType, state)
	if baseTypeText == "" {
		return ""
	}
	members, ok := parseObjectTypeMembersWithOptionalUndefined(baseTypeText, true)
	if !ok {
		return ""
	}
	propertyType := normalizeSourceTypeText(strings.TrimSpace(members[propertyName]))
	if propertyType == "" {
		return ""
	}
	if expanded := expandIndexedAccessesInTypeText(source, file, propertyType, state); expanded != "" {
		return expanded
	}
	return propertyType
}

func resolveIndexedAccessBaseType(source, file, baseType string, state *buildState) string {
	if expanded := expandLocalSourceTypeText(source, baseType); expanded != "" {
		return expanded
	}
	if file == "" || state == nil {
		return ""
	}
	identifier, ok := sourceIdentifierExpression(baseType)
	if !ok {
		return ""
	}
	candidate, ok := parseNamedValueImports(source)[identifier]
	if !ok || candidate.ImportedName == "" || candidate.ImportedName == "default" {
		return ""
	}
	resolvedFile, ok := resolveImportFile(file, candidate.Path, state)
	if !ok {
		return ""
	}
	content, err := os.ReadFile(resolvedFile)
	if err != nil {
		return ""
	}
	return expandLocalSourceTypeText(string(content), candidate.ImportedName)
}

func specializeLocalTypeTextWithDefaultArguments(source, typeText string) string {
	identifier, ok := sourceIdentifierExpression(typeText)
	if !ok {
		return ""
	}
	return instantiatedTypeTextWithDefaultArguments(source, identifier)
}

func specializeImportedTypeTextWithDefaultArguments(source, file, typeText string, state *buildState) string {
	if file == "" || state == nil {
		return ""
	}
	identifier, ok := sourceIdentifierExpression(typeText)
	if !ok {
		return ""
	}
	candidate, ok := parseNamedValueImports(source)[identifier]
	if !ok || candidate.ImportedName == "" || candidate.ImportedName == "default" {
		return ""
	}
	resolvedFile, ok := resolveImportFile(file, candidate.Path, state)
	if !ok {
		return ""
	}
	content, err := os.ReadFile(resolvedFile)
	if err != nil {
		return ""
	}
	return instantiatedTypeTextWithDefaultArguments(string(content), candidate.ImportedName)
}

func instantiatedTypeTextWithDefaultArguments(source, name string) string {
	parameters := localTypeParameterListText(source, name)
	if parameters == "" {
		return ""
	}
	parts, err := splitTopLevelList(parameters)
	if err != nil || len(parts) == 0 {
		return ""
	}

	defaults := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return ""
		}
		index := findTopLevelParameterDefault(part)
		if index == -1 {
			return ""
		}
		defaultType := normalizeSourceTypeText(strings.TrimSpace(part[index+1:]))
		if defaultType == "" {
			return ""
		}
		defaults = append(defaults, defaultType)
	}
	if len(defaults) == 0 {
		return ""
	}
	return name + "<" + strings.Join(defaults, ", ") + ">"
}

func localTypeParameterListText(source, name string) string {
	headerPattern := regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:declare\s+)?(?:interface|type)\s+` + regexp.QuoteMeta(name) + `\b`)
	matches := headerPattern.FindAllStringIndex(source, -1)
	for _, match := range matches {
		start := skipTrivia(source, match[1])
		if start >= len(source) || source[start] != '<' {
			continue
		}
		end, err := findMatching(source, start, '<', '>')
		if err != nil || end <= start+1 {
			continue
		}
		return source[start+1 : end]
	}
	return ""
}

func localTypeParameterDefaults(source, name string) map[string]string {
	parameters := localTypeParameterListText(source, name)
	if parameters == "" {
		return nil
	}
	parts, err := splitTopLevelList(parameters)
	if err != nil || len(parts) == 0 {
		return nil
	}

	defaults := map[string]string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil
		}
		index := findTopLevelParameterDefault(part)
		if index == -1 {
			return nil
		}
		parameterName := parseTypeParameterName(part[:index])
		defaultType := normalizeSourceTypeText(strings.TrimSpace(part[index+1:]))
		if parameterName == "" || defaultType == "" {
			return nil
		}
		defaults[parameterName] = defaultType
	}
	if len(defaults) == 0 {
		return nil
	}
	return defaults
}

func parseTypeParameterName(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for index := 0; index < len(text); index++ {
		if !isIdentifierPart(text[index]) {
			if index == 0 {
				return ""
			}
			return text[:index]
		}
	}
	return text
}

func parameterDeclarationIsOptional(text string) bool {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "...")
	if index := strings.Index(text, "="); index != -1 {
		text = strings.TrimSpace(text[:index])
	}
	if index := strings.Index(text, ":"); index != -1 {
		text = strings.TrimSpace(text[:index])
	}
	return strings.HasSuffix(text, "?")
}

func sourceParameterTypeHintsWithDefault(source, file, parameters, defaultType string, state *buildState) map[string]string {
	return sourceParameterTypeHintsWithDefaults(source, file, parameters, []string{defaultType}, state)
}

func sourceParameterTypeHintsWithDefaults(source, file, parameters string, defaultTypes []string, state *buildState) map[string]string {
	hints := sourceParameterTypeHints(source, parameters)
	if len(defaultTypes) == 0 {
		return hints
	}

	text := strings.TrimSpace(parameters)
	if len(text) < 2 || text[0] != '(' || text[len(text)-1] != ')' {
		return hints
	}
	parts, err := splitTopLevelList(text[1 : len(text)-1])
	if err != nil || len(parts) == 0 {
		return hints
	}

	for index, part := range parts {
		if index >= len(defaultTypes) {
			break
		}
		hints = mergeDefaultSourceParameterTypeHints(hints, source, file, part, defaultTypes[index], state)
	}
	return hints
}

func mergeDefaultSourceParameterTypeHints(
	hints map[string]string,
	source,
	file,
	parameterText,
	defaultType string,
	state *buildState,
) map[string]string {
	defaultType = normalizeSourceTypeText(strings.TrimSpace(defaultType))
	if defaultType == "" || isAnyLikeType(defaultType) {
		return hints
	}

	parameterText = strings.TrimSpace(parameterText)
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

	destructured := sourceDestructuredParameterTypeHints(source, file, parameterText, defaultType, state)
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

func sourceDestructuredParameterTypeHints(source, file, parameterText, defaultType string, state *buildState) map[string]string {
	text := strings.TrimSpace(parameterText)
	if text == "" || text[0] != '{' {
		return nil
	}
	end, err := findMatching(text, 0, '{', '}')
	if err != nil || trimExpressionEnd(text, end+1) != len(text) {
		return nil
	}

	expandedType := defaultType
	expandedSource := source
	expandedFile := file
	if expanded := expandLocalSourceTypeText(source, defaultType); expanded != "" {
		expandedType = normalizeSourceTypeText(expanded)
	} else if importedSource, importedFile, expanded := expandImportedTypeAliasSourceWithContext(source, file, defaultType, state); expanded != "" {
		expandedSource = importedSource
		expandedFile = importedFile
		expandedType = normalizeSourceTypeText(expanded)
	}
	members, ok := parseObjectTypeMembersWithOptionalUndefined(expandedType, true)
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
		typeText = normalizeSourceTypeText(typeText)
		if expanded := expandSourceParameterHintTypeText(expandedSource, expandedFile, typeText, state); expanded != "" {
			typeText = normalizeSourceTypeText(expanded)
		}
		hints[name] = typeText
	}
	if len(hints) == 0 {
		return nil
	}
	return hints
}

func sourceObjectLiteralTypeTextWithHints(source, expression string, hints map[string]string) string {
	return sourceObjectLiteralTypeTextWithHintsOptions(source, expression, hints, false)
}

func sourceObjectLiteralTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
	return sourceObjectLiteralTypeTextWithContextOptions(source, file, expression, hints, state, false)
}

func sourceObjectLiteralTypeTextWithHintsOptions(
	source, expression string,
	hints map[string]string,
	preserveLiteralProperties bool,
) string {
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
					properties[shorthand] = normalizeSourceObjectPropertyType(valueType, preserveLiteralProperties)
				}
			}
			continue
		}
		valueType := sourceExpressionTypeTextWithHints(source, value, hints)
		if valueType == "" {
			continue
		}
		properties[name] = normalizeSourceObjectPropertyType(valueType, preserveLiteralProperties)
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

func sourceObjectLiteralTypeTextWithContextOptions(
	source,
	file,
	expression string,
	hints map[string]string,
	state *buildState,
	preserveLiteralProperties bool,
) string {
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
			spreadType := sourceExpressionTypeTextWithContext(source, file, strings.TrimSpace(text[3:]), hints, state)
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
				valueType := sourceExpressionTypeTextWithContext(source, file, shorthand, hints, state)
				if valueType != "" {
					properties[shorthand] = normalizeSourceObjectPropertyType(valueType, preserveLiteralProperties)
				}
			}
			continue
		}
		valueType := sourceExpressionTypeTextWithContext(source, file, value, hints, state)
		if valueType == "" {
			continue
		}
		properties[name] = normalizeSourceObjectPropertyType(valueType, preserveLiteralProperties)
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
	if asserted := sourceAssertedType(text); asserted != "" {
		return normalizeSourceTypeText(asserted)
	}
	if objectType := sourceObjectLiteralTypeTextWithHints(source, text, hints); objectType != "" {
		return objectType
	}
	if conditionalType := sourceConditionalExpressionTypeTextWithHints(source, text, hints); conditionalType != "" {
		return conditionalType
	}
	if logicalType := sourceLogicalExpressionTypeTextWithHints(source, text, hints); logicalType != "" {
		return logicalType
	}
	if comparisonType := sourceComparisonExpressionTypeText(text); comparisonType != "" {
		return comparisonType
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
	if awaitedType := sourceAwaitExpressionTypeTextWithContext(source, file, text, hints, state); awaitedType != "" {
		return awaitedType
	}
	if functionType := sourceArrowFunctionTypeTextWithOuterHints(source, file, text, hints, state); functionType != "" {
		return functionType
	}
	if callType := sourceCallExpressionTypeTextWithContext(source, file, text, hints, state); callType != "" {
		return callType
	}
	if arrayType := sourceArrayLiteralTypeTextWithContext(source, file, text, hints, state); arrayType != "" {
		return arrayType
	}
	if objectType := sourceObjectLiteralTypeTextWithContext(source, file, text, hints, state); objectType != "" {
		return objectType
	}
	if conditionalType := sourceConditionalExpressionTypeTextWithContext(source, file, text, hints, state); conditionalType != "" {
		return conditionalType
	}
	if logicalType := sourceLogicalExpressionTypeTextWithContext(source, file, text, hints, state); logicalType != "" {
		return logicalType
	}
	if binaryType := sourceBinaryExpressionTypeTextWithContext(source, file, text, hints, state); binaryType != "" {
		return binaryType
	}
	if comparisonType := sourceComparisonExpressionTypeText(text); comparisonType != "" {
		return comparisonType
	}
	if memberType := sourceMemberAccessTypeTextWithContextOptions(source, file, text, hints, state, true); memberType != "" {
		return memberType
	}
	if commonType := sourceCommonReturnExpressionType(text); commonType != "" {
		return commonType
	}
	if asserted := sourceAssertedType(text); asserted != "" {
		return normalizeSourceTypeText(asserted)
	}
	if literalType := sourceLiteralTypeText(text); literalType != "" {
		return literalType
	}
	if newType := sourceNewExpressionTypeTextWithContext(source, file, text, hints, state); newType != "" {
		return newType
	}
	if identifier, ok := sourceIdentifierExpression(text); ok {
		if hinted := strings.TrimSpace(hints[identifier]); hinted != "" {
			if expanded := expandLocalSourceTypeText(source, hinted); expanded != "" {
				return normalizeSourceTypeText(expanded)
			}
			return normalizeSourceTypeText(hinted)
		}
		inferredFromInitializer := ""
		initializerShouldStayOpaque := sourceLocalValueInitializerShouldStayOpaque(source, identifier)
		if initializer := findLocalValueInitializer(source, identifier); initializer != "" {
			if inferred := sourceExpressionTypeTextWithContext(source, file, initializer, hints, state); inferred != "" {
				inferredFromInitializer = inferred
				if !isAnyLikeType(inferred) {
					return inferred
				}
			}
		}
		if declared := findLocalValueDeclaredType(source, identifier); declared != "" {
			return declared
		}
		if !initializerShouldStayOpaque {
			if probed := sourceLocalValueTypeFromTypeProbe(source, file, identifier, state); probed != "" &&
				!isAnyLikeType(probed) &&
				!typeTextContainsStandaloneToken(probed, "any") &&
				!typeTextContainsStandaloneToken(probed, "unknown") {
				return probed
			}
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

func sourceLocalValueTypeFromTypeProbe(source, file, identifier string, state *buildState) string {
	if state == nil || file == "" || identifier == "" {
		return ""
	}
	match, ok := findLocalValueMatch(source, identifier)
	if !ok {
		return ""
	}
	if err := state.ensureAPIClient(); err != nil {
		return ""
	}
	projectID, err := state.projectIDForFile(file)
	if err != nil {
		return ""
	}

	probePositions := []int{match.IdentifierStart}
	probePositions = append(probePositions, selectorTypeProbePositions(source, match.InitializerStart, match.InitializerEnd)...)

	fallback := ""
	for _, position := range probePositions {
		typeText := normalizeSourceTypeText(state.cachedTypeAtPositionString(projectID, file, position))
		if typeText == "" {
			continue
		}
		if !isAnyLikeType(typeText) && !typeTextContainsStandaloneToken(typeText, "any") && !typeTextContainsStandaloneToken(typeText, "unknown") {
			return typeText
		}
		if fallback == "" {
			fallback = typeText
		}
	}
	return fallback
}

func sourceArrowFunctionTypeTextWithContext(source, file, expression string, state *buildState) string {
	return sourceArrowFunctionTypeTextWithOuterHints(source, file, expression, nil, state)
}

func sourceArrowFunctionTypeTextWithOuterHints(source, file, expression string, hints map[string]string, state *buildState) string {
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return ""
	}
	returnType := info.ExplicitReturn
	if returnType == "" {
		if inferred := sourceReturnExpressionTypeWithContext(source, file, info.Body, info.BlockBody, info.ParameterNames, nil, info.Async, state); inferred != "" {
			returnType = normalizeInferredTypeText(inferred)
			if info.Async {
				returnType = unwrapPromiseType(returnType)
			}
		} else if len(hints) > 0 {
			bodyHints := mergeSourceTypeHints(hints, sourceParameterTypeHintsWithContext(source, file, info.Parameters, state))
			body := strings.TrimSpace(info.Body)
			if info.BlockBody {
				body = singleReturnExpression(body)
				if body == "" {
					body = blockReturnExpression(info.Body)
				}
			}
			if body != "" {
				if inferred := sourceExpressionTypeTextWithContext(source, file, body, bodyHints, state); inferred != "" {
					returnType = normalizeInferredTypeText(inferred)
					if info.Async {
						returnType = unwrapPromiseType(returnType)
					}
				}
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

func mergeSourceTypeHints(base, extra map[string]string) map[string]string {
	if len(base) == 0 {
		return extra
	}
	if len(extra) == 0 {
		return base
	}
	merged := make(map[string]string, len(base)+len(extra))
	for name, typeText := range base {
		merged[name] = typeText
	}
	for name, typeText := range extra {
		if strings.TrimSpace(typeText) == "" {
			continue
		}
		merged[name] = typeText
	}
	return merged
}

func sourceAwaitExpressionTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if !strings.HasPrefix(text, "await ") {
		return ""
	}
	inner := strings.TrimSpace(text[len("await "):])
	if inner == "" {
		return ""
	}
	innerType := sourceExpressionTypeTextWithContext(source, file, inner, hints, state)
	if innerType == "" {
		return ""
	}
	return normalizeInferredTypeText(unwrapPromiseType(innerType))
}

func sourceLocalValueInitializerShouldStayOpaque(source, identifier string) bool {
	if source == "" || identifier == "" {
		return false
	}
	initializer := trimLeadingSourceTrivia(findLocalValueInitializer(source, identifier))
	if initializer == "" {
		return false
	}
	return sourceExpressionIsAwaitedJSONCall(initializer)
}

func sourceExpressionIsAwaitedJSONCall(expression string) bool {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if !strings.HasPrefix(text, "await ") {
		return false
	}
	callee, ok := stripTrailingCallExpression(strings.TrimSpace(text[len("await "):]))
	if !ok {
		return false
	}
	return strings.HasSuffix(strings.TrimSpace(callee), ".json")
}

func sourceCallExpressionTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	callee, ok := stripTrailingCallExpression(text)
	if !ok {
		return ""
	}
	if arrayMethodType := sourceArrayMethodCallType(source, file, text, callee, hints, state); arrayMethodType != "" {
		return arrayMethodType
	}
	if builtInType := sourceBuiltInCallExpressionTypeTextWithContext(source, file, text, callee, hints, state); builtInType != "" {
		return builtInType
	}
	if explicitReturnType := sourceExplicitGenericCallTypeText(source, callee); explicitReturnType != "" {
		return explicitReturnType
	}
	if identifier, ok := sourceIdentifierExpression(callee); ok {
		if returnType := sourceFunctionReturnTypeWithContext(source, file, identifier, state); returnType != "" {
			return normalizeInferredTypeText(returnType)
		}
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

func sourceArrayMethodCallType(source, file, expression, callee string, hints map[string]string, state *buildState) string {
	resolveReceiverType := func(suffixes ...string) string {
		for _, suffix := range suffixes {
			if !strings.HasSuffix(callee, suffix) {
				continue
			}
			receiverExpression := strings.TrimSpace(callee[:len(callee)-len(suffix)])
			if receiverExpression == "" || receiverExpression == expression {
				continue
			}
			receiverType := sourceExpressionTypeTextWithContext(source, file, receiverExpression, hints, state)
			return normalizeInferredTypeText(normalizeInternalHelperParameterType(receiverType))
		}
		return ""
	}

	for _, method := range []string{"sort", "reverse", "slice", "filter", "toSorted"} {
		receiverType := resolveReceiverType("?."+method, "."+method)
		if !isArrayLikeType(receiverType) {
			continue
		}
		return receiverType
	}

	if receiverType := resolveReceiverType("?.map", ".map"); receiverType != "" {
		if !isArrayLikeType(receiverType) {
			return ""
		}
		elementType, ok := parseNormalizedArrayElementType(receiverType)
		if !ok || elementType == "" {
			return ""
		}
		arguments, ok := trailingCallArguments(expression)
		if !ok || len(arguments) == 0 {
			return ""
		}
		returnType := sourceArrayMapCallbackReturnType(source, file, arguments[0], elementType, receiverType, state)
		if returnType == "" {
			return ""
		}
		return arrayTypeText(returnType)
	}

	if receiverType := resolveReceiverType("?.find", ".find"); receiverType != "" {
		elementType, ok := parseNormalizedArrayElementType(receiverType)
		if !ok || elementType == "" {
			return ""
		}
		return mergeNormalizedTypeUnion(elementType, "undefined")
	}

	for _, method := range []string{"some", "every"} {
		receiverType := resolveReceiverType("?."+method, "."+method)
		if !isArrayLikeType(receiverType) {
			continue
		}
		return "boolean"
	}

	return ""
}

func sourceBuiltInCallExpressionTypeTextWithContext(source, file, expression, callee string, hints map[string]string, state *buildState) string {
	if numericType := sourceNumericBuiltInCallExpressionTypeText(callee); numericType != "" {
		return numericType
	}
	if strings.HasSuffix(callee, ".split") {
		receiverExpression := strings.TrimSpace(callee[:len(callee)-len(".split")])
		receiverType := normalizeInferredTypeText(sourceExpressionTypeTextWithContext(source, file, receiverExpression, hints, state))
		if receiverType == "string" || isQuotedString(receiverType) || selectorTypeIsLiteralUnionOfKind(receiverType, "string") {
			return "string[]"
		}
	}
	switch strings.TrimSpace(callee) {
	case "Object.keys":
		return "string[]"
	case "Object.fromEntries":
		arguments, ok := trailingCallArguments(expression)
		if !ok || len(arguments) != 1 {
			return ""
		}
		collectionType := sourceCollectionExpressionTypeTextWithContext(source, file, strings.TrimSpace(arguments[0]), hints, state)
		elementType := collectionElementType(collectionType)
		keyType, valueType, ok := tupleKeyValueTypes(elementType)
		if !ok {
			return ""
		}
		keyType = normalizeObjectFromEntriesKeyType(keyType)
		if keyType == "" || valueType == "" {
			return ""
		}
		if normalizeSourceTypeText(strings.TrimSpace(valueType)) == "true" {
			return normalizeSourceTypeText(fmt.Sprintf("{ [k: %s]: true; }", keyType))
		}
		return normalizeSourceTypeText(fmt.Sprintf("Record<%s, %s>", keyType, valueType))
	default:
		return ""
	}
}

func sourceNumericBuiltInCallExpressionTypeText(callee string) string {
	switch strings.TrimSpace(callee) {
	case "Date.now",
		"Date.parse",
		"Math.abs",
		"Math.acos",
		"Math.acosh",
		"Math.asin",
		"Math.asinh",
		"Math.atan",
		"Math.atan2",
		"Math.atanh",
		"Math.cbrt",
		"Math.ceil",
		"Math.clz32",
		"Math.cos",
		"Math.cosh",
		"Math.exp",
		"Math.expm1",
		"Math.floor",
		"Math.fround",
		"Math.hypot",
		"Math.imul",
		"Math.log",
		"Math.log1p",
		"Math.log2",
		"Math.log10",
		"Math.max",
		"Math.min",
		"Math.pow",
		"Math.random",
		"Math.round",
		"Math.sign",
		"Math.sin",
		"Math.sinh",
		"Math.sqrt",
		"Math.tan",
		"Math.tanh",
		"Math.trunc",
		"Number",
		"Number.parseFloat",
		"Number.parseInt",
		"parseFloat",
		"parseInt":
		return "number"
	default:
		return ""
	}
}

func normalizeObjectFromEntriesKeyType(typeText string) string {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return ""
	}
	if text == "number" || text == "string" || isNumericLiteralType(text) || isQuotedString(text) || selectorTypeIsLiteralUnionOfKind(text, "string") {
		return "string"
	}
	return text
}

func sourceArrayMapCallbackReturnType(source, file, callbackExpression, elementType, receiverType string, state *buildState) string {
	info, ok := parseSourceArrowInfo(strings.TrimSpace(callbackExpression))
	if !ok {
		return ""
	}
	dependencyTypes := []string{elementType, "number", receiverType}
	if tupleType := sourceTupleReturnTypeWithContext(source, file, info.Body, info.BlockBody, info.ParameterNames, dependencyTypes, state); tupleType != "" {
		return tupleType
	}
	return sourceReturnExpressionTypeWithContext(source, file, info.Body, info.BlockBody, info.ParameterNames, dependencyTypes, info.Async, state)
}

func sourceTupleReturnTypeWithContext(source, file, body string, blockBody bool, parameterNames, dependencyTypes []string, state *buildState) string {
	expression := strings.TrimSpace(body)
	if blockBody {
		expression = singleReturnExpression(body)
		if expression == "" {
			expression = blockReturnExpression(body)
		}
	}
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return ""
	}
	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(expression, 0, len(expression))
	if err != nil || !ok || arrayStart != 0 || arrayEnd != len(expression)-1 {
		return ""
	}
	parts, err := splitTopLevelList(expression[arrayStart+1 : arrayEnd])
	if err != nil || len(parts) != 2 {
		return ""
	}
	for _, part := range parts {
		if strings.HasPrefix(strings.TrimSpace(part), "...") {
			return ""
		}
	}
	hints := sourceReturnExpressionTypeHintsWithContext(source, file, parameterNames, dependencyTypes, state)
	keyType := normalizeSourceTypeText(sourceExpressionTypeTextWithContext(source, file, strings.TrimSpace(parts[0]), hints, state))
	valueType := normalizeSourceTypeText(sourceExpressionTypeTextWithContext(source, file, strings.TrimSpace(parts[1]), hints, state))
	if keyType == "" || valueType == "" {
		return ""
	}
	return normalizeSourceTypeText("[" + keyType + ", " + valueType + "]")
}

func sourceExplicitGenericCallTypeText(source, callee string) string {
	baseCallee, typeArguments, ok := stripTrailingTypeArguments(callee)
	if !ok {
		return ""
	}
	typeParts, err := splitTopLevelList(typeArguments)
	if err != nil || len(typeParts) == 0 {
		return ""
	}
	rootAlias, methodName, ok := sourceCallRootAliasAndMethod(baseCallee)
	if !ok || !isLikelyGenericAPIHelperCall(source, rootAlias, methodName) {
		return ""
	}
	responseType := normalizeSourceTypeText(strings.TrimSpace(typeParts[0]))
	if responseType == "" {
		return ""
	}
	return promiseTypeText(responseType)
}

func stripTrailingTypeArguments(text string) (string, string, bool) {
	text = strings.TrimSpace(text)
	if text == "" || !strings.HasSuffix(text, ">") {
		return "", "", false
	}

	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0
	typeArgumentStart := -1
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\'':
			end, err := skipQuoted(text, i, '\'')
			if err != nil {
				return "", "", false
			}
			i = end
		case '"':
			end, err := skipQuoted(text, i, '"')
			if err != nil {
				return "", "", false
			}
			i = end
		case '`':
			end, err := skipTemplate(text, i)
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
			if shouldOpenAngle(text, i) {
				angleDepth++
				if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 1 {
					typeArgumentStart = i
				}
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
				if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 && i == len(text)-1 && typeArgumentStart != -1 {
					base := strings.TrimSpace(text[:typeArgumentStart])
					args := strings.TrimSpace(text[typeArgumentStart+1 : i])
					if base == "" || args == "" {
						return "", "", false
					}
					return base, args, true
				}
			}
		}
	}
	return "", "", false
}

func sourceCallRootAliasAndMethod(callee string) (string, string, bool) {
	segments, _, end, ok := parseMemberExpressionSegments(callee, 0, len(callee))
	if !ok || end != len(callee) || len(segments) < 2 {
		return "", "", false
	}
	return segments[0], segments[len(segments)-1], true
}

func isLikelyGenericAPIHelperCall(source, rootAlias, methodName string) bool {
	switch methodName {
	case "get", "put", "update", "create", "post", "patch", "delete":
	default:
		return false
	}
	if candidate, ok := parseDefaultValueImports(source)[rootAlias]; ok {
		return isLikelyAPIDefaultImportPath(candidate.Path)
	}
	return rootAlias == "api"
}

func isLikelyAPIDefaultImportPath(importPath string) bool {
	importPath = strings.TrimSpace(importPath)
	switch importPath {
	case "lib/api", "~/lib/api":
		return true
	}
	return strings.HasSuffix(importPath, "/lib/api")
}

func sourceConditionalExpressionTypeTextWithHints(source, expression string, hints map[string]string) string {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	questionIndex, colonIndex, ok := findTopLevelConditional(text)
	if !ok {
		return ""
	}
	trueType := sourceExpressionTypeTextWithHints(source, strings.TrimSpace(text[questionIndex+1:colonIndex]), hints)
	falseType := sourceExpressionTypeTextWithHints(source, strings.TrimSpace(text[colonIndex+1:]), hints)
	return mergeConditionalOperandTypes(trueType, falseType)
}

func sourceConditionalExpressionTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	questionIndex, colonIndex, ok := findTopLevelConditional(text)
	if !ok {
		return ""
	}
	trueType := sourceExpressionTypeTextWithContext(source, file, strings.TrimSpace(text[questionIndex+1:colonIndex]), hints, state)
	falseType := sourceExpressionTypeTextWithContext(source, file, strings.TrimSpace(text[colonIndex+1:]), hints, state)
	return mergeConditionalOperandTypes(trueType, falseType)
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

func sourceLogicalExpressionTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" {
		return ""
	}
	for _, operator := range []string{"??", "||"} {
		index := findLastTopLevelOperator(text, operator)
		if index == -1 {
			continue
		}
		left := sourceExpressionTypeTextWithContext(source, file, strings.TrimSpace(text[:index]), hints, state)
		right := sourceExpressionTypeTextWithContext(source, file, strings.TrimSpace(text[index+len(operator):]), hints, state)
		if merged := mergeLogicalOperandTypes(left, right, operator); merged != "" {
			return merged
		}
	}
	return ""
}

func sourceBinaryExpressionTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" {
		return ""
	}
	for _, operator := range []string{"*", "/", "%", "-", "+"} {
		index := findLastTopLevelOperator(text, operator)
		if index == -1 {
			continue
		}
		left := normalizeInferredTypeText(sourceExpressionTypeTextWithContext(source, file, strings.TrimSpace(text[:index]), hints, state))
		right := normalizeInferredTypeText(sourceExpressionTypeTextWithContext(source, file, strings.TrimSpace(text[index+len(operator):]), hints, state))
		if left == "" || right == "" {
			continue
		}
		if operator == "+" && (sourceExpressionTypeLooksStringLike(left) || sourceExpressionTypeLooksStringLike(right)) {
			return "string"
		}
		if sourceExpressionTypeLooksNumeric(left) && sourceExpressionTypeLooksNumeric(right) {
			return "number"
		}
	}
	return ""
}

func sourceExpressionTypeLooksNumeric(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	return text == "number" || isNumericLiteralType(text) || selectorTypeIsLiteralUnionOfKind(text, "number")
}

func sourceExpressionTypeLooksStringLike(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	return text == "string" || isQuotedString(text) || selectorTypeIsLiteralUnionOfKind(text, "string")
}

func sourceComparisonExpressionTypeText(expression string) string {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" {
		return ""
	}
	for _, operator := range []string{"!==", "===", "!=", "=="} {
		index := findLastTopLevelOperator(text, operator)
		if index == -1 {
			continue
		}
		left := strings.TrimSpace(text[:index])
		right := strings.TrimSpace(text[index+len(operator):])
		if left == "" || right == "" {
			continue
		}
		return "boolean"
	}
	return ""
}

func mergeLogicalOperandTypes(left, right, operator string) string {
	rawLeft := normalizeSourceTypeText(strings.TrimSpace(left))
	left = normalizeSourceTypeText(strings.TrimSpace(left))
	right = normalizeSourceTypeText(strings.TrimSpace(right))

	switch operator {
	case "??":
		left = normalizeInternalHelperParameterType(left)
	case "||":
		left = normalizeInternalHelperParameterType(left)
	}
	if (operator == "??" || operator == "||") && right == "any[]" {
		normalizedLeft := normalizeInternalHelperParameterType(rawLeft)
		if isArrayLikeType(normalizedLeft) {
			return normalizedLeft
		}
	}

	switch {
	case left == "":
		if sourceFallbackLiteralShouldDefer(right) {
			return ""
		}
		return right
	case right == "":
		return left
	case left == right:
		return left
	}
	if logicalFallbackLiteralCanDeferToConcreteLeft(rawLeft, left, right) {
		return left
	}

	if right == "string" || isQuotedString(right) {
		if normalizedUnionContainsType(left, "string") {
			return mergeNormalizedTypeUnion(left, "string")
		}
	}
	if right == "number" || isNumericLiteralType(right) {
		if normalizedUnionContainsType(left, "number") {
			return mergeNormalizedTypeUnion(left, "number")
		}
	}

	return mergeNormalizedTypeUnion(left, right)
}

func logicalFallbackLiteralCanDeferToConcreteLeft(rawLeft, left, right string) bool {
	right = normalizeSourceTypeText(strings.TrimSpace(right))
	if right == "any[]" && isArrayLikeType(left) && !normalizedUnionContainsType(rawLeft, "null") && !normalizedUnionContainsType(rawLeft, "undefined") {
		return true
	}
	if right != "null" && right != "undefined" {
		return false
	}
	if normalizedUnionContainsType(rawLeft, "null") || normalizedUnionContainsType(rawLeft, "undefined") {
		return false
	}
	if isAnyLikeType(left) || isLooselyTypedType(left) {
		return false
	}
	if selectorTypeIsLiteralUnionOfKind(left, "string") ||
		selectorTypeIsLiteralUnionOfKind(left, "number") ||
		selectorTypeIsLiteralUnionOfKind(left, "boolean") {
		return true
	}
	return true
}

func mergeConditionalOperandTypes(trueType, falseType string) string {
	trueType = normalizeSourceTypeText(strings.TrimSpace(trueType))
	falseType = normalizeSourceTypeText(strings.TrimSpace(falseType))

	if trueType == "any[]" && isArrayLikeType(falseType) {
		return falseType
	}
	if falseType == "any[]" && isArrayLikeType(trueType) {
		return trueType
	}

	switch {
	case trueType == "" && falseType == "":
		return ""
	case trueType == "":
		if sourceFallbackLiteralShouldDefer(falseType) {
			return ""
		}
		return falseType
	case falseType == "":
		if sourceFallbackLiteralShouldDefer(trueType) {
			return ""
		}
		return trueType
	case trueType == falseType:
		return trueType
	default:
		return mergeNormalizedTypeUnion(trueType, falseType)
	}
}

func sourceFallbackLiteralShouldDefer(typeText string) bool {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return false
	}
	return text == "null" ||
		text == "undefined" ||
		isQuotedString(text) ||
		isNumericLiteralType(text) ||
		isBooleanLiteralType(text)
}

func findTopLevelConditional(text string) (int, int, bool) {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0
	questionIndex := -1
	conditionalDepth := 0

	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\'':
			end, err := skipQuoted(text, i, '\'')
			if err != nil {
				return -1, -1, false
			}
			i = end
		case '"':
			end, err := skipQuoted(text, i, '"')
			if err != nil {
				return -1, -1, false
			}
			i = end
		case '`':
			end, err := skipTemplate(text, i)
			if err != nil {
				return -1, -1, false
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
		case '?':
			if parenDepth != 0 || bracketDepth != 0 || braceDepth != 0 || angleDepth != 0 {
				continue
			}
			if i > 0 && text[i-1] == '?' {
				continue
			}
			if i+1 < len(text) && (text[i+1] == '.' || text[i+1] == '?') {
				continue
			}
			if questionIndex == -1 {
				questionIndex = i
			}
			conditionalDepth++
		case ':':
			if parenDepth != 0 || bracketDepth != 0 || braceDepth != 0 || angleDepth != 0 || questionIndex == -1 {
				continue
			}
			conditionalDepth--
			if conditionalDepth == 0 {
				return questionIndex, i, true
			}
		}
	}

	return -1, -1, false
}

func normalizedUnionContainsType(typeText, target string) bool {
	parts, err := splitTopLevelUnion(typeText)
	if err != nil || len(parts) == 0 {
		return normalizeSourceTypeText(strings.TrimSpace(typeText)) == target
	}
	for _, part := range parts {
		if normalizeSourceTypeText(strings.TrimSpace(part)) == target {
			return true
		}
	}
	return false
}

func mergeNormalizedTypeUnion(left, right string) string {
	parts := make([]string, 0, 4)
	seen := map[string]bool{}
	appendParts := func(text string) {
		unionParts, err := splitTopLevelUnion(text)
		if err != nil || len(unionParts) == 0 {
			unionParts = []string{text}
		}
		for _, part := range unionParts {
			part = normalizeSourceTypeText(strings.TrimSpace(part))
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			parts = append(parts, part)
		}
	}

	appendParts(left)
	appendParts(right)
	if len(parts) == 0 {
		return ""
	}
	for _, part := range parts {
		if part == "any" {
			return "any"
		}
	}

	hasString := false
	hasNumber := false
	hasBoolean := false
	for _, part := range parts {
		switch part {
		case "string":
			hasString = true
		case "number":
			hasNumber = true
		case "boolean":
			hasBoolean = true
		}
	}

	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		switch {
		case hasString && isQuotedString(part):
			continue
		case hasNumber && isNumericLiteralType(part):
			continue
		case hasBoolean && isBooleanLiteralType(part):
			continue
		default:
			filtered = append(filtered, part)
		}
	}
	if len(filtered) == 0 {
		filtered = parts
	}
	filtered = simplifyRedundantWrapperUnionParts(filtered)
	filtered = simplifyRedundantObjectArrayUnionParts(filtered)
	return normalizeSourceTypeText(strings.Join(filtered, " | "))
}

func simplifyRedundantWrapperUnionParts(parts []string) []string {
	if len(parts) < 2 {
		return parts
	}

	filtered := make([]string, 0, len(parts))
	for index, part := range parts {
		redundant := false
		for otherIndex, other := range parts {
			if index == otherIndex {
				continue
			}
			if normalizedWrapperUnionSubsumes(other, part) {
				redundant = true
				break
			}
		}
		if !redundant {
			filtered = append(filtered, part)
		}
	}
	if len(filtered) == 0 {
		return parts
	}
	return filtered
}

func normalizedWrapperUnionSubsumes(container, candidate string) bool {
	container = normalizeSourceTypeText(strings.TrimSpace(container))
	candidate = normalizeSourceTypeText(strings.TrimSpace(candidate))
	if container == "" || candidate == "" || container == candidate {
		return false
	}

	for _, wrapper := range []string{"Partial", "DeepPartial", "Readonly"} {
		inner, ok := parseSingleGenericTypeArgument(container, wrapper)
		if !ok {
			continue
		}
		inner = normalizeSourceTypeText(strings.TrimSpace(inner))
		if inner == "" {
			continue
		}
		switch wrapper {
		case "Partial":
			if candidate == inner {
				return true
			}
		case "DeepPartial":
			if candidate == inner || candidate == "Partial<"+inner+">" {
				return true
			}
		case "Readonly":
			if candidate == inner {
				return true
			}
		}
	}
	return false
}

func simplifyRedundantObjectArrayUnionParts(parts []string) []string {
	if len(parts) < 2 {
		return parts
	}

	filtered := make([]string, 0, len(parts))
	for index, part := range parts {
		redundant := false
		for otherIndex, other := range parts {
			if index == otherIndex {
				continue
			}
			if normalizedObjectArrayUnionSubsumes(other, part) {
				redundant = true
				break
			}
		}
		if !redundant {
			filtered = append(filtered, part)
		}
	}
	if len(filtered) == 0 {
		return parts
	}
	return filtered
}

func normalizedObjectArrayUnionSubsumes(container, candidate string) bool {
	containerElement, ok := parseNormalizedArrayElementType(container)
	if !ok {
		return false
	}
	candidateElement, ok := parseNormalizedArrayElementType(candidate)
	if !ok {
		return false
	}

	containerParts, err := splitTopLevelUnion(containerElement)
	if err != nil || len(containerParts) < 2 {
		return false
	}
	candidateParts, err := splitTopLevelUnion(candidateElement)
	if err != nil || len(candidateParts) == 0 {
		candidateParts = []string{candidateElement}
	}

	allowed := map[string]bool{}
	for _, part := range containerParts {
		part = normalizeInlineObjectTypeText(strings.TrimSpace(part))
		if !isInlineObjectUnionPart(part) {
			return false
		}
		allowed[part] = true
	}
	for _, part := range candidateParts {
		part = normalizeInlineObjectTypeText(strings.TrimSpace(part))
		if !isInlineObjectUnionPart(part) || !allowed[part] {
			return false
		}
	}
	return true
}

func parseNormalizedArrayElementType(typeText string) (string, bool) {
	text := normalizeSourceTypeTextWithOptions(strings.TrimSpace(typeText), false)
	switch {
	case strings.HasSuffix(text, "[]"):
		element := strings.TrimSpace(text[:len(text)-2])
		element = strings.TrimSpace(unwrapWrappedExpression(element))
		if element == "" {
			return "", false
		}
		return normalizeSourceTypeTextWithOptions(element, false), true
	case strings.HasPrefix(text, "Array<") && strings.HasSuffix(text, ">"):
		element := strings.TrimSpace(text[len("Array<") : len(text)-1])
		if element == "" {
			return "", false
		}
		return normalizeSourceTypeTextWithOptions(element, false), true
	case strings.HasPrefix(text, "ReadonlyArray<") && strings.HasSuffix(text, ">"):
		element := strings.TrimSpace(text[len("ReadonlyArray<") : len(text)-1])
		if element == "" {
			return "", false
		}
		return normalizeSourceTypeTextWithOptions(element, false), true
	default:
		return "", false
	}
}

func isInlineObjectUnionPart(typeText string) bool {
	text := normalizeInlineObjectTypeText(strings.TrimSpace(typeText))
	if text == "" ||
		isAnyLikeType(text) ||
		typeTextContainsStandaloneToken(text, "any") ||
		typeTextContainsStandaloneToken(text, "unknown") {
		return false
	}
	_, ok := parseObjectTypeMembers(text)
	return ok
}

type sourceAccessSegment struct {
	Text     string
	Bracket  bool
	Optional bool
}

func sourceMemberAccessTypeTextWithHints(source, expression string, hints map[string]string) string {
	return sourceMemberAccessTypeTextWithContext(source, "", expression, hints, nil)
}

func sourceMemberAccessTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
	return sourceMemberAccessTypeTextWithContextOptions(source, file, expression, hints, state, false)
}

func sourceMemberAccessTypeTextWithContextOptions(
	source,
	file,
	expression string,
	hints map[string]string,
	state *buildState,
	allowImportedAliasExpansion bool,
) string {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" {
		return ""
	}
	root, path, ok := parseSourceAccessPath(text)
	if ok && root != "" && len(path) > 0 {
		rootType := strings.TrimSpace(hints[root])
		if rootType == "" {
			if declared := findLocalValueDeclaredType(source, root); declared != "" {
				rootType = declared
			}
		}
		if rootType == "" {
			if initializer := findLocalValueInitializer(source, root); initializer != "" {
				rootType = sourceExpressionTypeTextWithContext(source, file, initializer, hints, state)
			}
		}
		if rootType == "" {
			return ""
		}
		return sourceAccessPathTypeTextWithContextOptions(source, file, rootType, path, hints, state, allowImportedAliasExpansion)
	}

	baseExpression, path, ok := parseWrappedSourceAccessExpression(text)
	if !ok || len(path) == 0 {
		return ""
	}
	baseType := sourceExpressionTypeTextWithContext(source, file, baseExpression, hints, state)
	if baseType == "" {
		return ""
	}
	return sourceAccessPathTypeTextWithContextOptions(source, file, baseType, path, hints, state, allowImportedAliasExpansion)
}

func parseSourceAccessPath(expression string) (string, []sourceAccessSegment, bool) {
	text := strings.TrimSpace(expression)
	if text == "" {
		return "", nil, false
	}

	index := skipWhitespaceForward(text, 0)
	root, next, ok := readConnectedTargetIdentifier(text, index)
	if !ok {
		return "", nil, false
	}

	var segments []sourceAccessSegment
	index = next
	for {
		index = skipWhitespaceForward(text, index)
		if index >= len(text) {
			break
		}

		switch {
		case strings.HasPrefix(text[index:], "?.["):
			segment, next, ok := parseSourceIndexAccessSegment(text, index+2)
			if !ok {
				return "", nil, false
			}
			segment.Optional = true
			segments = append(segments, segment)
			index = next
		case text[index] == '[':
			segment, next, ok := parseSourceIndexAccessSegment(text, index)
			if !ok {
				return "", nil, false
			}
			segments = append(segments, segment)
			index = next
		case strings.HasPrefix(text[index:], "?."):
			index = skipWhitespaceForward(text, index+2)
			segment, next, ok := readConnectedTargetIdentifier(text, index)
			if !ok {
				return "", nil, false
			}
			segments = append(segments, sourceAccessSegment{Text: segment, Optional: true})
			index = next
		case text[index] == '.':
			index = skipWhitespaceForward(text, index+1)
			segment, next, ok := readConnectedTargetIdentifier(text, index)
			if !ok {
				return "", nil, false
			}
			segments = append(segments, sourceAccessSegment{Text: segment})
			index = next
		default:
			return "", nil, false
		}
	}

	if len(segments) == 0 {
		return "", nil, false
	}
	return root, segments, true
}

func parseSourceIndexAccessSegment(text string, start int) (sourceAccessSegment, int, bool) {
	if start >= len(text) || text[start] != '[' {
		return sourceAccessSegment{}, start, false
	}
	end, err := findMatching(text, start, '[', ']')
	if err != nil {
		return sourceAccessSegment{}, start, false
	}
	content := strings.TrimSpace(text[start+1 : end])
	if content == "" {
		return sourceAccessSegment{}, start, false
	}
	if isQuotedString(content) {
		content = unquoteString(content)
	}
	return sourceAccessSegment{Text: content, Bracket: true}, end + 1, true
}

func parseWrappedSourceAccessExpression(expression string) (string, []sourceAccessSegment, bool) {
	text := strings.TrimSpace(expression)
	if text == "" {
		return "", nil, false
	}
	start := skipWhitespaceForward(text, 0)
	if start >= len(text) || text[start] != '(' {
		return "", nil, false
	}
	end, err := findMatching(text, start, '(', ')')
	if err != nil || end <= start {
		return "", nil, false
	}
	base := strings.TrimSpace(text[start : end+1])
	path, ok := parseSourceAccessSuffix(text, end+1)
	if !ok || len(path) == 0 {
		return "", nil, false
	}
	return base, path, true
}

func parseSourceAccessSuffix(text string, start int) ([]sourceAccessSegment, bool) {
	var segments []sourceAccessSegment
	index := start
	for {
		index = skipWhitespaceForward(text, index)
		if index >= len(text) {
			break
		}

		switch {
		case strings.HasPrefix(text[index:], "?.["):
			segment, next, ok := parseSourceIndexAccessSegment(text, index+2)
			if !ok {
				return nil, false
			}
			segment.Optional = true
			segments = append(segments, segment)
			index = next
		case text[index] == '[':
			segment, next, ok := parseSourceIndexAccessSegment(text, index)
			if !ok {
				return nil, false
			}
			segments = append(segments, segment)
			index = next
		case strings.HasPrefix(text[index:], "?."):
			index = skipWhitespaceForward(text, index+2)
			segment, next, ok := readConnectedTargetIdentifier(text, index)
			if !ok {
				return nil, false
			}
			segments = append(segments, sourceAccessSegment{Text: segment, Optional: true})
			index = next
		case text[index] == '.':
			index = skipWhitespaceForward(text, index+1)
			segment, next, ok := readConnectedTargetIdentifier(text, index)
			if !ok {
				return nil, false
			}
			segments = append(segments, sourceAccessSegment{Text: segment})
			index = next
		default:
			return nil, false
		}
	}
	return segments, len(segments) > 0
}

func sourceAccessPathTypeTextWithContextOptions(
	source,
	file,
	typeText string,
	path []sourceAccessSegment,
	hints map[string]string,
	state *buildState,
	allowImportedAliasExpansion bool,
) string {
	current := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if current == "" {
		return ""
	}
	for _, segment := range path {
		nullableReceiver := normalizedUnionContainsType(current, "null") || normalizedUnionContainsType(current, "undefined")
		var next string
		if segment.Bracket {
			next = sourceIndexedPathSegmentTypeTextOptions(source, file, current, segment.Text, hints, state, allowImportedAliasExpansion)
		} else {
			next = sourceMemberPathSegmentTypeTextOptions(source, file, current, segment.Text, state, allowImportedAliasExpansion)
		}
		if next == "" {
			return ""
		}
		if segment.Optional && nullableReceiver {
			next = mergeNormalizedTypeUnion(next, "undefined")
		}
		current = next
	}
	return current
}

func parseSourceMemberAccessSegments(expression string) ([]string, bool) {
	if segments, ok := parseConnectedTargetSegments(strings.TrimSpace(expression)); ok && len(segments) >= 2 {
		return segments, true
	}

	segments, _, end, ok := parseMemberExpressionSegments(expression, 0, len(expression))
	if !ok || end != len(expression) || len(segments) < 2 {
		return nil, false
	}
	return segments, true
}

func sourceMemberPathTypeText(source, typeText string, path []string) string {
	return sourceMemberPathTypeTextWithContext(source, "", typeText, path, nil)
}

func sourceMemberPathTypeTextWithContext(source, file, typeText string, path []string, state *buildState) string {
	return sourceMemberPathTypeTextWithContextOptions(source, file, typeText, path, state, false)
}

func sourceMemberPathTypeTextWithContextOptions(
	source,
	file,
	typeText string,
	path []string,
	state *buildState,
	allowImportedAliasExpansion bool,
) string {
	current := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if current == "" {
		return ""
	}
	for _, segment := range path {
		next := sourceMemberPathSegmentTypeTextOptions(source, file, current, segment, state, allowImportedAliasExpansion)
		if next == "" {
			return ""
		}
		current = next
	}
	return current
}

func sourceMemberPathSegmentTypeText(source, file, typeText, segment string, state *buildState) string {
	return sourceMemberPathSegmentTypeTextOptions(source, file, typeText, segment, state, false)
}

func sourceMemberPathSegmentTypeTextOptions(
	source,
	file,
	typeText,
	segment string,
	state *buildState,
	allowImportedAliasExpansion bool,
) string {
	parts, err := splitTopLevelUnion(typeText)
	if err != nil || len(parts) == 0 {
		parts = []string{typeText}
	}

	resolved := ""
	for _, part := range parts {
		current := normalizeSourceTypeText(strings.TrimSpace(part))
		if current == "" || current == "null" || current == "undefined" {
			continue
		}
		if segment == "length" && (isArrayLikeType(current) || current == "string" || isQuotedString(current) || selectorTypeIsLiteralUnionOfKind(current, "string")) {
			if resolved == "" {
				resolved = "number"
			} else {
				resolved = mergeNormalizedTypeUnion(resolved, "number")
			}
			continue
		}
		if expanded := expandLocalSourceTypeText(source, current); expanded != "" {
			current = normalizeSourceTypeText(expanded)
		}
		if allowImportedAliasExpansion {
			if expanded := expandImportedTypeAliasTextWithContext(source, file, current, state); expanded != "" {
				current = normalizeSourceTypeText(expanded)
			}
		}
		if inner, ok := parseSingleGenericTypeArgument(current, "Partial"); ok {
			if next := sourceMemberPathSegmentTypeTextOptions(source, file, inner, segment, state, true); next != "" {
				next = mergeNormalizedTypeUnion(next, "undefined")
				if resolved == "" {
					resolved = next
				} else {
					resolved = mergeNormalizedTypeUnion(resolved, next)
				}
			}
			continue
		}
		if inner, ok := parseSingleGenericTypeArgument(current, "DeepPartial"); ok {
			if next := sourceMemberPathSegmentTypeTextOptions(source, file, inner, segment, state, true); next != "" {
				next = mergeNormalizedTypeUnion(next, "undefined")
				if resolved == "" {
					resolved = next
				} else {
					resolved = mergeNormalizedTypeUnion(resolved, next)
				}
			}
			continue
		}
		if inner, ok := parseSingleGenericTypeArgument(current, "Readonly"); ok {
			if next := sourceMemberPathSegmentTypeTextOptions(source, file, inner, segment, state, true); next != "" {
				if resolved == "" {
					resolved = next
				} else {
					resolved = mergeNormalizedTypeUnion(resolved, next)
				}
			}
			continue
		}
		members, ok := parseObjectTypeMembersWithOptionalUndefined(current, true)
		if !ok {
			continue
		}
		next, ok := members[segment]
		if !ok {
			continue
		}
		next = normalizeSourceTypeText(strings.TrimSpace(next))
		if next == "" {
			continue
		}
		if resolved == "" {
			resolved = next
			continue
		}
		resolved = mergeNormalizedTypeUnion(resolved, next)
	}
	return resolved
}

func sourceIndexedPathSegmentTypeTextOptions(
	source,
	file,
	typeText,
	indexExpression string,
	hints map[string]string,
	state *buildState,
	allowImportedAliasExpansion bool,
) string {
	parts, err := splitTopLevelUnion(typeText)
	if err != nil || len(parts) == 0 {
		parts = []string{typeText}
	}

	resolved := ""
	for _, part := range parts {
		current := normalizeSourceTypeText(strings.TrimSpace(part))
		if current == "" || current == "null" || current == "undefined" {
			continue
		}
		if expanded := expandLocalSourceTypeText(source, current); expanded != "" {
			current = normalizeSourceTypeText(expanded)
		}
		if allowImportedAliasExpansion {
			if expanded := expandImportedTypeAliasTextWithContext(source, file, current, state); expanded != "" {
				current = normalizeSourceTypeText(expanded)
			}
		}
		if inner, ok := parseSingleGenericTypeArgument(current, "Readonly"); ok {
			current = normalizeSourceTypeText(inner)
		}
		if element, ok := parseNormalizedArrayElementType(current); ok {
			if resolved == "" {
				resolved = element
			} else {
				resolved = mergeNormalizedTypeUnion(resolved, element)
			}
			continue
		}
		if _, valueType, ok := parseRecordTypeArguments(current); ok {
			if valueType == "" {
				continue
			}
			if resolved == "" {
				resolved = valueType
			} else {
				resolved = mergeNormalizedTypeUnion(resolved, valueType)
			}
			continue
		}
		if propertyName := sourceIndexExpressionPropertyName(indexExpression, hints); propertyName != "" {
			if next := sourceMemberPathSegmentTypeTextOptions(source, file, current, propertyName, state, allowImportedAliasExpansion); next != "" {
				if resolved == "" {
					resolved = next
				} else {
					resolved = mergeNormalizedTypeUnion(resolved, next)
				}
			}
		}
	}
	return resolved
}

func parseSingleGenericTypeArgument(typeText, genericName string) (string, bool) {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	prefix := genericName + "<"
	if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, ">") {
		return "", false
	}
	inner := strings.TrimSpace(text[len(prefix) : len(text)-1])
	if inner == "" {
		return "", false
	}
	parts, err := splitTopLevelList(inner)
	if err != nil || len(parts) != 1 {
		return "", false
	}
	inner = normalizeSourceTypeText(strings.TrimSpace(parts[0]))
	return inner, inner != ""
}

func parseRecordTypeArguments(typeText string) (string, string, bool) {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if inner, ok := parseSingleGenericTypeArgument(text, "Readonly"); ok {
		return parseRecordTypeArguments(inner)
	}
	if !strings.HasPrefix(text, "Record<") || !strings.HasSuffix(text, ">") {
		return "", "", false
	}
	parts, err := splitTopLevelList(text[len("Record<") : len(text)-1])
	if err != nil || len(parts) != 2 {
		return "", "", false
	}
	keyType := normalizeSourceTypeText(strings.TrimSpace(parts[0]))
	valueType := normalizeSourceTypeText(strings.TrimSpace(parts[1]))
	if keyType == "" || valueType == "" {
		return "", "", false
	}
	return keyType, valueType, true
}

func sourceIndexExpressionPropertyName(expression string, hints map[string]string) string {
	text := strings.TrimSpace(expression)
	if text == "" {
		return ""
	}
	if isQuotedString(text) {
		return unquoteString(text)
	}
	if !isSimpleIdentifier(text) || len(hints) == 0 {
		return ""
	}
	hinted := normalizeSourceTypeText(strings.TrimSpace(hints[text]))
	if hinted == "" || !isQuotedString(hinted) {
		return ""
	}
	return unquoteString(hinted)
}

func expandImportedTypeAliasTextWithContext(source, file, typeText string, state *buildState) string {
	_, _, expanded := expandImportedTypeAliasSourceWithContext(source, file, typeText, state)
	return expanded
}

func expandImportedTypeAliasSourceWithContext(source, file, typeText string, state *buildState) (string, string, string) {
	if expanded := expandImportedTypeAliasText(source, file, typeText); expanded != "" {
		return source, file, expanded
	}
	if file == "" || state == nil {
		return "", "", ""
	}

	identifier, ok := sourceIdentifierExpression(typeText)
	if !ok {
		return "", "", ""
	}
	candidate, ok := parseNamedValueImports(source)[identifier]
	if !ok || candidate.ImportedName == "" || candidate.ImportedName == "default" {
		importPaths := parseAllImportPaths(source)
		if len(importPaths) == 0 {
			return "", "", ""
		}
		exportCache := map[string]map[string]bool{}
		packageExportCache := map[string]map[string]importCandidate{}
		resolvedCandidate, resolved := resolveImportedExportCandidate(file, importPaths, identifier, state, exportCache, packageExportCache)
		if !resolved {
			return "", "", ""
		}
		return expandedImportedTypeAliasFromImportPath(file, resolvedCandidate.Path, importReferenceName(resolvedCandidate.Name), state)
	}
	return expandedImportedTypeAliasFromImportPath(file, candidate.Path, candidate.ImportedName, state)
}

func expandedImportedTypeAliasFromImportPath(file, importPath, importedName string, state *buildState) (string, string, string) {
	if file == "" || importPath == "" || importedName == "" || state == nil {
		return "", "", ""
	}
	resolvedFile, ok := resolveImportFile(file, importPath, state)
	if !ok {
		return "", "", ""
	}
	content, err := os.ReadFile(resolvedFile)
	if err != nil {
		return "", "", ""
	}
	importedSource := string(content)
	return importedSource, resolvedFile, expandLocalSourceTypeText(importedSource, importedName)
}

func sourceReducerStateType(expression string) string {
	return sourceReducerStateTypeWithContext(expression, "", expression, nil)
}

func sourceReducerStateTypeFromProperty(source, file string, property SourceProperty, state *buildState) string {
	if source == "" || property.ValueEnd <= property.ValueStart {
		return ""
	}

	start := skipTrivia(source, property.ValueStart)
	if start >= property.ValueEnd {
		return ""
	}
	if source[start] == '[' {
		arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(source, property.ValueStart, property.ValueEnd)
		if err == nil && ok && arrayStart == start {
			parts, err := splitTopLevelSourceSegments(source, arrayStart+1, arrayEnd)
			if err == nil && len(parts) > 0 {
				stateType := sourceReducerLiteralStateTypeFromRange(source, file, parts[0].Start, parts[0].End, state)
				if len(parts) > 1 {
					if widened := sourceWidenReducerStateTypeFromHandlers(stateType, parts[len(parts)-1].Text); widened != "" {
						stateType = widened
					}
				}
				if stateType != "" {
					return normalizeInferredTypeText(stateType)
				}
			}
		}
	}

	return sourceReducerStateTypeWithContext(source, file, sourcePropertyText(source, property), state)
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

func sourceReducerLiteralStateTypeFromRange(source, file string, valueStart, valueEnd int, state *buildState) string {
	expression := strings.TrimSpace(source[valueStart:valueEnd])
	if expression == "" {
		return ""
	}

	inferred := sourceReducerLiteralStateTypeWithContext(source, file, expression, state)
	if parsed, ok := parseReducerStateType(inferred); ok {
		inferred = parsed
	}
	if inferred != "" && !typeTextNeedsSourceRecovery(inferred) {
		return inferred
	}
	if probed := sourceExpressionTypeFromTypeProbeRange(source, file, valueStart, valueEnd, state); probed != "" &&
		!typeTextNeedsSourceRecovery(probed) {
		if parsed, ok := parseReducerStateType(probed); ok {
			probed = parsed
		}
		return normalizeInferredTypeText(widenLiteralReducerStateType(probed))
	}
	return inferred
}

func sourceReducerLiteralStateTypeWithContext(source, file, expression string, state *buildState) string {
	expression = trimLeadingSourceTrivia(expression)
	if hinted := sourceAssertedType(expression); hinted != "" {
		return normalizeInferredTypeText(hinted)
	}
	if source != "" {
		text := trimLeadingSourceTrivia(unwrapWrappedExpression(expression))
		if identifier, ok := sourceIdentifierExpression(text); ok {
			if declared := findLocalValueDeclaredType(source, identifier); declared != "" &&
				!isAnyLikeType(declared) &&
				!typeTextContainsStandaloneToken(declared, "any") &&
				!typeTextContainsStandaloneToken(declared, "unknown") {
				return normalizeInferredTypeText(widenLiteralReducerStateType(declared))
			}
		}
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

func sourceExpressionTypeFromTypeProbeRange(source, file string, valueStart, valueEnd int, state *buildState) string {
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

	probePositions := []int{valueStart}
	probePositions = append(probePositions, selectorTypeProbePositions(source, valueStart, valueEnd)...)

	fallback := ""
	for _, position := range probePositions {
		if position <= 0 {
			continue
		}
		typeText := normalizeSourceTypeText(state.cachedTypeAtPositionString(projectID, file, position))
		if typeText == "" {
			continue
		}
		if isFunctionLikeTypeText(typeText) {
			continue
		}
		if !isAnyLikeType(typeText) &&
			!typeTextContainsStandaloneToken(typeText, "any") &&
			!typeTextContainsStandaloneToken(typeText, "unknown") {
			return typeText
		}
		if fallback == "" {
			fallback = typeText
		}
	}
	return fallback
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

func sourceSelectorProjectorRange(source string, property SourceProperty) (int, int, bool) {
	elements, err := FindTopLevelArrayElements(source, property.ValueStart, property.ValueEnd)
	if err != nil || len(elements) < 2 {
		return 0, 0, false
	}
	index := len(elements) - 1
	for candidate := len(elements) - 1; candidate >= 0; candidate-- {
		element := strings.TrimSpace(source[elements[candidate].Start:elements[candidate].End])
		if sourceSelectorProjectorCandidate(element) {
			index = candidate
			break
		}
	}
	selected := elements[index]
	if selected.End <= selected.Start {
		return 0, 0, false
	}
	return selected.Start, selected.End, true
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
		return normalizeSourceTypeTextWithOptions("("+head+")", false), "", true
	}
	end, err := findMatching(head, 0, '(', ')')
	if err != nil {
		return "", "", false
	}
	parameters := strings.TrimSpace(head[:end+1])
	returnType := strings.TrimSpace(head[end+1:])
	if strings.HasPrefix(returnType, ":") {
		rawReturnType := strings.TrimSpace(returnType[1:])
		returnType = normalizeSourceTypeTextWithOptions(rawReturnType, !strings.Contains(rawReturnType, "=>"))
	} else {
		returnType = ""
	}
	return normalizeSourceTypeTextWithOptions(parameters, false), returnType, true
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

func recoverUntypedBuilderKeyType(source, file string, property SourceProperty, propsType string, state *buildState) string {
	expression := normalizeSingleCallbackExpression(sourcePropertyText(source, property))
	info, ok := parseSourceArrowInfo(expression)
	if !ok || strings.TrimSpace(info.ExplicitReturn) != "" {
		return ""
	}
	parts, ok := splitFunctionParameterParts(info.Parameters)
	if !ok || len(parts) == 0 {
		return ""
	}
	for _, part := range parts {
		if strings.Contains(part, ":") {
			return ""
		}
	}

	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
		if body == "" {
			body = blockReturnExpression(info.Body)
		}
	}
	if !sourceKeyBodyUsesDeclaredHelper(source, file, body, state) {
		return ""
	}

	if probed := normalizeSourceTypeText(sourceCallbackReturnTypeFromTypeProbe(source, file, property, state)); isUsableRecoveredUntypedBuilderKeyType(probed) {
		return probed
	}
	if recovered := normalizeSourceTypeText(sourceKeyTypeFromSource(source, file, property, propsType, state)); isUsableRecoveredUntypedBuilderKeyType(recovered) {
		return recovered
	}
	return ""
}

func sourceKeyBodyUsesDeclaredHelper(source, file, body string, state *buildState) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	callee, ok := stripTrailingCallExpression(body)
	if !ok {
		return false
	}
	identifier, ok := sourceIdentifierExpression(callee)
	if !ok {
		return false
	}
	return sourceFunctionReturnTypeWithContext(source, file, identifier, state) != ""
}

func isUsableRecoveredUntypedBuilderKeyType(typeText string) bool {
	typeText = normalizeSourceTypeText(strings.TrimSpace(typeText))
	if typeText == "" || isAnyLikeType(typeText) || typeTextContainsStandaloneToken(typeText, "any") || typeTextContainsStandaloneToken(typeText, "unknown") {
		return false
	}
	return isPrimitiveLikeKeyType(typeText)
}

func shouldUseRecoveredUntypedBuilderKeyType(source, file, expression, propsType, typeText string, allowTypedParameterMemberRecovery bool, state *buildState) bool {
	if !isUsableRecoveredUntypedBuilderKeyType(typeText) {
		return false
	}
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return false
	}
	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
		if body == "" {
			body = blockReturnExpression(info.Body)
		}
	}
	return sourceKeyBodyUsesDeclaredHelper(source, file, body, state) ||
		(allowTypedParameterMemberRecovery &&
			sourceKeyBodyUsesTypedParameterMember(source, file, info.Parameters, body, propsType, state))
}

func sourceKeyBodyUsesTypedParameterMember(source, file, parameters, body, propsType string, state *buildState) bool {
	body = strings.TrimSpace(body)
	if body == "" || propsType == "" || isAnyLikeType(propsType) {
		return false
	}
	hints := sourceParameterTypeHintsWithDefault(source, file, parameters, propsType, state)
	hints = expandSourceTypeHintsWithContext(source, file, hints, state)
	resolved := normalizeSourceTypeText(sourceExpressionTypeTextWithContext(source, file, body, hints, state))
	if !isUsableRecoveredUntypedBuilderKeyType(resolved) {
		return false
	}
	for name := range hints {
		name = strings.TrimSpace(name)
		if name == "" || name == "_" {
			continue
		}
		if strings.Contains(body, name+".") || strings.Contains(body, name+"?.") || strings.Contains(body, "${"+name+".") {
			return true
		}
	}
	return false
}

func sourceKeyTypeFromSource(source, file string, property SourceProperty, propsType string, state *buildState) string {
	expression := normalizeSingleCallbackExpression(sourcePropertyText(source, property))
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
				hints := sourceParameterTypeHintsWithDefault(source, file, info.Parameters, propsType, state)
				hints = expandSourceTypeHintsWithContext(source, file, hints, state)
				if inferred := sourceKeyExpressionTypeTextWithContext(source, file, body, hints, state); inferred != "" {
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

func expandSourceTypeHintsWithContext(source, file string, hints map[string]string, state *buildState) map[string]string {
	if len(hints) == 0 {
		return hints
	}

	var expandedHints map[string]string
	for name, typeText := range hints {
		normalized := normalizeSourceTypeText(strings.TrimSpace(typeText))
		if expanded := expandImportedTypeAliasTextWithContext(source, file, normalized, state); expanded != "" {
			if expandedHints == nil {
				expandedHints = make(map[string]string, len(hints))
				for existingName, existingType := range hints {
					expandedHints[existingName] = existingType
				}
			}
			expandedHints[name] = normalizeSourceTypeText(expanded)
		}
	}
	if expandedHints != nil {
		return expandedHints
	}
	return hints
}

func sourceKeyExpressionTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
	text := trimLeadingSourceTrivia(expression)
	if text == "" {
		return ""
	}
	if logicalType := sourceKeyLogicalExpressionTypeTextWithContext(source, file, text, hints, state); logicalType != "" {
		return logicalType
	}
	if memberType := sourceMemberAccessTypeTextWithContextOptions(source, file, text, hints, state, true); memberType != "" {
		return memberType
	}
	if commonType := sourceCommonReturnExpressionType(text); commonType != "" {
		return commonType
	}
	return sourceExpressionTypeTextWithContext(source, file, text, hints, state)
}

func sourceKeyLogicalExpressionTypeTextWithContext(source, file, expression string, hints map[string]string, state *buildState) string {
	text := strings.TrimSpace(unwrapWrappedExpression(expression))
	if text == "" {
		return ""
	}
	for _, operator := range []string{"??", "||"} {
		index := findLastTopLevelOperator(text, operator)
		if index == -1 {
			continue
		}
		left := sourceKeyExpressionTypeTextWithContext(source, file, strings.TrimSpace(text[:index]), hints, state)
		right := sourceKeyExpressionTypeTextWithContext(source, file, strings.TrimSpace(text[index+len(operator):]), hints, state)
		if merged := mergeLogicalOperandTypes(left, right, operator); merged != "" {
			return merged
		}
	}
	return ""
}

func sourceSelectorInferredType(logic ParsedLogic, source, file, expression string, state *buildState) string {
	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, expression, state)
	if element := sourceSelectorProjectorElement(expression); element != "" {
		if info, ok := parseSourceArrowInfo(element); ok {
			if returnType := sourceReturnExpressionTypeWithContext(source, file, info.Body, info.BlockBody, info.ParameterNames, dependencyTypes, info.Async, state); returnType != "" {
				normalized := normalizeRecoveredSelectorType(source, file, returnType)
				objectFromEntriesMap := selectorObjectFromEntriesPrimitiveMapType(logic, source, file, expression, state)
				if selectorRecoveredTypeAliasShouldBePreserved(expression, returnType) {
					normalized = normalizeSelectorFunctionTypeOptionalUndefined(normalizeInferredTypeText(strings.TrimSpace(returnType)))
				}
				if objectFromEntriesMap != "" && selectorObjectFromEntriesPrimitiveMapShouldOverride(normalized) {
					normalized = objectFromEntriesMap
				}
				if selectorIdentityPropsReturnShouldStayAny(expression, normalized) {
					return "any"
				}
				return normalized
			}
		}
	}
	if info, ok := parseSourceArrowInfo(expression); ok {
		if returnType := sourceReturnExpressionTypeWithContext(source, file, info.Body, info.BlockBody, info.ParameterNames, dependencyTypes, info.Async, state); returnType != "" {
			normalized := normalizeRecoveredSelectorType(source, file, returnType)
			objectFromEntriesMap := selectorObjectFromEntriesPrimitiveMapType(logic, source, file, expression, state)
			if selectorRecoveredTypeAliasShouldBePreserved(expression, returnType) {
				normalized = normalizeSelectorFunctionTypeOptionalUndefined(normalizeInferredTypeText(strings.TrimSpace(returnType)))
			}
			if objectFromEntriesMap != "" && selectorObjectFromEntriesPrimitiveMapShouldOverride(normalized) {
				normalized = objectFromEntriesMap
			}
			if selectorIdentityPropsReturnShouldStayAny(expression, normalized) {
				return "any"
			}
			return normalized
		}
	}
	return ""
}

func selectorIdentityPropsReturnShouldStayAny(expression, typeText string) bool {
	typeText = normalizeInferredTypeText(strings.TrimSpace(typeText))
	if typeText != "any[]" || !selectorSourceUsesPropsDependency(expression) {
		return false
	}
	projector := sourceSelectorProjectorElement(expression)
	if projector == "" {
		projector = expression
	}
	info, ok := parseSourceArrowInfo(projector)
	if !ok {
		return false
	}
	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	for _, name := range info.ParameterNames {
		if body == strings.TrimSpace(name) {
			return true
		}
	}
	return false
}

func normalizeRecoveredSelectorType(source, file, typeText string) string {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return ""
	}
	if expanded := expandImportedTypeAliasText(source, file, text); expanded != "" {
		text = normalizeInferredTypeText(expanded)
	}
	text = collapseBooleanLiteralUnion(text)
	return normalizeSelectorFunctionTypeOptionalUndefined(text)
}

func collapseBooleanLiteralUnion(typeText string) string {
	text := normalizeSourceTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return ""
	}
	parts, err := splitTopLevelUnion(text)
	if err != nil || len(parts) == 0 {
		return text
	}
	filtered := make([]string, 0, len(parts))
	sawTrue := false
	sawFalse := false
	for _, part := range parts {
		part = normalizeSourceTypeText(strings.TrimSpace(part))
		switch part {
		case "", "true":
			if part == "true" {
				sawTrue = true
			}
			continue
		case "false":
			sawFalse = true
			continue
		default:
			filtered = append(filtered, part)
		}
	}
	if !sawTrue || !sawFalse {
		return text
	}
	filtered = append(filtered, "boolean")
	return strings.Join(filtered, " | ")
}

func selectorRecoveredTypeAliasShouldBePreserved(expression, typeText string) bool {
	if !shouldPreserveRecoveredTypeAlias(typeText) {
		return false
	}
	_, isIdentity := selectorProjectorIdentityParameterIndex(expression)
	return !isIdentity
}

func shouldPreserveRecoveredTypeAlias(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if text == "" || isFunctionLikeTypeText(text) {
		return false
	}
	if _, ok := sourceIdentifierExpression(text); ok {
		return true
	}
	parts, err := splitTopLevelUnion(text)
	if err != nil || len(parts) == 0 {
		parts = []string{text}
	}
	for _, part := range parts {
		part = normalizeInferredTypeText(strings.TrimSpace(part))
		switch part {
		case "", "null", "undefined":
			continue
		}
		if _, ok := sourceIdentifierExpression(part); ok {
			continue
		}
		if strings.Contains(part, "<") && !strings.Contains(part, "{") && !strings.Contains(part, "=>") {
			continue
		}
		return false
	}
	return true
}

func normalizeSelectorFunctionTypeOptionalUndefined(typeText string) string {
	parameters, returnType, ok := splitFunctionType(typeText)
	if !ok {
		return normalizeSelectorOptionalUndefinedTypeText(typeText)
	}
	parts, ok := splitFunctionParameterParts(parameters)
	if !ok {
		return normalizeSelectorOptionalUndefinedTypeText(typeText)
	}
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized = append(normalized, normalizeSelectorFunctionParameterOptionalUndefined(part))
	}
	return "(" + strings.Join(normalized, ", ") + ") => " + normalizeSelectorOptionalUndefinedTypeText(strings.TrimSpace(returnType))
}

func normalizeSelectorFunctionParameterOptionalUndefined(part string) string {
	text := strings.TrimSpace(part)
	rawName, typeText, ok := splitTopLevelPropertyRaw(text)
	if !ok {
		return normalizeSelectorOptionalUndefinedTypeText(text)
	}
	name := strings.TrimSpace(rawName)
	normalizedType := normalizeSelectorOptionalUndefinedTypeText(strings.TrimSpace(typeText))
	if strings.HasSuffix(name, "?") && normalizedType != "" && !strings.Contains(normalizedType, "undefined") {
		normalizedType = normalizeSourceTypeTextWithOptions(normalizedType+" | undefined", false)
	}
	return name + ": " + normalizedType
}

func normalizeSelectorOptionalUndefinedTypeText(typeText string) string {
	return normalizeSourceTypeTextWithOptions(normalizeSelectorOptionalUndefinedTypeLiterals(strings.TrimSpace(typeText)), false)
}

func normalizeSelectorOptionalUndefinedTypeLiterals(text string) string {
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

		inner := normalizeSelectorOptionalUndefinedTypeLiterals(text[i+1 : end])
		normalized, ok := normalizeSelectorOptionalUndefinedTypeLiteralBody(inner)
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

func normalizeSelectorOptionalUndefinedTypeLiteralBody(text string) (string, bool) {
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
		if typeLiteralEntryLooksLikeBindingPattern(entry) {
			return "", false
		}
		if name, value, ok := splitTopLevelTypeMember(entry); ok {
			name = strings.TrimSpace(name)
			value = normalizeSelectorOptionalUndefinedTypeText(value)
			if strings.HasSuffix(name, "?") && value != "" && !strings.Contains(value, "undefined") {
				value = normalizeSourceTypeTextWithOptions(value+" | undefined", false)
			}
			parts = append(parts, name+": "+value)
			continue
		}
		parts = append(parts, normalizeSelectorOptionalUndefinedTypeText(entry))
	}
	if len(parts) == 0 {
		return "", true
	}
	return strings.Join(parts, "; ") + ";", true
}

func wrapSelectorFunctionType(typeText string) string {
	text := strings.TrimSpace(typeText)
	if text == "" || !isFunctionLikeTypeText(text) {
		return typeText
	}
	if unwrapped := unwrapWrappedExpression(text); unwrapped != "" && unwrapped != text && isFunctionLikeTypeText(unwrapped) {
		return text
	}
	return "(" + text + ")"
}

func selectorSourceHasMultipleReturnPaths(expression string) bool {
	projector := sourceSelectorProjectorElement(expression)
	if projector == "" {
		projector = expression
	}
	return sourceArrowExpressionHasMultipleReturnPaths(projector)
}

func sourceArrowExpressionHasMultipleReturnPaths(expression string) bool {
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return false
	}
	if info.BlockBody && sourceHasMultipleReturnPaths(info.Body) {
		return true
	}
	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
		if body == "" {
			body = blockReturnExpression(info.Body)
		}
	}
	if body == "" {
		return false
	}
	if _, ok := parseSourceArrowInfo(body); ok {
		return sourceArrowExpressionHasMultipleReturnPaths(body)
	}
	return false
}

func shouldPreferProbedSelectorTypeForMultipleReturns(current, probed string) bool {
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	probed = normalizeInferredTypeText(strings.TrimSpace(probed))
	if probed == "" || current == probed {
		return false
	}
	if current == "" || selectorTypeNeedsSourceRecovery(current) {
		return true
	}
	if selectorTypeIsBareLiteral(current) {
		return true
	}
	return isFunctionLikeTypeText(current) && isFunctionLikeTypeText(probed)
}

func shouldPreferRecoveredMultiReturnFunctionSelector(current, inferred string) bool {
	current = normalizeInferredTypeText(strings.TrimSpace(current))
	inferred = normalizeInferredTypeText(strings.TrimSpace(inferred))
	if current == "" || inferred == "" || current == inferred {
		return false
	}
	if !isFunctionLikeTypeText(current) || !isFunctionLikeTypeText(inferred) {
		return false
	}
	if strings.Contains(current, "...") && !strings.Contains(inferred, "...") {
		return true
	}
	return strings.Contains(inferred, "| null") && !strings.Contains(current, "| null")
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

func sourceSelectorDependencyNamesWithPlaceholders(expression string) []string {
	partExpressions := sourceSelectorDependencyPartExpressions(expression)
	if len(partExpressions) == 0 {
		return nil
	}

	names := make([]string, 0, len(partExpressions))
	for _, part := range partExpressions {
		if _, fieldName, ok := parseSelectorReference(part); ok {
			names = append(names, fieldName)
			continue
		}
		names = append(names, "")
	}
	return names
}

func sourceSelectorDependencyPartExpressions(expression string) []string {
	var parts []string
	for _, part := range sourceSelectorDependencyElements(expression) {
		parts = append(parts, sourceSelectorDependencyPartExpressionsFromElement(part)...)
	}
	return parts
}

func sourceSelectorDependencyPartExpressionsFromElement(expression string) []string {
	if info, ok := parseSourceArrowInfo(expression); ok {
		body := info.Body
		if info.BlockBody {
			body = singleReturnExpression(body)
		}
		if dependencyParts, ok := sourceDependencyPartExpressionsFromReturnedArray(body); ok {
			return dependencyParts
		}
	}

	if dependencyParts, ok := sourceDependencyPartExpressionsFromReturnedArray(expression); ok {
		return dependencyParts
	}

	text := strings.TrimSpace(expression)
	if text == "" {
		return nil
	}
	return []string{text}
}

func sourceDependencyPartExpressionsFromReturnedArray(expression string) ([]string, bool) {
	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(expression, 0, len(expression))
	if err != nil || !ok {
		return nil, false
	}
	parts, err := splitTopLevelList(expression[arrayStart+1 : arrayEnd])
	if err != nil {
		return nil, false
	}

	dependencyParts := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		dependencyParts = append(dependencyParts, part)
	}
	return dependencyParts, true
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

func sourceSelectorUnnamedDependencyKeepsRecoveredType(expression string) bool {
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return false
	}

	hints := sourceParameterTypeHints("", info.Parameters)
	if len(hints) == 0 {
		return false
	}

	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
		if body == "" {
			body = blockReturnExpression(info.Body)
		}
	}
	if body == "" {
		return false
	}

	for name, typeText := range hints {
		if strings.TrimSpace(typeText) == "" {
			continue
		}
		if typeTextContainsStandaloneToken(body, name) {
			return true
		}
	}
	return false
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
		hints := sourceSelectorInputParameterHints(logic, source, info.Parameters)
		if dependencyTypes := sourceDependencyTypesFromReturnedArray(logic, source, file, body, hints, state); len(dependencyTypes) > 0 {
			return dependencyTypes
		}
		if dependencyType := resolveSelectorDependencyType(logic, source, file, body, state); dependencyType != "" {
			return []string{dependencyType}
		}
		if returnType := sourceArrowReturnTypeText(source, body); returnType != "" {
			return []string{returnType}
		}
		if returnType := sourceExpressionTypeTextWithContext(source, file, body, hints, state); returnType != "" {
			if normalized := normalizeSelectorDependencyType(returnType); normalized != "" {
				return []string{normalized}
			}
		}
	}

	if dependencyTypes := sourceDependencyTypesFromReturnedArray(logic, source, file, expression, nil, state); len(dependencyTypes) > 0 {
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

func normalizeSelectorDependencyType(typeText string) string {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if text == "" {
		return ""
	}
	if returnType, ok := parseFunctionReturnType(text); ok {
		return normalizeInferredTypeText(strings.TrimSpace(returnType))
	}
	return text
}

func sourceSelectorInputParameterHints(logic ParsedLogic, source, parameters string) map[string]string {
	defaults := []string{selectorInputStateType(logic), logic.PropsType}
	return sourceParameterTypeHintsWithDefaults(source, "", parameters, defaults, nil)
}

func selectorInputStateType(logic ParsedLogic) string {
	fields := mergeParsedFields(logic.Reducers, logic.Selectors...)
	if len(fields) == 0 {
		return ""
	}

	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		fieldType := normalizeSourceTypeText(strings.TrimSpace(field.Type))
		if field.Name == "" || fieldType == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", field.Name, fieldType))
	}
	if len(parts) == 0 {
		return ""
	}
	return "{ " + strings.Join(parts, "; ") + "; }"
}

func sourceDependencyTypesFromReturnedArray(logic ParsedLogic, source, file, expression string, hints map[string]string, state *buildState) []string {
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
		if callbackType := sourceSelectorDependencyCallbackReturnType(logic, source, file, part, state); callbackType != "" {
			dependencyTypes = append(dependencyTypes, callbackType)
			continue
		}
		if returnType := sourceArrowReturnTypeText(source, part); returnType != "" {
			dependencyTypes = append(dependencyTypes, returnType)
			continue
		}
		if returnType := sourceExpressionTypeTextWithContext(source, file, part, hints, state); returnType != "" {
			if normalized := normalizeSelectorDependencyType(returnType); normalized != "" {
				dependencyTypes = append(dependencyTypes, normalized)
			}
		}
	}
	return dependencyTypes
}

func sourceSelectorDependencyCallbackReturnType(logic ParsedLogic, source, file, expression string, state *buildState) string {
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
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
	hints := sourceSelectorInputParameterHints(logic, source, info.Parameters)
	if returnType := sourceExpressionTypeTextWithContext(source, file, body, hints, state); returnType != "" {
		return normalizeSelectorDependencyType(returnType)
	}
	return ""
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

	if inferred := sourceExpressionTypeTextWithHints(source, expression, sourceReturnExpressionTypeHints(parameterNames, dependencyTypes)); inferred != "" {
		inferred = normalizeInferredTypeText(inferred)
		if async {
			return promiseTypeText(inferred)
		}
		return inferred
	}

	return ""
}

func sourceReturnExpressionTypeWithContext(source, file, body string, blockBody bool, parameterNames, dependencyTypes []string, async bool, state *buildState) string {
	hints := sourceReturnExpressionTypeHintsWithContext(source, file, parameterNames, dependencyTypes, state)
	mergeExplicitBlockReturn := func(typeText string) string {
		if !blockBody {
			return typeText
		}
		return mergeSourceBlockExplicitReturnType(body, typeText)
	}

	if blockBody && sourceHasMultipleReturnPaths(body) {
		candidates := collectSourceReturnCandidates(body)
		if len(candidates) >= 2 {
			if merged := sourceMultiReturnBlockTypeWithContext(source, file, candidates, async, hints, state); merged != "" {
				return merged
			}
		}
	}

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

	if functionType := sourceArrowFunctionTypeTextWithContext(source, file, expression, state); functionType != "" {
		return normalizeInferredTypeText(functionType)
	}

	if dependencyType := sourceDependencyTypeForReturnExpression(source, expression, parameterNames, dependencyTypes); dependencyType != "" {
		dependencyType = mergeExplicitBlockReturn(dependencyType)
		if async {
			return promiseTypeText(dependencyType)
		}
		return normalizeInferredTypeText(dependencyType)
	}
	if derivedType := sourceCommonReturnExpressionType(expression); derivedType != "" {
		derivedType = mergeExplicitBlockReturn(derivedType)
		if async {
			return promiseTypeText(derivedType)
		}
		return derivedType
	}

	if asserted := sourceAssertedType(expression); asserted != "" {
		inferred := mergeExplicitBlockReturn(normalizeInferredTypeText(asserted))
		if async {
			return promiseTypeText(inferred)
		}
		return inferred
	}

	if inferred := sourceExpressionTypeTextWithContext(
		source,
		file,
		expression,
		hints,
		state,
	); !blockBody && inferred != "" {
		inferred = mergeExplicitBlockReturn(normalizeInferredTypeText(inferred))
		if async {
			return promiseTypeText(inferred)
		}
		return inferred
	}
	if blockBody {
		if inferred := sourceExpressionTypeTextWithBlockScope(source, file, expression, body, hints, state); inferred != "" {
			inferred = mergeExplicitBlockReturn(normalizeInferredTypeText(inferred))
			if async {
				return promiseTypeText(inferred)
			}
			return inferred
		}
	}

	return ""
}

func mergeSourceBlockExplicitReturnType(body, typeText string) string {
	typeText = normalizeInferredTypeText(strings.TrimSpace(typeText))
	if typeText == "" {
		return ""
	}
	nullish := explicitBlockReturnNullishType(body)
	if nullish == "" {
		return typeText
	}
	return mergeNormalizedTypeUnion(typeText, nullish)
}

func explicitBlockReturnNullishType(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	hasNull := regexp.MustCompile(`\breturn\s+(?:await\s+)?null\b`).MatchString(body)
	hasUndefined := regexp.MustCompile(`\breturn\s+(?:await\s+)?undefined\b`).MatchString(body)
	switch {
	case hasNull && hasUndefined:
		return "null | undefined"
	case hasNull:
		return "null"
	case hasUndefined:
		return "undefined"
	default:
		return ""
	}
}

func sourceReturnExpressionTypeHints(parameterNames, dependencyTypes []string) map[string]string {
	return sourceReturnExpressionTypeHintsWithContext("", "", parameterNames, dependencyTypes, nil)
}

func sourceReturnExpressionTypeHintsWithContext(source, file string, parameterNames, dependencyTypes []string, state *buildState) map[string]string {
	if len(parameterNames) == 0 || len(dependencyTypes) == 0 {
		return nil
	}

	hints := map[string]string{}
	for index, name := range parameterNames {
		name = strings.TrimSpace(name)
		if name == "" || index >= len(dependencyTypes) {
			continue
		}

		typeText := normalizeSourceTypeText(strings.TrimSpace(dependencyTypes[index]))
		if typeText == "" {
			continue
		}
		if expanded := expandSourceParameterHintTypeText(source, file, typeText, state); expanded != "" {
			typeText = normalizeSourceTypeText(expanded)
		}
		hints[name] = typeText
	}
	if len(hints) == 0 {
		return nil
	}
	return hints
}

type sourceReturnCandidate struct {
	Expression  string
	ScopePrefix string
}

func sourceMultiReturnBlockTypeWithContext(source, file string, candidates []sourceReturnCandidate, async bool, hints map[string]string, state *buildState) string {
	if len(candidates) == 0 {
		return ""
	}

	merged := ""
	for _, candidate := range candidates {
		inferred := normalizeInferredTypeText(sourceExpressionTypeTextWithBlockScope(source, file, candidate.Expression, candidate.ScopePrefix, hints, state))
		if inferred == "" {
			continue
		}
		if merged == "" {
			merged = inferred
			continue
		}
		merged = mergeNormalizedTypeUnion(merged, inferred)
	}
	if merged == "" {
		return ""
	}
	if async {
		return promiseTypeText(merged)
	}
	return merged
}

func collectSourceReturnCandidates(body string) []sourceReturnCandidate {
	text := strings.TrimSpace(body)
	if len(text) < 2 || text[0] != '{' {
		return nil
	}
	end, err := findMatching(text, 0, '{', '}')
	if err != nil || trimExpressionEnd(text, end+1) != len(text) {
		return nil
	}

	inner := text[1:end]
	candidates := make([]sourceReturnCandidate, 0, 4)
	parenDepth := 0
	bracketDepth := 0
	angleDepth := 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '\'':
			end, err := skipQuoted(inner, i, '\'')
			if err != nil {
				return candidates
			}
			i = end
		case '"':
			end, err := skipQuoted(inner, i, '"')
			if err != nil {
				return candidates
			}
			i = end
		case '`':
			end, err := skipTemplate(inner, i)
			if err != nil {
				return candidates
			}
			i = end
		case '/':
			if i+1 < len(inner) && inner[i+1] == '/' {
				i += 2
				for i < len(inner) && inner[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < len(inner) && inner[i+1] == '*' {
				i += 2
				for i+1 < len(inner) && !(inner[i] == '*' && inner[i+1] == '/') {
					i++
				}
				if i+1 >= len(inner) {
					return candidates
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
		case '<':
			if shouldOpenAngle(inner, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		default:
			if parenDepth != 0 || bracketDepth != 0 || angleDepth != 0 {
				continue
			}
			if !matchesIdentifierAt(inner, i, "return") {
				continue
			}
			expressionStart := skipTrivia(inner, i+len("return"))
			if expressionStart >= len(inner) || inner[expressionStart] == ';' {
				continue
			}
			expressionEnd, err := findReturnExpressionEnd(inner, expressionStart, len(inner))
			if err != nil || expressionEnd <= expressionStart {
				continue
			}
			expression := strings.TrimSpace(inner[expressionStart:expressionEnd])
			if expression == "" {
				continue
			}
			candidates = append(candidates, sourceReturnCandidate{
				Expression:  expression,
				ScopePrefix: inner[:i],
			})
			i = expressionEnd
		}
	}
	return candidates
}

func findReturnExpressionEnd(source string, start, limit int) (int, error) {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0

	for i := start; i < limit; i++ {
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
			if i+1 < limit && source[i+1] == '/' {
				return trimExpressionEnd(source, i), nil
			}
			if i+1 < limit && source[i+1] == '*' {
				i += 2
				for i+1 < limit && !(source[i] == '*' && source[i+1] == '/') {
					i++
				}
				if i+1 >= limit {
					return 0, fmt.Errorf("unterminated block comment")
				}
				i++
				continue
			}
			if isRegexStart(source, i) {
				end, err := skipRegexLiteral(source, i)
				if err != nil {
					return 0, err
				}
				i = end
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
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				return trimExpressionEnd(source, i), nil
			}
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
	return trimExpressionEnd(source, limit), nil
}

func sourceExpressionTypeTextWithBlockScope(source, file, expression, scopePrefix string, hints map[string]string, state *buildState) string {
	expression = strings.TrimSpace(expandBlockScopedExpression(scopePrefix, expression))
	if expression == "" {
		return ""
	}
	scopeHints := mergeSourceTypeHints(hints, sourceBlockScopeTypeHints(source, file, scopePrefix, hints, state))
	if arrayType := sourceArrayLiteralTypeTextWithContextAndScope(source, file, expression, scopePrefix, scopeHints, state); arrayType != "" {
		return arrayType
	}
	return sourceExpressionTypeTextWithContext(source, file, expression, scopeHints, state)
}

func expandBlockScopedExpression(scopePrefix, expression string) string {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return ""
	}
	if identifier, ok := sourceIdentifierExpression(expression); ok {
		if initializer := findLocalValueInitializer(scopePrefix, identifier); initializer != "" {
			return initializer
		}
	}
	if expanded := expandBlockScopedObjectLiteralExpression(scopePrefix, expression); expanded != "" {
		return expanded
	}

	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(expression, 0, len(expression))
	if err != nil || !ok || arrayStart != 0 || arrayEnd != len(expression)-1 {
		return expression
	}
	parts, err := splitTopLevelList(expression[arrayStart+1 : arrayEnd])
	if err != nil {
		return expression
	}

	changed := false
	rewritten := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "...") {
			spread := strings.TrimSpace(part[3:])
			if identifier, ok := sourceIdentifierExpression(spread); ok {
				if initializer := findLocalValueInitializer(scopePrefix, identifier); initializer != "" {
					part = "..." + initializer
					changed = true
				}
			}
		}
		rewritten = append(rewritten, part)
	}
	if !changed {
		return expression
	}
	return "[" + strings.Join(rewritten, ", ") + "]"
}

func expandBlockScopedObjectLiteralExpression(scopePrefix, expression string) string {
	text := strings.TrimSpace(expression)
	if len(text) < 2 || text[0] != '{' {
		return ""
	}

	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(text, 0, len(text))
	if err != nil || !ok || objectStart != 0 || objectEnd != len(text)-1 {
		return ""
	}
	segments, err := splitTopLevelSourceSegments(text, objectStart+1, objectEnd)
	if err != nil || len(segments) == 0 {
		return ""
	}

	changed := false
	rewritten := make([]string, 0, len(segments))
	for _, segment := range segments {
		part := strings.TrimSpace(segment.Text)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "...") {
			rewritten = append(rewritten, part)
			continue
		}
		if _, _, ok := splitTopLevelPropertyRaw(part); ok {
			rewritten = append(rewritten, part)
			continue
		}
		if identifier, ok := sourceIdentifierExpression(part); ok {
			if initializer := findLocalValueInitializer(scopePrefix, identifier); initializer != "" {
				rewritten = append(rewritten, identifier+": "+initializer)
				changed = true
				continue
			}
		}
		rewritten = append(rewritten, part)
	}
	if !changed {
		return ""
	}
	return "{ " + strings.Join(rewritten, ", ") + " }"
}

func sourceBlockScopeTypeHints(source, file, scopePrefix string, baseHints map[string]string, state *buildState) map[string]string {
	scopePrefix = strings.TrimSpace(scopePrefix)
	if scopePrefix == "" {
		return nil
	}

	valuePattern := regexp.MustCompile(`(?m)^\s*(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`)
	matches := valuePattern.FindAllStringSubmatch(scopePrefix, -1)
	if len(matches) == 0 {
		return nil
	}

	scopeHints := mergeSourceTypeHints(nil, baseHints)
	if scopeHints == nil {
		scopeHints = map[string]string{}
	}
	var added map[string]string
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		if existing := strings.TrimSpace(scopeHints[name]); existing != "" && !isAnyLikeType(existing) && !isLooselyTypedType(existing) {
			continue
		}

		typeText := normalizeSourceTypeText(strings.TrimSpace(findLocalValueDeclaredType(scopePrefix, name)))
		if typeText == "" {
			initializer := findLocalValueInitializer(scopePrefix, name)
			if initializer == "" {
				continue
			}
			typeText = normalizeSourceTypeText(sourceExpressionTypeTextWithContext(source, file, initializer, scopeHints, state))
		}
		if typeText == "" {
			continue
		}
		if added == nil {
			added = map[string]string{}
		}
		added[name] = typeText
		scopeHints[name] = typeText
	}
	return added
}

func sourceArrayLiteralTypeTextWithContextAndScope(source, file, expression, scopePrefix string, hints map[string]string, state *buildState) string {
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
			spreadType := strings.TrimSpace(sourceExpressionTypeTextWithBlockScope(source, file, strings.TrimSpace(part[3:]), scopePrefix, hints, state))
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
			part = strings.TrimSpace(sourceExpressionTypeTextWithBlockScope(source, file, part, scopePrefix, hints, state))
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

func sourceBlockContainsNestedFunction(body string) bool {
	return strings.Contains(body, "=>") || strings.Contains(body, "function")
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
	if callee, ok := stripTrailingCallExpression(text); ok {
		for _, suffix := range []string{
			".join",
			".toUpperCase",
			".toLowerCase",
			".trim",
			".charAt",
			".slice",
			".substring",
			".replace",
			".padStart",
			".padEnd",
			".repeat",
			".normalize",
			".concat",
			".toString",
		} {
			if strings.HasSuffix(strings.TrimSpace(callee), suffix) {
				return "string"
			}
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
	text = strings.TrimSuffix(text, "?")
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
	text = normalizeTypeLiteralTextWithOptions(text, collapseOptionalUndefined)
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
	return normalizeTypeLiteralTextWithOptions(text, true)
}

func normalizeTypeLiteralTextWithOptions(text string, collapseOptionalUndefined bool) string {
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

		inner := normalizeTypeLiteralTextWithOptions(text[i+1:end], collapseOptionalUndefined)
		normalized, ok := normalizeTypeLiteralBodyWithOptions(inner, collapseOptionalUndefined)
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
	return normalizeTypeLiteralBodyWithOptions(text, true)
}

func normalizeTypeLiteralBodyWithOptions(text string, collapseOptionalUndefined bool) (string, bool) {
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
		if typeLiteralEntryLooksLikeBindingPattern(entry) {
			return "", false
		}
		if name, value, ok := splitTopLevelTypeMember(entry); ok {
			parts = append(parts, strings.TrimSpace(name)+": "+normalizeSourceTypeTextWithOptions(value, collapseOptionalUndefined))
			continue
		}
		parts = append(parts, normalizeSourceTypeTextWithOptions(entry, collapseOptionalUndefined))
	}
	if len(parts) == 0 {
		return "", true
	}
	return strings.Join(parts, "; ") + ";", true
}

func typeLiteralEntryLooksLikeBindingPattern(entry string) bool {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return false
	}
	if _, _, ok := splitTopLevelTypeMember(entry); ok {
		return false
	}
	if _, ok := sourceIdentifierExpression(entry); ok {
		return true
	}
	if strings.Contains(entry, "=>") || strings.Contains(entry, "(") {
		return false
	}
	if strings.HasPrefix(entry, "{") || strings.HasPrefix(entry, "[") || strings.HasPrefix(entry, "...") {
		return true
	}
	return strings.Contains(entry, ",") || findTopLevelDelimiter(entry, '=') != -1
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
	sourceMembers := canonicalizeSourceProperties(source, file, sectionSourceProperties(source, property), state)
	sourceEntries := sectionSourceEntries(source, property)
	sourceNames := sectionSourceMemberNames(source, property)
	listeners := make([]ParsedListener, 0, len(section.Members)+len(sourceEntries)+len(sourceNames)+len(resolvedExternal))
	var imports []TypeImport
	memberByName := map[string]MemberReport{}
	for _, member := range section.Members {
		member.Name = canonicalSourceObjectMemberName(source, file, member.Name, state)
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

		if logic.InputKind != "builders" || parityModeEnabled() {
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

	if len(sourceEntries) > 0 || len(sourceMembers) > 0 || len(sourceNames) > 0 {
		seenSourceNames := map[string]bool{}
		for _, entry := range sourceEntries {
			name := entry.Name
			if _, typeName, _, ok := resolveActionReferenceFromSourceKey(source, file, entry.Name, state); ok {
				name = typeName
			} else {
				name = canonicalSourceObjectMemberName(source, file, name, state)
			}
			seenSourceNames[name] = true
			if member, ok := memberByName[name]; ok {
				appendListener(name, &member, false)
			} else {
				appendListener(name, nil, false)
			}
		}
		for _, name := range orderedSourcePropertyNames(sourceMembers) {
			if seenSourceNames[name] {
				continue
			}
			seenSourceNames[name] = true
			if member, ok := memberByName[name]; ok {
				appendListener(name, &member, false)
			} else {
				appendListener(name, nil, false)
			}
		}
		for _, name := range sourceNames {
			name = canonicalSourceObjectMemberName(source, file, name, state)
			if name == "" || seenSourceNames[name] {
				continue
			}
			seenSourceNames[name] = true
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
	imports := filterImportsByTypeTexts(targetLogic.Imports, actionImportTypeTexts(action))
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
	parameters, _, ok := splitFunctionType(preferredMemberFunctionTypeText(member))
	if !ok {
		return ""
	}
	return normalizeInferredTypeText(firstParameterType(parameters))
}

func parseSharedListeners(section SectionReport) []ParsedSharedListener {
	return parseSharedListenersWithSource(section, "", SourceProperty{}, nil, "", nil)
}

func parseSharedListenersWithSource(
	section SectionReport,
	source string,
	property SourceProperty,
	listenersProperties []SourceProperty,
	file string,
	state *buildState,
) []ParsedSharedListener {
	sourceMembers := canonicalizeSourceProperties(source, file, sectionSourceProperties(source, property), state)
	sourceEntries := sectionSourceEntries(source, property)
	sourceNames := sectionSourceMemberNames(source, property)
	referencedNames := collectReferencedSharedListenerNames(source, listenersProperties)
	listeners := make([]ParsedSharedListener, 0, len(section.Members)+len(sourceEntries)+len(sourceNames))
	memberByName := map[string]MemberReport{}
	for _, member := range section.Members {
		member.Name = canonicalSourceObjectMemberName(source, file, member.Name, state)
		memberByName[member.Name] = member
	}

	seenNames := map[string]bool{}
	appendListener := func(name string, member *MemberReport, allowGenericFallback bool) {
		name = strings.TrimSpace(name)
		if name == "" || seenNames[name] {
			return
		}

		payloadType := "any"
		actionType := "{ type: string; payload: any }"
		if member != nil {
			if payload := sharedListenerPayloadTypeFromMember(*member); payload != "" && !isAnyLikeType(payload) {
				payloadType = payload
				actionType = normalizeSourceTypeText(fmt.Sprintf("{ type: string; payload: %s }", payload))
			} else if !allowGenericFallback {
				return
			}
		} else if !allowGenericFallback {
			return
		}

		listeners = append(listeners, ParsedSharedListener{
			Name:        name,
			PayloadType: payloadType,
			ActionType:  actionType,
		})
		seenNames[name] = true
	}

	if len(sourceEntries) > 0 || len(sourceMembers) > 0 || len(sourceNames) > 0 {
		seenSourceNames := map[string]bool{}
		for _, entry := range sourceEntries {
			name := canonicalSourceObjectMemberName(source, file, entry.Name, state)
			seenSourceNames[name] = true
			if member, ok := memberByName[name]; ok {
				appendListener(name, &member, referencedNames[name])
			} else {
				appendListener(name, nil, referencedNames[name])
			}
		}
		for _, name := range orderedSourcePropertyNames(sourceMembers) {
			if seenSourceNames[name] {
				continue
			}
			seenSourceNames[name] = true
			if member, ok := memberByName[name]; ok {
				appendListener(name, &member, referencedNames[name])
			} else {
				appendListener(name, nil, referencedNames[name])
			}
		}
		for _, name := range sourceNames {
			name = canonicalSourceObjectMemberName(source, file, name, state)
			if name == "" || seenSourceNames[name] {
				continue
			}
			seenSourceNames[name] = true
			if member, ok := memberByName[name]; ok {
				appendListener(name, &member, referencedNames[name])
			} else {
				appendListener(name, nil, referencedNames[name])
			}
		}
		return listeners
	}

	for _, member := range section.Members {
		member := member
		appendListener(member.Name, &member, false)
	}
	return listeners
}

func collectReferencedSharedListenerNames(source string, listenersProperties []SourceProperty) map[string]bool {
	if source == "" || len(listenersProperties) == 0 {
		return nil
	}

	pattern := regexp.MustCompile(`\bsharedListeners\??\.([A-Za-z_$][A-Za-z0-9_$]*)`)
	referenced := map[string]bool{}
	for _, property := range listenersProperties {
		expression := sourcePropertyText(source, property)
		if expression == "" {
			continue
		}
		for _, match := range pattern.FindAllStringSubmatch(expression, -1) {
			if len(match) < 2 {
				continue
			}
			referenced[match[1]] = true
		}
	}
	if len(referenced) == 0 {
		return nil
	}
	return referenced
}

func sharedListenerPayloadTypeFromMember(member MemberReport) string {
	parameters, _, ok := splitFunctionType(preferredMemberFunctionTypeText(member))
	if !ok {
		return ""
	}
	return normalizeInferredTypeText(firstParameterType(parameters))
}

func parseInternalReducerActions(source, file, inputKind string, reducersProperties, listenersProperties []SourceProperty, state *buildState) []ParsedAction {
	actionsByName := map[string]ParsedAction{}

	appendAction := func(action ParsedAction, actionType string) {
		action.Name = actionType
		actionsByName[actionType] = action
	}

	if inputKind != "builders" || len(listenersProperties) > 0 {
		for _, reducersProperty := range reducersProperties {
			for _, keyText := range collectReducerActionReferenceKeys(source, reducersProperty) {
				action, actionType, _, ok := resolveActionReferenceFromSourceKey(source, file, keyText, state)
				if !ok {
					continue
				}
				appendAction(action, actionType)
			}
		}
	}
	for _, listenersProperty := range listenersProperties {
		for _, entry := range sectionSourceEntries(source, listenersProperty) {
			action, actionType, _, ok := resolveActionReferenceFromSourceKey(source, file, entry.Name, state)
			if !ok {
				continue
			}
			appendAction(action, actionType)
		}
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
		refined[index].FunctionType = refineActionFunctionReturnType(refined[index].FunctionType, hinted)
	}
	return refined
}

func refineActionFunctionType(functionType, payloadType string) string {
	return refineActionFunctionTypePreservingSourceUnknowns(functionType, payloadType, nil)
}

func refineActionFunctionTypePreservingSourceUnknowns(functionType, payloadType string, preserveUnknownParameters map[string]bool) string {
	functionType = strings.TrimSpace(functionType)
	payloadType = normalizeActionPayloadType(payloadType)
	if functionType == "" || payloadType == "" {
		return functionType
	}

	parameters, returnType, ok := splitFunctionType(functionType)
	if !ok {
		return functionType
	}

	refinedParameters := refineActionFunctionParametersFromPayload(parameters, payloadType, preserveUnknownParameters)
	refinedReturnType := refinedActionFunctionReturnType(returnType, payloadType)
	if refinedParameters == parameters && refinedReturnType == returnType {
		return functionType
	}
	return refinedParameters + " => " + refinedReturnType
}

func refineActionFunctionReturnType(functionType, payloadType string) string {
	functionType = strings.TrimSpace(functionType)
	payloadType = normalizeActionPayloadType(payloadType)
	if functionType == "" || payloadType == "" {
		return functionType
	}

	parameters, returnType, ok := splitFunctionType(functionType)
	if !ok {
		return functionType
	}

	refinedReturnType := refinedActionFunctionReturnType(returnType, payloadType)
	if refinedReturnType == returnType {
		return functionType
	}
	return parameters + " => " + refinedReturnType
}

func refinedActionFunctionReturnType(currentReturnType, payloadType string) string {
	if shouldRefineActionPayloadType(currentReturnType, payloadType) {
		return payloadType
	}
	return currentReturnType
}

func refineActionFunctionParametersFromPayload(parameters, payloadType string, preserveUnknownParameters map[string]bool) string {
	parts, ok := splitFunctionParameterParts(parameters)
	if !ok || len(parts) == 0 {
		return parameters
	}
	payloadMembers, ok := parseActionPayloadObjectMembers(payloadType)
	if !ok || len(payloadMembers) == 0 {
		return parameters
	}

	refined := make([]string, 0, len(parts))
	changed := false
	for _, part := range parts {
		normalizedPart := normalizeParameterDeclarationText(part)
		if normalizedPart == "" {
			continue
		}

		rawName, typeText, ok := splitTopLevelPropertyRaw(normalizedPart)
		if !ok {
			refined = append(refined, normalizedPart)
			continue
		}
		name := strings.TrimSpace(strings.TrimSuffix(rawName, "?"))
		if preserveUnknownParameters[name] && typeTextContainsUnquotedStandaloneToken(typeText, "unknown") {
			refined = append(refined, normalizedPart)
			continue
		}
		member, ok := payloadMembers[name]
		allowUnknownToAny := typeTextContainsUnquotedStandaloneToken(typeText, "unknown") && isAnyLikeType(member.Type)
		if !ok || parameterTypeNeedsSourceRecovery(typeText) == false || (parameterTypeNeedsSourceRecovery(member.Type) && !allowUnknownToAny) {
			refined = append(refined, normalizedPart)
			continue
		}

		refined = append(refined, strings.TrimSpace(rawName)+": "+normalizeSourceTypeText(member.Type))
		changed = true
	}
	if !changed {
		return parameters
	}
	return "(" + strings.Join(refined, ", ") + ")"
}

func refineLoaderSuccessActionPayloadTypes(actions []ParsedAction) []ParsedAction {
	if len(actions) == 0 {
		return actions
	}

	actionByName := make(map[string]ParsedAction, len(actions))
	for _, action := range actions {
		actionByName[action.Name] = action
	}

	refined := append([]ParsedAction(nil), actions...)
	for index, action := range refined {
		if !strings.HasSuffix(action.Name, "Success") {
			continue
		}

		baseAction, ok := actionByName[strings.TrimSuffix(action.Name, "Success")]
		if !ok {
			continue
		}
		basePayload := normalizeActionPayloadType(baseAction.PayloadType)
		if basePayload == "" {
			continue
		}

		updated, ok := loaderSuccessActionWithPayloadType(action, basePayload)
		if !ok {
			continue
		}
		refined[index] = updated
	}
	return refined
}

func loaderSuccessActionWithPayloadType(action ParsedAction, payloadType string) (ParsedAction, bool) {
	parameters, _, ok := splitFunctionType(action.FunctionType)
	if !ok {
		return ParsedAction{}, false
	}
	parsedParameters, ok := parseFunctionParameters(parameters)
	if !ok || len(parsedParameters) != 2 {
		return ParsedAction{}, false
	}

	loaderName, loaderType, ok := splitTopLevelProperty(parsedParameters[0].Text)
	if !ok {
		return ParsedAction{}, false
	}
	payloadName, currentPayloadType, ok := splitTopLevelProperty(parsedParameters[1].Text)
	if !ok || payloadName != "payload" || !shouldRefineActionPayloadType(currentPayloadType, payloadType) {
		return ParsedAction{}, false
	}

	payloadType = normalizeActionPayloadType(payloadType)
	return ParsedAction{
		Name:         action.Name,
		FunctionType: fmt.Sprintf("(%s: %s, payload?: %s) => { %s: %s; payload?: %s }", loaderName, loaderType, payloadType, loaderName, loaderType, payloadType),
		PayloadType:  fmt.Sprintf("{ %s: %s; payload?: %s }", loaderName, loaderType, payloadType),
	}, true
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
	if actionPayloadMembersImproved(current, hinted) {
		return true
	}
	if isAnyLikeType(current) || strings.Contains(current, "...") || isGenericIndexSignatureType(current) {
		return true
	}
	currentMain, currentNullish, currentOK := actionPayloadMainType(current)
	hintedMain, hintedNullish, hintedOK := actionPayloadMainType(hinted)
	if currentOK && hintedOK && currentMain == hintedMain && currentNullish == "null" && hintedNullish == "" {
		return true
	}
	if currentOK && hintedOK && currentMain == hintedMain && currentNullish == "undefined" && hintedNullish == "" {
		return true
	}
	return false
}

func isGenericIndexSignatureType(typeText string) bool {
	text := normalizeInferredTypeText(typeText)
	return strings.Contains(text, "[x: string]: any") || strings.Contains(text, "[x:string]: any")
}

func actionPayloadMembersImproved(current, hinted string) bool {
	currentMembers, ok := parseActionPayloadObjectMembers(current)
	if !ok {
		return false
	}
	hintedMembers, ok := parseActionPayloadObjectMembers(hinted)
	if !ok || len(hintedMembers) < len(currentMembers) {
		return false
	}

	improved := false
	for name, currentMember := range currentMembers {
		hintedMember, ok := hintedMembers[name]
		if !ok || currentMember.Optional != hintedMember.Optional {
			return false
		}
		if currentMember.Type == hintedMember.Type {
			continue
		}
		if !typeTextNeedsSourceRecovery(currentMember.Type) || typeTextNeedsSourceRecovery(hintedMember.Type) {
			return false
		}
		improved = true
	}
	for name, hintedMember := range hintedMembers {
		if _, ok := currentMembers[name]; ok {
			continue
		}
		if typeTextNeedsSourceRecovery(hintedMember.Type) {
			continue
		}
		improved = true
	}
	return improved
}

func parseEventNames(section SectionReport) []string {
	return parseEventNamesWithSource(section, "", SourceProperty{})
}

func parseEventNamesWithSource(section SectionReport, source string, property SourceProperty) []string {
	seen := map[string]bool{}
	names := make([]string, 0, len(section.Members))
	appendName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}

	for _, member := range section.Members {
		appendName(member.Name)
	}
	for _, entry := range sectionSourceEntries(source, property) {
		appendName(entry.Name)
	}
	for _, name := range orderedSourcePropertyNames(sectionSourceProperties(source, property)) {
		appendName(name)
	}
	for _, name := range sectionSourceMemberNames(source, property) {
		appendName(name)
	}

	sort.Strings(names)
	return names
}

func sourceOrderedSectionMemberNames(source string, property SourceProperty, members []MemberReport) []string {
	seen := map[string]bool{}
	names := make([]string, 0, len(members))
	appendName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, name := range sectionSourceMemberNames(source, property) {
		appendName(name)
	}
	for _, member := range members {
		appendName(member.Name)
	}
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

func mergeParsedFieldsPreferExisting(existing []ParsedField, extra ...ParsedField) []ParsedField {
	indexByName := make(map[string]int, len(existing))
	merged := append([]ParsedField(nil), existing...)
	for index, field := range merged {
		indexByName[field.Name] = index
	}
	for _, field := range extra {
		if index, ok := indexByName[field.Name]; ok {
			if isAnyLikeType(merged[index].Type) && !isAnyLikeType(field.Type) {
				merged[index] = field
			}
			continue
		}
		indexByName[field.Name] = len(merged)
		merged = append(merged, field)
	}
	return merged
}

func mergeParsedFunctions(existing []ParsedFunction, extra ...ParsedFunction) []ParsedFunction {
	indexByName := make(map[string]int, len(existing))
	merged := append([]ParsedFunction(nil), existing...)
	for index, fn := range merged {
		indexByName[fn.Name] = index
	}
	for _, fn := range extra {
		if index, ok := indexByName[fn.Name]; ok {
			merged[index] = fn
			continue
		}
		indexByName[fn.Name] = len(merged)
		merged = append(merged, fn)
	}
	return merged
}

func mergeParsedListeners(existing []ParsedListener, extra ...ParsedListener) []ParsedListener {
	indexByName := make(map[string]int, len(existing))
	merged := append([]ParsedListener(nil), existing...)
	for index, listener := range merged {
		indexByName[listener.Name] = index
	}
	for _, listener := range extra {
		if index, ok := indexByName[listener.Name]; ok {
			merged[index] = listener
			continue
		}
		indexByName[listener.Name] = len(merged)
		merged = append(merged, listener)
	}
	return merged
}

func mergeParsedSharedListeners(existing []ParsedSharedListener, extra ...ParsedSharedListener) []ParsedSharedListener {
	indexByName := make(map[string]int, len(existing))
	merged := append([]ParsedSharedListener(nil), existing...)
	for index, listener := range merged {
		indexByName[listener.Name] = index
	}
	for _, listener := range extra {
		if index, ok := indexByName[listener.Name]; ok {
			merged[index] = listener
			continue
		}
		indexByName[listener.Name] = len(merged)
		merged = append(merged, listener)
	}
	return merged
}

func mergeEventNames(existing []string, extra ...string) []string {
	seen := make(map[string]bool, len(existing)+len(extra))
	merged := make([]string, 0, len(existing)+len(extra))
	for _, name := range existing {
		if seen[name] {
			continue
		}
		seen[name] = true
		merged = append(merged, name)
	}
	for _, name := range extra {
		if seen[name] {
			continue
		}
		seen[name] = true
		merged = append(merged, name)
	}
	sort.Strings(merged)
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

func mergeParsedActionsPreferLoaderRecovery(existing []ParsedAction, extra ...ParsedAction) []ParsedAction {
	indexByName := make(map[string]int, len(existing))
	merged := append([]ParsedAction(nil), existing...)
	for index, action := range merged {
		indexByName[action.Name] = index
	}
	for _, action := range extra {
		if index, ok := indexByName[action.Name]; ok {
			if shouldPreferRecoveredLoaderAction(merged[index], action) {
				merged[index] = action
			}
			continue
		}
		indexByName[action.Name] = len(merged)
		merged = append(merged, action)
	}
	return merged
}

func reorderParsedLogicForSourceProperties(
	source string,
	sourceLogic SourceLogic,
	sections map[string][]SectionReport,
	properties map[string][]SourceProperty,
	parsed ParsedLogic,
	file string,
	state *buildState,
) ParsedLogic {
	if source == "" || len(sourceLogic.Properties) == 0 {
		return parsed
	}

	var actionOrder []string
	var reducerOrder []string
	var selectorOrder []string
	propertyIndexes := map[string]int{}
	nextPropertyIndex := func(name string) int {
		index := propertyIndexes[name]
		propertyIndexes[name] = index + 1
		return index
	}
	appendActionOrder := func(actions []ParsedAction) {
		for _, action := range actions {
			actionOrder = append(actionOrder, action.Name)
		}
	}
	appendReducerOrder := func(fields []ParsedField) {
		for _, field := range fields {
			reducerOrder = append(reducerOrder, field.Name)
		}
	}

	for _, property := range sourceLogic.Properties {
		index := nextPropertyIndex(property.Name)
		switch property.Name {
		case "connect":
			references, err := parseConnectedReferences(source, property)
			if err != nil {
				continue
			}
			for _, reference := range references {
				switch reference.Kind {
				case "actions":
					for _, name := range reference.Names {
						actionOrder = append(actionOrder, name.LocalName)
					}
				case "values", "props":
					for _, name := range reference.Names {
						selectorOrder = append(selectorOrder, name.LocalName)
					}
				}
			}
		case "actions":
			if index < len(sections["actions"]) {
				appendActionOrder(parseActionsWithSource(sections["actions"][index], source, property, parsed.InputKind, file, state))
			}
		case "defaults":
			if index < len(sections["defaults"]) {
				appendReducerOrder(parseDefaultFieldsWithSource(sections["defaults"][index], source, property, file, state))
			}
		case "reducers":
			if index < len(sections["reducers"]) {
				appendReducerOrder(parseReducersWithSource(sections["reducers"][index], source, property, file, state))
			}
		case "windowValues":
			if index < len(sections["windowValues"]) {
				appendReducerOrder(parseWindowValues(sections["windowValues"][index]))
			}
		case "form":
			if index < len(sections["form"]) {
				formActions, formReducers, _ := parseFormPluginSection(sections["form"][index])
				appendActionOrder(formActions)
				appendReducerOrder(formReducers)
			}
		case "forms":
			if index < len(sections["forms"]) {
				formActions, formReducers, formSelectors, _ := parseFormsPluginSectionWithSource(
					sections["forms"][index],
					source,
					property,
					file,
					state,
				)
				appendActionOrder(formActions)
				appendReducerOrder(formReducers)
				for _, field := range formSelectors {
					selectorOrder = append(selectorOrder, field.Name)
				}
			}
		case "loaders":
			if index < len(sections["loaders"]) {
				loaderActions, loaderReducers := parseLoadersWithSource(sections["loaders"][index], source, property, file, state)
				appendActionOrder(loaderActions)
				appendReducerOrder(loaderReducers)
			}
		case "lazyLoaders":
			if index < len(sections["lazyLoaders"]) {
				loaderActions, loaderReducers := parseLazyLoadersWithSource(sections["lazyLoaders"][index], source, property, file, state)
				appendActionOrder(loaderActions)
				appendReducerOrder(loaderReducers)
			}
		case "selectors":
			if index < len(sections["selectors"]) {
				selectorOrder = append(selectorOrder, sourceOrderedSectionMemberNames(source, property, sections["selectors"][index].Members)...)
			}
		}
	}

	parsed.Actions = reorderParsedActionsByName(parsed.Actions, actionOrder)
	parsed.Reducers = reorderParsedFieldsByName(parsed.Reducers, reducerOrder)
	parsed.Selectors = reorderParsedFieldsByName(parsed.Selectors, selectorOrder)
	parsed.InternalSelectorTypes = reorderParsedFunctionsByName(parsed.InternalSelectorTypes, selectorOrder)
	if len(parsed.InternalReducerActions) > 0 {
		internalActions := parseInternalReducerActions(
			source,
			file,
			parsed.InputKind,
			properties["reducers"],
			properties["listeners"],
			state,
		)
		parsed.InternalReducerActions = reorderParsedActionsByName(parsed.InternalReducerActions, parsedActionNames(internalActions))
	}
	return parsed
}

func reorderParsedActionsByName(actions []ParsedAction, order []string) []ParsedAction {
	if len(actions) == 0 || len(order) == 0 {
		return actions
	}
	indexByName := map[string]int{}
	for index, action := range actions {
		indexByName[action.Name] = index
	}
	used := map[string]bool{}
	reordered := make([]ParsedAction, 0, len(actions))
	for _, name := range order {
		index, ok := indexByName[name]
		if !ok || used[name] {
			continue
		}
		used[name] = true
		reordered = append(reordered, actions[index])
	}
	for _, action := range actions {
		if used[action.Name] {
			continue
		}
		reordered = append(reordered, action)
	}
	return reordered
}

func reorderParsedFieldsByName(fields []ParsedField, order []string) []ParsedField {
	if len(fields) == 0 || len(order) == 0 {
		return fields
	}
	indexByName := map[string]int{}
	for index, field := range fields {
		indexByName[field.Name] = index
	}
	used := map[string]bool{}
	reordered := make([]ParsedField, 0, len(fields))
	for _, name := range order {
		index, ok := indexByName[name]
		if !ok || used[name] {
			continue
		}
		used[name] = true
		reordered = append(reordered, fields[index])
	}
	for _, field := range fields {
		if used[field.Name] {
			continue
		}
		reordered = append(reordered, field)
	}
	return reordered
}

func reorderParsedFunctionsByName(functions []ParsedFunction, order []string) []ParsedFunction {
	if len(functions) == 0 || len(order) == 0 {
		return functions
	}
	indexByName := map[string]int{}
	for index, fn := range functions {
		indexByName[fn.Name] = index
	}
	used := map[string]bool{}
	reordered := make([]ParsedFunction, 0, len(functions))
	for _, name := range order {
		index, ok := indexByName[name]
		if !ok || used[name] {
			continue
		}
		used[name] = true
		reordered = append(reordered, functions[index])
	}
	for _, fn := range functions {
		if used[fn.Name] {
			continue
		}
		reordered = append(reordered, fn)
	}
	return reordered
}

func parsedActionNames(actions []ParsedAction) []string {
	if len(actions) == 0 {
		return nil
	}
	names := make([]string, 0, len(actions))
	for _, action := range actions {
		names = append(names, action.Name)
	}
	return names
}

func shouldPreferRecoveredLoaderAction(current, candidate ParsedAction) bool {
	if current.Name == "" {
		return true
	}
	return shouldPreferRecoveredLoaderPayloadType(current.PayloadType, candidate.PayloadType)
}

func shouldPreferRecoveredLoaderPayloadType(currentPayload, candidatePayload string) bool {
	currentMembers, currentOK := parseActionPayloadObjectMembers(currentPayload)
	candidateMembers, candidateOK := parseActionPayloadObjectMembers(candidatePayload)
	if !currentOK || !candidateOK {
		return false
	}

	improved := false
	for name, candidateMember := range candidateMembers {
		currentMember, ok := currentMembers[name]
		if !ok || currentMember.Optional != candidateMember.Optional {
			return false
		}
		if currentMember.Type == candidateMember.Type {
			continue
		}
		if name == "payload" || !loaderPayloadMemberStripsNullishOnly(currentMember.Type, candidateMember.Type) {
			return false
		}
		improved = true
	}
	return improved
}

func loaderPayloadMemberStripsNullishOnly(current, candidate string) bool {
	currentMain, currentNullish, ok := actionPayloadMainType(current)
	if !ok || currentNullish == "" {
		return false
	}
	candidateMain, candidateNullish, ok := actionPayloadMainType(candidate)
	if !ok {
		return false
	}
	return currentMain == candidateMain && candidateNullish == ""
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
	s.typeTextByTypeID = nil
	s.typeTextByPos = nil
	s.typeTextByLocation = nil
	s.callbackByPos = nil
	s.signatureByPos = nil
	s.sourceOffsetByFile = nil
	s.sourceNodesByFile = nil
	s.parsedFileByFile = nil
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

func (s *buildState) typeKey(projectID, typeID string) string {
	return projectID + "\x00" + typeID
}

func (s *buildState) positionKey(projectID, file string, position int) string {
	return projectID + "\x00" + filepath.Clean(file) + "\x00" + strconv.Itoa(position)
}

func (s *buildState) locationKey(projectID, location string) string {
	return projectID + "\x00" + location
}

func (s *buildState) sourceOffsetMapForFile(file string) (sourceOffsetMap, bool) {
	if s == nil || file == "" {
		return sourceOffsetMap{}, false
	}

	file = filepath.Clean(file)
	if s.sourceOffsetByFile != nil {
		if offsets, ok := s.sourceOffsetByFile[file]; ok {
			return offsets, true
		}
	}

	source := ""
	if s.parsedFileByFile != nil {
		if entry, ok := s.parsedFileByFile[file]; ok {
			source = entry.Source
		}
	}
	if source == "" {
		sourceBytes, err := os.ReadFile(file)
		if err != nil {
			return sourceOffsetMap{}, false
		}
		source = string(sourceBytes)
	}

	offsets := newSourceOffsetMap(source)
	if s.sourceOffsetByFile == nil {
		s.sourceOffsetByFile = map[string]sourceOffsetMap{}
	}
	s.sourceOffsetByFile[file] = offsets
	return offsets, true
}

func (s *buildState) normalizedPositionForFile(file string, position int) int {
	if position <= 0 {
		return position
	}
	offsets, ok := s.sourceOffsetMapForFile(file)
	if !ok {
		return position
	}
	return offsets.utf16Offset(position)
}

func (s *buildState) sourceNodesForFile(projectID, file string) (sourceFileNodeCache, bool) {
	if s == nil || s.apiClient == nil || projectID == "" || file == "" {
		return sourceFileNodeCache{}, false
	}

	file = filepath.Clean(file)
	if s.sourceNodesByFile != nil {
		if entry, ok := s.sourceNodesByFile[file]; ok {
			return entry, entry.OK
		}
	}

	offsets, ok := s.sourceOffsetMapForFile(file)
	if !ok {
		return sourceFileNodeCache{}, false
	}

	raw, err := s.apiClient.CallRaw(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		"getSourceFile",
		map[string]any{
			"snapshot": s.apiSnapshot,
			"project":  projectID,
			"file":     file,
		},
	)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return sourceFileNodeCache{}, false
	}

	var payload struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Data == "" {
		return sourceFileNodeCache{}, false
	}

	blob, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		return sourceFileNodeCache{}, false
	}
	nodes, err := parseSourceFileNodes(blob, offsets.utf16Length())
	if err != nil {
		return sourceFileNodeCache{}, false
	}

	entry := sourceFileNodeCache{
		Nodes:         nodes,
		CanonicalFile: file,
		OK:            true,
	}
	if s.sourceNodesByFile == nil {
		s.sourceNodesByFile = map[string]sourceFileNodeCache{}
	}
	s.sourceNodesByFile[file] = entry
	return entry, true
}

func (s *buildState) locationHandleForRange(projectID, file string, start, end int) string {
	if end <= start {
		return ""
	}
	nodes, ok := s.sourceNodesForFile(projectID, file)
	if !ok {
		return ""
	}

	return sourceNodeLocationHandleForRange(
		nodes.Nodes,
		s.normalizedPositionForFile(file, start),
		s.normalizedPositionForFile(file, end),
		nodes.CanonicalFile,
	)
}

func (s *buildState) cachedTypeString(projectID, typeID string) string {
	if s == nil || s.apiClient == nil || projectID == "" || typeID == "" {
		return ""
	}
	if s.typeTextByTypeID == nil {
		s.typeTextByTypeID = map[string]string{}
	}
	key := s.typeKey(projectID, typeID)
	if cached, ok := s.typeTextByTypeID[key]; ok {
		return cached
	}
	text := normalizeSourceTypeText(safeTypeString(
		context.Background(),
		s.apiClient,
		s.timeout,
		s.apiSnapshot,
		projectID,
		typeID,
	))
	s.typeTextByTypeID[key] = text
	return text
}

func (s *buildState) cachedTypeAtLocationString(projectID, location string) string {
	if s == nil || s.apiClient == nil || projectID == "" || location == "" {
		return ""
	}
	if s.typeTextByLocation == nil {
		s.typeTextByLocation = map[string]string{}
	}
	key := s.locationKey(projectID, location)
	if cached, ok := s.typeTextByLocation[key]; ok {
		return cached
	}
	typ, err := s.apiClient.GetTypeAtLocation(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		s.apiSnapshot,
		projectID,
		location,
	)
	if err != nil || typ == nil {
		s.typeTextByLocation[key] = ""
		return ""
	}
	text := s.cachedTypeString(projectID, typ.ID)
	s.typeTextByLocation[key] = text
	return text
}

func (s *buildState) cachedTypeAtPositionString(projectID, file string, position int) string {
	if s == nil || s.apiClient == nil || projectID == "" || file == "" || position <= 0 {
		return ""
	}
	position = s.normalizedPositionForFile(file, position)
	if s.typeTextByPos == nil {
		s.typeTextByPos = map[string]string{}
	}
	key := s.positionKey(projectID, file, position)
	if cached, ok := s.typeTextByPos[key]; ok {
		return cached
	}
	typ, err := s.apiClient.GetTypeAtPosition(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		s.apiSnapshot,
		projectID,
		file,
		position,
	)
	if err != nil || typ == nil {
		s.typeTextByPos[key] = ""
		return ""
	}
	text := s.cachedTypeString(projectID, typ.ID)
	s.typeTextByPos[key] = text
	return text
}

func (s *buildState) cachedSignatureReturnTypeAtPositionString(projectID, file string, position int) string {
	if s == nil || s.apiClient == nil || projectID == "" || file == "" || position <= 0 {
		return ""
	}
	position = s.normalizedPositionForFile(file, position)
	if s.signatureByPos == nil {
		s.signatureByPos = map[string]string{}
	}
	key := s.positionKey(projectID, file, position)
	if cached, ok := s.signatureByPos[key]; ok {
		return cached
	}

	functionType, err := s.apiClient.GetTypeAtPosition(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		s.apiSnapshot,
		projectID,
		file,
		position,
	)
	if err != nil || functionType == nil {
		s.signatureByPos[key] = ""
		return ""
	}

	signatures, err := s.apiClient.GetSignaturesOfType(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		s.apiSnapshot,
		projectID,
		functionType.ID,
	)
	if err != nil || len(signatures) == 0 {
		s.signatureByPos[key] = ""
		return ""
	}

	returnType, err := s.apiClient.GetReturnTypeOfSignature(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		s.apiSnapshot,
		projectID,
		signatures[0].ID,
	)
	if err != nil || returnType == nil {
		s.signatureByPos[key] = ""
		return ""
	}

	text := s.cachedTypeString(projectID, returnType.ID)
	s.signatureByPos[key] = text
	return text
}

func (s *buildState) cachedCallbackReturnTypeAtPositionString(projectID, file string, position int) string {
	if s == nil || s.apiClient == nil || projectID == "" || file == "" || position <= 0 {
		return ""
	}
	position = s.normalizedPositionForFile(file, position)
	if s.callbackByPos == nil {
		s.callbackByPos = map[string]string{}
	}
	key := s.positionKey(projectID, file, position)
	if cached, ok := s.callbackByPos[key]; ok {
		return cached
	}

	typ, err := s.apiClient.GetTypeAtPosition(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		s.apiSnapshot,
		projectID,
		file,
		position,
	)
	if err != nil || typ == nil {
		s.callbackByPos[key] = ""
		return ""
	}

	signatures, err := s.apiClient.GetSignaturesOfType(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		s.apiSnapshot,
		projectID,
		typ.ID,
	)
	if err == nil && len(signatures) > 0 {
		returnType, err := s.apiClient.GetReturnTypeOfSignature(
			tsgoapi.WithTimeout(context.Background(), s.timeout),
			s.apiSnapshot,
			projectID,
			signatures[0].ID,
		)
		if err == nil && returnType != nil {
			if text := s.cachedTypeString(projectID, returnType.ID); text != "" {
				s.callbackByPos[key] = text
				return text
			}
		}
	}

	typeText := s.cachedTypeString(projectID, typ.ID)
	if typeText == "" {
		s.callbackByPos[key] = ""
		return ""
	}
	if isFunctionLikeTypeText(typeText) {
		if returnType, ok := parseFunctionReturnType(typeText); ok {
			text := normalizeSourceTypeText(returnType)
			s.callbackByPos[key] = text
			return text
		}
	}
	s.callbackByPos[key] = ""
	return ""
}

func (s *buildState) firstCallablePropertySignatureOnType(projectID, typeID string) string {
	if s == nil || s.apiClient == nil || projectID == "" || typeID == "" {
		return ""
	}

	properties, err := s.apiClient.GetPropertiesOfType(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		s.apiSnapshot,
		projectID,
		typeID,
	)
	if err != nil {
		return ""
	}
	for _, property := range properties {
		if property == nil || property.ID == "" {
			continue
		}
		propertyType, err := s.apiClient.GetTypeOfSymbol(
			tsgoapi.WithTimeout(context.Background(), s.timeout),
			s.apiSnapshot,
			projectID,
			property.ID,
		)
		if err != nil || propertyType == nil || propertyType.ID == "" {
			continue
		}
		signatures, err := s.apiClient.GetSignaturesOfType(
			tsgoapi.WithTimeout(context.Background(), s.timeout),
			s.apiSnapshot,
			projectID,
			propertyType.ID,
		)
		if err != nil || len(signatures) == 0 || signatures[0] == nil || signatures[0].ID == "" {
			continue
		}
		return signatures[0].ID
	}
	return ""
}

func (s *buildState) firstTypeOfSymbol(projectID, symbolID string) string {
	if s == nil || s.apiClient == nil || projectID == "" || symbolID == "" {
		return ""
	}

	raw, err := s.apiClient.CallRaw(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		"getTypesOfSymbols",
		map[string]any{
			"snapshot": s.apiSnapshot,
			"project":  projectID,
			"symbols":  []string{symbolID},
		},
	)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var types []*tsgoapi.TypeResponse
	if err := json.Unmarshal(raw, &types); err != nil {
		return ""
	}
	if len(types) == 0 || types[0] == nil {
		return ""
	}
	return types[0].ID
}

func (s *buildState) returnTypeToString(projectID, signatureID string) string {
	if s == nil || s.apiClient == nil || projectID == "" || signatureID == "" {
		return ""
	}

	raw, err := s.apiClient.CallRaw(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		"returnTypeToString",
		map[string]any{
			"snapshot":  s.apiSnapshot,
			"project":   projectID,
			"signature": signatureID,
		},
	)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return ""
	}
	return normalizeSourceTypeText(text)
}

func (s *buildState) typeAtLocationMethod(projectID, method, location string) (*tsgoapi.TypeResponse, error) {
	if s == nil || s.apiClient == nil || projectID == "" || location == "" {
		return nil, nil
	}

	raw, err := s.apiClient.CallRaw(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		method,
		map[string]any{
			"snapshot": s.apiSnapshot,
			"project":  projectID,
			"location": location,
		},
	)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return nil, err
	}

	var typ tsgoapi.TypeResponse
	if err := json.Unmarshal(raw, &typ); err != nil {
		return nil, err
	}
	return &typ, nil
}

func (s *buildState) signatureTypesForType(projectID, typeID string) ([]string, string, bool) {
	if s == nil || s.apiClient == nil || projectID == "" || typeID == "" {
		return nil, "", false
	}

	signatures, err := s.apiClient.GetSignaturesOfType(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		s.apiSnapshot,
		projectID,
		typeID,
	)
	if err != nil || len(signatures) == 0 || signatures[0] == nil {
		return nil, "", false
	}

	parameters := make([]string, 0, len(signatures[0].Parameters))
	for _, symbolID := range signatures[0].Parameters {
		typ, err := s.apiClient.GetTypeOfSymbol(
			tsgoapi.WithTimeout(context.Background(), s.timeout),
			s.apiSnapshot,
			projectID,
			symbolID,
		)
		if err != nil || typ == nil {
			return nil, "", false
		}
		typeText := s.cachedTypeString(projectID, typ.ID)
		if typeText == "" {
			return nil, "", false
		}
		parameters = append(parameters, typeText)
	}

	returnType, err := s.apiClient.GetReturnTypeOfSignature(
		tsgoapi.WithTimeout(context.Background(), s.timeout),
		s.apiSnapshot,
		projectID,
		signatures[0].ID,
	)
	if err != nil || returnType == nil {
		return nil, "", false
	}
	returnTypeText := s.cachedTypeString(projectID, returnType.ID)
	if returnTypeText == "" {
		return nil, "", false
	}

	return parameters, returnTypeText, true
}

func (s *buildState) signatureTypesAtLocation(projectID, location string) ([]string, string, bool) {
	for _, method := range []string{"getTypeAtLocation", "getContextualType"} {
		typ, err := s.typeAtLocationMethod(projectID, method, location)
		if err != nil || typ == nil || typ.ID == "" {
			continue
		}
		if parameters, returnType, ok := s.signatureTypesForType(projectID, typ.ID); ok {
			return parameters, returnType, true
		}
	}
	return nil, "", false
}

func parseSourceFileNodes(blob []byte, sourceLen int) ([]sourceFileNode, error) {
	offset, ok := findSourceFileNodeTableOffset(blob, sourceLen)
	if !ok {
		return nil, fmt.Errorf("could not find source file node table")
	}

	recordCount := (len(blob) - offset) / sourceNodeRecordBytes
	nodes := make([]sourceFileNode, 0, recordCount)
	for record := 0; record < recordCount; record++ {
		base := offset + record*sourceNodeRecordBytes
		nodes = append(nodes, sourceFileNode{
			Kind:   binary.LittleEndian.Uint32(blob[base:]),
			Pos:    binary.LittleEndian.Uint32(blob[base+4:]),
			End:    binary.LittleEndian.Uint32(blob[base+8:]),
			Parent: binary.LittleEndian.Uint32(blob[base+16:]),
		})
	}
	return nodes, nil
}

func findSourceFileNodeTableOffset(blob []byte, sourceLen int) (int, bool) {
	sourceLen32 := uint32(sourceLen)
	for offset := 0; offset+sourceNodeRecordBytes*2 <= len(blob); offset++ {
		if !sourceZeroNodeRecord(blob[offset : offset+sourceNodeRecordBytes]) {
			continue
		}
		next := offset + sourceNodeRecordBytes
		if binary.LittleEndian.Uint32(blob[next:]) != sourceFileSyntaxKind {
			continue
		}
		if binary.LittleEndian.Uint32(blob[next+4:]) != 0 || binary.LittleEndian.Uint32(blob[next+8:]) != sourceLen32 {
			continue
		}
		return offset, true
	}
	return 0, false
}

func sourceZeroNodeRecord(record []byte) bool {
	if len(record) < sourceNodeRecordBytes {
		return false
	}
	for offset := 0; offset < sourceNodeRecordBytes; offset += 4 {
		if binary.LittleEndian.Uint32(record[offset:]) != 0 {
			return false
		}
	}
	return true
}

func sourceNodeLocationHandleForRange(nodes []sourceFileNode, start, end int, file string) string {
	bestIndex := -1
	bestWidth := 0
	exactMatch := false
	for index, node := range nodes {
		if node.Kind == 0 || node.Kind == sourceInvalidNodeKind || node.End < node.Pos {
			continue
		}
		if int(node.Pos) > start || int(node.End) < end {
			continue
		}
		width := int(node.End - node.Pos)
		match := int(node.Pos) == start && int(node.End) == end
		if bestIndex == -1 ||
			(match && !exactMatch) ||
			(match == exactMatch && width < bestWidth) ||
			(match == exactMatch && width == bestWidth && node.Pos > nodes[bestIndex].Pos) {
			bestIndex = index
			bestWidth = width
			exactMatch = match
		}
	}
	if bestIndex == -1 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%s", nodes[bestIndex].Pos, nodes[bestIndex].End, nodes[bestIndex].Kind, file)
}

func (s *buildState) loadLogics(file string) ([]ParsedLogic, error) {
	if s == nil || s.binaryPath == "" || s.projectDir == "" || s.configFile == "" {
		return nil, fmt.Errorf("build state is not configured for recursive inspection")
	}

	file = filepath.Clean(file)
	if parsed, ok := s.parsedByFile[file]; ok {
		return parsed, nil
	}
	entry, err := s.loadParsedFile(file)
	if err != nil {
		return nil, err
	}
	return entry.Logics, nil
}

func (s *buildState) loadParsedFile(file string) (parsedFileCache, error) {
	if s == nil {
		return parsedFileCache{}, fmt.Errorf("build state is nil")
	}

	file = filepath.Clean(file)
	if s.parsedFileByFile != nil {
		if entry, ok := s.parsedFileByFile[file]; ok {
			return entry, nil
		}
	}
	if s.binaryPath == "" || s.projectDir == "" || s.configFile == "" {
		return parsedFileCache{}, fmt.Errorf("build state is not configured for recursive inspection")
	}
	if s.building[file] {
		return parsedFileCache{}, fmt.Errorf("cyclic logic inspection for %s", file)
	}

	s.building[file] = true
	defer delete(s.building, file)

	report, source, err := s.inspectFile(file)
	if err != nil {
		return parsedFileCache{}, err
	}
	sourceLogics, err := FindLogics(source)
	if err != nil {
		return parsedFileCache{}, err
	}

	parsed, err := buildParsedLogicsFromSource(report, source, s)
	if err != nil {
		return parsedFileCache{}, err
	}
	s.parsedByFile[file] = parsed
	if s.parsedFileByFile == nil {
		s.parsedFileByFile = map[string]parsedFileCache{}
	}
	entry := parsedFileCache{
		File:         file,
		Source:       source,
		SourceLogics: sourceLogics,
		Logics:       parsed,
	}
	s.parsedFileByFile[file] = entry
	return entry, nil
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
	offsets := newSourceOffsetMap(source)
	for _, logic := range logics {
		report.Logics = append(report.Logics, inspectLogic(
			context.Background(),
			s.apiClient,
			s.timeout,
			s.apiSnapshot,
			projectID,
			file,
			source,
			offsets,
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

func enrichConnectedSections(source, file string, property SourceProperty, listenerSections []SectionReport, parsed *ParsedLogic, state *buildState) []TypeImport {
	references, err := parseConnectedReferences(source, property)
	if err != nil {
		return nil
	}

	var imports []TypeImport
	for _, reference := range references {
		targetLogic, hasLocalLogic := resolveConnectedLogic(source, file, reference.TargetExpr, state)
		preferLocalParitySurface := parityModeEnabled() && hasLocalLogic

		switch reference.Kind {
		case "actions":
			var addedTypeTexts []string
			actionMembers, _ := resolveConnectedSectionMembersBySymbol(source, file, reference, "actions", state)
			actionCreatorMembers, _ := resolveConnectedSectionMembersBySymbol(source, file, reference, "actionCreators", state)
			for _, name := range reference.Names {
				if preferLocalParitySurface {
					action, found := findParsedAction(targetLogic.Actions, name.SourceName)
					if !found {
						continue
					}
					copied := action
					copied.Name = name.LocalName
					if member, found := findMemberReportInSections(listenerSections, name.LocalName); found {
						if payload := listenerPayloadTypeFromMember(member); payload != "" && !strings.Contains(payload, "...") && shouldRefineActionPayloadType(copied.PayloadType, payload) {
							copied.PayloadType = payload
						}
					}
					parsed.Actions = mergeParsedActions(parsed.Actions, copied)
					addedTypeTexts = append(addedTypeTexts, actionImportTypeTexts(copied)...)
					continue
				}
				if action, ok := synthesizeConnectedActionFromSymbols(name, actionMembers, actionCreatorMembers); ok {
					if hasLocalLogic && !preferLocalParitySurface {
						if parsedAction, found := findParsedAction(targetLogic.Actions, name.SourceName); found && shouldPreferParsedConnectedAction(action, parsedAction) {
							action = parsedAction
							action.Name = name.LocalName
						}
					}
					if member, found := findMemberReportInSections(listenerSections, name.LocalName); found {
						if payload := listenerPayloadTypeFromMember(member); payload != "" && !strings.Contains(payload, "...") && shouldRefineActionPayloadType(action.PayloadType, payload) {
							action.PayloadType = payload
						}
					}
					parsed.Actions = mergeParsedActions(parsed.Actions, action)
					addedTypeTexts = append(addedTypeTexts, actionImportTypeTexts(action)...)
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
					addedTypeTexts = append(addedTypeTexts, actionImportTypeTexts(copied)...)
				}
			}
			if hasLocalLogic {
				sourceImports := mergeTypeImports(
					filterImportsByTypeTexts(targetLogic.Imports, addedTypeTexts),
					collectConnectedImportsFromSource(targetLogic, addedTypeTexts, state),
				)
				imports = mergeTypeImports(imports, rebaseTypeImports(sourceImports, targetLogic.File, file))
				if generatedLogic, ok := loadLocalGeneratedLogicType(targetLogic.File, targetLogic.Name); ok {
					typeImports := filterImportsByTypeTexts(generatedLogic.Imports, addedTypeTexts)
					imports = mergeTypeImports(imports, rebaseTypeImports(typeImports, generatedLogic.File, file))
				}
			}
			if len(addedTypeTexts) > 0 {
				continue
			}

			if len(listenerSections) == 0 {
				continue
			}
			for _, name := range reference.Names {
				member, found := findMemberReportInSections(listenerSections, name.LocalName)
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
				if preferLocalParitySurface {
					field, found := findParsedField(targetFields, name.SourceName)
					if !found {
						continue
					}
					copied := field
					copied.Name = name.LocalName
					parsed.Selectors = mergeParsedFields(parsed.Selectors, copied)
					addedTypeTexts = append(addedTypeTexts, copied.Type)
					continue
				}
				if member, found := findMemberReport(valueMembers, name.SourceName); found && strings.TrimSpace(member.TypeString) != "" {
					if hasLocalLogic && !preferLocalParitySurface {
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
				sourceImports := mergeTypeImports(
					filterImportsByTypeTexts(targetLogic.Imports, addedTypeTexts),
					collectConnectedImportsFromSource(targetLogic, addedTypeTexts, state),
				)
				imports = mergeTypeImports(imports, rebaseTypeImports(sourceImports, targetLogic.File, file))
				if generatedLogic, ok := loadLocalGeneratedLogicType(targetLogic.File, targetLogic.Name); ok {
					typeImports := filterImportsByTypeTexts(generatedLogic.Imports, addedTypeTexts)
					imports = mergeTypeImports(imports, rebaseTypeImports(typeImports, generatedLogic.File, file))
				}
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

func connectedTargetIsBareLocalImport(source, expression string) bool {
	if !isSimpleIdentifier(strings.TrimSpace(expression)) {
		return false
	}
	target, ok := parseConnectedTargetReference(expression)
	if !ok {
		return false
	}
	if candidate, ok := parseNamedValueImports(source)[target.BaseAlias]; ok {
		return strings.HasPrefix(strings.TrimSpace(candidate.Path), ".")
	}
	if candidate, ok := parseDefaultValueImports(source)[target.BaseAlias]; ok {
		return strings.HasPrefix(strings.TrimSpace(candidate.Path), ".")
	}
	if importPath, ok := parseNamespaceValueImports(source)[target.BaseAlias]; ok {
		return strings.HasPrefix(strings.TrimSpace(importPath), ".")
	}
	return false
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
		if err == nil {
			if logic, ok := findConnectedLogic(logics, names...); ok {
				return logic, true
			}
		}
		candidates := append([]string{packageLogicName}, names...)
		seen := map[string]bool{}
		for _, candidateName := range candidates {
			candidateName = strings.TrimSpace(candidateName)
			if candidateName == "" || seen[candidateName] {
				continue
			}
			seen[candidateName] = true
			if logic, ok := loadLocalGeneratedLogicType(resolvedFile, candidateName); ok {
				return logic, true
			}
		}
	}

	if strings.TrimSpace(packageLogicName) == "" || strings.HasPrefix(importPath, ".") {
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
		strings.Contains(symbolAction.FunctionType, "[x: string]: any") ||
		connectedActionFunctionParametersLookMoreSpecific(symbolAction.FunctionType, parsedAction.FunctionType)
}

func connectedActionFunctionParametersLookMoreSpecific(current, candidate string) bool {
	currentParameters, _, currentOK := splitFunctionType(current)
	candidateParameters, _, candidateOK := splitFunctionType(candidate)
	if !currentOK || !candidateOK {
		return false
	}

	currentParams, currentOK := parseFunctionParameters(currentParameters)
	candidateParams, candidateOK := parseFunctionParameters(candidateParameters)
	if !currentOK || !candidateOK || len(currentParams) != len(candidateParams) {
		return false
	}

	better := false
	for index := range currentParams {
		currentType := normalizeInferredTypeText(strings.TrimSpace(currentParams[index].Type))
		candidateType := normalizeInferredTypeText(strings.TrimSpace(candidateParams[index].Type))
		if currentType == candidateType {
			continue
		}
		if connectedActionTypeLooksMoreSpecific(currentType, candidateType) {
			better = true
			continue
		}
		if connectedActionTypeLooksMoreSpecific(candidateType, currentType) {
			return false
		}
	}

	return better
}

func connectedActionTypeLooksMoreSpecific(current, candidate string) bool {
	current = normalizeActionPayloadType(current)
	candidate = normalizeActionPayloadType(candidate)
	if current == "" || candidate == "" || current == candidate {
		return false
	}
	if connectedActionTypeContainsIndexedAccess(candidate) && !connectedActionTypeContainsIndexedAccess(current) {
		return true
	}
	if strings.HasSuffix(current, "[]") && strings.HasSuffix(candidate, "[]") {
		return connectedActionTypeLooksMoreSpecific(
			strings.TrimSpace(current[:len(current)-2]),
			strings.TrimSpace(candidate[:len(candidate)-2]),
		)
	}
	currentMembers, currentOK := parseObjectTypeMembers(current)
	candidateMembers, candidateOK := parseObjectTypeMembers(candidate)
	if !currentOK || !candidateOK {
		return false
	}
	for name, candidateType := range candidateMembers {
		currentType, ok := currentMembers[name]
		if !ok {
			continue
		}
		if connectedActionTypeLooksMoreSpecific(currentType, candidateType) {
			return true
		}
	}
	return false
}

func connectedActionTypeContainsIndexedAccess(typeText string) bool {
	text := normalizeActionPayloadType(typeText)
	return strings.Contains(text, "['") || strings.Contains(text, "[\"")
}

func shouldPreferParsedConnectedField(symbolType, parsedType string) bool {
	if connectedFieldLiteralShouldYieldToParsed(symbolType, parsedType) {
		return true
	}
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

func connectedFieldLiteralShouldYieldToParsed(symbolType, parsedType string) bool {
	symbolType = normalizeInferredTypeText(strings.TrimSpace(symbolType))
	parsedType = normalizeInferredTypeText(strings.TrimSpace(parsedType))
	if symbolType == "" || parsedType == "" || symbolType == parsedType {
		return false
	}
	if !selectorTypeIsLiteralDriven(symbolType) {
		return false
	}
	if isAnyLikeType(parsedType) || isLooselyTypedType(parsedType) {
		return false
	}
	if !selectorTypeIsLiteralDriven(parsedType) {
		return true
	}
	parts, err := splitTopLevelUnion(parsedType)
	return err == nil && len(parts) > 1
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
		mergeTypeImports(logic.Imports, collectTypeImports(source, typeFile, logic, nil)),
	)
	return logic, true
}

func loadLocalGeneratedLogicType(sourceFile, logicName string) (ParsedLogic, bool) {
	sourceFile = filepath.Clean(sourceFile)
	logicName = strings.TrimSpace(logicName)
	if sourceFile == "" || logicName == "" {
		return ParsedLogic{}, false
	}

	typeFile := typeFileNameForSource(AppOptions{}, sourceFile)
	sourceBytes, err := os.ReadFile(typeFile)
	if err != nil {
		return ParsedLogic{}, false
	}
	source := string(sourceBytes)
	typeName := logicName + "Type"

	logic, ok := parseGeneratedLogicTypeFile(typeFile, source, logicName, typeName)
	if !ok {
		return ParsedLogic{}, false
	}
	logic.Imports = mergeTypeImports(logic.Imports, collectTypeImports(source, typeFile, logic, nil))
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
		} else if resolvedFile, ok := resolveImportFile(fromFile, path, nil); ok {
			rootName, subpath := splitPackageImportPath(path)
			if rootName != "" && subpath == "" && resolvedImportLivesInAnyNodeModules(resolvedFile) && !sourceFileDirectlyImportsPath(toFile, path, nil) {
				if importPath, ok := relativeImportPath(toFile, resolvedFile); ok {
					path = importPath
				}
			}
		}
		rebased = append(rebased, TypeImport{Path: path, Names: append([]string(nil), item.Names...)})
	}
	return rebased
}

func resolvedImportLivesInAnyNodeModules(resolvedFile string) bool {
	if resolvedFile == "" {
		return false
	}
	resolvedFile = filepath.Clean(resolvedFile)
	return strings.Contains(resolvedFile, string(os.PathSeparator)+"node_modules"+string(os.PathSeparator))
}

func sourceFileDirectlyImportsPath(file, importPath string, state *buildState) bool {
	file = filepath.Clean(file)
	importPath = strings.TrimSpace(importPath)
	if file == "" || importPath == "" {
		return false
	}

	sourceBytes, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	source := string(sourceBytes)

	for _, candidate := range parseNamedValueImports(source) {
		if strings.TrimSpace(candidate.Path) == importPath {
			return true
		}
	}
	for _, candidate := range parseDefaultValueImports(source) {
		if strings.TrimSpace(candidate.Path) == importPath {
			return true
		}
	}
	for _, candidatePath := range parseNamespaceValueImports(source) {
		if strings.TrimSpace(candidatePath) == importPath {
			return true
		}
	}

	return strings.Contains(source, "from '"+importPath+"'") ||
		strings.Contains(source, `from "`+importPath+`"`) ||
		strings.Contains(source, "import '"+importPath+"'") ||
		strings.Contains(source, `import "`+importPath+`"`)
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
	relative = strings.TrimSuffix(relative, ".d.ts")
	relative = strings.TrimSuffix(relative, ".ts")
	relative = strings.TrimSuffix(relative, ".tsx")
	relative = strings.TrimSuffix(relative, ".js")
	relative = strings.TrimSuffix(relative, ".jsx")
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

	functionType := preferredMemberFunctionTypeText(actionMember)
	if functionType == "" {
		functionType = preferredMemberFunctionTypeText(creatorMember)
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

func trailingCallArguments(text string) ([]string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasSuffix(text, ")") {
		return nil, false
	}

	depth := 0
	for i := len(text) - 1; i >= 0; i-- {
		switch text[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				arguments, err := splitTopLevelList(text[i+1 : len(text)-1])
				if err != nil {
					return nil, false
				}
				return arguments, true
			}
		}
	}
	return nil, false
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

func findMemberReportInSections(sections []SectionReport, name string) (MemberReport, bool) {
	for _, section := range sections {
		if member, ok := findMemberReport(section.Members, name); ok {
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

func findParsedFunction(functions []ParsedFunction, name string) (ParsedFunction, bool) {
	for _, fn := range functions {
		if fn.Name == name {
			return fn, true
		}
	}
	return ParsedFunction{}, false
}

func parseReducerStateType(typeText string) (string, bool) {
	text := strings.TrimSpace(typeText)
	if text == "" {
		return "", false
	}
	if collapsed, ok := parseCollapsedReducerTupleStateType(text); ok {
		return collapsed, true
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

func parseCollapsedReducerTupleStateType(typeText string) (string, bool) {
	text := strings.TrimSpace(typeText)
	if !strings.HasSuffix(text, "[]") {
		return "", false
	}

	elementType := strings.TrimSpace(unwrapWrappedExpression(text[:len(text)-2]))
	if elementType == "" {
		return "", false
	}

	parts, err := splitTopLevelUnion(elementType)
	if err != nil || len(parts) < 2 {
		return "", false
	}

	stateType := ""
	handlerType := ""
	for _, part := range parts {
		part = normalizeInferredTypeText(strings.TrimSpace(unwrapWrappedExpression(part)))
		if part == "" {
			continue
		}
		if isReducerHandlersObjectType(part) {
			if handlerType == "" {
				handlerType = part
			}
			continue
		}
		if _, ok := parseObjectTypeMembers(part); ok {
			if handlerType == "" {
				handlerType = part
			}
			continue
		}
		if stateType == "" {
			stateType = part
			continue
		}
		stateType = mergeNormalizedTypeUnion(stateType, part)
	}

	if stateType == "" || handlerType == "" {
		return "", false
	}
	if widened := widenReducerStateTypeFromHandlers(stateType, handlerType); widened != "" {
		stateType = widened
	}
	return normalizeInferredTypeText(stateType), true
}

func isReducerHandlersObjectType(typeText string) bool {
	members, ok := parseObjectTypeMembers(typeText)
	if !ok || len(members) == 0 {
		return false
	}

	sawFunction := false
	for _, memberType := range members {
		if _, _, ok := splitFunctionType(memberType); !ok {
			return false
		}
		sawFunction = true
	}
	return sawFunction
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

func selectorMemberLacksReportedSurface(member MemberReport, preferredReturn string) bool {
	return strings.TrimSpace(member.TypeString) == "" &&
		strings.TrimSpace(member.ReturnTypeString) == "" &&
		strings.TrimSpace(preferredReturn) == ""
}

func selectorTypeNeedsSourceRecovery(typeText string) bool {
	text := normalizeInferredTypeText(strings.TrimSpace(typeText))
	if _, returnType, ok := splitFunctionType(unwrapWrappedExpression(text)); ok {
		return selectorTypeNeedsSourceRecovery(returnType)
	}
	switch {
	case text == "":
		return true
	case isLooselyTypedType(text):
		return true
	case text == "{}" || text == "{ }":
		return true
	case strings.HasPrefix(text, "typeof "):
		return true
	case isBuiltInNamespaceLikeType(text):
		return true
	case strings.HasSuffix(text, "Constructor"):
		return true
	default:
		return false
	}
}

func isBuiltInNamespaceLikeType(typeText string) bool {
	switch normalizeInferredTypeText(strings.TrimSpace(typeText)) {
	case "Math", "JSON", "Intl", "Reflect", "Atomics":
		return true
	default:
		return false
	}
}

func selectorReportedTypeNeedsSourceRecovery(reportedType, memberType string) bool {
	if selectorTypeNeedsSourceRecovery(reportedType) {
		return true
	}
	return !isFunctionLikeTypeText(normalizeInferredTypeText(strings.TrimSpace(reportedType))) &&
		selectorMemberHasLooseReportedReturn(memberType)
}

func selectorTypeNeedsSourceRecoveryFromExpression(typeText, expression string) bool {
	if selectorTypeNeedsSourceRecovery(typeText) {
		return true
	}
	return sourceSelectorReturnsFunction(expression) != isFunctionLikeTypeText(normalizeInferredTypeText(strings.TrimSpace(typeText)))
}

func sourceSelectorReturnsFunction(expression string) bool {
	projector := sourceSelectorProjectorElement(expression)
	if projector == "" {
		projector = expression
	}
	info, ok := parseSourceArrowInfo(projector)
	if !ok {
		return false
	}
	body := strings.TrimSpace(info.Body)
	if info.BlockBody {
		body = singleReturnExpression(body)
		if body == "" {
			body = blockReturnExpression(info.Body)
		}
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	if strings.HasPrefix(body, "function ") {
		return true
	}
	_, ok = parseSourceArrowInfo(body)
	return ok
}

func selectorReturnTypeConflicts(reported, parsed string) bool {
	reported = normalizeInferredTypeText(strings.TrimSpace(reported))
	parsed = normalizeInferredTypeText(strings.TrimSpace(parsed))
	if reported == "" || parsed == "" {
		return false
	}
	return reported != parsed
}

func selectorFunctionTypePreservesMoreOptionalUndefined(current, candidate string) bool {
	current = normalizeSourceTypeTextWithOptions(strings.TrimSpace(current), false)
	candidate = normalizeSourceTypeTextWithOptions(strings.TrimSpace(candidate), false)
	if current == "" || candidate == "" || current == candidate {
		return false
	}
	currentCollapsed := optionalUndefinedUnionPattern.ReplaceAllString(current, "$1")
	candidateCollapsed := optionalUndefinedUnionPattern.ReplaceAllString(candidate, "$1")
	if currentCollapsed != candidateCollapsed {
		return false
	}
	return strings.Count(candidate, "| undefined") > strings.Count(current, "| undefined")
}

func selectorFunctionTypePreservesMoreNullable(current, candidate string) bool {
	current = normalizeSourceTypeTextWithOptions(strings.TrimSpace(current), false)
	candidate = normalizeSourceTypeTextWithOptions(strings.TrimSpace(candidate), false)
	if current == "" || candidate == "" || current == candidate {
		return false
	}

	currentParameters, currentReturn, ok := splitFunctionType(current)
	if !ok {
		return false
	}
	candidateParameters, candidateReturn, ok := splitFunctionType(candidate)
	if !ok {
		return false
	}
	currentParams, ok := parseFunctionParameters(currentParameters)
	if !ok {
		return false
	}
	candidateParams, ok := parseFunctionParameters(candidateParameters)
	if !ok || len(currentParams) != len(candidateParams) {
		return false
	}
	for index := range currentParams {
		currentName, currentType, ok := splitTopLevelProperty(currentParams[index].Text)
		if !ok {
			return false
		}
		candidateName, candidateType, ok := splitTopLevelProperty(candidateParams[index].Text)
		if !ok {
			return false
		}
		if strings.TrimSpace(currentName) != strings.TrimSpace(candidateName) {
			return false
		}
		if normalizeInternalHelperParameterType(currentType) != normalizeInternalHelperParameterType(candidateType) {
			return false
		}
	}
	if normalizeInternalHelperParameterType(currentReturn) != normalizeInternalHelperParameterType(candidateReturn) {
		return false
	}
	return strings.Count(candidate, "null")+strings.Count(candidate, "undefined") > strings.Count(current, "null")+strings.Count(current, "undefined")
}

func parseObjectTypeMembers(typeText string) (map[string]string, bool) {
	return parseObjectTypeMembersWithOptionalUndefined(typeText, false)
}

func parseObjectTypeMembersWithOptionalUndefined(typeText string, includeOptionalUndefined bool) (map[string]string, bool) {
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
		rawName, value, ok := splitTopLevelPropertyRaw(entry)
		if !ok {
			continue
		}
		name := strings.TrimSpace(rawName)
		optional := strings.HasSuffix(name, "?")
		name = strings.TrimSuffix(name, "?")
		name = strings.Trim(name, `"'`)
		if name == "" || value == "" {
			continue
		}
		if includeOptionalUndefined && optional {
			value = mergeNormalizedTypeUnion(value, "undefined")
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

func normalizeFunctionParametersTextWithContext(source, file, parameters string, state *buildState) string {
	parts, ok := splitFunctionParameterParts(parameters)
	if !ok {
		return normalizeSourceTypeText(parameters)
	}

	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := normalizeParameterDeclarationTextWithContext(source, file, part, state); item != "" {
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
	text = normalizeSourceTypeTextWithOptions(baseText, false)
	if hadDefault {
		text = addOptionalMarkerToParameterText(text)
	}
	return text
}

func normalizeParameterDeclarationTextWithContext(source, file, part string, state *buildState) string {
	text := strings.TrimSpace(part)
	if text == "" {
		return ""
	}

	baseText, hadDefault := stripTopLevelParameterDefault(text)
	rawName, typeText, ok := splitTopLevelPropertyRaw(baseText)
	if ok {
		normalizedType := normalizeSourceTypeTextWithOptions(strings.TrimSpace(typeText), false)
		if expanded := expandIndexedAccessesInTypeText(source, file, normalizedType, state); expanded != "" {
			normalizedType = normalizeSourceTypeText(expanded)
		}
		text = strings.TrimSpace(rawName) + ": " + normalizedType
	} else {
		text = normalizeSourceTypeTextWithOptions(baseText, false)
	}
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
	case parsed == "any" && isReducerHandlersObjectType(current):
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

func preferredAssertedReducerStateType(current, asserted string) string {
	current = normalizeInferredTypeText(current)
	asserted = normalizeInferredTypeText(asserted)

	switch {
	case asserted == "":
		return current
	case current == "":
		return asserted
	case current == asserted:
		return current
	case isAnyLikeType(current):
		return asserted
	case current == "string" && selectorTypeIsLiteralUnionOfKind(asserted, "string"):
		return asserted
	case current == "number" && selectorTypeIsLiteralUnionOfKind(asserted, "number"):
		return asserted
	case current == "boolean" && selectorTypeIsLiteralUnionOfKind(asserted, "boolean"):
		return asserted
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
	text = sortLiteralUnionMembers(text)
	text = collapseBooleanLiteralUnion(text)
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
	text := normalizeInferredTypeText(typeText)
	if members, ok := parseActionPayloadObjectMembers(text); ok {
		return renderActionPayloadObjectMembers(members)
	}
	return text
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
	signatureOpaqueReturn := ""
	if strings.TrimSpace(info.ExplicitReturn) == "" {
		signatureOpaqueReturn = opaqueLoaderReturnTypeFromSignatureProbe(file, property, state)
	}
	probeNarrowReturn := false
	if returnType == "" {
		returnType = sourceReturnExpressionType(source, info.Body, info.BlockBody, info.ParameterNames, nil, info.Async)
		probeNarrowReturn = shouldProbeNarrowLoaderReturnType(info, returnType)
		if shouldProbeSourceArrowReturnType(returnType, info.ExplicitReturn) || probeNarrowReturn {
			if contextual := sourceReturnExpressionTypeWithContext(
				source,
				file,
				info.Body,
				info.BlockBody,
				info.ParameterNames,
				nil,
				info.Async,
				state,
			); shouldPreferContextualLoaderReturnType(returnType, contextual) {
				returnType = normalizeInferredTypeText(contextual)
			}
			body := strings.TrimSpace(info.Body)
			if info.BlockBody {
				body = singleReturnExpression(body)
				if body == "" {
					body = blockReturnExpression(info.Body)
				}
			}
			if body != "" {
				if inferred := sourceExpressionTypeTextWithContext(source, file, body, nil, state); inferred != "" {
					candidate := normalizeInferredTypeText(inferred)
					if info.Async {
						candidate = promiseTypeText(candidate)
					}
					if shouldPreferContextualLoaderReturnType(returnType, candidate) {
						returnType = candidate
					}
				}
			}
		}
	}
	if state != nil && file != "" && (shouldProbeSourceArrowReturnType(returnType, info.ExplicitReturn) || probeNarrowReturn) {
		if probed := sourceArrowReturnTypeFromTypeProbe(source, file, property, state); probed != "" {
			if !shouldIgnoreNullableLoaderProbeReturn(returnType, probed, info.Body) {
				returnType = probed
			}
		}
	}
	if shouldPreferOpaqueLoaderSignatureReturn(returnType, signatureOpaqueReturn) {
		returnType = signatureOpaqueReturn
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

func shouldPreferContextualLoaderReturnType(current, candidate string) bool {
	currentMain := normalizeSourceTypeText(strings.TrimSpace(unwrapPromiseType(current)))
	candidateMain := normalizeSourceTypeText(strings.TrimSpace(unwrapPromiseType(candidate)))
	if candidateMain == "" || currentMain == candidateMain {
		return false
	}
	_, _, candidateHasSingleMain := returnTypeMainType(candidate)
	if currentMain == "" {
		return candidateHasSingleMain
	}
	if typeTextNeedsSourceRecovery(currentMain) && !typeTextNeedsSourceRecovery(candidateMain) {
		return candidateHasSingleMain
	}
	return sourceBroadensPureNullishLoaderReturn(current, candidate)
}

func opaqueLoaderReturnTypeFromSignatureProbe(file string, property SourceProperty, state *buildState) string {
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

	for _, position := range []int{property.NameStart, property.ValueStart} {
		if position <= 0 {
			continue
		}
		symbol, err := state.apiClient.GetSymbolAtPosition(
			tsgoapi.WithTimeout(context.Background(), state.timeout),
			state.apiSnapshot,
			projectID,
			file,
			state.normalizedPositionForFile(file, position),
		)
		if err != nil || symbol == nil || symbol.ID == "" {
			continue
		}
		typeID := state.firstTypeOfSymbol(projectID, symbol.ID)
		if typeID == "" {
			typ, err := state.apiClient.GetTypeOfSymbol(
				tsgoapi.WithTimeout(context.Background(), state.timeout),
				state.apiSnapshot,
				projectID,
				symbol.ID,
			)
			if err == nil && typ != nil {
				typeID = typ.ID
			}
		}
		if typeID == "" {
			continue
		}
		signatureID := state.firstCallablePropertySignatureOnType(projectID, typeID)
		if signatureID == "" {
			continue
		}
		typeText := state.returnTypeToString(projectID, signatureID)
		if loaderOpaqueSignatureReturnShouldBePreserved(typeText) {
			return typeText
		}
	}
	return ""
}

func loaderOpaqueSignatureReturnShouldBePreserved(typeText string) bool {
	mainType := normalizeSourceTypeText(strings.TrimSpace(unwrapPromiseType(typeText)))
	if mainType == "" || isAnyLikeType(mainType) || mainType == "{}" || mainType == "{ }" {
		return false
	}
	return typeTextContainsStandaloneToken(mainType, "any") || typeTextContainsStandaloneToken(mainType, "unknown")
}

func shouldPreferOpaqueLoaderSignatureReturn(current, signature string) bool {
	current = normalizeSourceTypeText(strings.TrimSpace(current))
	signature = normalizeSourceTypeText(strings.TrimSpace(signature))
	if signature == "" || current == signature || !loaderOpaqueSignatureReturnShouldBePreserved(signature) {
		return false
	}
	if current == "" {
		return true
	}

	currentMain := normalizeSourceTypeText(strings.TrimSpace(unwrapPromiseType(current)))
	signatureMain := normalizeSourceTypeText(strings.TrimSpace(unwrapPromiseType(signature)))
	if currentMain == signatureMain {
		return false
	}
	if isAnyLikeType(currentMain) || currentMain == "{}" || currentMain == "{ }" {
		return true
	}
	if typeTextContainsStandaloneToken(signatureMain, "any") && !typeTextContainsStandaloneToken(currentMain, "any") {
		return true
	}
	return typeTextContainsStandaloneToken(signatureMain, "unknown") && !typeTextContainsStandaloneToken(currentMain, "unknown")
}

func shouldProbeNarrowLoaderReturnType(info sourceArrowInfo, returnType string) bool {
	if strings.TrimSpace(info.ExplicitReturn) != "" || !info.BlockBody {
		return false
	}
	text := normalizeInferredTypeText(strings.TrimSpace(unwrapPromiseType(returnType)))
	switch text {
	case "", "null", "undefined":
		return sourceHasMultipleReturnPaths(info.Body)
	}
	return widenLiteralReducerStateType(text) != text && sourceHasMultipleReturnPaths(info.Body)
}

func sourceHasMultipleReturnPaths(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	return strings.Count(body, "return") > 1 || strings.Contains(body, "catch")
}

func shouldIgnoreNullableLoaderProbeReturn(current, candidate, body string) bool {
	if sourceHasExplicitNullishReturnPath(body) {
		return false
	}

	currentMain, currentNullish, ok := returnTypeMainType(current)
	if !ok || currentNullish != "" {
		return false
	}
	candidateMain, candidateNullish, ok := returnTypeMainType(candidate)
	if !ok || currentMain != candidateMain {
		return false
	}
	return candidateNullish == "null" || candidateNullish == "null|undefined" || candidateNullish == "undefined"
}

func sourceHasExplicitNullishReturnPath(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	return regexp.MustCompile(`\breturn\s+(?:await\s+)?(?:null|undefined)\b`).MatchString(body)
}

func returnTypeMainType(typeText string) (string, string, bool) {
	return actionPayloadMainType(unwrapPromiseType(typeText))
}

func sourceArrowReturnTypeFromTypeProbe(source, file string, property SourceProperty, state *buildState) string {
	return sourceArrowReturnTypeFromTypeProbeRange(source, file, property, property.ValueStart, property.ValueEnd, state)
}

const sourceArrowFunctionSyntaxKind = 220

func sourceArrowReturnTypeFromLocationProbe(source, file string, property SourceProperty, state *buildState) string {
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
	expression := strings.TrimSpace(source[property.ValueStart:end])
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return ""
	}

	location := state.locationHandleForRange(projectID, file, property.ValueStart, end)
	if location == "" {
		location = fmt.Sprintf(
			"%d.%d.%d.%s",
			state.normalizedPositionForFile(file, property.ValueStart),
			state.normalizedPositionForFile(file, end),
			sourceArrowFunctionSyntaxKind,
			filepath.Clean(file),
		)
	}
	if _, returnType, ok := state.signatureTypesAtLocation(projectID, location); ok {
		returnType = normalizeSourceTypeText(returnType)
		if returnType != "" && !(returnType == "void" && !info.BlockBody) {
			return returnType
		}
	}

	typeText := state.cachedTypeAtLocationString(projectID, location)
	if returnType, ok := parseFunctionReturnType(typeText); ok {
		returnType = normalizeSourceTypeText(returnType)
		if returnType != "" && !(returnType == "void" && !info.BlockBody) {
			return returnType
		}
	}
	return sourceArrowReturnTypeFromTypeProbeRange(source, file, property, property.ValueStart, end, state)
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
	if location := state.locationHandleForRange(projectID, file, property.ValueStart, end); location != "" {
		if _, returnType, ok := state.signatureTypesAtLocation(projectID, location); ok {
			returnType = normalizeSourceTypeText(returnType)
			if returnType != "" {
				if !isAnyLikeType(returnType) && !typeTextContainsStandaloneToken(returnType, "any") && !typeTextContainsStandaloneToken(returnType, "unknown") {
					return returnType
				}
				fallback = returnType
			}
		} else if typeText := state.cachedTypeAtLocationString(projectID, location); typeText != "" {
			if returnType, ok := parseFunctionReturnType(typeText); ok {
				returnType = normalizeSourceTypeText(returnType)
				if returnType != "" {
					if !isAnyLikeType(returnType) && !typeTextContainsStandaloneToken(returnType, "any") && !typeTextContainsStandaloneToken(returnType, "unknown") {
						return returnType
					}
					fallback = returnType
				}
			}
		}
	}
	for _, position := range callbackTypeProbePositions(source, property.ValueStart, end) {
		returnType := normalizeSourceTypeText(state.cachedCallbackReturnTypeAtPositionString(projectID, file, position))
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
	if projectorStart, projectorEnd, ok := sourceSelectorProjectorRange(source, property); ok {
		start = projectorStart
		end = projectorEnd
		probeProperty.NameStart = projectorStart
		probeProperty.ValueStart = projectorStart
		probeProperty.ValueEnd = projectorEnd
	}
	if callbackReturn := sourceCallbackReturnTypeFromTypeProbe(source, file, probeProperty, state); callbackReturn != "" {
		return callbackReturn
	}
	return sourceArrowReturnTypeFromTypeProbeRange(source, file, probeProperty, start, end, state)
}

func sourceSelectorProjectorFunctionTypeFromTypeProbe(logic ParsedLogic, source, file string, property SourceProperty, state *buildState) string {
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

	start := property.ValueStart
	end := property.ValueEnd
	if projectorStart, projectorEnd, ok := sourceSelectorProjectorRange(source, property); ok {
		start = projectorStart
		end = projectorEnd
	}

	dependencyTypes := sourceSelectorDependencyTypes(logic, source, file, strings.TrimSpace(source[property.ValueStart:property.ValueEnd]), state)
	reconstructedParameters := sourceArrowParametersFromTypeProbe(source, file, projectID, start, end, state)
	if len(dependencyTypes) > 0 {
		reconstructedParameters = mergeRecoveredSelectorProjectorParameters(reconstructedParameters, dependencyTypes)
	}

	if location := state.locationHandleForRange(projectID, file, start, end); location != "" {
		if parameterTypes, returnType, ok := state.signatureTypesAtLocation(projectID, location); ok {
			parameterTypes = mergeRecoveredSelectorProjectorParameters(parameterTypes, dependencyTypes)
			if typeText := rebuildSourceArrowFunctionType(source[start:end], parameterTypes, returnType); typeText != "" {
				return normalizeSelectorFunctionTypeOptionalUndefined(normalizeSourceTypeText(typeText))
			}
		} else {
			typeText := normalizeSelectorFunctionTypeOptionalUndefined(normalizeSourceTypeText(state.cachedTypeAtLocationString(projectID, location)))
			if typeText != "" && isFunctionLikeTypeText(typeText) {
				parameters, returnType, ok := splitFunctionType(typeText)
				if ok {
					if reconstructed := applyProbedArrowParameters(parameters, reconstructedParameters); reconstructed != "" {
						typeText = reconstructed + " => " + normalizeInferredTypeText(strings.TrimSpace(returnType))
					}
				}
				if !isAnyLikeType(typeText) && !typeTextContainsStandaloneToken(typeText, "any") && !typeTextContainsStandaloneToken(typeText, "unknown") {
					return typeText
				}
			}
		}
	}

	if callbackReturn := sourceCallbackReturnTypeFromTypeProbe(source, file, SourceProperty{
		NameStart:  start,
		ValueStart: start,
		ValueEnd:   end,
	}, state); callbackReturn != "" {
		if typeText := rebuildSourceArrowFunctionType(source[start:end], reconstructedParameters, callbackReturn); typeText != "" {
			return normalizeSelectorFunctionTypeOptionalUndefined(normalizeSourceTypeText(typeText))
		}
	}

	positions := append([]int{start}, selectorTypeProbePositions(source, start, end)...)
	var fallback string
	for _, position := range positions {
		if position <= 0 {
			continue
		}
		typeText := normalizeSelectorFunctionTypeOptionalUndefined(normalizeSourceTypeText(state.cachedTypeAtPositionString(projectID, file, position)))
		if typeText == "" || !isFunctionLikeTypeText(typeText) {
			continue
		}
		parameters, returnType, ok := splitFunctionType(typeText)
		if !ok {
			continue
		}
		if reconstructed := applyProbedArrowParameters(parameters, reconstructedParameters); reconstructed != "" {
			typeText = reconstructed + " => " + normalizeInferredTypeText(strings.TrimSpace(returnType))
		}
		if !isAnyLikeType(returnType) && !typeTextContainsStandaloneToken(returnType, "any") && !typeTextContainsStandaloneToken(returnType, "unknown") {
			return typeText
		}
		if fallback == "" {
			fallback = typeText
		}
	}
	return fallback
}

func mergeRecoveredSelectorProjectorParameters(recovered, dependencyTypes []string) []string {
	if len(dependencyTypes) == 0 {
		return recovered
	}
	if len(recovered) == 0 {
		result := make([]string, 0, len(dependencyTypes))
		for _, dependencyType := range dependencyTypes {
			dependencyType = normalizeSourceTypeText(strings.TrimSpace(dependencyType))
			if dependencyType == "" {
				return nil
			}
			result = append(result, dependencyType)
		}
		return result
	}
	if len(recovered) != len(dependencyTypes) {
		return recovered
	}

	result := append([]string(nil), recovered...)
	for index := range result {
		if !isAnyLikeType(result[index]) &&
			!typeTextContainsStandaloneToken(result[index], "any") &&
			!typeTextContainsStandaloneToken(result[index], "unknown") {
			continue
		}
		dependencyType := normalizeSourceTypeText(strings.TrimSpace(dependencyTypes[index]))
		if dependencyType == "" {
			continue
		}
		result[index] = dependencyType
	}
	return result
}

func sourceArrowParametersFromTypeProbe(
	source,
	file,
	projectID string,
	valueStart,
	valueEnd int,
	state *buildState,
) []string {
	if state == nil || file == "" || projectID == "" || valueEnd <= valueStart {
		return nil
	}

	expression := strings.TrimSpace(source[valueStart:valueEnd])
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return nil
	}

	parameterParts, ok := splitFunctionParameterParts(info.Parameters)
	if !ok {
		return nil
	}
	parameterTypes, ok := sourceArrowParameterTypesFromTypeProbe(source[valueStart:valueEnd], info.Parameters, parameterParts, projectID, file, state, valueStart)
	if !ok || len(parameterTypes) != len(parameterParts) {
		return nil
	}
	return parameterTypes
}

func sourceArrowParameterTypesFromTypeProbe(
	expression,
	parameters string,
	parts []string,
	projectID,
	file string,
	state *buildState,
	valueStart int,
) ([]string, bool) {
	if len(parts) == 0 {
		return nil, false
	}

	parametersOffset := strings.Index(expression, parameters)
	if parametersOffset == -1 {
		return nil, false
	}
	parametersStart := valueStart + parametersOffset
	cursor := 0

	types := make([]string, 0, len(parts))
	parametersText := parameters
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}

		explicitType := ""
		if parsed, ok := parseFunctionParameters("(" + part + ")"); ok && len(parsed) == 1 {
			explicitType = normalizeSourceTypeText(strings.TrimSpace(parsed[0].Type))
			if explicitType != "" && parameterDeclarationIsOptional(part) && !strings.Contains(explicitType, "undefined") {
				explicitType = normalizeSourceTypeText(explicitType + " | undefined")
			}
		}
		if explicitType != "" {
			types = append(types, explicitType)
			next := strings.Index(parametersText[cursor:], part)
			if next != -1 {
				cursor += next + len(part)
			}
			continue
		}

		name, ok := sourceParameterName(part)
		if !ok || name == "" {
			return nil, false
		}
		partIndex := strings.Index(parametersText[cursor:], part)
		if partIndex == -1 {
			return nil, false
		}
		partStart := cursor + partIndex
		nameIndex := strings.Index(part, name)
		if nameIndex == -1 {
			return nil, false
		}
		position := parametersStart + partStart + nameIndex
		typeText := normalizeSourceTypeText(state.cachedTypeAtPositionString(projectID, file, position))
		if typeText == "" {
			return nil, false
		}
		if parameterDeclarationIsOptional(part) && !strings.Contains(typeText, "undefined") {
			typeText = normalizeSourceTypeText(typeText + " | undefined")
		}
		types = append(types, typeText)
		cursor = partStart + len(part)
	}

	return types, true
}

func applyProbedArrowParameters(parameters string, parameterTypes []string) string {
	if len(parameterTypes) == 0 {
		return ""
	}

	parts, ok := splitFunctionParameterParts(parameters)
	if !ok || len(parts) != len(parameterTypes) {
		return ""
	}

	rebuilt := make([]string, 0, len(parts))
	for index, part := range parts {
		name, ok := sourceParameterName(part)
		if !ok || name == "" {
			return ""
		}
		rebuilt = append(rebuilt, fmt.Sprintf("%s: %s", name, parameterTypes[index]))
	}
	return "(" + strings.Join(rebuilt, ", ") + ")"
}

func rebuildSourceArrowFunctionType(expression string, parameterTypes []string, returnType string) string {
	expression = strings.TrimSpace(expression)
	info, ok := parseSourceArrowInfo(expression)
	if !ok {
		return ""
	}

	returnType = normalizeInferredTypeText(strings.TrimSpace(returnType))
	if returnType == "" {
		return ""
	}

	parts, ok := splitFunctionParameterParts(info.Parameters)
	if !ok {
		return ""
	}
	if len(parts) != len(parameterTypes) {
		return ""
	}
	if len(parts) == 0 {
		return "() => " + returnType
	}

	rebuilt := make([]string, 0, len(parts))
	for index, part := range parts {
		name, ok := sourceParameterName(part)
		if !ok || name == "" {
			return ""
		}
		rebuilt = append(rebuilt, fmt.Sprintf("%s: %s", name, parameterTypes[index]))
	}
	return "(" + strings.Join(rebuilt, ", ") + ") => " + returnType
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
		typeText := normalizeSourceTypeText(state.cachedTypeAtPositionString(projectID, file, position))
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
		typeText := normalizeSourceTypeText(state.cachedSignatureReturnTypeAtPositionString(projectID, file, position))
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

func collectTypeImports(source, file string, logic ParsedLogic, state *buildState) []TypeImport {
	return collectTypeImportsForTypeTexts(
		source,
		file,
		collectUsedTypeTexts(logic),
		state,
		existingImportReferenceNames(logic.Imports),
	)
}

func collectTypeImportsForTypeTexts(source, file string, typeTexts []string, state *buildState, coveredIdentifiers map[string]bool) []TypeImport {
	if len(typeTexts) == 0 {
		return nil
	}
	if coveredIdentifiers == nil {
		coveredIdentifiers = map[string]bool{}
	}

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
	importPaths := parseAllImportPaths(source)
	exportCache := map[string]map[string]bool{}
	packageExportCache := map[string]map[string]importCandidate{}
	usedReferences := collectUsedTypeReferences(typeTexts)
	for identifier := range collectExplicitImportableBareIdentifiers(typeTexts, importsByAlias) {
		usedReferences.BareIdentifiers[identifier] = true
	}

	grouped := map[string]map[string]bool{}
	for identifier := range usedReferences.QualifiedOwners {
		if coveredIdentifiers[identifier] {
			continue
		}
		candidate, ok := importsByAlias[identifier]
		if !ok || candidate.Path == "" || candidate.Name == "" {
			continue
		}
		if parityModeShouldOmitCollectedImportPath(candidate.Path) {
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
		if !ok && len(importPaths) > 0 {
			candidate, ok = resolveImportedExportCandidate(file, importPaths, identifier, state, exportCache, packageExportCache)
		}
		if !ok || candidate.Path == "" || candidate.Name == "" {
			continue
		}
		if parityModeShouldOmitCollectedImportPath(candidate.Path) {
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

func parityModeShouldOmitCollectedImportPath(importPath string) bool {
	if !parityModeEnabled() {
		return false
	}
	importPath = strings.TrimSpace(importPath)
	return importPath == "reactflow" ||
		importPath == "@reactflow/core" ||
		strings.HasPrefix(importPath, "@reactflow/core/")
}

func collectConnectedImportsFromSource(logic ParsedLogic, typeTexts []string, state *buildState) []TypeImport {
	if logic.File == "" || len(typeTexts) == 0 {
		return nil
	}
	sourceBytes, err := os.ReadFile(logic.File)
	if err != nil {
		return nil
	}
	return collectTypeImportsForTypeTexts(
		string(sourceBytes),
		logic.File,
		typeTexts,
		state,
		existingImportReferenceNames(logic.Imports),
	)
}

func parseAllImportPaths(source string) []string {
	seen := map[string]bool{}
	var paths []string
	matches := importClausePattern.FindAllStringSubmatch(source, -1)
	for _, match := range matches {
		importPath := strings.TrimSpace(match[2])
		if importPath != "" && !seen[importPath] {
			seen[importPath] = true
			paths = append(paths, importPath)
		}
	}
	mixedMatches := importDefaultNamedPattern.FindAllStringSubmatch(source, -1)
	for _, match := range mixedMatches {
		importPath := strings.TrimSpace(match[3])
		if importPath != "" && !seen[importPath] {
			seen[importPath] = true
			paths = append(paths, importPath)
		}
	}
	defaultMatches := importDefaultPattern.FindAllStringSubmatch(source, -1)
	for _, match := range defaultMatches {
		importPath := strings.TrimSpace(match[2])
		if importPath != "" && !seen[importPath] {
			seen[importPath] = true
			paths = append(paths, importPath)
		}
	}
	namespaceMatches := importNamespacePattern.FindAllStringSubmatch(source, -1)
	for _, match := range namespaceMatches {
		importPath := strings.TrimSpace(match[2])
		if importPath != "" && !seen[importPath] {
			seen[importPath] = true
			paths = append(paths, importPath)
		}
	}
	return paths
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
		texts = append(texts, actionImportTypeTexts(action)...)
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
		texts = append(texts, actionImportTypeTexts(action)...)
	}
	if logic.CustomType != "" {
		texts = append(texts, logic.CustomType)
	}
	if logic.ExtraInputForm != "" {
		texts = append(texts, logic.ExtraInputForm)
	}
	return texts
}

func actionImportTypeTexts(action ParsedAction) []string {
	texts := make([]string, 0, 2)
	if parameters, _, ok := splitFunctionType(action.FunctionType); ok {
		texts = append(texts, parameters)
	}
	if action.PayloadType != "" {
		texts = append(texts, action.PayloadType)
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

func appendImportPath(target *[]string, seen map[string]bool, importPath string, relativeOnly bool) {
	if importPath == "" {
		return
	}
	if relativeOnly {
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

func resolveImportedExportCandidate(
	file string,
	importPaths []string,
	identifier string,
	state *buildState,
	exportCache map[string]map[string]bool,
	packageExportCache map[string]map[string]importCandidate,
) (importCandidate, bool) {
	matches := make([]importCandidate, 0, 1)
	seen := map[string]bool{}
	for _, importPath := range importPaths {
		if resolvedFile, ok := resolveImportFile(file, importPath, state); ok && exportedTypeNames(resolvedFile, "", exportCache)[identifier] {
			candidate := importCandidate{Path: importPath, Name: identifier}
			key := candidate.Path + "::" + candidate.Name
			if !seen[key] {
				seen[key] = true
				matches = append(matches, candidate)
			}
		} else if !strings.HasPrefix(importPath, ".") {
			candidate, ok := resolvePackageExportCandidate(file, []string{importPath}, identifier, packageExportCache)
			if ok {
				key := candidate.Path + "::" + candidate.Name
				if !seen[key] {
					seen[key] = true
					matches = append(matches, candidate)
				}
			}
		}
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
			Main    string `json:"main"`
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
			if strings.TrimSpace(packageJSON.Main) != "" {
				if resolved, ok := resolvePackageModuleFile(rootDir, packageJSON.Main); ok {
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
	modulePath = strings.TrimSuffix(modulePath, ".js")
	modulePath = strings.TrimSuffix(modulePath, ".jsx")
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
	if resolvedFile, ok := resolveConfiguredImportFile(importPath, state); ok {
		return resolvedFile, true
	}
	return resolvePackageImportFile(file, importPath)
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

func resolvePackageImportFile(file, importPath string) (string, bool) {
	if file == "" || importPath == "" || strings.HasPrefix(importPath, ".") {
		return "", false
	}
	rootDir, _, subpath, ok := resolvePackageRoot(file, importPath)
	if !ok {
		return "", false
	}
	return resolvePackageTypesEntryFile(rootDir, subpath)
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

func collectExplicitImportableBareIdentifiers(typeTexts []string, candidates map[string]importCandidate) map[string]bool {
	used := map[string]bool{}
	if len(typeTexts) == 0 || len(candidates) == 0 {
		return used
	}
	for _, typeText := range typeTexts {
		collectExplicitImportableBareIdentifiersInto(typeText, candidates, used)
	}
	return used
}

func collectExplicitImportableBareIdentifiersInto(typeText string, candidates map[string]importCandidate, used map[string]bool) {
	for i := 0; i < len(typeText); {
		switch typeText[i] {
		case '\'':
			end, err := skipQuoted(typeText, i, '\'')
			if err != nil {
				return
			}
			i = end + 1
			continue
		case '"':
			end, err := skipQuoted(typeText, i, '"')
			if err != nil {
				return
			}
			i = end + 1
			continue
		case '`':
			end, err := skipTemplate(typeText, i)
			if err != nil {
				return
			}
			i = end + 1
			continue
		}
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
		candidate, ok := candidates[identifier]
		if !ok || candidate.Path == "" || candidate.Name == "" {
			continue
		}
		previous := previousNonWhitespaceByte(typeText, start)
		next := nextNonWhitespaceByte(typeText, i)
		if previous == '.' || next == '.' {
			continue
		}
		if next == ':' {
			continue
		}
		if next == '?' && nextNonWhitespaceByte(typeText, i+1) == ':' {
			continue
		}
		used[identifier] = true
	}
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
