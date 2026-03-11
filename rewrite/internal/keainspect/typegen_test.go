package keainspect

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kea-typegen/rewrite/internal/tsgoapi"
)

func TestResolveAppOptionsIncludesKeaConfig(t *testing.T) {
	tempDir := t.TempDir()
	rootPath := filepath.Join(tempDir, "frontend", "src")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatalf("os.MkdirAll rootPath: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tempDir, ".kearc"), []byte(`{
		"ignoreImportPaths": ["./donotimport.ts"],
		"writePaths": true
	}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile .kearc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "tsconfig.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile tsconfig: %v", err)
	}

	options, err := ResolveAppOptions(AppOptions{RootPath: rootPath}, nil, tempDir)
	if err != nil {
		t.Fatalf("ResolveAppOptions returned error: %v", err)
	}

	if !options.WritePaths {
		t.Fatalf("expected writePaths from .kearc to be true")
	}
	expectedIgnore := filepath.Join(tempDir, "donotimport.ts")
	if len(options.IgnoreImportPaths) != 1 || options.IgnoreImportPaths[0] != expectedIgnore {
		t.Fatalf("unexpected ignoreImportPaths: %#v", options.IgnoreImportPaths)
	}
	expectedConfig := filepath.Join(tempDir, "tsconfig.json")
	if options.TsConfigPath != expectedConfig {
		t.Fatalf("unexpected tsConfigPath: %s", options.TsConfigPath)
	}
	if options.TypesPath != rootPath {
		t.Fatalf("unexpected typesPath: %s", options.TypesPath)
	}
}

func TestResolveAppOptionsSingleFileDiscoversProjectFromSourcePath(t *testing.T) {
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, "samples")
	sourceDir := filepath.Join(projectDir, "nested")
	sourceFile := filepath.Join(sourceDir, "logic.ts")
	otherDir := filepath.Join(tempDir, "rewrite")

	for _, dir := range []string{projectDir, sourceDir, otherDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll %s: %v", dir, err)
		}
	}
	for _, file := range []string{
		filepath.Join(projectDir, "tsconfig.json"),
		filepath.Join(projectDir, "package.json"),
		sourceFile,
	} {
		if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
			t.Fatalf("os.WriteFile %s: %v", file, err)
		}
	}

	options, err := ResolveAppOptions(AppOptions{SourceFilePath: sourceFile}, nil, otherDir)
	if err != nil {
		t.Fatalf("ResolveAppOptions returned error: %v", err)
	}

	if options.TsConfigPath != filepath.Join(projectDir, "tsconfig.json") {
		t.Fatalf("unexpected tsConfigPath: %s", options.TsConfigPath)
	}
	if options.PackageJSONPath != filepath.Join(projectDir, "package.json") {
		t.Fatalf("unexpected packageJsonPath: %s", options.PackageJSONPath)
	}
	if options.RootPath != projectDir {
		t.Fatalf("unexpected rootPath: %s", options.RootPath)
	}
	if options.TypesPath != projectDir {
		t.Fatalf("unexpected typesPath: %s", options.TypesPath)
	}
}

func TestNormalizedTypeImportsIgnoresResolvedRelativeFilesWithExternalTypesDir(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "src")
	typesDir := filepath.Join(tempDir, "generated")
	sourceFile := filepath.Join(sourceDir, "logic.ts")
	typeFile := filepath.Join(typesDir, "logicType.ts")

	for _, dir := range []string{sourceDir, typesDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll %s: %v", dir, err)
		}
	}
	for _, file := range []string{
		filepath.Join(tempDir, "package.json"),
		sourceFile,
		filepath.Join(sourceDir, "types.ts"),
		filepath.Join(sourceDir, "donotimport.ts"),
	} {
		if err := os.WriteFile(file, []byte(""), 0o644); err != nil {
			t.Fatalf("os.WriteFile %s: %v", file, err)
		}
	}

	imports := normalizedTypeImports([]ParsedLogic{
		{
			File: sourceFile,
			Imports: []TypeImport{
				{Path: "./types", Names: []string{"Kept"}},
				{Path: "./donotimport", Names: []string{"Ignored"}},
			},
		},
	}, fileEmitOptions{
		TypeFile:        typeFile,
		PackageJSONPath: filepath.Join(tempDir, "package.json"),
		IgnoreImportPaths: []string{
			filepath.Join(sourceDir, "donotimport.ts"),
		},
	})

	if len(imports) != 1 {
		t.Fatalf("expected only non-ignored import to remain, got %+v", imports)
	}
	if imports[0].Path != "../src/types" {
		t.Fatalf("expected kept import path %q, got %+v", "../src/types", imports)
	}
	if len(imports[0].Names) != 1 || imports[0].Names[0] != "Kept" {
		t.Fatalf("expected kept import names, got %+v", imports)
	}
}

func TestRunTypegenRoundsPreservesReducedWriteRoundTypes(t *testing.T) {
	root := repoRoot(t)
	tempDir := t.TempDir()
	copyDir(t, filepath.Join(root, "samples"), tempDir)

	_, err := runTypegenRounds(context.Background(), AppOptions{
		BinaryPath:      tsgoapi.PreferredBinary(root),
		TsConfigPath:    filepath.Join(tempDir, "tsconfig.json"),
		PackageJSONPath: filepath.Join(root, "package.json"),
		RootPath:        tempDir,
		TypesPath:       tempDir,
		Write:           true,
		Delete:          true,
		Quiet:           true,
		Log:             func(string) {},
		Timeout:         15 * time.Second,
	})
	if err != nil {
		t.Fatalf("runTypegenRounds returned error: %v", err)
	}

	autoImportTypegen := mustReadFile(t, filepath.Join(tempDir, "autoImportLogicType.ts"))
	for _, expected := range []string{
		"import type { A1, A2, A3, A4, A5, A7, D1, D3, D6, EventIndex, ExportedApi, R1, R6, RandomThing, S6, S7 } from './autoImportTypes'",
		"eventIndex: (state: any, props?: any) => EventIndex",
		"randomDetectedReturn: (state: any, props?: any) => RandomThing",
		"randomInterfacedReturn: (state: any, props?: any) => RandomAPI",
		"randomSpecifiedReturn: (state: any, props?: any) => ExportedApi.RandomThing",
		"sbla: (state: any, props?: any) => Partial<Record<string, S7>>",
		"sbla: Partial<Record<string, S7>>",
		"__keaTypeGenInternalSelectorTypes: {",
		"sbla: (arg: S6) => Partial<Record<string, S7>>",
	} {
		if !strings.Contains(autoImportTypegen, expected) {
			t.Fatalf("expected reduced write round output to contain %q:\n%s", expected, autoImportTypegen)
		}
	}

	logicTypegen := mustReadFile(t, filepath.Join(tempDir, "logicType.ts"))
	for _, expected := range []string{
		"capitalizedName: (state: any, props?: any) => string",
		"upperCaseName: (state: any, props?: any) => string",
		"capitalizedName: string",
		"upperCaseName: string",
		"someRandomFunction: (payload: { name: string; id?: number; }, breakpoint: BreakPointFunction, action: { type: string; payload: { name: string; id?: number; } }, previousState: any) => void | Promise<void>",
		"__keaTypeGenInternalSelectorTypes: {",
		"capitalizedName: (name: string, number: number) => string",
		"upperCaseName: (capitalizedName: string) => string",
		"randomSelector: (capitalizedName: string) => Record<string, any>",
		"longSelector: (name: string, number: number, capitalizedName: string, upperCaseName: string, randomSelector: Record<string, any>, randomSelector2: Record<string, any>) => false",
		"__keaTypeGenInternalReducerActions: {",
		"'set username (githubLogic)': (username: string) => {",
	} {
		if !strings.Contains(logicTypegen, expected) {
			t.Fatalf("expected reduced write round output to contain %q:\n%s", expected, logicTypegen)
		}
	}

	complexTypegen := mustReadFile(t, filepath.Join(tempDir, "complexLogicType.ts"))
	for _, expected := range []string{
		"selectedActionId: number | 'new' | null",
		"selectedActionId: (state: any, props?: any) => number | 'new' | null",
		"hideButtonActions: ((action: { type: 'hide button actions (complexLogic)'; payload: { value: true } }, previousState: any) => void | Promise<void>)[]",
		"selectAction: (id: string | null) => {",
		"payload: { id: string; }",
		"inspectForElementWithIndex: (index: number | null) => {",
		"payload: { index: number; }",
		"newAction: (element?: HTMLElement) => {",
		"payload: { element: HTMLElement; }",
		"updateDashboardInsight: (id: number, payload: DashboardItemType) => {",
		"key39: string",
		"__keaTypeGenInternalSelectorTypes: {",
		"selectedAction: (selectedActionId: number | 'new', newActionForElement: HTMLElement) => ActionType | null",
		"initialValuesForForm: (selectedAction: ActionType) => ActionForm",
		"selectedEditedAction: (selectedAction: ActionType, initialValuesForForm: ActionForm, form: FormInstance, editingFields: AntdFieldData[], inspectingElement: number, counter: number) => ActionForm",
	} {
		if !strings.Contains(complexTypegen, expected) {
			t.Fatalf("expected reduced write round output to contain %q:\n%s", expected, complexTypegen)
		}
	}
	if strings.Contains(complexTypegen, "selectedActionId: string") {
		t.Fatalf("expected reduced write round output to avoid widened selectedActionId: string:\n%s", complexTypegen)
	}
	if strings.Contains(complexTypegen, "... 24 more ...") {
		t.Fatalf("expected reduced write round output to keep complex selector object types fully expanded:\n%s", complexTypegen)
	}

	loadersTypegen := mustReadFile(t, filepath.Join(tempDir, "loadersLogicType.ts"))
	for _, expected := range []string{
		"loadItSuccess: (misc: { id: number; name: void; pinned: boolean; }, payload?: any) => void",
		"payload: { misc: { id: number; name: void; pinned: boolean; }; payload?: any }",
	} {
		if !strings.Contains(loadersTypegen, expected) {
			t.Fatalf("expected reduced write round output to contain %q:\n%s", expected, loadersTypegen)
		}
	}

	routerConnectTypegen := mustReadFile(t, filepath.Join(tempDir, "routerConnectLogicType.ts"))
	for _, expected := range []string{
		"import type { LocationChangedPayload } from 'kea-router/lib/types'",
		"locationChanged: ({ method, pathname, search, searchParams, hash, hashParams, initial, }: LocationChangedPayload) => void",
		"hashParams: Record<string, any>",
		"searchParams: Record<string, any>",
	} {
		if !strings.Contains(routerConnectTypegen, expected) {
			t.Fatalf("expected clean write output to contain %q:\n%s", expected, routerConnectTypegen)
		}
	}
	if strings.Contains(routerConnectTypegen, "[x: string]: any") {
		t.Fatalf("expected clean write output to avoid generic router payload fallback:\n%s", routerConnectTypegen)
	}

	githubNamespaceConnectTypegen := mustReadFile(t, filepath.Join(tempDir, "githubNamespaceConnectLogicType.ts"))
	for _, expected := range []string{
		"__keaTypeGenInternalReducerActions: {",
		"'set repositories (githubLogic)': (repositories: Repository[]) => {",
		"repositories: Repository[]",
	} {
		if !strings.Contains(githubNamespaceConnectTypegen, expected) {
			t.Fatalf("expected clean write output to contain %q:\n%s", expected, githubNamespaceConnectTypegen)
		}
	}

	pluginTypegen := mustReadFile(t, filepath.Join(tempDir, "pluginLogicType.ts"))
	for _, expected := range []string{
		"submitForm: () => void",
		"form: { name: string; age: number; }",
		"__keaTypeGenInternalExtraInput: {",
		"default?: Record<string, any>",
		"submit?: (form: { name: string; age: number; }) => void",
	} {
		if !strings.Contains(pluginTypegen, expected) {
			t.Fatalf("expected clean write output to contain %q:\n%s", expected, pluginTypegen)
		}
	}
	for _, unexpected := range []string{
		"inlineAction: () => void",
		"inlineReducer: { asd: boolean }",
	} {
		if strings.Contains(pluginTypegen, unexpected) {
			t.Fatalf("expected clean write output to omit %q:\n%s", unexpected, pluginTypegen)
		}
	}

	typedFormTypegen := mustReadFile(t, filepath.Join(tempDir, "typed-builder", "typedFormDemoLogicType.ts"))
	for _, unexpected := range []string{
		"submitForm: () => void",
		"form: Record<string, any>",
		"__keaTypeGenInternalExtraInput: {",
	} {
		if strings.Contains(typedFormTypegen, unexpected) {
			t.Fatalf("expected clean write output to defer typedForm builder heuristics and omit %q:\n%s", unexpected, typedFormTypegen)
		}
	}
}

func TestRunTypegenRoundsRewritesStaleLogicTypeImports(t *testing.T) {
	root := repoRoot(t)
	tempDir := t.TempDir()
	copyDir(t, filepath.Join(root, "samples"), tempDir)
	sourcePath := filepath.Join(tempDir, "githubNamespaceConnectLogic.ts")
	source := mustReadFile(t, sourcePath)
	source = strings.Replace(
		source,
		"import type { githubNamespaceConnectLogicType } from './githubNamespaceConnectLogicType'",
		"import type { githubNamespaceConnectLogicType } from '../../../../../../tmp/kea-ts-types/githubNamespaceConnectLogicType'",
		1,
	)
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("os.WriteFile githubNamespaceConnectLogic.ts: %v", err)
	}

	_, err := runTypegenRounds(context.Background(), AppOptions{
		BinaryPath:      tsgoapi.PreferredBinary(root),
		TsConfigPath:    filepath.Join(tempDir, "tsconfig.json"),
		PackageJSONPath: filepath.Join(root, "package.json"),
		RootPath:        tempDir,
		TypesPath:       tempDir,
		Write:           true,
		Delete:          true,
		Quiet:           true,
		Log:             func(string) {},
		Timeout:         15 * time.Second,
	})
	if err != nil {
		t.Fatalf("runTypegenRounds returned error: %v", err)
	}

	updated := mustReadFile(t, filepath.Join(tempDir, "githubNamespaceConnectLogic.ts"))
	if strings.Contains(updated, "../../../../../../tmp/kea-ts-types/githubNamespaceConnectLogicType") {
		t.Fatalf("expected stale githubNamespaceConnectLogicType import to be rewritten:\n%s", updated)
	}
	if strings.Count(updated, "import type { githubNamespaceConnectLogicType }") != 1 {
		t.Fatalf("expected exactly one githubNamespaceConnectLogicType import after write:\n%s", updated)
	}
	if !strings.Contains(updated, "import type { githubNamespaceConnectLogicType } from './githubNamespaceConnectLogicType'") {
		t.Fatalf("expected local githubNamespaceConnectLogicType import after write:\n%s", updated)
	}
}

func TestPlanSourceEditsAddsTypeImportAndGeneric(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"const propsLogic = kea({",
		"    actions: {},",
		"})",
		"",
	}, "\n")
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	file := parsedFile{
		File:         "/tmp/propsLogic.ts",
		TypeFile:     "/tmp/propsLogicType.ts",
		Source:       source,
		SourceLogics: sourceLogics,
		Logics: []ParsedLogic{
			{Name: "propsLogic", TypeName: "propsLogicType", Path: []string{"propsLogic"}},
		},
	}

	plan, err := planSourceEdits(file, AppOptions{})
	if err != nil {
		t.Fatalf("planSourceEdits returned error: %v", err)
	}
	if plan.ImportCount != 1 {
		t.Fatalf("expected 1 import edit count, got %d", plan.ImportCount)
	}

	updated, err := applySourceEditPlan(plan)
	if err != nil {
		t.Fatalf("applySourceEditPlan returned error: %v", err)
	}

	expectedPrefix := "import { kea } from 'kea'\nimport type { propsLogicType } from './propsLogicType'\n\nconst propsLogic = kea<propsLogicType>({"
	if !strings.Contains(updated, expectedPrefix) {
		t.Fatalf("updated source missing expected import/generic:\n%s", updated)
	}
}

func TestPlanSourceEditsRewritesStaleTypeImportPath(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"import type { githubNamespaceConnectLogicType } from '../../../../../../tmp/kea-ts-types/githubNamespaceConnectLogicType'",
		"",
		"export const githubNamespaceConnectLogic = kea<githubNamespaceConnectLogicType>({",
		"    actions: {},",
		"})",
		"",
	}, "\n")
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	file := parsedFile{
		File:         "/tmp/samples/githubNamespaceConnectLogic.ts",
		TypeFile:     "/tmp/samples/githubNamespaceConnectLogicType.ts",
		Source:       source,
		SourceLogics: sourceLogics,
		Logics: []ParsedLogic{
			{Name: "githubNamespaceConnectLogic", TypeName: "githubNamespaceConnectLogicType", Path: []string{"githubNamespaceConnectLogic"}},
		},
	}

	plan, err := planSourceEdits(file, AppOptions{})
	if err != nil {
		t.Fatalf("planSourceEdits returned error: %v", err)
	}
	if plan.ImportCount != 1 {
		t.Fatalf("expected 1 import edit count, got %d", plan.ImportCount)
	}

	updated, err := applySourceEditPlan(plan)
	if err != nil {
		t.Fatalf("applySourceEditPlan returned error: %v", err)
	}

	if strings.Contains(updated, "../../../../../../tmp/kea-ts-types/githubNamespaceConnectLogicType") {
		t.Fatalf("expected stale type import path to be removed:\n%s", updated)
	}
	if strings.Count(updated, "import type { githubNamespaceConnectLogicType }") != 1 {
		t.Fatalf("expected exactly one logic type import after rewrite:\n%s", updated)
	}
	if !strings.Contains(updated, "import type { githubNamespaceConnectLogicType } from './githubNamespaceConnectLogicType'") {
		t.Fatalf("expected rewritten local type import:\n%s", updated)
	}
}

func TestPlanSourceEditsDeduplicatesLogicTypeImports(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"import type { githubNamespaceConnectLogicType } from '../../../../../../tmp/kea-ts-types/githubNamespaceConnectLogicType'",
		"import type { githubNamespaceConnectLogicType } from './githubNamespaceConnectLogicType'",
		"",
		"export const githubNamespaceConnectLogic = kea<githubNamespaceConnectLogicType>({",
		"    actions: {},",
		"})",
		"",
	}, "\n")
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	file := parsedFile{
		File:         "/tmp/samples/githubNamespaceConnectLogic.ts",
		TypeFile:     "/tmp/samples/githubNamespaceConnectLogicType.ts",
		Source:       source,
		SourceLogics: sourceLogics,
		Logics: []ParsedLogic{
			{Name: "githubNamespaceConnectLogic", TypeName: "githubNamespaceConnectLogicType", Path: []string{"githubNamespaceConnectLogic"}},
		},
	}

	plan, err := planSourceEdits(file, AppOptions{})
	if err != nil {
		t.Fatalf("planSourceEdits returned error: %v", err)
	}
	if plan.ImportCount != 1 {
		t.Fatalf("expected 1 import edit count for duplicate cleanup, got %d", plan.ImportCount)
	}

	updated, err := applySourceEditPlan(plan)
	if err != nil {
		t.Fatalf("applySourceEditPlan returned error: %v", err)
	}

	if strings.Count(updated, "import type { githubNamespaceConnectLogicType }") != 1 {
		t.Fatalf("expected duplicate logic type imports to collapse to one line:\n%s", updated)
	}
	if strings.Contains(updated, "../../../../../../tmp/kea-ts-types/githubNamespaceConnectLogicType") {
		t.Fatalf("expected stale duplicate type import to be removed:\n%s", updated)
	}
}

func copyDir(t *testing.T, sourceDir, targetDir string) {
	t.Helper()

	err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, relative)

		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(targetPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode())
	})
	if err != nil {
		t.Fatalf("copyDir(%s, %s) returned error: %v", sourceDir, targetDir, err)
	}
}

func TestPlanSourceEditsAddsObjectPath(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"import type { propsLogicType } from './propsLogicType'",
		"",
		"const propsLogic = kea<propsLogicType>({",
		"    actions: {},",
		"})",
		"",
	}, "\n")
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	file := parsedFile{
		File:         "/tmp/propsLogic.ts",
		TypeFile:     "/tmp/propsLogicType.ts",
		Source:       source,
		SourceLogics: sourceLogics,
		Logics: []ParsedLogic{
			{Name: "propsLogic", TypeName: "propsLogicType", Path: []string{"propsLogic"}},
		},
	}

	plan, err := planSourceEdits(file, AppOptions{WritePaths: true})
	if err != nil {
		t.Fatalf("planSourceEdits returned error: %v", err)
	}
	if plan.PathCount != 1 {
		t.Fatalf("expected 1 path edit count, got %d", plan.PathCount)
	}

	updated, err := applySourceEditPlan(plan)
	if err != nil {
		t.Fatalf("applySourceEditPlan returned error: %v", err)
	}

	if !strings.Contains(updated, "const propsLogic = kea<propsLogicType>({\n    path: ['propsLogic'],\n    actions: {},") {
		t.Fatalf("updated source missing inserted path:\n%s", updated)
	}
}

func TestConvertFileToBuildersRewritesObjectLogicAndAddsImports(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"export const demoLogic = kea({",
		"    path: ['demoLogic'],",
		"    actions: {",
		"        setName: (name: string) => ({ name }),",
		"    },",
		"    loaders: ({}) => ({",
		"        name: [",
		"            '' as string,",
		"            { loadName: async () => 'test' },",
		"        ],",
		"    }),",
		"    mystery: createMystery(),",
		"})",
		"",
	}, "\n")
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	file := parsedFile{
		File:         "/tmp/demoLogic.ts",
		Source:       source,
		SourceLogics: sourceLogics,
		Logics: []ParsedLogic{
			{Name: "demoLogic", Path: []string{"demoLogic"}},
		},
	}

	updated, converted, warnings, err := convertFileToBuilders(source, file, AppOptions{})
	if err != nil {
		t.Fatalf("convertFileToBuilders returned error: %v", err)
	}
	if converted != 1 {
		t.Fatalf("expected 1 converted logic, got %d", converted)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "mystery") {
		t.Fatalf("expected unsupported mystery warning, got %#v", warnings)
	}
	for _, expected := range []string{
		"import { kea, actions, path } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"export const demoLogic = kea([",
		"path(['demoLogic'])",
		"actions({",
		"loaders(({}) => ({",
		"mystery(createMystery())",
		"])",
	} {
		if !strings.Contains(updated, expected) {
			t.Fatalf("updated source missing %q:\n%s", expected, updated)
		}
	}
}

func TestConvertFileToBuildersReusesExistingPathAliasAndAddsMissingPathBuilder(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path as logicPath } from 'kea'",
		"",
		"export const aliasedLogic = kea({",
		"    actions: {},",
		"})",
		"",
	}, "\n")
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	file := parsedFile{
		File:         "/tmp/aliasedLogic.ts",
		Source:       source,
		SourceLogics: sourceLogics,
		Logics: []ParsedLogic{
			{Name: "aliasedLogic", Path: []string{"aliasedLogic"}},
		},
	}

	updated, converted, warnings, err := convertFileToBuilders(source, file, AppOptions{WritePaths: true})
	if err != nil {
		t.Fatalf("convertFileToBuilders returned error: %v", err)
	}
	if converted != 1 {
		t.Fatalf("expected 1 converted logic, got %d", converted)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
	for _, expected := range []string{
		"import { kea, path as logicPath, actions } from 'kea'",
		"export const aliasedLogic = kea([",
		"logicPath(['aliasedLogic'])",
		"actions({})",
		"])",
	} {
		if !strings.Contains(updated, expected) {
			t.Fatalf("updated source missing %q:\n%s", expected, updated)
		}
	}
	if strings.Contains(updated, "path(['aliasedLogic'])") {
		t.Fatalf("expected path alias reuse instead of raw path import:\n%s", updated)
	}
}

func TestConvertFileToBuildersMergesIntoMixedAndDefaultImports(t *testing.T) {
	source := strings.Join([]string{
		"import KeaRuntime, { kea } from 'kea'",
		"import loadersPlugin from 'kea-loaders'",
		"",
		"export const demoLogic = kea({",
		"    actions: {},",
		"    loaders: ({}) => ({",
		"        name: ['', { loadName: async () => 'test' }],",
		"    }),",
		"})",
		"",
	}, "\n")
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	file := parsedFile{
		File:         "/tmp/demoLogic.ts",
		Source:       source,
		SourceLogics: sourceLogics,
		Logics: []ParsedLogic{
			{Name: "demoLogic", Path: []string{"demoLogic"}},
		},
	}

	updated, converted, warnings, err := convertFileToBuilders(source, file, AppOptions{})
	if err != nil {
		t.Fatalf("convertFileToBuilders returned error: %v", err)
	}
	if converted != 1 {
		t.Fatalf("expected 1 converted logic, got %d", converted)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
	for _, expected := range []string{
		"import KeaRuntime, { kea, actions } from 'kea'",
		"import loadersPlugin, { loaders } from 'kea-loaders'",
		"export const demoLogic = kea([",
	} {
		if !strings.Contains(updated, expected) {
			t.Fatalf("updated source missing %q:\n%s", expected, updated)
		}
	}
	if strings.Contains(updated, "import { loaders } from 'kea-loaders'") {
		t.Fatalf("expected loaders import to merge into default import instead of creating a second import:\n%s", updated)
	}
}

func TestConvertFileToBuildersHandlesQuotedAndComputedKeys(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"export const quotedLogic = kea({",
		"    'path': ['quotedLogic'],",
		"    [\"actions\"]: {},",
		"    [`reducers`]: {",
		"        count: [0, {}],",
		"    },",
		"})",
		"",
	}, "\n")
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	file := parsedFile{
		File:         "/tmp/quotedLogic.ts",
		Source:       source,
		SourceLogics: sourceLogics,
		Logics: []ParsedLogic{
			{Name: "quotedLogic", Path: []string{"quotedLogic"}},
		},
	}

	updated, converted, warnings, err := convertFileToBuilders(source, file, AppOptions{})
	if err != nil {
		t.Fatalf("convertFileToBuilders returned error: %v", err)
	}
	if converted != 1 {
		t.Fatalf("expected 1 converted logic, got %d", converted)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
	for _, expected := range []string{
		"import { kea, actions, path, reducers } from 'kea'",
		"export const quotedLogic = kea([",
		"path(['quotedLogic'])",
		"actions({})",
		"reducers({",
	} {
		if !strings.Contains(updated, expected) {
			t.Fatalf("updated source missing %q:\n%s", expected, updated)
		}
	}
}

func TestConvertFileToBuildersReusesNamespaceBuilderImports(t *testing.T) {
	source := strings.Join([]string{
		"import * as keaBuilders from 'kea'",
		"import * as loaderBuilders from 'kea-loaders'",
		"",
		"export const demoLogic = keaBuilders.kea({",
		"    actions: {},",
		"    loaders: ({}) => ({",
		"        name: ['', { loadName: async () => 'test' }],",
		"    }),",
		"})",
		"",
	}, "\n")
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	file := parsedFile{
		File:         "/tmp/demoLogic.ts",
		Source:       source,
		SourceLogics: sourceLogics,
		Logics: []ParsedLogic{
			{Name: "demoLogic", Path: []string{"demoLogic"}},
		},
	}

	updated, converted, warnings, err := convertFileToBuilders(source, file, AppOptions{})
	if err != nil {
		t.Fatalf("convertFileToBuilders returned error: %v", err)
	}
	if converted != 1 {
		t.Fatalf("expected 1 converted logic, got %d", converted)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
	for _, expected := range []string{
		"import * as keaBuilders from 'kea'",
		"import * as loaderBuilders from 'kea-loaders'",
		"export const demoLogic = keaBuilders.kea([",
		"keaBuilders.actions({})",
		"loaderBuilders.loaders(({}) => ({",
	} {
		if !strings.Contains(updated, expected) {
			t.Fatalf("updated source missing %q:\n%s", expected, updated)
		}
	}
	if strings.Contains(updated, "import { actions } from 'kea'") || strings.Contains(updated, "import { loaders } from 'kea-loaders'") {
		t.Fatalf("expected namespace builder imports to be reused without extra named imports:\n%s", updated)
	}
}

func TestPlanSourceEditsAddsBuilderPathIntoMixedKeaImport(t *testing.T) {
	source := strings.Join([]string{
		"import KeaRuntime, { kea, actions } from 'kea'",
		"import type { builderLogicType } from './builderLogicType'",
		"",
		"export const builderLogic = kea<builderLogicType>([",
		"    actions({}),",
		"])",
		"",
	}, "\n")
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	file := parsedFile{
		File:         "/tmp/builderLogic.ts",
		TypeFile:     "/tmp/builderLogicType.ts",
		Source:       source,
		SourceLogics: sourceLogics,
		Logics: []ParsedLogic{
			{Name: "builderLogic", TypeName: "builderLogicType", Path: []string{"builderLogic"}},
		},
	}

	plan, err := planSourceEdits(file, AppOptions{WritePaths: true})
	if err != nil {
		t.Fatalf("planSourceEdits returned error: %v", err)
	}
	if plan.PathCount != 1 {
		t.Fatalf("expected 1 path edit count, got %d", plan.PathCount)
	}

	updated, err := applySourceEditPlan(plan)
	if err != nil {
		t.Fatalf("applySourceEditPlan returned error: %v", err)
	}

	for _, expected := range []string{
		"import KeaRuntime, { kea, actions, path } from 'kea'",
		"kea<builderLogicType>([\n    path(['builderLogic']),\n    actions({}),",
	} {
		if !strings.Contains(updated, expected) {
			t.Fatalf("updated source missing %q:\n%s", expected, updated)
		}
	}
}

func TestPlanSourceEditsAddsBuilderPathIntoNamespaceKeaImport(t *testing.T) {
	source := strings.Join([]string{
		"import * as keaBuilders from 'kea'",
		"import type { builderLogicType } from './builderLogicType'",
		"",
		"export const builderLogic = keaBuilders.kea<builderLogicType>([",
		"    keaBuilders.actions({}),",
		"])",
		"",
	}, "\n")
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	file := parsedFile{
		File:         "/tmp/builderLogic.ts",
		TypeFile:     "/tmp/builderLogicType.ts",
		Source:       source,
		SourceLogics: sourceLogics,
		Logics: []ParsedLogic{
			{Name: "builderLogic", TypeName: "builderLogicType", Path: []string{"builderLogic"}},
		},
	}

	plan, err := planSourceEdits(file, AppOptions{WritePaths: true})
	if err != nil {
		t.Fatalf("planSourceEdits returned error: %v", err)
	}
	if plan.PathCount != 1 {
		t.Fatalf("expected 1 path edit count, got %d", plan.PathCount)
	}

	updated, err := applySourceEditPlan(plan)
	if err != nil {
		t.Fatalf("applySourceEditPlan returned error: %v", err)
	}
	if !strings.Contains(updated, "keaBuilders.kea<builderLogicType>([\n    keaBuilders.path(['builderLogic']),\n    keaBuilders.actions({}),") {
		t.Fatalf("updated source missing namespace-qualified path insertion:\n%s", updated)
	}
	if strings.Contains(updated, "import { path } from 'kea'") {
		t.Fatalf("expected namespace import reuse instead of new named path import:\n%s", updated)
	}
}

func TestPlanSourceEditsSkipsAliasedExistingBuilderPath(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path as logicPath, actions } from 'kea'",
		"import type { builderLogicType } from './builderLogicType'",
		"",
		"export const builderLogic = kea<builderLogicType>([",
		"    logicPath(['builderLogic']),",
		"    actions({}),",
		"])",
		"",
	}, "\n")
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	file := parsedFile{
		File:         "/tmp/builderLogic.ts",
		TypeFile:     "/tmp/builderLogicType.ts",
		Source:       source,
		SourceLogics: sourceLogics,
		Logics: []ParsedLogic{
			{Name: "builderLogic", TypeName: "builderLogicType", Path: []string{"builderLogic"}},
		},
	}

	plan, err := planSourceEdits(file, AppOptions{WritePaths: true})
	if err != nil {
		t.Fatalf("planSourceEdits returned error: %v", err)
	}
	if plan.PathCount != 0 {
		t.Fatalf("expected no new path edits, got %d", plan.PathCount)
	}

	updated, err := applySourceEditPlan(plan)
	if err != nil {
		t.Fatalf("applySourceEditPlan returned error: %v", err)
	}
	if updated != source {
		t.Fatalf("expected source to stay unchanged:\n%s", updated)
	}
}

func TestCachePathStaysUnderTypegen(t *testing.T) {
	options := AppOptions{WorkingDir: "/tmp/project"}
	got := cachePath(options, "/tmp/project/src/logicType.ts")
	want := "/tmp/project/.typegen/src/logicType.ts"
	if got != want {
		t.Fatalf("unexpected cachePath: %s", got)
	}
}
