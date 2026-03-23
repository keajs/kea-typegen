package keainspect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kea-typegen/rewrite/internal/tsgoapi"
)

func writeTempProject(t *testing.T, files map[string]string) (string, string) {
	t.Helper()

	tempDir := t.TempDir()
	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["src/**/*"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	for relativePath, contents := range files {
		fullPath := filepath.Join(tempDir, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("MkdirAll returned error: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
	}
	return tempDir, tsconfigPath
}

func inspectTempLogicFile(t *testing.T, files map[string]string, targetRelative string) []ParsedLogic {
	t.Helper()

	tempDir, tsconfigPath := writeTempProject(t, files)

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       filepath.Join(tempDir, filepath.FromSlash(targetRelative)),
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	return logics
}

func TestCollectTypeImportsForTypeTextsRecoversMissingPackageImportsFromSource(t *testing.T) {
	source := strings.Join([]string{
		"import type { FrameType } from './types'",
		"import type { DeepPartial } from 'kea-forms'",
	}, "\n")

	imports := collectTypeImportsForTypeTexts(
		source,
		"/tmp/scheduleLogic.ts",
		[]string{"(values: DeepPartial<FrameType>)", "{ values: DeepPartial<FrameType>; }"},
		nil,
		existingImportReferenceNames([]TypeImport{{Path: "./types", Names: []string{"FrameType"}}}),
	)

	if !hasImport(imports, "kea-forms", "DeepPartial") {
		t.Fatalf("expected missing package import DeepPartial from kea-forms, got %+v", imports)
	}
	if hasImport(imports, "./types", "FrameType") {
		t.Fatalf("expected already-covered FrameType import to stay skipped, got %+v", imports)
	}
}

func TestCollectTypeImportsForTypeTextsRecoversPackageSiblingExportsFromValueImports(t *testing.T) {
	root := repoRoot(t)
	source := strings.Join([]string{
		"import type { FrameType } from './types'",
		"import { forms } from 'kea-forms'",
	}, "\n")

	imports := collectTypeImportsForTypeTexts(
		source,
		filepath.Join(root, "samples", "scheduleLogic.ts"),
		[]string{"(values: DeepPartial<FrameType>)", "{ values: DeepPartial<FrameType>; }"},
		nil,
		existingImportReferenceNames([]TypeImport{{Path: "./types", Names: []string{"FrameType"}}}),
	)

	if !hasImport(imports, "kea-forms", "DeepPartial") &&
		!hasImport(imports, "kea-forms/lib/index", "DeepPartial") &&
		!hasImport(imports, "kea-forms/lib/types", "DeepPartial") {
		t.Fatalf("expected package sibling DeepPartial import from kea-forms value import, got %+v", imports)
	}
}

func TestBuildParsedLogicsFromSourcePropsLogic(t *testing.T) {
	root := repoRoot(t)
	sourcePath := filepath.Join(root, "samples", "propsLogic.ts")
	source := mustReadFile(t, sourcePath)

	report := &Report{
		ProjectDir: filepath.Join(root, "samples"),
		File:       sourcePath,
		Logics: []LogicReport{
			{
				Name:      "propsLogic",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name:                "props",
						EffectiveTypeString: "{ page: string; id: number; }",
						PrintedTypeNode:     "{\n    page: string;\n    id: number;\n}",
					},
					{
						Name:                "key",
						EffectiveTypeString: "number",
						PrintedTypeNode:     "number",
					},
					{
						Name: "actions",
						Members: []MemberReport{
							{Name: "setId", TypeString: "(id: number) => { id: number; }", ReturnTypeString: "{ id: number; }"},
							{Name: "setPage", TypeString: "(page: string) => { page: string; }", ReturnTypeString: "{ page: string; }"},
						},
					},
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "currentPage", TypeString: "[string, { setPage: (_: string, { page }: { page: string; }) => string; }]"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if logic.PathString != "propsLogic.*" {
		t.Fatalf("expected path string %q, got %q", "propsLogic.*", logic.PathString)
	}
	if logic.KeyType != "number" {
		t.Fatalf("expected key type %q, got %q", "number", logic.KeyType)
	}
	if len(logic.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(logic.Actions))
	}
	if len(logic.Reducers) != 1 || logic.Reducers[0].Type != "string" {
		t.Fatalf("expected string reducer, got %+v", logic.Reducers)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	expected := `// Generated by kea-typegen on Wed, 11 Mar 2026 12:00:00 UTC. DO NOT EDIT THIS FILE MANUALLY.

import type { Logic } from 'kea'

export interface propsLogicType extends Logic {
    actionCreators: {
        setId: (id: number) => {
            type: 'set id (propsLogic.*)'
            payload: { id: number; }
        }
        setPage: (page: string) => {
            type: 'set page (propsLogic.*)'
            payload: { page: string; }
        }
    }
    actionKeys: {
        'set id (propsLogic.*)': 'setId'
        'set page (propsLogic.*)': 'setPage'
    }
    actionTypes: {
        setId: 'set id (propsLogic.*)'
        setPage: 'set page (propsLogic.*)'
    }
    actions: {
        setId: (id: number) => void
        setPage: (page: string) => void
    }
    asyncActions: {
        setId: (id: number) => Promise<any>
        setPage: (page: string) => Promise<any>
    }
    defaults: {
        currentPage: string
    }
    events: {}
    key: number
    listeners: {}
    path: ['propsLogic', '*']
    pathString: 'propsLogic.*'
    props: {
        page: string;
        id: number;
    }
    reducer: (
        state: any,
        action: any,
        fullState: any,
    ) => {
        currentPage: string
    }
    reducers: {
        currentPage: (state: string, action: any, fullState: any) => string
    }
    selector: (
        state: any,
    ) => {
        currentPage: string
    }
    selectors: {
        currentPage: (state: any, props?: any) => string
    }
    sharedListeners: {}
    values: {
        currentPage: string
    }
    _isKea: true
    _isKeaWithKey: true
}
`
	if rendered != expected {
		t.Fatalf("unexpected emitted typegen:\n%s", rendered)
	}
}

func TestBuildParsedLogicsInternalSelectorHelpersMatchTupleShapedSelectorInputs(t *testing.T) {
	tests := []struct {
		name            string
		source          string
		report          *Report
		expectedHelpers []string
	}{
		{
			name: "skips array-collapsed selector inputs",
			source: strings.Join([]string{
				"import { kea, reducers, selectors } from 'kea'",
				"",
				"type EnrichedEarlyAccessFeature = { id: string }",
				"",
				"export const featurePreviewsLogic = kea([",
				"    reducers({",
				"        rawEarlyAccessFeatures: [[] as EnrichedEarlyAccessFeature[], {}],",
				"        searchTerm: ['', {}],",
				"    }),",
				"    selectors({",
				"        earlyAccessFeatures: [",
				"            (s) => [s.rawEarlyAccessFeatures],",
				"            (rawEarlyAccessFeatures): EnrichedEarlyAccessFeature[] => rawEarlyAccessFeatures,",
				"        ],",
				"        filteredEarlyAccessFeatures: [",
				"            (s) => [s.earlyAccessFeatures, s.searchTerm],",
				"            (earlyAccessFeatures: EnrichedEarlyAccessFeature[], searchTerm: string): EnrichedEarlyAccessFeature[] => {",
				"                return earlyAccessFeatures.filter((feature) => feature.id.includes(searchTerm))",
				"            },",
				"        ],",
				"    }),",
				"])",
			}, "\n"),
			report: &Report{
				ProjectDir: "/tmp",
				File:       "/tmp/featurePreviewsLogic.ts",
				Logics: []LogicReport{
					{
						Name:      "featurePreviewsLogic",
						InputKind: "builders",
						Sections: []SectionReport{
							{
								Name: "reducers",
								Members: []MemberReport{
									{Name: "rawEarlyAccessFeatures", TypeString: "[EnrichedEarlyAccessFeature[], {}]"},
									{Name: "searchTerm", TypeString: "[string, {}]"},
								},
							},
							{
								Name: "selectors",
								Members: []MemberReport{
									{
										Name:             "earlyAccessFeatures",
										TypeString:       "(((s: any) => any[]) | ((rawEarlyAccessFeatures: any) => EnrichedEarlyAccessFeature[]))[]",
										ReturnTypeString: "EnrichedEarlyAccessFeature[]",
									},
									{
										Name:             "filteredEarlyAccessFeatures",
										TypeString:       "(((s: any) => any[]) | ((earlyAccessFeatures: EnrichedEarlyAccessFeature[], searchTerm: string) => EnrichedEarlyAccessFeature[]))[]",
										ReturnTypeString: "EnrichedEarlyAccessFeature[]",
									},
								},
							},
						},
					},
				},
			},
			expectedHelpers: nil,
		},
		{
			name: "keeps tuple-shaped selector inputs",
			source: strings.Join([]string{
				"import { kea, reducers, selectors } from 'kea'",
				"",
				"type Repository = { name: string }",
				"",
				"export const variantsPanelLogic = kea([",
				"    reducers({",
				"        repositories: [[] as Repository[], {}],",
				"    }),",
				"    selectors({",
				"        sortedRepositories: [",
				"            (s) => [s.repositories],",
				"            (repositories) => {",
				"                return repositories.slice()",
				"            },",
				"        ],",
				"    }),",
				"])",
			}, "\n"),
			report: &Report{
				ProjectDir: "/tmp",
				File:       "/tmp/variantsPanelLogic.ts",
				Logics: []LogicReport{
					{
						Name:      "variantsPanelLogic",
						InputKind: "builders",
						Sections: []SectionReport{
							{
								Name: "reducers",
								Members: []MemberReport{
									{Name: "repositories", TypeString: "[Repository[], {}]"},
								},
							},
							{
								Name: "selectors",
								Members: []MemberReport{
									{
										Name:             "sortedRepositories",
										TypeString:       "[(selectors: { repositories: (state: any, props?: any) => Repository[]; sortedRepositories: (state: any, props?: any) => Repository[]; }) => [(state: any, props?: any) => Repository[]], (repositories: Repository[]) => Repository[]]",
										ReturnTypeString: "Repository[]",
									},
								},
							},
						},
					},
				},
			},
			expectedHelpers: []string{"sortedRepositories"},
		},
		{
			name: "skips selectors whose reported tuple hints only describe projector returns",
			source: strings.Join([]string{
				"import { kea, reducers, selectors } from 'kea'",
				"",
				"type Dayjs = { value: string }",
				"type TimelineResult = { from: Dayjs; to: Dayjs }",
				"",
				"export const propertiesTimelineLogic = kea([",
				"    reducers({",
				"        result: [null as TimelineResult | null, {}],",
				"        timezone: ['UTC' as string, {}],",
				"    }),",
				"    selectors({",
				"        dateRange: [",
				"            (s) => [s.result, s.timezone],",
				"            (result: TimelineResult | null, timezone: string): [Dayjs, Dayjs] | null => {",
				"                return result ? [result.from, result.to] : null",
				"            },",
				"        ],",
				"    }),",
				"])",
			}, "\n"),
			report: &Report{
				ProjectDir: "/tmp",
				File:       "/tmp/propertiesTimelineLogic.ts",
				Logics: []LogicReport{
					{
						Name:      "propertiesTimelineLogic",
						InputKind: "builders",
						Sections: []SectionReport{
							{
								Name: "reducers",
								Members: []MemberReport{
									{Name: "result", TypeString: "[TimelineResult | null, {}]"},
									{Name: "timezone", TypeString: "[string, {}]"},
								},
							},
							{
								Name: "selectors",
								Members: []MemberReport{
									{
										Name:             "dateRange",
										TypeString:       "(((s: any) => any[]) | ((result: TimelineResult | null, timezone: string) => [Dayjs, Dayjs] | null))[]",
										ReturnTypeString: "Dayjs",
									},
								},
							},
						},
					},
				},
			},
			expectedHelpers: nil,
		},
		{
			name: "keeps truncated tuple-shaped selector inputs during write rounds",
			source: strings.Join([]string{
				"import { kea, reducers, selectors } from 'kea'",
				"",
				"type Repository = { name: string }",
				"",
				"export const builderLogic = kea([",
				"    reducers({",
				"        repositories: [[] as Repository[], {}],",
				"    }),",
				"    selectors({",
				"        sortedRepositories: [",
				"            (selectors) => [selectors.repositories],",
				"            (repositories) => {",
				"                return [...repositories].sort((a, b) => a.name.localeCompare(b.name))",
				"            },",
				"        ],",
				"    }),",
				"])",
			}, "\n"),
			report: &Report{
				ProjectDir: "/tmp",
				File:       "/tmp/builderLogic.ts",
				Logics: []LogicReport{
					{
						Name:      "builderLogic",
						InputKind: "builders",
						Sections: []SectionReport{
							{
								Name: "reducers",
								Members: []MemberReport{
									{Name: "repositories", TypeString: "[Repository[], {}]"},
								},
							},
							{
								Name: "selectors",
								Members: []MemberReport{
									{
										Name:             "sortedRepositories",
										TypeString:       "[(selectors: { repositories: (state: any, props?: any) => Repository[]; username: (state: any, props?: any) => string; lazyValue: (state: any, props?: any) => string; lazyValueLoading: (state: any, props?: any) => boo...",
										ReturnTypeString: "any",
									},
								},
							},
						},
					},
				},
			},
			expectedHelpers: []string{"sortedRepositories"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logics, err := BuildParsedLogicsFromSource(tt.report, tt.source)
			if err != nil {
				t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
			}
			if len(logics) != 1 {
				t.Fatalf("expected 1 parsed logic, got %d", len(logics))
			}

			logic := logics[0]
			if len(logic.InternalSelectorTypes) != len(tt.expectedHelpers) {
				t.Fatalf("expected internal selector helpers %+v, got %+v", tt.expectedHelpers, logic.InternalSelectorTypes)
			}
			for index, expected := range tt.expectedHelpers {
				if logic.InternalSelectorTypes[index].Name != expected {
					t.Fatalf("expected internal selector helper %q, got %+v", expected, logic.InternalSelectorTypes)
				}
			}
		})
	}
}

func TestBuildParsedLogicsSkipsUninformativeBuilderInternalSelectorHelpers(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, props, selectors } from 'kea'",
		"",
		"type ExportedData = { exportToken: string }",
		"",
		"export const exporterViewLogic = kea([",
		"    props({} as ExportedData),",
		"    selectors({",
		"        exportedData: [",
		"            () => [(_, props: ExportedData) => props],",
		"            (props: ExportedData) => props,",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/exporterViewLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "exporterViewLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "props",
						Members: []MemberReport{
							{Name: "props", TypeString: "any"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "exportedData",
								TypeString:       "((() => ((_: any, props?: any) => any)[]) | ((props: any) => any))[]",
								ReturnTypeString: "any",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if len(logic.InternalSelectorTypes) != 0 {
		t.Fatalf("expected no internal selector helpers, got %+v", logic.InternalSelectorTypes)
	}
}

func TestBuildParsedLogicsInternalSelectorHelpersPreferReportedNullableMemberTypes(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, reducers, selectors } from 'kea'",
		"",
		"type DiagramNode = { id: string; data?: { config?: Record<string, any> } }",
		"",
		"export const appNodeLogic = kea([",
		"    reducers({",
		"        nodes: [[] as DiagramNode[], {}],",
		"        nodeId: [null as string | null, {}],",
		"    }),",
		"    selectors({",
		"        node: [",
		"            (s) => [s.nodes, s.nodeId],",
		"            (nodes, nodeId) => nodes.find((node) => node.id === nodeId) ?? null,",
		"        ],",
		"        nodeConfig: [",
		"            (s) => [s.node],",
		"            (node): Record<string, any> => node?.data?.config ?? {},",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/appNodeLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "appNodeLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "nodes", TypeString: "[DiagramNode[], {}]"},
							{Name: "nodeId", TypeString: "[string | null, {}]"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "node",
								TypeString:       "[(selectors: { nodes: (state: any, props?: any) => DiagramNode[]; nodeId: (state: any, props?: any) => string | null; }) => [(state: any, props?: any) => DiagramNode[], (state: any, props?: any) => string | null], (nodes: DiagramNode[], nodeId: string | null) => DiagramNode | null]",
								ReturnTypeString: "null",
							},
							{
								Name:             "nodeConfig",
								TypeString:       "[(selectors: { node: (state: any, props?: any) => DiagramNode | null; }) => [(state: any, props?: any) => DiagramNode | null], (node: DiagramNode | null) => Record<string, any>]",
								ReturnTypeString: "Record<string, any>",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if helper, ok := findParsedFunction(logic.InternalSelectorTypes, "node"); !ok || helper.FunctionType != "(nodes: DiagramNode[], nodeId: string | null) => DiagramNode | null" {
		t.Fatalf("expected node helper type %q, got %+v", "(nodes: DiagramNode[], nodeId: string | null) => DiagramNode | null", logic.InternalSelectorTypes)
	}
	if helper, ok := findParsedFunction(logic.InternalSelectorTypes, "nodeConfig"); !ok || helper.FunctionType != "(node: DiagramNode | null) => Record<string, any>" {
		t.Fatalf("expected nodeConfig helper type %q, got %+v", "(node: DiagramNode | null) => Record<string, any>", logic.InternalSelectorTypes)
	}
}

func TestBuildParsedLogicsInternalSelectorHelpersPreferDependencyNamesOverDestructuredProjectorParams(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, reducers, selectors } from 'kea'",
		"",
		"type LoadResult = [string | null, Error | null]",
		"",
		"export const appNodeLogic = kea([",
		"    reducers({",
		"        loadResult: [[null, null] as LoadResult, {}],",
		"    }),",
		"    selectors({",
		"        hasError: [",
		"            (s) => [s.loadResult],",
		"            ([_, error]) => error !== null,",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/appNodeLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "appNodeLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "loadResult", TypeString: "[LoadResult, {}]"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "hasError",
								TypeString:       "[(selectors: { loadResult: (state: any, props?: any) => LoadResult; }) => [(state: any, props?: any) => LoadResult], ([_, error]: LoadResult) => boolean]",
								ReturnTypeString: "boolean",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if selector, ok := findParsedField(logic.Selectors, "hasError"); !ok || selector.Type != "boolean" {
		t.Fatalf("expected hasError selector type %q, got %+v", "boolean", logic.Selectors)
	}
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	if len(sourceLogics) != 1 {
		t.Fatalf("expected 1 source logic, got %d", len(sourceLogics))
	}
	selectorsProperty := mustFindLogicProperty(t, sourceLogics[0], "selectors")
	sourceMembers := sectionSourceProperties(source, selectorsProperty)
	hasErrorProperty, ok := sourceMembers["hasError"]
	if !ok {
		t.Fatalf("expected source selector property for hasError, got %+v", sourceMembers)
	}
	directHelper := sourceInternalSelectorFunctionType(logic, source, "", sourcePropertyText(source, hasErrorProperty), nil)
	if directHelper != "(loadResult: LoadResult) => boolean" {
		t.Fatalf("expected direct source helper %q, got %q", "(loadResult: LoadResult) => boolean", directHelper)
	}
	if helper, ok := findParsedFunction(logic.InternalSelectorTypes, "hasError"); !ok || helper.FunctionType != "(loadResult: LoadResult) => boolean" {
		t.Fatalf("expected dependency-named internal selector helper %q, got %+v (direct source helper: %q)", "(loadResult: LoadResult) => boolean", logic.InternalSelectorTypes, directHelper)
	}
}

func TestBuildParsedLogicsPreservesFunctionReturningSelectorTypes(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, reducers, selectors } from 'kea'",
		"",
		"type Theme = { id: string }",
		"type Dataset = { id: string }",
		"type ColorToken = string",
		"",
		"export const trendsPaletteLogic = kea([",
		"    reducers({",
		"        theme: [null as Theme | null, {}],",
		"    }),",
		"    selectors({",
		"        getColorToken: [",
		"            (s) => [s.theme],",
		"            (theme) => {",
		"                return (dataset: Dataset): [Theme | null, ColorToken | null] => {",
		"                    return theme ? [theme, dataset.id] : [null, null]",
		"                }",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/trendsPaletteLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "trendsPaletteLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "theme", TypeString: "[Theme | null, {}]"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "getColorToken",
								TypeString:       "(((s: any) => any[]) | ((theme: any) => (dataset: Dataset) => [...]))[]",
								ReturnTypeString: "Dataset",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	selector, ok := findParsedField(logic.Selectors, "getColorToken")
	if !ok {
		t.Fatalf("expected getColorToken selector, got %+v", logic.Selectors)
	}
	if selector.Type != "((dataset: Dataset) => [Theme | null, ColorToken | null])" {
		t.Fatalf("expected function-returning selector type to be preserved, got %+v", selector)
	}
	if _, ok := findParsedFunction(logic.InternalSelectorTypes, "getColorToken"); ok {
		t.Fatalf("expected function-returning selector helper to be omitted, got %+v", logic.InternalSelectorTypes)
	}
}

func TestBuildParsedLogicsCollectsImports(t *testing.T) {
	root := repoRoot(t)
	sourcePath := filepath.Join(root, "samples", "builderLogic.ts")
	source := mustReadFile(t, sourcePath)

	report := &Report{
		ProjectDir: filepath.Join(root, "samples"),
		File:       sourcePath,
		Logics: []LogicReport{
			{
				Name:      "builderLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{
								Name:             "setRepositories",
								TypeString:       "(repositories: Repository[]) => { repositories: Repository[]; }",
								ReturnTypeString: "{ repositories: Repository[]; }",
							},
						},
					},
					{
						Name: "reducers",
						Members: []MemberReport{
							{
								Name:       "repositories",
								TypeString: "[Repository[], { setRepositories: (_: Repository[], { repositories }: { repositories: Repository[]; }) => Repository[]; }]",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	imports := logics[0].Imports
	if len(imports) != 1 {
		t.Fatalf("expected 1 import, got %+v", imports)
	}
	if imports[0].Path != "./types" {
		t.Fatalf("expected import path %q, got %q", "./types", imports[0].Path)
	}
	if len(imports[0].Names) != 1 || imports[0].Names[0] != "Repository" {
		t.Fatalf("expected Repository import, got %+v", imports[0].Names)
	}
}

func TestBuildParsedLogicsCollectsNamespaceAndDefaultImports(t *testing.T) {
	source := strings.Join([]string{
		"import Foo from './foo'",
		"import type * as ts from 'typescript'",
		"import { kea } from 'kea'",
		"",
		"export const importLogic = kea({",
		"    path: ['importLogic'],",
		"    props: {} as {",
		"        node: ts.Node",
		"        child: Foo.Bar",
		"        owner: Foo",
		"    },",
		"})",
		"",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/importLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "importLogic",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name:                "props",
						EffectiveTypeString: "{ node: ts.Node; child: Foo.Bar; owner: Foo; }",
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	imports := logics[0].Imports
	if !hasImport(imports, "./foo", "default as Foo") {
		t.Fatalf("expected default import ownership for Foo, got %+v", imports)
	}
	if !hasImport(imports, "typescript", "* as ts") {
		t.Fatalf("expected namespace import ownership for ts, got %+v", imports)
	}
}

func TestBuildParsedLogicsResolvesDefaultImportedConnectTargets(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "githubLogic.ts")
	if err := os.WriteFile(targetFile, []byte("export default null\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile targetFile: %v", err)
	}

	sourceFile := filepath.Join(tempDir, "defaultConnectLogic.ts")
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"import github from './githubLogic'",
		"",
		"export const defaultConnectLogic = kea({",
		"    connect: {",
		"        actions: [github, ['setRepositories']],",
		"        values: [github(), ['repositories']],",
		"    },",
		"})",
		"",
	}, "\n")

	report := &Report{
		ProjectDir: tempDir,
		File:       sourceFile,
		Logics: []LogicReport{
			{
				Name:      "defaultConnectLogic",
				InputKind: "object",
			},
		},
	}

	state := &buildState{
		binaryPath: "tsgo",
		projectDir: tempDir,
		configFile: filepath.Join(tempDir, "tsconfig.json"),
		parsedByFile: map[string][]ParsedLogic{
			filepath.Clean(targetFile): {
				{
					Name: "githubLogic",
					Actions: []ParsedAction{
						{
							Name:         "setRepositories",
							FunctionType: "(repositories: Repository[]) => { repositories: Repository[]; }",
							PayloadType:  "{ repositories: Repository[]; }",
						},
					},
					Selectors: []ParsedField{
						{Name: "repositories", Type: "Repository[]"},
					},
					Imports: []TypeImport{
						{Path: "./types", Names: []string{"Repository"}},
					},
				},
			},
		},
		building:      map[string]bool{},
		projectByFile: map[string]string{},
	}

	logics, err := buildParsedLogicsFromSource(report, source, state)
	if err != nil {
		t.Fatalf("buildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if !hasAction(logic.Actions, "setRepositories") {
		t.Fatalf("expected connected action for setRepositories, got %+v", logic.Actions)
	}
	if !hasImport(logic.Imports, "./types", "Repository") {
		t.Fatalf("expected connected Repository import, got %+v", logic.Imports)
	}
	if !hasReducer(logic.Selectors, "repositories", "Repository[]") {
		t.Fatalf("expected connected repositories selector type Repository[], got %+v", logic.Selectors)
	}
}

func TestBuildParsedLogicsConnectedActionsIgnoreLoaderReturnImports(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "organizationLogic.ts")
	if err := os.WriteFile(targetFile, []byte("export default null\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile targetFile: %v", err)
	}

	sourceFile := filepath.Join(tempDir, "consumerLogic.ts")
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"import organization from './organizationLogic'",
		"",
		"export const consumerLogic = kea({",
		"    connect: {",
		"        actions: [organization, ['loadCurrentOrganization']],",
		"    },",
		"})",
		"",
	}, "\n")

	report := &Report{
		ProjectDir: tempDir,
		File:       sourceFile,
		Logics: []LogicReport{
			{
				Name:      "consumerLogic",
				InputKind: "object",
			},
		},
	}

	state := &buildState{
		binaryPath: "tsgo",
		projectDir: tempDir,
		configFile: filepath.Join(tempDir, "tsconfig.json"),
		parsedByFile: map[string][]ParsedLogic{
			filepath.Clean(targetFile): {
				{
					Name: "organizationLogic",
					Actions: []ParsedAction{
						{
							Name:         "loadCurrentOrganization",
							FunctionType: "() => Promise<OrganizationType | null>",
							PayloadType:  "any",
						},
					},
					Imports: []TypeImport{
						{Path: "./types", Names: []string{"OrganizationType"}},
					},
				},
			},
		},
		building:      map[string]bool{},
		projectByFile: map[string]string{},
	}

	logics, err := buildParsedLogicsFromSource(report, source, state)
	if err != nil {
		t.Fatalf("buildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if !hasAction(logic.Actions, "loadCurrentOrganization") {
		t.Fatalf("expected connected action for loadCurrentOrganization, got %+v", logic.Actions)
	}
	if hasImport(logic.Imports, "./types", "OrganizationType") {
		t.Fatalf("expected connected loader action import to ignore OrganizationType return type, got %+v", logic.Imports)
	}
}

func TestBuildParsedLogicsConnectedActionsRecoverPackageSiblingExportsFromTargetSource(t *testing.T) {
	tempDir := t.TempDir()

	targetFile := filepath.Join(tempDir, "frameLogic.ts")
	targetSource := strings.Join([]string{
		"import { forms } from 'kea-forms'",
		"import type { FrameType } from './types'",
		"",
		"export const frameLogic = forms",
	}, "\n")
	if err := os.WriteFile(targetFile, []byte(targetSource), 0o644); err != nil {
		t.Fatalf("os.WriteFile targetFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "types.ts"), []byte("export interface FrameType { id: string }\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile types.ts: %v", err)
	}

	sourceFile := filepath.Join(tempDir, "consumerLogic.ts")
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"import { frameLogic } from './frameLogic'",
		"",
		"export const consumerLogic = kea({",
		"    connect: {",
		"        actions: [frameLogic, ['setFrameFormValues']],",
		"    },",
		"})",
		"",
	}, "\n")

	report := &Report{
		ProjectDir: tempDir,
		File:       sourceFile,
		Logics: []LogicReport{
			{
				Name:      "consumerLogic",
				InputKind: "object",
			},
		},
	}

	state := &buildState{
		binaryPath: "tsgo",
		projectDir: tempDir,
		configFile: filepath.Join(tempDir, "tsconfig.json"),
		parsedByFile: map[string][]ParsedLogic{
			filepath.Clean(targetFile): {
				{
					Name: "frameLogic",
					File: targetFile,
					Actions: []ParsedAction{
						{
							Name:         "setFrameFormValues",
							FunctionType: "(values: DeepPartial<FrameType>) => { values: DeepPartial<FrameType>; }",
							PayloadType:  "{ values: DeepPartial<FrameType>; }",
						},
					},
				},
			},
		},
		building:      map[string]bool{},
		projectByFile: map[string]string{},
	}

	logics, err := buildParsedLogicsFromSource(report, source, state)
	if err != nil {
		t.Fatalf("buildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if !hasAction(logic.Actions, "setFrameFormValues") {
		t.Fatalf("expected connected setFrameFormValues action, got %+v", logic.Actions)
	}
	if !hasImport(logic.Imports, "kea-forms", "DeepPartial") &&
		!hasImport(logic.Imports, "kea-forms/lib/index", "DeepPartial") &&
		!hasImport(logic.Imports, "kea-forms/lib/types", "DeepPartial") {
		t.Fatalf("expected connected imports to recover DeepPartial from target source, got %+v", logic.Imports)
	}
}

func TestBuildParsedLogicsConnectedParameterizedFormsRecoverDeepPartialImports(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tempDir, "types.ts"), []byte("export interface FrameType { id: string }\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	frameLogicTypeFile := filepath.Join(tempDir, "frameLogicType.ts")
	frameLogicTypeSource := strings.Join([]string{
		"import type { Logic } from 'kea'",
		"import type { FrameType } from './types'",
		"import type { DeepPartial } from 'kea-forms'",
		"",
		"export interface frameLogicType extends Logic {",
		"  actionCreators: {",
		"    setFrameFormValues: (values: DeepPartial<FrameType>) => {",
		"      type: 'set frame form values (frameLogic)'",
		"      payload: { values: DeepPartial<FrameType> }",
		"    }",
		"  }",
		"  actions: {",
		"    setFrameFormValues: (values: DeepPartial<FrameType>) => void",
		"  }",
		"}",
	}, "\n")
	if err := os.WriteFile(frameLogicTypeFile, []byte(frameLogicTypeSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	frameLogicFile := filepath.Join(tempDir, "frameLogic.ts")
	frameLogicSource := strings.Join([]string{
		"import { kea, path } from 'kea'",
		"import { forms } from 'kea-forms'",
		"import type { frameLogicType } from './frameLogicType'",
		"import type { FrameType } from './types'",
		"",
		"export const frameLogic = kea<frameLogicType>([",
		"  path(['frameLogic']),",
		"  forms(({ values }) => ({",
		"    frameForm: {",
		"      defaults: {} as FrameType,",
		"      submit: async () => values.frameForm,",
		"    },",
		"  })),",
		"])",
	}, "\n")
	if err := os.WriteFile(frameLogicFile, []byte(frameLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	consumerLogicFile := filepath.Join(tempDir, "consumerLogic.ts")
	consumerLogicSource := strings.Join([]string{
		"import { connect, kea, path } from 'kea'",
		"import { frameLogic } from './frameLogic'",
		"",
		"export const consumerLogic = kea([",
		"  path(['consumerLogic']),",
		"  connect({",
		"    actions: [frameLogic, ['setFrameFormValues']],",
		"  }),",
		"])",
	}, "\n")
	if err := os.WriteFile(consumerLogicFile, []byte(consumerLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       consumerLogicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if !hasAction(logic.Actions, "setFrameFormValues") {
		t.Fatalf("expected connected setFrameFormValues action, got %+v", logic.Actions)
	}
	if !hasImport(logic.Imports, "kea-forms", "DeepPartial") &&
		!hasImport(logic.Imports, "kea-forms/lib/index", "DeepPartial") &&
		!hasImport(logic.Imports, "kea-forms/lib/types", "DeepPartial") {
		t.Fatalf("expected connected parameterized forms import recovery, got %+v", logic.Imports)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 19, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"import type { DeepPartial } from 'kea-forms'",
		"setFrameFormValues: (values: DeepPartial<FrameType>) => void",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsNestedConnectedParameterizedFormsKeepDeepPartialImports(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["src/**/*"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	for path, contents := range map[string]string{
		filepath.Join(tempDir, "src", "types.ts"): strings.Join([]string{
			"export interface FrameType {",
			"  id: string",
			"  scenes?: { id: string }[]",
			"}",
		}, "\n"),
		filepath.Join(tempDir, "src", "scenes", "frame", "frameLogicType.ts"): strings.Join([]string{
			"import type { Logic } from 'kea'",
			"import type { FrameType } from '../../types'",
			"import type { DeepPartial } from 'kea-forms'",
			"",
			"export interface frameLogicType extends Logic {",
			"  actionCreators: {",
			"    setFrameFormValues: (values: DeepPartial<FrameType>) => {",
			"      type: 'set frame form values (src.scenes.frame.frameLogic)'",
			"      payload: { values: DeepPartial<FrameType> }",
			"    }",
			"  }",
			"  actions: {",
			"    setFrameFormValues: (values: DeepPartial<FrameType>) => void",
			"  }",
			"}",
		}, "\n"),
		filepath.Join(tempDir, "src", "scenes", "frame", "frameLogic.ts"): strings.Join([]string{
			"import { kea, path } from 'kea'",
			"import { forms } from 'kea-forms'",
			"import type { frameLogicType } from './frameLogicType'",
			"import type { FrameType } from '../../types'",
			"",
			"export const frameLogic = kea<frameLogicType>([",
			"  path(['src', 'scenes', 'frame', 'frameLogic']),",
			"  forms(({ values }) => ({",
			"    frameForm: {",
			"      defaults: {} as FrameType,",
			"      submit: async () => values.frameForm,",
			"    },",
			"  })),",
			"])",
		}, "\n"),
		filepath.Join(tempDir, "src", "scenes", "frame", "panels", "Schedule", "scheduleLogic.tsx"): strings.Join([]string{
			"import { connect, kea, listeners, path, props } from 'kea'",
			"import type { scheduleLogicType } from './scheduleLogicType'",
			"import { frameLogic } from '../../frameLogic'",
			"",
			"export interface ScheduleLogicProps {",
			"  frameId: number",
			"}",
			"",
			"export const scheduleLogic = kea<scheduleLogicType>([",
			"  path(['src', 'scenes', 'frame', 'panels', 'Schedule', 'scheduleLogic']),",
			"  props({} as ScheduleLogicProps),",
			"  connect((props: ScheduleLogicProps) => ({",
			"    actions: [frameLogic(props), ['setFrameFormValues']],",
			"  })),",
			"  listeners(({ actions }) => ({",
			"    saveFrame: () => {",
			"      actions.setFrameFormValues({ id: 'frame-1' })",
			"    },",
			"  })),",
			"])",
		}, "\n"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll returned error: %v", err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
	}

	scheduleLogicFile := filepath.Join(tempDir, "src", "scenes", "frame", "panels", "Schedule", "scheduleLogic.tsx")
	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       scheduleLogicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	action, ok := findParsedAction(logic.Actions, "setFrameFormValues")
	if !ok {
		t.Fatalf("expected connected setFrameFormValues action, got %+v", logic.Actions)
	}
	if action.FunctionType != "(values: DeepPartial<FrameType>) => void" {
		t.Fatalf("expected connected action function type %q, got %+v", "(values: DeepPartial<FrameType>) => void", action)
	}
	if action.PayloadType != "{ values: DeepPartial<FrameType>; }" {
		t.Fatalf("expected connected action payload type %q, got %+v", "{ values: DeepPartial<FrameType>; }", action)
	}
	if !hasImport(logic.Imports, "kea-forms", "DeepPartial") &&
		!hasImport(logic.Imports, "kea-forms/lib/index", "DeepPartial") &&
		!hasImport(logic.Imports, "kea-forms/lib/types", "DeepPartial") {
		t.Fatalf("expected nested connected parameterized forms import recovery, got %+v", logic.Imports)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 19, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"import type { DeepPartial } from 'kea-forms'",
		"setFrameFormValues: (values: DeepPartial<FrameType>) => void",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildStateInspectFileSkipsAPIForFilesWithoutLogics(t *testing.T) {
	tempDir := t.TempDir()
	sourceFile := filepath.Join(tempDir, "plain.ts")
	source := "export const meaning = 42\n"
	if err := os.WriteFile(sourceFile, []byte(source), 0o644); err != nil {
		t.Fatalf("os.WriteFile sourceFile: %v", err)
	}

	state := &buildState{
		binaryPath:    filepath.Join(tempDir, "missing-tsgo"),
		projectDir:    tempDir,
		configFile:    filepath.Join(tempDir, "tsconfig.json"),
		timeout:       time.Second,
		parsedByFile:  map[string][]ParsedLogic{},
		building:      map[string]bool{},
		projectByFile: map[string]string{},
	}

	report, gotSource, err := state.inspectFile(sourceFile)
	if err != nil {
		t.Fatalf("inspectFile returned error: %v", err)
	}
	if gotSource != source {
		t.Fatalf("expected source %q, got %q", source, gotSource)
	}
	if len(report.Logics) != 0 {
		t.Fatalf("expected no logics, got %+v", report.Logics)
	}
	if report.Snapshot != "" || report.Config != nil {
		t.Fatalf("expected no tsgo-backed metadata for no-logic file, got %+v", report)
	}
}

func TestBuildStateLoadParsedFileReturnsCachedEntryWithoutConfiguredAPI(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "logic.ts")
	logicSource := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"",
		"export const logic = kea([",
		"    path(['logic']),",
		"    actions({",
		"        setName: (name: string) => ({ name }),",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	state := &buildState{
		binaryPath:    tsgoapi.PreferredBinary(root),
		projectDir:    tempDir,
		configFile:    tsconfigPath,
		timeout:       15 * time.Second,
		parsedByFile:  map[string][]ParsedLogic{},
		building:      map[string]bool{},
		projectByFile: map[string]string{},
	}

	entry, err := state.loadParsedFile(logicFile)
	if err != nil {
		t.Fatalf("loadParsedFile returned error: %v", err)
	}
	if entry.Source != logicSource {
		t.Fatalf("expected cached source to match original source")
	}
	if len(entry.SourceLogics) != 1 || len(entry.Logics) != 1 {
		t.Fatalf("expected one cached source logic and one parsed logic, got %+v", entry)
	}

	if state.apiClient != nil {
		client := state.apiClient
		defer func() {
			_ = client.Close()
		}()
		state.apiClient = nil
	}
	state.binaryPath = ""
	state.projectDir = ""
	state.configFile = ""

	cached, err := state.loadParsedFile(logicFile)
	if err != nil {
		t.Fatalf("expected cached parsed file to load without configured API, got %v", err)
	}
	if cached.Source != logicSource {
		t.Fatalf("expected cached source %q, got %q", logicSource, cached.Source)
	}
	if len(cached.SourceLogics) != 1 || len(cached.Logics) != 1 {
		t.Fatalf("expected cached parsed file to keep its source and parsed logics, got %+v", cached)
	}
}

func TestBuildStateProjectIDForFileUsesPrimaryProjectShortcut(t *testing.T) {
	state := &buildState{primaryProjectID: "project-1"}

	projectID, err := state.projectIDForFile("/tmp/logic.ts")
	if err != nil {
		t.Fatalf("projectIDForFile returned error: %v", err)
	}
	if projectID != "project-1" {
		t.Fatalf("expected project-1, got %q", projectID)
	}
}

func TestParseConnectedTargetReferenceSupportsWrappedNamespaceTargets(t *testing.T) {
	for _, tc := range []struct {
		expression string
		baseAlias  string
		logicName  string
	}{
		{
			expression: `github["githubLogic"]`,
			baseAlias:  "github",
			logicName:  "githubLogic",
		},
		{
			expression: `github['githubLogic']()`,
			baseAlias:  "github",
			logicName:  "githubLogic",
		},
		{
			expression: `github?.githubLogic?.()`,
			baseAlias:  "github",
			logicName:  "githubLogic",
		},
		{
			expression: `((github['githubLogic'] as typeof github.githubLogic)!)`,
			baseAlias:  "github",
			logicName:  "githubLogic",
		},
		{
			expression: `(github['githubLogic'] satisfies typeof github.githubLogic)`,
			baseAlias:  "github",
			logicName:  "githubLogic",
		},
	} {
		target, ok := parseConnectedTargetReference(tc.expression)
		if !ok {
			t.Fatalf("expected %q to parse as a connected target", tc.expression)
		}
		if target.BaseAlias != tc.baseAlias || target.LogicName != tc.logicName {
			t.Fatalf("expected %q to parse as %s/%s, got %+v", tc.expression, tc.baseAlias, tc.logicName, target)
		}
	}
}

func TestBuildParsedLogicsResolvesBracketedAssertedNamespaceConnectTargets(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "githubLogic.ts")
	if err := os.WriteFile(targetFile, []byte("export default null\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile targetFile: %v", err)
	}

	sourceFile := filepath.Join(tempDir, "namespaceBracketConnectLogic.ts")
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"import * as github from './githubLogic'",
		"",
		"export const namespaceBracketConnectLogic = kea({",
		"    connect: {",
		"        actions: [((github['githubLogic'] as typeof github.githubLogic)!), ['setRepositories']],",
		"        values: [((github['githubLogic'] as typeof github.githubLogic)!)(), ['repositories']],",
		"    },",
		"})",
		"",
	}, "\n")

	report := &Report{
		ProjectDir: tempDir,
		File:       sourceFile,
		Logics: []LogicReport{
			{
				Name:      "namespaceBracketConnectLogic",
				InputKind: "object",
			},
		},
	}

	state := &buildState{
		binaryPath: "tsgo",
		projectDir: tempDir,
		configFile: filepath.Join(tempDir, "tsconfig.json"),
		parsedByFile: map[string][]ParsedLogic{
			filepath.Clean(targetFile): {
				{
					Name: "githubLogic",
					Actions: []ParsedAction{
						{
							Name:         "setRepositories",
							FunctionType: "(repositories: Repository[]) => { repositories: Repository[]; }",
							PayloadType:  "{ repositories: Repository[]; }",
						},
					},
					Selectors: []ParsedField{
						{Name: "repositories", Type: "Repository[]"},
					},
					Imports: []TypeImport{
						{Path: "./types", Names: []string{"Repository"}},
					},
				},
			},
		},
		building:      map[string]bool{},
		projectByFile: map[string]string{},
	}

	logics, err := buildParsedLogicsFromSource(report, source, state)
	if err != nil {
		t.Fatalf("buildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if !hasAction(logic.Actions, "setRepositories") {
		t.Fatalf("expected connected action for setRepositories, got %+v", logic.Actions)
	}
	if !hasImport(logic.Imports, "./types", "Repository") {
		t.Fatalf("expected connected Repository import, got %+v", logic.Imports)
	}
	if !hasReducer(logic.Selectors, "repositories", "Repository[]") {
		t.Fatalf("expected connected repositories selector type Repository[], got %+v", logic.Selectors)
	}
}

func TestEmitTypegenRendersNamespaceAndDefaultImports(t *testing.T) {
	rendered := EmitTypegenAt([]ParsedLogic{
		{
			Name:     "importLogic",
			TypeName: "importLogicType",
			Path:     []string{"importLogic"},
			Imports: []TypeImport{
				{Path: "./foo", Names: []string{"default as Foo"}},
				{Path: "typescript", Names: []string{"* as ts"}},
			},
			PropsType: "{ node: ts.Node; child: Foo.Bar; }",
		},
	}, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))

	if !strings.Contains(rendered, "import type { default as Foo } from './foo'\n") {
		t.Fatalf("expected default type import in output:\n%s", rendered)
	}
	if !strings.Contains(rendered, "import type * as ts from 'typescript'\n") {
		t.Fatalf("expected namespace type import in output:\n%s", rendered)
	}
}

func TestParseSelectorsPrefersReportedReturnType(t *testing.T) {
	section := SectionReport{
		Name: "selectors",
		Members: []MemberReport{
			{
				Name:             "capitalizedName",
				TypeString:       "[(s: { ... }) => [...], (name: string, number: number) => string]",
				ReturnTypeString: "string",
			},
		},
	}

	selectors := parseSelectors(section)
	if len(selectors) != 1 {
		t.Fatalf("expected 1 selector, got %+v", selectors)
	}
	if selectors[0].Type != "string" {
		t.Fatalf("expected selector type %q, got %q", "string", selectors[0].Type)
	}
}

func TestParseSelectorReturnTypeHandlesWriteRoundFunctionArrays(t *testing.T) {
	for _, tc := range []struct {
		name     string
		typeText string
		expected string
	}{
		{
			name:     "single function array",
			typeText: "(() => EventIndex)[]",
			expected: "EventIndex",
		},
		{
			name:     "dependency array union",
			typeText: "((() => never[]) | (() => RandomThing))[]",
			expected: "RandomThing",
		},
		{
			name:     "prefers shallower final array return",
			typeText: "(((selectors: any) => Repository[][]) | ((repositories: Repository[]) => Repository[]))[]",
			expected: "Repository[]",
		},
		{
			name:     "keeps props passthrough selector return scalar",
			typeText: "((value: any) => any)[]",
			expected: "any",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			returnType, ok := parseSelectorReturnType(tc.typeText)
			if !ok {
				t.Fatalf("expected selector return type to parse for %q", tc.typeText)
			}
			if returnType != tc.expected {
				t.Fatalf("expected selector return type %q, got %q", tc.expected, returnType)
			}
		})
	}
}

func TestParseLoaderMemberTypeHandlesTupleLoader(t *testing.T) {
	defaultType, properties, ok := parseLoaderMemberType("[Record<string, any>, { loadIt: () => { id: number; name: void; pinned: boolean; }; }]")
	if !ok {
		t.Fatalf("expected tuple loader member type to parse")
	}
	if defaultType != "Record<string, any>" {
		t.Fatalf("expected default type %q, got %q", "Record<string, any>", defaultType)
	}
	if properties["loadIt"] != "() => { id: number; name: void; pinned: boolean; }" {
		t.Fatalf("unexpected loadIt property type: %#v", properties)
	}

	actions := parseLoaderActions("misc", properties, "__default", properties["__default"])
	if len(actions) != 3 {
		t.Fatalf("expected request/success/failure loader actions, got %+v", actions)
	}
}

func TestParseReducerStateTypeWidensBooleanLiteralState(t *testing.T) {
	stateType, ok := parseReducerStateType("[false, { setUsername: () => true; setRepositories: () => false; setFetchError: () => false; }]")
	if !ok {
		t.Fatalf("expected reducer state type to parse")
	}
	if stateType != "boolean" {
		t.Fatalf("expected reducer state type %q, got %q", "boolean", stateType)
	}
}

func TestParseReducerStateTypeRecoversCollapsedTupleArrayUnion(t *testing.T) {
	stateType, ok := parseReducerStateType("(boolean | { syncDarkModePreference: (_: any, { darkModePreference }: { darkModePreference: any; }) => any; })[]")
	if !ok {
		t.Fatalf("expected collapsed reducer tuple array to parse")
	}
	if stateType != "boolean" {
		t.Fatalf("expected collapsed reducer state type %q, got %q", "boolean", stateType)
	}
}

func TestParseLoadersWithSourceRecoversCollapsedTupleLoaderMembers(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"export const loadersLogic = kea({",
		"    loaders: {",
		"        dashboard: {",
		"            __default: null as Dashboard | null,",
		"            addDashboard: ({ name }: { name: string }) => ({ id: -1, name, pinned: true } as Dashboard),",
		"        },",
		"        shouldNotBeNeverButAny: {",
		"            __default: [],",
		"        },",
		"        misc: [",
		"            {} as Record<string, any>,",
		"            {",
		"                loadIt: () => ({ id: -1, name, pinned: true }),",
		"            },",
		"        ],",
		"    },",
		"})",
	}, "\n")

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	property := mustFindLogicProperty(t, logics[0], "loaders")
	actions, reducers := parseLoadersWithSource(SectionReport{
		Name: "loaders",
		Members: []MemberReport{
			{Name: "dashboard", TypeString: "{ __default: Dashboard | null; addDashboard: ({ name }: { name: string; }) => Dashboard; }"},
			{Name: "shouldNotBeNeverButAny", TypeString: "{ __default: never[]; }"},
			{Name: "misc", TypeString: "Record<string, any>[]"},
		},
	}, source, property, "", nil)

	for _, actionName := range []string{"loadIt", "loadItSuccess", "loadItFailure"} {
		if !hasAction(actions, actionName) {
			t.Fatalf("expected source recovery to synthesize %s, got %+v", actionName, actions)
		}
	}
	for _, reducer := range []struct {
		name string
		typ  string
	}{
		{name: "misc", typ: "Record<string, any>"},
		{name: "miscLoading", typ: "boolean"},
		{name: "shouldNotBeNeverButAny", typ: "any[]"},
	} {
		if !hasReducer(reducers, reducer.name, reducer.typ) {
			t.Fatalf("expected reducer %s: %s, got %+v", reducer.name, reducer.typ, reducers)
		}
	}
}

func TestParseLoadersWithSourceRecoversDefaultTypeFromExplicitLoaderReturns(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"type UserProductListItem = { product_path: string }",
		"const getAppContext = () => ({ custom_products: undefined as UserProductListItem[] | undefined })",
		"",
		"export const customProductsLogic = kea({",
		"    loaders: {",
		"        customProducts: [",
		"            getAppContext()?.custom_products ?? [],",
		"            {",
		"                loadCustomProducts: async (): Promise<UserProductListItem[]> => {",
		"                    return []",
		"                },",
		"                seed: async (): Promise<UserProductListItem[]> => {",
		"                    return []",
		"                },",
		"            },",
		"        ],",
		"    },",
		"})",
	}, "\n")

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	property := mustFindLogicProperty(t, logics[0], "loaders")
	actions, reducers := parseLoadersWithSource(SectionReport{
		Name: "loaders",
		Members: []MemberReport{
			{Name: "customProducts", TypeString: "any[]"},
		},
	}, source, property, "", nil)

	if !hasReducer(reducers, "customProducts", "UserProductListItem[]") {
		t.Fatalf("expected customProducts reducer type UserProductListItem[], got %+v", reducers)
	}
	if action, ok := findParsedAction(actions, "loadCustomProductsSuccess"); !ok || !strings.Contains(action.PayloadType, "UserProductListItem[]") {
		t.Fatalf("expected loadCustomProductsSuccess payload to keep UserProductListItem[], got %+v", actions)
	}
}

func TestPreferredLoaderReturnTypeWithDefaultHintPrefersRecoveredDefaultAlias(t *testing.T) {
	source := strings.Join([]string{
		"type FileSystemEntry = { id: string }",
		"type SearchResults = { searchTerm: string; results: FileSystemEntry[]; hasMore: boolean; lastCount: number }",
	}, "\n")

	got := preferredLoaderReturnTypeWithDefaultHint(
		"{ results: any[]; searchTerm: any; }",
		"SearchResults",
		source,
		"",
		nil,
	)
	if got != "SearchResults" {
		t.Fatalf("expected SearchResults fallback alias, got %q", got)
	}
}

func TestPreferredLoaderReturnTypeWithDefaultHintFallsBackToDefaultArray(t *testing.T) {
	got := preferredLoaderReturnTypeWithDefaultHint("string", "FileSystemEntry[]", "", "", nil)
	if got != "FileSystemEntry[]" {
		t.Fatalf("expected array default fallback, got %q", got)
	}
}

func TestParseReducersWithSourceRecoversCollapsedLiteralReducerState(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"export const githubLogic = kea({",
		"    reducers: {",
		"        username: [",
		"            'keajs',",
		"            {",
		"                setUsername: (_, { username }) => username,",
		"            },",
		"        ],",
		"        isLoading: [",
		"            false,",
		"            {",
		"                setUsername: () => true,",
		"                setRepositories: () => false,",
		"                setFetchError: () => false,",
		"            },",
		"        ],",
		"    },",
		"})",
	}, "\n")

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	property := mustFindLogicProperty(t, logics[0], "reducers")
	reducers := parseReducersWithSource(SectionReport{
		Name: "reducers",
		Members: []MemberReport{
			{Name: "username", TypeString: "(string | { setUsername: (_: any, { username }: { username: any; }) => any; })[]"},
			{Name: "isLoading", TypeString: "(boolean | { setUsername: () => boolean; setRepositories: () => boolean; setFetchError: () => boolean; })[]"},
		},
	}, source, property, "", nil)

	for _, reducer := range []struct {
		name string
		typ  string
	}{
		{name: "username", typ: "string"},
		{name: "isLoading", typ: "boolean"},
	} {
		if !hasReducer(reducers, reducer.name, reducer.typ) {
			t.Fatalf("expected reducer %s: %s, got %+v", reducer.name, reducer.typ, reducers)
		}
	}
}

func TestParseReducersWithSourceRecoversBuilderReducerStateFromCommentsAndTypedConstants(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, reducers } from 'kea'",
		"",
		"const DEFAULT_SIDEBAR_WIDTH_PX: number = 288",
		"",
		"export const navigationLogic = kea([",
		"    reducers({",
		"        sidebarWidth: [",
		"            DEFAULT_SIDEBAR_WIDTH_PX,",
		"            {",
		"                setSidebarWidth: (_, { width }) => width,",
		"            },",
		"        ],",
		"        sidebarOverslide: [",
		"            // Overslide is how far beyond the min/max sidebar width the cursor has moved",
		"            0,",
		"            {",
		"                setSidebarOverslide: (_, { overslide }) => overslide,",
		"            },",
		"        ],",
		"        internalSearchTerm: [",
		"            // Do not reference this outside of this file",
		"            // `searchTerm` is the outwards-facing value, as it's made empty when search is hidden",
		"            '',",
		"            {",
		"                setSearchTerm: (_, { searchTerm }) => searchTerm,",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	property := mustFindLogicProperty(t, logics[0], "reducers")
	reducers := parseReducersWithSource(SectionReport{
		Name: "reducers",
		Members: []MemberReport{
			{Name: "sidebarWidth", TypeString: "(number | { setSidebarWidth: (_: any, { width }: { width: any; }) => any; })[]"},
			{Name: "sidebarOverslide", TypeString: "(number | { setSidebarOverslide: (_: any, { overslide }: { overslide: any; }) => any; })[]"},
			{Name: "internalSearchTerm", TypeString: "(string | { setSearchTerm: (_: any, { searchTerm }: { searchTerm: any; }) => any; })[]"},
		},
	}, source, property, "/tmp/navigationLogic.tsx", nil)

	for _, reducer := range []struct {
		name string
		typ  string
	}{
		{name: "sidebarWidth", typ: "number"},
		{name: "sidebarOverslide", typ: "number"},
		{name: "internalSearchTerm", typ: "string"},
	} {
		if !hasReducer(reducers, reducer.name, reducer.typ) {
			t.Fatalf("expected reducer %s: %s, got %+v", reducer.name, reducer.typ, reducers)
		}
	}
}

func TestBuildParsedLogicsRecoversCollapsedImportedSelectorsAndListenersFromSource(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "githubLogic.ts")
	typesFile := filepath.Join(tempDir, "types.ts")
	sourceFile := filepath.Join(tempDir, "githubImportLogic.ts")
	for path, content := range map[string]string{
		targetFile: "export {}",
		typesFile: strings.Join([]string{
			"export interface Repository {",
			"    id: number",
			"}",
			"",
		}, "\n"),
		sourceFile: "",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("os.WriteFile %s: %v", path, err)
		}
	}

	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"import { githubLogic } from './githubLogic'",
		"import { Repository } from './types'",
		"",
		"export const githubImportLogic = kea({",
		"    selectors: {",
		"        repositorySelectorCopy: [() => [githubLogic.selectors.repositories], (repositories) => repositories],",
		"    },",
		"    listeners: () => ({",
		"        [githubLogic.actionTypes.setUsername]: ({ username }) => {",
		"            console.log(username)",
		"        },",
		"    }),",
		"})",
		"",
	}, "\n")
	if err := os.WriteFile(sourceFile, []byte(source), 0o644); err != nil {
		t.Fatalf("os.WriteFile sourceFile: %v", err)
	}

	report := &Report{
		ProjectDir: tempDir,
		File:       sourceFile,
		Logics: []LogicReport{
			{
				Name:      "githubImportLogic",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name: "selectors",
						Members: []MemberReport{
							{Name: "repositorySelectorCopy", TypeString: "((repositories: any) => any)[]", ReturnTypeString: "any"},
						},
					},
					{
						Name:                "listeners",
						EffectiveTypeString: "{ [x: number]: ({ username }: { username: any; }) => void; }",
						PrintedTypeNode:     "{ [x: number]: ({ username }: { username: any; }) => void; }",
					},
				},
			},
		},
	}

	state := &buildState{
		binaryPath: "tsgo",
		projectDir: tempDir,
		configFile: filepath.Join(tempDir, "tsconfig.json"),
		parsedByFile: map[string][]ParsedLogic{
			filepath.Clean(targetFile): {
				{
					Name:       "githubLogic",
					Path:       []string{"githubLogic"},
					PathString: "githubLogic",
					Actions: []ParsedAction{
						{
							Name:         "setUsername",
							FunctionType: "(username: string) => { username: string; }",
							PayloadType:  "{ username: string; }",
						},
					},
					Reducers: []ParsedField{
						{Name: "repositories", Type: "Repository[]"},
					},
					Imports: []TypeImport{
						{Path: "./types", Names: []string{"Repository"}},
					},
				},
			},
		},
		building:      map[string]bool{},
		projectByFile: map[string]string{},
	}

	logics, err := buildParsedLogicsFromSource(report, source, state)
	if err != nil {
		t.Fatalf("buildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if selector, ok := findParsedField(logic.Selectors, "repositorySelectorCopy"); !ok || selector.Type != "Repository[]" {
		t.Fatalf("expected repositorySelectorCopy selector type Repository[], got %+v", logic.Selectors)
	}
	listener, ok := findParsedListener(logic.Listeners, "set username (githubLogic)")
	if !ok {
		t.Fatalf("expected computed imported listener to recover from source, got %+v", logic.Listeners)
	}
	if listener.PayloadType != "{ username: string; }" {
		t.Fatalf("expected listener payload type { username: string; }, got %+v", listener)
	}
	if !hasImport(logic.Imports, "./types", "Repository") {
		t.Fatalf("expected Repository import to be preserved, got %+v", logic.Imports)
	}
}

func TestParseSelectorsWithSourceRecoversCollapsedBlockBodiedArraySelector(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"export const githubLogic = kea({",
		"    selectors: {",
		"        sortedRepositories: [",
		"            (selectors) => [selectors.repositories],",
		"            (repositories) => {",
		"                return [...repositories].sort((a, b) => b.stargazers_count - a.stargazers_count)",
		"            },",
		"        ],",
		"    },",
		"})",
	}, "\n")

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	property := mustFindLogicProperty(t, logics[0], "selectors")
	selectors := parseSelectorsWithSource(
		SectionReport{
			Name: "selectors",
			Members: []MemberReport{
				{Name: "sortedRepositories", TypeString: "((repositories: any[]) => any[])[]", ReturnTypeString: "any[]"},
			},
		},
		ParsedLogic{
			Reducers: []ParsedField{
				{Name: "repositories", Type: "Repository[]"},
			},
		},
		source,
		property,
		"/tmp/githubLogic.ts",
		nil,
	)

	if selector, ok := findParsedField(selectors, "sortedRepositories"); !ok || selector.Type != "Repository[]" {
		t.Fatalf("expected sortedRepositories selector type Repository[], got %+v", selectors)
	}
}

func TestParseSelectorsWithSourceCanonicalizesImportedComputedStringKeys(t *testing.T) {
	tempDir := t.TempDir()
	logicFile := filepath.Join(tempDir, "logic.ts")
	typesFile := filepath.Join(tempDir, "types.ts")

	if err := os.WriteFile(typesFile, []byte("export const SIDE_PANEL_CONTEXT_KEY = 'sidePanelContext' as const\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"import { SIDE_PANEL_CONTEXT_KEY } from './types'",
		"",
		"type SidePanelSceneContext = { activity_scope?: string } | null",
		"",
		"export const logic = kea({",
		"    selectors: {",
		"        [SIDE_PANEL_CONTEXT_KEY]: [",
		"            () => [],",
		"            (): SidePanelSceneContext => null,",
		"        ],",
		"    },",
		"})",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	property := mustFindLogicProperty(t, logics[0], "selectors")
	selectors := parseSelectorsWithSource(
		SectionReport{
			Name: "selectors",
			Members: []MemberReport{
				{Name: "[SIDE_PANEL_CONTEXT_KEY]", TypeString: "((state: any) => any[])[]", ReturnTypeString: "any"},
			},
		},
		ParsedLogic{},
		source,
		property,
		logicFile,
		&buildState{},
	)

	selector, ok := findParsedField(selectors, "sidePanelContext")
	if !ok {
		t.Fatalf("expected sidePanelContext selector, got %+v", selectors)
	}
	if selector.Type != "SidePanelSceneContext" {
		t.Fatalf("expected sidePanelContext selector type SidePanelSceneContext, got %+v", selector)
	}
	if _, ok := findParsedField(selectors, "[SIDE_PANEL_CONTEXT_KEY]"); ok {
		t.Fatalf("expected computed selector key to be canonicalized, got %+v", selectors)
	}
	if got := canonicalSourceObjectMemberName(source, logicFile, "[SIDE_PANEL_CONTEXT_KEY]", &buildState{}); got != "sidePanelContext" {
		t.Fatalf("expected canonical selector key sidePanelContext, got %q", got)
	}
}

func TestBuildParsedLogicsFromSourceIgnoresSectionShapedPropsSelectorReturns(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, props, selectors } from 'kea'",
		"",
		"type LogicProps = {",
		"    filter?: any",
		"    value?: any",
		"}",
		"",
		"export const logic = kea([",
		"    props({} as LogicProps),",
		"    selectors({",
		"        selectedItemMeta: [() => [(_, props) => props.filter], (filter) => filter],",
		"        value: [() => [(_, props) => props.value], (value) => value],",
		"    }),",
		"])",
	}, "\n")

	sectionType := "{ selectedItemMeta: ((filter: any) => any)[]; value: ((value: any) => any)[]; }"
	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/logic.ts",
		Logics: []LogicReport{
			{
				Name:      "logic",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name:                "props",
						EffectiveTypeString: "LogicProps",
						PrintedTypeNode:     "LogicProps",
					},
					{
						Name:                "selectors",
						EffectiveTypeString: sectionType,
						PrintedTypeNode:     sectionType,
						Members: []MemberReport{
							{
								Name:             "selectedItemMeta",
								TypeString:       "((filter: any) => any)[]",
								ReturnTypeString: sectionType,
							},
							{
								Name:       "value",
								TypeString: "((value: any) => any)[]",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	selectedItemMeta, ok := findParsedField(logics[0].Selectors, "selectedItemMeta")
	if !ok {
		t.Fatalf("expected selectedItemMeta selector, got %+v", logics[0].Selectors)
	}
	if selectedItemMeta.Type != "any" {
		t.Fatalf("expected selectedItemMeta selector type any, got %+v", selectedItemMeta)
	}

	value, ok := findParsedField(logics[0].Selectors, "value")
	if !ok {
		t.Fatalf("expected value selector, got %+v", logics[0].Selectors)
	}
	if value.Type != "any" {
		t.Fatalf("expected value selector type any, got %+v", value)
	}
}

func TestParseSelectorsWithSourceRecoversObjectSelectorStringReturnFromLooseReport(t *testing.T) {
	source := mustReadFile(t, filepath.Join(repoRoot(t), "samples", "logic.ts"))

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	property := mustFindLogicProperty(t, logics[0], "selectors")
	selectors := parseSelectorsWithSource(
		SectionReport{
			Name: "selectors",
			Members: []MemberReport{
				{
					Name:             "capitalizedName",
					TypeString:       "((name: any, number: any) => any)[]",
					ReturnTypeString: "any",
				},
			},
		},
		ParsedLogic{
			Reducers: []ParsedField{
				{Name: "name", Type: "string"},
				{Name: "number", Type: "number"},
			},
		},
		source,
		property,
		filepath.Join(repoRoot(t), "samples", "logic.ts"),
		nil,
	)

	capitalizedName, ok := findParsedField(selectors, "capitalizedName")
	if !ok {
		t.Fatalf("expected capitalizedName selector, got %+v", selectors)
	}
	if capitalizedName.Type != "string" {
		t.Fatalf("expected capitalizedName selector type string, got %+v", capitalizedName)
	}
}

func TestPreferredMemberReturnTypeTextPreservesOptionalUndefinedInPrintedFunctionTypes(t *testing.T) {
	member := MemberReport{
		PrintedReturnTypeNode: "(feature: AvailableFeature, currentUsage?: number | undefined) => boolean",
		ReturnTypeString:      "(feature: AvailableFeature, currentUsage?: number | undefined) => boolean",
	}

	if got := preferredMemberReturnTypeText(member); got != "(feature: AvailableFeature, currentUsage?: number | undefined) => boolean" {
		t.Fatalf("expected preferred member return type to preserve optional undefined, got %q", got)
	}
}

func TestBuildParsedLogicsPreservesOptionalUndefinedInSelectorFunctionReturnTypes(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path, reducers, selectors } from 'kea'",
		"",
		"type AvailableFeature = 'flag'",
		"",
		"export const featureLogic = kea([",
		"    path(['featureLogic']),",
		"    reducers({",
		"        availableFeatures: [[] as AvailableFeature[], {}],",
		"    }),",
		"    selectors({",
		"        hasAvailableFeature: [",
		"            (s) => [s.availableFeatures],",
		"            (availableFeatures): ((feature: AvailableFeature, currentUsage?: number | undefined) => boolean) => {",
		"                return (feature, currentUsage) => availableFeatures.includes(feature) && !!currentUsage",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/featureLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "featureLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "availableFeatures", TypeString: "[AvailableFeature[], {}]"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:                  "hasAvailableFeature",
								TypeString:            "(((s: any) => AvailableFeature[]) | ((availableFeatures: AvailableFeature[]) => any))[]",
								PrintedReturnTypeNode: "(feature: AvailableFeature, currentUsage?: number | undefined) => boolean",
								ReturnTypeString:      "(feature: AvailableFeature, currentUsage?: number | undefined) => boolean",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	selector, ok := findParsedField(logics[0].Selectors, "hasAvailableFeature")
	if !ok {
		t.Fatalf("expected hasAvailableFeature selector, got %+v", logics[0].Selectors)
	}
	if !strings.Contains(selector.Type, "currentUsage?: number | undefined") {
		t.Fatalf("expected hasAvailableFeature selector type to preserve optional undefined, got %q", selector.Type)
	}
}

func TestBuildParsedLogicsRecoversOptionalUndefinedInSelectorFunctionReturnTypesFromTypeProbe(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "featureLogic.ts")
	logicSource := strings.Join([]string{
		"import { kea, path, reducers, selectors } from 'kea'",
		"",
		"type AvailableFeature = 'flag'",
		"",
		"export const featureLogic = kea([",
		"    path(['featureLogic']),",
		"    reducers({",
		"        availableFeatures: [[] as AvailableFeature[], {}],",
		"    }),",
		"    selectors({",
		"        hasAvailableFeature: [",
		"            (s) => [s.availableFeatures],",
		"            (availableFeatures) => {",
		"                return (feature: AvailableFeature, currentUsage?: number) =>",
		"                    availableFeatures.includes(feature) && !!currentUsage",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	selector, ok := findParsedField(logics[0].Selectors, "hasAvailableFeature")
	if !ok {
		t.Fatalf("expected hasAvailableFeature selector, got %+v", logics[0].Selectors)
	}
	if !strings.Contains(selector.Type, "currentUsage?: number | undefined") {
		t.Fatalf("expected hasAvailableFeature selector type to preserve optional undefined, got %q", selector.Type)
	}
}

func TestBuildParsedLogicsPrefersReportedSelectorFunctionReturnOverLiteralSourceFallback(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "featureLogic.ts")
	logicSource := strings.Join([]string{
		"import { kea, path, reducers, selectors } from 'kea'",
		"",
		"type AvailableFeature = 'flag'",
		"type AvailableProductFeature = { key: AvailableFeature; limit?: number }",
		"",
		"export const featureLogic = kea([",
		"    path(['featureLogic']),",
		"    reducers({",
		"        availableProductFeatures: [[] as AvailableProductFeature[], {}],",
		"    }),",
		"    selectors({",
		"        hasAvailableFeature: [",
		"            (s) => [s.availableProductFeatures],",
		"            (availableProductFeatures) => {",
		"                return (feature: AvailableFeature, currentUsage?: number) => {",
		"                    if (availableProductFeatures && availableProductFeatures.length > 0) {",
		"                        const availableFeature = availableProductFeatures.find((obj) => obj.key === feature)",
		"                        return availableFeature",
		"                            ? currentUsage",
		"                                ? availableFeature?.limit",
		"                                    ? availableFeature?.limit > currentUsage",
		"                                    : true",
		"                                : true",
		"                            : false",
		"                    }",
		"                    return false",
		"                }",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	selector, ok := findParsedField(logics[0].Selectors, "hasAvailableFeature")
	if !ok {
		t.Fatalf("expected hasAvailableFeature selector, got %+v", logics[0].Selectors)
	}
	if strings.Contains(selector.Type, "=> false") {
		t.Fatalf("expected hasAvailableFeature selector type to avoid literal false fallback, got %q", selector.Type)
	}
	if !strings.Contains(selector.Type, "=> boolean") {
		t.Fatalf("expected hasAvailableFeature selector type to preserve reported boolean return, got %q", selector.Type)
	}
}

func TestBuildParsedLogicsPrefersReportedConnectedSelectorFunctionReturnOverLiteralSourceFallback(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	featureLogicFile := filepath.Join(tempDir, "featureLogic.ts")
	featureLogicSource := strings.Join([]string{
		"import { kea, path, reducers, selectors } from 'kea'",
		"",
		"type AvailableFeature = 'flag'",
		"type AvailableProductFeature = { key: AvailableFeature; limit?: number }",
		"",
		"export const featureLogic = kea([",
		"    path(['featureLogic']),",
		"    reducers({",
		"        availableProductFeatures: [[] as AvailableProductFeature[], {}],",
		"    }),",
		"    selectors({",
		"        hasAvailableFeature: [",
		"            (s) => [s.availableProductFeatures],",
		"            (availableProductFeatures) => {",
		"                return (feature: AvailableFeature, currentUsage?: number) => {",
		"                    if (availableProductFeatures && availableProductFeatures.length > 0) {",
		"                        const availableFeature = availableProductFeatures.find((obj) => obj.key === feature)",
		"                        return availableFeature",
		"                            ? currentUsage",
		"                                ? availableFeature?.limit",
		"                                    ? availableFeature?.limit > currentUsage",
		"                                    : true",
		"                                : true",
		"                            : false",
		"                    }",
		"                    return false",
		"                }",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(featureLogicFile, []byte(featureLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	consumerLogicFile := filepath.Join(tempDir, "consumerLogic.ts")
	consumerLogicSource := strings.Join([]string{
		"import { connect, kea, path } from 'kea'",
		"import { featureLogic } from './featureLogic'",
		"",
		"export const consumerLogic = kea([",
		"    path(['consumerLogic']),",
		"    connect(() => ({",
		"        values: [featureLogic, ['hasAvailableFeature']],",
		"    })),",
		"])",
	}, "\n")
	if err := os.WriteFile(consumerLogicFile, []byte(consumerLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       consumerLogicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	selector, ok := findParsedField(logics[0].Selectors, "hasAvailableFeature")
	if !ok {
		t.Fatalf("expected connected hasAvailableFeature selector, got %+v", logics[0].Selectors)
	}
	if strings.Contains(selector.Type, "=> false") {
		t.Fatalf("expected connected hasAvailableFeature selector type to avoid literal false fallback, got %q", selector.Type)
	}
	if !strings.Contains(selector.Type, "=> boolean") {
		t.Fatalf("expected connected hasAvailableFeature selector type to preserve reported boolean return, got %q", selector.Type)
	}
}

func TestSelectorRecoveredLiteralFunctionShouldYieldToReported(t *testing.T) {
	current := "(feature: AvailableFeature, currentUsage?: number | undefined) => false"
	reported := "(feature: AvailableFeature, currentUsage?: number) => boolean"

	if !selectorRecoveredLiteralShouldYieldToReported(current, reported) {
		t.Fatalf("expected literal-driven selector function return to yield to reported type")
	}
}

func TestBuildParsedLogicsKeepsPropsPassthroughSelectorAsAny(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "logic.ts")
	logicSource := strings.Join([]string{
		"import { kea, props, selectors } from 'kea'",
		"",
		"type LogicProps = {",
		"    filter?: any",
		"    showNumericalPropsOnly?: boolean",
		"    value?: any",
		"}",
		"",
		"export const logic = kea([",
		"    props({} as LogicProps),",
		"    selectors({",
		"        selectedItemMeta: [() => [(_, props) => props.filter], (filter) => filter],",
		"        showNumericalPropsOnly: [",
		"            () => [(_, props) => props.showNumericalPropsOnly],",
		"            (showNumericalPropsOnly) => showNumericalPropsOnly ?? false,",
		"        ],",
		"        value: [() => [(_, props) => props.value], (value) => value],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	value, ok := findParsedField(logics[0].Selectors, "value")
	if !ok {
		t.Fatalf("expected value selector, got %+v", logics[0].Selectors)
	}
	if value.Type != "any" {
		t.Fatalf("expected value selector type any, got %+v", value)
	}
}

func TestBuildParsedLogicsSynthesizesLoadersAndDefaults(t *testing.T) {
	root := repoRoot(t)
	sourcePath := filepath.Join(root, "samples", "logic.ts")
	source := mustReadFile(t, sourcePath)

	report := &Report{
		ProjectDir: filepath.Join(root, "samples"),
		File:       sourcePath,
		Logics: []LogicReport{
			{
				Name:      "logic",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{Name: "updateName", TypeString: "(name: string) => { name: string; }", ReturnTypeString: "{ name: string; }"},
						},
					},
					{
						Name: "defaults",
						Members: []MemberReport{
							{Name: "yetAnotherNameWithNullDefault", TypeString: "string | null"},
						},
					},
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "name", TypeString: "[string, { updateName: (_: string, { name }: { name: string; }) => string; }]"},
							{Name: "otherNameNoDefault", TypeString: "{ updateName: (_: any, { name }: { name: string; }) => string; }"},
						},
					},
					{
						Name: "loaders",
						Members: []MemberReport{
							{Name: "sessions", TypeString: "{ __default: Session[]; loadSessions: (selectedDate: string) => Promise<Session[]>; }"},
						},
					},
					{
						Name: "listeners",
						Members: []MemberReport{
							{Name: "updateName", TypeString: "(payload: { name: string; }, previousState: any) => void"},
						},
					},
					{
						Name: "sharedListeners",
						Members: []MemberReport{
							{Name: "someRandomFunction", TypeString: "({ name }: { name: string; id?: number | undefined; }) => void"},
						},
					},
					{
						Name: "events",
						Members: []MemberReport{
							{Name: "afterMount", TypeString: "() => void"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	logic := logics[0]

	if !hasAction(logic.Actions, "loadSessions") || !hasAction(logic.Actions, "loadSessionsSuccess") || !hasAction(logic.Actions, "loadSessionsFailure") {
		t.Fatalf("expected synthesized loader actions, got %+v", logic.Actions)
	}
	if !hasReducer(logic.Reducers, "sessions", "Session[]") || !hasReducer(logic.Reducers, "sessionsLoading", "boolean") {
		t.Fatalf("expected synthesized loader reducers, got %+v", logic.Reducers)
	}
	if !hasReducer(logic.Reducers, "yetAnotherNameWithNullDefault", "string | null") {
		t.Fatalf("expected defaults to merge into reducers, got %+v", logic.Reducers)
	}
	if !hasImport(logic.Imports, "./logic", "Session") {
		t.Fatalf("expected Session import from ./logic, got %+v", logic.Imports)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"import type { Logic, BreakPointFunction } from 'kea'",
		"import type { Session } from './logic'",
		"loadSessions: 'load sessions (scenes.homepage.index.*)'",
		"loadSessionsSuccess: 'load sessions success (scenes.homepage.index.*)'",
		"loadSessionsFailure: 'load sessions failure (scenes.homepage.index.*)'",
		"updateName: ((action: { type: 'update name (scenes.homepage.index.*)'; payload: { name: string; } }, previousState: any) => void | Promise<void>)[]",
		"afterMount: () => void",
		"someRandomFunction: (payload: { name: string; id?: number; }, breakpoint: BreakPointFunction, action: { type: string; payload: { name: string; id?: number; }; }, previousState: any) => void | Promise<void>",
		"sessionsLoading: boolean",
		"payload?: string",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestSourceLoaderMemberTypeFromExpressionWidensSpreadUpdaterReturns(t *testing.T) {
	source := strings.Join([]string{
		"type RoleType = {",
		"    id: string",
		"    members: string[]",
		"    name: string",
		"}",
		"",
		"const roles = [",
		"    [] as RoleType[],",
		"    {",
		"        addMembersToRole: async ({ role, members }: { role: RoleType; members: string[] }) => {",
		"            if (!values.roles) {",
		"                return []",
		"            }",
		"            role.members = [...role.members, ...members]",
		"            return [...values.roles]",
		"        },",
		"        removeMemberFromRole: async ({ role, roleMemberId }: { role: RoleType; roleMemberId: string }) => {",
		"            if (!values.roles) {",
		"                return []",
		"            }",
		"            role.members = role.members.filter((member) => member !== roleMemberId)",
		"            return [...values.roles]",
		"        },",
		"        deleteRole: async ({ roleId }: { roleId: string }) => {",
		"            return values.roles?.filter((role) => role.id !== roleId) || []",
		"        },",
		"    },",
		"]",
	}, "\n")

	expression := strings.Join([]string{
		"[",
		"    [] as RoleType[],",
		"    {",
		"        addMembersToRole: async ({ role, members }: { role: RoleType; members: string[] }) => {",
		"            if (!values.roles) {",
		"                return []",
		"            }",
		"            role.members = [...role.members, ...members]",
		"            return [...values.roles]",
		"        },",
		"        removeMemberFromRole: async ({ role, roleMemberId }: { role: RoleType; roleMemberId: string }) => {",
		"            if (!values.roles) {",
		"                return []",
		"            }",
		"            role.members = role.members.filter((member) => member !== roleMemberId)",
		"            return [...values.roles]",
		"        },",
		"        deleteRole: async ({ roleId }: { roleId: string }) => {",
		"            return values.roles?.filter((role) => role.id !== roleId) || []",
		"        },",
		"    },",
		"]",
	}, "\n")

	defaultType, properties, ok := sourceLoaderMemberTypeFromExpression(source, expression)
	if !ok {
		t.Fatalf("expected loader member expression to parse")
	}
	if defaultType != "RoleType[]" {
		t.Fatalf("expected loader default type %q, got %q", "RoleType[]", defaultType)
	}
	if properties["addMembersToRole"] != "({ role, members }: { role: RoleType; members: string[]; }) => Promise<any[]>" {
		t.Fatalf("expected widened addMembersToRole property, got %#v", properties["addMembersToRole"])
	}
	if properties["removeMemberFromRole"] != "({ role, roleMemberId }: { role: RoleType; roleMemberId: string; }) => Promise<any[]>" {
		t.Fatalf("expected widened removeMemberFromRole property, got %#v", properties["removeMemberFromRole"])
	}
	if properties["deleteRole"] != "({ roleId }: { roleId: string; }) => Promise<RoleType[]>" {
		t.Fatalf("expected typed deleteRole property, got %#v", properties["deleteRole"])
	}
}

func TestSourceLoaderMemberTypeFromPropertyRecoversBuilderLoaderMembers(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, loaders } from 'kea'",
		"",
		"type RoleType = {",
		"    id: string",
		"    members: string[]",
		"    name: string",
		"}",
		"",
		"export const roleLogic = kea([",
		"    loaders(({ values }) => ({",
		"        roles: [",
		"            [] as RoleType[],",
		"            {",
		"                addMembersToRole: async ({ role, members }: { role: RoleType; members: string[] }) => {",
		"                    if (!values.roles) {",
		"                        return []",
		"                    }",
		"                    role.members = [...role.members, ...members]",
		"                    return [...values.roles]",
		"                },",
		"                removeMemberFromRole: async ({ role, roleMemberId }: { role: RoleType; roleMemberId: string }) => {",
		"                    if (!values.roles) {",
		"                        return []",
		"                    }",
		"                    role.members = role.members.filter((member) => member !== roleMemberId)",
		"                    return [...values.roles]",
		"                },",
		"                deleteRole: async ({ roleId }: { roleId: string }) => {",
		"                    return values.roles?.filter((role) => role.id !== roleId) || []",
		"                },",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	loadersProperty := mustFindLogicProperty(t, logics[0], "loaders")
	rolesProperty, ok := sectionSourceProperties(source, loadersProperty)["roles"]
	if !ok {
		t.Fatalf("expected roles loader property inside loaders section")
	}

	defaultType, properties, ok := sourceLoaderMemberTypeFromProperty(source, rolesProperty, "", nil)
	if !ok {
		t.Fatalf("expected builder loader property to parse")
	}
	if defaultType != "RoleType[]" {
		t.Fatalf("expected loader default type %q, got %q", "RoleType[]", defaultType)
	}
	if properties["addMembersToRole"] != "({ role, members }: { role: RoleType; members: string[]; }) => Promise<any[]>" {
		t.Fatalf("expected widened addMembersToRole property, got %#v", properties["addMembersToRole"])
	}
	if properties["removeMemberFromRole"] != "({ role, roleMemberId }: { role: RoleType; roleMemberId: string; }) => Promise<any[]>" {
		t.Fatalf("expected widened removeMemberFromRole property, got %#v", properties["removeMemberFromRole"])
	}
	if properties["deleteRole"] != "({ roleId }: { roleId: string; }) => Promise<RoleType[]>" {
		t.Fatalf("expected typed deleteRole property, got %#v", properties["deleteRole"])
	}
}

func TestBuildParsedLogicsLoadersSample(t *testing.T) {
	report := inspectSampleReport(t, "loadersLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if action, ok := findParsedAction(logic.Actions, "addDashboard"); !ok || action.FunctionType != "(name: string) => { name: string; }" {
		t.Fatalf("expected explicit addDashboard action signature to be preserved, got %+v", action)
	}
	for _, actionName := range []string{"loadIt", "loadItSuccess", "loadItFailure"} {
		if !hasAction(logic.Actions, actionName) {
			t.Fatalf("expected synthesized %s action, got %+v", actionName, logic.Actions)
		}
	}
	for _, reducer := range []struct {
		name     string
		typeText string
	}{
		{name: "misc", typeText: "Record<string, any>"},
		{name: "miscLoading", typeText: "boolean"},
		{name: "shouldNotBeNeverButAny", typeText: "any[]"},
		{name: "shouldNotBeNeverButAnyLoading", typeText: "boolean"},
	} {
		if !hasReducer(logic.Reducers, reducer.name, reducer.typeText) {
			t.Fatalf("expected reducer %s to have type %s, got %+v", reducer.name, reducer.typeText, logic.Reducers)
		}
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"addDashboard: (name: string) => void",
		"loadIt: () => void",
		"loadItSuccess: (misc: { id: number; name: void; pinned: boolean; }, payload?: any) => void",
		"shouldNotBeNeverButAny: any[]",
		"misc: Record<string, any>",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsLoadersPreferDefaultStateTypeWhenReturnTypeCollapses(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, loaders } from 'kea'",
		"",
		"type DemoItem = { id: string }",
		"",
		"export const loaderLogic = kea([",
		"    loaders({",
		"        items: [",
		"            [] as DemoItem[],",
		"            {",
		"                loadItems: async () => {",
		"                    return await new Promise((resolve) => resolve([] as DemoItem[]))",
		"                },",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/loaderLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "loaderLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "loaders",
						Members: []MemberReport{
							{
								Name:       "items",
								TypeString: "[DemoItem[], { loadItems: () => PromiseConstructor; }]",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	action, ok := findParsedAction(logic.Actions, "loadItemsSuccess")
	if !ok {
		t.Fatalf("expected loadItemsSuccess action, got %+v", logic.Actions)
	}
	if action.FunctionType != "(items: DemoItem[], payload?: any) => { items: DemoItem[]; payload?: any }" {
		t.Fatalf("expected loadItemsSuccess function type to use loader default type, got %+v", action)
	}
	if action.PayloadType != "{ items: DemoItem[]; payload?: any }" {
		t.Fatalf("expected loadItemsSuccess payload type to use loader default type, got %+v", action)
	}
}

func TestBuildParsedLogicsRefinesLoaderSuccessListenerPayloadsAfterActionRecovery(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, listeners, path } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"",
		"type DemoItem = { id: string }",
		"",
		"export const loaderLogic = kea([",
		"    path(['loaderLogic']),",
		"    loaders(() => ({",
		"        items: [",
		"            [] as DemoItem[],",
		"            {",
		"                loadItems: async () => new Promise((resolve) => resolve([] as DemoItem[])),",
		"            },",
		"        ],",
		"    })),",
		"    listeners(({ actions }) => ({",
		"        loadItemsSuccess: () => {",
		"            actions.loadItems()",
		"        },",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/loaderLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "loaderLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "loaders",
						Members: []MemberReport{
							{
								Name:       "items",
								TypeString: "[DemoItem[], { loadItems: () => PromiseConstructor; }]",
							},
						},
					},
					{
						Name: "listeners",
						Members: []MemberReport{
							{
								Name:       "loadItemsSuccess",
								TypeString: "() => void",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	listener, ok := findParsedListener(logic.Listeners, "loadItemsSuccess")
	if !ok {
		t.Fatalf("expected loadItemsSuccess listener, got %+v", logic.Listeners)
	}
	expectedActionType := "{ type: " + quoteString("load items success (loaderLogic)") + "; payload: { items: DemoItem[]; payload?: any } }"
	if listener.ActionType != expectedActionType {
		t.Fatalf("expected loadItemsSuccess listener action type %q, got %q", expectedActionType, listener.ActionType)
	}
}

func TestBuildParsedLogicsWidensLiteralLoaderSuccessTypesToReducerState(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"",
		"export const counterLogic = kea([",
		"    path(['counterLogic']),",
		"    loaders(() => ({",
		"        count: [",
		"            0,",
		"            {",
		"                increment: () => 1,",
		"                reset: () => 0,",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/counterLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "counterLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "loaders",
						Members: []MemberReport{
							{
								Name:       "count",
								TypeString: "[number, { increment: () => 1; reset: () => 0; }]",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	for _, actionName := range []string{"incrementSuccess", "resetSuccess"} {
		action, ok := findParsedAction(logic.Actions, actionName)
		if !ok {
			t.Fatalf("expected %s action, got %+v", actionName, logic.Actions)
		}
		if !strings.Contains(action.FunctionType, "(count: number, payload?: any) => { count: number; payload?: any }") {
			t.Fatalf("expected %s to use widened number success type, got %+v", actionName, action)
		}
	}
}

func TestBuildParsedLogicsRecoversBuilderPropsTypeFromSourceAssertion(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, key, path, props } from 'kea'",
		"",
		"interface DemoLogicProps {",
		"    id: string",
		"}",
		"",
		"export const demoLogic = kea([",
		"    path(['demoLogic']),",
		"    props({} as DemoLogicProps),",
		"    key((props: DemoLogicProps) => props.id),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/demoLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "demoLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name:                "props",
						EffectiveTypeString: "any",
						PrintedTypeNode:     "any",
					},
					{
						Name:                "key",
						EffectiveTypeString: "any",
						PrintedTypeNode:     "any",
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if logic.PropsType != "DemoLogicProps" {
		t.Fatalf("expected props type %q, got %q", "DemoLogicProps", logic.PropsType)
	}
	if logic.KeyType != "string" {
		t.Fatalf("expected key type %q, got %q", "string", logic.KeyType)
	}
}

func TestBuildParsedLogicsKeepsReportedAnyForUntypedBuilderKeyCallbacks(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, key, path, props } from 'kea'",
		"",
		"interface DemoLogicProps {",
		"    id: string",
		"}",
		"",
		"export const demoLogic = kea([",
		"    path(['demoLogic']),",
		"    props({} as DemoLogicProps),",
		"    key((props) => props.id),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/demoLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "demoLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name:                "props",
						EffectiveTypeString: "any",
						PrintedTypeNode:     "any",
					},
					{
						Name:                "key",
						EffectiveTypeString: "any",
						PrintedTypeNode:     "any",
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if logic.PropsType != "DemoLogicProps" {
		t.Fatalf("expected props type %q, got %q", "DemoLogicProps", logic.PropsType)
	}
	if logic.KeyType != "any" {
		t.Fatalf("expected key type %q, got %q", "any", logic.KeyType)
	}
}

func TestBuildParsedLogicsRecoversBuilderPropsAndUntypedKeyFromLogicBuilderLeak(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, key, path, props } from 'kea'",
		"",
		"interface DemoLogicProps {",
		"    id: string",
		"}",
		"",
		"export const demoLogic = kea([",
		"    path(['demoLogic']),",
		"    props({} as DemoLogicProps),",
		"    key((props) => props.id),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/demoLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "demoLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name:                "props",
						EffectiveTypeString: "LogicBuilder<L>",
						PrintedTypeNode:     "LogicBuilder<L>",
					},
					{
						Name:                "key",
						EffectiveTypeString: "LogicBuilder<L>",
						PrintedTypeNode:     "LogicBuilder<L>",
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if logic.PropsType != "DemoLogicProps" {
		t.Fatalf("expected props type %q, got %q", "DemoLogicProps", logic.PropsType)
	}
	if logic.KeyType != "string" {
		t.Fatalf("expected key type %q, got %q", "string", logic.KeyType)
	}
}

func TestShouldPreferRecoveredKeyTypePrefersPrimitiveOverReportedAny(t *testing.T) {
	if !shouldPreferRecoveredKeyType("any", "number") {
		t.Fatalf("expected primitive recovered key type to replace reported any")
	}
}

func TestShouldPreferRecoveredKeyTypeKeepsPrimitiveOverAnyDowngrade(t *testing.T) {
	if shouldPreferRecoveredKeyType("number", "any") {
		t.Fatalf("expected recovered primitive key type to survive weaker any inference")
	}
}

func TestShouldPreferRecoveredKeyTypePrefersPrimitiveOverFunctionSurface(t *testing.T) {
	if !shouldPreferRecoveredKeyType("(props: DemoLogicProps) => number", "number") {
		t.Fatalf("expected primitive key type to replace broader function surface")
	}
}

func TestBuildParsedLogicsRecoversUntypedBuilderKeyFromTypeProbeWhenTypegenImportIsMissing(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "demoLogic.ts")
	source := strings.Join([]string{
		"import { kea, key, path, props } from 'kea'",
		"import type { demoLogicType } from './demoLogicType'",
		"",
		"export interface DemoLogicProps {",
		"    id: number",
		"}",
		"",
		"export const demoLogic = kea<demoLogicType>([",
		"    path(['demoLogic']),",
		"    props({} as DemoLogicProps),",
		"    key((props) => props.id),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if logic.PropsType != "DemoLogicProps" {
		t.Fatalf("expected props type %q, got %q", "DemoLogicProps", logic.PropsType)
	}
	if logic.KeyType != "number" {
		t.Fatalf("expected key type %q, got %q", "number", logic.KeyType)
	}
}

func TestSourceKeyTypeFromPropsHandlesFrameOSStyleBuilderScaffolding(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, afterMount, BuiltLogic, connect, kea, key, listeners, path, props, reducers, selectors } from 'kea'",
		"import equal from 'fast-deep-equal'",
		"import { AppNodeData, Area, Panel, PanelWithMetadata } from '../../../types'",
		"",
		"import type { panelsLogicType } from './panelsLogicType'",
		"import { frameLogic } from '../frameLogic'",
		"import { actionToUrl, router, urlToAction } from 'kea-router'",
		"import { subscriptions } from 'kea-subscriptions'",
		"import { isFrameControlMode } from '../../../utils/frameControlMode'",
		"import { isInFrameAdminMode } from '../../../utils/frameAdmin'",
		"",
		"export interface PanelsLogicProps {",
		"  frameId: number",
		"}",
		"",
		"export interface AnyBuiltLogic extends BuiltLogic {}",
		"",
		"const DEFAULT_LAYOUT: Record<Area, PanelWithMetadata[]> = {",
		"  [Area.TopLeft]: [{ panel: Panel.Scenes, active: false, hidden: false }],",
		"}",
		"",
		"function panelsEqual(panel1: PanelWithMetadata, panel2: PanelWithMetadata) {",
		"  return panel1.panel === panel2.panel && panel1.key === panel2.key",
		"}",
		"",
		"export const panelsLogic = kea<panelsLogicType>([",
		"  path(['src', 'scenes', 'frame', 'panelsLogic']),",
		"  props({} as PanelsLogicProps),",
		"  key((props) => props.frameId),",
		"  connect((props: PanelsLogicProps) => ({",
		"    values: [frameLogic(props), ['defaultScene', 'frame', 'frameForm']],",
		"    actions: [frameLogic(props), ['closeScenePanels']],",
		"  })),",
		"  actions({",
		"    setPanels: (panels: Record<Area, PanelWithMetadata[]>) => ({ panels }),",
		"  }),",
		"])",
	}, "\n")

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 source logic, got %d", len(logics))
	}

	keyProperty := mustFindLogicProperty(t, logics[0], "key")
	if got := sourceKeyTypeFromProps(source, keyProperty, "PanelsLogicProps"); got != "number" {
		t.Fatalf("expected direct source key type %q, got %q", "number", got)
	}
}

func TestBuildParsedLogicsRecoversFrameOSScheduleSelectorAndKeyTypesWithoutExistingTypegenFiles(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "jsx": "react-jsx",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts", "**/*.tsx"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	typesPath := filepath.Join(srcDir, "types.ts")
	typesSource := strings.Join([]string{
		"export interface ScheduledEvent {",
		"  id: string",
		"  hour: number",
		"  minute: number",
		"  weekday: number",
		"  event: string",
		"  payload: { sceneId?: string | null; state?: Record<string, any> }",
		"}",
		"",
		"export interface FrameSchedule {",
		"  events: ScheduledEvent[]",
		"  disabled?: boolean",
		"}",
		"",
		"export interface FrameScene {",
		"  id: string",
		"  name?: string",
		"  fields?: StateField[]",
		"}",
		"",
		"export interface StateField {",
		"  name: string",
		"}",
		"",
		"export interface FrameType {",
		"  schedule?: FrameSchedule",
		"  scenes?: FrameScene[]",
		"}",
	}, "\n")
	if err := os.WriteFile(typesPath, []byte(typesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	frameLogicDir := filepath.Join(tempDir, "src", "scenes", "frame")
	if err := os.MkdirAll(frameLogicDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	frameLogicPath := filepath.Join(frameLogicDir, "frameLogic.ts")
	frameLogicSource := strings.Join([]string{
		"import { actions, kea, key, path, props, selectors } from 'kea'",
		"import type { FrameType } from '../../types'",
		"",
		"import type { frameLogicType } from './frameLogicType'",
		"",
		"export interface FrameLogicProps {",
		"  frameId: number",
		"}",
		"",
		"export const frameLogic = kea<frameLogicType>([",
		"  path(['src', 'scenes', 'frame', 'frameLogic']),",
		"  props({} as FrameLogicProps),",
		"  key((props: FrameLogicProps) => props.frameId),",
		"  actions({",
		"    setFrameFormValues: (values: Partial<FrameType>) => ({ values }),",
		"  }),",
		"  selectors({",
		"    frame: [() => [], (): FrameType => ({ schedule: { events: [] }, scenes: [] })],",
		"    frameForm: [() => [], (): Partial<FrameType> => ({})],",
		"  }),",
		"])",
	}, "\n")
	if err := os.WriteFile(frameLogicPath, []byte(frameLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	scheduleDir := filepath.Join(tempDir, "src", "scenes", "frame", "panels", "Schedule")
	if err := os.MkdirAll(scheduleDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	scheduleLogicPath := filepath.Join(scheduleDir, "scheduleLogic.tsx")
	scheduleLogicSource := strings.Join([]string{
		"import { actions, connect, kea, key, listeners, path, props, reducers, selectors } from 'kea'",
		"",
		"import type { scheduleLogicType } from './scheduleLogicType'",
		"import { FrameScene, FrameSchedule, ScheduledEvent, StateField } from '../../../../types'",
		"import { frameLogic } from '../../frameLogic'",
		"",
		"export interface ScheduleLogicProps {",
		"  frameId: number",
		"}",
		"",
		"export const scheduleLogic = kea<scheduleLogicType>([",
		"  path(['src', 'scenes', 'frame', 'panels', 'Schedule', 'scheduleLogic']),",
		"  props({} as ScheduleLogicProps),",
		"  // 👈 keep this ahead of key() so source offsets diverge from UTF-16 offsets",
		"  key((props) => props.frameId),",
		"  connect(({ frameId }: ScheduleLogicProps) => ({",
		"    values: [frameLogic({ frameId }), ['frame', 'frameForm']],",
		"    actions: [frameLogic({ frameId }), ['setFrameFormValues']],",
		"  })),",
		"  actions({",
		"    addEvent: () => ({ event: newScheduledEvent() }),",
		"    editEvent: (id: string) => ({ id }),",
		"    closeEvent: (id: string) => ({ id }),",
		"    deleteEvent: (id: string) => ({ id }),",
		"    toggleDescription: (id: string) => ({ id }),",
		"    setSort: (sort: string) => ({ sort }),",
		"  }),",
		"  reducers({",
		"    editingEvents: [",
		"      {} as Record<string, boolean>,",
		"      {",
		"        addEvent: (state, { event }) => ({ ...state, [event.id]: true }),",
		"        editEvent: (state, { id }) => ({ ...state, [id]: true }),",
		"        closeEvent: (state, { id }) => {",
		"          const { [id]: _, ...rest } = state",
		"          return rest",
		"        },",
		"        deleteEvent: (state, { id }) => {",
		"          const { [id]: _, ...rest } = state",
		"          return rest",
		"        },",
		"      },",
		"    ],",
		"    expandedDescriptions: [",
		"      {} as Record<string, boolean>,",
		"      {",
		"        toggleDescription: (state, { id }) => ({ ...state, [id]: !state[id] }),",
		"      },",
		"    ],",
		"    sort: [",
		"      'hour' as string,",
		"      {",
		"        setSort: (_, { sort }) => sort,",
		"      },",
		"    ],",
		"  }),",
		"  selectors({",
		"    schedule: [(s) => [s.frameForm, s.frame], (frameForm, frame) => frameForm.schedule ?? frame.schedule],",
		"    events: [(s) => [s.schedule], (schedule) => schedule?.events ?? []],",
		"    disabled: [(s) => [s.schedule], (schedule) => schedule?.disabled ?? false],",
		"    scenes: [(s) => [s.frame, s.frameForm], (frame, frameForm) => frameForm.scenes ?? frame.scenes],",
		"    sceneNames: [",
		"      (s) => [s.scenes],",
		"      (scenes): Record<string, string> =>",
		"        (scenes ?? []).reduce((acc, scene) => {",
		"          acc[scene.id] = scene.name || 'Unnamed Scene'",
		"          return acc",
		"        }, {} as Record<string, string>),",
		"    ],",
		"    scenesAsOptions: [",
		"      (s) => [s.scenes],",
		"      (scenes): { label: string; value: string }[] =>",
		"        [{ label: '- Select Scene -', value: '' }].concat(",
		"          (scenes ?? [])",
		"            .map((scene) => ({",
		"              label: scene.name || 'Unnamed Scene',",
		"              value: scene.id || '',",
		"            }))",
		"            .sort((a, b) => a.label.localeCompare(b.label))",
		"        ),",
		"    ],",
		"    fieldsForScene: [",
		"      (s) => [s.frame, s.frameForm],",
		"      (frame, frameForm): Record<string, StateField[]> =>",
		"        (frameForm?.scenes ?? frame.scenes ?? []).reduce((acc, scene) => {",
		"          acc[scene.id] = scene.fields ?? []",
		"          return acc",
		"        }, {} as Record<string, StateField[]>),",
		"    ],",
		"    sortedEvents: [",
		"      (s) => [s.events, s.sort, s.sceneNames],",
		"      (events, sort, sceneNames) => {",
		"        if (sort === 'day') {",
		"          return events.sort((a, b) => a.weekday - b.weekday)",
		"        } else if (sort === 'hour') {",
		"          return events.sort((a, b) => (a.hour === b.hour ? a.minute - b.minute : a.hour - b.hour))",
		"        } else if (sort === 'scene') {",
		"          return events.sort((a, b) =>",
		"            (sceneNames[a.payload.sceneId ?? ''] || a.payload.sceneId || '').localeCompare(",
		"              sceneNames[b.payload.sceneId ?? ''] || b.payload.sceneId || ''",
		"            )",
		"          )",
		"        }",
		"        return events",
		"      },",
		"    ],",
		"  }),",
		"  listeners(({ actions, values }) => ({",
		"    addEvent: ({ event }) => {",
		"      actions.setFrameFormValues({ ...values.frameForm, schedule: { events: [...values.events, event] } })",
		"    },",
		"    deleteEvent: ({ id }) => {",
		"      actions.setFrameFormValues({",
		"        ...values.frameForm,",
		"        schedule: { events: values.events.filter((event) => event.id !== id) },",
		"      })",
		"    },",
		"  })),",
		"])",
		"",
		"function newScheduledEvent(): ScheduledEvent {",
		"  return {",
		"    id: 'evt-1',",
		"    hour: 23,",
		"    minute: 59,",
		"    weekday: 0,",
		"    event: 'setCurrentScene',",
		"    payload: { sceneId: '', state: {} },",
		"  }",
		"}",
	}, "\n")
	if err := os.WriteFile(scheduleLogicPath, []byte(scheduleLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	state := &buildState{projectDir: tempDir, configFile: tsconfigPath}
	if expanded := expandImportedTypeAliasTextWithContext(scheduleLogicSource, scheduleLogicPath, "FrameType", state); expanded == "" {
		t.Fatalf("expected sibling exported type expansion for %q", "FrameType")
	}
	if inferred := sourceExpressionTypeTextWithContext(
		scheduleLogicSource,
		scheduleLogicPath,
		"schedule?.events ?? []",
		map[string]string{"schedule": "FrameSchedule | undefined"},
		state,
	); inferred != "ScheduledEvent[]" {
		t.Fatalf("expected direct nullish array fallback type %q, got %q", "ScheduledEvent[]", inferred)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       scheduleLogicPath,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if logic.KeyType != "number" {
		t.Fatalf("expected key type %q, got %q", "number", logic.KeyType)
	}
	if selector, ok := findParsedField(logic.Selectors, "schedule"); !ok || selector.Type != "FrameSchedule | undefined" {
		t.Fatalf("expected schedule selector type %q, got %+v", "FrameSchedule | undefined", logic.Selectors)
	}
	if selector, ok := findParsedField(logic.Selectors, "events"); !ok || selector.Type != "ScheduledEvent[]" {
		t.Fatalf("expected events selector type %q, got %+v", "ScheduledEvent[]", logic.Selectors)
	}
	if selector, ok := findParsedField(logic.Selectors, "sortedEvents"); !ok || selector.Type != "ScheduledEvent[]" {
		t.Fatalf("expected sortedEvents selector type %q, got %+v", "ScheduledEvent[]", logic.Selectors)
	}
}

func TestBuildParsedLogicsRecoversTemplateLiteralKeyFromMalformedReportedType(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, key, path, props } from 'kea'",
		"",
		"type StepProps = {",
		"    node?: { id?: string; templateId?: string }",
		"}",
		"",
		"export const stepLogic = kea([",
		"    path(['stepLogic']),",
		"    props({} as StepProps),",
		"    key(({ node }: StepProps) => `${node?.id}_${node?.templateId}`),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/stepLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "stepLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name:                "props",
						EffectiveTypeString: "StepProps",
						PrintedTypeNode:     "StepProps",
					},
					{
						Name:                "key",
						EffectiveTypeString: "`${ node?.id; }_${ node?.templateId; }`",
						PrintedTypeNode:     "`${ node?.id; }_${ node?.templateId; }`",
					},
				},
			},
		},
	}

	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	keyProperty := mustFindLogicProperty(t, sourceLogics[0], "key")
	if got := sourceArrowReturnTypeText(source, sourcePropertyText(source, keyProperty)); got != "string" {
		t.Fatalf("expected template literal key source return type %q, got %q", "string", got)
	}
	if got := sourceKeyTypeFromSource(source, "", keyProperty, "StepProps", nil); got != "string" {
		t.Fatalf("expected template literal key source type %q, got %q", "string", got)
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	if logics[0].KeyType != "string" {
		t.Fatalf("expected recovered template literal key type %q, got %q", "string", logics[0].KeyType)
	}
}

func TestMergeLogicalOperandTypesKeepsNonStringUnionMembers(t *testing.T) {
	got := mergeLogicalOperandTypes("string | number | null", "'new'", "||")
	if got != "string | number" {
		t.Fatalf("expected merged logical type %q, got %q", "string | number", got)
	}
}

func TestMergeLogicalOperandTypesDropsNullFallbackWhenLeftIsAlreadyConcrete(t *testing.T) {
	got := mergeLogicalOperandTypes("FrameType", "null", "||")
	if got != "FrameType" {
		t.Fatalf("expected merged logical type %q, got %q", "FrameType", got)
	}
}

func TestMergeLogicalOperandTypesPreservesOptionalObjectMemberForNullishCoalescing(t *testing.T) {
	got := mergeLogicalOperandTypes("FrameSchedule | undefined", "FrameSchedule | undefined", "??")
	if got != "FrameSchedule | undefined" {
		t.Fatalf("expected merged logical type %q, got %q", "FrameSchedule | undefined", got)
	}
}

func TestMergeLogicalOperandTypesPreservesConcreteArrayAcrossEmptyNullishFallback(t *testing.T) {
	got := mergeLogicalOperandTypes("ScheduledEvent[] | undefined", "any[]", "??")
	if got != "ScheduledEvent[]" {
		t.Fatalf("expected merged logical type %q, got %q", "ScheduledEvent[]", got)
	}
}

func TestMergeLogicalOperandTypesCollapsesPartialWrapperFallback(t *testing.T) {
	got := mergeLogicalOperandTypes("Partial<FrameType>", "FrameType", "||")
	if got != "Partial<FrameType>" {
		t.Fatalf("expected merged logical type %q, got %q", "Partial<FrameType>", got)
	}
}

func TestMergeNormalizedTypeUnionCollapsesWrapperSupertypes(t *testing.T) {
	got := mergeNormalizedTypeUnion("Partial<FrameType>", "FrameType")
	if got != "Partial<FrameType>" {
		t.Fatalf("expected merged union %q, got %q", "Partial<FrameType>", got)
	}
}

func TestRefineSelectorTypesFromInternalHelpersPrefersHelperReturnOverLeakedDependencyType(t *testing.T) {
	refined := refineSelectorTypesFromInternalHelpers(
		[]ParsedField{
			{Name: "frame", Type: "Record<number, FrameType>"},
			{Name: "defaultScene", Type: "FrameScene[]"},
		},
		[]ParsedFunction{
			{Name: "frame", FunctionType: "(frames: Record<number, FrameType>, frameId: number) => FrameType"},
			{Name: "defaultScene", FunctionType: "(frame: FrameType, frameForm: Partial<FrameType>) => string"},
		},
		false,
	)
	if len(refined) != 2 || refined[0].Type != "FrameType" || refined[1].Type != "string" {
		t.Fatalf("expected helper refinement to recover leaked selector returns, got %+v", refined)
	}
}

func TestShouldPreferParsedConnectedFieldWhenSymbolSideIsOnlyLiteralFallback(t *testing.T) {
	if !shouldPreferParsedConnectedField("null", "FrameType") {
		t.Fatalf("expected parsed connected field to beat literal null fallback")
	}
	if !shouldPreferParsedConnectedField("'rpios'", "'buildroot' | 'nixos' | 'rpios'") {
		t.Fatalf("expected parsed connected field to beat literal string fallback")
	}
	if !shouldPreferParsedConnectedField("false", "boolean") {
		t.Fatalf("expected parsed connected field to beat literal boolean fallback")
	}
}

func TestShouldPreferExistingParsedSelectorTypeForReportOnlyLiteralFallbacks(t *testing.T) {
	if !shouldPreferExistingParsedSelectorType("string", "null") {
		t.Fatalf("expected existing parsed selector type to beat report-only null fallback")
	}
	if !shouldPreferExistingParsedSelectorType("FrameType", "any") {
		t.Fatalf("expected existing parsed selector type to beat report-only any fallback")
	}
	if shouldPreferExistingParsedSelectorType("null", "string") {
		t.Fatalf("expected concrete report type to replace literal existing fallback")
	}
}

func TestSelectorLiteralNullishShouldYieldToInternalHelper(t *testing.T) {
	if !selectorLiteralNullishShouldYieldToInternalHelper("null", "string") {
		t.Fatalf("expected literal null selector fallback to yield to internal helper string return")
	}
	if !selectorLiteralNullishShouldYieldToInternalHelper("undefined", "number") {
		t.Fatalf("expected literal undefined selector fallback to yield to internal helper number return")
	}
	if selectorLiteralNullishShouldYieldToInternalHelper("string | null", "string") {
		t.Fatalf("did not expect mixed selector type to yield to internal helper")
	}
	if selectorLiteralNullishShouldYieldToInternalHelper("null", "string | null") {
		t.Fatalf("did not expect nullish helper return to win over null fallback")
	}
}

func TestSelectorOpaqueCurrentShouldYieldToInternalHelper(t *testing.T) {
	if !selectorOpaqueCurrentShouldYieldToInternalHelper("any", "string") {
		t.Fatalf("expected opaque selector current type to yield to concrete helper string return")
	}
	if !selectorOpaqueCurrentShouldYieldToInternalHelper("unknown", "number") {
		t.Fatalf("expected opaque selector current type to yield to concrete helper number return")
	}
	if selectorOpaqueCurrentShouldYieldToInternalHelper("any", "FrameType") {
		t.Fatalf("did not expect opaque selector current type to yield to non-primitive helper return")
	}
	if selectorOpaqueCurrentShouldYieldToInternalHelper("string", "number") {
		t.Fatalf("did not expect concrete selector current type to yield in opaque-current helper check")
	}
}

func TestInternalHelperComplexRecoveryShouldStayOpaque(t *testing.T) {
	if !internalHelperComplexRecoveryShouldStayOpaque("(frameForm: Partial<FrameType>, frame: any) => any", "(frameForm: Partial<FrameType>, frame: FrameType) => FrameSchedule | undefined") {
		t.Fatalf("expected mixed opaque helper parameters to block complex helper recovery")
	}
	if internalHelperComplexRecoveryShouldStayOpaque("(currentProject: any) => any", "(currentProject: ProjectType | null) => string | null") {
		t.Fatalf("did not expect primitive/nullish helper recovery to stay opaque")
	}
	if internalHelperComplexRecoveryShouldStayOpaque("(availableProjectLevels: AccessControlLevel[]) => any", "(availableProjectLevels: AccessControlLevel[]) => { label: string; value: AccessControlLevel }[]") {
		t.Fatalf("did not expect fully concrete helper dependencies to stay opaque")
	}
}

func TestSelectorOpaqueComplexRecoveryShouldStayAny(t *testing.T) {
	logic := ParsedLogic{
		Selectors: []ParsedField{
			{Name: "frameForm", Type: "Partial<FrameType>"},
			{Name: "frame", Type: "any"},
			{Name: "currentProject", Type: "ProjectType | null"},
		},
	}

	if !selectorOpaqueComplexRecoveryShouldStayAny(
		logic,
		"",
		"",
		"[(s) => [s.frameForm, s.frame], (frameForm, frame) => frameForm.schedule ?? frame.schedule]",
		"FrameSchedule | undefined",
		nil,
	) {
		t.Fatalf("expected opaque connected dependency to keep complex selector recovery as any")
	}

	if selectorOpaqueComplexRecoveryShouldStayAny(
		logic,
		"",
		"",
		"[(s) => [s.currentProject], (currentProject) => currentProject?.id ?? null]",
		"string | null",
		nil,
	) {
		t.Fatalf("did not expect primitive/nullish selector recovery to stay any")
	}

	if selectorOpaqueComplexRecoveryShouldStayAny(
		logic,
		"",
		"",
		"[() => [], () => ({ foo: true })]",
		"{ foo: boolean; }",
		nil,
	) {
		t.Fatalf("did not expect dependency-free complex selector recovery to stay any")
	}
}

func TestSourceExpressionTypeTextWithContextRecoversOptionalMemberLogicalFallbackFromUnionHint(t *testing.T) {
	got := sourceExpressionTypeTextWithContext(
		"type ProjectType = { id: number }",
		"",
		"currentProject?.id || null",
		map[string]string{"currentProject": "ProjectType | null"},
		nil,
	)
	if got != "number | null" {
		t.Fatalf("expected recovered logical fallback type %q, got %q", "number | null", got)
	}
}

func TestSourceExpressionTypeTextWithContextRecoversArrayFindFallbackAndSomeReturns(t *testing.T) {
	source := strings.Join([]string{
		"type FrameScene = { id: string; default?: boolean }",
		"type ChangeDetail = { requiresFullDeploy: boolean }",
	}, "\n")

	if got := sourceExpressionTypeTextWithContext(
		source,
		"",
		"(allScenes.find((scene) => scene.id === 'default' || scene.default) || allScenes[0])?.id ?? null",
		map[string]string{"allScenes": "FrameScene[]"},
		nil,
	); got != "string" {
		t.Fatalf("expected array find fallback type %q, got %q", "string", got)
	}

	if got := sourceExpressionTypeTextWithContext(
		source,
		"",
		"undeployedChangeDetails.some((change) => change.requiresFullDeploy)",
		map[string]string{"undeployedChangeDetails": "ChangeDetail[]"},
		nil,
	); got != "boolean" {
		t.Fatalf("expected array some return type %q, got %q", "boolean", got)
	}

	if got := sourceExpressionTypeTextWithContext(
		"",
		"",
		"error !== null",
		map[string]string{"error": "Error | null"},
		nil,
	); got != "boolean" {
		t.Fatalf("expected equality comparison type %q, got %q", "boolean", got)
	}
}

func TestSourceExpressionTypeTextWithContextMergesConditionalLiteralBranches(t *testing.T) {
	got := sourceExpressionTypeTextWithContext("", "", "someCondition ? 'project' : null", nil, nil)
	if got != "'project' | null" {
		t.Fatalf("expected conditional branches to merge, got %q", got)
	}
}

func TestSourceExpressionTypeTextWithContextSkipsBareLiteralConditionalFallbackWhenOtherBranchIsUnknown(t *testing.T) {
	got := sourceExpressionTypeTextWithContext(
		"",
		"",
		"querySource ? getBreakdown(querySource) : null",
		nil,
		nil,
	)
	if got != "" {
		t.Fatalf("expected unresolved conditional fallback to defer, got %q", got)
	}
}

func TestParseSourceArrowSignaturePreservesDestructuredParameterPattern(t *testing.T) {
	parameters, returnType, ok := parseSourceArrowSignature(
		"({ search }: { search?: string }) => ({ search })",
	)
	if !ok {
		t.Fatalf("expected parseSourceArrowSignature to parse arrow function")
	}
	if parameters != "({ search }: { search?: string; })" {
		t.Fatalf("expected destructured parameters to stay comma-safe, got %q", parameters)
	}
	if returnType != "" {
		t.Fatalf("expected no explicit return type, got %q", returnType)
	}
}

func TestParseSourceArrowInfoPreservesDestructuredParameterPattern(t *testing.T) {
	info, ok := parseSourceArrowInfo(
		"({ source, config }: { source: ExternalDataSource; config: Partial<ExternalDataSourceRevenueAnalyticsConfig> }) => null",
	)
	if !ok {
		t.Fatalf("expected parseSourceArrowInfo to parse arrow function")
	}
	if info.Parameters != "({ source, config }: { source: ExternalDataSource; config: Partial<ExternalDataSourceRevenueAnalyticsConfig>; })" {
		t.Fatalf("expected destructured parameters to stay comma-safe, got %q", info.Parameters)
	}
}

func TestBuildParsedLogicsRecoversSelectorConstructorTypeAndHelper(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"type FeatureFlagsResult = { results: { key: string }[] }",
		"type ExperimentsResult = { results: { feature_flag_key: string }[] }",
		"",
		"export const variantsPanelLogic = kea({",
		"    defaults: {",
		"        featureFlags: {} as FeatureFlagsResult,",
		"        experiments: {} as ExperimentsResult,",
		"    },",
		"    selectors: {",
		"        unavailableFeatureFlagKeys: [",
		"            (s) => [s.featureFlags, s.experiments],",
		"            (featureFlags, experiments) => {",
		"                return new Set([",
		"                    ...featureFlags.results.map((flag) => flag.key),",
		"                    ...experiments.results.map((experiment) => experiment.feature_flag_key),",
		"                ])",
		"            },",
		"        ],",
		"    },",
		"})",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/variantsPanelLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "variantsPanelLogic",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name: "defaults",
						Members: []MemberReport{
							{Name: "featureFlags", TypeString: "FeatureFlagsResult"},
							{Name: "experiments", TypeString: "ExperimentsResult"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "unavailableFeatureFlagKeys",
								TypeString:       "[(s: Record<string, Selector>) => [Selector, Selector], (featureFlags: any, experiments: any) => Set<any>]",
								ReturnTypeString: "SetConstructor",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	selector, ok := findParsedField(logic.Selectors, "unavailableFeatureFlagKeys")
	if !ok {
		t.Fatalf("expected unavailableFeatureFlagKeys selector, got %+v", logic.Selectors)
	}
	if !strings.HasPrefix(selector.Type, "Set<") {
		t.Fatalf("expected selector type to recover from constructor fallback, got %q", selector.Type)
	}
	helper, ok := findParsedFunction(logic.InternalSelectorTypes, "unavailableFeatureFlagKeys")
	if !ok {
		t.Fatalf("expected unavailableFeatureFlagKeys helper, got %+v", logic.InternalSelectorTypes)
	}
	if !strings.Contains(helper.FunctionType, "featureFlags: FeatureFlagsResult") ||
		!strings.Contains(helper.FunctionType, "experiments: ExperimentsResult") ||
		!strings.Contains(helper.FunctionType, ") => Set<") {
		t.Fatalf("expected recovered helper function type, got %q", helper.FunctionType)
	}
}

func TestBuildParsedLogicsKeepsExplicitGenericNewMapSelectorType(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"type GroupTypeIndex = number",
		"type GroupType = { group_type_index: GroupTypeIndex; name: string }",
		"",
		"export const groupsModel = kea({",
		"    defaults: {",
		"        groupTypesRaw: [] as GroupType[],",
		"    },",
		"    selectors: {",
		"        groupTypes: [",
		"            (s) => [s.groupTypesRaw],",
		"            (groupTypesRaw) =>",
		"                new Map<GroupTypeIndex, GroupType>(",
		"                    (groupTypesRaw ?? []).map((groupType) => [groupType.group_type_index, groupType])",
		"                ),",
		"        ],",
		"    },",
		"})",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/groupsModel.ts",
		Logics: []LogicReport{
			{
				Name:      "groupsModel",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name: "defaults",
						Members: []MemberReport{
							{Name: "groupTypesRaw", TypeString: "GroupType[]"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "groupTypes",
								TypeString:       "[(s: Record<string, Selector>) => GroupType[], (groupTypesRaw: any) => MapConstructor]",
								ReturnTypeString: "MapConstructor",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	selector, ok := findParsedField(logic.Selectors, "groupTypes")
	if !ok {
		t.Fatalf("expected groupTypes selector, got %+v", logic.Selectors)
	}
	if selector.Type != "Map<GroupTypeIndex, GroupType>" {
		t.Fatalf("expected selector type %q, got %q", "Map<GroupTypeIndex, GroupType>", selector.Type)
	}
}

func TestBuildParsedLogicsRecoversOptionalMemberFallbackSelectorType(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"type ProjectType = { id: number }",
		"",
		"export const projectLogic = kea({",
		"    defaults: {",
		"        currentProject: null as ProjectType | null,",
		"    },",
		"    selectors: {",
		"        currentProjectId: [",
		"            (s) => [s.currentProject],",
		"            (currentProject) => currentProject?.id || null,",
		"        ],",
		"    },",
		"})",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/projectLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "projectLogic",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name: "defaults",
						Members: []MemberReport{
							{Name: "currentProject", TypeString: "ProjectType | null"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "currentProjectId",
								TypeString:       "[(s: Record<string, Selector>) => ProjectType | null, (currentProject: any) => any]",
								ReturnTypeString: "any",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	selector, ok := findParsedField(logic.Selectors, "currentProjectId")
	if !ok {
		t.Fatalf("expected currentProjectId selector, got %+v", logic.Selectors)
	}
	if selector.Type != "number | null" {
		t.Fatalf("expected selector type %q, got %q", "number | null", selector.Type)
	}
}

func TestBuildParsedLogicsRecoversImportedOptionalMemberFallbackSelectorType(t *testing.T) {
	tempDir := t.TempDir()

	typesFile := filepath.Join(tempDir, "types.ts")
	if err := os.WriteFile(
		typesFile,
		[]byte("export interface ProjectType {\n    id: string\n}\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "projectLogic.ts")
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"import type { ProjectType } from './types'",
		"",
		"export const projectLogic = kea({",
		"    defaults: {",
		"        currentProject: null as ProjectType | null,",
		"    },",
		"    selectors: {",
		"        currentProjectId: [",
		"            (s) => [s.currentProject],",
		"            (currentProject) => currentProject?.id ?? null,",
		"        ],",
		"    },",
		"})",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	report := &Report{
		ProjectDir: tempDir,
		File:       logicFile,
		Logics: []LogicReport{
			{
				Name:      "projectLogic",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name: "defaults",
						Members: []MemberReport{
							{Name: "currentProject", TypeString: "ProjectType | null"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "currentProjectId",
								TypeString:       "[(s: Record<string, Selector>) => ProjectType | null, (currentProject: any) => any]",
								ReturnTypeString: "any",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	selector, ok := findParsedField(logics[0].Selectors, "currentProjectId")
	if !ok {
		t.Fatalf("expected currentProjectId selector, got %+v", logics[0].Selectors)
	}
	if selector.Type != "string | null" {
		t.Fatalf("expected selector type %q, got %+v", "string | null", selector)
	}
}

func TestBuildParsedLogicsRecoversImportedSelectorChainsAcrossSameSectionDependencies(t *testing.T) {
	tempDir := t.TempDir()

	typesFile := filepath.Join(tempDir, "types.ts")
	if err := os.WriteFile(
		typesFile,
		[]byte(strings.Join([]string{
			"export interface FrameScene {",
			"    id: string",
			"}",
			"",
			"export interface FrameType {",
			"    id: number",
			"    mode: 'buildroot' | 'nixos' | 'rpios'",
			"    scenes: FrameScene[]",
			"    last_successful_deploy: Record<string, any> | null",
			"}",
		}, "\n")),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "frameLogic.ts")
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"import type { FrameType } from './types'",
		"",
		"interface FrameLogicProps {",
		"    frameId: number",
		"}",
		"",
		"export const frameLogic = kea({",
		"    props: {} as FrameLogicProps,",
		"    defaults: {",
		"        frames: {} as Record<number, FrameType>,",
		"        frameForm: {} as Partial<FrameType>,",
		"    },",
		"    selectors: {",
		"        frameId: [",
		"            () => [(_, props) => props.frameId],",
		"            (frameId) => frameId,",
		"        ],",
		"        frame: [",
		"            (s) => [s.frames, s.frameId],",
		"            (frames, frameId) => frames[frameId] || null,",
		"        ],",
		"        mode: [",
		"            (s) => [s.frame, s.frameForm],",
		"            (frame, frameForm) => frameForm?.mode || frame?.mode || 'rpios',",
		"        ],",
		"        lastDeploy: [",
		"            (s) => [s.frame],",
		"            (frame) => frame?.last_successful_deploy ?? null,",
		"        ],",
		"        defaultScene: [",
		"            (s) => [s.frame, s.frameForm],",
		"            (frame, frameForm) => {",
		"                const allScenes = frameForm?.scenes ?? frame?.scenes ?? []",
		"                return allScenes[0]?.id ?? null",
		"            },",
		"        ],",
		"    },",
		"})",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	report := &Report{
		ProjectDir: tempDir,
		File:       logicFile,
		Logics: []LogicReport{
			{
				Name:      "frameLogic",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name:                "props",
						EffectiveTypeString: "FrameLogicProps",
					},
					{
						Name: "defaults",
						Members: []MemberReport{
							{Name: "frames", TypeString: "Record<number, FrameType>"},
							{Name: "frameForm", TypeString: "Partial<FrameType>"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{Name: "frameId", TypeString: "[() => [(_: any, props: any) => any], (frameId: any) => any]", ReturnTypeString: "any"},
							{Name: "frame", TypeString: "[(s: { frames: (state: any, props?: any) => Record<number, FrameType>; frameId: (state: any, props?: any) => any; }) => [Record<number, FrameType>, any], (frames: any, frameId: any) => any]", ReturnTypeString: "any"},
							{Name: "mode", TypeString: "[(s: { frame: (state: any, props?: any) => any; frameForm: (state: any, props?: any) => Partial<FrameType>; }) => [any, Partial<FrameType>], (frame: any, frameForm: any) => any]", ReturnTypeString: "any"},
							{Name: "lastDeploy", TypeString: "[(s: { frame: (state: any, props?: any) => any; }) => [any], (frame: any) => any]", ReturnTypeString: "any"},
							{Name: "defaultScene", TypeString: "[(s: { frame: (state: any, props?: any) => any; frameForm: (state: any, props?: any) => Partial<FrameType>; }) => [any, Partial<FrameType>], (frame: any, frameForm: any) => any]", ReturnTypeString: "any"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if selector, ok := findParsedField(logic.Selectors, "frame"); !ok || selector.Type != "FrameType" {
		t.Fatalf("expected frame selector type %q, got %+v", "FrameType", selector)
	}
	if selector, ok := findParsedField(logic.Selectors, "mode"); !ok || selector.Type != "'buildroot' | 'nixos' | 'rpios'" {
		t.Fatalf("expected mode selector type %q, got %+v", "'buildroot' | 'nixos' | 'rpios'", selector)
	}
	if selector, ok := findParsedField(logic.Selectors, "lastDeploy"); !ok || selector.Type != "Record<string, any> | null" {
		t.Fatalf("expected lastDeploy selector type %q, got %+v", "Record<string, any> | null", selector)
	}
	defaultSceneSelector, ok := findParsedField(logic.Selectors, "defaultScene")
	if !ok {
		t.Fatalf("expected defaultScene selector, got %+v", logic.Selectors)
	}
	if defaultSceneSelector.Type != "string" {
		t.Fatalf("expected defaultScene selector type %q, got %+v", "string", defaultSceneSelector)
	}
}

func TestBuildParsedLogicsRecoversBuilderSelectorsWhenFallbackMembersLackReportedSurface(t *testing.T) {
	tempDir := t.TempDir()

	if err := os.WriteFile(
		filepath.Join(tempDir, "types.ts"),
		[]byte(strings.Join([]string{
			"export interface FrameScene {",
			"    id: string",
			"}",
			"",
			"export interface FrameType {",
			"    scenes?: FrameScene[]",
			"}",
		}, "\n")),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	source := strings.Join([]string{
		"import { defaults, kea, path, props, selectors } from 'kea'",
		"",
		"import type { FrameType } from './types'",
		"",
		"interface FrameLogicProps {",
		"    frameId: number",
		"}",
		"",
		"export const frameLogic = kea([",
		"    path(['frameLogic']),",
		"    props({} as FrameLogicProps),",
		"    defaults({",
		"        frames: {} as Record<number, FrameType>,",
		"    }),",
		"    selectors(() => ({",
		"        frameId: [",
		"            () => [(_, props) => props.frameId],",
		"            (frameId) => frameId,",
		"        ],",
		"        frame: [",
		"            (s) => [s.frames, s.frameId],",
		"            (frames, frameId) => frames[frameId] || null,",
		"        ],",
		"        defaultScene: [",
		"            (s) => [s.frame],",
		"            (frame) => frame?.scenes?.[0]?.id ?? null,",
		"        ],",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: tempDir,
		File:       filepath.Join(tempDir, "frameLogic.ts"),
		Logics: []LogicReport{
			{
				Name:      "frameLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name:                "props",
						EffectiveTypeString: "FrameLogicProps",
					},
					{
						Name: "defaults",
						Members: []MemberReport{
							{Name: "frames", TypeString: "Record<number, FrameType>"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{Name: "frameId"},
							{Name: "frame"},
							{Name: "defaultScene"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if selector, ok := findParsedField(logic.Selectors, "frame"); !ok || selector.Type != "FrameType" {
		t.Fatalf("expected recovered frame selector type %q, got %+v", "FrameType", logic.Selectors)
	}
	if _, ok := findParsedField(logic.Selectors, "defaultScene"); !ok {
		t.Fatalf("expected recovered defaultScene selector to remain present, got %+v", logic.Selectors)
	}
}

func TestBuildParsedLogicsFromSourceRecoversFrameOSStyleSelectorTypes(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, reducers, selectors } from 'kea'",
		"",
		"type FrameScene = {",
		"    id: string",
		"    name: string",
		"}",
		"",
		"export const scenesLogic = kea([",
		"    reducers({",
		"        scenes: [[] as FrameScene[], {}],",
		"        search: ['', {}],",
		"        activeSceneId: [null as string | null, {}],",
		"        uploadedScenes: [[] as FrameScene[], {}],",
		"        aiSceneRequestId: [null as string | null, {}],",
		"        aiSceneLogsByRequestId: [",
		"            {} as Record<string, { message: string; status?: string; stage?: string; timestamp: string }[]>,",
		"            {},",
		"        ],",
		"    }),",
		"    selectors({",
		"        filteredScenes: [",
		"            (s) => [s.scenes, s.search],",
		"            (scenes, search) => {",
		"                const searchPieces = search",
		"                    .toLowerCase()",
		"                    .split(' ')",
		"                    .filter((s) => s)",
		"                if (searchPieces.length === 0) {",
		"                    return scenes",
		"                }",
		"                return scenes.filter((scene) => searchPieces.every((piece) => scene.name.toLowerCase().includes(piece)))",
		"            },",
		"        ],",
		"        sceneTitles: [",
		"            (s) => [s.scenes],",
		"            (scenes) => Object.fromEntries(scenes.map((scene) => [scene.id, scene.name])),",
		"        ],",
		"        linkedActiveSceneId: [",
		"            (s) => [s.activeSceneId, s.scenes],",
		"            (activeSceneId, scenes) => {",
		"                if (!activeSceneId) {",
		"                    return null",
		"                }",
		"                if (!activeSceneId.startsWith('uploaded/')) {",
		"                    return activeSceneId",
		"                }",
		"                const candidateId = activeSceneId.slice('uploaded/'.length)",
		"                return scenes.some((scene) => scene.id === candidateId) ? candidateId : activeSceneId",
		"            },",
		"        ],",
		"        activeUploadedScene: [",
		"            (s) => [s.activeSceneId, s.uploadedScenes],",
		"            (activeSceneId, uploadedScenes) =>",
		"                activeSceneId ? uploadedScenes.find((scene) => `uploaded/${scene.id}` === activeSceneId) ?? null : null,",
		"        ],",
		"        missingActiveMatchesSearch: [",
		"            (s) => [s.search, s.activeSceneId, s.activeUploadedScene],",
		"            (search, activeSceneId, activeUploadedScene) => {",
		"                if (!activeSceneId) {",
		"                    return false",
		"                }",
		"                const searchPieces = search",
		"                    .toLowerCase()",
		"                    .split(' ')",
		"                    .filter((s) => s)",
		"                if (searchPieces.length === 0) {",
		"                    return true",
		"                }",
		"                const sceneName = activeUploadedScene?.name?.toLowerCase() ?? ''",
		"                if (!sceneName) {",
		"                    return false",
		"                }",
		"                return searchPieces.every((piece) => sceneName.includes(piece))",
		"            },",
		"        ],",
		"        aiSceneLastLog: [",
		"            (s) => [s.aiSceneRequestId, s.aiSceneLogsByRequestId],",
		"            (requestId, logsByRequestId) => {",
		"                if (!requestId) {",
		"                    return null",
		"                }",
		"                const logs = logsByRequestId[requestId] ?? []",
		"                return logs.length ? logs[logs.length - 1] : null",
		"            },",
		"        ],",
		"        aiSceneLogs: [",
		"            (s) => [s.aiSceneRequestId, s.aiSceneLogsByRequestId],",
		"            (requestId, logsByRequestId) =>",
		"                requestId",
		"                    ? [...(logsByRequestId[requestId] ?? [])].toSorted(",
		"                          (left, right) => new Date(left.timestamp).getTime() - new Date(right.timestamp).getTime()",
		"                      )",
		"                    : [],",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/scenesLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "scenesLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "scenes", TypeString: "[FrameScene[], {}]"},
							{Name: "search", TypeString: "[string, {}]"},
							{Name: "activeSceneId", TypeString: "[string | null, {}]"},
							{Name: "uploadedScenes", TypeString: "[FrameScene[], {}]"},
							{Name: "aiSceneRequestId", TypeString: "[string | null, {}]"},
							{Name: "aiSceneLogsByRequestId", TypeString: "[Record<string, { message: string; status?: string; stage?: string; timestamp: string; }[]>, {}]"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{Name: "filteredScenes", TypeString: "(((s: any) => any[]) | ((scenes: FrameScene[], search: string) => string))[]", ReturnTypeString: "string"},
							{Name: "sceneTitles", TypeString: "(((s: any) => any[]) | ((scenes: FrameScene[]) => ObjectConstructor))[]", ReturnTypeString: "ObjectConstructor"},
							{Name: "linkedActiveSceneId", TypeString: "(((s: any) => any[]) | ((activeSceneId: string, scenes: FrameScene[]) => string))[]", ReturnTypeString: "string"},
							{Name: "activeUploadedScene", TypeString: "(((s: any) => any[]) | ((activeSceneId: string, uploadedScenes: FrameScene[]) => null))[]", ReturnTypeString: "null"},
							{Name: "missingActiveMatchesSearch", TypeString: "(((s: any) => any[]) | ((search: string, activeSceneId: string, activeUploadedScene: FrameScene) => any))[]", ReturnTypeString: "any"},
							{Name: "aiSceneLastLog", TypeString: "(((s: any) => any[]) | ((requestId: string, logsByRequestId: Record<string, { message: string; status?: string; stage?: string; timestamp: string; }[]>) => null))[]", ReturnTypeString: "null"},
							{Name: "aiSceneLogs", TypeString: "(((s: any) => any[]) | ((requestId: string, logsByRequestId: Record<string, { message: string; status?: string; stage?: string; timestamp: string; }[]>) => string))[]", ReturnTypeString: "string"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	for _, expected := range []struct {
		name string
		typ  string
	}{
		{name: "filteredScenes", typ: "FrameScene[]"},
		{name: "sceneTitles", typ: "Record<string, string>"},
		{name: "linkedActiveSceneId", typ: "null | string"},
		{name: "activeUploadedScene", typ: "FrameScene | null"},
		{name: "missingActiveMatchesSearch", typ: "boolean"},
		{name: "aiSceneLastLog", typ: "null | { message: string; status?: string | undefined; stage?: string | undefined; timestamp: string; }"},
		{name: "aiSceneLogs", typ: "{ message: string; status?: string | undefined; stage?: string | undefined; timestamp: string; }[]"},
	} {
		if selector, ok := findParsedField(logic.Selectors, expected.name); !ok || selector.Type != expected.typ {
			t.Fatalf("expected selector %s: %s, got %+v", expected.name, expected.typ, logic.Selectors)
		}
	}
}

func TestBuildParsedLogicsFromSourceIgnoresTupleSelectorReportedReturnType(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, props, reducers, selectors } from 'kea'",
		"",
		"type DiagramNode = {",
		"    id: string",
		"    selected?: boolean",
		"}",
		"",
		"type AppNodeLogicProps = {",
		"    nodeId: string",
		"}",
		"",
		"export const appNodeLogic = kea({",
		"    props: {} as AppNodeLogicProps,",
		"    reducers: {",
		"        nodes: [[] as DiagramNode[], {}],",
		"    },",
		"    selectors: {",
		"        nodeId: [() => [(_, props) => props.nodeId], (nodeId): string => nodeId],",
		"        node: [",
		"            (s) => [s.nodes, s.nodeId],",
		"            (nodes: DiagramNode[], nodeId: string) => nodes.find((node) => node.id === nodeId) ?? null,",
		"        ],",
		"    },",
		"})",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/appNodeLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "appNodeLogic",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name:                "props",
						EffectiveTypeString: "AppNodeLogicProps",
					},
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "nodes", TypeString: "[DiagramNode[], {}]"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "nodeId",
								TypeString:       "[() => [(_: any, props: any) => any], (nodeId: any) => any]",
								ReturnTypeString: "any",
							},
							{
								Name:             "node",
								TypeString:       "[(s: { nodes: (state: any, props?: any) => DiagramNode[]; nodeId: (state: any, props?: any) => string; }) => [...], (nodes: DiagramNode[], nodeId: string) => Di...",
								ReturnTypeString: "DiagramNode[]",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if selector, ok := findParsedField(logic.Selectors, "node"); !ok || selector.Type != "DiagramNode | null" {
		t.Fatalf("expected node selector type %q, got %+v", "DiagramNode | null", logic.Selectors)
	}
	if helper, ok := findParsedFunction(logic.InternalSelectorTypes, "node"); !ok || helper.FunctionType != "(nodes: DiagramNode[], nodeId: string) => DiagramNode | null" {
		t.Fatalf("expected node internal selector helper %q, got %+v", "(nodes: DiagramNode[], nodeId: string) => DiagramNode | null", logic.InternalSelectorTypes)
	}
}

func TestBuildParsedLogicsRecoversConnectedSelectorTypesWithoutExistingTypegenFiles(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	typesFile := filepath.Join(tempDir, "types.ts")
	typesSource := strings.Join([]string{
		"export interface FrameScene {",
		"    id: string",
		"    default?: boolean",
		"}",
		"",
		"export interface FrameType {",
		"    scenes?: FrameScene[]",
		"}",
	}, "\n")
	if err := os.WriteFile(typesFile, []byte(typesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	frameLogicFile := filepath.Join(tempDir, "frameLogic.ts")
	frameLogicSource := strings.Join([]string{
		"import { kea, key, path, props, selectors } from 'kea'",
		"import type { frameLogicType } from './frameLogicType'",
		"import type { FrameType } from './types'",
		"",
		"export interface FrameLogicProps {",
		"    frameId: number",
		"}",
		"",
		"export const frameLogic = kea<frameLogicType>([",
		"    path(['frameLogic']),",
		"    props({} as FrameLogicProps),",
		"    key((props) => props.frameId),",
		"    selectors(() => ({",
		"        frameId: [() => [(_, props) => props.frameId], (frameId) => frameId],",
		"        frame: [() => [], (): FrameType => ({ scenes: [{ id: 'default' }] })],",
		"        frameForm: [() => [], (): Partial<FrameType> => ({})],",
		"        defaultScene: [",
		"            (s) => [s.frame, s.frameForm],",
		"            (frame, frameForm) => {",
		"                const allScenes = frameForm?.scenes ?? frame?.scenes ?? []",
		"                return (allScenes.find((scene) => scene.id === 'default' || scene.default) || allScenes[0])?.id ?? null",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")
	if err := os.WriteFile(frameLogicFile, []byte(frameLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	panelsLogicFile := filepath.Join(tempDir, "panelsLogic.ts")
	panelsLogicSource := strings.Join([]string{
		"import { connect, kea, key, path, props } from 'kea'",
		"import type { panelsLogicType } from './panelsLogicType'",
		"import { frameLogic } from './frameLogic'",
		"",
		"export interface PanelsLogicProps {",
		"    frameId: number",
		"}",
		"",
		"export const panelsLogic = kea<panelsLogicType>([",
		"    path(['panelsLogic']),",
		"    props({} as PanelsLogicProps),",
		"    key((props) => props.frameId),",
		"    connect((props: PanelsLogicProps) => ({",
		"        values: [frameLogic(props), ['defaultScene', 'frame', 'frameForm']],",
		"    })),",
		"])",
	}, "\n")
	if err := os.WriteFile(panelsLogicFile, []byte(panelsLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       panelsLogicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if logic.KeyType != "number" {
		t.Fatalf("expected key type %q, got %q", "number", logic.KeyType)
	}
	if selector, ok := findParsedField(logic.Selectors, "defaultScene"); !ok || selector.Type != "string" {
		t.Fatalf("expected connected defaultScene selector type %q, got %+v", "string", logic.Selectors)
	}
	if selector, ok := findParsedField(logic.Selectors, "frame"); !ok || selector.Type != "FrameType" {
		t.Fatalf("expected connected frame selector type %q, got %+v", "FrameType", logic.Selectors)
	}
	if selector, ok := findParsedField(logic.Selectors, "frameForm"); !ok || selector.Type != "Partial<FrameType>" {
		t.Fatalf("expected connected frameForm selector type %q, got %+v", "Partial<FrameType>", logic.Selectors)
	}
}

func TestBuildParsedLogicsRecoversAliasedImportedOptionalMemberFallbackSelectorType(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "baseUrl": ".",`,
		`    "paths": {`,
		`      "~/*": ["*"]`,
		`    },`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if err := os.WriteFile(
		filepath.Join(tempDir, "types.ts"),
		[]byte("export interface ProjectType {\n    id: string\n}\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "projectLogic.ts")
	source := strings.Join([]string{
		"import { kea, path, selectors } from 'kea'",
		"",
		"import type { ProjectType } from '~/types'",
		"",
		"export const projectLogic = kea([",
		"    path(['projectLogic']),",
		"    selectors({",
		"        currentProjectId: [",
		"            () => [null as ProjectType | null],",
		"            (currentProject) => currentProject?.id || null,",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	selector, ok := findParsedField(logics[0].Selectors, "currentProjectId")
	if !ok {
		t.Fatalf("expected currentProjectId selector, got %+v", logics[0].Selectors)
	}
	if selector.Type != "string | null" {
		t.Fatalf("expected selector type %q, got %+v", "string | null", selector)
	}
}

func TestBuildParsedLogicsRecoversAliasedLoaderBackedOptionalMemberFallbackSelectorType(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "baseUrl": ".",`,
		`    "paths": {`,
		`      "~/*": ["*"]`,
		`    },`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if err := os.WriteFile(
		filepath.Join(tempDir, "types.ts"),
		[]byte(strings.Join([]string{
			"export interface ProjectBasicType {",
			"    id: number",
			"}",
			"",
			"export interface ProjectType extends ProjectBasicType {",
			"    created_at: string",
			"}",
		}, "\n")),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "projectLogic.ts")
	source := strings.Join([]string{
		"import { kea, path, selectors } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"",
		"import type { ProjectType } from '~/types'",
		"",
		"export const projectLogic = kea([",
		"    path(['projectLogic']),",
		"    loaders(() => ({",
		"        currentProject: [",
		"            null as ProjectType | null,",
		"            {",
		"                loadCurrentProject: async () => null,",
		"            },",
		"        ],",
		"    })),",
		"    selectors({",
		"        currentProjectId: [",
		"            (s) => [s.currentProject],",
		"            (currentProject) => currentProject?.id || null,",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	selector, ok := findParsedField(logics[0].Selectors, "currentProjectId")
	if !ok {
		t.Fatalf("expected currentProjectId selector, got %+v", logics[0].Selectors)
	}
	if selector.Type != "number | null" {
		t.Fatalf("expected selector type %q, got %+v", "number | null", selector)
	}
}

func TestBuildParsedLogicsRecoversPackageImportedSelectorMemberAccessChain(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	packageDir := filepath.Join(tempDir, "node_modules", "@flow", "react")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(packageDir, "package.json"),
		[]byte("{\n  \"name\": \"@flow/react\",\n  \"types\": \"index.d.ts\"\n}\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(packageDir, "index.d.ts"),
		[]byte(strings.Join([]string{
			"export interface FlowNodeData {",
			"    appId?: string | null",
			"}",
			"",
			"export type FlowNode<T = FlowNodeData> = {",
			"    id: string",
			"    selected?: boolean",
			"    data?: T",
			"}",
			"",
			"export type FlowEdge<T = any> = {",
			"    id: string",
			"    data?: T",
			"}",
			"",
			"export interface FlowState {",
			"    selectedNode: FlowNode | null",
			"    selectedEdge: FlowEdge | null",
			"}",
		}, "\n")),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "diagramLogic.ts")
	source := strings.Join([]string{
		"import { kea, reducers, selectors } from 'kea'",
		"",
		"import type { FlowState } from '@flow/react'",
		"",
		"export const diagramLogic = kea([",
		"    reducers({",
		"        flowState: [{} as FlowState, {}],",
		"    }),",
		"    selectors({",
		"        selectedNode: [",
		"            (s) => [s.flowState],",
		"            (flowState) => flowState.selectedNode ?? null,",
		"        ],",
		"        selectedEdge: [",
		"            (s) => [s.flowState],",
		"            (flowState) => flowState.selectedEdge ?? null,",
		"        ],",
		"        selectedNodeId: [",
		"            (s) => [s.selectedNode],",
		"            (selectedNode) => selectedNode?.id ?? null,",
		"        ],",
		"        selectedEdgeId: [",
		"            (s) => [s.selectedEdge],",
		"            (selectedEdge) => selectedEdge?.id ?? null,",
		"        ],",
		"        isSelected: [",
		"            (s) => [s.selectedNode],",
		"            (selectedNode) => selectedNode?.selected ?? false,",
		"        ],",
		"        selectedAppId: [",
		"            (s) => [s.selectedNode],",
		"            (selectedNode) => selectedNode?.data?.appId ?? null,",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	state := &buildState{projectDir: tempDir, configFile: tsconfigPath}
	resolvedFile, ok := resolveImportFile(logicFile, "@flow/react", state)
	if !ok {
		t.Fatalf("expected package import to resolve")
	}
	expectedPackageFile := filepath.Join(packageDir, "index.d.ts")
	if resolvedFile != expectedPackageFile {
		t.Fatalf("expected resolved package file %q, got %q", expectedPackageFile, resolvedFile)
	}

	report := &Report{
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Logics: []LogicReport{
			{
				Name:      "diagramLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "flowState", TypeString: "[FlowState, {}]"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "selectedNode",
								TypeString:       "[(selectors: { flowState: (state: any, props?: any) => FlowState; }) => [(state: any, props?: any) => FlowState], (flowState: any) => any]",
								ReturnTypeString: "any",
							},
							{
								Name:             "selectedEdge",
								TypeString:       "[(selectors: { flowState: (state: any, props?: any) => FlowState; }) => [(state: any, props?: any) => FlowState], (flowState: any) => any]",
								ReturnTypeString: "any",
							},
							{
								Name:             "selectedNodeId",
								TypeString:       "[(selectors: { selectedNode: (state: any, props?: any) => any; }) => [(state: any, props?: any) => any], (selectedNode: any) => any]",
								ReturnTypeString: "any",
							},
							{
								Name:             "selectedEdgeId",
								TypeString:       "[(selectors: { selectedEdge: (state: any, props?: any) => any; }) => [(state: any, props?: any) => any], (selectedEdge: any) => any]",
								ReturnTypeString: "any",
							},
							{
								Name:             "isSelected",
								TypeString:       "[(selectors: { selectedNode: (state: any, props?: any) => any; }) => [(state: any, props?: any) => any], (selectedNode: any) => any]",
								ReturnTypeString: "any",
							},
							{
								Name:             "selectedAppId",
								TypeString:       "[(selectors: { selectedNode: (state: any, props?: any) => any; }) => [(state: any, props?: any) => any], (selectedNode: any) => any]",
								ReturnTypeString: "any",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	for _, expected := range []struct {
		name string
		typ  string
	}{
		{name: "selectedNode", typ: "FlowNode | null"},
		{name: "selectedEdge", typ: "FlowEdge | null"},
		{name: "selectedNodeId", typ: "string | null"},
		{name: "selectedEdgeId", typ: "string | null"},
		{name: "isSelected", typ: "boolean"},
		{name: "selectedAppId", typ: "string | null"},
	} {
		if selector, ok := findParsedField(logic.Selectors, expected.name); !ok || selector.Type != expected.typ {
			t.Fatalf("expected selector %s: %s, got %+v", expected.name, expected.typ, logic.Selectors)
		}
	}
	if helper, ok := findParsedFunction(logic.InternalSelectorTypes, "selectedNodeId"); !ok || helper.FunctionType != "(selectedNode: FlowNode | null) => string | null" {
		t.Fatalf("expected selectedNodeId helper type %q, got %+v", "(selectedNode: FlowNode | null) => string | null", logic.InternalSelectorTypes)
	}
}

func TestBuildParsedLogicsKeepsReportedAnyForPropsBackedIdentityAndStringMethodSelectors(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(tempDir, "types.ts"),
		[]byte("export type APIScopeObject = 'project' | 'feature_flag'\n"),
		0o644,
	); err != nil {
		t.Fatalf("failed writing imported type fixture: %v", err)
	}

	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"import type { APIScopeObject } from './types'",
		"type AccessControlLogicProps = { resource: APIScopeObject }",
		"",
		"export const accessControlLogic = kea({",
		"    props: {} as AccessControlLogicProps,",
		"    selectors: {",
		"        resource: [",
		"            (_, p) => [p.resource],",
		"            (resource) => resource,",
		"        ],",
		"        humanReadableResource: [",
		"            (_, p) => [p.resource],",
		"            (resource) => resource.replace(/_/g, ' '),",
		"        ],",
		"    },",
		"})",
	}, "\n")

	report := &Report{
		ProjectDir: tempDir,
		File:       filepath.Join(tempDir, "accessControlLogic.ts"),
		Logics: []LogicReport{
			{
				Name:      "accessControlLogic",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name:                "props",
						EffectiveTypeString: "any",
						PrintedTypeNode:     "any",
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "resource",
								TypeString:       "[(s: Record<string, Selector>) => any, (resource: any) => any]",
								ReturnTypeString: "any",
							},
							{
								Name:             "humanReadableResource",
								TypeString:       "[(s: Record<string, Selector>) => any, (resource: any) => any]",
								ReturnTypeString: "any",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	resource, ok := findParsedField(logic.Selectors, "resource")
	if !ok {
		t.Fatalf("expected resource selector, got %+v", logic.Selectors)
	}
	if resource.Type != "any" {
		t.Fatalf("expected selector type %q, got %q", "any", resource.Type)
	}

	selector, ok := findParsedField(logic.Selectors, "humanReadableResource")
	if !ok {
		t.Fatalf("expected humanReadableResource selector, got %+v", logic.Selectors)
	}
	if selector.Type != "any" {
		t.Fatalf("expected selector type %q, got %q", "any", selector.Type)
	}
}

func TestBuildParsedLogicsKeepsReportedAnyForBuilderPropsBackedImportedAliasSelectors(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if err := os.WriteFile(
		filepath.Join(tempDir, "types.ts"),
		[]byte("export type APIScopeObject = 'project' | 'feature_flag'\n"),
		0o644,
	); err != nil {
		t.Fatalf("failed writing imported type fixture: %v", err)
	}

	logicFile := filepath.Join(tempDir, "accessControlLogic.ts")
	source := strings.Join([]string{
		"import { kea, props, selectors } from 'kea'",
		"",
		"import type { APIScopeObject } from './types'",
		"type AccessControlLogicProps = { resource: APIScopeObject }",
		"",
		"export const accessControlLogic = kea([",
		"    props({} as AccessControlLogicProps),",
		"    selectors({",
		"        resource: [",
		"            (_, p) => [p.resource],",
		"            (resource) => resource,",
		"        ],",
		"        humanReadableResource: [",
		"            (_, p) => [p.resource],",
		"            (resource) => resource.replace(/_/g, ' '),",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	resource, ok := findParsedField(logics[0].Selectors, "resource")
	if !ok {
		t.Fatalf("expected resource selector, got %+v", logics[0].Selectors)
	}
	if resource.Type != "any" {
		t.Fatalf("expected resource selector type %q, got %+v", "any", resource)
	}

	humanReadableResource, ok := findParsedField(logics[0].Selectors, "humanReadableResource")
	if !ok {
		t.Fatalf("expected humanReadableResource selector, got %+v", logics[0].Selectors)
	}
	if humanReadableResource.Type != "any" {
		t.Fatalf("expected humanReadableResource selector type %q, got %+v", "any", humanReadableResource)
	}

}

func TestBuildParsedLogicsKeepsReportedAnyForInspectedBuilderPrimitivePropsIdentitySelector(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "demoLogic.ts")
	source := strings.Join([]string{
		"import { kea, props, selectors } from 'kea'",
		"",
		"interface DemoLogicProps {",
		"    frameId: number",
		"}",
		"",
		"export const demoLogic = kea([",
		"    props({} as DemoLogicProps),",
		"    selectors({",
		"        frameId: [",
		"            () => [(_, props) => props.frameId],",
		"            (frameId) => frameId,",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	selector, ok := findParsedField(logics[0].Selectors, "frameId")
	if !ok {
		t.Fatalf("expected frameId selector, got %+v", logics[0].Selectors)
	}
	if selector.Type != "any" {
		t.Fatalf("expected frameId selector type %q, got %+v", "any", selector)
	}
}

func TestSourceMemberPathTypeTextHandlesMultilineInterfaceCallbacks(t *testing.T) {
	source := strings.Join([]string{
		"interface CodeEditorLogicProps {",
		"    key: string",
		"    query: string",
		"    onError?: (error: string | null) => void",
		"    onMetadata?: (metadata: string | null) => void",
		"}",
	}, "\n")

	got := sourceMemberPathTypeText(source, "CodeEditorLogicProps", []string{"key"})
	if got != "string" {
		t.Fatalf("expected member type %q, got %q", "string", got)
	}
}

func TestSourceKeyTypeFromSourceResolvesAliasedImportedHelperWithAbsoluteBaseURL(t *testing.T) {
	tempDir := t.TempDir()
	sharedUtilsDir := filepath.Join(tempDir, "frontend", "src", "scenes", "insights")
	if err := os.MkdirAll(sharedUtilsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	sharedUtilsFile := filepath.Join(sharedUtilsDir, "sharedUtils.ts")
	sharedUtilsSource := strings.Join([]string{
		"export interface InsightLogicProps {",
		"    id?: string",
		"}",
		"",
		"export const keyForInsightLogicProps =",
		"    (defaultKey = 'new') =>",
		"    (props: InsightLogicProps): string => props.id || defaultKey",
	}, "\n")
	if err := os.WriteFile(sharedUtilsFile, []byte(sharedUtilsSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "frontend", "src", "logic.ts")
	logicSource := strings.Join([]string{
		"import { kea, key, path } from 'kea'",
		"import { keyForInsightLogicProps } from 'scenes/insights/sharedUtils'",
		"",
		"export const demoLogic = kea([",
		"    path(['demoLogic']),",
		"    key(keyForInsightLogicProps('new')),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logics, err := FindLogics(logicSource)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 logic, got %d", len(logics))
	}

	properties := map[string]SourceProperty{}
	for _, property := range logics[0].Properties {
		properties[property.Name] = property
	}
	keyProperty, ok := properties["key"]
	if !ok {
		t.Fatalf("expected key property in %+v", logics[0].Properties)
	}

	state := &buildState{
		configFile: filepath.Join(tempDir, "tsconfig.json"),
		config: &tsgoapi.ConfigResponse{
			Options: map[string]any{
				"baseUrl": filepath.Join(tempDir, "frontend"),
				"paths": map[string]any{
					"scenes/*": []any{"src/scenes/*"},
				},
			},
		},
	}

	candidate, ok := parseNamedValueImports(logicSource)["keyForInsightLogicProps"]
	if !ok {
		t.Fatalf("expected named import for keyForInsightLogicProps")
	}
	resolvedFile, ok := resolveImportFile(logicFile, candidate.Path, state)
	if !ok {
		t.Fatalf("expected aliased import %q to resolve", candidate.Path)
	}
	if resolvedFile != sharedUtilsFile {
		t.Fatalf("expected resolved import file %q, got %q", sharedUtilsFile, resolvedFile)
	}
	importedSource, importedFile, initializer, ok := sourceImportedValueInitializer(
		logicSource,
		logicFile,
		"keyForInsightLogicProps",
		state,
	)
	if !ok {
		t.Fatalf("expected imported initializer for keyForInsightLogicProps")
	}
	if importedFile != sharedUtilsFile {
		t.Fatalf("expected imported file %q, got %q", sharedUtilsFile, importedFile)
	}
	if !strings.Contains(initializer, "(defaultKey = 'new') =>") {
		t.Fatalf("expected imported initializer, got %q", initializer)
	}
	info, ok := parseSourceArrowInfo(initializer)
	if !ok {
		t.Fatalf("expected imported initializer to parse as arrow")
	}
	innerInfo, ok := parseSourceArrowInfo(strings.TrimSpace(info.Body))
	if !ok {
		t.Fatalf("expected inner helper to parse as arrow, got %q", strings.TrimSpace(info.Body))
	}
	if innerInfo.ExplicitReturn != "string" {
		t.Fatalf("expected explicit inner helper return %q, got %q", "string", innerInfo.ExplicitReturn)
	}
	innerType := sourceExpressionTypeTextWithContext(importedSource, importedFile, strings.TrimSpace(info.Body), nil, state)
	if innerType != "(props: InsightLogicProps) => string" {
		t.Fatalf("expected inner helper type %q, got %q", "(props: InsightLogicProps) => string", innerType)
	}
	importedType := sourceExpressionTypeTextWithContext(importedSource, importedFile, initializer, nil, state)
	if importedType != "(defaultKey = 'new') => (props: InsightLogicProps) => string" {
		t.Fatalf("expected imported helper type %q, got %q", "(defaultKey = 'new') => (props: InsightLogicProps) => string", importedType)
	}
	keyExpressionType := sourceExpressionTypeTextWithContext(logicSource, logicFile, sourcePropertyText(logicSource, keyProperty), nil, state)
	if keyExpressionType != "(props: InsightLogicProps) => string" {
		t.Fatalf("expected key expression type %q, got %q", "(props: InsightLogicProps) => string", keyExpressionType)
	}

	got := sourceKeyTypeFromSource(logicSource, logicFile, keyProperty, "", state)
	if got != "string" {
		t.Fatalf("expected imported helper key type %q, got %q", "string", got)
	}
}

func TestSourceKeyTypeFromSourceResolvesRealisticPostHogHelper(t *testing.T) {
	tempDir := t.TempDir()
	frontendDir := filepath.Join(tempDir, "frontend")
	sharedUtilsDir := filepath.Join(frontendDir, "src", "scenes", "insights")
	if err := os.MkdirAll(sharedUtilsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	typesFile := filepath.Join(frontendDir, "src", "types.ts")
	typesSource := strings.Join([]string{
		"export interface InsightLogicProps {",
		"    dashboardId?: string",
		"    dashboardItemId?: string",
		"}",
	}, "\n")
	if err := os.WriteFile(typesFile, []byte(typesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	sharedUtilsFile := filepath.Join(sharedUtilsDir, "sharedUtils.ts")
	sharedUtilsSource := strings.Join([]string{
		"import { InsightLogicProps } from '~/types'",
		"",
		"export const keyForInsightLogicProps =",
		"    (defaultKey = 'new') =>",
		"    (props: InsightLogicProps): string => {",
		"        if (!('dashboardItemId' in props)) {",
		"            throw new Error('Must init with dashboardItemId, even if undefined')",
		"        }",
		"        return props.dashboardItemId",
		"            ? `${props.dashboardItemId}${props.dashboardId ? `/on-dashboard-${props.dashboardId}` : ''}`",
		"            : defaultKey",
		"    }",
	}, "\n")
	if err := os.WriteFile(sharedUtilsFile, []byte(sharedUtilsSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(frontendDir, "src", "lib", "components", "AddToDashboard", "addToDashboardModalLogic.ts")
	if err := os.MkdirAll(filepath.Dir(logicFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	logicSource := strings.Join([]string{
		"import { kea, key, path, props } from 'kea'",
		"import { keyForInsightLogicProps } from 'scenes/insights/sharedUtils'",
		"import { InsightLogicProps } from '~/types'",
		"",
		"export const demoLogic = kea([",
		"    props({} as InsightLogicProps),",
		"    path(['demoLogic']),",
		"    key(keyForInsightLogicProps('new')),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logics, err := FindLogics(logicSource)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 logic, got %d", len(logics))
	}

	properties := map[string]SourceProperty{}
	for _, property := range logics[0].Properties {
		properties[property.Name] = property
	}
	keyProperty, ok := properties["key"]
	if !ok {
		t.Fatalf("expected key property in %+v", logics[0].Properties)
	}

	state := &buildState{
		configFile: filepath.Join(tempDir, "tsconfig.json"),
		config: &tsgoapi.ConfigResponse{
			Options: map[string]any{
				"baseUrl": frontendDir,
				"paths": map[string]any{
					"scenes/*": []any{"src/scenes/*"},
					"~/*":      []any{"src/*"},
				},
			},
		},
	}

	importedSource, importedFile, initializer, ok := sourceImportedValueInitializer(
		logicSource,
		logicFile,
		"keyForInsightLogicProps",
		state,
	)
	if !ok {
		t.Fatalf("expected imported initializer for keyForInsightLogicProps")
	}
	if importedFile != sharedUtilsFile {
		t.Fatalf("expected imported file %q, got %q", sharedUtilsFile, importedFile)
	}

	importedType := sourceExpressionTypeTextWithContext(importedSource, importedFile, initializer, nil, state)
	if importedType != "(defaultKey = 'new') => (props: InsightLogicProps) => string" {
		t.Fatalf("expected imported helper type %q, got %q", "(defaultKey = 'new') => (props: InsightLogicProps) => string", importedType)
	}

	keyExpressionType := sourceExpressionTypeTextWithContext(logicSource, logicFile, sourcePropertyText(logicSource, keyProperty), nil, state)
	if keyExpressionType != "(props: InsightLogicProps) => string" {
		t.Fatalf("expected key expression type %q, got %q", "(props: InsightLogicProps) => string", keyExpressionType)
	}

	got := sourceKeyTypeFromSource(logicSource, logicFile, keyProperty, "InsightLogicProps", state)
	if got != "string" {
		t.Fatalf("expected imported helper key type %q, got %q", "string", got)
	}
}

func TestBuildParsedLogicsRecoversRealisticPostHogKeyTypeEndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	frontendDir := filepath.Join(tempDir, "frontend")
	sharedUtilsDir := filepath.Join(frontendDir, "src", "scenes", "insights")
	if err := os.MkdirAll(sharedUtilsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "baseUrl": "frontend",`,
		`    "paths": {`,
		`      "scenes/*": ["src/scenes/*"],`,
		`      "~/*": ["src/*"]`,
		`    }`,
		`  },`,
		`  "include": ["frontend/src/**/*"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	typesFile := filepath.Join(frontendDir, "src", "types.ts")
	typesSource := strings.Join([]string{
		"export interface InsightLogicProps {",
		"    dashboardId?: string",
		"    dashboardItemId?: string",
		"}",
	}, "\n")
	if err := os.WriteFile(typesFile, []byte(typesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	sharedUtilsFile := filepath.Join(sharedUtilsDir, "sharedUtils.ts")
	sharedUtilsSource := strings.Join([]string{
		"import { InsightLogicProps } from '~/types'",
		"",
		"export const keyForInsightLogicProps =",
		"    (defaultKey = 'new') =>",
		"    (props: InsightLogicProps): string => {",
		"        if (!('dashboardItemId' in props)) {",
		"            throw new Error('Must init with dashboardItemId, even if undefined')",
		"        }",
		"        return props.dashboardItemId",
		"            ? `${props.dashboardItemId}${props.dashboardId ? `/on-dashboard-${props.dashboardId}` : ''}`",
		"            : defaultKey",
		"    }",
	}, "\n")
	if err := os.WriteFile(sharedUtilsFile, []byte(sharedUtilsSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(frontendDir, "src", "lib", "components", "AddToDashboard", "addToDashboardModalLogic.ts")
	if err := os.MkdirAll(filepath.Dir(logicFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	logicSource := strings.Join([]string{
		"import { kea, key, path, props } from 'kea'",
		"import { keyForInsightLogicProps } from 'scenes/insights/sharedUtils'",
		"import { InsightLogicProps } from '~/types'",
		"",
		"export const demoLogic = kea([",
		"    props({} as InsightLogicProps),",
		"    path(['demoLogic']),",
		"    key(keyForInsightLogicProps('new')),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 logic, got %d", len(logics))
	}
	if logics[0].PropsType != "InsightLogicProps" {
		t.Fatalf("expected props type %q, got %q", "InsightLogicProps", logics[0].PropsType)
	}
	if logics[0].KeyType != "string" {
		t.Fatalf("expected key type %q, got %q", "string", logics[0].KeyType)
	}
}

func TestBuildParsedLogicsResolvesAliasedConnectedLogicEndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	frontendDir := filepath.Join(tempDir, "frontend")

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "baseUrl": "frontend",`,
		`    "paths": {`,
		`      "~/*": ["src/*"]`,
		`    }`,
		`  },`,
		`  "include": ["frontend/src/**/*"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	modelFile := filepath.Join(frontendDir, "src", "models", "propertyDefinitionsModel.ts")
	if err := os.MkdirAll(filepath.Dir(modelFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	modelSource := strings.Join([]string{
		"import { actions, kea, path, reducers } from 'kea'",
		"",
		"export type Option = {",
		"    values?: string[]",
		"}",
		"",
		"export const propertyDefinitionsModel = kea([",
		"    path(['models', 'propertyDefinitionsModel']),",
		"    actions({",
		"        loadPropertyValues: (payload: { propertyKey: string }) => payload,",
		"        setOptions: (options: Record<string, Option>) => ({ options }),",
		"    }),",
		"    reducers({",
		"        options: [",
		"            {} as Record<string, Option>,",
		"            {",
		"                setOptions: (_, { options }) => options,",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(modelFile, []byte(modelSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(frontendDir, "src", "lib", "components", "QuickFilters", "quickFilterFormLogic.ts")
	if err := os.MkdirAll(filepath.Dir(logicFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	logicSource := strings.Join([]string{
		"import { connect, kea, path } from 'kea'",
		"",
		"import { propertyDefinitionsModel } from '~/models/propertyDefinitionsModel'",
		"",
		"export const quickFilterFormLogic = kea([",
		"    path(['lib', 'components', 'QuickFilters', 'quickFilterFormLogic']),",
		"    connect(() => ({",
		"        values: [propertyDefinitionsModel, ['options as propertyOptions']],",
		"        actions: [propertyDefinitionsModel, ['loadPropertyValues']],",
		"    })),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 logic, got %d", len(logics))
	}

	logic := logics[0]
	if !hasAction(logic.Actions, "loadPropertyValues") {
		t.Fatalf("expected aliased connected action to resolve, got %+v", logic.Actions)
	}
	if selector, ok := findParsedField(logic.Selectors, "propertyOptions"); !ok || selector.Type != "Record<string, Option>" {
		t.Fatalf("expected aliased connected selector type %q, got %+v", "Record<string, Option>", selector)
	}
	if !hasImport(logic.Imports, "../../../models/propertyDefinitionsModel", "Option") {
		t.Fatalf("expected connected Option import from aliased local model, got %+v", logic.Imports)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"loadPropertyValues: (payload: { propertyKey: string; }) => void",
		"propertyOptions: (state: any, props?: any) => Record<string, Option>",
		"propertyOptions: Record<string, Option>",
		"import type { Option } from '../../../models/propertyDefinitionsModel'",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsRecoversSelectorOptionsAndConnectedValueTypesEndToEnd(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["src/**/*"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	sceneLogicFile := filepath.Join(tempDir, "src", "scenes", "sceneLogic.ts")
	if err := os.MkdirAll(filepath.Dir(sceneLogicFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	sceneLogicSource := strings.Join([]string{
		"import { kea, path, reducers, selectors } from 'kea'",
		"",
		"export type Scene = 'demo'",
		"export type SceneConfig = { name: string }",
		"",
		"const equal = (a: any, b: any): boolean => a === b",
		"const sceneConfigurations: Record<Scene, SceneConfig> = {",
		"    demo: { name: 'Demo scene' },",
		"}",
		"",
		"export const sceneLogic = kea([",
		"    path(['scenes', 'sceneLogic']),",
		"    reducers({",
		"        sceneId: ['demo' as Scene, {}],",
		"    }),",
		"    selectors({",
		"        sceneConfig: [",
		"            (s) => [s.sceneId],",
		"            (sceneId: Scene): SceneConfig | null => {",
		"                const config = sceneConfigurations[sceneId] || null",
		"                return config",
		"            },",
		"            { resultEqualityCheck: equal },",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(sceneLogicFile, []byte(sceneLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	navigationLogicFile := filepath.Join(tempDir, "src", "layout", "navigationLogic.ts")
	if err := os.MkdirAll(filepath.Dir(navigationLogicFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	navigationLogicSource := strings.Join([]string{
		"import { connect, kea, path } from 'kea'",
		"import { sceneLogic } from '../scenes/sceneLogic'",
		"",
		"export const navigationLogic = kea([",
		"    path(['layout', 'navigationLogic']),",
		"    connect(() => ({",
		"        values: [sceneLogic, ['sceneConfig']],",
		"    })),",
		"])",
	}, "\n")
	if err := os.WriteFile(navigationLogicFile, []byte(navigationLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	sceneReport, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       sceneLogicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error for sceneLogic: %v", err)
	}

	sceneLogics, err := BuildParsedLogics(sceneReport)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error for sceneLogic: %v", err)
	}
	if len(sceneLogics) != 1 {
		t.Fatalf("expected 1 scene logic, got %d", len(sceneLogics))
	}
	sceneLogic := sceneLogics[0]
	if selector, ok := findParsedField(sceneLogic.Selectors, "sceneConfig"); !ok || selector.Type != "SceneConfig | null" {
		t.Fatalf("expected sceneConfig selector type %q, got %+v", "SceneConfig | null", sceneLogic.Selectors)
	}

	navigationReport, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       navigationLogicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error for navigationLogic: %v", err)
	}

	navigationLogics, err := BuildParsedLogics(navigationReport)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error for navigationLogic: %v", err)
	}
	if len(navigationLogics) != 1 {
		t.Fatalf("expected 1 navigation logic, got %d", len(navigationLogics))
	}
	navigationLogic := navigationLogics[0]
	if selector, ok := findParsedField(navigationLogic.Selectors, "sceneConfig"); !ok || selector.Type != "SceneConfig | null" {
		t.Fatalf("expected connected sceneConfig selector type %q, got %+v", "SceneConfig | null", navigationLogic.Selectors)
	}
	if !hasImport(navigationLogic.Imports, "../scenes/sceneLogic", "SceneConfig") {
		t.Fatalf("expected connected SceneConfig import from ../scenes/sceneLogic, got %+v", navigationLogic.Imports)
	}

	rendered := EmitTypegenAt(sceneLogics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"sceneConfig: (state: any, props?: any) => SceneConfig | null",
		"sceneConfig: SceneConfig | null",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered sceneLogic output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsMergesRepeatedBuilderSections(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path, reducers, selectors } from 'kea'",
		"",
		"export const sceneLogic = kea([",
		"    path(['scenes', 'sceneLogic']),",
		"    reducers({",
		"        tabs: [[] as string[], {}],",
		"    }),",
		"    reducers({",
		"        homepage: [null as string | null, {}],",
		"    }),",
		"    selectors({",
		"        activeTab: [",
		"            (s) => [s.tabs],",
		"            (tabs: string[]): string | null => tabs[0] ?? null,",
		"        ],",
		"    }),",
		"    selectors({",
		"        activeTabId: [",
		"            (s) => [s.homepage],",
		"            (homepage: string | null): string | null => homepage,",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/sceneLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "sceneLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "tabs", TypeString: "[string[], {}]"},
						},
					},
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "homepage", TypeString: "[string | null, {}]"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "activeTab",
								TypeString:       "(((s: any) => any[]) | ((tabs: string[]) => string | null))[]",
								ReturnTypeString: "string | null",
							},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "activeTabId",
								TypeString:       "(((s: any) => any[]) | ((homepage: string | null) => string | null))[]",
								ReturnTypeString: "string | null",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if field, ok := findParsedField(logic.Reducers, "tabs"); !ok || field.Type != "string[]" {
		t.Fatalf("expected tabs reducer type %q, got %+v", "string[]", logic.Reducers)
	}
	if field, ok := findParsedField(logic.Reducers, "homepage"); !ok || field.Type != "string | null" {
		t.Fatalf("expected homepage reducer type %q, got %+v", "string | null", logic.Reducers)
	}
	if field, ok := findParsedField(logic.Selectors, "activeTab"); !ok || field.Type != "string | null" {
		t.Fatalf("expected activeTab selector type %q, got %+v", "string | null", logic.Selectors)
	}
	if field, ok := findParsedField(logic.Selectors, "activeTabId"); !ok || field.Type != "string | null" {
		t.Fatalf("expected activeTabId selector type %q, got %+v", "string | null", logic.Selectors)
	}
}

func TestBuildParsedLogicsRecoversSelectorProjectorBeforeTrailingOptionsObject(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["src/**/*"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "src", "scenes", "logsLogic.ts")
	if err := os.MkdirAll(filepath.Dir(logicFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	logicSource := strings.Join([]string{
		"import { kea, path, reducers, selectors } from 'kea'",
		"",
		"type LogType = { ip?: string | null }",
		"",
		"export const logsLogic = kea([",
		"    path(['scenes', 'logsLogic']),",
		"    reducers({",
		"        logs: [[] as LogType[], {}],",
		"    }),",
		"    selectors({",
		"        ipAddresses: [",
		"            (s) => [s.logs],",
		"            (logs) => {",
		"                const ips = new Set<string>()",
		"                logs.forEach((log) => {",
		"                    if (log.ip) {",
		"                        ips.add(log.ip)",
		"                    }",
		"                })",
		"                return Array.from(ips).sort()",
		"            },",
		"            { resultEqualityCheck: (a, b) => JSON.stringify(a) === JSON.stringify(b) },",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if selector, ok := findParsedField(logic.Selectors, "ipAddresses"); !ok || selector.Type != "string[]" {
		t.Fatalf("expected ipAddresses selector type %q, got %+v", "string[]", logic.Selectors)
	}

	foundHelper := false
	for _, helper := range logic.InternalSelectorTypes {
		if helper.Name != "ipAddresses" {
			continue
		}
		foundHelper = true
		if helper.FunctionType != "(logs: LogType[]) => string[]" {
			t.Fatalf("expected ipAddresses internal selector type %q, got %+v", "(logs: LogType[]) => string[]", helper)
		}
	}
	if !foundHelper {
		t.Fatalf("expected ipAddresses internal selector helper, got %+v", logic.InternalSelectorTypes)
	}
}

func TestBuildParsedLogicsPrefersSourceActionPayloadTypesWhenTsgoLosesGenericWrappers(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path, actions } from 'kea'",
		"",
		"type QuickFilterOption = { label: string }",
		"",
		"export const quickFilterFormLogic = kea([",
		"    path(['lib', 'components', 'QuickFilters', 'quickFilterFormLogic']),",
		"    actions({",
		"        updateOption: (index: number, updates: Partial<QuickFilterOption>) => ({ index, updates }),",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/quickFilterFormLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "quickFilterFormLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{
								Name:             "updateOption",
								TypeString:       "(index: number, updates: Partial<QuickFilterOption>) => { index: number; updates: QuickFilterOption; }",
								ReturnTypeString: "{ index: number; updates: QuickFilterOption; }",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "updateOption")
	if !ok {
		t.Fatalf("expected updateOption action, got %+v", logics[0].Actions)
	}
	expected := "{ index: number; updates: Partial<QuickFilterOption>; }"
	if action.PayloadType != expected {
		t.Fatalf("expected payload type %q, got %q", expected, action.PayloadType)
	}
}

func TestBuildParsedLogicsPreservesOptionalActionPayloadMembers(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path, actions } from 'kea'",
		"",
		"type PropertyDefinitionType = 'event'",
		"",
		"export const propertyDefinitionsModel = kea([",
		"    path(['models', 'propertyDefinitionsModel']),",
		"    actions({",
		"        loadPropertyValues: (payload: {",
		"            endpoint: string | undefined",
		"            type: PropertyDefinitionType",
		"            newInput: string | undefined",
		"            propertyKey: string",
		"            eventNames?: string[]",
		"            properties?: { key: string; values: string | string[] }[]",
		"            forceRefresh?: boolean",
		"        }) => payload,",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/propertyDefinitionsModel.ts",
		Logics: []LogicReport{
			{
				Name:      "propertyDefinitionsModel",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{
								Name:             "loadPropertyValues",
								TypeString:       "(payload: { endpoint: string | undefined; type: PropertyDefinitionType; newInput: string | undefined; propertyKey: string; eventNames?: string[]; properties?: { key: string; values: string | string[]; }[]; forceRefresh?: boolean; }) => { endpoint: string | undefined; eventNames: string[]; forceRefresh: boolean; newInput: string | undefined; properties: { key: string; values: string | string[]; }[]; propertyKey: string; type: PropertyDefinitionType; }",
								ReturnTypeString: "{ endpoint: string | undefined; eventNames: string[]; forceRefresh: boolean; newInput: string | undefined; properties: { key: string; values: string | string[]; }[]; propertyKey: string; type: PropertyDefinitionType; }",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "loadPropertyValues")
	if !ok {
		t.Fatalf("expected loadPropertyValues action, got %+v", logics[0].Actions)
	}
	expected := "{ endpoint: string | undefined; eventNames?: string[]; forceRefresh?: boolean; newInput: string | undefined; properties?: { key: string; values: string | string[]; }[]; propertyKey: string; type: PropertyDefinitionType; }"
	if action.PayloadType != expected {
		t.Fatalf("expected payload type %q, got %q", expected, action.PayloadType)
	}
}

func TestBuildParsedLogicsPreservesExplicitAnyArraySelectorReturnType(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path, selectors } from 'kea'",
		"",
		"export const quickFilterFormLogic = kea([",
		"    path(['lib', 'components', 'QuickFilters', 'quickFilterFormLogic']),",
		"    selectors({",
		"        suggestions: [",
		"            (s) => [s.propertyOptions],",
		"            (propertyOptions): any[] => {",
		"                return Object.values(propertyOptions || {})",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/quickFilterFormLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "quickFilterFormLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "suggestions",
								TypeString:       "(((s: any) => any[]) | ((propertyOptions: any) => any))[]",
								ReturnTypeString: "any",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	selector, ok := findParsedField(logics[0].Selectors, "suggestions")
	if !ok {
		t.Fatalf("expected suggestions selector, got %+v", logics[0].Selectors)
	}
	if selector.Type != "any[]" {
		t.Fatalf("expected suggestions selector type %q, got %q", "any[]", selector.Type)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 12, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(rendered, "suggestions: (state: any, props?: any) => any[]") {
		t.Fatalf("expected rendered selector to preserve any[] return:\n%s", rendered)
	}
}

func TestBuildParsedLogicsRefinesLoaderSuccessPayloadFromBaseAction(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"",
		"export const featurePreviewsLogic = kea([",
		"    path(['layout', 'FeaturePreviews', 'featurePreviewsLogic']),",
		"    actions({",
		"        submitEarlyAccessFeatureFeedback: (message: string) => ({ message }),",
		"    }),",
		"    loaders({",
		"        activeFeedbackFlagKey: [",
		"            null as string | null,",
		"            {",
		"                submitEarlyAccessFeatureFeedback: async ({ message }) => {",
		"                    console.log(message)",
		"                    return null",
		"                },",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/featurePreviewsLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "featurePreviewsLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{
								Name:             "submitEarlyAccessFeatureFeedback",
								TypeString:       "(message: string) => { message: string; }",
								ReturnTypeString: "{ message: string; }",
							},
						},
					},
					{
						Name: "loaders",
						Members: []MemberReport{
							{Name: "activeFeedbackFlagKey", TypeString: "any[]"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "submitEarlyAccessFeatureFeedbackSuccess")
	if !ok {
		t.Fatalf("expected submitEarlyAccessFeatureFeedbackSuccess action, got %+v", logics[0].Actions)
	}
	expectedFunctionType := "(activeFeedbackFlagKey: null, payload?: { message: string; }) => { activeFeedbackFlagKey: null; payload?: { message: string; } }"
	if action.FunctionType != expectedFunctionType {
		t.Fatalf("expected success action function type %q, got %q", expectedFunctionType, action.FunctionType)
	}
	expectedPayloadType := "{ activeFeedbackFlagKey: null; payload?: { message: string; } }"
	if action.PayloadType != expectedPayloadType {
		t.Fatalf("expected success action payload type %q, got %q", expectedPayloadType, action.PayloadType)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 12, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"submitEarlyAccessFeatureFeedbackSuccess: (activeFeedbackFlagKey: null, payload?: { message: string; }) => {",
		"payload: { activeFeedbackFlagKey: null; payload?: { message: string; } }",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestMergePreferredActionPayloadTypeMergesMemberLevelImprovements(t *testing.T) {
	current := "{ exportedScene: SceneExport | undefined; scrollToTop: boolean; }"
	candidate := "{ exportedScene: SceneExport<SceneProps> | undefined; scrollToTop: boolean | undefined; }"

	merged := mergePreferredActionPayloadTypePreservingMembers(current, candidate, map[string]bool{"scrollToTop": true})
	expected := "{ exportedScene: SceneExport<SceneProps> | undefined; scrollToTop: boolean; }"
	if merged != expected {
		t.Fatalf("expected merged payload type %q, got %q", expected, merged)
	}
}

func TestMergePreferredActionPayloadTypePreservesOptionalMembers(t *testing.T) {
	current := "{ endpoint: string | undefined; eventNames: string[]; forceRefresh: boolean; newInput: string | undefined; properties: { key: string; values: string | string[]; }[]; propertyKey: string; type: PropertyDefinitionType; }"
	candidate := "{ endpoint: string | undefined; eventNames?: string[]; forceRefresh?: boolean; newInput: string | undefined; properties?: { key: string; values: string | string[]; }[]; propertyKey: string; type: PropertyDefinitionType; }"

	merged := mergePreferredActionPayloadType(current, candidate)
	expected := "{ endpoint: string | undefined; eventNames?: string[]; forceRefresh?: boolean; newInput: string | undefined; properties?: { key: string; values: string | string[]; }[]; propertyKey: string; type: PropertyDefinitionType; }"
	if merged != expected {
		t.Fatalf("expected merged payload type %q, got %q", expected, merged)
	}
}

func TestMergeParsedActionsPreferLoaderRecoveryPreservesReportedConcreteAnyArrays(t *testing.T) {
	current := ParsedAction{
		Name:         "addMembersToRoleSuccess",
		FunctionType: "(roles: any[], payload?: { members: string[]; role: RoleType; }) => { roles: any[]; payload?: { members: string[]; role: RoleType; } }",
		PayloadType:  "{ roles: any[]; payload?: { members: string[]; role: RoleType; } }",
	}
	candidate := ParsedAction{
		Name:         "addMembersToRoleSuccess",
		FunctionType: "(roles: RoleType[], payload?: { members: string[]; role: RoleType; }) => { roles: RoleType[]; payload?: { members: string[]; role: RoleType; } }",
		PayloadType:  "{ roles: RoleType[]; payload?: { members: string[]; role: RoleType; } }",
	}

	merged := mergeParsedActionsPreferLoaderRecovery([]ParsedAction{current}, candidate)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged action, got %+v", merged)
	}
	if merged[0].FunctionType != current.FunctionType || merged[0].PayloadType != current.PayloadType {
		t.Fatalf("expected reported loader action to win when source only specializes any arrays, got %+v", merged[0])
	}
}

func TestMergeSourceLoaderPropertyTypePreservesReportedAnyArrayReturns(t *testing.T) {
	current := "({ role, members }: { members: any; role: any; }) => Promise<any[]>"
	source := "({ role, members }: { role: RoleType; members: string[]; }) => Promise<RoleType[]>"

	merged := mergeSourceLoaderPropertyType(current, source)
	expected := "({ role, members }: { role: RoleType; members: string[]; }) => Promise<any[]>"
	if merged != expected {
		t.Fatalf("expected merged loader property type %q, got %q", expected, merged)
	}
}

func TestMergeSourceLoaderPropertyTypePrefersOpaqueAnyArraysOverTruncatedReportedReturns(t *testing.T) {
	current := "({ id }: { ...; }) => Promise<...>"
	source := "({ id }: { id: string; }) => Promise<any[]>"

	merged := mergeSourceLoaderPropertyType(current, source)
	expected := "({ id }: { id: string; }) => Promise<any[]>"
	if merged != expected {
		t.Fatalf("expected merged loader property type %q, got %q", expected, merged)
	}
}

func TestMergeSourceLoaderPropertyTypePrefersOpaqueAnyArraysOverReportedAnyReturns(t *testing.T) {
	current := "({ id }: any) => Promise<any>"
	source := "({ id }) => Promise<any[]>"

	merged := mergeSourceLoaderPropertyType(current, source)
	expected := "({ id }: any) => Promise<any[]>"
	if merged != expected {
		t.Fatalf("expected merged loader property type %q, got %q", expected, merged)
	}
}

func TestBuildParsedLogicsFromSourcePreservesReportedAnyArrayLoaderSuccesses(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, loaders, path } from 'kea'",
		"",
		"type RoleType = {",
		"    id: string",
		"    members: string[]",
		"}",
		"",
		"export const roleLogic = kea([",
		"    path(['roleLogic']),",
		"    loaders(({ values }) => ({",
		"        roles: [",
		"            [] as RoleType[],",
		"            {",
		"                loadRoles: async () => {",
		"                    return values.roles || []",
		"                },",
		"                addMembersToRole: async ({ role, members }: { role: RoleType; members: string[] }) => {",
		"                    if (!values.roles) {",
		"                        return []",
		"                    }",
		"                    role.members = [...role.members, ...members]",
		"                    return [...values.roles]",
		"                },",
		"                deleteRole: async ({ roleId }: { roleId: string }) => {",
		"                    return values.roles?.filter((role) => role.id !== roleId) || []",
		"                },",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/roleLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "roleLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "loaders",
						Members: []MemberReport{
							{
								Name:       "roles",
								TypeString: "(RoleType[] | { loadRoles: () => Promise<any>; addMembersToRole: ({ role, members }: { members: any; role: any; }) => Promise<any[]>; deleteRole: ({ roleId }: { roleId: string; }) => Promise<RoleType[]>; })[]",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	addMembersToRole, ok := findParsedAction(logics[0].Actions, "addMembersToRole")
	if !ok {
		t.Fatalf("expected addMembersToRole action, got %+v", logics[0].Actions)
	}
	if addMembersToRole.PayloadType != "{ role: RoleType; members: string[]; }" {
		t.Fatalf("expected refined addMembersToRole payload, got %+v", addMembersToRole)
	}

	addMembersToRoleSuccess, ok := findParsedAction(logics[0].Actions, "addMembersToRoleSuccess")
	if !ok {
		t.Fatalf("expected addMembersToRoleSuccess action, got %+v", logics[0].Actions)
	}
	expectedAnyArraySuccess := "{ roles: any[]; payload?: { role: RoleType; members: string[]; } }"
	if addMembersToRoleSuccess.PayloadType != expectedAnyArraySuccess {
		t.Fatalf("expected addMembersToRoleSuccess payload %q, got %+v", expectedAnyArraySuccess, addMembersToRoleSuccess)
	}

	deleteRoleSuccess, ok := findParsedAction(logics[0].Actions, "deleteRoleSuccess")
	if !ok {
		t.Fatalf("expected deleteRoleSuccess action, got %+v", logics[0].Actions)
	}
	expectedTypedSuccess := "{ roles: RoleType[]; payload?: { roleId: string; } }"
	if deleteRoleSuccess.PayloadType != expectedTypedSuccess {
		t.Fatalf("expected deleteRoleSuccess payload %q, got %+v", expectedTypedSuccess, deleteRoleSuccess)
	}
}

func TestBuildParsedLogicsFromSourcePrefersSpreadAnyArrayLoaderSuccessesOverTypedReportedArrays(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, loaders, path } from 'kea'",
		"",
		"type HogFlow = {",
		"    id: string",
		"    name: string",
		"}",
		"",
		"export const workflowsLogic = kea([",
		"    path(['workflowsLogic']),",
		"    loaders(({ values }) => ({",
		"        workflows: [",
		"            [] as HogFlow[],",
		"            {",
		"                duplicateWorkflow: async (workflow: HogFlow) => {",
		"                    const duplicatedWorkflow = { ...workflow, id: 'copy' }",
		"                    return [duplicatedWorkflow, ...values.workflows]",
		"                },",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/workflowsLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "workflowsLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "loaders",
						Members: []MemberReport{
							{
								Name:       "workflows",
								TypeString: "(HogFlow[] | { duplicateWorkflow: (workflow: HogFlow) => Promise<HogFlow[]>; })[]",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	duplicateWorkflowSuccess, ok := findParsedAction(logics[0].Actions, "duplicateWorkflowSuccess")
	if !ok {
		t.Fatalf("expected duplicateWorkflowSuccess action, got %+v", logics[0].Actions)
	}
	expectedSuccess := "{ workflows: any[]; payload?: HogFlow }"
	if duplicateWorkflowSuccess.PayloadType != expectedSuccess {
		t.Fatalf("expected duplicateWorkflowSuccess payload %q, got %+v", expectedSuccess, duplicateWorkflowSuccess)
	}
}

func TestBuildParsedLogicsPreservesOpaqueLoaderMapSuccessesFromSignatureProbe(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, loaders, path } from 'kea'",
		"",
		"type RepositoryType = {",
		"    id: string",
		"}",
		"",
		"type ApiResponse = {",
		"    ok: boolean",
		"    json: () => Promise<any>",
		"}",
		"",
		"declare function apiFetch(url: string, options?: any): Promise<ApiResponse>",
		"",
		"export const repositoriesLogic = kea([",
		"    path(['repositoriesLogic']),",
		"    loaders(({ values }) => ({",
		"        repositories: [",
		"            [] as RepositoryType[],",
		"            {",
		"                removeRepository: async ({ id }: { id: string }) => {",
		"                    return values.repositories.filter((repository) => repository.id !== id)",
		"                },",
		"                refreshRepository: async ({ id }: { id: string }) => {",
		"                    try {",
		"                        const response = await apiFetch(`/api/repositories/${id}`, {",
		"                            method: 'PATCH',",
		"                        })",
		"                        if (!response.ok) {",
		"                            throw new Error('Failed to refresh repository')",
		"                        }",
		"                        const data = await response.json()",
		"                        return values.repositories.map((repository) => (repository.id === id ? data : repository))",
		"                    } catch (error) {",
		"                        console.error(error)",
		"                        return values.repositories",
		"                    }",
		"                },",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")
	files := map[string]string{"src/repositoriesLogic.ts": source}
	logics := inspectTempLogicFile(t, files, "src/repositoriesLogic.ts")
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	tempDir, tsconfigPath := writeTempProject(t, files)
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	loadersProperty := mustFindLogicProperty(t, sourceLogics[0], "loaders")
	repositoriesProperty, ok := sectionSourceProperties(source, loadersProperty)["repositories"]
	if !ok {
		t.Fatalf("expected repositories loader property inside loaders section")
	}
	root := repoRoot(t)
	state := &buildState{
		binaryPath: tsgoapi.PreferredBinary(root),
		projectDir: tempDir,
		configFile: tsconfigPath,
		timeout:    15 * time.Second,
	}
	t.Cleanup(state.close)
	_, sourcePropertiesWithState, ok := sourceLoaderMemberTypeFromProperty(
		source,
		repositoriesProperty,
		filepath.Join(tempDir, "src", "repositoriesLogic.ts"),
		state,
	)
	if !ok {
		t.Fatalf("expected state-backed source loader recovery to succeed")
	}
	if !strings.HasSuffix(sourcePropertiesWithState["refreshRepository"], "=> Promise<any[]>") {
		t.Fatalf("expected state-backed source loader property type to widen to any[]; got %+v", sourcePropertiesWithState)
	}

	removeRepositorySuccess, ok := findParsedAction(logics[0].Actions, "removeRepositorySuccess")
	if !ok {
		t.Fatalf("expected removeRepositorySuccess action, got %+v", logics[0].Actions)
	}
	if removeRepositorySuccess.PayloadType != "{ repositories: RepositoryType[]; payload?: { id: string; } }" {
		t.Fatalf("expected typed removeRepositorySuccess payload, got %+v", removeRepositorySuccess)
	}

	refreshRepositorySuccess, ok := findParsedAction(logics[0].Actions, "refreshRepositorySuccess")
	if !ok {
		t.Fatalf("expected refreshRepositorySuccess action, got %+v", logics[0].Actions)
	}
	expectedSuccess := "{ repositories: any[]; payload?: { id: string; } }"
	if refreshRepositorySuccess.PayloadType != expectedSuccess {
		t.Fatalf("expected refreshRepositorySuccess payload %q, got %+v", expectedSuccess, refreshRepositorySuccess)
	}
}

func TestBuildParsedLogicsPreservesFrameOSStyleOpaqueLoaderMapSuccessesWithExternalTypes(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "jsx": "react-jsx",`,
		`    "lib": ["DOM", "ES2020"],`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["src/**/*"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	files := map[string]string{
		"src/types.ts": strings.Join([]string{
			"export interface RepositoryType {",
			"    id: string",
			"}",
		}, "\n"),
		"src/utils/apiFetch.ts": strings.Join([]string{
			"export async function apiFetch(_input: RequestInfo | URL, _options: RequestInit = {}): Promise<Response> {",
			"    throw new Error('not implemented')",
			"}",
		}, "\n"),
		"src/utils/frameControlMode.ts": strings.Join([]string{
			"export function isFrameControlMode(): boolean {",
			"    return false",
			"}",
		}, "\n"),
		"src/models/repositoriesModel.tsx": strings.Join([]string{
			"import { actions, afterMount, kea, path } from 'kea'",
			"",
			"import type { repositoriesModelType } from './repositoriesModelType'",
			"import { loaders } from 'kea-loaders'",
			"import { RepositoryType } from '../types'",
			"import { apiFetch } from '../utils/apiFetch'",
			"import { isFrameControlMode } from '../utils/frameControlMode'",
			"",
			"export const repositoriesModel = kea<repositoriesModelType>([",
			"  path(['src', 'models', 'repositoriesModel']),",
			"  actions({",
			"    updateRepository: (repository: RepositoryType) => ({ repository }),",
			"    removeRepository: (id: string) => ({ id }),",
			"    refreshRepository: (id: string) => ({ id }),",
			"  }),",
			"  loaders(({ values }) => ({",
			"    repositories: [",
			"      [] as RepositoryType[],",
			"      {",
			"        loadRepositories: async () => {",
			"          try {",
			"            const systemResponse = await apiFetch('/api/repositories/system')",
			"            if (!systemResponse.ok) {",
			"              throw new Error('Failed to fetch system repositories')",
			"            }",
			"            const systemData = await systemResponse.json()",
			"            const response = await apiFetch('/api/repositories')",
			"            if (!response.ok) {",
			"              throw new Error('Failed to fetch repositories')",
			"            }",
			"            const data = await response.json()",
			"            return [...systemData, ...data] as RepositoryType[]",
			"          } catch (error) {",
			"            console.error(error)",
			"            return values.repositories",
			"          }",
			"        },",
			"        removeRepository: async ({ id }) => {",
			"          try {",
			"            const response = await apiFetch(`/api/repositories/${id}`, { method: 'DELETE' })",
			"            if (!response.ok) {",
			"              throw new Error('Failed to remove repository')",
			"            }",
			"            return values.repositories.filter((t) => t.id !== id)",
			"          } catch (error) {",
			"            console.error(error)",
			"            return values.repositories",
			"          }",
			"        },",
			"        refreshRepository: async ({ id }) => {",
			"          try {",
			"            const response = await apiFetch(`/api/repositories/${id}`, {",
			"              method: 'PATCH',",
			"              body: '{}',",
			"              headers: {",
			"                'Content-Type': 'application/json',",
			"              },",
			"            })",
			"            if (!response.ok) {",
			"              throw new Error('Failed to refresh repository')",
			"            }",
			"            const data = await response.json()",
			"            return values.repositories.map((r) => (r.id === id ? data : r))",
			"          } catch (error) {",
			"            console.error(error)",
			"            return values.repositories",
			"          }",
			"        },",
			"      },",
			"    ],",
			"  })),",
			"  afterMount(({ actions }) => {",
			"    if (isFrameControlMode()) {",
			"      return",
			"    }",
			"    actions.loadRepositories()",
			"  }),",
			"])",
		}, "\n"),
	}
	for relativePath, contents := range files {
		fullPath := filepath.Join(tempDir, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("MkdirAll returned error: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       filepath.Join(tempDir, "src", "models", "repositoriesModel.tsx"),
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	source := files["src/models/repositoriesModel.tsx"]
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	loadersProperty := mustFindLogicProperty(t, sourceLogics[0], "loaders")
	repositoriesProperty, ok := sectionSourceProperties(source, loadersProperty)["repositories"]
	if !ok {
		t.Fatalf("expected repositories loader property inside loaders section")
	}
	state := &buildState{
		binaryPath: tsgoapi.PreferredBinary(root),
		projectDir: tempDir,
		configFile: tsconfigPath,
		timeout:    15 * time.Second,
	}
	t.Cleanup(state.close)
	_, sourcePropertiesWithState, ok := sourceLoaderMemberTypeFromProperty(
		source,
		repositoriesProperty,
		filepath.Join(tempDir, "src", "models", "repositoriesModel.tsx"),
		state,
	)
	if !ok {
		t.Fatalf("expected state-backed source loader recovery to succeed")
	}
	if !strings.HasSuffix(sourcePropertiesWithState["refreshRepository"], "=> Promise<any[]>") {
		t.Fatalf("expected state-backed source loader property type to widen to any[]; got %+v", sourcePropertiesWithState)
	}
	var loaderSections []SectionReport
	for _, section := range report.Logics[0].Sections {
		if section.Name == "loaders" {
			loaderSections = append(loaderSections, section)
		}
	}
	loadersSection, ok := lastSectionReport(loaderSections)
	if !ok {
		t.Fatalf("expected loaders section in report, got %+v", report.Logics[0].Sections)
	}
	reportDefaultType := ""
	reportProperties := map[string]string{}
	if len(loadersSection.Members) != 0 {
		if parsedDefault, parsedProperties, parsed := parseLoaderMemberType(loadersSection.Members[0].TypeString); parsed {
			reportDefaultType = parsedDefault
			reportProperties = parsedProperties
		}
	}
	loaderActions, _ := parseLoadersWithSource(
		loadersSection,
		source,
		loadersProperty,
		filepath.Join(tempDir, "src", "models", "repositoriesModel.tsx"),
		state,
	)
	directRefreshRepositorySuccess, ok := findParsedAction(loaderActions, "refreshRepositorySuccess")
	if !ok {
		t.Fatalf("expected direct refreshRepositorySuccess loader action, got %+v", loaderActions)
	}
	if directRefreshRepositorySuccess.PayloadType != "{ repositories: any[]; payload?: any }" {
		t.Fatalf("expected direct loader parsing to keep opaque refreshRepositorySuccess payload, got %+v with report default %q report properties %+v and source properties %+v", directRefreshRepositorySuccess, reportDefaultType, reportProperties, sourcePropertiesWithState)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	removeRepositorySuccess, ok := findParsedAction(logics[0].Actions, "removeRepositorySuccess")
	if !ok {
		t.Fatalf("expected removeRepositorySuccess action, got %+v", logics[0].Actions)
	}
	if removeRepositorySuccess.PayloadType != "{ repositories: RepositoryType[]; payload?: { id: string; } }" {
		t.Fatalf("expected typed removeRepositorySuccess payload, got %+v", removeRepositorySuccess)
	}

	refreshRepositorySuccess, ok := findParsedAction(logics[0].Actions, "refreshRepositorySuccess")
	if !ok {
		t.Fatalf("expected refreshRepositorySuccess action, got %+v", logics[0].Actions)
	}
	if refreshRepositorySuccess.PayloadType != "{ repositories: any[]; payload?: { id: string; } }" {
		t.Fatalf("expected opaque refreshRepositorySuccess payload, got %+v with source properties %+v", refreshRepositorySuccess, sourcePropertiesWithState)
	}
}

func TestRefineSourceLoaderFunctionTypeWidensSpreadCopiesToAnyArrays(t *testing.T) {
	functionType := "({ role, members }: { role: RoleType; members: string[]; }) => Promise<RoleType[]>"
	expression := strings.Join([]string{
		"async ({ role, members }: { role: RoleType; members: string[] }) => {",
		"    if (!values.roles) {",
		"        return []",
		"    }",
		"    role.members = [...role.members, ...members]",
		"    return [...values.roles]",
		"}",
	}, "\n")

	refined := refineSourceLoaderFunctionType(functionType, expression, "RoleType[]")
	expected := "({ role, members }: { role: RoleType; members: string[]; }) => Promise<any[]>"
	if refined != expected {
		t.Fatalf("expected refined loader function type %q, got %q", expected, refined)
	}
}

func TestRefineSourceLoaderFunctionTypeWidensPrependedSpreadCopiesToAnyArrays(t *testing.T) {
	functionType := "(workflow: HogFlow) => Promise<HogFlow[]>"
	expression := strings.Join([]string{
		"async (workflow: HogFlow) => {",
		"    const duplicatedWorkflow = { ...workflow, id: 'copy' }",
		"    return [duplicatedWorkflow, ...values.workflows]",
		"}",
	}, "\n")

	refined := refineSourceLoaderFunctionType(functionType, expression, "HogFlow[]")
	expected := "(workflow: HogFlow) => Promise<any[]>"
	if refined != expected {
		t.Fatalf("expected refined loader function type %q, got %q", expected, refined)
	}
}

func TestRefineSourceLoaderFunctionTypeWidensMappedAwaitedJSONValuesToAnyArrays(t *testing.T) {
	functionType := "({ id }: { id: string; }) => Promise<RepositoryType[]>"
	expression := strings.Join([]string{
		"async ({ id }: { id: string }) => {",
		"    const response = await apiFetch(`/api/repositories/${id}`)",
		"    const data = await response.json()",
		"    return values.repositories.map((repository) => (repository.id === id ? data : repository))",
		"}",
	}, "\n")

	refined := refineSourceLoaderFunctionType(functionType, expression, "RepositoryType[]")
	expected := "({ id }: { id: string; }) => Promise<any[]>"
	if refined != expected {
		t.Fatalf("expected refined loader function type %q, got %q", expected, refined)
	}
}

func TestRefineSourceLoaderFunctionTypePrefersDefaultTypesForDirectAwaitedJSONReturns(t *testing.T) {
	functionType := "() => Promise<Response>"
	expression := strings.Join([]string{
		"async () => {",
		"    const response = await apiFetch('/api/ai/embeddings/status')",
		"    if (!response.ok) {",
		"        throw new Error('Failed to load embeddings status')",
		"    }",
		"    return await response.json()",
		"}",
	}, "\n")

	refined := refineSourceLoaderFunctionType(functionType, expression, "{ count: number; total: number }")
	expected := "() => Promise<{ count: number; total: number; }>"
	if refined != expected {
		t.Fatalf("expected refined loader function type %q, got %q", expected, refined)
	}
}

func TestSourceLoaderMemberTypeFromPropertyWidensMappedAwaitedJSONValuesToAnyArrays(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, loaders, path } from 'kea'",
		"",
		"type RepositoryType = {",
		"    id: string",
		"}",
		"",
		"export const repositoriesLogic = kea([",
		"    path(['repositoriesLogic']),",
		"    loaders(({ values }) => ({",
		"        repositories: [",
		"            [] as RepositoryType[],",
		"            {",
		"                refreshRepository: async ({ id }: { id: string }) => {",
		"                    const response = await apiFetch(`/api/repositories/${id}`)",
		"                    const data = await response.json()",
		"                    return values.repositories.map((repository) => (repository.id === id ? data : repository))",
		"                },",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")

	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	if len(sourceLogics) != 1 {
		t.Fatalf("expected 1 source logic, got %d", len(sourceLogics))
	}
	loadersProperty := mustFindLogicProperty(t, sourceLogics[0], "loaders")
	repositoriesProperty, ok := sectionSourceProperties(source, loadersProperty)["repositories"]
	if !ok {
		t.Fatalf("expected repositories loader property inside loaders section")
	}

	defaultType, properties, ok := sourceLoaderMemberTypeFromProperty(source, repositoriesProperty, "/tmp/repositoriesLogic.ts", nil)
	if !ok {
		t.Fatalf("expected source loader member type recovery to succeed")
	}
	if defaultType != "RepositoryType[]" {
		t.Fatalf("expected loader default type %q, got %q", "RepositoryType[]", defaultType)
	}
	if properties["refreshRepository"] != "({ id }: { id: string; }) => Promise<any[]>" {
		t.Fatalf("expected refreshRepository source property type to widen to any[]; got %+v", properties)
	}
}

func TestBuildParsedLogicsPrefersDefaultLoaderSuccessTypesForDirectAwaitedJSONReturns(t *testing.T) {
	logics := inspectTempLogicFile(t, map[string]string{
		"src/settingsLogic.ts": strings.Join([]string{
			"import { kea, path } from 'kea'",
			"import { loaders } from 'kea-loaders'",
			"",
			"type EmbeddingsStatus = {",
			"    count: number",
			"    total: number",
			"}",
			"",
			"interface Response {",
			"    ok: boolean",
			"    json: () => Promise<any>",
			"}",
			"",
			"declare function apiFetch(url: string, options?: any): Promise<Response>",
			"",
			"export const settingsLogic = kea([",
			"    path(['settingsLogic']),",
			"    loaders(() => ({",
			"        aiEmbeddingsStatus: [",
			"            { count: 0, total: 0 } as EmbeddingsStatus,",
			"            {",
			"                generateMissingAiEmbeddings: async () => {",
			"                    const response = await apiFetch('/api/ai/embeddings/generate-missing', {",
			"                        method: 'POST',",
			"                    })",
			"                    if (!response.ok) {",
			"                        throw new Error('Failed to generate embeddings')",
			"                    }",
			"                    return await response.json()",
			"                },",
			"            },",
			"        ],",
			"    })),",
			"])",
		}, "\n"),
	}, "src/settingsLogic.ts")
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "generateMissingAiEmbeddingsSuccess")
	if !ok {
		t.Fatalf("expected generateMissingAiEmbeddingsSuccess action, got %+v", logics[0].Actions)
	}
	expectedFunctionType := "(aiEmbeddingsStatus: EmbeddingsStatus, payload?: any) => { aiEmbeddingsStatus: EmbeddingsStatus; payload?: any }"
	if action.FunctionType != expectedFunctionType {
		t.Fatalf("expected generateMissingAiEmbeddingsSuccess function type %q, got %+v", expectedFunctionType, action)
	}
}

func TestBuildParsedLogicsFromSourcePrefersRecoveredPrimitiveShorthandPayloadMembers(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"",
		"type RoleType = {",
		"    id: string",
		"}",
		"",
		"export const demoLogic = kea([",
		"    path(['demoLogic']),",
		"    actions({",
		"        deleteRole: (roleId: RoleType['id']) => ({ roleId }),",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/demoLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "demoLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{Name: "deleteRole", TypeString: "(roleId: string) => { roleId: RoleType; }"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "deleteRole")
	if !ok {
		t.Fatalf("expected deleteRole action, got %+v", logics[0].Actions)
	}
	expectedPayload := "{ roleId: string; }"
	if action.PayloadType != expectedPayload {
		t.Fatalf("expected recovered deleteRole payload %q, got %+v", expectedPayload, action)
	}
	expectedFunctionType := "(roleId: RoleType['id']) => { roleId: string; }"
	if action.FunctionType != expectedFunctionType {
		t.Fatalf("expected recovered deleteRole function type %q, got %+v", expectedFunctionType, action)
	}
}

func TestPreferredLoaderSuccessTypeFallsBackToDefaultForUnusableInferredTypes(t *testing.T) {
	defaultType := "{ system_status_ok: boolean; async_migrations_ok: boolean; }"

	cases := []struct {
		name       string
		returnType string
		expected   string
	}{
		{
			name:       "bare promise",
			returnType: "Promise",
			expected:   "EarlyAccessFeature[]",
		},
		{
			name:       "quoted api path",
			returnType: `Promise<"api/instance_settings">`,
			expected:   defaultType,
		},
		{
			name:       "broad primitive string",
			returnType: "Promise<string>",
			expected:   defaultType,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fallback := defaultType
			if tc.name == "bare promise" {
				fallback = "EarlyAccessFeature[]"
			}
			if actual := preferredLoaderSuccessType(tc.returnType, fallback); actual != tc.expected {
				t.Fatalf("expected loader success type %q, got %q", tc.expected, actual)
			}
		})
	}
}

func TestBuildParsedLogicsFallsBackToLoaderDefaultTypeForBroadPrimitiveReturn(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"",
		"type DebugResponse = {",
		"    queries: string[]",
		"}",
		"",
		"export const debugLogic = kea([",
		"    path(['debugLogic']),",
		"    loaders(() => ({",
		"        debugResponse: [",
		"            {} as DebugResponse,",
		"            {",
		"                loadDebugResponse: async (): Promise<string> => 'api/debug_ch_queries',",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/debugLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "debugLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "loaders",
						Members: []MemberReport{
							{Name: "debugResponse", TypeString: "any[]"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if action, ok := findParsedAction(logic.Actions, "loadDebugResponseSuccess"); !ok || action.FunctionType != "(debugResponse: DebugResponse, payload?: any) => { debugResponse: DebugResponse; payload?: any }" {
		t.Fatalf("expected loadDebugResponseSuccess to fall back to the loader default type, got %+v", action)
	}
}

func TestBuildParsedLogicsRecoversTypedIdentifierActionPayloads(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"",
		"type AccessControlLevel = 'none' | 'viewer'",
		"type OrganizationMemberType = { id: string }",
		"",
		"export const demoLogic = kea([",
		"    path(['demoLogic']),",
		"    actions({",
		"        updateAccessControlMembers: (",
		"            accessControls: {",
		"                member: OrganizationMemberType['id']",
		"                level: AccessControlLevel | null",
		"            }[]",
		"        ) => ({ accessControls }),",
		"        saveGroupedRules: (params: {",
		"            projectLevel: AccessControlLevel | null",
		"            resourceLevels: Record<string, AccessControlLevel | null>",
		"        }) => params,",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/demoLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "demoLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{Name: "updateAccessControlMembers", TypeString: "(accessControls: any) => any"},
							{Name: "saveGroupedRules", TypeString: "(params: any) => any"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	updateMembers, ok := findParsedAction(logics[0].Actions, "updateAccessControlMembers")
	if !ok {
		t.Fatalf("expected updateAccessControlMembers action, got %+v", logics[0].Actions)
	}
	expectedMembersPayload := "{ accessControls: { member: string; level: AccessControlLevel | null; }[]; }"
	if updateMembers.PayloadType != expectedMembersPayload {
		t.Fatalf("expected updateAccessControlMembers payload %q, got %q", expectedMembersPayload, updateMembers.PayloadType)
	}
	if !strings.Contains(updateMembers.FunctionType, "(accessControls: { member: OrganizationMemberType['id']; level: AccessControlLevel | null; }[]) =>") {
		t.Fatalf("expected updateAccessControlMembers function type to preserve indexed access parameter type, got %q", updateMembers.FunctionType)
	}

	saveGroupedRules, ok := findParsedAction(logics[0].Actions, "saveGroupedRules")
	if !ok {
		t.Fatalf("expected saveGroupedRules action, got %+v", logics[0].Actions)
	}
	expectedSaveGroupedRulesPayload := "{ projectLevel: AccessControlLevel | null; resourceLevels: Record<string, AccessControlLevel | null>; }"
	if saveGroupedRules.PayloadType != expectedSaveGroupedRulesPayload {
		t.Fatalf("expected saveGroupedRules payload %q, got %q", expectedSaveGroupedRulesPayload, saveGroupedRules.PayloadType)
	}
	if !strings.Contains(saveGroupedRules.FunctionType, expectedSaveGroupedRulesPayload) {
		t.Fatalf("expected saveGroupedRules function type to contain %q, got %q", expectedSaveGroupedRulesPayload, saveGroupedRules.FunctionType)
	}
}

func TestRefineActionFunctionTypeRecoversLowQualityParameterTypesFromPayload(t *testing.T) {
	functionType := "(experimentId: ExperimentIdType, metric: any, teamId: number | null | undefined, queryId: string | null, context: { duration_ms: number; metric_index: number; is_primary: boolean; is_retry: boolean; refresh_id: string; ... 4 more ...; status_code: number | null; }) => { context: { duration_ms: number; metric_index: number; is_primary: boolean; is_retry: boolean; refresh_id: string; metric_kind: string; error_type: 'timeout' | 'out_of_memory' | 'server_error' | 'network_error' | 'unknown'; error_code: string | null; error_message: string | null; status_code: number | null; }; experimentId: ExperimentIdType; metric: ExperimentMetric | ExperimentTrendsQuery | ExperimentFunnelsQuery; queryId: string | null; teamId: number | null | undefined; }"
	payloadType := "{ context: { duration_ms: number; metric_index: number; is_primary: boolean; is_retry: boolean; refresh_id: string; metric_kind: string; error_type: 'timeout' | 'out_of_memory' | 'server_error' | 'network_error' | 'unknown'; error_code: string | null; error_message: string | null; status_code: number | null; }; experimentId: ExperimentIdType; metric: ExperimentMetric | ExperimentTrendsQuery | ExperimentFunnelsQuery; queryId: string | null; teamId: number | null | undefined; }"

	refined := refineActionFunctionType(functionType, payloadType)
	if strings.Contains(refined, "metric: any") {
		t.Fatalf("expected refined function type to replace metric: any, got %q", refined)
	}
	if strings.Contains(refined, "... 4 more ...") {
		t.Fatalf("expected refined function type to remove summarized object placeholder, got %q", refined)
	}
	if !strings.Contains(refined, "metric: ExperimentMetric | ExperimentTrendsQuery | ExperimentFunnelsQuery") {
		t.Fatalf("expected refined function type to recover metric parameter type, got %q", refined)
	}
	if !strings.Contains(refined, "context: { duration_ms: number; metric_index: number; is_primary: boolean; is_retry: boolean; refresh_id: string; metric_kind: string; error_type: 'timeout' | 'out_of_memory' | 'server_error' | 'network_error' | 'unknown'; error_code: string | null; error_message: string | null; status_code: number | null; }") {
		t.Fatalf("expected refined function type to expand context parameter type, got %q", refined)
	}
}

func TestRefineActionFunctionTypeKeepsSpecificParameterTypesWhenPayloadDiffers(t *testing.T) {
	functionType := "({ experimentId, holdoutId }: { experimentId: ExperimentIdType; holdoutId: ExperimentHoldoutType['id']; }) => { experimentId: ExperimentIdType; holdoutId: ExperimentHoldoutType; }"
	payloadType := "{ experimentId: ExperimentIdType; holdoutId: ExperimentHoldoutType; }"

	refined := refineActionFunctionType(functionType, payloadType)
	if refined != functionType {
		t.Fatalf("expected specific parameter types to stay unchanged, got %q", refined)
	}
}

func TestParameterTypeNeedsSourceRecoveryIgnoresQuotedUnknownLiteral(t *testing.T) {
	typeText := "{ error_type: 'timeout' | 'out_of_memory' | 'server_error' | 'network_error' | 'unknown'; }"
	if parameterTypeNeedsSourceRecovery(typeText) {
		t.Fatalf("expected quoted string literal 'unknown' to not mark parameter type as unresolved, got %q", typeText)
	}
}

func TestBuildParsedLogicsKeepsExplicitUnknownActionParameters(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"",
		"type SubstitutionChoice = { resourceId: string }",
		"",
		"export const demoLogic = kea([",
		"    path(['demoLogic']),",
		"    actions({",
		"        setDestinationTeamId: (teamId: unknown) => ({ teamId: teamId as number | null }),",
		"        setSubstitutionChoice: (resourceKey: string, choice: unknown) => ({",
		"            resourceKey,",
		"            choice: choice as SubstitutionChoice,",
		"        }),",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/demoLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "demoLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{Name: "setDestinationTeamId", TypeString: "(teamId: unknown) => { teamId: number | null; }", ReturnTypeString: "{ teamId: number | null; }"},
							{Name: "setSubstitutionChoice", TypeString: "(resourceKey: string, choice: unknown) => { resourceKey: string; choice: SubstitutionChoice; }", ReturnTypeString: "{ resourceKey: string; choice: SubstitutionChoice; }"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	setDestinationTeamID, ok := findParsedAction(logics[0].Actions, "setDestinationTeamId")
	if !ok {
		t.Fatalf("expected setDestinationTeamId action, got %+v", logics[0].Actions)
	}
	if !strings.Contains(setDestinationTeamID.FunctionType, "(teamId: unknown) =>") {
		t.Fatalf("expected explicit unknown teamId parameter to be preserved, got %q", setDestinationTeamID.FunctionType)
	}

	setSubstitutionChoice, ok := findParsedAction(logics[0].Actions, "setSubstitutionChoice")
	if !ok {
		t.Fatalf("expected setSubstitutionChoice action, got %+v", logics[0].Actions)
	}
	if !strings.Contains(setSubstitutionChoice.FunctionType, "(resourceKey: string, choice: unknown) =>") {
		t.Fatalf("expected explicit unknown choice parameter to be preserved, got %q", setSubstitutionChoice.FunctionType)
	}
}

func TestBuildParsedLogicsRefinesReportedUnknownActionParametersWhenSourceIsUntyped(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"",
		"export const sceneLogic = kea([",
		"    path(['sceneLogic']),",
		"    actions({",
		"        setScene: (scene, params) => ({ scene, params }),",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/sceneLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "sceneLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{
								Name:             "setScene",
								TypeString:       "(scene: unknown, params: unknown) => { scene: any; params: any; }",
								ReturnTypeString: "{ scene: any; params: any; }",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "setScene")
	if !ok {
		t.Fatalf("expected setScene action, got %+v", logics[0].Actions)
	}
	expected := "(scene: any, params: any) => { scene: any; params: any; }"
	if action.FunctionType != expected {
		t.Fatalf("expected recovered untyped action parameters %q, got %+v", expected, action)
	}
}

func TestBuildParsedLogicsRefinesDefaultedUnknownActionParametersWhenSourceIsUntyped(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"",
		"export const entityImagesModel = kea([",
		"    path(['entityImagesModel']),",
		"    actions({",
		"        updateEntityImage: (entity: string | null, subentity: string, force = true) => ({ entity, subentity, force }),",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/entityImagesModel.ts",
		Logics: []LogicReport{
			{
				Name:      "entityImagesModel",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{
								Name:             "updateEntityImage",
								TypeString:       "(entity: string | null, subentity: string, force?: unknown) => { entity: string | null; force: any; subentity: string; }",
								ReturnTypeString: "{ entity: string | null; force: any; subentity: string; }",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "updateEntityImage")
	if !ok {
		t.Fatalf("expected updateEntityImage action, got %+v", logics[0].Actions)
	}
	expected := "(entity: string | null, subentity: string, force?: any) => { entity: string | null; force: any; subentity: string; }"
	if action.FunctionType != expected {
		t.Fatalf("expected recovered defaulted action parameter type %q, got %+v", expected, action)
	}
}

func TestBuildParsedLogicsRecoversImportedIndexedAccessActionPayloads(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "baseUrl": "frontend",`,
		`    "paths": {`,
		`      "~/*": ["src/*"]`,
		`    },`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	typesFile := filepath.Join(tempDir, "frontend", "src", "types.ts")
	if err := os.MkdirAll(filepath.Dir(typesFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	typesSource := strings.Join([]string{
		"export type AccessControlLevel = 'none' | 'viewer'",
		"export interface BaseMemberType {",
		"    id: string",
		"}",
		"export interface OrganizationMemberType extends BaseMemberType {",
		"    level: AccessControlLevel",
		"}",
	}, "\n")
	if err := os.WriteFile(typesFile, []byte(typesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "frontend", "src", "accessLogic.ts")
	logicSource := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"",
		"import { AccessControlLevel, OrganizationMemberType } from '~/types'",
		"",
		"export const accessLogic = kea([",
		"    path(['accessLogic']),",
		"    actions({",
		"        updateAccessControlMembers: (",
		"            accessControls: {",
		"                member: OrganizationMemberType['id']",
		"                level: AccessControlLevel | null",
		"            }[]",
		"        ) => ({ accessControls }),",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "updateAccessControlMembers")
	if !ok {
		t.Fatalf("expected updateAccessControlMembers action, got %+v", logics[0].Actions)
	}
	expectedPayload := "{ accessControls: { member: string; level: AccessControlLevel | null; }[]; }"
	if action.PayloadType != expectedPayload {
		t.Fatalf("expected imported indexed access payload %q, got %+v", expectedPayload, action)
	}
}

func TestBuildParsedLogicsRecoversImportedIndexedAccessPayloadsForShorthandActions(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	typesFile := filepath.Join(tempDir, "types.ts")
	typesSource := strings.Join([]string{
		"export interface RoleType {",
		"    id: string",
		"    name: string",
		"}",
	}, "\n")
	if err := os.WriteFile(typesFile, []byte(typesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "roleAccessLogic.ts")
	logicSource := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"",
		"import { RoleType } from './types'",
		"",
		"export const roleAccessLogic = kea([",
		"    path(['roleAccessLogic']),",
		"    actions({",
		"        deleteRole: (roleId: RoleType['id']) => ({ roleId }),",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "deleteRole")
	if !ok {
		t.Fatalf("expected deleteRole action, got %+v", logics[0].Actions)
	}
	expectedPayload := "{ roleId: string; }"
	if action.PayloadType != expectedPayload {
		t.Fatalf("expected deleteRole payload %q, got %+v", expectedPayload, action)
	}
	expectedFunctionType := "(roleId: string) => { roleId: string; }"
	expectedFunctionType = "(roleId: RoleType['id']) => { roleId: string; }"
	if action.FunctionType != expectedFunctionType {
		t.Fatalf("expected deleteRole function type %q, got %+v", expectedFunctionType, action)
	}
}

func TestConnectedActionFunctionParametersLookMoreSpecificForIndexedAccess(t *testing.T) {
	current := "(accessControls: { member: string; level: AccessControlLevel | null; }[]) => void"
	candidate := "(accessControls: { member: OrganizationMemberType['id']; level: AccessControlLevel | null; }[]) => { accessControls: { member: string; level: AccessControlLevel | null; }[]; }"

	if !connectedActionFunctionParametersLookMoreSpecific(current, candidate) {
		t.Fatalf("expected candidate connected action parameters to be preferred")
	}
	if connectedActionFunctionParametersLookMoreSpecific(candidate, current) {
		t.Fatalf("expected current connected action parameters to not beat the indexed-access candidate")
	}
}

func TestBuildParsedLogicsConnectedActionsKeepIndexedAccessParameterTypes(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	typesFile := filepath.Join(tempDir, "types.ts")
	typesSource := strings.Join([]string{
		"export type AccessControlLevel = 'none' | 'viewer'",
		"export interface OrganizationMemberType {",
		"    id: string",
		"}",
	}, "\n")
	if err := os.WriteFile(typesFile, []byte(typesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	accessLogicFile := filepath.Join(tempDir, "accessLogic.ts")
	accessLogicSource := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"",
		"import { AccessControlLevel, OrganizationMemberType } from './types'",
		"",
		"export const accessLogic = kea([",
		"    path(['accessLogic']),",
		"    actions({",
		"        updateAccessControlMembers: (",
		"            accessControls: {",
		"                member: OrganizationMemberType['id']",
		"                level: AccessControlLevel | null",
		"            }[]",
		"        ) => ({ accessControls }),",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(accessLogicFile, []byte(accessLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	consumerLogicFile := filepath.Join(tempDir, "consumerLogic.ts")
	consumerLogicSource := strings.Join([]string{
		"import { connect, kea, path } from 'kea'",
		"",
		"import { accessLogic } from './accessLogic'",
		"",
		"export const consumerLogic = kea([",
		"    path(['consumerLogic']),",
		"    connect({",
		"        actions: [accessLogic, ['updateAccessControlMembers']],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(consumerLogicFile, []byte(consumerLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       consumerLogicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "updateAccessControlMembers")
	if !ok {
		t.Fatalf("expected connected updateAccessControlMembers action, got %+v", logics[0].Actions)
	}
	expectedPayload := "{ accessControls: { member: string; level: AccessControlLevel | null; }[]; }"
	if action.PayloadType != expectedPayload {
		t.Fatalf("expected connected payload %q, got %+v", expectedPayload, action)
	}
	expectedFunctionType := "(accessControls: { member: OrganizationMemberType['id']; level: AccessControlLevel | null; }[]) =>"
	if !strings.Contains(action.FunctionType, expectedFunctionType) {
		t.Fatalf("expected connected function type to contain %q, got %+v", expectedFunctionType, action)
	}
	if !hasImport(logics[0].Imports, "./types", "OrganizationMemberType") {
		t.Fatalf("expected connected action import for OrganizationMemberType, got %+v", logics[0].Imports)
	}
}

func TestBuildParsedLogicsConnectedActionsKeepFullPayloadWhenLocalListenerDestructuresSubset(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	typesFile := filepath.Join(tempDir, "types.ts")
	typesSource := "export type SidePanelTab = 'docs' | 'settings'\n"
	if err := os.WriteFile(typesFile, []byte(typesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	stateLogicFile := filepath.Join(tempDir, "sidePanelStateLogic.ts")
	stateLogicSource := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"",
		"import type { SidePanelTab } from './types'",
		"",
		"export const sidePanelStateLogic = kea([",
		"    path(['sidePanelStateLogic']),",
		"    actions({",
		"        openSidePanel: (tab: SidePanelTab, options?: string) => ({ tab, options }),",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(stateLogicFile, []byte(stateLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	consumerLogicFile := filepath.Join(tempDir, "consumerLogic.ts")
	consumerLogicSource := strings.Join([]string{
		"import { connect, kea, listeners, path } from 'kea'",
		"",
		"import { sidePanelStateLogic } from './sidePanelStateLogic'",
		"",
		"export const consumerLogic = kea([",
		"    path(['consumerLogic']),",
		"    connect(() => ({",
		"        actions: [sidePanelStateLogic, ['openSidePanel']],",
		"    })),",
		"    listeners(() => ({",
		"        openSidePanel: ({ options }) => {",
		"            console.log(options)",
		"        },",
		"    })),",
		"])",
	}, "\n")
	if err := os.WriteFile(consumerLogicFile, []byte(consumerLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       consumerLogicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "openSidePanel")
	if !ok {
		t.Fatalf("expected connected openSidePanel action, got %+v", logics[0].Actions)
	}
	if !strings.Contains(action.FunctionType, "tab: SidePanelTab") || !strings.Contains(action.FunctionType, "options?: string") {
		t.Fatalf("expected connected function type to preserve both parameters, got %+v", action)
	}
	actionPayload, ok := parseActionPayloadObjectMembers(action.PayloadType)
	if !ok {
		t.Fatalf("expected connected action payload object type, got %+v", action)
	}
	if len(actionPayload) != 2 || actionPayload["tab"].Type != "SidePanelTab" || actionPayload["options"].Type != "string | undefined" {
		t.Fatalf("expected connected action payload to preserve tab/options members, got %+v", action)
	}

	listener, ok := findParsedListener(logics[0].Listeners, "openSidePanel")
	if !ok {
		t.Fatalf("expected openSidePanel listener, got %+v", logics[0].Listeners)
	}
	listenerPayload, ok := parseActionPayloadObjectMembers(listener.PayloadType)
	if !ok {
		t.Fatalf("expected openSidePanel listener payload object type, got %+v", listener)
	}
	if len(listenerPayload) != 2 || listenerPayload["tab"].Type != "SidePanelTab" || listenerPayload["options"].Type != "string | undefined" {
		t.Fatalf("expected openSidePanel listener payload to preserve tab/options members, got %+v", listener)
	}
}

func TestBuildParsedLogicsFromSourceRecoversSourceOnlyActionsReferencedByListeners(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, listeners, path } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"",
		"type IncidentIoSummary = { status: string }",
		"",
		"export const logic = kea([",
		"    path(['logic']),",
		"    actions({",
		"        setPageVisibility: (visible: boolean) => ({ visible }),",
		"    }),",
		"    loaders(() => ({",
		"        summary: [null as IncidentIoSummary | null, {",
		"            loadSummary: async () => null as IncidentIoSummary | null,",
		"        }],",
		"    })),",
		"    listeners(({ actions }) => ({",
		"        setPageVisibility: ({ visible }) => {",
		"            if (visible) {",
		"                actions.loadSummary()",
		"            }",
		"        },",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/logic.ts",
		Logics: []LogicReport{
			{
				Name:      "logic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name:    "actions",
						Members: nil,
					},
					{
						Name: "loaders",
						Members: []MemberReport{
							{Name: "summary", TypeString: "any[]"},
						},
					},
					{
						Name:    "listeners",
						Members: nil,
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "setPageVisibility")
	if !ok {
		t.Fatalf("expected source-only setPageVisibility action, got %+v", logics[0].Actions)
	}
	if !strings.Contains(action.FunctionType, "(visible: boolean) =>") {
		t.Fatalf("expected setPageVisibility function type to preserve boolean parameter, got %+v", action)
	}
	payload, ok := parseActionPayloadObjectMembers(action.PayloadType)
	if !ok || len(payload) != 1 || payload["visible"].Type != "boolean" {
		t.Fatalf("expected setPageVisibility payload type to recover visible boolean member, got %+v", action)
	}

	listener, ok := findParsedListener(logics[0].Listeners, "setPageVisibility")
	if !ok {
		t.Fatalf("expected setPageVisibility listener, got %+v", logics[0].Listeners)
	}
	listenerPayload, ok := parseActionPayloadObjectMembers(listener.PayloadType)
	if !ok || len(listenerPayload) != 1 || listenerPayload["visible"].Type != "boolean" {
		t.Fatalf("expected setPageVisibility listener payload type to recover visible boolean member, got %+v", listener)
	}
}

func TestBuildParsedLogicsFromSourceRecoversSourceOnlyEventNames(t *testing.T) {
	source := strings.Join([]string{
		"import { events, kea, path } from 'kea'",
		"",
		"export const logic = kea([",
		"    path(['logic']),",
		"    events(() => ({",
		"        afterMount() {",
		"            console.log('mounted')",
		"        },",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/logic.ts",
		Logics: []LogicReport{
			{
				Name:      "logic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name:    "events",
						Members: nil,
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}
	if !containsString(logics[0].Events, "afterMount") {
		t.Fatalf("expected afterMount event to recover from source, got %+v", logics[0].Events)
	}
}

func TestBuildParsedLogicsFromSourceRecoversBlockBodyEventNames(t *testing.T) {
	source := strings.Join([]string{
		"import { events, kea, path } from 'kea'",
		"",
		"export const logic = kea([",
		"    path(['logic']),",
		"    events(({ actions }) => {",
		"        const logAction = actions",
		"        return {",
		"            afterMount() {",
		"                console.log(logAction)",
		"            },",
		"        }",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/logic.ts",
		Logics: []LogicReport{
			{
				Name:      "logic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name:    "events",
						Members: nil,
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}
	if !containsString(logics[0].Events, "afterMount") {
		t.Fatalf("expected block-body afterMount event to recover from source, got %+v", logics[0].Events)
	}
}

func TestShouldRefineActionPayloadTypeIgnoresNestedUndefinedWhenHintDropsMembers(t *testing.T) {
	current := "{ tab: SidePanelTab; options: string | undefined; }"
	hinted := "{ options: any; }"

	if shouldRefineActionPayloadType(current, hinted) {
		t.Fatalf("expected nested undefined member to not trigger payload refinement from %+q to %+q", current, hinted)
	}
}

func TestBuildParsedLogicsRecoversImportedHelperReturnTypesForKeysAndSelectors(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	typesFile := filepath.Join(tempDir, "types.ts")
	typesSource := strings.Join([]string{
		"export type AccessControlLevel = 'viewer' | 'editor'",
		"",
		"export type Entry = {",
		"    id: string",
		"}",
	}, "\n")
	if err := os.WriteFile(typesFile, []byte(typesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	helpersFile := filepath.Join(tempDir, "helpers.ts")
	helpersSource := strings.Join([]string{
		"import type { AccessControlLevel, Entry } from './types'",
		"",
		"export function getEntryId(entry: Entry): string {",
		"    return entry.id",
		"}",
		"",
		"export function getLevelOptionsForResource(",
		"    availableLevels: AccessControlLevel[],",
		"    options: { resourceLabel: string }",
		"): { label: string; value: AccessControlLevel }[] {",
		"    return availableLevels.map((level) => ({ label: `${options.resourceLabel}:${level}`, value: level }))",
		"}",
	}, "\n")
	if err := os.WriteFile(helpersFile, []byte(helpersSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "groupedAccessLogic.ts")
	logicSource := strings.Join([]string{
		"import { kea, key, path, props, selectors } from 'kea'",
		"",
		"import { getEntryId, getLevelOptionsForResource } from './helpers'",
		"import type { AccessControlLevel, Entry } from './types'",
		"",
		"type LogicProps = {",
		"    entry: Entry",
		"    availableProjectLevels: AccessControlLevel[]",
		"}",
		"",
		"export const groupedAccessLogic = kea([",
		"    path((key) => ['groupedAccessLogic', key]),",
		"    key((props: LogicProps) => getEntryId(props.entry)),",
		"    props({} as LogicProps),",
		"    selectors({",
		"        entryId: [(_, p) => [p.entry], (entry) => getEntryId(entry)],",
		"        projectLevelOptions: [",
		"            (_, p) => [p.availableProjectLevels],",
		"            (availableProjectLevels) =>",
		"                getLevelOptionsForResource(availableProjectLevels, { resourceLabel: 'project' }),",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if logic.KeyType != "string" {
		t.Fatalf("expected helper-backed key type %q, got %+v", "string", logic)
	}

	entryID, ok := findParsedField(logic.Selectors, "entryId")
	if !ok {
		t.Fatalf("expected entryId selector, got %+v", logic.Selectors)
	}
	if entryID.Type != "string" {
		t.Fatalf("expected entryId selector type %q, got %+v", "string", entryID)
	}

	projectLevelOptions, ok := findParsedField(logic.Selectors, "projectLevelOptions")
	if !ok {
		t.Fatalf("expected projectLevelOptions selector, got %+v", logic.Selectors)
	}
	if !strings.HasSuffix(projectLevelOptions.Type, "[]") ||
		!strings.Contains(projectLevelOptions.Type, "label: string") ||
		!strings.Contains(projectLevelOptions.Type, "value: AccessControlLevel") {
		t.Fatalf("expected projectLevelOptions selector to recover helper return array, got %+v", projectLevelOptions)
	}
}

func TestBuildParsedLogicsRecoversUntypedBuilderKeyFromImportedHelper(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	typesFile := filepath.Join(tempDir, "types.ts")
	typesSource := strings.Join([]string{
		"export type Entry = {",
		"    id: string",
		"}",
	}, "\n")
	if err := os.WriteFile(typesFile, []byte(typesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	helpersFile := filepath.Join(tempDir, "helpers.ts")
	helpersSource := strings.Join([]string{
		"import type { Entry } from './types'",
		"",
		"export function getEntryId(entry: Entry): string {",
		"    return entry.id",
		"}",
	}, "\n")
	if err := os.WriteFile(helpersFile, []byte(helpersSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "groupedAccessLogic.ts")
	logicSource := strings.Join([]string{
		"import { kea, key, path, props } from 'kea'",
		"",
		"import { getEntryId } from './helpers'",
		"import type { Entry } from './types'",
		"",
		"type LogicProps = {",
		"    entry: Entry",
		"}",
		"",
		"export const groupedAccessLogic = kea([",
		"    path((key) => ['groupedAccessLogic', key]),",
		"    key((props) => getEntryId(props.entry)),",
		"    props({} as LogicProps),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	if logics[0].KeyType != "string" {
		t.Fatalf("expected imported helper-backed untyped key type %q, got %+v", "string", logics[0])
	}
}

func TestBuildParsedLogicsRecoversKeyFromImportedPropsMemberLogicalFallback(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	workflowFile := filepath.Join(tempDir, "workflowLogic.ts")
	workflowSource := strings.Join([]string{
		"export type WorkflowLogicProps = {",
		"    id?: string",
		"}",
	}, "\n")
	if err := os.WriteFile(workflowFile, []byte(workflowSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "stepDelayLogic.ts")
	logicSource := strings.Join([]string{
		"import { kea, key, path, props } from 'kea'",
		"import type { WorkflowLogicProps } from './workflowLogic'",
		"",
		"type StepDelayLogicProps = {",
		"    workflowLogicProps: WorkflowLogicProps",
		"}",
		"",
		"export const stepDelayLogic = kea([",
		"    path((key) => ['stepDelayLogic', key]),",
		"    props({} as StepDelayLogicProps),",
		"    key(({ workflowLogicProps }: StepDelayLogicProps) => workflowLogicProps.id || 'new'),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	if logics[0].KeyType != "string" {
		t.Fatalf("expected imported props member key type %q, got %+v", "string", logics[0])
	}
}

func TestBuildParsedLogicsRecoversKeyFromNestedImportedPropsMemberLogicalFallback(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	categoryFile := filepath.Join(tempDir, "optOutCategoriesLogic.ts")
	categorySource := strings.Join([]string{
		"export type MessageCategory = {",
		"    id?: string",
		"}",
	}, "\n")
	if err := os.WriteFile(categoryFile, []byte(categorySource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "newCategoryLogic.ts")
	logicSource := strings.Join([]string{
		"import { kea, key, path, props } from 'kea'",
		"import type { MessageCategory } from './optOutCategoriesLogic'",
		"",
		"type CategoryLogicProps = {",
		"    category?: MessageCategory | null",
		"}",
		"",
		"export const newCategoryLogic = kea([",
		"    path(['newCategoryLogic']),",
		"    props({} as CategoryLogicProps),",
		"    key((props: CategoryLogicProps) => props.category?.id || 'new'),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	if logics[0].KeyType != "string" {
		t.Fatalf("expected nested imported props member key type %q, got %+v", "string", logics[0])
	}
}

func TestBuildParsedLogicsPrefersTypeProbeForMultiReturnSelectors(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	typesFile := filepath.Join(tempDir, "types.ts")
	typesSource := strings.Join([]string{
		"export type APIScopeObject = 'project' | 'dashboard'",
		"export type AccessControlLevel = 'viewer' | 'editor'",
		"",
		"export type Entry = {",
		"    id: string",
		"    inheritedReason: 'organization_admin' | null",
		"    inheritedLevel: AccessControlLevel | null",
		"}",
	}, "\n")
	if err := os.WriteFile(typesFile, []byte(typesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	helpersFile := filepath.Join(tempDir, "helpers.ts")
	helpersSource := strings.Join([]string{
		"import type { AccessControlLevel } from './types'",
		"",
		"export function getLevelOptionsForResource(",
		"    availableLevels: AccessControlLevel[],",
		"    resourceLabel: string",
		"): { value: AccessControlLevel; label: string; disabledReason?: string }[] {",
		"    return availableLevels.map((level) => ({ value: level, label: `${resourceLabel}:${level}` }))",
		"}",
	}, "\n")
	if err := os.WriteFile(helpersFile, []byte(helpersSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "groupedAccessLogic.ts")
	logicSource := strings.Join([]string{
		"import { kea, path, props, selectors } from 'kea'",
		"",
		"import { getLevelOptionsForResource } from './helpers'",
		"import type { APIScopeObject, AccessControlLevel, Entry } from './types'",
		"",
		"type LogicProps = {",
		"    entry: Entry",
		"    availableLevels: AccessControlLevel[]",
		"    loading: boolean",
		"    canEdit: boolean",
		"}",
		"",
		"export const groupedAccessLogic = kea([",
		"    path(['groupedAccessLogic']),",
		"    props({} as LogicProps),",
		"    selectors({",
		"        loading: [(_, p) => [p.loading], (loading) => loading],",
		"        canEdit: [(_, p) => [p.canEdit], (canEdit) => canEdit],",
		"        entry: [(_, p) => [p.entry], (entry) => entry],",
		"        availableLevels: [(_, p) => [p.availableLevels], (availableLevels) => availableLevels],",
		"        featuresDisabledReason: [",
		"            (s) => [s.loading, s.canEdit, s.entry],",
		"            (loading, canEdit, entry) => {",
		"                if (loading) {",
		"                    return 'Loading...'",
		"                }",
		"                if (!canEdit) {",
		"                    return 'Cannot edit'",
		"                }",
		"                if (entry.inheritedReason === 'organization_admin') {",
		"                    return 'User is an organization admin and has access to all features'",
		"                }",
		"                return undefined",
		"            },",
		"        ],",
		"        projectDisabledReason: [",
		"            (s) => [s.loading, s.canEdit, s.entry],",
		"            (loading, canEdit, entry) => {",
		"                if (loading) {",
		"                    return 'Loading...'",
		"                }",
		"                if (!canEdit) {",
		"                    return 'Cannot edit'",
		"                }",
		"                if (entry.inheritedReason === 'organization_admin') {",
		"                    return 'User is an organization admin'",
		"                }",
		"                return undefined",
		"            },",
		"        ],",
		"        resourceLevelOptions: [",
		"            (s) => [s.availableLevels, s.entry],",
		"            (availableLevels, entry) => (resource: APIScopeObject, resourceLabel: string) => {",
		"                const levelOptions = getLevelOptionsForResource(availableLevels, resourceLabel)",
		"                if (resource === 'dashboard' && entry.inheritedLevel === null) {",
		"                    return [",
		"                        { value: null as AccessControlLevel | null, label: 'No override', disabledReason: undefined },",
		"                        ...levelOptions,",
		"                    ]",
		"                }",
		"                return levelOptions",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	featuresDisabledReason, ok := findParsedField(logics[0].Selectors, "featuresDisabledReason")
	if !ok {
		t.Fatalf("expected featuresDisabledReason selector, got %+v", logics[0].Selectors)
	}
	for _, expected := range []string{
		"Cannot edit",
		"Loading...",
		"User is an organization admin and has access to all features",
		"undefined",
	} {
		if !strings.Contains(featuresDisabledReason.Type, expected) {
			t.Fatalf("expected featuresDisabledReason type to contain %q, got %+v", expected, featuresDisabledReason)
		}
	}

	projectDisabledReason, ok := findParsedField(logics[0].Selectors, "projectDisabledReason")
	if !ok {
		t.Fatalf("expected projectDisabledReason selector, got %+v", logics[0].Selectors)
	}
	for _, expected := range []string{"Cannot edit", "Loading...", "User is an organization admin", "undefined"} {
		if !strings.Contains(projectDisabledReason.Type, expected) {
			t.Fatalf("expected projectDisabledReason type to contain %q, got %+v", expected, projectDisabledReason)
		}
	}

	resourceLevelOptions, ok := findParsedField(logics[0].Selectors, "resourceLevelOptions")
	if !ok {
		t.Fatalf("expected resourceLevelOptions selector, got %+v", logics[0].Selectors)
	}
	for _, expected := range []string{
		"(resource: APIScopeObject, resourceLabel: string) =>",
		"value: AccessControlLevel | null",
		"value: AccessControlLevel",
		"disabledReason: undefined",
	} {
		if !strings.Contains(resourceLevelOptions.Type, expected) {
			t.Fatalf("expected resourceLevelOptions type to contain %q, got %+v", expected, resourceLevelOptions)
		}
	}
}

func TestBuildParsedLogicsPrefersDeclaredReducerSeedTypes(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path, reducers } from 'kea'",
		"",
		"type AccessControlFilters = {",
		"    roleIds: string[]",
		"    memberIds: string[]",
		"}",
		"",
		"const initialFilters: AccessControlFilters = {",
		"    roleIds: [],",
		"    memberIds: [],",
		"}",
		"",
		"export const filtersLogic = kea([",
		"    path(['filtersLogic']),",
		"    reducers({",
		"        filters: [",
		"            initialFilters,",
		"            {",
		"                setFilters: (state, { filters }) => ({ ...state, ...filters }),",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/filtersLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "filtersLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "filters", TypeString: "(AccessControlFilters | { setFilters: (state: any, { filters }: { filters: any; }) => any; })[]"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	if reducer, ok := findParsedField(logics[0].Reducers, "filters"); !ok || reducer.Type != "AccessControlFilters" {
		t.Fatalf("expected filters reducer type %q, got %+v", "AccessControlFilters", logics[0].Reducers)
	}
}

func TestBuildParsedLogicsRecoversGenericAPILoaderReturnTypesFromSource(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"import api from 'lib/api'",
		"",
		"type AccessControlDefaultsResponse = {",
		"    can_edit: boolean",
		"}",
		"",
		"export const accessLogic = kea([",
		"    path(['accessLogic']),",
		"    loaders(() => ({",
		"        defaults: [",
		"            null as AccessControlDefaultsResponse | null,",
		"            {",
		"                loadDefaults: async () => api.get<AccessControlDefaultsResponse>('api/projects/@current/access_control_defaults'),",
		"            },",
		"        ],",
		"        updatedDefaults: [",
		"            null as AccessControlDefaultsResponse | null,",
		"            {",
		"                updateDefaults: async () =>",
		"                    await api.put<AccessControlDefaultsResponse, { can_edit: boolean }>(",
		"                        'api/projects/@current/access_control_defaults',",
		"                        { can_edit: true },",
		"                    ),",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/accessLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "accessLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "loaders",
						Members: []MemberReport{
							{Name: "defaults", TypeString: "any[]"},
							{Name: "updatedDefaults", TypeString: "any[]"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if reducer, ok := findParsedField(logic.Reducers, "defaults"); !ok || reducer.Type != "AccessControlDefaultsResponse | null" {
		t.Fatalf("expected defaults reducer type %q, got %+v", "AccessControlDefaultsResponse | null", logic.Reducers)
	}
	if reducer, ok := findParsedField(logic.Reducers, "updatedDefaults"); !ok || reducer.Type != "AccessControlDefaultsResponse | null" {
		t.Fatalf("expected updatedDefaults reducer type %q, got %+v", "AccessControlDefaultsResponse | null", logic.Reducers)
	}
	if action, ok := findParsedAction(logic.Actions, "loadDefaults"); !ok || action.FunctionType != "() => Promise<AccessControlDefaultsResponse>" {
		t.Fatalf("expected loadDefaults action signature %q, got %+v", "() => Promise<AccessControlDefaultsResponse>", action)
	}
	if action, ok := findParsedAction(logic.Actions, "loadDefaultsSuccess"); !ok || action.FunctionType != "(defaults: AccessControlDefaultsResponse, payload?: any) => { defaults: AccessControlDefaultsResponse; payload?: any }" {
		t.Fatalf("expected loadDefaultsSuccess action signature to preserve generic API type, got %+v", action)
	}
	if action, ok := findParsedAction(logic.Actions, "updateDefaults"); !ok || action.FunctionType != "() => Promise<AccessControlDefaultsResponse>" {
		t.Fatalf("expected updateDefaults action signature %q, got %+v", "() => Promise<AccessControlDefaultsResponse>", action)
	}
	if action, ok := findParsedAction(logic.Actions, "updateDefaultsSuccess"); !ok || action.FunctionType != "(updatedDefaults: AccessControlDefaultsResponse, payload?: any) => { updatedDefaults: AccessControlDefaultsResponse; payload?: any }" {
		t.Fatalf("expected updateDefaultsSuccess action signature to preserve awaited generic API type, got %+v", action)
	}
}

func TestBuildParsedLogicsKeepsConcreteLoaderSuccessTypesAcrossTryCatchFallbacks(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "baseUrl": "frontend",`,
		`    "paths": {`,
		`      "~/*": ["src/*"],`,
		`      "lib/*": ["src/lib/*"]`,
		`    },`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	apiFile := filepath.Join(tempDir, "frontend", "src", "lib", "api.ts")
	if err := os.MkdirAll(filepath.Dir(apiFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	apiSource := strings.Join([]string{
		"const api = {",
		"    async get<T>(_url: string): Promise<T> {",
		"        throw new Error('network')",
		"    },",
		"}",
		"",
		"export default api",
	}, "\n")
	if err := os.WriteFile(apiFile, []byte(apiSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	typesFile := filepath.Join(tempDir, "frontend", "src", "types.ts")
	typesSource := strings.Join([]string{
		"export enum AccessControlLevel {",
		"    None = 'none',",
		"    Viewer = 'viewer',",
		"    Editor = 'editor',",
		"}",
		"",
		"export type AccessControlResponseType = {",
		"    access_controls: { access_level: AccessControlLevel | null }[]",
		"    available_access_levels: AccessControlLevel[]",
		"    user_access_level: AccessControlLevel",
		"    default_access_level: AccessControlLevel",
		"    user_can_edit_access_levels: boolean",
		"}",
	}, "\n")
	if err := os.WriteFile(typesFile, []byte(typesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "frontend", "src", "accessLogic.ts")
	logicSource := strings.Join([]string{
		"import { kea, path } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"",
		"import api from 'lib/api'",
		"import { AccessControlLevel, AccessControlResponseType } from '~/types'",
		"",
		"export const accessLogic = kea([",
		"    path(['accessLogic']),",
		"    loaders(() => ({",
		"        accessControls: [",
		"            null as AccessControlResponseType | null,",
		"            {",
		"                loadAccessControls: async () => {",
		"                    try {",
		"                        const response = await api.get<AccessControlResponseType>('api/access_controls')",
		"                        return response",
		"                    } catch {",
		"                        return {",
		"                            access_controls: [],",
		"                            available_access_levels: [",
		"                                AccessControlLevel.None,",
		"                                AccessControlLevel.Viewer,",
		"                                AccessControlLevel.Editor,",
		"                            ],",
		"                            user_access_level: AccessControlLevel.None,",
		"                            default_access_level: AccessControlLevel.None,",
		"                            user_can_edit_access_levels: false,",
		"                        }",
		"                    }",
		"                },",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "loadAccessControlsSuccess")
	if !ok {
		t.Fatalf("expected loadAccessControlsSuccess action, got %+v", logics[0].Actions)
	}
	expectedFunctionType := "(accessControls: AccessControlResponseType, payload?: any) => { accessControls: AccessControlResponseType; payload?: any }"
	if action.FunctionType != expectedFunctionType {
		t.Fatalf("expected loadAccessControlsSuccess function type %q, got %+v", expectedFunctionType, action)
	}
}

func TestBuildParsedLogicsKeepsConcreteLazyLoaderSuccessTypesForAwaitedLocalResponses(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "baseUrl": "frontend",`,
		`    "paths": {`,
		`      "lib/*": ["src/lib/*"]`,
		`    },`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	apiFile := filepath.Join(tempDir, "frontend", "src", "lib", "api.ts")
	if err := os.MkdirAll(filepath.Dir(apiFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	apiSource := strings.Join([]string{
		"export type PaginatedResponse<T> = {",
		"    results: T[]",
		"    next: string | null",
		"}",
		"",
		"export type ActivityLogItem = { id: string }",
		"",
		"const api = {",
		"    activity: {",
		"        async list(_filters: Record<string, unknown>): Promise<PaginatedResponse<ActivityLogItem>> {",
		"            throw new Error('network')",
		"        },",
		"    },",
		"    async get<T>(_url: string): Promise<T> {",
		"        throw new Error('network')",
		"    },",
		"}",
		"",
		"export default api",
	}, "\n")
	if err := os.WriteFile(apiFile, []byte(apiSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "frontend", "src", "activityLogic.ts")
	logicSource := strings.Join([]string{
		"import { kea, path } from 'kea'",
		"import { lazyLoaders } from 'kea-loaders'",
		"",
		"import api, { ActivityLogItem, PaginatedResponse } from 'lib/api'",
		"",
		"export const activityLogic = kea([",
		"    path(['activityLogic']),",
		"    actions({",
		"        loadAllActivity: true,",
		"        loadOlderActivity: true,",
		"    }),",
		"    lazyLoaders(({ values }) => ({",
		"        allActivityResponse: [",
		"            null as PaginatedResponse<ActivityLogItem> | null,",
		"            {",
		"                loadAllActivity: async () => {",
		"                    const response = await api.activity.list({})",
		"                    return response",
		"                },",
		"                loadOlderActivity: async () => {",
		"                    if (!values.allActivityResponse?.next) {",
		"                        return values.allActivityResponse",
		"                    }",
		"                    const response = await api.get<PaginatedResponse<ActivityLogItem>>(values.allActivityResponse.next)",
		"                    response.results = [...values.allActivityResponse.results, ...response.results]",
		"                    return response",
		"                },",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	loadAllActivitySuccess, ok := findParsedAction(logics[0].Actions, "loadAllActivitySuccess")
	if !ok {
		t.Fatalf("expected loadAllActivitySuccess action, got %+v", logics[0].Actions)
	}
	expectedLoadAllActivitySuccess := "(allActivityResponse: PaginatedResponse<ActivityLogItem>, payload?: { value: true; }) => { allActivityResponse: PaginatedResponse<ActivityLogItem>; payload?: { value: true; } }"
	if loadAllActivitySuccess.FunctionType != expectedLoadAllActivitySuccess {
		t.Fatalf("expected loadAllActivitySuccess function type %q, got %+v", expectedLoadAllActivitySuccess, loadAllActivitySuccess)
	}

	loadOlderActivitySuccess, ok := findParsedAction(logics[0].Actions, "loadOlderActivitySuccess")
	if !ok {
		t.Fatalf("expected loadOlderActivitySuccess action, got %+v", logics[0].Actions)
	}
	expectedLoadOlderActivitySuccess := "(allActivityResponse: PaginatedResponse<ActivityLogItem> | null, payload?: { value: true; }) => { allActivityResponse: PaginatedResponse<ActivityLogItem> | null; payload?: { value: true; } }"
	if loadOlderActivitySuccess.FunctionType != expectedLoadOlderActivitySuccess {
		t.Fatalf("expected loadOlderActivitySuccess function type %q, got %+v", expectedLoadOlderActivitySuccess, loadOlderActivitySuccess)
	}
}

func TestSourceReturnExpressionTypeWithContextMergesMultipleLiteralReturns(t *testing.T) {
	source := strings.Join([]string{
		"type Entry = {",
		"    inheritedReason: 'organization_admin' | null",
		"}",
	}, "\n")
	body := strings.Join([]string{
		"{",
		"    if (loading) {",
		"        return 'Loading...'",
		"    }",
		"    if (!canEdit) {",
		"        return 'Cannot edit'",
		"    }",
		"    if (entry.inheritedReason === 'organization_admin') {",
		"        return 'User is an organization admin and has access to all features'",
		"    }",
		"    return undefined",
		"}",
	}, "\n")

	returnType := sourceReturnExpressionTypeWithContext(
		source,
		"",
		body,
		true,
		[]string{"loading", "canEdit", "entry"},
		[]string{"boolean", "boolean", "{ inheritedReason: 'organization_admin' | null; }"},
		false,
		nil,
	)
	for _, expected := range []string{
		"Cannot edit",
		"Loading...",
		"User is an organization admin and has access to all features",
		"undefined",
	} {
		if !strings.Contains(returnType, expected) {
			t.Fatalf("expected merged multi-return type to contain %q, got %q", expected, returnType)
		}
	}
}

func TestSourceArrowFunctionTypeTextWithContextKeepsNestedMultiReturnFunction(t *testing.T) {
	source := strings.Join([]string{
		"type APIScopeObject = 'project' | 'dashboard'",
		"type AccessControlLevel = 'viewer' | 'editor'",
		"",
		"function getLevelOptionsForResource(",
		"    resourceLabel: string",
		"): { value: AccessControlLevel; label: string; disabledReason?: string }[] {",
		"    return [{ value: 'viewer', label: resourceLabel }]",
		"}",
	}, "\n")
	expression := strings.Join([]string{
		"(resource: APIScopeObject, resourceLabel: string) => {",
		"    const levelOptions = getLevelOptionsForResource(resourceLabel)",
		"    if (resource === 'dashboard') {",
		"        return [",
		"            { value: null as AccessControlLevel | null, label: 'No override', disabledReason: undefined },",
		"            ...levelOptions,",
		"        ]",
		"    }",
		"    return levelOptions",
		"}",
	}, "\n")

	returnType := sourceArrowFunctionTypeTextWithContext(source, "", expression, nil)
	expected := "(resource: APIScopeObject, resourceLabel: string) => ({ disabledReason: undefined; label: string; value: AccessControlLevel | null; } | { value: AccessControlLevel; label: string; disabledReason?: string; })[]"
	if returnType != expected {
		t.Fatalf("expected nested multi-return function type %q, got %q", expected, returnType)
	}
	if strings.Contains(returnType, "| { value: AccessControlLevel; label: string; disabledReason?: string; }[]") {
		t.Fatalf("expected nested multi-return function type to collapse redundant array union, got %q", returnType)
	}
}

func TestBuildParsedLogicsFromSourcePrefersRecoveredMultiReturnFunctionSelectors(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path, selectors } from 'kea'",
		"",
		"type APIScopeObject = 'project' | 'dashboard'",
		"type AccessControlLevel = 'viewer' | 'editor'",
		"type Entry = {",
		"    inheritedLevel: AccessControlLevel | null",
		"}",
		"",
		"function getLevelOptionsForResource(",
		"    resourceLabel: string",
		"): { value: AccessControlLevel; label: string; disabledReason?: string }[] {",
		"    return [{ value: 'viewer', label: resourceLabel }]",
		"}",
		"",
		"export const groupedAccessLogic = kea([",
		"    path(['groupedAccessLogic']),",
		"    selectors({",
		"        resourceLevelOptions: [",
		"            () => [['viewer', 'editor'] as AccessControlLevel[], { inheritedLevel: null } as Entry],",
		"            (availableLevels, entry) => (resource: APIScopeObject, resourceLabel: string) => {",
		"                const levelOptions = getLevelOptionsForResource(resourceLabel)",
		"                if (resource === 'dashboard' && entry.inheritedLevel === null) {",
		"                    return [",
		"                        { value: null as AccessControlLevel | null, label: 'No override', disabledReason: undefined },",
		"                        ...levelOptions,",
		"                    ]",
		"                }",
		"                return levelOptions",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/groupedAccessLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "groupedAccessLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:       "resourceLevelOptions",
								TypeString: "(((s: any) => any[]) | ((availableLevels: AccessControlLevel[], entry: Entry) => (resource: APIScopeObject, resourceLabel: string) => { value: AccessControlLevel; label: string; disabledReason?: string; }[]))[]",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	resourceLevelOptions, ok := findParsedField(logics[0].Selectors, "resourceLevelOptions")
	if !ok {
		t.Fatalf("expected resourceLevelOptions selector, got %+v", logics[0].Selectors)
	}
	expected := "((resource: APIScopeObject, resourceLabel: string) => ({ disabledReason: undefined; label: string; value: AccessControlLevel | null; } | { value: AccessControlLevel; label: string; disabledReason?: string | undefined; })[])"
	if resourceLevelOptions.Type != expected {
		t.Fatalf("expected recovered multi-return function selector %q, got %+v", expected, resourceLevelOptions)
	}
	if strings.Contains(resourceLevelOptions.Type, "| { value: AccessControlLevel; label: string; disabledReason?: string | undefined; }[]") {
		t.Fatalf("expected recovered multi-return function selector to collapse redundant array union, got %+v", resourceLevelOptions)
	}
}

func TestMergeNormalizedTypeUnionDropsRedundantObjectArraySubset(t *testing.T) {
	left := "({ disabledReason: undefined; label: string; value: AccessControlLevel | null; } | { value: AccessControlLevel; label: string; disabledReason?: string; })[]"
	right := "{ value: AccessControlLevel; label: string; disabledReason?: string; }[]"

	merged := mergeNormalizedTypeUnion(left, right)
	if merged != left {
		t.Fatalf("expected redundant object array subset to be dropped, got %q", merged)
	}
}

func TestMergeNormalizedTypeUnionAbsorbsAny(t *testing.T) {
	merged := mergeNormalizedTypeUnion("true | false", "any")
	if merged != "any" {
		t.Fatalf("expected union containing any to collapse to any, got %q", merged)
	}
}

func TestNormalizeSelectorFunctionTypeOptionalUndefinedPreservesReturnObjectMembers(t *testing.T) {
	typeText := "((resource: APIScopeObject, resourceLabel: string) => ({ disabledReason: undefined; label: string; value: AccessControlLevel | null; } | { value: AccessControlLevel; label: string; disabledReason?: string; })[])"

	normalized := normalizeSelectorFunctionTypeOptionalUndefined(typeText)
	expected := "((resource: APIScopeObject, resourceLabel: string) => ({ disabledReason: undefined; label: string; value: AccessControlLevel | null; } | { value: AccessControlLevel; label: string; disabledReason?: string | undefined; })[])"
	if normalized != expected {
		t.Fatalf("expected selector function type %q, got %q", expected, normalized)
	}
}

func TestBuildParsedLogicsFallsBackToLoaderDefaultTypeForBarePromiseReturn(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"",
		"type EarlyAccessFeature = {",
		"    flagKey: string",
		"}",
		"",
		"export const featurePreviewsLogic = kea([",
		"    path(['featurePreviewsLogic']),",
		"    loaders(() => ({",
		"        rawEarlyAccessFeatures: [",
		"            [] as EarlyAccessFeature[],",
		"            {",
		"                loadEarlyAccessFeatures: async () => {",
		"                    return await new Promise((resolve) => resolve([] as EarlyAccessFeature[]))",
		"                },",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/featurePreviewsLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "featurePreviewsLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "loaders",
						Members: []MemberReport{
							{Name: "rawEarlyAccessFeatures", TypeString: "any[]"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if action, ok := findParsedAction(logic.Actions, "loadEarlyAccessFeaturesSuccess"); !ok || action.FunctionType != "(rawEarlyAccessFeatures: EarlyAccessFeature[], payload?: any) => { rawEarlyAccessFeatures: EarlyAccessFeature[]; payload?: any }" {
		t.Fatalf("expected loadEarlyAccessFeaturesSuccess to use the loader default state type, got %+v", action)
	}
}

func TestBuildParsedLogicsProbesLoaderReturnTypesWhenSourceLooksNarrowerThanReality(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "userLogic.ts")
	logicSource := strings.Join([]string{
		"import { kea, path } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"",
		"type UserType = { id: string }",
		"",
		"async function fetchUser(): Promise<UserType> {",
		"    return { id: '1' }",
		"}",
		"",
		"export const userLogic = kea([",
		"    path(['userLogic']),",
		"    loaders(() => ({",
		"        user: [",
		"            null as UserType | null,",
		"            {",
		"                loadUser: async () => {",
		"                    try {",
		"                        return await fetchUser()",
		"                    } catch {",
		"                        return null",
		"                    }",
		"                },",
		"                deleteUser: async () => {",
		"                    await Promise.resolve()",
		"                    return null",
		"                },",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if action, ok := findParsedAction(logic.Actions, "loadUser"); !ok || action.FunctionType != "() => Promise<UserType | null>" {
		t.Fatalf("expected loadUser action signature %q, got %+v", "() => Promise<UserType | null>", action)
	}
	if action, ok := findParsedAction(logic.Actions, "loadUserSuccess"); !ok || action.FunctionType != "(user: UserType | null, payload?: any) => { user: UserType | null; payload?: any }" {
		t.Fatalf("expected loadUserSuccess to recover the probed union return type, got %+v", action)
	}
	if action, ok := findParsedAction(logic.Actions, "deleteUserSuccess"); !ok || action.FunctionType != "(user: null, payload?: any) => { user: null; payload?: any }" {
		t.Fatalf("expected deleteUserSuccess to keep the null-only return type, got %+v", action)
	}
}

func TestBuildParsedLogicsRecoversTypedAPILoaderSuccessUnionFromBlockBody(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "userLogic.ts")
	logicSource := strings.Join([]string{
		"import { kea, path } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"",
		"type UserType = { id: string }",
		"",
		"declare const api: {",
		"    get<T>(url: string): Promise<T>",
		"    delete(url: string): Promise<void>",
		"}",
		"",
		"export const userLogic = kea([",
		"    path(['userLogic']),",
		"    loaders(() => ({",
		"        user: [",
		"            null as UserType | null,",
		"            {",
		"                loadUser: async () => {",
		"                    try {",
		"                        return await api.get<UserType>('api/users/@me/')",
		"                    } catch (error: any) {",
		"                        console.error(error)",
		"                    }",
		"                    return null",
		"                },",
		"                deleteUser: async () => {",
		"                    return await api.delete('api/users/@me/').then(() => {",
		"                        return null",
		"                    })",
		"                },",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if action, ok := findParsedAction(logic.Actions, "loadUserSuccess"); !ok || action.FunctionType != "(user: UserType | null, payload?: any) => { user: UserType | null; payload?: any }" {
		t.Fatalf("expected loadUserSuccess to recover the typed API union return type, got %+v", action)
	}
	if action, ok := findParsedAction(logic.Actions, "deleteUserSuccess"); !ok || action.FunctionType != "(user: null, payload?: any) => { user: null; payload?: any }" {
		t.Fatalf("expected deleteUserSuccess to keep the null-only return type, got %+v", action)
	}
}

func TestSourceLoaderMemberTypeFromPropertyRecoversTypedAPILoaderUnionWithActionsContext(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"import { loaders } from 'kea-loaders'",
		"",
		"type UserType = { id: string }",
		"",
		"declare const api: {",
		"    get<T>(url: string): Promise<T>",
		"}",
		"",
		"export const userLogic = kea([",
		"    loaders(({ values, actions }) => ({",
		"        user: [",
		"            null as UserType | null,",
		"            {",
		"                loadUser: async () => {",
		"                    try {",
		"                        return await api.get<UserType>('api/users/@me/')",
		"                    } catch (error: any) {",
		"                        console.error(error)",
		"                        actions.loadUserFailure(error.message)",
		"                    }",
		"                    return null",
		"                },",
		"            },",
		"        ],",
		"    })),",
		"])",
	}, "\n")

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	loadersProperty := mustFindLogicProperty(t, logics[0], "loaders")
	userProperty, ok := sectionSourceProperties(source, loadersProperty)["user"]
	if !ok {
		t.Fatalf("expected user loader source property, got %+v", sectionSourceProperties(source, loadersProperty))
	}
	defaultType, properties, ok := sourceLoaderMemberTypeFromProperty(source, userProperty, "", nil)
	if !ok {
		t.Fatalf("expected source loader property recovery to succeed")
	}
	if defaultType != "UserType | null" {
		t.Fatalf("expected loader default type %q, got %q", "UserType | null", defaultType)
	}
	if properties["loadUser"] != "() => Promise<UserType | null>" {
		expression := strings.Join([]string{
			"async () => {",
			"    try {",
			"        return await api.get<UserType>('api/users/@me/')",
			"    } catch (error: any) {",
			"        console.error(error)",
			"        actions.loadUserFailure(error.message)",
			"    }",
			"    return null",
			"}",
		}, "\n")
		simpleType := sourceArrowFunctionTypeText(source, expression)
		contextType := sourceArrowFunctionTypeTextWithContext(source, "", expression, nil)
		arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(source, userProperty.ValueStart, userProperty.ValueEnd)
		if err != nil || !ok {
			t.Fatalf("FindInspectableArrayLiteral returned ok=%v err=%v", ok, err)
		}
		parts, err := splitTopLevelSourceSegments(source, arrayStart+1, arrayEnd)
		if err != nil || len(parts) < 2 {
			t.Fatalf("splitTopLevelSourceSegments returned err=%v parts=%d", err, len(parts))
		}
		nestedProperties, err := parseTopLevelProperties(source, parts[1].Start, parts[1].End)
		if err != nil {
			t.Fatalf("parseTopLevelProperties returned error: %v", err)
		}
		rangeType := ""
		for _, nested := range nestedProperties {
			if nested.Name == "loadUser" {
				rangeType = sourceArrowFunctionTypeTextFromRange(source, "", nested, nil)
				break
			}
		}
		t.Fatalf(
			"expected loadUser loader function type %q, got %q (simple=%q range=%q context=%q)",
			"() => Promise<UserType | null>",
			properties["loadUser"],
			simpleType,
			rangeType,
			contextType,
		)
	}
}

func TestBuildParsedLogicsRecoversConnectedActionPayloadsWithDefaultedSourceParameters(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["src/**/*"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	sceneTypesFile := filepath.Join(tempDir, "src", "scenes", "sceneTypes.ts")
	if err := os.MkdirAll(filepath.Dir(sceneTypesFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	sceneTypesSource := strings.Join([]string{
		"export type SceneProps = Record<string, any>",
		"",
		"export interface SceneExport<T = SceneProps> {",
		"    props?: T",
		"}",
		"",
		"export interface SceneParams {",
		"    params: Record<string, any>",
		"}",
	}, "\n")
	if err := os.WriteFile(sceneTypesFile, []byte(sceneTypesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	sceneLogicFile := filepath.Join(tempDir, "src", "scenes", "sceneLogic.ts")
	sceneLogicSource := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"import { SceneExport, SceneParams } from './sceneTypes'",
		"",
		"export const sceneLogic = kea([",
		"    path(['scenes', 'sceneLogic']),",
		"    actions({",
		"        setScene: (",
		"            sceneId: string,",
		"            sceneKey: string | undefined,",
		"            tabId: string,",
		"            params: SceneParams,",
		"            scrollToTop: boolean = false,",
		"            exportedScene?: SceneExport,",
		"        ) => ({",
		"            sceneId,",
		"            sceneKey,",
		"            tabId,",
		"            params,",
		"            scrollToTop,",
		"            exportedScene,",
		"        }),",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(sceneLogicFile, []byte(sceneLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	navigationLogicFile := filepath.Join(tempDir, "src", "layout", "navigationLogic.ts")
	if err := os.MkdirAll(filepath.Dir(navigationLogicFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	navigationLogicSource := strings.Join([]string{
		"import { connect, kea, path } from 'kea'",
		"import { sceneLogic } from '../scenes/sceneLogic'",
		"",
		"export const navigationLogic = kea([",
		"    path(['layout', 'navigationLogic']),",
		"    connect(() => ({",
		"        actions: [sceneLogic, ['setScene']],",
		"    })),",
		"])",
	}, "\n")
	if err := os.WriteFile(navigationLogicFile, []byte(navigationLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	sceneReport, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       sceneLogicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error for sceneLogic: %v", err)
	}

	sceneLogics, err := BuildParsedLogics(sceneReport)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error for sceneLogic: %v", err)
	}
	if len(sceneLogics) != 1 {
		t.Fatalf("expected 1 scene logic, got %d", len(sceneLogics))
	}
	sceneLogic := sceneLogics[0]
	sceneAction, ok := findParsedAction(sceneLogic.Actions, "setScene")
	if !ok {
		t.Fatalf("expected setScene action, got %+v", sceneLogic.Actions)
	}
	for _, expected := range []string{
		"scrollToTop?: boolean",
		"exportedScene?: SceneExport",
		"exportedScene: SceneExport<SceneProps> | undefined",
		"scrollToTop: boolean",
	} {
		if !strings.Contains(sceneAction.FunctionType, expected) {
			t.Fatalf("expected scene setScene function type to contain %q, got %q", expected, sceneAction.FunctionType)
		}
	}
	if !hasImport(sceneLogic.Imports, "./sceneTypes", "SceneProps") {
		t.Fatalf("expected SceneProps import in scene logic, got %+v", sceneLogic.Imports)
	}

	navigationReport, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       navigationLogicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error for navigationLogic: %v", err)
	}

	navigationLogics, err := BuildParsedLogics(navigationReport)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error for navigationLogic: %v", err)
	}
	if len(navigationLogics) != 1 {
		t.Fatalf("expected 1 navigation logic, got %d", len(navigationLogics))
	}
	navigationLogic := navigationLogics[0]
	action, ok := findParsedAction(navigationLogic.Actions, "setScene")
	if !ok {
		t.Fatalf("expected connected setScene action, got %+v", navigationLogic.Actions)
	}
	for _, expected := range []string{
		"scrollToTop?: boolean",
		"exportedScene?: SceneExport",
		"exportedScene: SceneExport<SceneProps> | undefined",
	} {
		if !strings.Contains(action.FunctionType, expected) {
			t.Fatalf("expected connected setScene function type to contain %q, got %q", expected, action.FunctionType)
		}
	}
	if !hasImport(navigationLogic.Imports, "../scenes/sceneTypes", "SceneProps") {
		t.Fatalf("expected SceneProps import in navigation logic, got %+v", navigationLogic.Imports)
	}

	rendered := EmitTypegenAt(navigationLogics, time.Date(2026, time.March, 12, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"import type { SceneExport, SceneParams, SceneProps } from '../scenes/sceneTypes'",
		"scrollToTop?: boolean, exportedScene?: SceneExport",
		"exportedScene: SceneExport<SceneProps> | undefined",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered navigation typegen to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsRecoversAliasedConnectedActionPayloadsWithDefaultedSourceParameters(t *testing.T) {
	tempDir := t.TempDir()
	frontendDir := filepath.Join(tempDir, "frontend", "src")

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "baseUrl": "frontend/src",`,
		`    "paths": {`,
		`      "scenes/*": ["scenes/*"]`,
		`    },`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["frontend/src/**/*"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	sceneTypesFile := filepath.Join(frontendDir, "scenes", "sceneTypes.ts")
	if err := os.MkdirAll(filepath.Dir(sceneTypesFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	sceneTypesSource := strings.Join([]string{
		"export type SceneProps = Record<string, any>",
		"",
		"export interface SceneExport<T = SceneProps> {",
		"    props?: T",
		"}",
		"",
		"export interface SceneParams {",
		"    params: Record<string, any>",
		"}",
	}, "\n")
	if err := os.WriteFile(sceneTypesFile, []byte(sceneTypesSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	sceneLogicFile := filepath.Join(frontendDir, "scenes", "sceneLogic.ts")
	sceneLogicSource := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"import { SceneExport, SceneParams } from 'scenes/sceneTypes'",
		"",
		"export const sceneLogic = kea([",
		"    path(['scenes', 'sceneLogic']),",
		"    actions({",
		"        setScene: (",
		"            sceneId: string,",
		"            sceneKey: string | undefined,",
		"            tabId: string,",
		"            params: SceneParams,",
		"            scrollToTop: boolean = false,",
		"            exportedScene?: SceneExport,",
		"        ) => ({",
		"            sceneId,",
		"            sceneKey,",
		"            tabId,",
		"            params,",
		"            scrollToTop,",
		"            exportedScene,",
		"        }),",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(sceneLogicFile, []byte(sceneLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	navigationLogicFile := filepath.Join(frontendDir, "layout", "navigationLogic.ts")
	if err := os.MkdirAll(filepath.Dir(navigationLogicFile), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	navigationLogicSource := strings.Join([]string{
		"import { connect, kea, path } from 'kea'",
		"import { sceneLogic } from 'scenes/sceneLogic'",
		"",
		"export const navigationLogic = kea([",
		"    path(['layout', 'navigationLogic']),",
		"    connect(() => ({",
		"        actions: [sceneLogic, ['setScene']],",
		"    })),",
		"])",
	}, "\n")
	if err := os.WriteFile(navigationLogicFile, []byte(navigationLogicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	sceneReport, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       sceneLogicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error for sceneLogic: %v", err)
	}

	sceneLogics, err := BuildParsedLogics(sceneReport)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error for sceneLogic: %v", err)
	}
	if len(sceneLogics) != 1 {
		t.Fatalf("expected 1 scene logic, got %d", len(sceneLogics))
	}
	sceneLogic := sceneLogics[0]
	sceneAction, ok := findParsedAction(sceneLogic.Actions, "setScene")
	if !ok {
		t.Fatalf("expected setScene action, got %+v", sceneLogic.Actions)
	}
	for _, expected := range []string{
		"scrollToTop?: boolean",
		"exportedScene?: SceneExport",
		"exportedScene: SceneExport<SceneProps> | undefined",
		"scrollToTop: boolean",
	} {
		if !strings.Contains(sceneAction.FunctionType, expected) {
			t.Fatalf("expected aliased scene setScene function type to contain %q, got %q", expected, sceneAction.FunctionType)
		}
	}
	if !hasImport(sceneLogic.Imports, "scenes/sceneTypes", "SceneProps") {
		t.Fatalf("expected SceneProps import from aliased scene types, got %+v", sceneLogic.Imports)
	}

	navigationReport, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       navigationLogicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error for navigationLogic: %v", err)
	}

	navigationLogics, err := BuildParsedLogics(navigationReport)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error for navigationLogic: %v", err)
	}
	if len(navigationLogics) != 1 {
		t.Fatalf("expected 1 navigation logic, got %d", len(navigationLogics))
	}
	navigationLogic := navigationLogics[0]
	action, ok := findParsedAction(navigationLogic.Actions, "setScene")
	if !ok {
		t.Fatalf("expected connected setScene action, got %+v", navigationLogic.Actions)
	}
	for _, expected := range []string{
		"scrollToTop?: boolean",
		"exportedScene?: SceneExport",
		"exportedScene: SceneExport<SceneProps> | undefined",
	} {
		if !strings.Contains(action.FunctionType, expected) {
			t.Fatalf("expected aliased connected setScene function type to contain %q, got %q", expected, action.FunctionType)
		}
	}
	if !hasImport(navigationLogic.Imports, "scenes/sceneTypes", "SceneProps") {
		t.Fatalf("expected SceneProps import from aliased scene types in navigation logic, got %+v", navigationLogic.Imports)
	}
}

func TestBuildParsedLogicsRecoversReducerStateFromHelperCallWithReducerOptions(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path, reducers } from 'kea'",
		"",
		"type FeatureFlagsSet = {",
		"    [key: string]: boolean | string",
		"}",
		"",
		"function getPersistedFeatureFlags(): FeatureFlagsSet {",
		"    return {}",
		"}",
		"",
		"export const featureFlagLogic = kea([",
		"    path(['featureFlagLogic']),",
		"    reducers({",
		"        featureFlags: [",
		"            getPersistedFeatureFlags(),",
		"            { persist: true },",
		"            {",
		"                setFeatureFlags: (_, { variants }) => variants as FeatureFlagsSet,",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/featureFlagLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "featureFlagLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "reducers",
						Members: []MemberReport{
							{
								Name:       "featureFlags",
								TypeString: "(FeatureFlagsSet | { setFeatureFlags: (_: any, { variants }: { variants: any; }) => FeatureFlagsSet; })[]",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	if reducer, ok := findParsedField(logics[0].Reducers, "featureFlags"); !ok || reducer.Type != "FeatureFlagsSet" {
		t.Fatalf("expected helper-backed reducer state type %q, got %+v", "FeatureFlagsSet", logics[0].Reducers)
	}
}

func TestBuildParsedLogicsRecoversLazyLoadersFromSource(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"import { lazyLoaders } from 'kea-loaders'",
		"",
		"interface DemoItem {",
		"    id: string",
		"}",
		"",
		"export const lazyLogic = kea([",
		"    path(['lazyLogic']),",
		"    lazyLoaders(() => ({",
		"        item: [null as DemoItem | null, {",
		"            loadItem: async (id: string) => ({ id } as DemoItem),",
		"        }],",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/lazyLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "lazyLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "lazyLoaders",
						Members: []MemberReport{
							{Name: "item", TypeString: "any[]"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if !hasReducer(logic.Reducers, "item", "DemoItem | null") {
		t.Fatalf("expected item reducer type to recover from source, got %+v", logic.Reducers)
	}
	if !hasReducer(logic.Reducers, "itemLoading", "boolean") {
		t.Fatalf("expected itemLoading reducer to be synthesized, got %+v", logic.Reducers)
	}
	if action, ok := findParsedAction(logic.Actions, "loadItem"); !ok || action.FunctionType != "(id: string) => Promise<DemoItem>" {
		t.Fatalf("expected loadItem action signature to recover from source, got %+v", logic.Actions)
	}
	if action, ok := findParsedAction(logic.Actions, "loadItemSuccess"); !ok || action.FunctionType != "(item: DemoItem, payload?: string) => { item: DemoItem; payload?: string }" {
		t.Fatalf("expected loadItemSuccess action signature to recover from source, got %+v", logic.Actions)
	}
	if !hasAction(logic.Actions, "loadItemFailure") {
		t.Fatalf("expected loadItemFailure action to be synthesized, got %+v", logic.Actions)
	}
}

func TestBuildParsedLogicsRecoversLazyLoaderActionWithJSXAndBreakpoint(t *testing.T) {
	source := strings.Join([]string{
		"import { BreakPointFunction, kea, path } from 'kea'",
		"import { lazyLoaders } from 'kea-loaders'",
		"",
		"interface BillingType {",
		"    id: string",
		"}",
		"",
		"const Link = (_props: { to: string; children?: any }): any => null",
		"",
		"export const billingLogic = kea([",
		"    path(['billingLogic']),",
		"    lazyLoaders(() => ({",
		"        billing: [null as BillingType | null, {",
		"            deactivateProduct: async (key: string, breakpoint: BreakPointFunction) => {",
		"                const message = {",
		"                    link: (",
		"                        <Link to=\"/\">",
		"                            View invoices",
		"                        </Link>",
		"                    ),",
		"                }",
		"                await breakpoint(100)",
		"                return null as BillingType | null",
		"            },",
		"        }],",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/billingLogic.tsx",
		Logics: []LogicReport{
			{
				Name:      "billingLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "lazyLoaders",
						Members: []MemberReport{
							{Name: "billing", TypeString: "any[]"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if !hasReducer(logic.Reducers, "billing", "BillingType | null") {
		t.Fatalf("expected billing reducer type to recover from source, got %+v", logic.Reducers)
	}
	if !hasReducer(logic.Reducers, "billingLoading", "boolean") {
		t.Fatalf("expected billingLoading reducer to be synthesized, got %+v", logic.Reducers)
	}
	if action, ok := findParsedAction(logic.Actions, "deactivateProduct"); !ok || action.FunctionType != "(key: string) => Promise<BillingType | null>" {
		t.Fatalf("expected deactivateProduct action signature without breakpoint helper, got %+v", action)
	}
	if action, ok := findParsedAction(logic.Actions, "deactivateProductSuccess"); !ok || action.FunctionType != "(billing: BillingType | null, payload?: string) => { billing: BillingType | null; payload?: string }" {
		t.Fatalf("expected deactivateProductSuccess action signature to recover from source, got %+v", action)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, unexpected := range []string{
		"breakpoint: BreakPointFunction",
		"breakpoint: BreakpointFunction",
	} {
		if strings.Contains(rendered, unexpected) {
			t.Fatalf("expected emitted output to omit lazy loader helper parameter %q:\n%s", unexpected, rendered)
		}
	}
}

func TestBuildParsedLogicsPreservesMultilineActionPayloadObjectTypes(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path, actions } from 'kea'",
		"",
		"type PropertyDefinitionType = 'event'",
		"",
		"export const propertyDefinitionsModel = kea([",
		"    path(['models', 'propertyDefinitionsModel']),",
		"    actions({",
		"        loadPropertyValues: (payload: {",
		"            endpoint: string | undefined",
		"            type: PropertyDefinitionType",
		"            newInput: string | undefined",
		"            propertyKey: string",
		"            eventNames?: string[]",
		"            properties?: {",
		"                key: string",
		"                values: string | string[]",
		"            }[]",
		"            forceRefresh?: boolean",
		"        }) => payload,",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/propertyDefinitionsModel.ts",
		Logics: []LogicReport{
			{
				Name:      "propertyDefinitionsModel",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{Name: "loadPropertyValues", TypeString: "(payload: any) => any"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "loadPropertyValues")
	if !ok {
		t.Fatalf("expected loadPropertyValues action, got %+v", logics[0].Actions)
	}
	expectedParameterType := "(payload: { endpoint: string | undefined; type: PropertyDefinitionType; newInput: string | undefined; propertyKey: string; eventNames?: string[]; properties?: { key: string; values: string | string[]; }[]; forceRefresh?: boolean; }) =>"
	if !strings.HasPrefix(action.FunctionType, expectedParameterType) {
		t.Fatalf("expected action function type prefix %q, got %q", expectedParameterType, action.FunctionType)
	}
	if strings.Contains(action.FunctionType, "undefined type:") {
		t.Fatalf("expected multiline object members to stay separated, got %q", action.FunctionType)
	}
}

func TestBuildParsedLogicsLogicSampleRecoversDerivedSelectorReturnTypes(t *testing.T) {
	report := inspectSampleReport(t, "logic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	for _, expected := range []struct {
		name string
		typ  string
	}{
		{name: "capitalizedName", typ: "string"},
		{name: "upperCaseName", typ: "string"},
		{name: "randomSelector", typ: "Record<string, any>"},
		{name: "longSelector", typ: "false"},
	} {
		if selector, ok := findParsedField(logic.Selectors, expected.name); !ok || selector.Type != expected.typ {
			t.Fatalf("expected selector %s: %s, got %+v", expected.name, expected.typ, logic.Selectors)
		}
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"capitalizedName: (state: any, props?: any) => string",
		"upperCaseName: (state: any, props?: any) => string",
		"randomSelector: (state: any, props?: any) => Record<string, any>",
		"longSelector: (state: any, props?: any) => false",
		"capitalizedName: string",
		"upperCaseName: string",
		"__keaTypeGenInternalSelectorTypes: {",
		"capitalizedName: (name: string, number: number) => string",
		"upperCaseName: (capitalizedName: string) => string",
		"randomSelector: (capitalizedName: string) => Record<string, any>",
		"longSelector: (name: string, number: number, capitalizedName: string, upperCaseName: string, randomSelector: Record<string, any>, randomSelector2: Record<string, any>) => false",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsFromSourceRecoversReducedWriteRoundSelectorReturns(t *testing.T) {
	source := mustReadFile(t, filepath.Join(repoRoot(t), "samples", "logic.ts"))
	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/logic.ts",
		Logics: []LogicReport{
			{
				Name:      "logic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "reducers",
						Members: []MemberReport{
							{Name: "name", TypeString: "[string, { updateName: (_: string, { name }: { name: string; }) => string; }]"},
							{Name: "number", TypeString: "[number, { updateNumber: (_: number, { number }: { number: number; }) => number; }]"},
						},
					},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "capitalizedName",
								TypeString:       "((name: any, number: any) => any)[]",
								ReturnTypeString: "any",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	capitalizedName, ok := findParsedField(logics[0].Selectors, "capitalizedName")
	if !ok {
		t.Fatalf("expected capitalizedName selector, got %+v", logics[0].Selectors)
	}
	if capitalizedName.Type != "string" {
		t.Fatalf("expected capitalizedName selector type string, got %+v", capitalizedName)
	}
}

func TestBuildParsedLogicsRecoversImportedDefaultArrayTypesFromSourceExpressions(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	defaultTreeFile := filepath.Join(tempDir, "defaultTree.ts")
	defaultTreeSource := strings.Join([]string{
		"export type FileSystemImport = { path: string; category?: string }",
		"",
		"export function getDefaultTreeProducts(): FileSystemImport[] {",
		"    return [{ path: 'Product analytics' }, { path: 'Session Replay', category: 'Replay' }]",
		"}",
	}, "\n")
	if err := os.WriteFile(defaultTreeFile, []byte(defaultTreeSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "logic.ts")
	logicSource := strings.Join([]string{
		"import { defaults, kea, path } from 'kea'",
		"import { getDefaultTreeProducts } from './defaultTree'",
		"",
		"export const logic = kea([",
		"    path(['logic']),",
		"    defaults({",
		"        allProducts: getDefaultTreeProducts().sort((a, b) => a.path.localeCompare(b.path || 'b')),",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if reducer, ok := findParsedField(logic.Reducers, "allProducts"); !ok || reducer.Type != "FileSystemImport[]" {
		t.Fatalf("expected allProducts reducer type FileSystemImport[], got %+v", logic.Reducers)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"allProducts: FileSystemImport[]",
		"allProducts: (state: any, props?: any) => FileSystemImport[]",
	} {
		if !typegenAssertionContains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsRecoversImportedReducerTupleDefaultTypesFromSourceExpressions(t *testing.T) {
	tempDir := t.TempDir()

	tsconfigPath := filepath.Join(tempDir, "tsconfig.json")
	tsconfig := strings.Join([]string{
		"{",
		`  "compilerOptions": {`,
		`    "target": "ES2020",`,
		`    "module": "commonjs",`,
		`    "moduleResolution": "node",`,
		`    "skipLibCheck": true`,
		`  },`,
		`  "include": ["**/*.ts"]`,
		"}",
	}, "\n")
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	defaultTreeFile := filepath.Join(tempDir, "defaultTree.ts")
	defaultTreeSource := strings.Join([]string{
		"export type FileSystemImport = { path: string; category?: string }",
		"",
		"export function getDefaultTreeProducts(): FileSystemImport[] {",
		"    return [{ path: 'Product analytics' }, { path: 'Session Replay', category: 'Replay' }]",
		"}",
	}, "\n")
	if err := os.WriteFile(defaultTreeFile, []byte(defaultTreeSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	logicFile := filepath.Join(tempDir, "logic.ts")
	logicSource := strings.Join([]string{
		"import { kea, path, reducers } from 'kea'",
		"import { getDefaultTreeProducts } from './defaultTree'",
		"",
		"export const logic = kea([",
		"    path(['logic']),",
		"    reducers({",
		"        allProducts: [getDefaultTreeProducts().sort((a, b) => a.path.localeCompare(b.path || 'b')), {}],",
		"    }),",
		"])",
	}, "\n")
	if err := os.WriteFile(logicFile, []byte(logicSource), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: tempDir,
		ConfigFile: tsconfigPath,
		File:       logicFile,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if reducer, ok := findParsedField(logic.Reducers, "allProducts"); !ok || reducer.Type != "FileSystemImport[]" {
		t.Fatalf("expected allProducts reducer type FileSystemImport[], got %+v", logic.Reducers)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"allProducts: FileSystemImport[]",
		"allProducts: (state: any, props?: any) => FileSystemImport[]",
	} {
		if !typegenAssertionContains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestSourceSelectorInferredTypeRecoversReducedWriteRoundDerivedSelector(t *testing.T) {
	source := mustReadFile(t, filepath.Join(repoRoot(t), "samples", "logic.ts"))

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}

	selectorsProperty := mustFindLogicProperty(t, logics[0], "selectors")
	sourceMembers := sectionSourceProperties(source, selectorsProperty)
	capitalizedName, ok := sourceMembers["capitalizedName"]
	if !ok {
		t.Fatalf("expected capitalizedName source selector, got %+v", sourceMembers)
	}

	inferred := sourceSelectorInferredType(ParsedLogic{
		Reducers: []ParsedField{
			{Name: "name", Type: "string"},
			{Name: "number", Type: "number"},
		},
	}, source, filepath.Join(repoRoot(t), "samples", "logic.ts"), sourcePropertyText(source, capitalizedName), nil)
	if inferred != "string" {
		t.Fatalf("expected inferred capitalizedName selector type string, got %q", inferred)
	}
}

func TestBuildParsedLogicsWindowValuesSample(t *testing.T) {
	report := inspectSampleReport(t, "windowValuesLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if !hasReducer(logic.Reducers, "windowHeight", "number") || !hasReducer(logic.Reducers, "windowWidth", "number") {
		t.Fatalf("expected window values to be normalized into reducers, got %+v", logic.Reducers)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"windowHeight: number",
		"windowWidth: number",
		"windowHeight: (state: any, props?: any) => number",
		"windowWidth: (state: any, props?: any) => number",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsResolvesLocalConnectSections(t *testing.T) {
	report := inspectSampleReport(t, "autoImportLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if !hasAction(logic.Actions, "setRepositories") {
		t.Fatalf("expected connected action to be imported from githubLogic, got %+v", logic.Actions)
	}
	if selector, ok := findParsedField(logic.Selectors, "dashboard"); !ok || selector.Type != "Dashboard | null" {
		t.Fatalf("expected connected value dashboard: Dashboard | null, got %+v", logic.Selectors)
	}
	if !hasImport(logic.Imports, "./types", "Repository") || !hasImport(logic.Imports, "./types", "Dashboard") {
		t.Fatalf("expected connected imports from ./types, got %+v", logic.Imports)
	}
	if hasImport(logic.Imports, "kea/lib/playground/3.0/githubLogic", "Repository") {
		t.Fatalf("expected connected Repository import ownership to stay on ./types, got %+v", logic.Imports)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"setRepositories: (repositories: Repository[]) => void",
		"dashboard: (state: any, props?: any) => Dashboard | null",
		"dashboard: Dashboard | null",
		"import type { Dashboard, Repository } from './types'",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "kea/lib/playground/3.0/githubLogic") {
		t.Fatalf("expected rendered output to avoid synthetic package import for connected Repository:\n%s", rendered)
	}
}

func TestBuildParsedLogicsPrefersSymbolBackedConnectedValueTypes(t *testing.T) {
	report := inspectSampleReport(t, "githubConnectLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if selector, ok := findParsedField(logic.Selectors, "repositories"); !ok || selector.Type != "Repository[]" {
		t.Fatalf("expected connected repositories selector type Repository[], got %+v", selector)
	}
	if selector, ok := findParsedField(logic.Selectors, "isLoading"); !ok || selector.Type != "boolean" {
		t.Fatalf("expected connected isLoading selector type boolean, got %+v", selector)
	}
	if !hasImport(logic.Imports, "./types", "Repository") {
		t.Fatalf("expected Repository import from ./types, got %+v", logic.Imports)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"repositories: (state: any, props?: any) => Repository[]",
		"isLoading: (state: any, props?: any) => boolean",
		"repositories: Repository[]",
		"isLoading: boolean",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsPosthogBuilderPropsKeySampleRecoversKeyAndHelpers(t *testing.T) {
	report := inspectSampleReport(t, "posthogBuilderPropsKeyLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if logic.KeyType != "string" {
		t.Fatalf("expected key type %q, got %q", "string", logic.KeyType)
	}
	if helper, ok := findParsedFunction(logic.InternalSelectorTypes, "endpoint"); !ok || !strings.Contains(helper.FunctionType, "resource:") || !strings.Contains(helper.FunctionType, "resourceId: string") || !strings.Contains(helper.FunctionType, "'project' | 'feature_flag'") && !strings.Contains(helper.FunctionType, "'feature_flag' | 'project'") && !strings.Contains(helper.FunctionType, `"project" | "feature_flag"`) && !strings.Contains(helper.FunctionType, `"feature_flag" | "project"`) {
		t.Fatalf("expected endpoint helper to recover builder props types, got %+v", logic.InternalSelectorTypes)
	}
	if helper, ok := findParsedFunction(logic.InternalSelectorTypes, "humanReadableResource"); !ok || !strings.Contains(helper.FunctionType, "resource:") || !strings.Contains(helper.FunctionType, "'project' | 'feature_flag'") && !strings.Contains(helper.FunctionType, "'feature_flag' | 'project'") && !strings.Contains(helper.FunctionType, `"project" | "feature_flag"`) && !strings.Contains(helper.FunctionType, `"feature_flag" | "project"`) {
		t.Fatalf("expected humanReadableResource helper to recover builder props type, got %+v", logic.InternalSelectorTypes)
	}
	if selector, ok := findParsedField(logic.Selectors, "endpoint"); !ok || selector.Type != "string" {
		t.Fatalf("expected endpoint selector type %q, got %+v", "string", selector)
	}
	if selector, ok := findParsedField(logic.Selectors, "humanReadableResource"); !ok || selector.Type != "string" {
		t.Fatalf("expected humanReadableResource selector type %q, got %+v", "string", selector)
	}
}

func TestBuildParsedLogicsPosthogMapConnectSampleRecoversConnectedSelectorHelper(t *testing.T) {
	report := inspectSampleReport(t, "posthogMapConnectLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	helper, ok := findParsedFunction(logic.InternalSelectorTypes, "groupNames")
	if !ok {
		t.Fatalf("expected groupNames helper, got %+v", logic.InternalSelectorTypes)
	}
	if !strings.Contains(helper.FunctionType, "Map<") || !strings.Contains(helper.FunctionType, "GroupType") || !strings.HasSuffix(helper.FunctionType, ") => string[]") {
		t.Fatalf("expected groupNames helper to preserve connected Map type, got %q", helper.FunctionType)
	}
}

func TestBuildParsedLogicsGithubSamplePreservesBooleanReducerState(t *testing.T) {
	report := inspectSampleReport(t, "githubLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if !hasReducer(logic.Reducers, "isLoading", "boolean") {
		t.Fatalf("expected isLoading reducer type boolean, got %+v", logic.Reducers)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	selectorBlock := extractInterfaceBlock(t, rendered, "selector", "selectors")
	if !strings.Contains(selectorBlock, "isLoading: boolean") {
		t.Fatalf("expected aggregate selector block to include boolean isLoading:\n%s", selectorBlock)
	}
	if strings.Contains(selectorBlock, "sortedRepositories") {
		t.Fatalf("expected aggregate selector block to exclude derived selectors:\n%s", selectorBlock)
	}
	if strings.Contains(rendered, "isLoading: false") {
		t.Fatalf("expected emitted output to avoid literal false reducer state:\n%s", rendered)
	}
}

func TestBuildParsedLogicsAutoImportRichTypes(t *testing.T) {
	report := inspectSampleReport(t, "autoImportLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if selector, ok := findParsedField(logic.Selectors, "sbla"); !ok || selector.Type != "Partial<Record<string, S7>>" {
		t.Fatalf("expected selector sbla: Partial<Record<string, S7>>, got %+v", logic.Selectors)
	}
	for _, expectedImport := range []string{"D6", "R6", "S7", "RandomThing"} {
		if !hasImport(logic.Imports, "./autoImportTypes", expectedImport) {
			t.Fatalf("expected %s import from ./autoImportTypes, got %+v", expectedImport, logic.Imports)
		}
	}
	if len(logic.InternalSelectorTypes) != 1 || logic.InternalSelectorTypes[0].Name != "sbla" || logic.InternalSelectorTypes[0].FunctionType != "(arg: S6) => Partial<Record<string, S7>>" {
		t.Fatalf("expected sbla internal selector helper, got %+v", logic.InternalSelectorTypes)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"sbla: (state: any, props?: any) => Partial<Record<string, S7>>",
		"sbla: Partial<Record<string, S7>>",
		"__keaTypeGenInternalSelectorTypes: {",
		"sbla: (arg: S6) => Partial<Record<string, S7>>",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestExpandImportedTypeAliasTextResolvesRelativeAlias(t *testing.T) {
	root := repoRoot(t)
	sourceFile := filepath.Join(root, "samples", "autoImportLogic.ts")
	source := mustReadFile(t, sourceFile)
	importedSource := mustReadFile(t, filepath.Join(root, "samples", "autoImportTypes.ts"))

	if expanded := expandLocalSourceTypeText(importedSource, "S6"); expanded != "Partial<Record<string, S7>>" {
		t.Fatalf("expected local alias S6 to expand inside autoImportTypes.ts, got %q", expanded)
	}

	expanded := expandImportedTypeAliasText(source, sourceFile, "S6")
	if expanded != "Partial<Record<string, S7>>" {
		t.Fatalf("expected imported alias S6 to expand to Partial<Record<string, S7>>, got %q", expanded)
	}
}

func TestBuildParsedLogicsPreservesSourceAliases(t *testing.T) {
	report := inspectSampleReport(t, "autoImportLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]

	if action, ok := findParsedAction(logic.Actions, "actionA1"); !ok || !strings.Contains(action.FunctionType, "local1: L1") || !strings.Contains(action.FunctionType, "param2: A2") {
		t.Fatalf("expected actionA1 to preserve source aliases, got %+v", action)
	}
	if action, ok := findParsedAction(logic.Actions, "complexAction"); !ok || !strings.Contains(action.FunctionType, "timeout: NodeJS.Timeout") {
		t.Fatalf("expected complexAction timeout to preserve NodeJS.Timeout, got %+v", action)
	}
	if action, ok := findParsedAction(logic.Actions, "miscActionWithType"); !ok || !strings.Contains(action.FunctionType, "ExportedApi.RandomThing") || !strings.Contains(action.PayloadType, "ExportedApi.RandomThing") {
		t.Fatalf("expected miscActionWithType to preserve ExportedApi.RandomThing, got %+v", action)
	}
	if action, ok := findParsedAction(logic.Actions, "miscActionWithoutType"); !ok || !strings.Contains(action.FunctionType, "ExportedApi.RandomThing") || !strings.Contains(action.PayloadType, "RandomThing") {
		t.Fatalf("expected miscActionWithoutType to preserve source parameter alias and inferred payload, got %+v", action)
	}

	for _, expected := range []struct {
		name string
		typ  string
	}{
		{name: "asd", typ: "D6"},
		{name: "notimported", typ: "Bla"},
		{name: "rasd", typ: "R6"},
		{name: "then", typ: "null | ExportedApi.RandomThing"},
	} {
		if field, ok := findParsedField(logic.Reducers, expected.name); !ok || field.Type != expected.typ {
			t.Fatalf("expected reducer %s: %s, got %+v", expected.name, expected.typ, field)
		}
	}

	if selector, ok := findParsedField(logic.Selectors, "randomSpecifiedReturn"); !ok || selector.Type != "ExportedApi.RandomThing" {
		t.Fatalf("expected selector randomSpecifiedReturn: ExportedApi.RandomThing, got %+v", selector)
	}

	for _, expectedImport := range []struct {
		path string
		name string
	}{
		{path: "./autoImportLogic", name: "L1"},
		{path: "./donotimport", name: "Bla"},
		{path: "./autoImportTypes", name: "D6"},
		{path: "./autoImportTypes", name: "ExportedApi"},
		{path: "./autoImportTypes", name: "R6"},
		{path: "./autoImportTypes", name: "S6"},
	} {
		if !hasImport(logic.Imports, expectedImport.path, expectedImport.name) {
			t.Fatalf("expected import %s from %s, got %+v", expectedImport.name, expectedImport.path, logic.Imports)
		}
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"local1: L1",
		"param2: A2",
		"timeout: NodeJS.Timeout",
		"miscActionWithType: (randomThing: ExportedApi.RandomThing) => void",
		"import type { Bla } from './donotimport'",
		"notimported: Bla",
		"rasd: R6",
		"randomSpecifiedReturn: (state: any, props?: any) => ExportedApi.RandomThing",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsNormalizesPluginSections(t *testing.T) {
	report := inspectSampleReport(t, "pluginLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 2 {
		t.Fatalf("expected 2 parsed logics, got %d", len(logics))
	}

	pluginLogic := logics[0]
	if hasAction(pluginLogic.Actions, "inlineAction") || !hasAction(pluginLogic.Actions, "submitForm") {
		t.Fatalf("expected only form plugin action to survive, got %+v", pluginLogic.Actions)
	}
	if _, ok := findParsedField(pluginLogic.Reducers, "inlineReducer"); ok {
		t.Fatalf("expected inline plugin reducer to stay deferred, got %+v", pluginLogic.Reducers)
	}
	if reducer, ok := findParsedField(pluginLogic.Reducers, "form"); !ok || !strings.Contains(reducer.Type, "name: string") || !strings.Contains(reducer.Type, "age: number") {
		t.Fatalf("expected form reducer type from plugin form defaults, got %+v", pluginLogic.Reducers)
	}
	if pluginLogic.ExtraInputForm == "" {
		t.Fatalf("expected plugin form extra input type, got %+v", pluginLogic)
	}

	anotherPluginLogic := logics[1]
	if !hasAction(anotherPluginLogic.Actions, "submitForm") {
		t.Fatalf("expected submitForm action for second plugin logic, got %+v", anotherPluginLogic.Actions)
	}
	if reducer, ok := findParsedField(anotherPluginLogic.Reducers, "form"); !ok || !strings.Contains(reducer.Type, "name: string") || !strings.Contains(reducer.Type, "age: number") {
		t.Fatalf("expected second form reducer type from plugin form defaults, got %+v", anotherPluginLogic.Reducers)
	}
	if anotherPluginLogic.ExtraInputForm == "" {
		t.Fatalf("expected second plugin logic extra input type, got %+v", anotherPluginLogic)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"submitForm: () => void",
		"form: { name: string; age: number; }",
		"__keaTypeGenInternalExtraInput: {",
		"default?: Record<string, any>",
		"submit?: (form: { name: string; age: number; }) => void",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
	for _, unexpected := range []string{
		"inlineAction: () => void",
		"inlineReducer: { asd: boolean }",
	} {
		if strings.Contains(rendered, unexpected) {
			t.Fatalf("expected emitted output to defer inline plugin heuristics and omit %q:\n%s", unexpected, rendered)
		}
	}
}

func TestBuildParsedLogicsNormalizesTypedFormBuilder(t *testing.T) {
	report := inspectSampleReport(t, filepath.Join("typed-builder", "typedFormDemoLogic.ts"))

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if len(logic.Actions) != 0 || len(logic.Reducers) != 0 || logic.CustomType != "" || logic.ExtraInputForm != "" {
		t.Fatalf("expected typedForm builder heuristics to stay deferred, got %+v", logic)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, unexpected := range []string{
		"submitForm: () => void",
		"form: Record<string, any>",
		"custom: {",
		"__keaTypeGenInternalExtraInput: {",
	} {
		if strings.Contains(rendered, unexpected) {
			t.Fatalf("expected emitted output to defer typedForm builder heuristics and omit %q:\n%s", unexpected, rendered)
		}
	}
}

func TestBuildParsedLogicsEmitsSampleBuilderFormsPluginPublicSurface(t *testing.T) {
	report := inspectSampleReport(t, "builderLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	for _, expectedAction := range []string{
		"setMyFormValue",
		"setMyFormValues",
		"resetMyForm",
		"submitMyForm",
		"submitMyFormRequest",
		"submitMyFormSuccess",
		"submitMyFormFailure",
		"setMyFormManualErrors",
		"touchMyFormField",
	} {
		if !hasAction(logic.Actions, expectedAction) {
			t.Fatalf("expected sample builder forms action %s, got %+v", expectedAction, logic.Actions)
		}
	}
	for _, expectedReducer := range []string{
		"myForm",
		"isMyFormSubmitting",
		"showMyFormErrors",
		"myFormChanged",
		"myFormTouches",
		"myFormManualErrors",
	} {
		if _, ok := findParsedField(logic.Reducers, expectedReducer); !ok {
			t.Fatalf("expected sample builder forms reducer %s, got %+v", expectedReducer, logic.Reducers)
		}
	}
	if reducer, ok := findParsedField(logic.Reducers, "myForm"); !ok || !strings.Contains(reducer.Type, "asd: true") || !strings.Contains(reducer.Type, "key39: string") {
		t.Fatalf("expected sample builder form reducer type to preserve literal defaults, got %+v", logic.Reducers)
	}
	for _, expectedSelector := range []string{
		"myFormTouched",
		"myFormValidationErrors",
		"myFormAllErrors",
		"myFormHasErrors",
		"myFormErrors",
		"isMyFormValid",
	} {
		if _, ok := findParsedField(logic.Selectors, expectedSelector); !ok {
			t.Fatalf("expected sample builder forms selector %s, got %+v", expectedSelector, logic.Selectors)
		}
	}
	for _, expectedImport := range []string{"DeepPartial", "DeepPartialMap", "FieldName", "ValidationErrorType"} {
		if !hasImport(logic.Imports, "kea-forms", expectedImport) {
			t.Fatalf("expected sample builder forms import %s, got %+v", expectedImport, logic.Imports)
		}
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"import type { DeepPartial, DeepPartialMap, FieldName, ValidationErrorType } from 'kea-forms'",
		"setMyFormValue: (key: FieldName, value: any) => void",
		"submitMyFormFailure: (error: Error, errors: Record<string, any>) => void",
		"myFormValidationErrors: (state: any, props?: any) => DeepPartialMap<",
		"isMyFormValid: (state: any, props?: any) => boolean",
		"myForm: {",
		"asd: true",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestSourceObjectLiteralTypeTextWithHintsOptionsPreservesLiteralProperties(t *testing.T) {
	source := strings.Join([]string{
		"type DashboardItemType = { key1: string }",
		"const item = {} as any as DashboardItemType",
	}, "\n")

	got := sourceObjectLiteralTypeTextWithHintsOptions(source, "{ ...item, asd: true }", nil, true)
	if got != "{ asd: true; key1: string; }" {
		t.Fatalf("expected preserved literal object type, got %q", got)
	}

	widened := sourceObjectLiteralTypeTextWithHints(source, "{ ...item, asd: true }", nil)
	if widened != "{ asd: boolean; key1: string; }" {
		t.Fatalf("expected widened literal object type, got %q", widened)
	}
}

func TestSourceActionPayloadTypeFromSourcePreservesExplicitAliasParameterTypes(t *testing.T) {
	source := strings.Join([]string{
		"type NodeData = { kind: string }",
		"type NodeType = 'app'",
		"type Node<TData, TType> = { id: string; data: TData; type: TType }",
		"type DiagramNode = Node<NodeData, NodeType>",
	}, "\n")

	got := sourceActionPayloadTypeFromSource(source, "", "(nodes: DiagramNode[]) => ({ nodes })", nil)
	if got != "{ nodes: DiagramNode[]; }" {
		t.Fatalf("expected aliased action payload type, got %q", got)
	}
}

func TestBuildParsedLogicsEmitsBuilderFormsPluginOutputsFromFormsSection(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path } from 'kea'",
		"import { forms } from 'kea-forms'",
		"",
		"export interface LoginLogicForm {",
		"    email: string",
		"    password: string",
		"}",
		"",
		"export const loginLogic = kea([",
		"    path(['src', 'scenes', 'login', 'loginForm']),",
		"    forms(() => ({",
		"        loginForm: {",
		"            defaults: { email: '', password: '' } as LoginLogicForm,",
		"            options: { showErrorsOnTouch: true, canSubmitWithErrors: true },",
		"            errors: (frame: Partial<LoginLogicForm>) => ({",
		"                email: frame.email ? null : 'missing',",
		"                password: frame.password ? null : 'missing',",
		"            }),",
		"            submit: async () => undefined,",
		"        },",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/loginLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "loginLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "forms",
						Members: []MemberReport{
							{
								Name:       "loginForm",
								TypeString: "{ defaults: LoginLogicForm; options: { showErrorsOnTouch: true; canSubmitWithErrors: true; }; errors: (frame: Partial<LoginLogicForm>) => { email: string | null; password: string | null; }; submit: (frame: LoginLogicForm) => Promise<void>; }",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	for _, expectedAction := range []string{
		"setLoginFormValue",
		"setLoginFormValues",
		"setLoginFormManualErrors",
		"touchLoginFormField",
		"resetLoginForm",
		"submitLoginForm",
		"submitLoginFormRequest",
		"submitLoginFormSuccess",
		"submitLoginFormFailure",
	} {
		if !hasAction(logic.Actions, expectedAction) {
			t.Fatalf("expected builder forms action %s, got %+v", expectedAction, logic.Actions)
		}
	}
	if reducer, ok := findParsedField(logic.Reducers, "loginForm"); !ok || reducer.Type != "LoginLogicForm" {
		t.Fatalf("expected loginForm reducer type LoginLogicForm, got %+v", logic.Reducers)
	}
	if selector, ok := findParsedField(logic.Selectors, "loginFormValidationErrors"); !ok || selector.Type != "DeepPartialMap<LoginLogicForm, ValidationErrorType>" {
		t.Fatalf("expected loginFormValidationErrors selector type, got %+v", logic.Selectors)
	}
	for _, expectedImport := range []string{"DeepPartial", "DeepPartialMap", "FieldName", "ValidationErrorType"} {
		if !hasImport(logic.Imports, "kea-forms", expectedImport) {
			t.Fatalf("expected kea-forms import %s, got %+v", expectedImport, logic.Imports)
		}
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"import type { LoginLogicForm } from './loginLogic'",
		"import type { DeepPartial, DeepPartialMap, FieldName, ValidationErrorType } from 'kea-forms'",
		"setLoginFormValue: (key: FieldName, value: any) => void",
		"submitLoginFormFailure: (error: Error, errors: Record<string, any>) => void",
		"loginFormValidationErrors: (state: any, props?: any) => DeepPartialMap<LoginLogicForm, ValidationErrorType>",
		"isLoginFormValid: (state: any, props?: any) => boolean",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsEmitsSmallInlineBuilderFormsPluginOutputs(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path } from 'kea'",
		"import { forms } from 'kea-forms'",
		"",
		"export const scenesLogic = kea([",
		"    path(['src', 'scenes', 'frame', 'panels', 'Scenes', 'scenesLogic']),",
		"    forms(() => ({",
		"        newScene: {",
		"            defaults: { name: '' },",
		"            submit: async (newScene) => {",
		"                console.log(newScene)",
		"            },",
		"        },",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/scenesLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "scenesLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "forms",
						Members: []MemberReport{
							{
								Name:       "newScene",
								TypeString: "{ defaults: { name: string; }; errors: (values: { name: string; }) => { name: string | undefined; }; submit: ({ name }: { name: string; }, breakpoint: BreakPointFunction) => void; }",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	for _, expectedAction := range []string{
		"setNewSceneValue",
		"submitNewScene",
		"submitNewSceneFailure",
	} {
		if !hasAction(logic.Actions, expectedAction) {
			t.Fatalf("expected inline builder forms action %s, got %+v", expectedAction, logic.Actions)
		}
	}
	if reducer, ok := findParsedField(logic.Reducers, "newScene"); !ok || reducer.Type != "{ name: string; }" {
		t.Fatalf("expected newScene reducer type, got %+v", logic.Reducers)
	}
	if selector, ok := findParsedField(logic.Selectors, "newSceneValidationErrors"); !ok || selector.Type != "DeepPartialMap<{ name: string; }, ValidationErrorType>" {
		t.Fatalf("expected newSceneValidationErrors selector type, got %+v", logic.Selectors)
	}
	for _, expectedImport := range []string{"DeepPartial", "DeepPartialMap", "FieldName", "ValidationErrorType"} {
		if !hasImport(logic.Imports, "kea-forms", expectedImport) {
			t.Fatalf("expected kea-forms import %s, got %+v", expectedImport, logic.Imports)
		}
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"import type { DeepPartial, DeepPartialMap, FieldName, ValidationErrorType } from 'kea-forms'",
		"setNewSceneValue: (key: FieldName, value: any) => void",
		"newSceneValidationErrors: (state: any, props?: any) => DeepPartialMap<{ name: string; }, ValidationErrorType>",
		"isNewSceneValid: (state: any, props?: any) => boolean",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsEmitsParameterizedBuilderFormsPluginOutputs(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path } from 'kea'",
		"import { forms } from 'kea-forms'",
		"",
		"export interface LoginLogicForm {",
		"    email: string",
		"    password: string",
		"}",
		"",
		"export const loginLogic = kea([",
		"    path(['src', 'scenes', 'login', 'loginForm']),",
		"    forms(({ actions }) => ({",
		"        loginForm: {",
		"            defaults: { email: '', password: '' } as LoginLogicForm,",
		"            errors: (frame: Partial<LoginLogicForm>) => ({",
		"                email: frame.email ? null : 'missing',",
		"                password: frame.password ? null : 'missing',",
		"            }),",
		"            submit: async (frame) => {",
		"                actions.setLoginFormManualErrors({ password: frame.password })",
		"            },",
		"        },",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/loginLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "loginLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "forms",
						Members: []MemberReport{
							{
								Name:       "loginForm",
								TypeString: "{ defaults: LoginLogicForm; errors: (frame: Partial<LoginLogicForm>) => { email: string | null; password: string | null; }; submit: (frame: LoginLogicForm) => Promise<void>; }",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	for _, expectedAction := range []string{
		"setLoginFormValue",
		"setLoginFormValues",
		"setLoginFormManualErrors",
		"submitLoginFormFailure",
	} {
		if !hasAction(logic.Actions, expectedAction) {
			t.Fatalf("expected parameterized builder forms action %s, got %+v", expectedAction, logic.Actions)
		}
	}
	if reducer, ok := findParsedField(logic.Reducers, "loginForm"); !ok || reducer.Type != "LoginLogicForm" {
		t.Fatalf("expected loginForm reducer type, got %+v", logic.Reducers)
	}
	if selector, ok := findParsedField(logic.Selectors, "loginFormValidationErrors"); !ok || selector.Type != "DeepPartialMap<LoginLogicForm, ValidationErrorType>" {
		t.Fatalf("expected loginFormValidationErrors selector type, got %+v", logic.Selectors)
	}
	for _, expectedImport := range []string{"DeepPartial", "DeepPartialMap", "FieldName", "ValidationErrorType"} {
		if !hasImport(logic.Imports, "kea-forms", expectedImport) {
			t.Fatalf("expected kea-forms import %s, got %+v", expectedImport, logic.Imports)
		}
	}
}

func TestBuildParsedLogicsPreservesExplicitReducerTypesOverBuilderFormsDefaults(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path, reducers } from 'kea'",
		"import { forms } from 'kea-forms'",
		"",
		"export interface FrameType {",
		"    name: string",
		"}",
		"",
		"export const frameLogic = kea([",
		"    path(['src', 'scenes', 'frame', 'frameLogic']),",
		"    forms(() => ({",
		"        frameForm: {",
		"            defaults: {} as FrameType,",
		"            submit: async (frame) => frame,",
		"        },",
		"    })),",
		"    reducers({",
		"        frameForm: [",
		"            {} as Partial<FrameType>,",
		"            {",
		"                setDeployWithAgent: (state) => state,",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/frameLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "frameLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "forms",
						Members: []MemberReport{
							{
								Name:       "frameForm",
								TypeString: "{ defaults: FrameType; submit: (frame: FrameType) => Promise<FrameType>; }",
							},
						},
					},
					{
						Name: "reducers",
						Members: []MemberReport{
							{
								Name:       "frameForm",
								TypeString: "[Partial<FrameType>, { setDeployWithAgent: (_: Partial<FrameType>) => Partial<FrameType>; }]",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if reducer, ok := findParsedField(logic.Reducers, "frameForm"); !ok || reducer.Type != "Partial<FrameType>" {
		t.Fatalf("expected explicit reducer type to win over forms default, got %+v", logic.Reducers)
	}
	if selector, ok := findParsedField(logic.Selectors, "frameFormValidationErrors"); !ok || selector.Type != "DeepPartialMap<FrameType, ValidationErrorType>" {
		t.Fatalf("expected builder forms companion selector, got %+v", logic.Selectors)
	}
	for _, expectedImport := range []string{"DeepPartial", "DeepPartialMap", "FieldName", "ValidationErrorType"} {
		if !hasImport(logic.Imports, "kea-forms", expectedImport) {
			t.Fatalf("expected kea-forms import %s, got %+v", expectedImport, logic.Imports)
		}
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(rendered, "frameForm: Partial<FrameType>") {
		t.Fatalf("expected emitted output to contain %q:\n%s", "frameForm: Partial<FrameType>", rendered)
	}
	if !strings.Contains(rendered, "frameFormValidationErrors: (state: any, props?: any) => DeepPartialMap<FrameType, ValidationErrorType>") {
		t.Fatalf("expected emitted output to contain builder forms companion selectors:\n%s", rendered)
	}
}

func TestBuildParsedLogicsRecoversInterfaceBackedBuilderPropsKeyButKeepsReportedAnySelectorType(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, key, path, props, selectors } from 'kea'",
		"",
		"export interface DemoLogicProps {",
		"    frameId: number",
		"}",
		"",
		"export const demoLogic = kea([",
		"    path(['demoLogic']),",
		"    props({} as DemoLogicProps),",
		"    key((props) => props.frameId),",
		"    selectors({",
		"        frameId: [() => [(_, props) => props.frameId], (frameId) => frameId],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/demoLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "demoLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{Name: "props", EffectiveTypeString: "DemoLogicProps"},
					{Name: "key", EffectiveTypeString: "any"},
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "frameId",
								TypeString:       "[(s: any) => [(_: any, props: any) => any], (frameId: any) => any]",
								ReturnTypeString: "any",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	sourceLogics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	if len(sourceLogics) != 1 {
		t.Fatalf("expected 1 source logic, got %d", len(sourceLogics))
	}
	keyProperty := mustFindLogicProperty(t, sourceLogics[0], "key")
	directKeyType := sourceKeyTypeFromProps(source, keyProperty, "DemoLogicProps")
	selectorsProperty := mustFindLogicProperty(t, sourceLogics[0], "selectors")
	selectorMembers := sectionSourceProperties(source, selectorsProperty)
	frameIDProperty, ok := selectorMembers["frameId"]
	if !ok {
		t.Fatalf("expected frameId selector source property, got %+v", selectorMembers)
	}
	frameIDExpression := sourcePropertyText(source, frameIDProperty)
	directDependencyTypes := sourceSelectorDependencyTypes(ParsedLogic{PropsType: "DemoLogicProps"}, source, "", frameIDExpression, nil)
	directSelectorType := sourceSelectorInferredType(ParsedLogic{PropsType: "DemoLogicProps"}, source, "", frameIDExpression, nil)
	directInternalSelectorType := sourceInternalSelectorFunctionType(ParsedLogic{PropsType: "DemoLogicProps"}, source, "", frameIDExpression, nil)
	directNormalizedSelectorType := normalizePublicRecoveredSelectorType(ParsedLogic{PropsType: "DemoLogicProps"}, source, "", frameIDExpression, "any", "number", nil)
	directParsedSelectors := parseSelectorsWithSource(report.Logics[0].Sections[2], ParsedLogic{PropsType: "DemoLogicProps"}, source, selectorsProperty, "", nil)
	if logic.KeyType != "number" {
		t.Fatalf("expected key type %q, got %q (direct source recovery: %q)", "number", logic.KeyType, directKeyType)
	}
	if selector, ok := findParsedField(logic.Selectors, "frameId"); !ok || selector.Type != "any" {
		t.Fatalf("expected frameId selector type %q, got %+v (dependency types: %+v, direct selector type: %q, internal selector type: %q, normalized selector type: %q, direct parsed selectors: %+v)", "any", logic.Selectors, directDependencyTypes, directSelectorType, directInternalSelectorType, directNormalizedSelectorType, directParsedSelectors)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"key: number",
		"frameId: (state: any, props?: any) => any",
		"frameId: any",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsUsesSourceListenerEntriesAndOmitsUnresolvedLocalListeners(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, listeners, path } from 'kea'",
		"",
		"export const listenerDemoLogic = kea([",
		"    path(['listenerDemoLogic']),",
		"    actions({",
		"        updateName: (name: string) => ({ name }),",
		"    }),",
		"    listeners(({ actions, values }) => ({",
		"        updateName: ({ name }) => {",
		"            actions.updateName(name)",
		"        },",
		"        setQuickFilterValue: ({ name, value }) => {",
		"            console.log(name, value, values)",
		"        },",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/listenerDemoLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "listenerDemoLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{
								Name:             "updateName",
								TypeString:       "(name: string) => { name: string; }",
								ReturnTypeString: "{ name: string; }",
							},
						},
					},
					{
						Name: "listeners",
						Members: []MemberReport{
							{Name: "actions", TypeString: "{ updateName: (name: string) => void; }"},
							{Name: "values", TypeString: "{ name: string; }"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	listener, ok := findParsedListener(logic.Listeners, "updateName")
	if !ok {
		t.Fatalf("expected source-backed listener for updateName, got %+v", logic.Listeners)
	}
	if listener.PayloadType != "{ name: string; }" {
		t.Fatalf("expected updateName listener payload type { name: string; }, got %+v", listener)
	}
	for _, unexpected := range []string{"actions", "values", "setQuickFilterValue"} {
		if _, ok := findParsedListener(logic.Listeners, unexpected); ok {
			t.Fatalf("expected unresolved local listener %q to be omitted, got %+v", unexpected, logic.Listeners)
		}
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"updateName: ((action: { type: 'update name (listenerDemoLogic)'; payload: { name: string; } }, previousState: any) => void | Promise<void>)[]",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
	for _, unexpected := range []string{
		"setQuickFilterValue: ((action: { type: string; payload: any }, previousState: any) => void | Promise<void>)[]",
		"actions: ((action: { type: string; payload: any }, previousState: any) => void | Promise<void>)[]",
		"values: ((action: { type: string; payload: any }, previousState: any) => void | Promise<void>)[]",
	} {
		if strings.Contains(rendered, unexpected) {
			t.Fatalf("expected emitted output to omit %q:\n%s", unexpected, rendered)
		}
	}
}

func TestBuildParsedLogicsRecoversNavigationStyleListenersMissingFromSectionMembers(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, listeners, path } from 'kea'",
		"",
		"export const listenerRecoveryLogic = kea([",
		"    path(['listenerRecoveryLogic']),",
		"    actions({",
		"        focusNextItem: () => ({ value: true }),",
		"        focusPreviousItem: () => ({ value: true }),",
		"        syncSidebarWidthWithViewport: () => ({ value: true }),",
		"    }),",
		"    listeners(({ actions, values }) => ({",
		"        focusNextItem: () => {",
		"            const nextIndex = values.lastFocusedItemIndex + 1",
		"            if (nextIndex < values.sidebarContentsFlattened.length) {",
		"                actions.focusNextItem()",
		"            }",
		"        },",
		"        focusPreviousItem: () => {",
		"            const nextIndex = values.lastFocusedItemIndex - 1",
		"            if (nextIndex >= -1) {",
		"                actions.focusPreviousItem()",
		"            }",
		"        },",
		"        syncSidebarWidthWithViewport: () => {",
		"            if (values.sidebarWidth > window.innerWidth * (50 / 100)) {",
		"                actions.syncSidebarWidthWithViewport()",
		"            }",
		"        },",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/listenerRecoveryLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "listenerRecoveryLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{Name: "focusNextItem", TypeString: "() => { value: true }", ReturnTypeString: "{ value: true }"},
							{Name: "focusPreviousItem", TypeString: "() => { value: true }", ReturnTypeString: "{ value: true }"},
							{Name: "syncSidebarWidthWithViewport", TypeString: "() => { value: true }", ReturnTypeString: "{ value: true }"},
						},
					},
					{
						Name: "listeners",
						Members: []MemberReport{
							{Name: "focusNextItem", TypeString: "() => void", ReturnTypeString: "void"},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	for _, expected := range []string{"focusNextItem", "focusPreviousItem", "syncSidebarWidthWithViewport"} {
		listener, ok := findParsedListener(logic.Listeners, expected)
		if !ok {
			t.Fatalf("expected source-backed listener for %q, got %+v", expected, logic.Listeners)
		}
		if listener.PayloadType != "{ value: true; }" {
			t.Fatalf("expected %s payload type { value: true; }, got %+v", expected, listener)
		}
	}
}

func TestBuildParsedLogicsRecoversListenerMethodShorthandFromArrowReturnedObject(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, listeners, path } from 'kea'",
		"",
		"export const discussionLogic = kea([",
		"    path(['discussionLogic']),",
		"    actions({",
		"        scrollToLastComment: true,",
		"    }),",
		"    listeners(({ values }) => ({",
		"        scrollToLastComment() {",
		"            console.log(values)",
		"        },",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/discussionLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "discussionLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{Name: "scrollToLastComment", TypeString: "() => { value: true; }", ReturnTypeString: "{ value: true; }"},
						},
					},
					{
						Name:    "listeners",
						Members: nil,
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	listener, ok := findParsedListener(logics[0].Listeners, "scrollToLastComment")
	if !ok {
		t.Fatalf("expected listener method shorthand to be recovered, got %+v", logics[0].Listeners)
	}
	payload, ok := parseActionPayloadObjectMembers(listener.PayloadType)
	if !ok || len(payload) != 1 || payload["value"].Type != "true" {
		t.Fatalf("expected scrollToLastComment payload type { value: true; }, got %+v", listener)
	}
}

func TestBuildParsedLogicsRecoversListenersFromBlockBodyReturnedObject(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, listeners, path } from 'kea'",
		"",
		"export const workflowLogic = kea([",
		"    path(['workflowLogic']),",
		"    actions({",
		"        start: true,",
		"        stop: true,",
		"    }),",
		"    listeners(({ actions }) => {",
		"        let timeout: ReturnType<typeof setTimeout> | null = null",
		"        return {",
		"            start: () => {",
		"                timeout = setTimeout(() => actions.stop(), 10)",
		"            },",
		"            stop() {",
		"                if (timeout) {",
		"                    clearTimeout(timeout)",
		"                    timeout = null",
		"                }",
		"            },",
		"        }",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/workflowLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "workflowLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{Name: "start", TypeString: "() => { value: true; }", ReturnTypeString: "{ value: true; }"},
							{Name: "stop", TypeString: "() => { value: true; }", ReturnTypeString: "{ value: true; }"},
						},
					},
					{
						Name:    "listeners",
						Members: nil,
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	for _, expected := range []string{"start", "stop"} {
		listener, ok := findParsedListener(logics[0].Listeners, expected)
		if !ok {
			t.Fatalf("expected block-body listener %q, got %+v", expected, logics[0].Listeners)
		}
		payload, ok := parseActionPayloadObjectMembers(listener.PayloadType)
		if !ok || len(payload) != 1 || payload["value"].Type != "true" {
			t.Fatalf("expected %s payload type { value: true; }, got %+v", expected, listener)
		}
	}
}

func TestBuildParsedLogicsRecoversSourceOnlySharedListenersFromBlockBody(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, listeners, path, sharedListeners } from 'kea'",
		"",
		"export const logic = kea([",
		"    path(['logic']),",
		"    sharedListeners(() => {",
		"        const noop = true",
		"        return {",
		"            saveUrls: async () => {",
		"                return noop",
		"            },",
		"        }",
		"    }),",
		"    listeners(({ sharedListeners }) => ({",
		"        triggerSave: sharedListeners.saveUrls,",
		"    })),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/logic.ts",
		Logics: []LogicReport{
			{
				Name:      "logic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name:    "sharedListeners",
						Members: nil,
					},
					{
						Name:    "listeners",
						Members: nil,
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	if len(logics[0].SharedListeners) != 1 {
		t.Fatalf("expected one shared listener, got %+v", logics[0].SharedListeners)
	}
	listener := logics[0].SharedListeners[0]
	if listener.Name != "saveUrls" {
		t.Fatalf("expected shared listener name saveUrls, got %+v", listener)
	}
	if listener.PayloadType != "any" {
		t.Fatalf("expected shared listener payload any, got %+v", listener)
	}
	if listener.ActionType != "{ type: string; payload: any }" {
		t.Fatalf("expected shared listener action type { type: string; payload: any }, got %+v", listener)
	}
}

func TestBuildParsedLogicsRecoversMissingSelectorNamesAndSkipsOpaqueBuilderHelpers(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path, selectors } from 'kea'",
		"",
		"type SidebarItem = { key: string }",
		"type SidebarNavbarItem = { logic: { findMounted: () => { selectors: { contents: (state: any) => SidebarItem[]; activeListItemKey?: (state: any) => string | number | string[] | null } } } }",
		"const Scene = { SQLEditor: 'sql-editor' } as const",
		"",
		"export const selectorRecoveryLogic = kea([",
		"    path(['selectorRecoveryLogic']),",
		"    selectors({",
		"        sidebarContentsFlattened: [",
		"            (s) => [(state) => s.activeNavbarItem(state)?.logic?.findMounted()?.selectors.contents(state) || null],",
		"            (sidebarContents): SidebarItem[] => {",
		"                return sidebarContents ? sidebarContents : []",
		"            },",
		"        ],",
		"        normalizedActiveListItemKey: [",
		"            (s) => [",
		"                (state) => s.activeNavbarItem(state)?.logic?.findMounted()?.selectors.activeListItemKey?.(state) || null,",
		"            ],",
		"            (activeListItemKey): string | number | string[] | null => {",
		"                return activeListItemKey ? activeListItemKey : null",
		"            },",
		"        ],",
		"        activeNavbarItemId: [",
		"            (s) => [s.activeNavbarItemIdRaw],",
		"            (activeNavbarItemIdRaw): string | null => {",
		"                if (activeNavbarItemIdRaw === Scene.SQLEditor) {",
		"                    return Scene.SQLEditor",
		"                }",
		"                return null",
		"            },",
		"        ],",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/selectorRecoveryLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "selectorRecoveryLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "selectors",
						Members: []MemberReport{
							{
								Name:             "sidebarContentsFlattened",
								TypeString:       "(((s: any) => ((state: any) => any)[]) | ((sidebarContents: any) => SidebarItem[]))[]",
								ReturnTypeString: "any",
							},
							{
								Name:             "normalizedActiveListItemKey",
								TypeString:       "(((s: any) => ((state: any) => any)[]) | ((activeListItemKey: any) => string | number | string[] | null))[]",
								ReturnTypeString: "(((s: any) => ((state: any) => any)[]) | ((activeListItemKey: any) => string | number | string[] | null))[]",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if selector, ok := findParsedField(logic.Selectors, "activeNavbarItemId"); !ok || selector.Type != "string | null" {
		t.Fatalf("expected activeNavbarItemId selector type %q, got %+v", "string | null", logic.Selectors)
	}
	for _, unexpected := range []string{"sidebarContentsFlattened", "normalizedActiveListItemKey"} {
		if _, ok := findParsedFunction(logic.InternalSelectorTypes, unexpected); ok {
			t.Fatalf("expected opaque builder helper %q to be omitted, got %+v", unexpected, logic.InternalSelectorTypes)
		}
	}
}

func TestBuildParsedLogicsPreservesNullableActionPayloadProperties(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"",
		"type DemoFlag = { id: string }",
		"",
		"export const nullableActionLogic = kea([",
		"    path(['nullableActionLogic']),",
		"    actions({",
		"        setLinkedFlag: (flag: DemoFlag | null) => ({ flag }),",
		"        setAutocompleteKey: (key: string | null) => ({ key }),",
		"        loadSomething: (endpoint: string | undefined, newInput: string | undefined) => ({ endpoint, newInput }),",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/nullableActionLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "nullableActionLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{
								Name:             "setLinkedFlag",
								TypeString:       "(flag: DemoFlag | null) => { flag: any; }",
								ReturnTypeString: "{ flag: any; }",
							},
							{
								Name:             "setAutocompleteKey",
								TypeString:       "(key: string | null) => { key: any; }",
								ReturnTypeString: "{ key: any; }",
							},
							{
								Name:             "loadSomething",
								TypeString:       "(endpoint: string | undefined, newInput: string | undefined) => { endpoint: any; newInput: any; }",
								ReturnTypeString: "{ endpoint: any; newInput: any; }",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	for _, expected := range []struct {
		name        string
		payloadType string
	}{
		{name: "setLinkedFlag", payloadType: "{ flag: DemoFlag | null; }"},
		{name: "setAutocompleteKey", payloadType: "{ key: string | null; }"},
		{name: "loadSomething", payloadType: "{ endpoint: string | undefined; newInput: string | undefined; }"},
	} {
		action, ok := findParsedAction(logic.Actions, expected.name)
		if !ok {
			t.Fatalf("expected action %q, got %+v", expected.name, logic.Actions)
		}
		if action.PayloadType != expected.payloadType {
			t.Fatalf("expected payload type %q for %s, got %+v", expected.payloadType, expected.name, action)
		}
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"setLinkedFlag: (flag: DemoFlag | null) => {",
		"payload: { flag: DemoFlag | null; }",
		"payload: { key: string | null; }",
		"payload: { endpoint: string | undefined; newInput: string | undefined; }",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsKeepsReportedActionPayloadAliasWhenSourceAddsNullish(t *testing.T) {
	source := strings.Join([]string{
		"import { actions, kea, path } from 'kea'",
		"",
		"type EditorSidebarTreeRef = { current: HTMLDivElement | null } | null",
		"",
		"export const queryDatabaseLogic = kea([",
		"    path(['queryDatabaseLogic']),",
		"    actions({",
		"        setTreeRef: (ref: EditorSidebarTreeRef | null) => ({ ref }),",
		"    }),",
		"])",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/queryDatabaseLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "queryDatabaseLogic",
				InputKind: "builders",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{
								Name:             "setTreeRef",
								TypeString:       "(ref: EditorSidebarTreeRef) => { ref: EditorSidebarTreeRef; }",
								ReturnTypeString: "{ ref: EditorSidebarTreeRef; }",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	action, ok := findParsedAction(logics[0].Actions, "setTreeRef")
	if !ok {
		t.Fatalf("expected setTreeRef action, got %+v", logics[0].Actions)
	}
	if action.FunctionType != "(ref: EditorSidebarTreeRef | null) => { ref: EditorSidebarTreeRef; }" {
		t.Fatalf("expected nullable source parameter with reported payload alias, got %+v", action)
	}
	if action.PayloadType != "{ ref: EditorSidebarTreeRef; }" {
		t.Fatalf("expected reported payload alias to survive source recovery, got %+v", action)
	}
}

func TestBuildParsedLogicsObjectActionsStripNullablePayloadProperties(t *testing.T) {
	source := strings.Join([]string{
		"import { kea } from 'kea'",
		"",
		"export const complexLikeLogic = kea({",
		"    actions: {",
		"        selectAction: (id: string | null) => ({ id: id || null }),",
		"        newAction: (element?: HTMLElement) => ({ element }),",
		"        inspectForElementWithIndex: (index: number | null) => ({ index }),",
		"        inspectElementSelected: (element: HTMLElement, index: number | null) => ({ element, index }),",
		"    },",
		"})",
	}, "\n")

	report := &Report{
		ProjectDir: "/tmp",
		File:       "/tmp/complexLikeLogic.ts",
		Logics: []LogicReport{
			{
				Name:      "complexLikeLogic",
				InputKind: "object",
				Sections: []SectionReport{
					{
						Name: "actions",
						Members: []MemberReport{
							{
								Name:             "selectAction",
								TypeString:       "(id: string | null) => { id: string | null; }",
								ReturnTypeString: "{ id: string | null; }",
							},
							{
								Name:             "newAction",
								TypeString:       "(element?: HTMLElement | undefined) => { element: HTMLElement | undefined; }",
								ReturnTypeString: "{ element: HTMLElement | undefined; }",
							},
							{
								Name:             "inspectForElementWithIndex",
								TypeString:       "(index: number | null) => { index: number | null; }",
								ReturnTypeString: "{ index: number | null; }",
							},
							{
								Name:             "inspectElementSelected",
								TypeString:       "(element: HTMLElement, index: number | null) => { element: HTMLElement; index: number | null; }",
								ReturnTypeString: "{ element: HTMLElement; index: number | null; }",
							},
						},
					},
				},
			},
		},
	}

	logics, err := BuildParsedLogicsFromSource(report, source)
	if err != nil {
		t.Fatalf("BuildParsedLogicsFromSource returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	for _, expected := range []struct {
		name        string
		payloadType string
	}{
		{name: "selectAction", payloadType: "{ id: string; }"},
		{name: "newAction", payloadType: "{ element: HTMLElement; }"},
		{name: "inspectForElementWithIndex", payloadType: "{ index: number; }"},
		{name: "inspectElementSelected", payloadType: "{ element: HTMLElement; index: number; }"},
	} {
		action, ok := findParsedAction(logic.Actions, expected.name)
		if !ok {
			t.Fatalf("expected action %q, got %+v", expected.name, logic.Actions)
		}
		if action.PayloadType != expected.payloadType {
			t.Fatalf("expected payload type %q for %s, got %+v", expected.payloadType, expected.name, action)
		}
	}
}

func TestBuildParsedLogicsResolvesExternalConnectActionsFromSymbols(t *testing.T) {
	report := inspectSampleReport(t, "routerConnectLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	logic := logics[0]

	action, ok := findParsedAction(logic.Actions, "locationChanged")
	if !ok {
		t.Fatalf("expected connected action for locationChanged, got %+v", logic.Actions)
	}
	if !strings.Contains(action.FunctionType, "LocationChangedPayload") {
		t.Fatalf("expected symbol-backed action type to preserve LocationChangedPayload, got %q", action.FunctionType)
	}
	if strings.Contains(action.PayloadType, "Record<...>") || !strings.Contains(action.PayloadType, "hashParams: Record<string, any>") {
		t.Fatalf("expected symbol-backed payload type to keep expanded Record<string, any> members, got %q", action.PayloadType)
	}
	if !hasImport(logic.Imports, "kea-router/lib/types", "LocationChangedPayload") {
		t.Fatalf("expected LocationChangedPayload import from kea-router/lib/types, got %+v", logic.Imports)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	if strings.Contains(rendered, "{ type: string; payload: any }") || strings.Contains(rendered, "Record<...>") {
		t.Fatalf("expected listener typing to avoid generic connect fallback:\n%s", rendered)
	}
	for _, expected := range []string{
		"import type { LocationChangedPayload } from 'kea-router/lib/types'",
		"locationChanged: ({ method, pathname, search, searchParams, hash, hashParams, initial, }: LocationChangedPayload) => void",
		"locationChanged: ((action: { type: 'location changed (containers.pages.main)'; payload:",
		"hashParams: Record<string, any>",
		"searchParams: Record<string, any>",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsResolvesNamespaceImportedConnectTargets(t *testing.T) {
	report := inspectSampleReport(t, "githubNamespaceConnectLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	logic := logics[0]

	action, ok := findParsedAction(logic.Actions, "setRepositories")
	if !ok {
		t.Fatalf("expected connected action for setRepositories, got %+v", logic.Actions)
	}
	if !strings.Contains(action.FunctionType, "Repository[]") {
		t.Fatalf("expected connected namespace action type to preserve Repository[], got %+v", action)
	}
	if selector, ok := findParsedField(logic.Selectors, "repositories"); !ok || selector.Type != "Repository[]" {
		t.Fatalf("expected connected namespace repositories selector type Repository[], got %+v", selector)
	}
	if selector, ok := findParsedField(logic.Selectors, "githubIsLoading"); !ok || selector.Type != "boolean" {
		t.Fatalf("expected renamed namespace selector githubIsLoading: boolean, got %+v", selector)
	}
	if !hasImport(logic.Imports, "./types", "Repository") {
		t.Fatalf("expected Repository import from ./types, got %+v", logic.Imports)
	}
	listener, ok := findParsedListener(logic.Listeners, "set repositories (githubLogic)")
	if !ok {
		t.Fatalf("expected namespace-imported listener for set repositories (githubLogic), got %+v", logic.Listeners)
	}
	if listener.PayloadType != "{ repositories: Repository[]; }" {
		t.Fatalf("expected namespace-imported listener payload type { repositories: Repository[]; }, got %+v", listener)
	}
	internalAction, ok := findParsedAction(logic.InternalReducerActions, "set repositories (githubLogic)")
	if !ok {
		t.Fatalf("expected internal reducer action for namespace-imported listener, got %+v", logic.InternalReducerActions)
	}
	if internalAction.FunctionType != "(repositories: Repository[]) => { repositories: Repository[]; }" {
		t.Fatalf("expected namespace-imported internal reducer action type to preserve Repository[], got %+v", internalAction)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"import type { Repository } from './types'",
		"setRepositories: (repositories: Repository[]) => void",
		"repositories: (state: any, props?: any) => Repository[]",
		"githubIsLoading: (state: any, props?: any) => boolean",
		"'set repositories (githubLogic)': ((action: { type: 'set repositories (githubLogic)'; payload: { repositories: Repository[]; } }, previousState: any) => void | Promise<void>)[]",
		"__keaTypeGenInternalReducerActions: {",
		"'set repositories (githubLogic)': (repositories: Repository[]) => {",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsSkipsBuilderReducerOnlyInternalReducerActions(t *testing.T) {
	logics := inspectTempLogicFile(t, map[string]string{
		"src/types.ts": strings.Join([]string{
			"export interface LogType {",
			"    id: number",
			"}",
		}, "\n"),
		"src/socketLogic.ts": strings.Join([]string{
			"import { kea, actions, path } from 'kea'",
			"import type { LogType } from './types'",
			"",
			"export const socketLogic = kea([",
			"    path(['src', 'socketLogic']),",
			"    actions({",
			"        newLog: (log: LogType) => ({ log }),",
			"    }),",
			"])",
		}, "\n"),
		"src/logsLogic.ts": strings.Join([]string{
			"import { kea, path, reducers } from 'kea'",
			"import { socketLogic } from './socketLogic'",
			"import type { LogType } from './types'",
			"",
			"export const logsLogic = kea([",
			"    path(['src', 'logsLogic']),",
			"    reducers(() => ({",
			"        logs: [",
			"            [] as LogType[],",
			"            {",
			"                [socketLogic.actionTypes.newLog]: (state, { log }) => [...state, log],",
			"            },",
			"        ],",
			"    })),",
			"])",
		}, "\n"),
	}, "src/logsLogic.ts")
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if len(logic.InternalReducerActions) != 0 {
		t.Fatalf("expected builder reducer-only imported actions to stay out of internal reducer actions, got %+v", logic.InternalReducerActions)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 18, 12, 0, 0, 0, time.UTC))
	if strings.Contains(rendered, "__keaTypeGenInternalReducerActions") {
		t.Fatalf("expected builder reducer-only imported actions to stay out of emitted internal reducer actions:\n%s", rendered)
	}
}

func TestBuildParsedLogicsKeepsBuilderListenerInternalReducerActions(t *testing.T) {
	logics := inspectTempLogicFile(t, map[string]string{
		"src/types.ts": strings.Join([]string{
			"export interface LogType {",
			"    id: number",
			"}",
		}, "\n"),
		"src/socketLogic.ts": strings.Join([]string{
			"import { kea, actions, path } from 'kea'",
			"import type { LogType } from './types'",
			"",
			"export const socketLogic = kea([",
			"    path(['src', 'socketLogic']),",
			"    actions({",
			"        newLog: (log: LogType) => ({ log }),",
			"        updateLog: (log: LogType) => ({ log }),",
			"    }),",
			"])",
		}, "\n"),
		"src/controlLogic.ts": strings.Join([]string{
			"import { kea, listeners, path, reducers } from 'kea'",
			"import { socketLogic } from './socketLogic'",
			"import type { LogType } from './types'",
			"",
			"export const controlLogic = kea([",
			"    path(['src', 'controlLogic']),",
			"    reducers(() => ({",
			"        logs: [",
			"            [] as LogType[],",
			"            {",
			"                [socketLogic.actionTypes.updateLog]: (state, { log }) => [...state, log],",
			"            },",
			"        ],",
			"    })),",
			"    listeners(() => ({",
			"        [socketLogic.actionTypes.newLog]: ({ log }) => {",
			"            console.log(log)",
			"        },",
			"    })),",
			"])",
		}, "\n"),
	}, "src/controlLogic.ts")
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if _, ok := findParsedListener(logic.Listeners, "new log (src.socketLogic)"); ok {
		t.Fatalf("did not expect builder imported listener to become public, got %+v", logic.Listeners)
	}
	for _, expected := range []struct {
		name         string
		functionType string
	}{
		{name: "new log (src.socketLogic)", functionType: "(log: LogType) => { log: LogType; }"},
		{name: "update log (src.socketLogic)", functionType: "(log: LogType) => { log: LogType; }"},
	} {
		internalAction, ok := findParsedAction(logic.InternalReducerActions, expected.name)
		if !ok {
			t.Fatalf("expected builder listener logic to keep internal reducer action %s, got %+v", expected.name, logic.InternalReducerActions)
		}
		if internalAction.FunctionType != expected.functionType {
			t.Fatalf("expected builder listener internal reducer action type %q for %s, got %+v", expected.functionType, expected.name, internalAction)
		}
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 18, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"__keaTypeGenInternalReducerActions: {",
		"'new log (src.socketLogic)': (log: LogType) => {",
		"'update log (src.socketLogic)': (log: LogType) => {",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "'new log (src.socketLogic)': ((action:") {
		t.Fatalf("did not expect builder imported listener to appear in public listeners:\n%s", rendered)
	}
}

func TestBuildParsedLogicsResolvesImportedActionTypeListeners(t *testing.T) {
	report := inspectSampleReport(t, "githubImportLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	logic := logics[0]

	listener, ok := findParsedListener(logic.Listeners, "set username (githubLogic)")
	if !ok {
		t.Fatalf("expected imported listener for set username (githubLogic), got %+v", logic.Listeners)
	}
	if listener.PayloadType != "{ username: string; }" {
		t.Fatalf("expected imported listener payload type { username: string; }, got %+v", listener)
	}
	if listener.ActionType != "{ type: 'set username (githubLogic)'; payload: { username: string; } }" {
		t.Fatalf("expected imported listener action type to be normalized, got %+v", listener)
	}
	if len(logic.SharedListeners) != 0 {
		t.Fatalf("expected generic shared listeners to be omitted, got %+v", logic.SharedListeners)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	if strings.Contains(rendered, "'set username (githubLogic)': ((action: { type: string; payload: any }, previousState: any) => void | Promise<void>)[]") {
		t.Fatalf("expected imported listener typing to avoid generic fallback:\n%s", rendered)
	}
	if strings.Contains(rendered, "import type { Logic, BreakPointFunction } from 'kea'") || strings.Contains(rendered, "'bla': (payload: any") {
		t.Fatalf("expected generic shared listeners to stay out of emitted output:\n%s", rendered)
	}
	for _, expected := range []string{
		"'set username (githubLogic)': ((action: { type: 'set username (githubLogic)'; payload: { username: string; } }, previousState: any) => void | Promise<void>)[]",
		"repositorySelectorCopy: (state: any, props?: any) => Repository[]",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsResolvesWildcardImportedActionTypeListeners(t *testing.T) {
	report := inspectSampleReport(t, "githubImportViaWildcardLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	logic := logics[0]

	listener, ok := findParsedListener(logic.Listeners, "set username (githubLogic)")
	if !ok {
		t.Fatalf("expected wildcard-imported listener for set username (githubLogic), got %+v", logic.Listeners)
	}
	if listener.PayloadType != "{ username: string; }" {
		t.Fatalf("expected wildcard-imported listener payload type { username: string; }, got %+v", listener)
	}
	if len(logic.SharedListeners) != 0 {
		t.Fatalf("expected generic wildcard shared listeners to be omitted, got %+v", logic.SharedListeners)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"import type { Logic } from 'kea'",
		"import type { Repository } from './wildcardExportTypes'",
		"'set username (githubLogic)': ((action: { type: 'set username (githubLogic)'; payload: { username: string; } }, previousState: any) => void | Promise<void>)[]",
		"repositorySelectorCopy: (state: any, props?: any) => Repository[]",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "BreakPointFunction") || strings.Contains(rendered, "'bla': (payload: any") {
		t.Fatalf("expected wildcard shared listeners to stay out of emitted output:\n%s", rendered)
	}
}

func TestBuildParsedLogicsNormalizesBooleanActionShorthands(t *testing.T) {
	report := inspectSampleReport(t, "complexLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	logic := logics[0]

	for _, expected := range []struct {
		name         string
		functionType string
		payloadType  string
	}{
		{name: "deleteAction", functionType: "() => { value: true }", payloadType: "{ value: true }"},
		{name: "hideButtonActions", functionType: "() => { value: true }", payloadType: "{ value: true }"},
		{name: "incrementCounter", functionType: "() => { value: true }", payloadType: "{ value: true }"},
		{name: "showButtonActions", functionType: "() => { value: true }", payloadType: "{ value: true }"},
	} {
		action, ok := findParsedAction(logic.Actions, expected.name)
		if !ok {
			t.Fatalf("expected shorthand action %s, got %+v", expected.name, logic.Actions)
		}
		if action.FunctionType != expected.functionType || action.PayloadType != expected.payloadType {
			t.Fatalf("expected shorthand action %s to normalize to %q / %q, got %+v", expected.name, expected.functionType, expected.payloadType, action)
		}
	}

	listener, ok := findParsedListener(logic.Listeners, "hideButtonActions")
	if !ok {
		t.Fatalf("expected local hideButtonActions listener, got %+v", logic.Listeners)
	}
	if listener.PayloadType != "{ value: true }" {
		t.Fatalf("expected hideButtonActions listener payload type { value: true }, got %+v", listener)
	}
	if listener.ActionType != "{ type: 'hide button actions (complexLogic)'; payload: { value: true } }" {
		t.Fatalf("expected hideButtonActions listener action type to use shorthand payload, got %+v", listener)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"deleteAction: () => {",
		"payload: { value: true }",
		"hideButtonActions: ((action: { type: 'hide button actions (complexLogic)'; payload: { value: true } }, previousState: any) => void | Promise<void>)[]",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsComplexSamplePreservesReducerUnionState(t *testing.T) {
	report := inspectSampleReport(t, "complexLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	logic := logics[0]

	for _, expected := range []struct {
		name string
		typ  string
	}{
		{name: "selectedActionId", typ: "number | 'new' | null"},
		{name: "newActionForElement", typ: "HTMLElement | null"},
		{name: "inspectingElement", typ: "number | null"},
	} {
		if field, ok := findParsedField(logic.Reducers, expected.name); !ok || field.Type != expected.typ {
			t.Fatalf("expected reducer %s: %s, got %+v", expected.name, expected.typ, logic.Reducers)
		}
	}
	for _, expected := range []struct {
		name         string
		functionType string
	}{
		{name: "selectedAction", functionType: "(selectedActionId: number | 'new' | null, newActionForElement: HTMLElement | null) => ActionType | null"},
		{name: "initialValuesForForm", functionType: "(selectedAction: ActionType | null) => ActionForm"},
		{name: "selectedEditedAction", functionType: "(selectedAction: ActionType | null, initialValuesForForm: ActionForm, form: FormInstance | null, editingFields: AntdFieldData[] | null, inspectingElement: number | null, counter: number) => ActionForm"},
	} {
		found := false
		for _, selector := range logic.InternalSelectorTypes {
			if selector.Name != expected.name {
				continue
			}
			found = true
			if selector.FunctionType != expected.functionType {
				t.Fatalf("expected internal selector helper %s: %s, got %+v", expected.name, expected.functionType, selector)
			}
		}
		if !found {
			t.Fatalf("expected internal selector helper %s, got %+v", expected.name, logic.InternalSelectorTypes)
		}
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"selectedActionId: number | 'new' | null",
		"selectedActionId: (state: any, props?: any) => number | 'new' | null",
		"newActionForElement: HTMLElement | null",
		"inspectingElement: number | null",
		"miscSelector: {",
		"key39: string",
		"miscSelector: (state: any, props?: any) => {",
		"__keaTypeGenInternalSelectorTypes: {",
		"selectedAction: (selectedActionId: number | 'new' | null, newActionForElement: HTMLElement | null) => ActionType | null",
		"initialValuesForForm: (selectedAction: ActionType | null) => ActionForm",
		"selectedEditedAction: (selectedAction: ActionType | null, initialValuesForForm: ActionForm, form: FormInstance | null, editingFields: AntdFieldData[] | null, inspectingElement: number | null, counter: number) => ActionForm",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "selectedActionId: string") {
		t.Fatalf("expected emitted output to avoid widened selectedActionId: string:\n%s", rendered)
	}
	if strings.Contains(rendered, "... 24 more ...") {
		t.Fatalf("expected complex selector object types to stay fully expanded, got summarized placeholder:\n%s", rendered)
	}
}

func TestBuildParsedLogicsBuilderLazyLoaders(t *testing.T) {
	report := inspectSampleReport(t, "builderLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	logic := logics[0]

	if !hasAction(logic.Actions, "initValue") || !hasAction(logic.Actions, "initValueSuccess") || !hasAction(logic.Actions, "initValueFailure") {
		t.Fatalf("expected synthesized lazy loader actions, got %+v", logic.Actions)
	}
	if !hasReducer(logic.Reducers, "lazyValue", "string") || !hasReducer(logic.Reducers, "lazyValueLoading", "boolean") {
		t.Fatalf("expected synthesized lazy loader reducers, got %+v", logic.Reducers)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"initValue: () => void",
		"initValueSuccess: (lazyValue: void, payload?: any) => void",
		"lazyValue: string",
		"lazyValueLoading: boolean",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildParsedLogicsBuilderSamplePreservesBooleanReducerState(t *testing.T) {
	report := inspectSampleReport(t, "builderLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 parsed logic, got %d", len(logics))
	}

	logic := logics[0]
	if !hasReducer(logic.Reducers, "isLoading", "boolean") {
		t.Fatalf("expected builder isLoading reducer type boolean, got %+v", logic.Reducers)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	selectorBlock := extractInterfaceBlock(t, rendered, "selector", "selectors")
	if !strings.Contains(selectorBlock, "isLoading: boolean") {
		t.Fatalf("expected aggregate selector block to include boolean isLoading:\n%s", selectorBlock)
	}
	if strings.Contains(selectorBlock, "sortedRepositories") {
		t.Fatalf("expected aggregate selector block to exclude builder derived selectors:\n%s", selectorBlock)
	}
	if strings.Contains(rendered, "isLoading: false") {
		t.Fatalf("expected emitted output to avoid literal false builder reducer state:\n%s", rendered)
	}
}

func TestBuildParsedLogicsRecoversBuilderBlockSelectorReturnTypes(t *testing.T) {
	report := inspectSampleReport(t, "builderLogic.ts")

	logics, err := BuildParsedLogics(report)
	if err != nil {
		t.Fatalf("BuildParsedLogics returned error: %v", err)
	}
	logic := logics[0]

	if selector, ok := findParsedField(logic.Selectors, "sortedRepositories"); !ok || selector.Type != "Repository[]" {
		t.Fatalf("expected sortedRepositories selector type Repository[], got %+v", selector)
	}

	rendered := EmitTypegenAt(logics, time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{
		"sortedRepositories: (state: any, props?: any) => Repository[]",
		"sortedRepositories: Repository[]",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected emitted output to contain %q:\n%s", expected, rendered)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	return abs
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	return string(content)
}

func inspectSampleReport(t *testing.T, fileName string) *Report {
	t.Helper()
	root := repoRoot(t)
	report, err := InspectFile(context.Background(), InspectOptions{
		BinaryPath: tsgoapi.PreferredBinary(root),
		ProjectDir: filepath.Join(root, "samples"),
		ConfigFile: filepath.Join(root, "samples", "tsconfig.json"),
		File:       filepath.Join(root, "samples", fileName),
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	return report
}

func mustFindLogicProperty(t *testing.T, logic SourceLogic, name string) SourceProperty {
	t.Helper()
	for _, property := range logic.Properties {
		if property.Name == name {
			return property
		}
	}
	t.Fatalf("expected source logic to contain property %q", name)
	return SourceProperty{}
}

func hasAction(actions []ParsedAction, name string) bool {
	for _, action := range actions {
		if action.Name == name {
			return true
		}
	}
	return false
}

func hasReducer(reducers []ParsedField, name, typeText string) bool {
	for _, reducer := range reducers {
		if reducer.Name == name && reducer.Type == typeText {
			return true
		}
	}
	return false
}

func hasImport(imports []TypeImport, path, name string) bool {
	for _, item := range imports {
		if item.Path != path {
			continue
		}
		for _, imported := range item.Names {
			if imported == name {
				return true
			}
		}
	}
	return false
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func findParsedListener(listeners []ParsedListener, name string) (ParsedListener, bool) {
	for _, listener := range listeners {
		if listener.Name == name {
			return listener, true
		}
	}
	return ParsedListener{}, false
}

func findParsedFunction(functions []ParsedFunction, name string) (ParsedFunction, bool) {
	for _, fn := range functions {
		if fn.Name == name {
			return fn, true
		}
	}
	return ParsedFunction{}, false
}

func TestListenerPayloadHelpersPreferPrintedFunctionType(t *testing.T) {
	member := MemberReport{
		TypeString:      "(action: { foo: string; ... 2 more ...; }) => void",
		PrintedTypeNode: "(action: { foo: string; bar: number; baz: boolean; }) => void",
	}

	expected := "{ foo: string; bar: number; baz: boolean; }"
	if payload := listenerPayloadTypeFromMember(member); payload != expected {
		t.Fatalf("expected listener payload %q, got %q", expected, payload)
	}
	if payload := sharedListenerPayloadTypeFromMember(member); payload != expected {
		t.Fatalf("expected shared listener payload %q, got %q", expected, payload)
	}
}

func TestSynthesizeConnectedActionFromSymbolsPrefersPrintedFunctionType(t *testing.T) {
	action, ok := synthesizeConnectedActionFromSymbols(
		connectedName{SourceName: "loadThing", LocalName: "loadThing"},
		[]MemberReport{
			{
				Name:            "loadThing",
				TypeString:      "(payload: { foo: string; ... 1 more ...; }) => void",
				PrintedTypeNode: "(payload: { foo: string; bar: number; }) => void",
			},
		},
		[]MemberReport{
			{
				Name:                  "loadThing",
				TypeString:            "(payload: { foo: string; ... 1 more ...; }) => { type: 'load thing'; payload: { foo: string; ... 1 more ...; } }",
				PrintedTypeNode:       "(payload: { foo: string; bar: number; }) => { type: 'load thing'; payload: { foo: string; bar: number; } }",
				ReturnTypeString:      "{ type: 'load thing'; payload: { foo: string; ... 1 more ...; } }",
				PrintedReturnTypeNode: "{ type: 'load thing'; payload: { foo: string; bar: number; } }",
			},
		},
	)
	if !ok {
		t.Fatalf("expected connected action to be synthesized")
	}
	if strings.Contains(action.FunctionType, "...") || !strings.Contains(action.FunctionType, "bar: number") {
		t.Fatalf("expected synthesized action function type to prefer printed type node, got %q", action.FunctionType)
	}
	expectedPayload := "{ foo: string; bar: number; }"
	if action.PayloadType != expectedPayload {
		t.Fatalf("expected synthesized action payload %q, got %q", expectedPayload, action.PayloadType)
	}
}

func extractInterfaceBlock(t *testing.T, rendered, startProperty, endProperty string) string {
	t.Helper()

	startMarker := "    " + startProperty + ":"
	endMarker := "\n    " + endProperty + ":"

	start := strings.Index(rendered, startMarker)
	if start == -1 {
		t.Fatalf("expected rendered output to contain %q:\n%s", startMarker, rendered)
	}

	rest := rendered[start:]
	end := strings.Index(rest, endMarker)
	if end == -1 {
		t.Fatalf("expected rendered output to contain %q after %q:\n%s", endMarker, startMarker, rendered)
	}

	return rest[:end]
}
