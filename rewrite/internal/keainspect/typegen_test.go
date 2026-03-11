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

func TestCachePathStaysUnderTypegen(t *testing.T) {
	options := AppOptions{WorkingDir: "/tmp/project"}
	got := cachePath(options, "/tmp/project/src/logicType.ts")
	want := "/tmp/project/.typegen/src/logicType.ts"
	if got != want {
		t.Fatalf("unexpected cachePath: %s", got)
	}
}
