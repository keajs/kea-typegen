package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"kea-typegen/rewrite/internal/keainspect"
	"kea-typegen/rewrite/internal/tsgoapi"
)

const invalidProbeHandle = "__probe_invalid_handle__"

type probeAPIOptions struct {
	BinaryPath string
	ProjectDir string
	ConfigFile string
	File       string
	Methods    []string
	ParamsJSON string
	JSON       bool
	Timeout    time.Duration
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
	SamplePosition  int             `json:"samplePosition"`
	SampleSymbolPos int             `json:"sampleSymbolPosition,omitempty"`
	SampleSymbol    string          `json:"sampleSymbol,omitempty"`
	SampleType      string          `json:"sampleType,omitempty"`
	SampleSignature string          `json:"sampleSignature,omitempty"`
	SampleLocations []string        `json:"sampleLocations,omitempty"`
	Warnings        []string        `json:"warnings,omitempty"`
}

type probeMethodResult struct {
	Method string          `json:"method"`
	Status string          `json:"status"`
	Params any             `json:"params,omitempty"`
	Error  string          `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

type probeReport struct {
	Session probeSession        `json:"session"`
	Results []probeMethodResult `json:"results"`
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
	session.bootstrapSampleHandles(ctx, options.Timeout)
	return session, nil
}

func (s *probeSession) bootstrapSampleHandles(ctx context.Context, timeout time.Duration) {
	sourceBytes, err := os.ReadFile(s.File)
	if err != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("read file: %v", err))
		return
	}

	source := string(sourceBytes)
	type candidate struct {
		namePosition  int
		valuePosition int
	}
	candidates := []candidate{{namePosition: 0, valuePosition: 0}}
	if logics, err := keainspect.FindLogics(source); err == nil && len(logics) > 0 {
		candidates = candidates[:0]
		for _, logic := range logics {
			for _, property := range logic.Properties {
				if property.ValueStart > 0 {
					candidates = append(candidates, candidate{
						namePosition:  property.NameStart,
						valuePosition: property.ValueStart,
					})
				}
			}
		}
	} else if err != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("find logics: %v", err))
	}
	s.SamplePosition = candidates[0].valuePosition
	s.SampleSymbolPos = candidates[0].namePosition

	positionType := ""
	symbolType := ""
	signatureType := ""
	for _, candidate := range candidates {
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

		typ, err := s.Client.GetTypeAtPosition(tsgoapi.WithTimeout(ctx, timeout), s.Snapshot, s.ProjectID, s.File, candidate.valuePosition)
		if err != nil {
			s.Warnings = append(s.Warnings, fmt.Sprintf("get type at position %d: %v", candidate.valuePosition, err))
			continue
		}
		if typ == nil {
			continue
		}
		if positionType == "" {
			positionType = typ.ID
		}
		s.bootstrapTypeLocations(ctx, timeout, typ.ID)
		if s.SampleSignature == "" {
			if signature, err := s.firstSignatureOnType(ctx, timeout, typ.ID); err == nil && signature != "" {
				s.SampleSignature = signature
			} else if err != nil {
				s.Warnings = append(s.Warnings, fmt.Sprintf("find first signature on type %s: %v", typ.ID, err))
			}
		}
		signatures, err := s.Client.GetSignaturesOfType(tsgoapi.WithTimeout(ctx, timeout), s.Snapshot, s.ProjectID, typ.ID)
		if err != nil {
			s.Warnings = append(s.Warnings, fmt.Sprintf("get signatures of type %s: %v", typ.ID, err))
			continue
		}
		if len(signatures) > 0 && signatures[0] != nil {
			signatureType = typ.ID
			s.SampleSignature = signatures[0].ID
			s.SamplePosition = candidate.valuePosition
			if currentSymbol != "" {
				s.SampleSymbolPos = candidate.namePosition
			}
			break
		}
	}

	switch {
	case signatureType != "":
		s.SampleType = signatureType
	case symbolType != "":
		s.SampleType = symbolType
	case positionType != "":
		s.SampleType = positionType
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
	if s.SampleSymbolPos == 0 {
		s.SampleSymbolPos = s.SamplePosition
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
	for _, handle := range handles {
		handle = strings.TrimSpace(handle)
		if handle == "" || handle == invalidProbeHandle {
			continue
		}
		duplicate := false
		for _, existing := range s.SampleLocations {
			if existing == handle {
				duplicate = true
				break
			}
		}
		if !duplicate {
			s.SampleLocations = append(s.SampleLocations, handle)
		}
	}
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

func probeParamCandidates(method string, session *probeSession) []any {
	if !probeMethodUsesLocation(method) || len(session.SampleLocations) <= 1 {
		return []any{paramsForProbeMethod(method, session)}
	}
	candidates := make([]any, 0, len(session.SampleLocations))
	for _, location := range session.SampleLocations {
		candidates = append(candidates, paramsForProbeMethodWithLocation(method, session, location))
	}
	return candidates
}

func probeMethodUsesLocation(method string) bool {
	switch method {
	case "getContextualType", "getSymbolAtLocation", "getSymbolsAtLocations", "getTypeAtLocation", "getTypeOfSymbolAtLocation":
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
	}
}

func paramsForProbeMethod(method string, session *probeSession) any {
	return paramsForProbeMethodWithLocation(method, session, probeLocationHandle(session))
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
	case "getBaseTypeOfLiteralType", "getBaseTypeOfType", "getBaseTypes", "getCheckTypeOfType", "getConstraintOfType", "getExtendsTypeOfType", "getIndexInfosOfType", "getIndexTypeOfType", "getLocalTypeParametersOfType", "getObjectTypeOfType", "getOuterTypeParametersOfType", "getSymbolOfType", "getTargetOfType", "getTypeArguments", "getTypeParametersOfType", "getTypesOfType":
		return withType()
	case "getDeclaredTypeOfSymbol", "getExportSymbolOfSymbol", "getExportsOfSymbol", "getMembersOfSymbol", "getParentOfSymbol":
		return withSymbol()
	case "getRestTypeOfSignature", "getReturnTypeOfSignature", "getTypePredicateOfSignature":
		params := base()
		params["signature"] = probeSignatureHandle(session)
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
	case "getContextualType", "getSymbolAtLocation", "getTypeAtLocation":
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

func probeLocationHandle(session *probeSession) string {
	if len(session.SampleLocations) > 0 {
		return session.SampleLocations[0]
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
