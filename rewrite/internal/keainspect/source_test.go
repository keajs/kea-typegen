package keainspect

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestFindLogics(t *testing.T) {
	source := `
import { kea } from 'kea'

export const logic = kea<logicType>({
    props: {} as {
        id: number
    },
    actions: () => ({
        updateName: (name: string) => ({ name }),
    }),
    selectors: ({ selectors, values }) => ({
        upperCaseName: [
            () => [selectors.capitalizedName],
            (capitalizedName) => capitalizedName.toUpperCase(),
        ],
    }),
    reducers: () => ({
        name: [
            'birdname',
            {
                updateName: (_, { name }) => name,
            },
        ],
    }),
})
`

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 logic, got %d", len(logics))
	}
	if logics[0].Name != "logic" {
		t.Fatalf("expected logic name %q, got %q", "logic", logics[0].Name)
	}
	if logics[0].InputKind != "object" {
		t.Fatalf("expected input kind %q, got %q", "object", logics[0].InputKind)
	}

	names := map[string]bool{}
	for _, property := range logics[0].Properties {
		names[property.Name] = true
	}
	for _, expected := range []string{"props", "actions", "selectors", "reducers"} {
		if !names[expected] {
			t.Fatalf("expected property %q to be found", expected)
		}
	}
}

func TestFindLogicsBuilderArray(t *testing.T) {
	source := `
import { kea, path, actions, reducers, selectors } from 'kea'

export const builderLogic = kea([
    path(['builderLogic']),
    actions({
        setUsername: (username: string) => ({ username }),
    }),
    reducers({
        username: ['x', { setUsername: (_, { username }) => username }],
    }),
    selectors({
        upperName: [() => [], () => 'x'],
    }),
])
`

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 logic, got %d", len(logics))
	}
	if logics[0].InputKind != "builders" {
		t.Fatalf("expected input kind %q, got %q", "builders", logics[0].InputKind)
	}

	gotNames := []string{}
	for _, property := range logics[0].Properties {
		gotNames = append(gotNames, property.Name)
	}
	expectedNames := []string{"path", "actions", "reducers", "selectors"}
	if len(gotNames) != len(expectedNames) {
		t.Fatalf("expected %d builders, got %d (%v)", len(expectedNames), len(gotNames), gotNames)
	}
	for index, expected := range expectedNames {
		if gotNames[index] != expected {
			t.Fatalf("expected builder %d to be %q, got %q", index, expected, gotNames[index])
		}
	}
}

func TestFindLogicsPreservesTypedAssignmentName(t *testing.T) {
	source := `
import { kea, type LogicWrapper } from 'kea'

import type { builderLogicType } from './builderLogicType'

export const builderLogic: LogicWrapper<builderLogicType> = kea([
    path(['builderLogic']),
])
`

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 logic, got %d", len(logics))
	}
	if logics[0].Name != "builderLogic" {
		t.Fatalf("expected logic name %q, got %q", "builderLogic", logics[0].Name)
	}
}

func TestFindLogicsBuilderArrayCanonicalizesAliasedImports(t *testing.T) {
	source := strings.Join([]string{
		"import { kea, path as logicPath, actions as logicActions, reducers as logicReducers } from 'kea'",
		"",
		"export const builderLogic = kea([",
		"    logicPath(['builderLogic']),",
		"    logicActions({}),",
		"    logicReducers({",
		"        username: ['x', {}],",
		"    }),",
		"])",
		"",
	}, "\n")

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 logic, got %d", len(logics))
	}

	gotNames := []string{}
	for _, property := range logics[0].Properties {
		gotNames = append(gotNames, property.Name)
	}
	expectedNames := []string{"path", "actions", "reducers"}
	if len(gotNames) != len(expectedNames) {
		t.Fatalf("expected %d builders, got %d (%v)", len(expectedNames), len(gotNames), gotNames)
	}
	for index, expected := range expectedNames {
		if gotNames[index] != expected {
			t.Fatalf("expected builder %d to be %q, got %q", index, expected, gotNames[index])
		}
	}
}

func TestFindLogicsBuilderArrayCanonicalizesNamespaceQualifiedBuilders(t *testing.T) {
	source := strings.Join([]string{
		"import * as keaBuilders from 'kea'",
		"import * as loaderBuilders from 'kea-loaders'",
		"",
		"export const builderLogic = keaBuilders.kea([",
		"    keaBuilders.path(['builderLogic']),",
		"    keaBuilders.actions({}),",
		"    loaderBuilders.loaders(({}) => ({",
		"        name: ['', { loadName: async () => 'test' }],",
		"    })),",
		"])",
		"",
	}, "\n")

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 logic, got %d", len(logics))
	}
	if logics[0].InputKind != "builders" {
		t.Fatalf("expected input kind %q, got %q", "builders", logics[0].InputKind)
	}

	gotNames := []string{}
	for _, property := range logics[0].Properties {
		gotNames = append(gotNames, property.Name)
	}
	expectedNames := []string{"path", "actions", "loaders"}
	if len(gotNames) != len(expectedNames) {
		t.Fatalf("expected %d builders, got %d (%v)", len(expectedNames), len(gotNames), gotNames)
	}
	for index, expected := range expectedNames {
		if gotNames[index] != expected {
			t.Fatalf("expected builder %d to be %q, got %q", index, expected, gotNames[index])
		}
	}
}

func TestFindLogicsObjectLiteralSupportsQuotedAndComputedKeys(t *testing.T) {
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

	logics, err := FindLogics(source)
	if err != nil {
		t.Fatalf("FindLogics returned error: %v", err)
	}
	if len(logics) != 1 {
		t.Fatalf("expected 1 logic, got %d", len(logics))
	}
	if logics[0].InputKind != "object" {
		t.Fatalf("expected input kind %q, got %q", "object", logics[0].InputKind)
	}

	gotNames := []string{}
	for _, property := range logics[0].Properties {
		gotNames = append(gotNames, property.Name)
	}
	expectedNames := []string{"path", "actions", "reducers"}
	if len(gotNames) != len(expectedNames) {
		t.Fatalf("expected %d properties, got %d (%v)", len(expectedNames), len(gotNames), gotNames)
	}
	for index, expected := range expectedNames {
		if gotNames[index] != expected {
			t.Fatalf("expected property %d to be %q, got %q", index, expected, gotNames[index])
		}
	}
}

func TestFindInspectableObjectLiteral(t *testing.T) {
	source := `
listeners(({ actions }) => ({
    setUsername: async ({ username }, breakpoint) => {
        await breakpoint(300)
        actions.setUsername(username)
    },
}))
`

	start := strings.Index(source, "(")
	if start == -1 {
		t.Fatalf("expected opening parenthesis")
	}
	end := len(source) - 1
	objectStart, objectEnd, ok, err := FindInspectableObjectLiteral(source, start, end)
	if err != nil {
		t.Fatalf("FindInspectableObjectLiteral returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected object literal to be found")
	}

	properties, err := parseTopLevelProperties(source, objectStart, objectEnd)
	if err != nil {
		t.Fatalf("parseTopLevelProperties returned error: %v", err)
	}
	if len(properties) != 1 || properties[0].Name != "setUsername" {
		t.Fatalf("expected one setUsername property, got %+v", properties)
	}
}

func TestFindLastTopLevelArrayElement(t *testing.T) {
	source := `
capitalizedName: [
    (s) => [s.name, s.number],
    (name, number) => name.toUpperCase() + number.toString(),
],
`

	valueStart := strings.Index(source, "[")
	if valueStart == -1 {
		t.Fatalf("expected array literal start")
	}
	valueEnd := len(source) - 1

	elementStart, elementEnd, ok, err := FindLastTopLevelArrayElement(source, valueStart, valueEnd)
	if err != nil {
		t.Fatalf("FindLastTopLevelArrayElement returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected last array element to be found")
	}

	got := strings.TrimSpace(source[elementStart:elementEnd])
	expected := "(name, number) => name.toUpperCase() + number.toString()"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestFindTopLevelArrayElements(t *testing.T) {
	source := `
capitalizedName: [
    (s) => [s.name, s.number],
    (name, number) => name.toUpperCase() + number.toString(),
]
`

	valueStart := strings.Index(source, "[")
	if valueStart == -1 {
		t.Fatalf("expected array literal start")
	}
	valueEnd := len(source) - 1

	elements, err := FindTopLevelArrayElements(source, valueStart, valueEnd)
	if err != nil {
		t.Fatalf("FindTopLevelArrayElements returned error: %v", err)
	}
	if len(elements) != 2 {
		t.Fatalf("expected 2 array elements, got %+v", elements)
	}

	first := strings.TrimSpace(source[elements[0].Start:elements[0].End])
	if first != "(s) => [s.name, s.number]" {
		t.Fatalf("unexpected first array element: %q", first)
	}
	second := strings.TrimSpace(source[elements[1].Start:elements[1].End])
	if second != "(name, number) => name.toUpperCase() + number.toString()" {
		t.Fatalf("unexpected second array element: %q", second)
	}
}

func TestFindLastFunctionLikeTopLevelArrayElementSkipsTrailingOptionsObject(t *testing.T) {
	source := `
ipAddresses: [
    (s) => [s.logs],
    (logs) => logs.map((log) => log.ip).filter(Boolean),
    { resultEqualityCheck: (a, b) => JSON.stringify(a) === JSON.stringify(b) },
]
`

	valueStart := strings.Index(source, "[")
	if valueStart == -1 {
		t.Fatalf("expected array literal start")
	}
	valueEnd := len(source) - 1

	elementStart, elementEnd, ok, err := FindLastFunctionLikeTopLevelArrayElement(source, valueStart, valueEnd)
	if err != nil {
		t.Fatalf("FindLastFunctionLikeTopLevelArrayElement returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected function-like array element to be found")
	}

	got := strings.TrimSpace(source[elementStart:elementEnd])
	want := "(logs) => logs.map((log) => log.ip).filter(Boolean)"
	if got != want {
		t.Fatalf("expected function-like array element %q, got %q", want, got)
	}
}

func TestSourceOffsetMapUsesUTF16Offsets(t *testing.T) {
	source := "key: '👈 emoji'\nselector: value\n"
	keyByte := strings.Index(source, "selector")
	valueByte := strings.Index(source, "value")
	if keyByte == -1 || valueByte == -1 {
		t.Fatalf("expected selector markers in source")
	}

	offsets := newSourceOffsetMap(source)
	wantKey := len(utf16.Encode([]rune(source[:keyByte])))
	wantValue := len(utf16.Encode([]rune(source[:valueByte])))

	if got := offsets.utf16Offset(keyByte); got != wantKey {
		t.Fatalf("expected UTF-16 selector offset %d, got %d", wantKey, got)
	}
	if got := offsets.utf16Offset(valueByte); got != wantValue {
		t.Fatalf("expected UTF-16 value offset %d, got %d", wantValue, got)
	}
	if got := offsets.utf16Length(); got != len(utf16.Encode([]rune(source))) {
		t.Fatalf("expected UTF-16 source length %d, got %d", len(utf16.Encode([]rune(source))), got)
	}
}

func TestFindArrowFunctionReturnProbeBlockBody(t *testing.T) {
	source := `(repositories) => {
    return [...repositories].sort((a, b) => b.stargazers_count - a.stargazers_count)
}`

	probe, ok, err := FindArrowFunctionReturnProbe(source, 0, len(source))
	if err != nil {
		t.Fatalf("FindArrowFunctionReturnProbe returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected return probe to be found")
	}

	got := strings.TrimSpace(source[probe:])
	expected := "[...repositories].sort((a, b) => b.stargazers_count - a.stargazers_count)\n}"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestCallbackTypeProbePositionsIncludeCurriedCallBody(t *testing.T) {
	source := `(props) => keyForInsightLogicProps('new')(props)`

	positions := callbackTypeProbePositions(source, 0, len(source))
	bodyStart := strings.Index(source, "keyForInsightLogicProps")
	firstCallClose := strings.Index(source, "('new')") + len("('new')") - 1

	hasPosition := func(target int) bool {
		for _, position := range positions {
			if position == target {
				return true
			}
		}
		return false
	}

	if !hasPosition(bodyStart) {
		t.Fatalf("expected body start position %d in %v", bodyStart, positions)
	}
	if !hasPosition(firstCallClose) {
		t.Fatalf("expected first call close position %d in %v", firstCallClose, positions)
	}
}
