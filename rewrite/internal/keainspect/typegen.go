package keainspect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"kea-typegen/rewrite/internal/tsgoapi"
)

type AppOptions struct {
	BinaryPath        string
	TsConfigPath      string
	PackageJSONPath   string
	SourceFilePath    string
	RootPath          string
	TypesPath         string
	WorkingDir        string
	KeaConfigPath     string
	Write             bool
	Watch             bool
	Quiet             bool
	Verbose           bool
	NoImport          bool
	ImportGlobalTypes bool
	IgnoreImportPaths []string
	WritePaths        bool
	Delete            bool
	AddTsNocheck      bool
	ConvertToBuilders bool
	ShowTSErrors      bool
	UseCache          bool
	Timeout           time.Duration
	PollInterval      time.Duration
	Log               func(string)
}

type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit with status %d", e.Code)
}

func (e *ExitCodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type keaConfig struct {
	RootPath          *string  `json:"rootPath"`
	TypesPath         *string  `json:"typesPath"`
	TsConfigPath      *string  `json:"tsConfigPath"`
	SourceFilePath    *string  `json:"sourceFilePath"`
	PackageJSONPath   *string  `json:"packageJsonPath"`
	Quiet             *bool    `json:"quiet"`
	NoImport          *bool    `json:"noImport"`
	WritePaths        *bool    `json:"writePaths"`
	Delete            *bool    `json:"delete"`
	AddTsNocheck      *bool    `json:"addTsNocheck"`
	ConvertToBuilders *bool    `json:"convertToBuilders"`
	ImportGlobalTypes *bool    `json:"importGlobalTypes"`
	IgnoreImportPaths []string `json:"ignoreImportPaths"`
	ShowTSErrors      *bool    `json:"showTsErrors"`
	UseCache          *bool    `json:"useCache"`
	Verbose           *bool    `json:"verbose"`
}

type runSummary struct {
	FilesToWrite  int
	WrittenFiles  int
	FilesToModify int
}

type parsedFile struct {
	File         string
	Source       string
	SourceLogics []SourceLogic
	Logics       []ParsedLogic
	TypeFile     string
}

type fileEmitOptions struct {
	TypeFile          string
	SourceFile        string
	PackageJSONPath   string
	RootPath          string
	AddTsNocheck      bool
	ImportGlobalTypes bool
	IgnoreImportPaths []string
	CompilerTypes     []string
	GeneratedAt       time.Time
}

type namedImportDecl struct {
	Start          int
	End            int
	SpecifiersText string
	Path           string
	ResolvedPath   string
	DefaultImport  string
	IsTypeOnly     bool
}

type logicImportState struct {
	TypeArgStart          int
	TypeArgEnd            int
	HasTypeArg            bool
	HasExtraTypeArguments bool
	HasExpectedImport     bool
}

type sourceEditPlan struct {
	Source            string
	ImportEdits       []textEdit
	TypeArgumentEdits []textEdit
	PathEdits         []textEdit
	ImportCount       int
	PathCount         int
}

type textEdit struct {
	Start       int
	End         int
	Replacement string
}

func ResolveAppOptions(options AppOptions, setFlags map[string]bool, cwd string) (AppOptions, error) {
	resolved := options
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return AppOptions{}, err
		}
		cwd = wd
	}
	resolved.WorkingDir = cwd
	if resolved.Timeout <= 0 {
		resolved.Timeout = 15 * time.Second
	}
	if resolved.PollInterval <= 0 {
		resolved.PollInterval = time.Second
	}

	resolveCLIPath := func(value string) string {
		if value == "" {
			return ""
		}
		if filepath.IsAbs(value) {
			return filepath.Clean(value)
		}
		return filepath.Clean(filepath.Join(cwd, value))
	}
	resolveCLIPaths := func(values []string) []string {
		if len(values) == 0 {
			return nil
		}
		resolvedValues := make([]string, 0, len(values))
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				continue
			}
			resolvedValues = append(resolvedValues, resolveCLIPath(value))
		}
		return resolvedValues
	}

	resolved.BinaryPath = resolveCLIPath(resolved.BinaryPath)
	resolved.TsConfigPath = resolveCLIPath(resolved.TsConfigPath)
	resolved.PackageJSONPath = resolveCLIPath(resolved.PackageJSONPath)
	resolved.SourceFilePath = resolveCLIPath(resolved.SourceFilePath)
	resolved.RootPath = resolveCLIPath(resolved.RootPath)
	resolved.TypesPath = resolveCLIPath(resolved.TypesPath)
	resolved.IgnoreImportPaths = resolveCLIPaths(resolved.IgnoreImportPaths)

	configPath := ""
	if resolved.TsConfigPath != "" {
		configPath = findConfigUp(filepath.Dir(resolved.TsConfigPath), ".kearc")
	}
	if configPath == "" && resolved.RootPath != "" {
		configPath = findConfigUp(resolved.RootPath, ".kearc")
	}
	if configPath == "" {
		configPath = findConfigUp(cwd, ".kearc")
	}
	resolved.KeaConfigPath = configPath

	if configPath != "" {
		rawData, err := os.ReadFile(configPath)
		if err != nil {
			return AppOptions{}, fmt.Errorf("error reading Kea config file: %s", configPath)
		}
		var config keaConfig
		if err := json.Unmarshal(rawData, &config); err != nil {
			return AppOptions{}, fmt.Errorf("error parsing Kea config JSON: %s", configPath)
		}
		configDir := filepath.Dir(configPath)
		resolveConfigPath := func(value string) string {
			if filepath.IsAbs(value) {
				return filepath.Clean(value)
			}
			return filepath.Clean(filepath.Join(configDir, value))
		}
		resolveConfigPaths := func(values []string) []string {
			if len(values) == 0 {
				return nil
			}
			resolvedValues := make([]string, 0, len(values))
			for _, value := range values {
				if strings.TrimSpace(value) == "" {
					continue
				}
				resolvedValues = append(resolvedValues, resolveConfigPath(value))
			}
			return resolvedValues
		}

		if config.RootPath != nil && !flagWasSet(setFlags, "root", "r") {
			resolved.RootPath = resolveConfigPath(*config.RootPath)
		}
		if config.TypesPath != nil && !flagWasSet(setFlags, "types", "t") {
			resolved.TypesPath = resolveConfigPath(*config.TypesPath)
		}
		if config.TsConfigPath != nil && !flagWasSet(setFlags, "config", "c") {
			resolved.TsConfigPath = resolveConfigPath(*config.TsConfigPath)
		}
		if config.SourceFilePath != nil && !flagWasSet(setFlags, "file", "f") {
			resolved.SourceFilePath = resolveConfigPath(*config.SourceFilePath)
		}
		if config.PackageJSONPath != nil && !flagWasSet(setFlags, "package-json") {
			resolved.PackageJSONPath = resolveConfigPath(*config.PackageJSONPath)
		}
		if config.Quiet != nil && !flagWasSet(setFlags, "quiet", "q") {
			resolved.Quiet = *config.Quiet
		}
		if config.NoImport != nil && !flagWasSet(setFlags, "no-import") {
			resolved.NoImport = *config.NoImport
		}
		if config.WritePaths != nil && !flagWasSet(setFlags, "write-paths") {
			resolved.WritePaths = *config.WritePaths
		}
		if config.Delete != nil && !flagWasSet(setFlags, "delete") {
			resolved.Delete = *config.Delete
		}
		if config.AddTsNocheck != nil && !flagWasSet(setFlags, "add-ts-nocheck") {
			resolved.AddTsNocheck = *config.AddTsNocheck
		}
		if config.ConvertToBuilders != nil && !flagWasSet(setFlags, "convert-to-builders") {
			resolved.ConvertToBuilders = *config.ConvertToBuilders
		}
		if config.ImportGlobalTypes != nil && !flagWasSet(setFlags, "import-global-types") {
			resolved.ImportGlobalTypes = *config.ImportGlobalTypes
		}
		if len(config.IgnoreImportPaths) > 0 && !flagWasSet(setFlags, "ignore-import-paths") {
			resolved.IgnoreImportPaths = resolveConfigPaths(config.IgnoreImportPaths)
		}
		if config.ShowTSErrors != nil && !flagWasSet(setFlags, "show-ts-errors") {
			resolved.ShowTSErrors = *config.ShowTSErrors
		}
		if config.UseCache != nil && !flagWasSet(setFlags, "use-cache") {
			resolved.UseCache = *config.UseCache
		}
		if config.Verbose != nil && !flagWasSet(setFlags, "verbose") {
			resolved.Verbose = *config.Verbose
		}
	}

	if resolved.RootPath == "" {
		if resolved.TsConfigPath != "" {
			resolved.RootPath = filepath.Dir(resolved.TsConfigPath)
		} else {
			resolved.RootPath = cwd
		}
	}
	if resolved.TypesPath == "" {
		resolved.TypesPath = resolved.RootPath
	}
	if resolved.TsConfigPath == "" {
		resolved.TsConfigPath = firstNonEmpty(
			findConfigUp(resolved.RootPath, "tsconfig.json"),
			findConfigUp(cwd, "tsconfig.json"),
		)
	}
	if resolved.PackageJSONPath == "" {
		resolved.PackageJSONPath = firstNonEmpty(
			findConfigUp(resolved.RootPath, "package.json"),
			findConfigUp(cwd, "package.json"),
		)
	}
	if resolved.BinaryPath == "" {
		resolved.BinaryPath = filepath.Clean(tsgoapi.PreferredBinary(findRepoRoot(cwd)))
	}
	if resolved.Log == nil {
		if resolved.Quiet {
			resolved.Log = func(string) {}
		} else {
			resolved.Log = func(message string) {
				fmt.Println(message)
			}
		}
	}

	return resolved, nil
}

func RunTypegen(ctx context.Context, options AppOptions) error {
	if options.TsConfigPath == "" && options.SourceFilePath == "" {
		return fmt.Errorf("no tsconfig.json found and no source file specified")
	}

	if version := readPackageVersion(options.PackageJSONPath); version != "" {
		options.Log(fmt.Sprintf("🦜 Kea-TypeGen v%s", version))
	}

	if options.SourceFilePath != "" {
		options.Log(fmt.Sprintf("❇️ Loading file: %s", options.SourceFilePath))
	} else if options.TsConfigPath != "" {
		options.Log(fmt.Sprintf("🥚 TypeScript Config: %s", options.TsConfigPath))
	}

	if options.Watch {
		return watchTypegen(ctx, options)
	}

	summary, err := runTypegenRounds(ctx, options)
	if err != nil {
		return err
	}
	if !options.Write && !options.Watch && (summary.FilesToWrite > 0 || summary.FilesToModify > 0) {
		return &ExitCodeError{Code: 1}
	}
	return nil
}

func runTypegenRounds(ctx context.Context, options AppOptions) (runSummary, error) {
	if !options.Write {
		return processProjectOnce(ctx, options)
	}

	candidateFiles, err := candidateSourceFiles(options)
	if err != nil {
		return runSummary{}, err
	}
	if restored, err := restoreCachedTypes(candidateFiles, options); err != nil {
		return runSummary{}, err
	} else if restored {
		// A restored cache entry changes the next inspection round.
	}

	var summary runSummary
	for round := 1; ; round++ {
		summary, err = processProjectOnce(ctx, options)
		if err != nil {
			return summary, err
		}
		if summary.WrittenFiles == 0 && summary.FilesToModify == 0 {
			options.Log("👋 Finished writing files! Exiting.")
			return summary, nil
		}
		if round > 50 {
			return summary, fmt.Errorf("we seem to be stuck in a loop (ran %d times)", round)
		}
	}
}

func processProjectOnce(ctx context.Context, options AppOptions) (runSummary, error) {
	_ = ctx

	state := &buildState{
		binaryPath:    options.BinaryPath,
		projectDir:    options.RootPath,
		configFile:    options.TsConfigPath,
		timeout:       options.Timeout,
		parsedByFile:  map[string][]ParsedLogic{},
		building:      map[string]bool{},
		projectByFile: map[string]string{},
	}
	defer state.close()

	files, err := collectParsedFiles(options, state)
	if err != nil {
		return runSummary{}, err
	}
	if options.Verbose {
		logicCount := 0
		for _, file := range files {
			logicCount += len(file.Logics)
		}
		options.Log(fmt.Sprintf("🗒️ %d logic%s found!", logicCount, pluralSuffix(logicCount)))
	}

	if options.Delete {
		if err := deleteOrphanTypeFiles(state, options); err != nil {
			return runSummary{}, err
		}
	}

	return writeTypegenFiles(files, state.compilerTypes(), options)
}

func collectParsedFiles(options AppOptions, state *buildState) ([]parsedFile, error) {
	candidateFiles, err := candidateSourceFilesFromState(options, state)
	if err != nil {
		return nil, err
	}

	parsedFiles := make([]parsedFile, 0, len(candidateFiles))
	for _, file := range candidateFiles {
		if options.Verbose {
			options.Log(fmt.Sprintf("👀 Visiting: %s", relativeToWorkingDir(options, file)))
		}

		report, source, err := state.inspectFile(file)
		if err != nil {
			return nil, err
		}
		if len(report.Logics) == 0 {
			continue
		}

		sourceLogics, err := FindLogics(source)
		if err != nil {
			return nil, err
		}
		logics, err := buildParsedLogicsFromSource(report, source, state)
		if err != nil {
			return nil, err
		}
		state.parsedByFile[filepath.Clean(file)] = logics
		parsedFiles = append(parsedFiles, parsedFile{
			File:         file,
			Source:       source,
			SourceLogics: sourceLogics,
			Logics:       logics,
			TypeFile:     typeFileNameForSource(options, file),
		})
	}

	return parsedFiles, nil
}

func writeTypegenFiles(files []parsedFile, compilerTypes []string, options AppOptions) (runSummary, error) {
	ignoredPrefixes := ignoredImportPrefixes(options, compilerTypes)
	summary := runSummary{}
	generatedAt := time.Now().UTC()

	for _, file := range files {
		output := emitTypegenFile(file.Logics, fileEmitOptions{
			TypeFile:          file.TypeFile,
			SourceFile:        file.File,
			PackageJSONPath:   options.PackageJSONPath,
			RootPath:          options.RootPath,
			AddTsNocheck:      options.AddTsNocheck,
			ImportGlobalTypes: options.ImportGlobalTypes,
			IgnoreImportPaths: ignoredPrefixes,
			CompilerTypes:     compilerTypes,
			GeneratedAt:       generatedAt,
		})

		existing, _ := os.ReadFile(file.TypeFile)
		if len(existing) == 0 || !sameTypegenBody(string(existing), output) {
			summary.FilesToWrite++
			if options.Write {
				if err := os.MkdirAll(filepath.Dir(file.TypeFile), 0o755); err != nil {
					return summary, err
				}
				if err := os.WriteFile(file.TypeFile, []byte(output), 0o644); err != nil {
					return summary, err
				}
				cacheWrittenFile(file.TypeFile, options)
				summary.WrittenFiles++
				options.Log(fmt.Sprintf("🔥 Writing: %s", relativeToWorkingDir(options, file.TypeFile)))
			} else {
				options.Log(fmt.Sprintf("❌ Will not write: %s", relativeToWorkingDir(options, file.TypeFile)))
			}
		} else if options.Verbose {
			options.Log(fmt.Sprintf("🤷 Unchanged: %s", relativeToWorkingDir(options, file.TypeFile)))
		}

		plan, err := planSourceEdits(file, options)
		if err != nil {
			return summary, err
		}
		if plan.ImportCount > 0 {
			if options.Write && !options.NoImport {
				summary.FilesToModify += plan.ImportCount
			} else {
				options.Log(fmt.Sprintf("❌ Will not write %d logic type import%s", plan.ImportCount, pluralSuffix(plan.ImportCount)))
			}
		}
		if plan.PathCount > 0 {
			if options.Write && !options.NoImport {
				summary.FilesToModify += plan.PathCount
			} else {
				options.Log(fmt.Sprintf("❌ Will not write %d logic path%s", plan.PathCount, pluralSuffix(plan.PathCount)))
			}
		}
		convertCount := 0
		if options.ConvertToBuilders {
			convertCount = countConvertibleLogics(file.SourceLogics)
			if convertCount > 0 {
				if options.Write && !options.NoImport {
					summary.FilesToModify += convertCount
				} else {
					options.Log(fmt.Sprintf("❌ Will not convert %d logic%s to builders", convertCount, pluralSuffix(convertCount)))
				}
			}
		}

		if (plan.ImportCount > 0 || plan.PathCount > 0 || convertCount > 0) && options.Write && !options.NoImport {
			updatedSource := file.Source
			if plan.ImportCount > 0 || plan.PathCount > 0 {
				updatedSource, err = applySourceEditPlan(plan)
				if err != nil {
					return summary, err
				}
			}
			if convertCount > 0 {
				var warnings []string
				updatedSource, _, warnings, err = convertFileToBuilders(updatedSource, file, options)
				if err != nil {
					return summary, err
				}
				for _, warning := range warnings {
					options.Log(warning)
				}
			}
			if err := os.WriteFile(file.File, []byte(updatedSource), 0o644); err != nil {
				return summary, err
			}
			if plan.ImportCount > 0 {
				options.Log(fmt.Sprintf("🔥 Import added: %s", relativeToWorkingDir(options, file.File)))
			}
			if plan.PathCount > 0 {
				options.Log(fmt.Sprintf("🔥 Path added: %s", relativeToWorkingDir(options, file.File)))
			}
			if convertCount > 0 {
				options.Log(fmt.Sprintf("🔥 Converted to builders: %s", relativeToWorkingDir(options, file.File)))
			}
		}
	}

	if summary.WrittenFiles == 0 && summary.FilesToModify == 0 {
		logicCount := 0
		for _, file := range files {
			logicCount += len(file.Logics)
		}
		if options.Write {
			options.Log(fmt.Sprintf("💚 %d logic type%s up to date!", logicCount, pluralSuffix(logicCount)))
			options.Log("")
		} else if summary.FilesToWrite > 0 || summary.FilesToModify > 0 {
			options.Log(fmt.Sprintf("🚨 Run \"kea-typegen write\" to save %d file%s to disk", summary.FilesToWrite+summary.FilesToModify, pluralSuffix(summary.FilesToWrite+summary.FilesToModify)))
		}
	}

	return summary, nil
}

func emitTypegenFile(logics []ParsedLogic, options fileEmitOptions) string {
	var builder strings.Builder

	headerLines := []string{
		fmt.Sprintf("// Generated by kea-typegen on %s. DO NOT EDIT THIS FILE MANUALLY.", options.GeneratedAt.Format(time.RFC1123)),
	}
	if options.AddTsNocheck {
		headerLines = append(headerLines, "// @ts-nocheck")
	}
	builder.WriteString(strings.Join(headerLines, "\n"))
	builder.WriteString("\n\n")
	builder.WriteString(fmt.Sprintf("import type { %s } from 'kea'\n", strings.Join(keaImports(logics), ", ")))

	imports := normalizedTypeImports(logics, options)
	if len(imports) > 0 {
		builder.WriteByte('\n')
		for _, item := range imports {
			for _, line := range renderImportLines(item) {
				builder.WriteString(line)
				builder.WriteByte('\n')
			}
		}
	}

	for index, logic := range logics {
		builder.WriteString("\n")
		if index > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(renderInterface(logic))
	}

	return builder.String()
}

func normalizedTypeImports(logics []ParsedLogic, options fileEmitOptions) []TypeImport {
	grouped := map[string]map[string]bool{}
	for _, logic := range logics {
		for _, item := range logic.Imports {
			finalPath, fullPath := normalizeImportPath(item.Path, logic.File, options.TypeFile, options.PackageJSONPath)
			if finalPath == "" || shouldIgnoreImportPath(fullPath, options.IgnoreImportPaths) {
				continue
			}
			grouped[finalPath] = grouped[finalPath]
			if grouped[finalPath] == nil {
				grouped[finalPath] = map[string]bool{}
			}
			for _, name := range item.Names {
				grouped[finalPath][name] = true
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

func planSourceEdits(file parsedFile, options AppOptions) (sourceEditPlan, error) {
	plan := sourceEditPlan{Source: file.Source}
	if options.NoImport {
		return plan, nil
	}

	importStates, imports, typeImportPath, needsImportCleanup, err := analyzeTypeImportNeeds(file)
	if err != nil {
		return plan, err
	}
	needsTypeImportEdits := needsImportCleanup
	for _, state := range importStates {
		if !state.HasExpectedImport || state.HasExtraTypeArguments {
			plan.ImportCount++
			needsTypeImportEdits = true
		}
	}
	if needsImportCleanup && plan.ImportCount == 0 {
		plan.ImportCount++
	}

	if needsTypeImportEdits || plan.ImportCount > 0 {
		importEdits, typeArgEdits, err := buildTypeImportEdits(file, imports, importStates, typeImportPath)
		if err != nil {
			return plan, err
		}
		plan.ImportEdits = append(plan.ImportEdits, importEdits...)
		plan.TypeArgumentEdits = append(plan.TypeArgumentEdits, typeArgEdits...)
	}

	if options.WritePaths {
		pathEdits, pathCount, err := buildPathEdits(file)
		if err != nil {
			return plan, err
		}
		plan.PathEdits = append(plan.PathEdits, pathEdits...)
		plan.PathCount = pathCount
	}

	return plan, nil
}

func analyzeTypeImportNeeds(file parsedFile) ([]logicImportState, []namedImportDecl, string, bool, error) {
	decls := findNamedImportDecls(file.Source, file.File)
	typeImportPath := typeImportLocation(file.File, file.TypeFile)
	typeImportAbs := filepath.Clean(file.TypeFile)
	states := make([]logicImportState, 0, len(file.Logics))

	for index, logic := range file.Logics {
		sourceLogic := file.SourceLogics[index]
		typeArgStart, typeArgEnd, typeArgText, hasTypeArg, err := keaTypeArgumentSpan(file.Source, sourceLogic)
		if err != nil {
			return nil, nil, "", false, err
		}
		state := logicImportState{
			TypeArgStart: typeArgStart,
			TypeArgEnd:   typeArgEnd,
			HasTypeArg:   hasTypeArg,
		}

		typeName := strings.TrimSpace(typeArgText)
		if strings.Contains(typeName, "<") {
			state.HasExtraTypeArguments = true
			typeName = strings.TrimSpace(typeName[:strings.Index(typeName, "<")])
		}
		if typeName == logic.TypeName {
			for _, decl := range decls {
				if !typeImportDeclMatches(decl, typeImportAbs, typeImportPath) {
					continue
				}
				specifiers := parseImportSpecifiers(decl.SpecifiersText, decl.Path)
				if _, ok := specifiers[logic.TypeName]; ok {
					state.HasExpectedImport = true
					break
				}
			}
		}
		states = append(states, state)
	}

	_, desiredNames := desiredTypeImportNames(file.Logics)
	exactDecls, staleDecls := matchingTypeImportDecls(decls, typeImportAbs, typeImportPath, desiredNames)
	needsImportCleanup := len(staleDecls) > 0 || len(exactDecls) > 1

	return states, decls, typeImportPath, needsImportCleanup, nil
}

func buildTypeImportEdits(file parsedFile, decls []namedImportDecl, states []logicImportState, typeImportPath string) ([]textEdit, []textEdit, error) {
	typeNames, desiredNames := desiredTypeImportNames(file.Logics)
	importLine := fmt.Sprintf("import type { %s } from '%s'", strings.Join(typeNames, ", "), typeImportPath)
	typeImportAbs := filepath.Clean(file.TypeFile)

	var importEdits []textEdit
	exactDecls, staleDecls := matchingTypeImportDecls(decls, typeImportAbs, typeImportPath, desiredNames)
	if len(exactDecls) == 0 && len(staleDecls) == 0 {
		insertPos, err := importInsertionPoint(file.Source)
		if err != nil {
			return nil, nil, err
		}
		replacement := importLine + "\n"
		importEdits = append(importEdits, textEdit{Start: insertPos, End: insertPos, Replacement: replacement})
	} else {
		primary := firstTypeImportDecl(exactDecls, staleDecls)
		replacementSpecifiers := typeNames
		if len(exactDecls) > 0 {
			replacementSpecifiers = mergeImportSpecifierLists(splitImportSpecifierList(primary.SpecifiersText), typeNames)
		}
		replacement := replacementForTypeImportDecl(file.Source, primary, typeImportPath, replacementSpecifiers)
		if file.Source[primary.Start:primary.End] != replacement {
			importEdits = append(importEdits, textEdit{Start: primary.Start, End: primary.End, Replacement: replacement})
		}

		for _, decl := range exactDecls {
			if decl.Start == primary.Start && decl.End == primary.End {
				continue
			}
			if typeImportDeclUsesOnlyNames(decl, desiredNames) {
				importEdits = append(importEdits, textEdit{Start: decl.Start, End: decl.End, Replacement: ""})
			}
		}
		for _, decl := range staleDecls {
			if decl.Start == primary.Start && decl.End == primary.End {
				continue
			}
			importEdits = append(importEdits, textEdit{Start: decl.Start, End: decl.End, Replacement: ""})
		}
	}

	typeArgEdits := make([]textEdit, 0, len(states))
	for index, state := range states {
		desired := "<" + file.Logics[index].TypeName + ">"
		if state.HasTypeArg {
			if strings.TrimSpace(file.Source[state.TypeArgStart:state.TypeArgEnd]) == desired && !state.HasExtraTypeArguments && state.HasExpectedImport {
				continue
			}
			typeArgEdits = append(typeArgEdits, textEdit{Start: state.TypeArgStart, End: state.TypeArgEnd, Replacement: desired})
			continue
		}
		insertPos := skipTrivia(file.Source, file.SourceLogics[index].KeaStart+len("kea"))
		typeArgEdits = append(typeArgEdits, textEdit{Start: insertPos, End: insertPos, Replacement: desired})
	}

	return importEdits, typeArgEdits, nil
}

func buildPathEdits(file parsedFile) ([]textEdit, int, error) {
	needBuilderPath := false
	pathCount := 0
	pathEdits := make([]textEdit, 0, len(file.Logics)+1)

	for index, logic := range file.Logics {
		sourceLogic := file.SourceLogics[index]
		if hasSourceProperty(sourceLogic.Properties, "path") {
			continue
		}
		pathCount++

		pathCode := formatPathSegments(logic.Path)
		if sourceLogic.InputKind == "builders" {
			needBuilderPath = true
			insert := insertionTextForContainer(file.Source, sourceLogic.ObjectStart, sourceLogic.ObjectEnd, "path("+pathCode+"),", ']')
			pathEdits = append(pathEdits, textEdit{
				Start:       sourceLogic.ObjectStart + 1,
				End:         sourceLogic.ObjectStart + 1,
				Replacement: insert,
			})
		} else {
			insert := insertionTextForContainer(file.Source, sourceLogic.ObjectStart, sourceLogic.ObjectEnd, "path: "+pathCode+",", '}')
			pathEdits = append(pathEdits, textEdit{
				Start:       sourceLogic.ObjectStart + 1,
				End:         sourceLogic.ObjectStart + 1,
				Replacement: insert,
			})
		}
	}

	if !needBuilderPath {
		return pathEdits, pathCount, nil
	}

	alias, importDecl, ok := pathImportAliasAndDecl(file.Source, file.File)
	if ok {
		if alias != "path" {
			for index, edit := range pathEdits {
				pathEdits[index].Replacement = strings.ReplaceAll(edit.Replacement, "path(", alias+"(")
			}
		}
		if strings.Contains(alias, ".") {
			return pathEdits, pathCount, nil
		}
		if importDecl.Start != 0 || importDecl.End != 0 {
			return pathEdits, pathCount, nil
		}
	}

	localName := alias
	if localName == "" {
		localName = nextAvailableIdentifier(file.Source, "path", "logicPath")
	}
	for index, edit := range pathEdits {
		pathEdits[index].Replacement = strings.ReplaceAll(edit.Replacement, "path(", localName+"(")
	}

	valueDecls := findNamedImportDecls(file.Source, file.File)
	for _, decl := range valueDecls {
		if decl.Path != "kea" {
			continue
		}
		specifiers := parseImportSpecifiers(decl.SpecifiersText, decl.Path)
		if existing, ok := specifiers[localName]; ok && existing.ImportedName == "path" {
			return pathEdits, pathCount, nil
		}
		newSpecifier := "path"
		if localName != "path" {
			newSpecifier = "path as " + localName
		}
		specifierList := splitImportSpecifierList(decl.SpecifiersText)
		specifierList = append(specifierList, newSpecifier)
		pathEdits = append(pathEdits, textEdit{
			Start:       decl.Start,
			End:         decl.End,
			Replacement: replacementForImportDecl(file.Source, decl, specifierList),
		})
		return pathEdits, pathCount, nil
	}

	insertPos, err := importInsertionPoint(file.Source)
	if err != nil {
		return nil, 0, err
	}
	importText := fmt.Sprintf("import { path } from 'kea'\n")
	if localName != "path" {
		importText = fmt.Sprintf("import { path as %s } from 'kea'\n", localName)
	}
	pathEdits = append(pathEdits, textEdit{Start: insertPos, End: insertPos, Replacement: importText})
	return pathEdits, pathCount, nil
}

func applySourceEditPlan(plan sourceEditPlan) (string, error) {
	allEdits := make([]textEdit, 0, len(plan.ImportEdits)+len(plan.TypeArgumentEdits)+len(plan.PathEdits))
	allEdits = append(allEdits, plan.ImportEdits...)
	allEdits = append(allEdits, plan.TypeArgumentEdits...)
	allEdits = append(allEdits, plan.PathEdits...)
	return applyTextEdits(plan.Source, allEdits)
}

func applyTextEdits(source string, edits []textEdit) (string, error) {
	if len(edits) == 0 {
		return source, nil
	}
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].Start == edits[j].Start {
			return edits[i].End > edits[j].End
		}
		return edits[i].Start > edits[j].Start
	})

	result := source
	lastStart := len(source) + 1
	for _, edit := range edits {
		if edit.Start < 0 || edit.End < edit.Start || edit.End > len(result) {
			return "", fmt.Errorf("invalid text edit range %d:%d", edit.Start, edit.End)
		}
		if edit.End > lastStart {
			return "", fmt.Errorf("overlapping text edits around %d:%d", edit.Start, edit.End)
		}
		result = result[:edit.Start] + edit.Replacement + result[edit.End:]
		lastStart = edit.Start
	}
	return result, nil
}

func watchTypegen(ctx context.Context, options AppOptions) error {
	options.Log("👀 Starting TypeScript watch mode")

	if _, err := runTypegenRounds(ctx, options); err != nil {
		return err
	}

	lastSnapshot, err := watchSnapshot(options)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(options.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			nextSnapshot, err := watchSnapshot(options)
			if err != nil {
				return err
			}
			if !watchSnapshotsEqual(lastSnapshot, nextSnapshot) {
				options.Log("🔄 Reloading...")
				if _, err := runTypegenRounds(ctx, options); err != nil {
					var exitErr *ExitCodeError
					if errors.As(err, &exitErr) && exitErr.Code == 1 {
						lastSnapshot = nextSnapshot
						continue
					}
					return err
				}
				lastSnapshot = nextSnapshot
			}
		}
	}
}

func candidateSourceFiles(options AppOptions) ([]string, error) {
	state := &buildState{
		binaryPath:    options.BinaryPath,
		projectDir:    options.RootPath,
		configFile:    options.TsConfigPath,
		timeout:       options.Timeout,
		parsedByFile:  map[string][]ParsedLogic{},
		building:      map[string]bool{},
		projectByFile: map[string]string{},
	}
	defer state.close()
	return candidateSourceFilesFromState(options, state)
}

func candidateSourceFilesFromState(options AppOptions, state *buildState) ([]string, error) {
	if options.SourceFilePath != "" {
		return []string{filepath.Clean(options.SourceFilePath)}, nil
	}
	if options.TsConfigPath == "" {
		return nil, fmt.Errorf("missing tsconfig path")
	}
	files, err := state.configFileNames()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	candidates := make([]string, 0, len(files))
	for _, file := range files {
		file = filepath.Clean(file)
		if seen[file] || !shouldConsiderSourceFile(file) {
			continue
		}
		seen[file] = true
		candidates = append(candidates, file)
	}
	sort.Strings(candidates)
	return candidates, nil
}

func deleteOrphanTypeFiles(state *buildState, options AppOptions) error {
	files, err := state.configFileNames()
	if err != nil {
		return err
	}
	extensions := []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".mjsx"}
	for _, file := range files {
		if !strings.HasSuffix(file, "Type.ts") {
			continue
		}
		keep := false
		for _, extension := range extensions {
			if _, err := os.Stat(strings.TrimSuffix(file, "Type.ts") + extension); err == nil {
				keep = true
				break
			}
		}
		if keep {
			continue
		}
		options.Log(fmt.Sprintf("🗑️ Deleting: %s", relativeToWorkingDir(options, file)))
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func restoreCachedTypes(files []string, options AppOptions) (bool, error) {
	if !options.UseCache {
		return false, nil
	}
	restored := false
	for _, file := range files {
		typeFile := typeFileNameForSource(options, file)
		if _, err := os.Stat(typeFile); err == nil {
			continue
		}
		from := cachePath(options, typeFile)
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(typeFile), 0o755); err != nil {
			return restored, err
		}
		if err := copyFile(from, typeFile); err != nil {
			return restored, err
		}
		options.Log(fmt.Sprintf("♻️ Restored from cache: %s", relativeToWorkingDir(options, typeFile)))
		restored = true
	}
	return restored, nil
}

func cacheWrittenFile(fileName string, options AppOptions) {
	if !options.UseCache {
		return
	}
	dest := cachePath(options, fileName)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return
	}
	_ = copyFile(fileName, dest)
}

func cachePath(options AppOptions, fileName string) string {
	relative, err := filepath.Rel(options.WorkingDir, fileName)
	if err != nil {
		relative = filepath.Base(fileName)
	}
	return filepath.Join(options.WorkingDir, ".typegen", relative)
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o644)
}

func watchSnapshot(options AppOptions) (map[string]time.Time, error) {
	snapshot := map[string]time.Time{}
	record := func(path string) {
		if path == "" {
			return
		}
		if info, err := os.Stat(path); err == nil {
			snapshot[filepath.Clean(path)] = info.ModTime()
		}
	}
	record(options.TsConfigPath)
	record(options.KeaConfigPath)
	record(options.PackageJSONPath)
	if options.SourceFilePath != "" {
		record(options.SourceFilePath)
		return snapshot, nil
	}

	root := options.RootPath
	if root == "" {
		root = options.WorkingDir
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		base := entry.Name()
		if entry.IsDir() && (base == "node_modules" || base == ".git" || strings.HasPrefix(base, ".")) {
			if path == root || base == ".typegen" {
				return nil
			}
			return filepath.SkipDir
		}
		if entry.IsDir() || !shouldConsiderSourceFile(path) {
			return nil
		}
		record(path)
		return nil
	})
	return snapshot, err
}

func watchSnapshotsEqual(a, b map[string]time.Time) bool {
	if len(a) != len(b) {
		return false
	}
	for path, modTime := range a {
		if !modTime.Equal(b[path]) {
			return false
		}
	}
	return true
}

func ignoredImportPrefixes(options AppOptions, compilerTypes []string) []string {
	prefixes := append([]string(nil), options.IgnoreImportPaths...)
	if options.ImportGlobalTypes {
		return prefixes
	}
	baseDir := options.RootPath
	if options.PackageJSONPath != "" {
		baseDir = filepath.Dir(options.PackageJSONPath)
	}
	for _, compilerType := range compilerTypes {
		if strings.TrimSpace(compilerType) == "" {
			continue
		}
		prefixes = append(prefixes, filepath.Join(baseDir, "node_modules", "@types", compilerType)+string(os.PathSeparator))
	}
	return prefixes
}

func shouldIgnoreImportPath(fullPath string, ignoredPrefixes []string) bool {
	if fullPath == "" {
		return false
	}
	for _, prefix := range ignoredPrefixes {
		if strings.HasPrefix(fullPath, prefix) {
			return true
		}
	}
	return false
}

func normalizeImportPath(importPath, sourceFile, typeFile, packageJSONPath string) (string, string) {
	finalPath := importPath
	fullPath := importPath

	nodeModulesPath := filepath.Join(filepath.Dir(firstNonEmpty(packageJSONPath, filepath.Join(filepath.Dir(sourceFile), "package.json"))), "node_modules")
	if strings.HasPrefix(finalPath, ".") {
		if resolvedFile, ok := resolveLocalImportFile(sourceFile, finalPath); ok {
			fullPath = resolvedFile
		} else {
			fullPath = filepath.Clean(filepath.Join(filepath.Dir(sourceFile), finalPath))
		}
		finalPath = fullPath
	} else if strings.HasPrefix(finalPath, "node_modules"+string(os.PathSeparator)) {
		fullPath = filepath.Clean(filepath.Join(filepath.Dir(nodeModulesPath), finalPath))
		finalPath = fullPath
	}

	if strings.HasPrefix(finalPath, nodeModulesPath+string(os.PathSeparator)) {
		finalPath = strings.TrimPrefix(finalPath, nodeModulesPath+string(os.PathSeparator))
		if strings.HasPrefix(finalPath, ".pnpm/") {
			regex := regexp.MustCompile(`\.pnpm/[^/]+@[^/]+/node_modules/(.*)`)
			if matches := regex.FindStringSubmatch(finalPath); len(matches) == 2 {
				finalPath = matches[1]
			}
		}
		if strings.HasPrefix(finalPath, "@types/") {
			finalPath = strings.TrimPrefix(finalPath, "@types/")
		}
	} else if filepath.IsAbs(finalPath) {
		relative, err := filepath.Rel(filepath.Dir(typeFile), finalPath)
		if err == nil {
			finalPath = filepath.ToSlash(relative)
			if !strings.HasPrefix(finalPath, ".") {
				finalPath = "./" + finalPath
			}
		}
	}

	finalPath = strings.TrimSuffix(finalPath, ".d.ts")
	finalPath = strings.TrimSuffix(finalPath, ".ts")
	finalPath = strings.TrimSuffix(finalPath, ".tsx")
	finalPath = strings.TrimSuffix(finalPath, ".js")
	finalPath = strings.TrimSuffix(finalPath, ".jsx")
	if strings.Count(finalPath, "/") == 1 && strings.HasSuffix(finalPath, "/index") {
		finalPath = strings.TrimSuffix(finalPath, "/index")
	}
	return filepath.ToSlash(finalPath), fullPath
}

func typeFileNameForSource(options AppOptions, sourceFile string) string {
	typeFile := sourceFile
	switch {
	case strings.HasSuffix(typeFile, ".tsx"):
		typeFile = strings.TrimSuffix(typeFile, ".tsx") + "Type.ts"
	case strings.HasSuffix(typeFile, ".ts"):
		typeFile = strings.TrimSuffix(typeFile, ".ts") + "Type.ts"
	case strings.HasSuffix(typeFile, ".jsx"):
		typeFile = strings.TrimSuffix(typeFile, ".jsx") + "Type.ts"
	case strings.HasSuffix(typeFile, ".js"):
		typeFile = strings.TrimSuffix(typeFile, ".js") + "Type.ts"
	case strings.HasSuffix(typeFile, ".mjsx"):
		typeFile = strings.TrimSuffix(typeFile, ".mjsx") + "Type.ts"
	case strings.HasSuffix(typeFile, ".mjs"):
		typeFile = strings.TrimSuffix(typeFile, ".mjs") + "Type.ts"
	default:
		typeFile = typeFile + "Type.ts"
	}
	if options.RootPath != "" && options.TypesPath != "" {
		if relative, err := filepath.Rel(options.RootPath, typeFile); err == nil {
			typeFile = filepath.Join(options.TypesPath, relative)
		}
	}
	return filepath.Clean(typeFile)
}

func typeImportLocation(sourceFile, typeFile string) string {
	relative, err := filepath.Rel(filepath.Dir(sourceFile), typeFile)
	if err != nil {
		return "./" + filepath.Base(strings.TrimSuffix(typeFile, ".ts"))
	}
	relative = filepath.ToSlash(strings.TrimSuffix(relative, ".ts"))
	if !strings.HasPrefix(relative, ".") {
		relative = "./" + relative
	}
	return relative
}

func sameTypegenBody(existing, next string) bool {
	return stripFirstLine(existing) == stripFirstLine(next)
}

func stripFirstLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[index+1:]
	}
	return ""
}

func hasSourceProperty(properties []SourceProperty, name string) bool {
	for _, property := range properties {
		if property.Name == name {
			return true
		}
	}
	return false
}

func formatPathSegments(segments []string) string {
	filtered := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment == ".." {
			continue
		}
		filtered = append(filtered, quoteString(segment))
	}
	return "[" + strings.Join(filtered, ", ") + "]"
}

func insertionTextForContainer(source string, openPos, closePos int, value string, closing byte) string {
	contentStart := skipTrivia(source, openPos+1)
	childIndent := containerChildIndent(source, openPos, closePos)
	parentIndent := lineIndent(source, openPos)
	if contentStart >= closePos || source[contentStart] == closing {
		return "\n" + childIndent + value + "\n" + parentIndent
	}
	if strings.Contains(source[openPos+1:closePos], "\n") {
		return "\n" + childIndent + value
	}
	return " " + value + " "
}

func containerChildIndent(source string, openPos, closePos int) string {
	contentStart := skipTrivia(source, openPos+1)
	if contentStart < closePos {
		start := strings.LastIndexByte(source[:contentStart], '\n')
		if start >= 0 {
			start++
		} else {
			start = 0
		}
		indentEnd := start
		for indentEnd < len(source) && (source[indentEnd] == ' ' || source[indentEnd] == '\t') {
			indentEnd++
		}
		if indentEnd < contentStart {
			return source[start:contentStart]
		}
	}
	return lineIndent(source, openPos) + "    "
}

func lineIndent(source string, pos int) string {
	start := strings.LastIndexByte(source[:pos], '\n')
	if start >= 0 {
		start++
	} else {
		start = 0
	}
	end := start
	for end < len(source) && (source[end] == ' ' || source[end] == '\t') {
		end++
	}
	return source[start:end]
}

func pathImportAliasAndDecl(source, file string) (string, namedImportDecl, bool) {
	decls := findNamedImportDecls(source, file)
	imports := parseNamedValueImports(source)
	if existing, ok := imports["path"]; ok && existing.Path == "kea" && existing.ImportedName == "path" {
		for _, decl := range decls {
			if decl.Path == "kea" {
				return "path", decl, true
			}
		}
		return "path", namedImportDecl{}, true
	}
	if existing, ok := imports["logicPath"]; ok && existing.Path == "kea" && existing.ImportedName == "path" {
		for _, decl := range decls {
			if decl.Path == "kea" {
				return "logicPath", decl, true
			}
		}
		return "logicPath", namedImportDecl{}, true
	}
	namespaceAliases := make([]string, 0)
	for alias, importPath := range parseNamespaceValueImports(source) {
		if importPath == "kea" {
			namespaceAliases = append(namespaceAliases, alias)
		}
	}
	sort.Strings(namespaceAliases)
	if len(namespaceAliases) > 0 {
		return namespaceAliases[0] + ".path", namedImportDecl{}, true
	}
	return "", namedImportDecl{}, false
}

func nextAvailableIdentifier(source string, candidates ...string) string {
	imports := parseNamedValueImports(source)
	for _, candidate := range candidates {
		if _, ok := imports[candidate]; !ok {
			return candidate
		}
	}
	base := candidates[len(candidates)-1]
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s%d", base, index)
		if _, ok := imports[candidate]; !ok {
			return candidate
		}
	}
}

func splitImportSpecifierList(text string) []string {
	parts := strings.Split(text, ",")
	specifiers := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		specifiers = append(specifiers, part)
	}
	return specifiers
}

func desiredTypeImportNames(logics []ParsedLogic) ([]string, map[string]bool) {
	typeNames := make([]string, 0, len(logics))
	desiredNames := make(map[string]bool, len(logics))
	for _, logic := range logics {
		typeNames = append(typeNames, logic.TypeName)
		desiredNames[logic.TypeName] = true
	}
	sort.Strings(typeNames)
	return typeNames, desiredNames
}

func matchingTypeImportDecls(decls []namedImportDecl, typeImportAbs, typeImportPath string, desiredNames map[string]bool) ([]namedImportDecl, []namedImportDecl) {
	exactDecls := make([]namedImportDecl, 0)
	staleDecls := make([]namedImportDecl, 0)
	for _, decl := range decls {
		if !decl.IsTypeOnly {
			continue
		}
		if typeImportDeclMatches(decl, typeImportAbs, typeImportPath) {
			exactDecls = append(exactDecls, decl)
			continue
		}
		if typeImportDeclUsesOnlyNames(decl, desiredNames) {
			staleDecls = append(staleDecls, decl)
		}
	}
	return exactDecls, staleDecls
}

func firstTypeImportDecl(exactDecls, staleDecls []namedImportDecl) namedImportDecl {
	if len(exactDecls) > 0 {
		return exactDecls[0]
	}
	return staleDecls[0]
}

func typeImportDeclUsesOnlyNames(decl namedImportDecl, desiredNames map[string]bool) bool {
	if len(desiredNames) == 0 || decl.DefaultImport != "" {
		return false
	}
	specifiers := parseImportSpecifiers(decl.SpecifiersText, decl.Path)
	if len(specifiers) == 0 {
		return false
	}
	for localName := range specifiers {
		if !desiredNames[localName] {
			return false
		}
	}
	return true
}

func findNamedImportDecls(source, file string) []namedImportDecl {
	decls := make([]namedImportDecl, 0)
	for _, match := range importClausePattern.FindAllStringSubmatchIndex(source, -1) {
		if len(match) < 6 {
			continue
		}
		decls = append(decls, buildNamedImportDecl(source, file, match[0], match[1], match[2], match[3], match[4], match[5], ""))
	}
	for _, match := range importDefaultNamedPattern.FindAllStringSubmatchIndex(source, -1) {
		if len(match) < 8 {
			continue
		}
		decls = append(decls, buildNamedImportDecl(source, file, match[0], match[1], match[4], match[5], match[6], match[7], source[match[2]:match[3]]))
	}
	for _, match := range importDefaultPattern.FindAllStringSubmatchIndex(source, -1) {
		if len(match) < 6 {
			continue
		}
		decls = append(decls, buildNamedImportDecl(source, file, match[0], match[1], -1, -1, match[4], match[5], source[match[2]:match[3]]))
	}
	sort.Slice(decls, func(i, j int) bool {
		return decls[i].Start < decls[j].Start
	})
	return decls
}

func buildNamedImportDecl(source, file string, start, end, specStart, specEnd, pathStart, pathEnd int, defaultImport string) namedImportDecl {
	pathText := source[pathStart:pathEnd]
	resolved := pathText
	if strings.HasPrefix(pathText, ".") {
		resolvedFile, ok := resolveLocalImportFile(file, pathText)
		if ok {
			resolved = resolvedFile
		}
	}
	rawDecl := source[start:end]
	specifiersText := ""
	if specStart >= 0 && specEnd >= specStart {
		specifiersText = source[specStart:specEnd]
	}
	return namedImportDecl{
		Start:          start,
		End:            extendImportMatchEnd(source, end),
		SpecifiersText: specifiersText,
		Path:           pathText,
		ResolvedPath:   filepath.Clean(resolved),
		DefaultImport:  strings.TrimSpace(defaultImport),
		IsTypeOnly:     strings.Contains(rawDecl, "import type"),
	}
}

func replacementForTypeImportDecl(source string, decl namedImportDecl, importPath string, specifiers []string) string {
	trailing := importDeclTrailing(source, decl)
	if decl.DefaultImport != "" {
		return fmt.Sprintf("import type %s, { %s } from '%s'%s", decl.DefaultImport, strings.Join(specifiers, ", "), importPath, trailing)
	}
	return fmt.Sprintf("import type { %s } from '%s'%s", strings.Join(specifiers, ", "), importPath, trailing)
}

func importDeclTrailing(source string, decl namedImportDecl) string {
	trailing := "\n"
	if decl.Start >= 0 && decl.End <= len(source) && decl.End > decl.Start {
		raw := source[decl.Start:decl.End]
		trimmed := strings.TrimRight(raw, "\r\n")
		if len(trimmed) < len(raw) {
			trailing = raw[len(trimmed):]
		}
	}
	return trailing
}

func extendImportMatchEnd(source string, end int) int {
	for end < len(source) && (source[end] == ' ' || source[end] == '\t') {
		end++
	}
	if end < len(source) && source[end] == ';' {
		end++
	}
	for end < len(source) && (source[end] == '\r' || source[end] == '\n') {
		end++
	}
	return end
}

func typeImportDeclMatches(decl namedImportDecl, typeImportAbs, typeImportPath string) bool {
	if decl.ResolvedPath == typeImportAbs {
		return true
	}
	declPath := filepath.ToSlash(strings.TrimSpace(decl.Path))
	expectedPath := filepath.ToSlash(strings.TrimSpace(typeImportPath))
	if declPath == expectedPath {
		return true
	}
	return strings.TrimSuffix(declPath, ".ts") == expectedPath
}

func keaTypeArgumentSpan(source string, logic SourceLogic) (int, int, string, bool, error) {
	start := skipTrivia(source, logic.KeaStart+len("kea"))
	if start >= len(source) || source[start] != '<' {
		return start, start, "", false, nil
	}
	end, err := findMatching(source, start, '<', '>')
	if err != nil {
		return 0, 0, "", false, err
	}
	return start, end + 1, source[start+1 : end], true, nil
}

func importInsertionPoint(source string) (int, error) {
	position := skipTrivia(source, 0)
	for position < len(source) && matchesIdentifierAt(source, position, "import") {
		end, err := findImportStatementEnd(source, position)
		if err != nil {
			return 0, err
		}
		position = end
	}
	return position, nil
}

func findImportStatementEnd(source string, start int) (int, error) {
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
				return i + 1, nil
			}
		case '\n':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				statement := strings.TrimSpace(source[start:i])
				if strings.Contains(statement, " from ") || strings.HasPrefix(statement, "import '") || strings.HasPrefix(statement, "import \"") {
					return i + 1, nil
				}
			}
		}
	}
	return len(source), nil
}

func shouldConsiderSourceFile(file string) bool {
	if strings.HasSuffix(file, "Type.ts") || strings.HasSuffix(file, ".d.ts") {
		return false
	}
	for _, extension := range []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".mjsx"} {
		if strings.HasSuffix(file, extension) {
			return true
		}
	}
	return false
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func relativeToWorkingDir(options AppOptions, file string) string {
	if options.WorkingDir == "" {
		return file
	}
	if relative, err := filepath.Rel(options.WorkingDir, file); err == nil {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(file)
}

func findConfigUp(start, name string) string {
	if start == "" {
		return ""
	}
	current := filepath.Clean(start)
	for {
		candidate := filepath.Join(current, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func findRepoRoot(cwd string) string {
	candidates := []string{
		cwd,
		filepath.Dir(cwd),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "rewrite", "go.mod")); err == nil {
			return candidate
		}
	}
	return cwd
}

func readPackageVersion(packageJSONPath string) string {
	if packageJSONPath == "" {
		return ""
	}
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return ""
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	return payload.Version
}

func flagWasSet(setFlags map[string]bool, names ...string) bool {
	for _, name := range names {
		if setFlags[name] {
			return true
		}
	}
	return false
}
