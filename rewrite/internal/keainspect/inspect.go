package keainspect

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"kea-typegen/rewrite/internal/tsgoapi"
)

type MemberReport struct {
	Name                                 string `json:"name"`
	TypeString                           string `json:"typeString,omitempty"`
	PrintedTypeNode                      string `json:"printedTypeNode,omitempty"`
	ReturnTypeString                     string `json:"returnTypeString,omitempty"`
	PrintedReturnTypeNode                string `json:"printedReturnTypeNode,omitempty"`
	SelectorInputReturnTypeString        string `json:"selectorInputReturnTypeString,omitempty"`
	SelectorInputPrintedReturnTypeNode   string `json:"selectorInputPrintedReturnTypeNode,omitempty"`
	ProjectorDirectTypeString            string `json:"projectorDirectTypeString,omitempty"`
	ProjectorDirectPrintedTypeNode       string `json:"projectorDirectPrintedTypeNode,omitempty"`
	ProjectorDirectReturnTypeString      string `json:"projectorDirectReturnTypeString,omitempty"`
	ProjectorDirectPrintedReturnTypeNode string `json:"projectorDirectPrintedReturnTypeNode,omitempty"`
	ProjectorTypeString                  string `json:"projectorTypeString,omitempty"`
	ProjectorPrintedTypeNode             string `json:"projectorPrintedTypeNode,omitempty"`
	ProjectorReturnTypeString            string `json:"projectorReturnTypeString,omitempty"`
	ProjectorPrintedReturnTypeNode       string `json:"projectorPrintedReturnTypeNode,omitempty"`
	SignatureCount                       int    `json:"signatureCount,omitempty"`
}

type SectionReport struct {
	Name                string                  `json:"name"`
	Position            int                     `json:"position"`
	RawTypeString       string                  `json:"rawTypeString,omitempty"`
	EffectiveTypeString string                  `json:"effectiveTypeString,omitempty"`
	PrintedTypeNode     string                  `json:"printedTypeNode,omitempty"`
	Members             []MemberReport          `json:"members,omitempty"`
	Symbol              *tsgoapi.SymbolResponse `json:"symbol,omitempty"`
	Error               string                  `json:"error,omitempty"`
}

type LogicReport struct {
	Name      string          `json:"name"`
	InputKind string          `json:"inputKind"`
	Sections  []SectionReport `json:"sections"`
}

type Report struct {
	BinaryPath         string                      `json:"binaryPath"`
	ProjectDir         string                      `json:"projectDir"`
	ConfigFile         string                      `json:"configFile"`
	File               string                      `json:"file"`
	Initialize         *tsgoapi.InitializeResponse `json:"initialize,omitempty"`
	Config             *tsgoapi.ConfigResponse     `json:"config,omitempty"`
	Snapshot           string                      `json:"snapshot,omitempty"`
	Project            *tsgoapi.ProjectResponse    `json:"project,omitempty"`
	DefaultProjectFile string                      `json:"defaultProjectFile,omitempty"`
	Logics             []LogicReport               `json:"logics"`
}

type InspectOptions struct {
	BinaryPath string
	ProjectDir string
	ConfigFile string
	File       string
	Timeout    time.Duration
}

func InspectFile(ctx context.Context, options InspectOptions) (*Report, error) {
	sourceBytes, err := os.ReadFile(options.File)
	if err != nil {
		return nil, err
	}
	sourceText := string(sourceBytes)
	offsets := newSourceOffsetMap(sourceText)

	logics, err := FindLogics(sourceText)
	if err != nil {
		return nil, err
	}

	if err := tsgoapi.Preflight(options.BinaryPath, options.ProjectDir); err != nil {
		return nil, err
	}

	client, err := tsgoapi.Start(options.ProjectDir, options.BinaryPath)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	initialize, err := client.Initialize(tsgoapi.WithTimeout(ctx, options.Timeout))
	if err != nil {
		return nil, err
	}
	config, err := client.ParseConfigFile(tsgoapi.WithTimeout(ctx, options.Timeout), options.ConfigFile)
	if err != nil {
		return nil, err
	}
	snapshot, err := client.UpdateSnapshot(tsgoapi.WithTimeout(ctx, options.Timeout), options.ConfigFile)
	if err != nil {
		return nil, err
	}
	project := tsgoapi.PickProject(snapshot.Projects, options.ConfigFile)
	if project == nil {
		return nil, fmt.Errorf("no project returned for %s", options.ConfigFile)
	}
	defaultProject, err := client.GetDefaultProjectForFile(
		tsgoapi.WithTimeout(ctx, options.Timeout),
		snapshot.Snapshot,
		options.File,
	)
	if err != nil {
		return nil, err
	}

	report := &Report{
		BinaryPath:         options.BinaryPath,
		ProjectDir:         options.ProjectDir,
		ConfigFile:         options.ConfigFile,
		File:               options.File,
		Initialize:         initialize,
		Config:             config,
		Snapshot:           snapshot.Snapshot,
		Project:            project,
		DefaultProjectFile: defaultProject.ConfigFileName,
	}

	for _, logic := range logics {
		report.Logics = append(report.Logics, inspectLogic(ctx, client, options.Timeout, snapshot.Snapshot, project.ID, options.File, sourceText, offsets, logic))
	}

	return report, nil
}

func inspectLogic(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	file string,
	source string,
	offsets sourceOffsetMap,
	logic SourceLogic,
) LogicReport {
	report := LogicReport{Name: logic.Name, InputKind: logic.InputKind}
	for _, property := range logic.Properties {
		report.Sections = append(report.Sections, inspectSection(ctx, client, timeout, snapshot, projectID, file, source, offsets, property, logic.InputKind))
	}

	return report
}

func inspectSection(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	file string,
	source string,
	offsets sourceOffsetMap,
	property SourceProperty,
	inputKind string,
) SectionReport {
	symbolPosition := property.NameStart
	if len(property.Name) > 1 {
		symbolPosition++
	}
	symbolPosition = offsets.utf16Offset(symbolPosition)
	valuePosition := offsets.utf16Offset(property.ValueStart)

	report := SectionReport{
		Name:     property.Name,
		Position: symbolPosition,
	}

	var (
		rawType *tsgoapi.TypeResponse
		err     error
	)
	symbol, symbolErr := client.GetSymbolAtPosition(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, file, symbolPosition)
	if symbolErr == nil {
		report.Symbol = symbol
	}

	if inputKind == "builders" {
		rawType, err = client.GetTypeAtPosition(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, file, valuePosition)
	} else {
		if symbolErr != nil {
			report.Error = symbolErr.Error()
			return report
		}
		if symbol == nil {
			report.Error = "no symbol returned"
			return report
		}
		rawType, err = client.GetTypeOfSymbol(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, symbol.ID)
	}
	if err != nil {
		report.Error = err.Error()
		return report
	}
	if rawType == nil {
		report.Error = "no type returned"
		return report
	}

	report.RawTypeString = safeTypeString(ctx, client, timeout, snapshot, projectID, rawType.ID)
	effectiveType := rawType

	signatures, err := client.GetSignaturesOfType(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, rawType.ID)
	if err == nil && len(signatures) > 0 {
		returnType, returnErr := client.GetReturnTypeOfSignature(
			tsgoapi.WithTimeout(ctx, timeout),
			snapshot,
			projectID,
			signatures[0].ID,
		)
		if returnErr == nil && returnType != nil {
			effectiveType = returnType
		}
	}

	report.EffectiveTypeString = safeTypeString(ctx, client, timeout, snapshot, projectID, effectiveType.ID)
	if printed, err := client.PrintTypeNode(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, effectiveType.ID); err == nil {
		report.PrintedTypeNode = printed
	}

	shouldUseFallback := false
	if inputKind == "builders" {
		if _, _, ok, _ := FindInspectableObjectLiteral(source, property.ValueStart, property.ValueEnd); ok {
			shouldUseFallback = true
		}
	}

	if shouldEnumerateMembers(report.EffectiveTypeString, shouldUseFallback) {
		members, err := client.GetPropertiesOfType(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, effectiveType.ID)
		if err != nil {
			report.Error = appendError(report.Error, err.Error())
		} else {
			for _, member := range members {
				item := MemberReport{Name: member.Name}
				memberType, err := client.GetTypeOfSymbol(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, member.ID)
				if err == nil && memberType != nil {
					item.TypeString = safeTypeString(ctx, client, timeout, snapshot, projectID, memberType.ID)
					if printed, err := client.PrintTypeNode(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, memberType.ID); err == nil {
						item.PrintedTypeNode = printed
					}
					memberSignatures, sigErr := client.GetSignaturesOfType(
						tsgoapi.WithTimeout(ctx, timeout),
						snapshot,
						projectID,
						memberType.ID,
					)
					if sigErr == nil && len(memberSignatures) > 0 {
						item.SignatureCount = len(memberSignatures)
						returnType, returnErr := client.GetReturnTypeOfSignature(
							tsgoapi.WithTimeout(ctx, timeout),
							snapshot,
							projectID,
							memberSignatures[0].ID,
						)
						if returnErr == nil && returnType != nil {
							item.ReturnTypeString = safeTypeString(ctx, client, timeout, snapshot, projectID, returnType.ID)
							if printed, err := client.PrintTypeNode(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, returnType.ID); err == nil {
								item.PrintedReturnTypeNode = printed
							}
						}
					}
				}
				report.Members = append(report.Members, item)
			}
		}
	}

	if shouldUseFallback {
		fallbackMembers, fallbackErr := inspectBuilderFallbackMembers(ctx, client, timeout, snapshot, projectID, file, source, offsets, property)
		if fallbackErr != nil {
			report.Error = appendError(report.Error, fallbackErr.Error())
		} else if len(fallbackMembers) > 0 {
			report.Members = fallbackMembers
		}
	}

	if property.Name == "selectors" {
		report.Members = enrichSelectorCallbackTypes(ctx, client, timeout, snapshot, projectID, file, source, offsets, property, report.Members)
	}

	sort.Slice(report.Members, func(i, j int) bool {
		return report.Members[i].Name < report.Members[j].Name
	})

	return report
}

func safeTypeString(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	typeID string,
) string {
	text, err := client.TypeToString(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, typeID)
	if err != nil {
		return ""
	}
	return text
}

func inspectBuilderFallbackMembers(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	file string,
	source string,
	offsets sourceOffsetMap,
	property SourceProperty,
) ([]MemberReport, error) {
	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(source, property.ValueStart, property.ValueEnd)
	if err != nil || !ok {
		return nil, err
	}

	properties, err := parseTopLevelProperties(source, objectStart, objectEnd)
	if err != nil {
		return nil, err
	}

	members := make([]MemberReport, 0, len(properties))
	for _, nested := range properties {
		position := nested.NameStart
		if len(nested.Name) > 1 {
			position++
		}
		position = offsets.utf16Offset(position)

		item := MemberReport{Name: nested.Name}
		symbol, err := client.GetSymbolAtPosition(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, file, position)
		if err == nil && symbol != nil {
			memberType, typeErr := client.GetTypeOfSymbol(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, symbol.ID)
			if typeErr == nil && memberType != nil {
				item.TypeString = safeTypeString(ctx, client, timeout, snapshot, projectID, memberType.ID)
				memberSignatures, sigErr := client.GetSignaturesOfType(
					tsgoapi.WithTimeout(ctx, timeout),
					snapshot,
					projectID,
					memberType.ID,
				)
				if sigErr == nil && len(memberSignatures) > 0 {
					item.SignatureCount = len(memberSignatures)
					returnType, returnErr := client.GetReturnTypeOfSignature(
						tsgoapi.WithTimeout(ctx, timeout),
						snapshot,
						projectID,
						memberSignatures[0].ID,
					)
					if returnErr == nil && returnType != nil {
						item.ReturnTypeString = safeTypeString(ctx, client, timeout, snapshot, projectID, returnType.ID)
					}
				}
			}
		}
		members = append(members, item)
	}

	sort.Slice(members, func(i, j int) bool {
		return members[i].Name < members[j].Name
	})
	return members, nil
}

func appendError(existing, next string) string {
	if existing == "" {
		return next
	}
	return existing + "; " + next
}

func shouldEnumerateMembers(effectiveType string, hasBuilderFallback bool) bool {
	if hasBuilderFallback {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(effectiveType), "{")
}

func enrichSelectorCallbackTypes(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	file string,
	source string,
	offsets sourceOffsetMap,
	property SourceProperty,
	members []MemberReport,
) []MemberReport {
	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(source, property.ValueStart, property.ValueEnd)
	if err != nil || !ok {
		return members
	}

	nestedProperties, err := parseTopLevelProperties(source, objectStart, objectEnd)
	if err != nil {
		return members
	}

	nodes, hasNodes := inspectSourceFileNodes(ctx, client, timeout, snapshot, projectID, file, offsets)

	indexByName := map[string]int{}
	for index, member := range members {
		indexByName[member.Name] = index
	}

	for _, nested := range nestedProperties {
		index, ok := indexByName[nested.Name]
		if !ok {
			continue
		}

		if hasNodes {
			if elementStart, elementEnd, ok, err := findFirstFunctionLikeTopLevelArrayElement(source, nested.ValueStart, nested.ValueEnd); err == nil && ok {
				location := sourceNodeLocationHandleForRange(
					nodes,
					offsets.utf16Offset(elementStart),
					offsets.utf16Offset(elementEnd),
					file,
				)
				if location != "" {
					inputType, err := client.GetTypeAtLocation(
						tsgoapi.WithTimeout(ctx, timeout),
						snapshot,
						projectID,
						location,
					)
					if err == nil && inputType != nil {
						members[index] = populateSelectorInputTypeReport(ctx, client, timeout, snapshot, projectID, members[index], inputType.ID)
					}
				}
			}
		}

		elementStart, elementEnd, ok, err := FindLastFunctionLikeTopLevelArrayElement(source, nested.ValueStart, nested.ValueEnd)
		if err != nil || !ok {
			continue
		}

		if hasNodes {
			location := sourceNodeLocationHandleForRange(
				nodes,
				offsets.utf16Offset(elementStart),
				offsets.utf16Offset(elementEnd),
				file,
			)
			if location != "" {
				projectorType, err := client.GetTypeAtLocation(
					tsgoapi.WithTimeout(ctx, timeout),
					snapshot,
					projectID,
					location,
				)
				if err == nil && projectorType != nil {
					members[index] = populateProjectorDirectTypeReport(ctx, client, timeout, snapshot, projectID, members[index], projectorType.ID)
				}

				contextualType, err := client.GetContextualType(
					tsgoapi.WithTimeout(ctx, timeout),
					snapshot,
					projectID,
					location,
				)
				if err == nil && contextualType != nil {
					members[index] = populateProjectorContextualTypeReport(ctx, client, timeout, snapshot, projectID, members[index], contextualType.ID)
				}
			}
		}

		if !selectorReturnTypeNeedsRecovery(members[index]) {
			continue
		}
		if directReturn := strings.TrimSpace(members[index].ProjectorDirectReturnTypeString); directReturn != "" {
			members[index].ReturnTypeString = directReturn
			if printed := strings.TrimSpace(members[index].ProjectorDirectPrintedReturnTypeNode); printed != "" {
				members[index].PrintedReturnTypeNode = printed
			}
			continue
		}
		if typeText := selectorReturnTypeFromSourceProbe(ctx, client, timeout, snapshot, projectID, file, source, offsets, elementStart, elementEnd); typeText != "" {
			members[index].ReturnTypeString = typeText
			continue
		}
		if typeText := selectorReturnTypeFromSignature(ctx, client, timeout, snapshot, projectID, file, offsets, elementStart); typeText != "" {
			members[index].ReturnTypeString = typeText
		}
	}

	return members
}

func inspectSourceFileNodes(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	file string,
	offsets sourceOffsetMap,
) ([]sourceFileNode, bool) {
	raw, err := client.CallRaw(
		tsgoapi.WithTimeout(ctx, timeout),
		"getSourceFile",
		map[string]any{
			"snapshot": snapshot,
			"project":  projectID,
			"file":     file,
		},
	)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}

	var payload struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Data == "" {
		return nil, false
	}

	blob, err := decodeBase64String(payload.Data)
	if err != nil {
		return nil, false
	}
	nodes, err := parseSourceFileNodes(blob, offsets.utf16Length())
	if err != nil {
		return nil, false
	}
	return nodes, len(nodes) > 0
}

func typeSurfaceReport(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	typeID string,
) (string, string, string, string) {
	typeString := safeTypeString(ctx, client, timeout, snapshot, projectID, typeID)
	printedType := ""
	if printed, err := client.PrintTypeNode(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, typeID); err == nil {
		printedType = printed
	}

	signatures, err := client.GetSignaturesOfType(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, typeID)
	if err != nil || len(signatures) == 0 || signatures[0] == nil {
		return typeString, printedType, "", ""
	}

	returnType, err := client.GetReturnTypeOfSignature(
		tsgoapi.WithTimeout(ctx, timeout),
		snapshot,
		projectID,
		signatures[0].ID,
	)
	if err != nil || returnType == nil {
		return typeString, printedType, "", ""
	}

	returnTypeString := safeTypeString(ctx, client, timeout, snapshot, projectID, returnType.ID)
	printedReturnType := ""
	if printed, err := client.PrintTypeNode(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, returnType.ID); err == nil {
		printedReturnType = printed
	}
	return typeString, printedType, returnTypeString, printedReturnType
}

func populateSelectorInputTypeReport(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	member MemberReport,
	typeID string,
) MemberReport {
	_, _, returnTypeString, printedReturnType := typeSurfaceReport(ctx, client, timeout, snapshot, projectID, typeID)
	member.SelectorInputReturnTypeString = returnTypeString
	member.SelectorInputPrintedReturnTypeNode = printedReturnType
	return member
}

func populateProjectorDirectTypeReport(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	member MemberReport,
	typeID string,
) MemberReport {
	typeString, printedType, returnTypeString, printedReturnType := typeSurfaceReport(ctx, client, timeout, snapshot, projectID, typeID)
	member.ProjectorDirectTypeString = typeString
	member.ProjectorDirectPrintedTypeNode = printedType
	member.ProjectorDirectReturnTypeString = returnTypeString
	member.ProjectorDirectPrintedReturnTypeNode = printedReturnType
	return member
}

func populateProjectorContextualTypeReport(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	member MemberReport,
	typeID string,
) MemberReport {
	typeString, printedType, returnTypeString, printedReturnType := typeSurfaceReport(ctx, client, timeout, snapshot, projectID, typeID)
	member.ProjectorTypeString = typeString
	member.ProjectorPrintedTypeNode = printedType
	member.ProjectorReturnTypeString = returnTypeString
	member.ProjectorPrintedReturnTypeNode = printedReturnType
	return member
}

func findFirstFunctionLikeTopLevelArrayElement(source string, valueStart, valueEnd int) (int, int, bool, error) {
	elements, err := FindTopLevelArrayElements(source, valueStart, valueEnd)
	if err != nil {
		return 0, 0, false, err
	}
	for _, element := range elements {
		if element.End <= element.Start {
			continue
		}
		if !isFunctionLikeTopLevelArrayElement(strings.TrimSpace(source[element.Start:element.End])) {
			continue
		}
		return element.Start, element.End, true, nil
	}
	return 0, 0, false, nil
}

func selectorReturnTypeFromSourceProbe(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	file string,
	source string,
	offsets sourceOffsetMap,
	elementStart int,
	elementEnd int,
) string {
	probePosition, ok, err := FindArrowFunctionReturnProbe(source, elementStart, elementEnd)
	if err != nil || !ok {
		return ""
	}

	probeEnd, err := findPropertyEnd(source, probePosition, elementEnd)
	if err != nil {
		probeEnd = elementEnd
	}
	probeEnd = trimExpressionEnd(source, probeEnd)

	var fallback string
	for _, position := range selectorTypeProbePositions(source, probePosition, probeEnd) {
		typeText := typeAtPositionString(ctx, client, timeout, snapshot, projectID, file, offsets, position)
		if typeText == "" {
			continue
		}
		if !isAnyLikeType(typeText) {
			return typeText
		}
		if fallback == "" {
			fallback = typeText
		}
	}
	return fallback
}

func decodeBase64String(value string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(value)
}

func selectorReturnTypeFromSignature(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	file string,
	offsets sourceOffsetMap,
	elementStart int,
) string {
	selectorType, err := client.GetTypeAtPosition(
		tsgoapi.WithTimeout(ctx, timeout),
		snapshot,
		projectID,
		file,
		offsets.utf16Offset(elementStart),
	)
	if err != nil || selectorType == nil {
		return ""
	}
	signatures, err := client.GetSignaturesOfType(tsgoapi.WithTimeout(ctx, timeout), snapshot, projectID, selectorType.ID)
	if err != nil || len(signatures) == 0 {
		return ""
	}
	returnType, err := client.GetReturnTypeOfSignature(
		tsgoapi.WithTimeout(ctx, timeout),
		snapshot,
		projectID,
		signatures[0].ID,
	)
	if err != nil || returnType == nil {
		return ""
	}
	return safeTypeString(ctx, client, timeout, snapshot, projectID, returnType.ID)
}

func typeAtPositionString(
	ctx context.Context,
	client *tsgoapi.Client,
	timeout time.Duration,
	snapshot string,
	projectID string,
	file string,
	offsets sourceOffsetMap,
	position int,
) string {
	typ, err := client.GetTypeAtPosition(
		tsgoapi.WithTimeout(ctx, timeout),
		snapshot,
		projectID,
		file,
		offsets.utf16Offset(position),
	)
	if err != nil || typ == nil {
		return ""
	}
	return safeTypeString(ctx, client, timeout, snapshot, projectID, typ.ID)
}

func selectorTypeProbePositions(source string, start, end int) []int {
	positions := make([]int, 0, 4)
	appendPosition := func(position int) {
		if position < start || position >= end {
			return
		}
		for _, existing := range positions {
			if existing == position {
				return
			}
		}
		positions = append(positions, position)
	}

	appendPosition(start)

	firstIdentifier := -1
	for i := start; i < end; i++ {
		switch source[i] {
		case '\'':
			skip, err := skipQuoted(source, i, '\'')
			if err != nil {
				continue
			}
			i = skip
		case '"':
			skip, err := skipQuoted(source, i, '"')
			if err != nil {
				continue
			}
			i = skip
		case '`':
			skip, err := skipTemplate(source, i)
			if err != nil {
				continue
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
				if i+1 < end {
					i++
				}
				continue
			}
		default:
			if !isIdentifierStart(source[i]) {
				continue
			}
			if i > start && isIdentifierPart(source[i-1]) {
				continue
			}
			if matchesIdentifierAt(source, i, "return") || matchesIdentifierAt(source, i, "await") || matchesIdentifierAt(source, i, "new") || matchesIdentifierAt(source, i, "typeof") || matchesIdentifierAt(source, i, "void") {
				for i+1 < end && isIdentifierPart(source[i+1]) {
					i++
				}
				continue
			}
			if firstIdentifier == -1 {
				firstIdentifier = i
			}
			for i+1 < end && isIdentifierPart(source[i+1]) {
				i++
			}
		}
	}

	appendPosition(firstIdentifier)
	if tail := trimExpressionEnd(source, end); tail > start {
		appendPosition(tail - 1)
	}
	return positions
}

func callbackTypeProbePositions(source string, start, end int) []int {
	positions := append([]int(nil), selectorTypeProbePositions(source, start, end)...)
	appendPosition := func(position int) {
		if position < start || position >= end {
			return
		}
		for _, existing := range positions {
			if existing == position {
				return
			}
		}
		positions = append(positions, position)
	}

	if arrowIndex, ok, err := findTopLevelArrow(source, start, end); err == nil && ok {
		appendPosition(skipTrivia(source, arrowIndex+2))
	}

	parenDepth := 0
	for i := start; i < end; i++ {
		switch source[i] {
		case '\'':
			skip, err := skipQuoted(source, i, '\'')
			if err != nil {
				continue
			}
			i = skip
		case '"':
			skip, err := skipQuoted(source, i, '"')
			if err != nil {
				continue
			}
			i = skip
		case '`':
			skip, err := skipTemplate(source, i)
			if err != nil {
				continue
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
				if i+1 < end {
					i++
				}
				continue
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
				if parenDepth == 0 {
					appendPosition(i)
				}
			}
		}
	}

	return positions
}

func CallbackTypeProbePositions(source string, start, end int) []int {
	return callbackTypeProbePositions(source, start, end)
}
