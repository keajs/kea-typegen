package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"kea-typegen/rewrite/internal/keainspect"
	"kea-typegen/rewrite/internal/tsgoapi"
)

const invalidProbeHandle = "__probe_invalid_handle__"

const (
	probeNodeRecordWidth               = 7
	probeNodeRecordBytes               = probeNodeRecordWidth * 4
	probeSourceFileSyntaxKind          = 307
	probeIdentifierSyntaxKind          = 79
	probePrivateIdentifierKind         = 80
	probeInvalidNodeKind        uint32 = ^uint32(0)
	probeFallbackRankContextual        = 1
	probeFallbackRankPosition          = 2
	probeFallbackRankLocation          = 3
)

type probeAPIOptions struct {
	BinaryPath string
	ProjectDir string
	ConfigFile string
	File       string
	Sample     string
	Member     string
	Element    string
	Property   string
	Methods    []string
	ParamsJSON string
	JSON       bool
	Timeout    time.Duration
}

type probePositionCandidate struct {
	propertyName   string
	memberName     string
	elementName    string
	nestedName     string
	locationHandle string
	namePosition   int
	valuePosition  int
	valueEnd       int
	typePositions  []int
}

type probeSession struct {
	Client          *tsgoapi.Client `json:"-"`
	BinaryPath      string          `json:"binaryPath"`
	ProjectDir      string          `json:"projectDir"`
	ConfigFile      string          `json:"configFile"`
	File            string          `json:"file"`
	Snapshot        string          `json:"snapshot"`
	ProjectID       string          `json:"projectID"`
	ProjectConfig   string          `json:"projectConfigFile"`
	SampleProperty  string          `json:"sampleProperty,omitempty"`
	SampleMember    string          `json:"sampleMember,omitempty"`
	SampleElement   string          `json:"sampleElement,omitempty"`
	SampleNested    string          `json:"sampleNestedProperty,omitempty"`
	SamplePosition  int             `json:"samplePosition"`
	SampleSymbolPos int             `json:"sampleSymbolPosition,omitempty"`
	SampleSymbol    string          `json:"sampleSymbol,omitempty"`
	SampleType      string          `json:"sampleType,omitempty"`
	SampleSignature string          `json:"sampleSignature,omitempty"`
	SampleLocations []string        `json:"sampleLocations,omitempty"`
	SampleNameLocs  []string        `json:"sampleNameLocations,omitempty"`
	Warnings        []string        `json:"warnings,omitempty"`
}

type probeMethodResult struct {
	Method string          `json:"method"`
	Status string          `json:"status"`
	Params any             `json:"params,omitempty"`
	Error  string          `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

type probeTypeSummary struct {
	ID   string `json:"id,omitempty"`
	Text string `json:"text,omitempty"`
}

type probeSignatureParameterDetail struct {
	Index        int              `json:"index"`
	Name         string           `json:"name,omitempty"`
	Symbol       string           `json:"symbol"`
	Type         probeTypeSummary `json:"type,omitempty"`
	DeclaredType probeTypeSummary `json:"declaredType,omitempty"`
}

type probeSignatureDetail struct {
	Signature       string                          `json:"signature"`
	Declaration     string                          `json:"declaration,omitempty"`
	DeclarationText string                          `json:"declarationText,omitempty"`
	ThisParam       *probeSignatureParameterDetail  `json:"thisParameter,omitempty"`
	Parameters      []probeSignatureParameterDetail `json:"parameters,omitempty"`
	ReturnType      probeTypeSummary                `json:"returnType,omitempty"`
	TypeParams      []string                        `json:"typeParameters,omitempty"`
	Target          string                          `json:"target,omitempty"`
}

type probeSymbolDetail struct {
	Name             string                 `json:"name,omitempty"`
	Symbol           string                 `json:"symbol"`
	Flags            uint32                 `json:"flags,omitempty"`
	CheckFlags       uint32                 `json:"checkFlags,omitempty"`
	ValueDeclaration string                 `json:"valueDeclaration,omitempty"`
	Declarations     []string               `json:"declarations,omitempty"`
	Type             probeTypeSummary       `json:"type,omitempty"`
	DeclaredType     probeTypeSummary       `json:"declaredType,omitempty"`
	Signatures       []probeSignatureDetail `json:"signatures,omitempty"`
}

type probeReport struct {
	Session probeSession        `json:"session"`
	Results []probeMethodResult `json:"results"`
}

type probeSourceFileNode struct {
	Kind   uint32
	Pos    uint32
	End    uint32
	Parent uint32
}

type probeSourceOffsetMap struct {
	utf16ByByte []int
}

type probeLocationHandle struct {
	Start int
	End   int
	Kind  uint32
	File  string
}

type probeFallbackType struct {
	ID          string
	Flags       uint32
	ObjectFlags uint32
	Rank        int
}

func runProbeAPICommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("probe-api", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	baseDir := findBaseDir()
	options := probeAPIOptions{
		ProjectDir: mustAbsFrom(baseDir, "samples"),
		ConfigFile: mustAbsFrom(baseDir, "samples/tsconfig.json"),
		File:       mustAbsFrom(baseDir, "samples/logic.ts"),
		BinaryPath: tsgoapi.PreferredBinary(baseDir),
		Timeout:    15 * time.Second,
	}
	var methods stringSliceFlag

	flags.StringVar(&options.ProjectDir, "project-dir", options.ProjectDir, "TypeScript project directory")
	flags.StringVar(&options.ConfigFile, "config", options.ConfigFile, "tsconfig path")
	flags.StringVar(&options.File, "file", options.File, "TypeScript file to probe against")
	flags.StringVar(&options.BinaryPath, "tsgo-bin", options.BinaryPath, "Path to the tsgo binary")
	flags.StringVar(&options.Sample, "sample", "", "Logic property to use as the primary probe target (for example props, actions, selectors)")
	flags.StringVar(&options.Member, "member", "", "Nested property inside the selected sample section (for example updateName or capitalizedName)")
	flags.StringVar(&options.Element, "element", "", "Top-level array element inside the selected sample/member (use first, last, or a zero-based index)")
	flags.StringVar(&options.Property, "property", "", "Nested property inside the current target (for example a reducer handler or loader callback)")
	flags.DurationVar(&options.Timeout, "timeout", options.Timeout, "Per-request timeout")
	flags.Var(&methods, "method", "RPC method to call (repeatable)")
	flags.StringVar(&options.ParamsJSON, "params", "", "Raw JSON params to use for every probed method")
	flags.BoolVar(&options.JSON, "json", false, "Print probe results as JSON")

	if err := flags.Parse(args); err != nil {
		return err
	}
	options.Methods = methods
	options.ProjectDir = mustAbsFrom(cliWorkingDir(), options.ProjectDir)
	options.ConfigFile = mustAbsFrom(cliWorkingDir(), options.ConfigFile)
	options.File = mustAbsFrom(cliWorkingDir(), options.File)
	if strings.Contains(options.BinaryPath, string(os.PathSeparator)) {
		options.BinaryPath = mustAbsFrom(cliWorkingDir(), options.BinaryPath)
	}

	session, err := startProbeSession(ctx, options)
	if err != nil {
		return err
	}
	defer session.Client.Close()

	results, err := runProbeCalls(ctx, session, options)
	if err != nil {
		return err
	}

	report := probeReport{
		Session: *session,
		Results: results,
	}
	if options.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	printProbeReport(report)
	return nil
}

func startProbeSession(ctx context.Context, options probeAPIOptions) (*probeSession, error) {
	if err := tsgoapi.Preflight(options.BinaryPath, options.ProjectDir); err != nil {
		return nil, err
	}

	client, err := tsgoapi.Start(options.ProjectDir, options.BinaryPath)
	if err != nil {
		return nil, err
	}

	if _, err := client.Initialize(tsgoapi.WithTimeout(ctx, options.Timeout)); err != nil {
		_ = client.Close()
		return nil, err
	}
	snapshotResponse, err := client.UpdateSnapshot(tsgoapi.WithTimeout(ctx, options.Timeout), options.ConfigFile)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	project := tsgoapi.PickProject(snapshotResponse.Projects, options.ConfigFile)
	if project == nil {
		_ = client.Close()
		return nil, fmt.Errorf("no project returned for %s", options.ConfigFile)
	}
	if defaultProject, err := client.GetDefaultProjectForFile(tsgoapi.WithTimeout(ctx, options.Timeout), snapshotResponse.Snapshot, options.File); err == nil && defaultProject != nil {
		project = defaultProject
	}

	session := &probeSession{
		Client:        client,
		BinaryPath:    options.BinaryPath,
		ProjectDir:    options.ProjectDir,
		ConfigFile:    options.ConfigFile,
		File:          options.File,
		Snapshot:      snapshotResponse.Snapshot,
		ProjectID:     project.ID,
		ProjectConfig: project.ConfigFileName,
	}
	if err := session.bootstrapSampleHandles(ctx, options.Timeout, options.Sample, options.Member, options.Element, options.Property); err != nil {
		_ = client.Close()
		return nil, err
	}
	return session, nil
}

func (s *probeSession) bootstrapSampleHandles(ctx context.Context, timeout time.Duration, sample, member, element, property string) error {
	sourceBytes, err := os.ReadFile(s.File)
	if err != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("read file: %v", err))
		return nil
	}

	source := string(sourceBytes)
	candidates, err := probePositionCandidates(source)
	if err != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("find logics: %v", err))
	}
	candidates, err = selectProbePositionCandidates(candidates, sample)
	if err != nil {
		return err
	}
	candidates, err = probeMemberPositionCandidates(source, candidates, member)
	if err != nil {
		return err
	}
	candidates, err = probeElementPositionCandidates(source, candidates, element)
	if err != nil {
		return err
	}
	candidates, err = probePropertyPositionCandidates(source, candidates, property)
	if err != nil {
		return err
	}
	offsets := newProbeSourceOffsetMap(source)
	candidates = normalizeProbePositionCandidates(offsets, candidates)
	nodes, canonicalFile, hasNodes := s.sourceFileNodes(ctx, timeout, offsets.utf16Length())
	if hasNodes {
		for index := range candidates {
			if handle := probeNodeLocationHandleForRange(nodes, candidates[index].valuePosition, candidates[index].valueEnd, canonicalFile); handle != "" {
				candidates[index].locationHandle = handle
			}
		}
	}
	if len(candidates) == 0 {
		candidates = []probePositionCandidate{{namePosition: 0, valuePosition: 0}}
	}
	s.SampleProperty = candidates[0].propertyName
	s.SampleMember = candidates[0].memberName
	s.SampleElement = candidates[0].elementName
	s.SampleNested = candidates[0].nestedName
	s.SamplePosition = candidates[0].valuePosition
	s.SampleSymbolPos = candidates[0].namePosition

	var positionType probeFallbackType
	symbolType := ""
	signatureType := ""
	for _, candidate := range candidates {
		if candidate.propertyName != "" && s.SampleProperty == "" {
			s.SampleProperty = candidate.propertyName
		}
		if candidate.memberName != "" && s.SampleMember == "" {
			s.SampleMember = candidate.memberName
		}
		if candidate.elementName != "" && s.SampleElement == "" {
			s.SampleElement = candidate.elementName
		}
		if candidate.nestedName != "" && s.SampleNested == "" {
			s.SampleNested = candidate.nestedName
		}
		var currentSymbol string
		if symbol, err := s.Client.GetSymbolAtPosition(tsgoapi.WithTimeout(ctx, timeout), s.Snapshot, s.ProjectID, s.File, candidate.namePosition); err == nil && symbol != nil {
			currentSymbol = symbol.ID
			if s.SampleSymbol == "" {
				s.SampleSymbol = symbol.ID
			}
			s.addSymbolLocationCandidates(symbol)
			s.bootstrapRelatedSymbolLocations(ctx, timeout, symbol.ID)
		} else if err != nil {
			s.Warnings = append(s.Warnings, fmt.Sprintf("get symbol at position %d: %v", candidate.namePosition, err))
		}

		if currentSymbol != "" {
			if nextType, err := s.typesOfFirstSymbol(ctx, timeout, currentSymbol); err == nil && nextType != "" {
				if symbolType == "" {
					symbolType = nextType
				}
				if s.SampleSymbolPos == 0 {
					s.SampleSymbolPos = candidate.namePosition
				}
				s.bootstrapTypeLocations(ctx, timeout, nextType)
				if s.SampleSignature == "" {
					if signature, err := s.firstSignatureOnType(ctx, timeout, nextType); err == nil && signature != "" {
						s.SampleSignature = signature
					} else if err != nil {
						s.Warnings = append(s.Warnings, fmt.Sprintf("find first signature on symbol type %s: %v", nextType, err))
					}
				}
			} else if err != nil {
				s.Warnings = append(s.Warnings, fmt.Sprintf("get types of symbols: %v", err))
			}
		}

		if candidate.locationHandle != "" {
			s.addLocationCandidates(candidate.locationHandle)
			for _, method := range []struct {
				name string
				rank int
			}{
				{name: "getTypeAtLocation", rank: probeFallbackRankLocation},
				{name: "getContextualType", rank: probeFallbackRankContextual},
			} {
				typ, err := s.typeAtLocationMethod(ctx, timeout, method.name, candidate.locationHandle)
				if err != nil {
					s.Warnings = append(s.Warnings, fmt.Sprintf("%s at location %s: %v", method.name, candidate.locationHandle, err))
					continue
				}
				if typ == nil || typ.ID == "" {
					continue
				}
				matchedType, err := s.applyTypeCandidate(ctx, timeout, candidate, typ, candidate.valuePosition, currentSymbol, method.rank, &positionType)
				if err != nil {
					s.Warnings = append(s.Warnings, fmt.Sprintf("use type %s from %s: %v", typ.ID, method.name, err))
					continue
				}
				if matchedType != "" {
					signatureType = matchedType
					break
				}
			}
			if signatureType != "" {
				break
			}
		}

		for _, typePosition := range probeCandidateTypePositions(candidate) {
			typ, err := s.Client.GetTypeAtPosition(tsgoapi.WithTimeout(ctx, timeout), s.Snapshot, s.ProjectID, s.File, typePosition)
			if err != nil {
				s.Warnings = append(s.Warnings, fmt.Sprintf("get type at position %d: %v", typePosition, err))
				continue
			}
			if typ == nil {
				continue
			}
			matchedType, err := s.applyTypeCandidate(ctx, timeout, candidate, typ, typePosition, currentSymbol, probeFallbackRankPosition, &positionType)
			if err != nil {
				s.Warnings = append(s.Warnings, fmt.Sprintf("use type %s at position %d: %v", typ.ID, typePosition, err))
				continue
			}
			if matchedType != "" {
				signatureType = matchedType
				break
			}
		}
		if signatureType != "" {
			break
		}
	}

	switch {
	case signatureType != "":
		s.SampleType = signatureType
	case symbolType != "":
		s.SampleType = symbolType
	case positionType.ID != "":
		s.SampleType = positionType.ID
	}
	if s.SampleType == "" && s.SampleSymbol != "" {
		if typ, err := s.Client.GetTypeOfSymbol(tsgoapi.WithTimeout(ctx, timeout), s.Snapshot, s.ProjectID, s.SampleSymbol); err == nil && typ != nil {
			s.SampleType = typ.ID
		} else if err != nil {
			s.Warnings = append(s.Warnings, fmt.Sprintf("get type of symbol: %v", err))
		}
	}

	if s.SampleSignature == "" && s.SampleType != "" {
		if signatures, err := s.Client.GetSignaturesOfType(tsgoapi.WithTimeout(ctx, timeout), s.Snapshot, s.ProjectID, s.SampleType); err == nil && len(signatures) > 0 && signatures[0] != nil {
			s.SampleSignature = signatures[0].ID
		} else if err != nil {
			s.Warnings = append(s.Warnings, fmt.Sprintf("get signatures of type: %v", err))
		}
	}
	if s.SampleSignature == "" && s.SampleType != "" {
		if signature, err := s.firstSignatureOnType(ctx, timeout, s.SampleType); err == nil && signature != "" {
			s.SampleSignature = signature
		} else if err != nil {
			s.Warnings = append(s.Warnings, fmt.Sprintf("find first signature on sample type: %v", err))
		}
	}
	if len(s.SampleLocations) == 0 && s.SampleType != "" {
		s.bootstrapTypeLocations(ctx, timeout, s.SampleType)
	}
	s.bootstrapNameLocations(ctx, timeout, offsets.utf16Length(), candidates)
	if s.SampleSymbolPos == 0 {
		s.SampleSymbolPos = s.SamplePosition
	}
	return nil
}

func probePositionCandidates(source string) ([]probePositionCandidate, error) {
	candidates := []probePositionCandidate{{namePosition: 0, valuePosition: 0}}
	logics, err := keainspect.FindLogics(source)
	if err != nil {
		return candidates, err
	}
	if len(logics) == 0 {
		return candidates, nil
	}

	candidates = candidates[:0]
	for _, logic := range logics {
		for _, property := range logic.Properties {
			if property.ValueStart <= 0 {
				continue
			}
			candidates = append(candidates, probePositionCandidate{
				propertyName:  property.Name,
				namePosition:  property.NameStart,
				valuePosition: property.ValueStart,
				valueEnd:      probeTrimExpressionEnd(source, property.ValueEnd),
			})
		}
	}
	if len(candidates) == 0 {
		return []probePositionCandidate{{namePosition: 0, valuePosition: 0}}, nil
	}
	return candidates, nil
}

func selectProbePositionCandidates(candidates []probePositionCandidate, sample string) ([]probePositionCandidate, error) {
	sample = strings.TrimSpace(sample)
	if sample == "" {
		return candidates, nil
	}

	selected := make([]probePositionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.propertyName, sample) {
			selected = append(selected, candidate)
		}
	}
	if len(selected) > 0 {
		return selected, nil
	}

	available := probeCandidatePropertyNames(candidates)
	if len(available) == 0 {
		return nil, fmt.Errorf("sample %q not found; no logic properties were discovered", sample)
	}
	return nil, fmt.Errorf("sample %q not found; available properties: %s", sample, strings.Join(available, ", "))
}

func probeCandidatePropertyNames(candidates []probePositionCandidate) []string {
	seen := make(map[string]bool, len(candidates))
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		name := strings.TrimSpace(candidate.propertyName)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func probeMemberPositionCandidates(source string, candidates []probePositionCandidate, member string) ([]probePositionCandidate, error) {
	member = strings.TrimSpace(member)
	if member == "" {
		return candidates, nil
	}

	selected := make([]probePositionCandidate, 0, len(candidates))
	available := make(map[string]bool)
	sampleName := probeCandidateSampleName(candidates)
	for _, candidate := range candidates {
		properties := keainspect.FindSectionProperties(source, keainspect.SourceProperty{
			Name:       candidate.propertyName,
			NameStart:  candidate.namePosition,
			ValueStart: candidate.valuePosition,
			ValueEnd:   candidate.valueEnd,
		})
		for _, property := range properties {
			available[property.Name] = true
			if !strings.EqualFold(property.Name, member) {
				continue
			}
			selected = append(selected, probePositionCandidate{
				propertyName:  candidate.propertyName,
				memberName:    property.Name,
				namePosition:  property.NameStart,
				valuePosition: property.ValueStart,
				valueEnd:      probeTrimExpressionEnd(source, property.ValueEnd),
			})
		}
	}

	if len(selected) > 0 {
		return selected, nil
	}

	if len(available) == 0 {
		return nil, fmt.Errorf("member %q not found under sample %q; no nested properties were discovered", member, sampleName)
	}
	return nil, fmt.Errorf("member %q not found under sample %q; available members: %s", member, sampleName, strings.Join(probeSortedNames(available), ", "))
}

func probeElementPositionCandidates(source string, candidates []probePositionCandidate, element string) ([]probePositionCandidate, error) {
	selector, ok, err := parseProbeElementSelector(element)
	if err != nil {
		return nil, err
	}
	if !ok {
		return candidates, nil
	}

	selected := make([]probePositionCandidate, 0, len(candidates))
	available := make(map[int]bool)
	targetName := probeCandidateTargetName(candidates)
	foundArray := false

	for _, candidate := range candidates {
		elements, err := keainspect.FindTopLevelArrayElements(source, candidate.valuePosition, candidate.valueEnd)
		if err != nil {
			return nil, fmt.Errorf("find top-level array elements under %s: %w", targetName, err)
		}
		if len(elements) == 0 {
			continue
		}
		foundArray = true
		for index := range elements {
			available[index] = true
		}

		index, ok, err := resolveProbeElementIndex(source, candidate, elements, selector)
		if err != nil {
			return nil, fmt.Errorf("resolve element %q under %s: %w", selector.raw, targetName, err)
		}
		if !ok {
			continue
		}
		selected = append(selected, probePositionCandidate{
			propertyName:  candidate.propertyName,
			memberName:    candidate.memberName,
			elementName:   selector.label(index),
			namePosition:  elements[index].Start,
			valuePosition: elements[index].Start,
			valueEnd:      elements[index].End,
			typePositions: keainspect.CallbackTypeProbePositions(source, elements[index].Start, elements[index].End),
		})
	}

	if len(selected) > 0 {
		return selected, nil
	}
	if !foundArray {
		return nil, fmt.Errorf("element %q not found under %s; no top-level array elements were discovered", selector.raw, targetName)
	}
	return nil, fmt.Errorf("element %q not found under %s; available elements: %s", selector.raw, targetName, strings.Join(probeAvailableElementLabels(available), ", "))
}

func resolveProbeElementIndex(source string, candidate probePositionCandidate, elements []keainspect.SourceRange, selector probeElementSelector) (int, bool, error) {
	if selector.kind != "last" {
		index, ok := selector.resolve(len(elements))
		return index, ok, nil
	}
	if len(elements) == 0 {
		return 0, false, nil
	}

	index := len(elements) - 1
	start, end, ok, err := keainspect.FindLastFunctionLikeTopLevelArrayElement(source, candidate.valuePosition, candidate.valueEnd)
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return index, true, nil
	}
	for candidateIndex, element := range elements {
		if element.Start == start && element.End == end {
			return candidateIndex, true, nil
		}
	}
	return index, true, nil
}

func probePropertyPositionCandidates(source string, candidates []probePositionCandidate, property string) ([]probePositionCandidate, error) {
	property = strings.TrimSpace(property)
	if property == "" {
		return candidates, nil
	}

	selected := make([]probePositionCandidate, 0, len(candidates))
	available := make(map[string]bool)
	targetName := probeCandidateTargetName(candidates)
	for _, candidate := range candidates {
		properties := keainspect.FindSectionProperties(source, keainspect.SourceProperty{
			NameStart:  candidate.namePosition,
			ValueStart: candidate.valuePosition,
			ValueEnd:   candidate.valueEnd,
		})
		for _, nested := range properties {
			available[nested.Name] = true
			if !strings.EqualFold(nested.Name, property) {
				continue
			}
			selected = append(selected, probePositionCandidate{
				propertyName:  candidate.propertyName,
				memberName:    candidate.memberName,
				elementName:   candidate.elementName,
				nestedName:    nested.Name,
				namePosition:  nested.NameStart,
				valuePosition: nested.ValueStart,
				valueEnd:      probeTrimExpressionEnd(source, nested.ValueEnd),
				typePositions: keainspect.CallbackTypeProbePositions(source, nested.ValueStart, probeTrimExpressionEnd(source, nested.ValueEnd)),
			})
		}
	}

	if len(selected) > 0 {
		return selected, nil
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("property %q not found under %s; no nested properties were discovered", property, targetName)
	}
	return nil, fmt.Errorf("property %q not found under %s; available properties: %s", property, targetName, strings.Join(probeSortedNames(available), ", "))
}

func probeCandidateSampleName(candidates []probePositionCandidate) string {
	for _, candidate := range candidates {
		name := strings.TrimSpace(candidate.propertyName)
		if name != "" {
			return name
		}
	}
	return ""
}

func probeCandidateTargetName(candidates []probePositionCandidate) string {
	for _, candidate := range candidates {
		parts := make([]string, 0, 4)
		if sampleName := strings.TrimSpace(candidate.propertyName); sampleName != "" {
			parts = append(parts, fmt.Sprintf("sample %q", sampleName))
		}
		if memberName := strings.TrimSpace(candidate.memberName); memberName != "" {
			parts = append(parts, fmt.Sprintf("member %q", memberName))
		}
		if elementName := strings.TrimSpace(candidate.elementName); elementName != "" {
			parts = append(parts, fmt.Sprintf("element %q", elementName))
		}
		if nestedName := strings.TrimSpace(candidate.nestedName); nestedName != "" {
			parts = append(parts, fmt.Sprintf("property %q", nestedName))
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}
	return "the current target"
}

func probeSortedNames(values map[string]bool) []string {
	names := make([]string, 0, len(values))
	for name, ok := range values {
		if !ok {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type probeElementSelector struct {
	raw   string
	kind  string
	index int
}

func parseProbeElementSelector(value string) (probeElementSelector, bool, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return probeElementSelector{}, false, nil
	}
	switch strings.ToLower(text) {
	case "first":
		return probeElementSelector{raw: text, kind: "first"}, true, nil
	case "last":
		return probeElementSelector{raw: text, kind: "last"}, true, nil
	}

	index, err := strconv.Atoi(text)
	if err != nil || index < 0 {
		return probeElementSelector{}, false, fmt.Errorf("element %q is invalid; use first, last, or a zero-based index", text)
	}
	return probeElementSelector{raw: text, kind: "index", index: index}, true, nil
}

func (s probeElementSelector) resolve(count int) (int, bool) {
	if count <= 0 {
		return 0, false
	}
	switch s.kind {
	case "first":
		return 0, true
	case "last":
		return count - 1, true
	case "index":
		if s.index >= 0 && s.index < count {
			return s.index, true
		}
	}
	return 0, false
}

func (s probeElementSelector) label(index int) string {
	switch s.kind {
	case "first", "last":
		return s.kind
	default:
		return strconv.Itoa(index)
	}
}

func probeAvailableElementLabels(values map[int]bool) []string {
	if len(values) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(values))
	for index, ok := range values {
		if !ok {
			continue
		}
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	labels := make([]string, 0, len(indexes)+2)
	if len(indexes) > 0 && indexes[0] == 0 {
		labels = append(labels, "first")
	}
	for _, index := range indexes {
		labels = append(labels, strconv.Itoa(index))
	}
	if len(indexes) > 1 {
		labels = append(labels, "last")
	}
	return labels
}

func probeCandidateTypePositions(candidate probePositionCandidate) []int {
	if len(candidate.typePositions) == 0 {
		return []int{candidate.valuePosition}
	}
	positions := make([]int, 0, len(candidate.typePositions)+1)
	seen := make(map[int]bool, len(candidate.typePositions)+1)
	for _, position := range append([]int{candidate.valuePosition}, candidate.typePositions...) {
		if position < 0 || seen[position] {
			continue
		}
		seen[position] = true
		positions = append(positions, position)
	}
	if len(positions) == 0 {
		return []int{candidate.valuePosition}
	}
	return positions
}

func newProbeSourceOffsetMap(source string) probeSourceOffsetMap {
	utf16ByByte := make([]int, len(source)+1)
	utf16Offset := 0

	for byteOffset := 0; byteOffset < len(source); {
		utf16ByByte[byteOffset] = utf16Offset
		r, size := utf8.DecodeRuneInString(source[byteOffset:])
		for extra := 1; extra < size; extra++ {
			utf16ByByte[byteOffset+extra] = utf16Offset
		}
		byteOffset += size
		utf16Offset += probeUTF16Width(r)
		utf16ByByte[byteOffset] = utf16Offset
	}

	return probeSourceOffsetMap{utf16ByByte: utf16ByByte}
}

func (m probeSourceOffsetMap) utf16Length() int {
	if len(m.utf16ByByte) == 0 {
		return 0
	}
	return m.utf16ByByte[len(m.utf16ByByte)-1]
}

func (m probeSourceOffsetMap) utf16Offset(byteOffset int) int {
	if len(m.utf16ByByte) == 0 || byteOffset <= 0 {
		return 0
	}
	if byteOffset >= len(m.utf16ByByte) {
		return m.utf16Length()
	}
	return m.utf16ByByte[byteOffset]
}

func (m probeSourceOffsetMap) byteOffset(utf16Offset int) (int, bool) {
	if utf16Offset < 0 || utf16Offset > m.utf16Length() {
		return 0, false
	}
	index := sort.Search(len(m.utf16ByByte), func(i int) bool {
		return m.utf16ByByte[i] >= utf16Offset
	})
	if index >= len(m.utf16ByByte) || m.utf16ByByte[index] != utf16Offset {
		return 0, false
	}
	return index, true
}

func normalizeProbePositionCandidates(offsets probeSourceOffsetMap, candidates []probePositionCandidate) []probePositionCandidate {
	normalized := make([]probePositionCandidate, len(candidates))
	for index, candidate := range candidates {
		normalized[index] = candidate
		normalized[index].namePosition = offsets.utf16Offset(candidate.namePosition)
		normalized[index].valuePosition = offsets.utf16Offset(candidate.valuePosition)
		normalized[index].valueEnd = offsets.utf16Offset(candidate.valueEnd)
		if len(candidate.typePositions) == 0 {
			continue
		}
		normalized[index].typePositions = make([]int, 0, len(candidate.typePositions))
		for _, position := range candidate.typePositions {
			normalized[index].typePositions = append(normalized[index].typePositions, offsets.utf16Offset(position))
		}
	}
	return normalized
}

func probeUTF16Width(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

func probeTrimExpressionEnd(source string, end int) int {
	for end > 0 {
		ch := source[end-1]
		if ch == ',' || ch == ';' || ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' {
			end--
			continue
		}
		break
	}
	return end
}

func (s *probeSession) sourceFileNodes(ctx context.Context, timeout time.Duration, sourceLen int) ([]probeSourceFileNode, string, bool) {
	if sourceLen == 0 {
		return nil, "", false
	}

	raw, err := s.Client.CallRaw(tsgoapi.WithTimeout(ctx, timeout), "getSourceFile", map[string]any{
		"snapshot": s.Snapshot,
		"project":  s.ProjectID,
		"file":     s.File,
	})
	if err != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("get source file: %v", err))
		return nil, "", false
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, "", false
	}

	var payload struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("decode source file payload: %v", err))
		return nil, "", false
	}
	if payload.Data == "" {
		return nil, "", false
	}

	blob, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("decode source file payload data: %v", err))
		return nil, "", false
	}
	nodes, err := parseProbeSourceFileNodes(blob, sourceLen)
	if err != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("parse source file AST blob: %v", err))
		return nil, "", false
	}
	return nodes, strings.ToLower(s.File), true
}

func probeNodeLocationHandleForRange(nodes []probeSourceFileNode, start, end int, file string) string {
	bestIndex := -1
	bestWidth := 0
	exactMatch := false
	for index, node := range nodes {
		if node.Kind == 0 || node.Kind == probeInvalidNodeKind || node.End < node.Pos {
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

func (s *probeSession) typeAtLocationMethod(ctx context.Context, timeout time.Duration, method, location string) (*tsgoapi.TypeResponse, error) {
	raw, err := s.Client.CallRaw(tsgoapi.WithTimeout(ctx, timeout), method, map[string]any{
		"snapshot": s.Snapshot,
		"project":  s.ProjectID,
		"location": location,
	})
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var typ tsgoapi.TypeResponse
	if err := json.Unmarshal(raw, &typ); err != nil {
		return nil, err
	}
	return &typ, nil
}

func (s *probeSession) applyTypeCandidate(
	ctx context.Context,
	timeout time.Duration,
	candidate probePositionCandidate,
	typ *tsgoapi.TypeResponse,
	samplePosition int,
	currentSymbol string,
	fallbackRank int,
	positionType *probeFallbackType,
) (string, error) {
	if typ == nil || typ.ID == "" {
		return "", nil
	}
	typeID := typ.ID
	if typeID == "" {
		return "", nil
	}
	fallback := probeFallbackType{
		ID:          typeID,
		Flags:       typ.Flags,
		ObjectFlags: typ.ObjectFlags,
		Rank:        fallbackRank,
	}
	if probeShouldUseFallbackType(*positionType, fallback) {
		*positionType = fallback
	}
	s.bootstrapTypeLocations(ctx, timeout, typeID)
	if s.SampleSignature == "" {
		if signature, err := s.firstSignatureOnType(ctx, timeout, typeID); err == nil && signature != "" {
			s.SampleSignature = signature
		} else if err != nil {
			return "", fmt.Errorf("find first signature on type %s: %w", typeID, err)
		}
	}
	signatures, err := s.Client.GetSignaturesOfType(tsgoapi.WithTimeout(ctx, timeout), s.Snapshot, s.ProjectID, typeID)
	if err != nil {
		return "", fmt.Errorf("get signatures of type %s: %w", typeID, err)
	}
	if len(signatures) == 0 || signatures[0] == nil {
		return "", nil
	}

	s.SampleSignature = signatures[0].ID
	if candidate.propertyName != "" {
		s.SampleProperty = candidate.propertyName
	}
	if candidate.memberName != "" {
		s.SampleMember = candidate.memberName
	}
	if candidate.elementName != "" {
		s.SampleElement = candidate.elementName
	}
	if candidate.nestedName != "" {
		s.SampleNested = candidate.nestedName
	}
	s.SamplePosition = samplePosition
	if currentSymbol != "" {
		s.SampleSymbolPos = candidate.namePosition
	}
	return typeID, nil
}

func probeShouldUseFallbackType(current, next probeFallbackType) bool {
	if next.ID == "" {
		return false
	}
	if current.ID == "" {
		return true
	}

	currentWeak := probeFallbackTypeIsWeak(current)
	nextWeak := probeFallbackTypeIsWeak(next)
	switch {
	case currentWeak && !nextWeak:
		return true
	case !currentWeak && nextWeak:
		return false
	case next.Rank > current.Rank:
		return true
	default:
		return false
	}
}

func probeFallbackTypeIsWeak(typ probeFallbackType) bool {
	switch typ.Flags {
	case 1, 2:
		return typ.ObjectFlags == 0
	default:
		return false
	}
}

func (s *probeSession) typesOfFirstSymbol(ctx context.Context, timeout time.Duration, symbolID string) (string, error) {
	raw, err := s.Client.CallRaw(tsgoapi.WithTimeout(ctx, timeout), "getTypesOfSymbols", map[string]any{
		"snapshot": s.Snapshot,
		"project":  s.ProjectID,
		"symbols":  []string{symbolID},
	})
	if err != nil {
		return "", err
	}
	var types []*tsgoapi.TypeResponse
	if len(raw) == 0 {
		return "", nil
	}
	if err := json.Unmarshal(raw, &types); err != nil {
		return "", err
	}
	if len(types) == 0 || types[0] == nil {
		return "", nil
	}
	return types[0].ID, nil
}

func (s *probeSession) firstSignatureOnType(ctx context.Context, timeout time.Duration, typeID string) (string, error) {
	properties, err := s.Client.GetPropertiesOfType(tsgoapi.WithTimeout(ctx, timeout), s.Snapshot, s.ProjectID, typeID)
	if err != nil {
		return "", err
	}
	for _, property := range properties {
		if property == nil {
			continue
		}
		propertyType, err := s.Client.GetTypeOfSymbol(tsgoapi.WithTimeout(ctx, timeout), s.Snapshot, s.ProjectID, property.ID)
		if err != nil || propertyType == nil {
			continue
		}
		signatures, err := s.Client.GetSignaturesOfType(tsgoapi.WithTimeout(ctx, timeout), s.Snapshot, s.ProjectID, propertyType.ID)
		if err != nil || len(signatures) == 0 || signatures[0] == nil {
			continue
		}
		return signatures[0].ID, nil
	}
	return "", nil
}

func (s *probeSession) bootstrapRelatedSymbolLocations(ctx context.Context, timeout time.Duration, symbolID string) {
	if symbolID == "" {
		return
	}
	if symbol, err := s.callSymbolMethod(ctx, timeout, "getExportSymbolOfSymbol", map[string]any{
		"snapshot": s.Snapshot,
		"project":  s.ProjectID,
		"symbol":   symbolID,
	}); err == nil && symbol != nil {
		s.addSymbolLocationCandidates(symbol)
	} else if err != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("get export symbol of symbol %s: %v", symbolID, err))
	}
	if symbol, err := s.callSymbolMethod(ctx, timeout, "getParentOfSymbol", map[string]any{
		"snapshot": s.Snapshot,
		"project":  s.ProjectID,
		"symbol":   symbolID,
	}); err == nil && symbol != nil {
		s.addSymbolLocationCandidates(symbol)
	} else if err != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("get parent of symbol %s: %v", symbolID, err))
	}
}

func (s *probeSession) bootstrapTypeLocations(ctx context.Context, timeout time.Duration, typeID string) {
	if typeID == "" {
		return
	}
	if symbol, err := s.callSymbolMethod(ctx, timeout, "getSymbolOfType", map[string]any{
		"snapshot": s.Snapshot,
		"project":  s.ProjectID,
		"type":     typeID,
	}); err == nil && symbol != nil {
		s.addSymbolLocationCandidates(symbol)
	} else if err != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("get symbol of type %s: %v", typeID, err))
	}
}

func (s *probeSession) callSymbolMethod(ctx context.Context, timeout time.Duration, method string, params map[string]any) (*tsgoapi.SymbolResponse, error) {
	raw, err := s.Client.CallRaw(tsgoapi.WithTimeout(ctx, timeout), method, params)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var symbol *tsgoapi.SymbolResponse
	if err := json.Unmarshal(raw, &symbol); err != nil {
		return nil, err
	}
	return symbol, nil
}

func (s *probeSession) addSymbolLocationCandidates(symbol *tsgoapi.SymbolResponse) {
	if symbol == nil {
		return
	}
	if symbol.ValueDeclaration != "" {
		s.addLocationCandidates(symbol.ValueDeclaration)
	}
	s.addLocationCandidates(symbol.Declarations...)
}

func (s *probeSession) addLocationCandidates(handles ...string) {
	addUniqueProbeLocationHandles(&s.SampleLocations, handles...)
}

func (s *probeSession) addNameLocationCandidates(handles ...string) {
	addUniqueProbeLocationHandles(&s.SampleNameLocs, handles...)
}

func addUniqueProbeLocationHandles(target *[]string, handles ...string) {
	for _, handle := range handles {
		handle = strings.TrimSpace(handle)
		if handle == "" || handle == invalidProbeHandle {
			continue
		}
		duplicate := false
		for _, existing := range *target {
			if existing == handle {
				duplicate = true
				break
			}
		}
		if !duplicate {
			*target = append(*target, handle)
		}
	}
}

func (s *probeSession) bootstrapNameLocations(ctx context.Context, timeout time.Duration, sourceLen int, candidates []probePositionCandidate) {
	nodes, canonicalFile, ok := s.sourceFileNodes(ctx, timeout, sourceLen)
	if !ok {
		return
	}

	for _, candidate := range candidates {
		if candidate.namePosition <= 0 {
			continue
		}
		if handle := probeNameLocationHandleAtPosition(nodes, candidate.namePosition, canonicalFile); handle != "" {
			s.addNameLocationCandidates(handle)
		}
	}
}

func parseProbeSourceFileNodes(blob []byte, sourceLen int) ([]probeSourceFileNode, error) {
	offset, ok := findProbeSourceFileNodeTableOffset(blob, sourceLen)
	if !ok {
		return nil, fmt.Errorf("could not find source file node table")
	}

	recordCount := (len(blob) - offset) / probeNodeRecordBytes
	nodes := make([]probeSourceFileNode, 0, recordCount)
	for record := 0; record < recordCount; record++ {
		base := offset + record*probeNodeRecordBytes
		nodes = append(nodes, probeSourceFileNode{
			Kind:   binary.LittleEndian.Uint32(blob[base:]),
			Pos:    binary.LittleEndian.Uint32(blob[base+4:]),
			End:    binary.LittleEndian.Uint32(blob[base+8:]),
			Parent: binary.LittleEndian.Uint32(blob[base+16:]),
		})
	}
	return nodes, nil
}

func findProbeSourceFileNodeTableOffset(blob []byte, sourceLen int) (int, bool) {
	sourceLen32 := uint32(sourceLen)
	for offset := 0; offset+probeNodeRecordBytes*2 <= len(blob); offset++ {
		if !probeZeroNodeRecord(blob[offset : offset+probeNodeRecordBytes]) {
			continue
		}
		next := offset + probeNodeRecordBytes
		if binary.LittleEndian.Uint32(blob[next:]) != probeSourceFileSyntaxKind {
			continue
		}
		if binary.LittleEndian.Uint32(blob[next+4:]) != 0 || binary.LittleEndian.Uint32(blob[next+8:]) != sourceLen32 {
			continue
		}
		return offset, true
	}
	return 0, false
}

func probeZeroNodeRecord(record []byte) bool {
	if len(record) < probeNodeRecordBytes {
		return false
	}
	for offset := 0; offset < probeNodeRecordBytes; offset += 4 {
		if binary.LittleEndian.Uint32(record[offset:]) != 0 {
			return false
		}
	}
	return true
}

func probeNameLocationHandleAtPosition(nodes []probeSourceFileNode, position int, file string) string {
	bestIndex := -1
	bestWidth := 0
	for index, node := range nodes {
		if !probeNodeContainsPosition(node, position) || !probeNodeLooksLikeName(node.Kind) {
			continue
		}
		width := int(node.End - node.Pos)
		if bestIndex == -1 || width < bestWidth || (width == bestWidth && node.Pos > nodes[bestIndex].Pos) {
			bestIndex = index
			bestWidth = width
		}
	}
	if bestIndex == -1 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%s", nodes[bestIndex].Pos, nodes[bestIndex].End, nodes[bestIndex].Kind, file)
}

func probeNodeContainsPosition(node probeSourceFileNode, position int) bool {
	if node.Kind == 0 || node.Kind == probeInvalidNodeKind || node.End < node.Pos {
		return false
	}
	pos := uint32(position)
	return pos >= node.Pos && pos < node.End
}

func probeNodeLooksLikeName(kind uint32) bool {
	switch kind {
	case probeIdentifierSyntaxKind, probePrivateIdentifierKind:
		return true
	default:
		return false
	}
}

func (s *probeSession) canonicalLocationFilePath() string {
	for _, handle := range append(append([]string{}, s.SampleNameLocs...), s.SampleLocations...) {
		if file := probeLocationFilePath(handle); file != "" {
			return file
		}
	}
	return strings.ToLower(s.File)
}

func probeLocationFilePath(handle string) string {
	parts := strings.SplitN(handle, ".", 4)
	if len(parts) != 4 {
		return ""
	}
	return parts[3]
}

func runProbeCalls(ctx context.Context, session *probeSession, options probeAPIOptions) ([]probeMethodResult, error) {
	methods := options.Methods
	if len(methods) == 0 {
		methods = defaultProbeMethods()
	}

	var explicitParams any
	if strings.TrimSpace(options.ParamsJSON) != "" {
		if err := json.Unmarshal([]byte(options.ParamsJSON), &explicitParams); err != nil {
			return nil, fmt.Errorf("decode --params: %w", err)
		}
	}

	results := make([]probeMethodResult, 0, len(methods))
	for _, method := range methods {
		paramsList := []any{explicitParams}
		if explicitParams == nil {
			paramsList = probeParamCandidates(method, session)
		}

		var result probeMethodResult
		var lastSuccess *probeMethodResult
		for index, params := range paramsList {
			if pseudoResult, ok, err := runProbePseudoMethod(ctx, session, options.Timeout, method, params); ok {
				if err != nil {
					return nil, err
				}
				candidate := *pseudoResult
				lastSuccess = &candidate
				if probeResultHasValue(candidate.Result) || index == len(paramsList)-1 {
					result = candidate
					break
				}
				continue
			}
			raw, err := session.Client.CallRaw(tsgoapi.WithTimeout(ctx, options.Timeout), method, params)
			candidate := probeMethodResult{
				Method: method,
				Status: "success",
				Params: params,
				Result: raw,
			}
			if err != nil {
				candidate.Status = classifyProbeError(err)
				candidate.Error = err.Error()
				candidate.Result = nil
				if candidate.Status == "known-method-bad-params" && index < len(paramsList)-1 {
					continue
				}
				if lastSuccess != nil {
					result = *lastSuccess
				} else {
					result = candidate
				}
				break
			}
			lastSuccess = &candidate
			if probeResultHasValue(raw) || index == len(paramsList)-1 {
				result = candidate
				break
			}
		}
		if result.Method == "" && lastSuccess != nil {
			result = *lastSuccess
		}
		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Method < results[j].Method
	})
	return results, nil
}

func runProbePseudoMethod(
	ctx context.Context,
	session *probeSession,
	timeout time.Duration,
	method string,
	params any,
) (*probeMethodResult, bool, error) {
	switch method {
	case "printTypeNode":
		typeID := probeTypeIDFromParams(params)
		if typeID == "" {
			typeID = probeTypeHandle(session)
		}
		printed, err := session.Client.PrintTypeNode(tsgoapi.WithTimeout(ctx, timeout), session.Snapshot, session.ProjectID, typeID)
		if err != nil {
			return &probeMethodResult{
				Method: method,
				Status: classifyProbeError(err),
				Params: params,
				Error:  err.Error(),
			}, true, nil
		}
		raw, err := json.Marshal(printed)
		if err != nil {
			return nil, true, err
		}
		return &probeMethodResult{
			Method: method,
			Status: "success",
			Params: params,
			Result: raw,
		}, true, nil
	case "printTypeAtLocationNode", "printContextualTypeNode":
		location := probeLocationFromParams(params)
		if location == "" {
			location = probeLocationHandleForMethod("getTypeAtLocation", session)
		}
		if location == "" {
			return &probeMethodResult{
				Method: method,
				Status: "known-method-bad-params",
				Params: params,
				Error:  "missing location",
			}, true, nil
		}
		var (
			typ *tsgoapi.TypeResponse
			err error
		)
		switch method {
		case "printContextualTypeNode":
			typ, err = session.Client.GetContextualType(tsgoapi.WithTimeout(ctx, timeout), session.Snapshot, session.ProjectID, location)
		default:
			typ, err = session.Client.GetTypeAtLocation(tsgoapi.WithTimeout(ctx, timeout), session.Snapshot, session.ProjectID, location)
		}
		if err != nil {
			return &probeMethodResult{
				Method: method,
				Status: classifyProbeError(err),
				Params: params,
				Error:  err.Error(),
			}, true, nil
		}
		if typ == nil || typ.ID == "" {
			raw, err := json.Marshal("")
			if err != nil {
				return nil, true, err
			}
			return &probeMethodResult{
				Method: method,
				Status: "success",
				Params: params,
				Result: raw,
			}, true, nil
		}
		printed, err := session.Client.PrintTypeNodeAtLocation(
			tsgoapi.WithTimeout(ctx, timeout),
			session.Snapshot,
			session.ProjectID,
			typ.ID,
			location,
		)
		if err != nil {
			return &probeMethodResult{
				Method: method,
				Status: classifyProbeError(err),
				Params: params,
				Error:  err.Error(),
			}, true, nil
		}
		raw, err := json.Marshal(printed)
		if err != nil {
			return nil, true, err
		}
		return &probeMethodResult{
			Method: method,
			Status: "success",
			Params: params,
			Result: raw,
		}, true, nil
	case "printReturnTypeNode", "returnTypeToString":
		signatureID := probeSignatureIDFromParams(params)
		if signatureID == "" {
			signatureID = probeSignatureHandle(session)
		}
		returnType, err := session.Client.GetReturnTypeOfSignature(
			tsgoapi.WithTimeout(ctx, timeout),
			session.Snapshot,
			session.ProjectID,
			signatureID,
		)
		if err != nil {
			return &probeMethodResult{
				Method: method,
				Status: classifyProbeError(err),
				Params: params,
				Error:  err.Error(),
			}, true, nil
		}
		if returnType == nil || returnType.ID == "" {
			raw, err := json.Marshal("")
			if err != nil {
				return nil, true, err
			}
			return &probeMethodResult{
				Method: method,
				Status: "success",
				Params: params,
				Result: raw,
			}, true, nil
		}
		var text string
		switch method {
		case "printReturnTypeNode":
			text, err = session.Client.PrintTypeNode(tsgoapi.WithTimeout(ctx, timeout), session.Snapshot, session.ProjectID, returnType.ID)
		default:
			text, err = session.Client.TypeToString(tsgoapi.WithTimeout(ctx, timeout), session.Snapshot, session.ProjectID, returnType.ID)
		}
		if err != nil {
			return &probeMethodResult{
				Method: method,
				Status: classifyProbeError(err),
				Params: params,
				Error:  err.Error(),
			}, true, nil
		}
		raw, err := json.Marshal(text)
		if err != nil {
			return nil, true, err
		}
		return &probeMethodResult{
			Method: method,
			Status: "success",
			Params: params,
			Result: raw,
		}, true, nil
	case "signatureDetails":
		signatureID := probeSignatureIDFromParams(params)
		typeID := probeTypeIDFromParams(params)
		if typeID == "" {
			typeID = probeTypeHandle(session)
		}
		detail, err := session.signatureDetail(tsgoapi.WithTimeout(ctx, timeout), typeID, signatureID)
		if err != nil {
			return &probeMethodResult{
				Method: method,
				Status: classifyProbeError(err),
				Params: params,
				Error:  err.Error(),
			}, true, nil
		}
		raw, err := json.Marshal(detail)
		if err != nil {
			return nil, true, err
		}
		return &probeMethodResult{
			Method: method,
			Status: "success",
			Params: params,
			Result: raw,
		}, true, nil
	case "propertyDetails":
		typeID := probeTypeIDFromParams(params)
		if typeID == "" {
			typeID = probeTypeHandle(session)
		}
		details, err := session.propertyDetails(tsgoapi.WithTimeout(ctx, timeout), typeID)
		if err != nil {
			return &probeMethodResult{
				Method: method,
				Status: classifyProbeError(err),
				Params: params,
				Error:  err.Error(),
			}, true, nil
		}
		raw, err := json.Marshal(details)
		if err != nil {
			return nil, true, err
		}
		return &probeMethodResult{
			Method: method,
			Status: "success",
			Params: params,
			Result: raw,
		}, true, nil
	case "memberDetails":
		symbolID := probeSymbolIDFromParams(params)
		if symbolID == "" {
			symbolID = probeSymbolHandle(session)
		}
		typeID := probeTypeIDFromParams(params)
		if typeID == "" {
			typeID = probeTypeHandle(session)
		}
		details, err := session.memberDetails(tsgoapi.WithTimeout(ctx, timeout), symbolID, typeID)
		if err != nil {
			return &probeMethodResult{
				Method: method,
				Status: classifyProbeError(err),
				Params: params,
				Error:  err.Error(),
			}, true, nil
		}
		raw, err := json.Marshal(details)
		if err != nil {
			return nil, true, err
		}
		return &probeMethodResult{
			Method: method,
			Status: "success",
			Params: params,
			Result: raw,
		}, true, nil
	default:
		return nil, false, nil
	}
}

func (s *probeSession) signatureDetail(ctx context.Context, typeID, signatureID string) (*probeSignatureDetail, error) {
	if typeID == "" {
		return nil, fmt.Errorf("signatureDetails: empty type handle")
	}

	signatures, err := s.Client.GetSignaturesOfType(ctx, s.Snapshot, s.ProjectID, typeID)
	if err != nil {
		return nil, err
	}
	signature, ok := probeSelectSignature(signatures, signatureID)
	if !ok {
		if signatureID != "" {
			return nil, fmt.Errorf("signatureDetails: signature %q not found on type %q", signatureID, typeID)
		}
		return nil, fmt.Errorf("signatureDetails: no signatures found on type %q", typeID)
	}

	detail, err := s.signatureDetailFromResponse(ctx, signature)
	if err != nil {
		return nil, err
	}
	return &detail, nil
}

func (s *probeSession) signatureParameterDetail(ctx context.Context, index int, name, symbolID string) (probeSignatureParameterDetail, error) {
	detail := probeSignatureParameterDetail{
		Index:  index,
		Name:   name,
		Symbol: symbolID,
	}
	if symbolID == "" {
		return detail, nil
	}

	typ, err := s.Client.GetTypeOfSymbol(ctx, s.Snapshot, s.ProjectID, symbolID)
	if err != nil {
		return detail, err
	}
	if typ == nil {
		return detail, nil
	}

	detail.Type, err = s.typeSummary(ctx, typ.ID)
	if err != nil {
		return detail, err
	}

	declaredType, err := s.Client.GetDeclaredTypeOfSymbol(ctx, s.Snapshot, s.ProjectID, symbolID)
	if err != nil {
		return detail, err
	}
	if declaredType != nil {
		detail.DeclaredType, err = s.typeSummary(ctx, declaredType.ID)
		if err != nil {
			return detail, err
		}
	}
	return detail, nil
}

func (s *probeSession) typeSummary(ctx context.Context, typeID string) (probeTypeSummary, error) {
	summary := probeTypeSummary{ID: typeID}
	if typeID == "" {
		return summary, nil
	}
	text, err := s.Client.TypeToString(ctx, s.Snapshot, s.ProjectID, typeID)
	if err != nil {
		return summary, err
	}
	summary.Text = text
	return summary, nil
}

func (s *probeSession) propertyDetails(ctx context.Context, typeID string) ([]probeSymbolDetail, error) {
	if typeID == "" {
		return nil, fmt.Errorf("propertyDetails: empty type handle")
	}
	if details, err := s.typeProperties(ctx, typeID); err != nil {
		return nil, err
	} else if len(details) > 0 {
		return details, nil
	}

	signatures, err := s.Client.GetSignaturesOfType(ctx, s.Snapshot, s.ProjectID, typeID)
	if err != nil {
		return nil, err
	}
	signature, ok := probeSelectSignature(signatures, "")
	if !ok {
		return nil, nil
	}

	returnType, err := s.Client.GetReturnTypeOfSignature(ctx, s.Snapshot, s.ProjectID, signature.ID)
	if err != nil {
		return nil, err
	}
	fallbackTypeID := probePropertyDetailsFallbackType(typeID, signature, returnType)
	if fallbackTypeID == "" {
		return nil, nil
	}
	return s.typeProperties(ctx, fallbackTypeID)
}

func (s *probeSession) memberDetails(ctx context.Context, symbolID, typeID string) ([]probeSymbolDetail, error) {
	if symbolID == "" && typeID == "" {
		return nil, fmt.Errorf("memberDetails: empty symbol and type handles")
	}
	if details, err := s.symbolMembers(ctx, symbolID, typeID); err != nil {
		return nil, err
	} else if len(details) > 0 {
		return details, nil
	}
	if typeID == "" {
		return nil, nil
	}
	symbol, err := s.Client.GetSymbolOfType(ctx, s.Snapshot, s.ProjectID, typeID)
	if err != nil {
		return nil, err
	}
	if symbol == nil || strings.TrimSpace(symbol.ID) == "" || symbol.ID == symbolID {
		return s.propertyDetails(ctx, typeID)
	}
	if details, err := s.symbolMembers(ctx, symbol.ID, typeID); err != nil {
		return nil, err
	} else if len(details) > 0 {
		return details, nil
	}
	return s.propertyDetails(ctx, typeID)
}

func (s *probeSession) symbolDetails(ctx context.Context, symbols []*tsgoapi.SymbolResponse) ([]probeSymbolDetail, error) {
	if len(symbols) == 0 {
		return nil, nil
	}
	details := make([]probeSymbolDetail, 0, len(symbols))
	for _, symbol := range symbols {
		if symbol == nil || strings.TrimSpace(symbol.ID) == "" {
			continue
		}
		detail, err := s.symbolDetail(ctx, symbol)
		if err != nil {
			return nil, err
		}
		details = append(details, detail)
	}
	return details, nil
}

func (s *probeSession) symbolMembers(ctx context.Context, symbolID, typeID string) ([]probeSymbolDetail, error) {
	if strings.TrimSpace(symbolID) == "" {
		return nil, nil
	}
	members, err := s.Client.GetMembersOfSymbol(ctx, s.Snapshot, s.ProjectID, symbolID)
	if err != nil {
		return nil, err
	}
	members, err = s.filterSymbolsForType(ctx, typeID, members)
	if err != nil {
		return nil, err
	}
	return s.symbolDetails(ctx, members)
}

func (s *probeSession) typeProperties(ctx context.Context, typeID string) ([]probeSymbolDetail, error) {
	properties, err := s.Client.GetPropertiesOfType(ctx, s.Snapshot, s.ProjectID, typeID)
	if err != nil {
		return nil, err
	}
	properties, err = s.filterSymbolsForType(ctx, typeID, properties)
	if err != nil {
		return nil, err
	}
	return s.symbolDetails(ctx, properties)
}

func (s *probeSession) filterSymbolsForType(ctx context.Context, typeID string, symbols []*tsgoapi.SymbolResponse) ([]*tsgoapi.SymbolResponse, error) {
	if strings.TrimSpace(typeID) == "" || len(symbols) == 0 {
		return symbols, nil
	}
	summary, err := s.typeSummary(ctx, typeID)
	if err != nil {
		return nil, err
	}
	if !probeIsTupleLikeTypeText(summary.Text) {
		return symbols, nil
	}
	filtered, ok := probeTrimTupleSymbolSurface(symbols)
	if !ok {
		return symbols, nil
	}
	return filtered, nil
}

func (s *probeSession) symbolDetail(ctx context.Context, symbol *tsgoapi.SymbolResponse) (probeSymbolDetail, error) {
	detail := probeSymbolDetail{
		Name:             symbol.Name,
		Symbol:           symbol.ID,
		Flags:            symbol.Flags,
		CheckFlags:       symbol.CheckFlags,
		ValueDeclaration: symbol.ValueDeclaration,
		Declarations:     append([]string(nil), symbol.Declarations...),
	}

	typ, err := s.Client.GetTypeOfSymbol(ctx, s.Snapshot, s.ProjectID, symbol.ID)
	if err != nil {
		return detail, err
	}
	if typ != nil && typ.ID != "" {
		detail.Type, err = s.typeSummary(ctx, typ.ID)
		if err != nil {
			return detail, err
		}
		signatures, err := s.Client.GetSignaturesOfType(ctx, s.Snapshot, s.ProjectID, typ.ID)
		if err != nil {
			return detail, err
		}
		if len(signatures) > 0 {
			detail.Signatures = make([]probeSignatureDetail, 0, len(signatures))
			for _, signature := range signatures {
				if signature == nil || strings.TrimSpace(signature.ID) == "" {
					continue
				}
				signatureDetail, err := s.signatureDetailFromResponse(ctx, signature)
				if err != nil {
					return detail, err
				}
				detail.Signatures = append(detail.Signatures, signatureDetail)
			}
		}
	}

	declaredType, err := s.Client.GetDeclaredTypeOfSymbol(ctx, s.Snapshot, s.ProjectID, symbol.ID)
	if err != nil {
		return detail, err
	}
	if declaredType != nil && declaredType.ID != "" {
		detail.DeclaredType, err = s.typeSummary(ctx, declaredType.ID)
		if err != nil {
			return detail, err
		}
	}

	return detail, nil
}

func probePropertyDetailsFallbackType(currentTypeID string, signature *tsgoapi.SignatureResponse, returnType *tsgoapi.TypeResponse) string {
	if signature == nil || strings.TrimSpace(signature.ID) == "" || returnType == nil {
		return ""
	}
	returnTypeID := strings.TrimSpace(returnType.ID)
	if returnTypeID == "" || returnTypeID == strings.TrimSpace(currentTypeID) {
		return ""
	}
	return returnTypeID
}

func parseProbeLocationHandle(handle string) (probeLocationHandle, bool) {
	parts := strings.SplitN(strings.TrimSpace(handle), ".", 4)
	if len(parts) != 4 {
		return probeLocationHandle{}, false
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return probeLocationHandle{}, false
	}
	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return probeLocationHandle{}, false
	}
	kind, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		return probeLocationHandle{}, false
	}
	return probeLocationHandle{
		Start: start,
		End:   end,
		Kind:  uint32(kind),
		File:  parts[3],
	}, true
}

func probeTextForLocationHandle(handle probeLocationHandle) (string, bool) {
	if strings.TrimSpace(handle.File) == "" || handle.End < handle.Start || handle.Start < 0 {
		return "", false
	}
	path := handle.File
	content, err := os.ReadFile(path)
	if err != nil {
		resolved, ok := probeResolveLocationHandlePath(path)
		if !ok {
			return "", false
		}
		path = resolved
		content, err = os.ReadFile(path)
	}
	if err != nil {
		return "", false
	}
	source := string(content)
	offsets := newProbeSourceOffsetMap(source)
	start, ok := offsets.byteOffset(handle.Start)
	if !ok {
		return "", false
	}
	end, ok := offsets.byteOffset(handle.End)
	if !ok || end < start {
		return "", false
	}
	return source[start:end], true
}

func probeResolveLocationHandlePath(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	if _, err := os.Stat(path); err == nil {
		return path, true
	}

	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	remainder := strings.TrimPrefix(clean, volume)
	separator := string(filepath.Separator)
	absolute := strings.HasPrefix(remainder, separator)
	remainder = strings.TrimPrefix(remainder, separator)
	if remainder == "" {
		if absolute {
			if volume != "" {
				return volume + separator, true
			}
			return separator, true
		}
		return clean, true
	}

	parts := strings.Split(remainder, separator)
	current := "."
	if absolute {
		current = separator
		if volume != "" {
			current = volume + separator
		}
	} else if volume != "" {
		current = volume
	}

	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		entries, err := os.ReadDir(current)
		if err != nil {
			return "", false
		}
		match := ""
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), part) {
				match = entry.Name()
				break
			}
		}
		if match == "" {
			return "", false
		}
		current = filepath.Join(current, match)
	}
	return current, true
}

func probeSignatureParameterNamesFromDeclaration(text string, includeThis bool) ([]string, string, bool) {
	start := strings.Index(text, "(")
	if start == -1 {
		return nil, "", false
	}
	end, ok := probeMatchingDelimiterIndex(text, start, '(', ')')
	if !ok || end <= start {
		return nil, "", false
	}
	paramsText := text[start+1 : end]
	parts := probeSplitTopLevel(paramsText, ',')
	if len(parts) == 0 {
		return nil, "", true
	}
	names := make([]string, 0, len(parts))
	thisName := ""
	for _, part := range parts {
		name := probeSignatureParameterName(part)
		if name == "" {
			continue
		}
		if includeThis && thisName == "" && name == "this" {
			thisName = name
			continue
		}
		names = append(names, name)
	}
	return names, thisName, true
}

func probeSignatureParameterName(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "...") {
		text = strings.TrimSpace(strings.TrimPrefix(text, "..."))
	}
	cut := len(text)
	for _, delimiter := range []rune{':', '=', ','} {
		if index, ok := probeTopLevelDelimiterIndex(text, delimiter); ok && index < cut {
			cut = index
		}
	}
	name := strings.TrimSpace(text[:cut])
	name = strings.TrimSuffix(name, "?")
	return strings.TrimSpace(name)
}

func probeSplitTopLevel(text string, delimiter rune) []string {
	var parts []string
	start := 0
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	depthAngle := 0
	inString := rune(0)
	escaped := false

	flush := func(end int) {
		part := strings.TrimSpace(text[start:end])
		if part != "" {
			parts = append(parts, part)
		}
		start = end + 1
	}

	for index, r := range text {
		if inString != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == inString {
				inString = 0
			}
			continue
		}
		switch r {
		case '\'', '"', '`':
			inString = r
		case '(':
			depthParen++
		case ')':
			if depthParen > 0 {
				depthParen--
			}
		case '[':
			depthBracket++
		case ']':
			if depthBracket > 0 {
				depthBracket--
			}
		case '{':
			depthBrace++
		case '}':
			if depthBrace > 0 {
				depthBrace--
			}
		case '<':
			depthAngle++
		case '>':
			if depthAngle > 0 {
				depthAngle--
			}
		default:
			if r == delimiter && depthParen == 0 && depthBracket == 0 && depthBrace == 0 && depthAngle == 0 {
				flush(index)
			}
		}
	}
	if start < len(text) {
		part := strings.TrimSpace(text[start:])
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func probeTopLevelDelimiterIndex(text string, delimiter rune) (int, bool) {
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	depthAngle := 0
	inString := rune(0)
	escaped := false

	for index, r := range text {
		if inString != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == inString {
				inString = 0
			}
			continue
		}
		switch r {
		case '\'', '"', '`':
			inString = r
		case '(':
			depthParen++
		case ')':
			if depthParen > 0 {
				depthParen--
			}
		case '[':
			depthBracket++
		case ']':
			if depthBracket > 0 {
				depthBracket--
			}
		case '{':
			depthBrace++
		case '}':
			if depthBrace > 0 {
				depthBrace--
			}
		case '<':
			depthAngle++
		case '>':
			if depthAngle > 0 {
				depthAngle--
			}
		default:
			if r == delimiter && depthParen == 0 && depthBracket == 0 && depthBrace == 0 && depthAngle == 0 {
				return index, true
			}
		}
	}
	return 0, false
}

func probeMatchingDelimiterIndex(text string, start int, open, close rune) (int, bool) {
	depth := 0
	inString := rune(0)
	escaped := false
	for index, r := range text {
		if index < start {
			continue
		}
		if inString != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == inString {
				inString = 0
			}
			continue
		}
		switch r {
		case '\'', '"', '`':
			inString = r
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return index, true
			}
		}
	}
	return 0, false
}

func probeIsTupleLikeTypeText(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "[") || strings.HasPrefix(text, "readonly [")
}

func probeTrimTupleSymbolSurface(symbols []*tsgoapi.SymbolResponse) ([]*tsgoapi.SymbolResponse, bool) {
	if len(symbols) == 0 {
		return nil, false
	}
	type tupleEntry struct {
		index  int
		symbol *tsgoapi.SymbolResponse
	}
	indexed := make([]tupleEntry, 0, len(symbols))
	var lengthSymbol *tsgoapi.SymbolResponse
	for _, symbol := range symbols {
		if symbol == nil {
			continue
		}
		name := strings.TrimSpace(symbol.Name)
		if name == "length" {
			lengthSymbol = symbol
			continue
		}
		index, ok := probeTupleIndex(name)
		if !ok {
			continue
		}
		indexed = append(indexed, tupleEntry{index: index, symbol: symbol})
	}
	if len(indexed) == 0 {
		return nil, false
	}
	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].index < indexed[j].index
	})
	filtered := make([]*tsgoapi.SymbolResponse, 0, len(indexed)+1)
	for _, entry := range indexed {
		filtered = append(filtered, entry.symbol)
	}
	if lengthSymbol != nil {
		filtered = append(filtered, lengthSymbol)
	}
	return filtered, true
}

func probeTupleIndex(name string) (int, bool) {
	if name == "" {
		return 0, false
	}
	for _, r := range name {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	index, err := strconv.Atoi(name)
	if err != nil || index < 0 {
		return 0, false
	}
	return index, true
}

func (s *probeSession) signatureDetailFromResponse(ctx context.Context, signature *tsgoapi.SignatureResponse) (probeSignatureDetail, error) {
	detail := probeSignatureDetail{
		Signature:   signature.ID,
		Declaration: signature.Declaration,
		TypeParams:  append([]string(nil), signature.TypeParameters...),
		Target:      signature.Target,
	}

	var parameterNames []string
	var thisName string
	if handle, ok := parseProbeLocationHandle(signature.Declaration); ok {
		if text, ok := probeTextForLocationHandle(handle); ok {
			detail.DeclarationText = strings.TrimSpace(text)
			parameterNames, thisName, _ = probeSignatureParameterNamesFromDeclaration(text, signature.ThisParameter != "")
		}
	}

	if signature.ThisParameter != "" {
		param, err := s.signatureParameterDetail(ctx, -1, thisName, signature.ThisParameter)
		if err != nil {
			return detail, err
		}
		detail.ThisParam = &param
	}

	if len(signature.Parameters) > 0 {
		detail.Parameters = make([]probeSignatureParameterDetail, 0, len(signature.Parameters))
		for index, symbolID := range signature.Parameters {
			name := ""
			if index < len(parameterNames) {
				name = parameterNames[index]
			}
			param, err := s.signatureParameterDetail(ctx, index, name, symbolID)
			if err != nil {
				return detail, err
			}
			detail.Parameters = append(detail.Parameters, param)
		}
	}

	returnType, err := s.Client.GetReturnTypeOfSignature(ctx, s.Snapshot, s.ProjectID, signature.ID)
	if err != nil {
		return detail, err
	}
	if returnType != nil {
		detail.ReturnType, err = s.typeSummary(ctx, returnType.ID)
		if err != nil {
			return detail, err
		}
	}

	return detail, nil
}

func probeSelectSignature(signatures []*tsgoapi.SignatureResponse, signatureID string) (*tsgoapi.SignatureResponse, bool) {
	signatureID = strings.TrimSpace(signatureID)
	var first *tsgoapi.SignatureResponse
	for _, signature := range signatures {
		if signature == nil || strings.TrimSpace(signature.ID) == "" {
			continue
		}
		if first == nil {
			first = signature
		}
		if signatureID != "" && signature.ID == signatureID {
			return signature, true
		}
	}
	if signatureID != "" {
		return nil, false
	}
	return first, first != nil
}

func probeTypeIDFromParams(params any) string {
	typed, ok := params.(map[string]any)
	if !ok {
		return ""
	}
	value, ok := typed["type"]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func probeSymbolIDFromParams(params any) string {
	typed, ok := params.(map[string]any)
	if !ok {
		return ""
	}
	value, ok := typed["symbol"]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func probeSignatureIDFromParams(params any) string {
	typed, ok := params.(map[string]any)
	if !ok {
		return ""
	}
	value, ok := typed["signature"]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func probeLocationFromParams(params any) string {
	typed, ok := params.(map[string]any)
	if !ok {
		return ""
	}
	value, ok := typed["location"]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func probeParamCandidates(method string, session *probeSession) []any {
	locationHandles := probeLocationHandlesForMethod(method, session)
	if !probeMethodUsesLocation(method) || len(locationHandles) <= 1 {
		return []any{paramsForProbeMethod(method, session)}
	}
	candidates := make([]any, 0, len(locationHandles))
	for _, location := range locationHandles {
		candidates = append(candidates, paramsForProbeMethodWithLocation(method, session, location))
	}
	return candidates
}

func probeLocationHandlesForMethod(method string, session *probeSession) []string {
	switch method {
	case "getSymbolAtLocation", "getSymbolsAtLocations":
		if len(session.SampleNameLocs) > 0 {
			return session.SampleNameLocs
		}
	}
	if len(session.SampleLocations) > 0 {
		if sameFile := probeSameFileLocationHandles(session.SampleLocations, session.canonicalLocationFilePath()); len(sameFile) > 0 {
			return sameFile
		}
		return session.SampleLocations
	}
	return session.SampleNameLocs
}

func probeSameFileLocationHandles(handles []string, file string) []string {
	file = strings.TrimSpace(file)
	if file == "" {
		return nil
	}
	filtered := make([]string, 0, len(handles))
	for _, handle := range handles {
		if probeLocationFilePath(handle) == file {
			filtered = append(filtered, handle)
		}
	}
	return filtered
}

func probeMethodUsesLocation(method string) bool {
	switch method {
	case "getContextualType", "getSymbolAtLocation", "getSymbolsAtLocations", "getTypeAtLocation", "getTypeOfSymbolAtLocation", "printTypeAtLocationNode", "printContextualTypeNode":
		return true
	default:
		return false
	}
}

func probeResultHasValue(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return true
	}
	return probeJSONValueHasContent(value)
}

func probeJSONValueHasContent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case []any:
		for _, item := range typed {
			if probeJSONValueHasContent(item) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func defaultProbeMethods() []string {
	return []string{
		"getBaseTypeOfLiteralType",
		"getBaseTypeOfType",
		"getBaseTypes",
		"getCheckTypeOfType",
		"getConstraintOfType",
		"getContextualType",
		"getDeclaredTypeOfSymbol",
		"getExportSymbolOfSymbol",
		"getExportsOfSymbol",
		"getExtendsTypeOfType",
		"getIndexInfosOfType",
		"getIndexTypeOfType",
		"getLocalTypeParametersOfType",
		"getMembersOfSymbol",
		"getObjectTypeOfType",
		"getOuterTypeParametersOfType",
		"getParentOfSymbol",
		"getRestTypeOfSignature",
		"getReturnTypeOfSignature",
		"getSourceFile",
		"getSymbolAtLocation",
		"getSymbolOfType",
		"getSymbolsAtLocations",
		"getSymbolsAtPositions",
		"getTypeArguments",
		"getTypeAtLocation",
		"getTypeAtLocations",
		"getTypeOfSymbolAtLocation",
		"getTypeParametersOfType",
		"getTypePredicateOfSignature",
		"getTypesAtPositions",
		"getTypesOfSymbols",
		"getTypesOfType",
		"getTargetOfType",
		"typeToString",
	}
}

func paramsForProbeMethod(method string, session *probeSession) any {
	return paramsForProbeMethodWithLocation(method, session, probeLocationHandleForMethod(method, session))
}

func paramsForProbeMethodWithLocation(method string, session *probeSession, location string) any {
	base := func() map[string]any {
		return map[string]any{
			"snapshot": session.Snapshot,
			"project":  session.ProjectID,
		}
	}
	withType := func() map[string]any {
		params := base()
		params["type"] = probeTypeHandle(session)
		return params
	}
	withSymbol := func() map[string]any {
		params := base()
		params["symbol"] = probeSymbolHandle(session)
		return params
	}
	switch method {
	case "getBaseTypeOfLiteralType", "getBaseTypeOfType", "getBaseTypes", "getCheckTypeOfType", "getConstraintOfType", "getExtendsTypeOfType", "getIndexInfosOfType", "getIndexTypeOfType", "getLocalTypeParametersOfType", "getObjectTypeOfType", "getOuterTypeParametersOfType", "getPropertiesOfType", "getSymbolOfType", "getTargetOfType", "getTypeArguments", "getTypeParametersOfType", "getTypesOfType", "printTypeNode", "propertyDetails", "signatureDetails", "typeToString":
		return withType()
	case "getDeclaredTypeOfSymbol", "getExportSymbolOfSymbol", "getExportsOfSymbol", "getMembersOfSymbol", "getParentOfSymbol", "getTypeOfSymbol", "memberDetails":
		return withSymbol()
	case "getRestTypeOfSignature", "getReturnTypeOfSignature", "getTypePredicateOfSignature", "printReturnTypeNode", "returnTypeToString":
		params := base()
		params["signature"] = probeSignatureHandle(session)
		return params
	case "getSignaturesOfType":
		params := withType()
		params["kind"] = 0
		return params
	case "getTypeOfSymbolAtLocation":
		params := withSymbol()
		params["location"] = location
		return params
	case "getTypesOfSymbols":
		params := base()
		params["symbols"] = []string{probeSymbolHandle(session)}
		return params
	case "getSymbolsAtPositions":
		params := base()
		params["file"] = session.File
		params["positions"] = []int{probeSymbolPosition(session)}
		return params
	case "getTypesAtPositions":
		params := base()
		params["file"] = session.File
		params["positions"] = []int{probeTypePosition(session)}
		return params
	case "getContextualType", "getSymbolAtLocation", "getTypeAtLocation", "printTypeAtLocationNode", "printContextualTypeNode":
		params := base()
		params["location"] = location
		return params
	case "getSymbolsAtLocations":
		params := base()
		params["locations"] = []string{location}
		return params
	case "getSourceFile":
		params := base()
		params["file"] = session.File
		return params
	case "getTypeAtLocations":
		params := base()
		params["locations"] = []string{location}
		return params
	default:
		return base()
	}
}

func classifyProbeError(err error) string {
	if err == nil {
		return "success"
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "panic:") ||
		strings.Contains(text, "invalid memory address") ||
		strings.Contains(text, "unhandled case"):
		return "call-error"
	case strings.Contains(text, "unknown api method"):
		return "unknown-method"
	case strings.Contains(text, "invalid node handle") ||
		strings.Contains(text, "empty") ||
		strings.Contains(text, "not found") ||
		strings.Contains(text, "expected") ||
		strings.Contains(text, "could not read file") ||
		strings.Contains(text, "source file not found") ||
		strings.Contains(text, "project has no program"):
		return "known-method-bad-params"
	default:
		return "call-error"
	}
}

func printProbeReport(report probeReport) {
	fmt.Fprintf(os.Stdout, "tsgo binary: %s\n", report.Session.BinaryPath)
	fmt.Fprintf(os.Stdout, "project dir: %s\n", report.Session.ProjectDir)
	fmt.Fprintf(os.Stdout, "config: %s\n", report.Session.ConfigFile)
	fmt.Fprintf(os.Stdout, "file: %s\n", report.Session.File)
	fmt.Fprintf(os.Stdout, "snapshot: %s\n", report.Session.Snapshot)
	fmt.Fprintf(os.Stdout, "project handle: %s\n", report.Session.ProjectID)
	if report.Session.SampleProperty != "" {
		fmt.Fprintf(os.Stdout, "sample property: %s\n", report.Session.SampleProperty)
	}
	if report.Session.SampleMember != "" {
		fmt.Fprintf(os.Stdout, "sample member: %s\n", report.Session.SampleMember)
	}
	if report.Session.SampleElement != "" {
		fmt.Fprintf(os.Stdout, "sample element: %s\n", report.Session.SampleElement)
	}
	if report.Session.SampleNested != "" {
		fmt.Fprintf(os.Stdout, "sample nested property: %s\n", report.Session.SampleNested)
	}
	fmt.Fprintf(os.Stdout, "sample position: %d\n", report.Session.SamplePosition)
	if report.Session.SampleSymbolPos > 0 {
		fmt.Fprintf(os.Stdout, "sample symbol position: %d\n", report.Session.SampleSymbolPos)
	}
	if report.Session.SampleSymbol != "" {
		fmt.Fprintf(os.Stdout, "sample symbol: %s\n", report.Session.SampleSymbol)
	}
	if report.Session.SampleType != "" {
		fmt.Fprintf(os.Stdout, "sample type: %s\n", report.Session.SampleType)
	}
	if report.Session.SampleSignature != "" {
		fmt.Fprintf(os.Stdout, "sample signature: %s\n", report.Session.SampleSignature)
	}
	if len(report.Session.SampleLocations) > 0 {
		fmt.Fprintln(os.Stdout, "sample locations:")
		for _, location := range report.Session.SampleLocations {
			fmt.Fprintf(os.Stdout, "  - %s\n", location)
		}
	}
	if len(report.Session.SampleNameLocs) > 0 {
		fmt.Fprintln(os.Stdout, "sample name locations:")
		for _, location := range report.Session.SampleNameLocs {
			fmt.Fprintf(os.Stdout, "  - %s\n", location)
		}
	}
	if len(report.Session.Warnings) > 0 {
		fmt.Fprintln(os.Stdout, "warnings:")
		for _, warning := range report.Session.Warnings {
			fmt.Fprintf(os.Stdout, "  - %s\n", warning)
		}
	}
	fmt.Fprintln(os.Stdout)
	for _, result := range report.Results {
		fmt.Fprintf(os.Stdout, "%-26s %s\n", result.Method, result.Status)
		if result.Error != "" {
			fmt.Fprintf(os.Stdout, "  error: %s\n", result.Error)
		}
		if len(result.Result) > 0 {
			fmt.Fprintf(os.Stdout, "  result: %s\n", compactProbeJSON(result.Result))
		}
	}
}

func compactProbeJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	text := strings.TrimSpace(string(raw))
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	if len(text) > 160 {
		return text[:157] + "..."
	}
	return text
}

func probeSymbolHandle(session *probeSession) string {
	if session.SampleSymbol != "" {
		return session.SampleSymbol
	}
	return invalidProbeHandle
}

func probeTypeHandle(session *probeSession) string {
	if session.SampleType != "" {
		return session.SampleType
	}
	return invalidProbeHandle
}

func probeSignatureHandle(session *probeSession) string {
	if session.SampleSignature != "" {
		return session.SampleSignature
	}
	return invalidProbeHandle
}

func probeLocationHandleForMethod(method string, session *probeSession) string {
	locationHandles := probeLocationHandlesForMethod(method, session)
	if len(locationHandles) > 0 {
		return locationHandles[0]
	}
	return invalidProbeHandle
}

func probeSymbolPosition(session *probeSession) int {
	if session.SampleSymbolPos > 0 {
		return session.SampleSymbolPos
	}
	if session.SamplePosition > 0 {
		return session.SamplePosition
	}
	return 0
}

func probeTypePosition(session *probeSession) int {
	if session.SamplePosition > 0 {
		return session.SamplePosition
	}
	if session.SampleSymbolPos > 0 {
		return session.SampleSymbolPos
	}
	return 0
}
