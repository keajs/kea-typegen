package keainspect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
