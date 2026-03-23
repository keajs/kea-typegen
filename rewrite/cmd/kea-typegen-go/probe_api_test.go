package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"kea-typegen/rewrite/internal/tsgoapi"
)

func TestParamsForProbeMethodUsesBootstrappedHandles(t *testing.T) {
	session := &probeSession{
		Snapshot:        "snapshot-1",
		ProjectID:       "project-1",
		File:            "/tmp/logic.ts",
		SamplePosition:  42,
		SampleSymbol:    "symbol-1",
		SampleType:      "type-1",
		SampleSignature: "signature-1",
		SampleLocations: []string{"location-1"},
	}

	got := paramsForProbeMethod("getTypeOfSymbolAtLocation", session)
	params, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected params map, got %T", got)
	}
	if params["snapshot"] != "snapshot-1" || params["project"] != "project-1" {
		t.Fatalf("expected snapshot/project context, got %+v", params)
	}
	if params["symbol"] != "symbol-1" {
		t.Fatalf("expected symbol handle, got %+v", params)
	}
	if params["location"] != "location-1" {
		t.Fatalf("expected bootstrapped location handle, got %+v", params)
	}
}

func TestParamsForProbeMethodFallsBackToInvalidHandles(t *testing.T) {
	session := &probeSession{
		Snapshot:       "snapshot-1",
		ProjectID:      "project-1",
		File:           "/tmp/logic.ts",
		SamplePosition: 7,
	}

	got := paramsForProbeMethod("getTypesOfSymbols", session)
	params, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected params map, got %T", got)
	}
	symbols, ok := params["symbols"].([]string)
	if !ok {
		t.Fatalf("expected symbols slice, got %+v", params)
	}
	if len(symbols) != 1 || symbols[0] != invalidProbeHandle {
		t.Fatalf("expected placeholder symbol handle, got %+v", params)
	}
}

func TestParamsForProbeMethodUsesSymbolPositionForSymbolsAtPositions(t *testing.T) {
	session := &probeSession{
		Snapshot:        "snapshot-1",
		ProjectID:       "project-1",
		File:            "/tmp/logic.ts",
		SamplePosition:  42,
		SampleSymbolPos: 7,
	}

	got := paramsForProbeMethod("getSymbolsAtPositions", session)
	params, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected params map, got %T", got)
	}
	positions, ok := params["positions"].([]int)
	if !ok {
		t.Fatalf("expected positions slice, got %+v", params)
	}
	if len(positions) != 1 || positions[0] != 7 {
		t.Fatalf("expected symbol position, got %+v", positions)
	}
}

func TestParamsForProbeMethodUsesTypePositionForTypesAtPositions(t *testing.T) {
	session := &probeSession{
		Snapshot:        "snapshot-1",
		ProjectID:       "project-1",
		File:            "/tmp/logic.ts",
		SamplePosition:  42,
		SampleSymbolPos: 7,
	}

	got := paramsForProbeMethod("getTypesAtPositions", session)
	params, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected params map, got %T", got)
	}
	positions, ok := params["positions"].([]int)
	if !ok {
		t.Fatalf("expected positions slice, got %+v", params)
	}
	if len(positions) != 1 || positions[0] != 42 {
		t.Fatalf("expected type position, got %+v", positions)
	}
}

func TestParamsForProbeMethodUsesTypeHandleForTypeToString(t *testing.T) {
	session := &probeSession{
		Snapshot:   "snapshot-1",
		ProjectID:  "project-1",
		SampleType: "type-1",
	}

	got := paramsForProbeMethod("typeToString", session)
	params, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected params map, got %T", got)
	}
	if params["type"] != "type-1" {
		t.Fatalf("expected type handle, got %+v", params)
	}
}

func TestParamsForProbeMethodUsesTypeHandleForPrintTypeNode(t *testing.T) {
	session := &probeSession{
		Snapshot:   "snapshot-1",
		ProjectID:  "project-1",
		SampleType: "type-1",
	}

	got := paramsForProbeMethod("printTypeNode", session)
	params, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected params map, got %T", got)
	}
	if params["type"] != "type-1" {
		t.Fatalf("expected type handle, got %+v", params)
	}
}

func TestParamsForProbeMethodUsesTypeHandleForGetPropertiesOfType(t *testing.T) {
	session := &probeSession{
		Snapshot:   "snapshot-1",
		ProjectID:  "project-1",
		SampleType: "type-1",
	}

	got := paramsForProbeMethod("getPropertiesOfType", session)
	params, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected params map, got %T", got)
	}
	if params["type"] != "type-1" {
		t.Fatalf("expected type handle, got %+v", params)
	}
}

func TestParamsForProbeMethodUsesTypeHandleForPropertyDetails(t *testing.T) {
	session := &probeSession{
		Snapshot:   "snapshot-1",
		ProjectID:  "project-1",
		SampleType: "type-1",
	}

	got := paramsForProbeMethod("propertyDetails", session)
	params, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected params map, got %T", got)
	}
	if params["type"] != "type-1" {
		t.Fatalf("expected type handle, got %+v", params)
	}
}

func TestParamsForProbeMethodUsesTypeHandleForGetSignaturesOfType(t *testing.T) {
	session := &probeSession{
		Snapshot:   "snapshot-1",
		ProjectID:  "project-1",
		SampleType: "type-1",
	}

	got := paramsForProbeMethod("getSignaturesOfType", session)
	params, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected params map, got %T", got)
	}
	if params["type"] != "type-1" {
		t.Fatalf("expected type handle, got %+v", params)
	}
	if params["kind"] != 0 {
		t.Fatalf("expected signature kind 0, got %+v", params)
	}
}

func TestParamsForProbeMethodUsesSymbolHandleForGetTypeOfSymbol(t *testing.T) {
	session := &probeSession{
		Snapshot:     "snapshot-1",
		ProjectID:    "project-1",
		SampleSymbol: "symbol-1",
	}

	got := paramsForProbeMethod("getTypeOfSymbol", session)
	params, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected params map, got %T", got)
	}
	if params["symbol"] != "symbol-1" {
		t.Fatalf("expected symbol handle, got %+v", params)
	}
}

func TestParamsForProbeMethodUsesSymbolHandleForMemberDetails(t *testing.T) {
	session := &probeSession{
		Snapshot:     "snapshot-1",
		ProjectID:    "project-1",
		SampleSymbol: "symbol-1",
	}

	got := paramsForProbeMethod("memberDetails", session)
	params, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected params map, got %T", got)
	}
	if params["symbol"] != "symbol-1" {
		t.Fatalf("expected symbol handle, got %+v", params)
	}
}

func TestParamsForProbeMethodUsesSignatureHandleForReturnTypePseudoMethods(t *testing.T) {
	session := &probeSession{
		Snapshot:        "snapshot-1",
		ProjectID:       "project-1",
		SampleSignature: "signature-1",
	}

	for _, method := range []string{"returnTypeToString", "printReturnTypeNode"} {
		got := paramsForProbeMethod(method, session)
		params, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("%s: expected params map, got %T", method, got)
		}
		if params["signature"] != "signature-1" {
			t.Fatalf("%s: expected signature handle, got %+v", method, params)
		}
	}
}

func TestProbeParamCandidatesUseEveryLocationHandle(t *testing.T) {
	session := &probeSession{
		Snapshot:        "snapshot-1",
		ProjectID:       "project-1",
		SampleSymbol:    "symbol-1",
		SampleLocations: []string{"location-1", "location-2"},
	}

	got := probeParamCandidates("getTypeOfSymbolAtLocation", session)
	if len(got) != 2 {
		t.Fatalf("expected 2 param candidates, got %d", len(got))
	}

	first, ok := got[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first params map, got %T", got[0])
	}
	second, ok := got[1].(map[string]any)
	if !ok {
		t.Fatalf("expected second params map, got %T", got[1])
	}
	if first["location"] != "location-1" || second["location"] != "location-2" {
		t.Fatalf("unexpected location candidates: %+v %+v", first, second)
	}
}

func TestProbeParamCandidatesUseNameLocationsForSymbolLookups(t *testing.T) {
	session := &probeSession{
		Snapshot:  "snapshot-1",
		ProjectID: "project-1",
		SampleLocations: []string{
			"decl-location-1",
			"decl-location-2",
		},
		SampleNameLocs: []string{
			"name-location-1",
			"name-location-2",
		},
	}

	got := probeParamCandidates("getSymbolAtLocation", session)
	if len(got) != 2 {
		t.Fatalf("expected 2 param candidates, got %d", len(got))
	}

	first, ok := got[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first params map, got %T", got[0])
	}
	second, ok := got[1].(map[string]any)
	if !ok {
		t.Fatalf("expected second params map, got %T", got[1])
	}
	if first["location"] != "name-location-1" || second["location"] != "name-location-2" {
		t.Fatalf("unexpected name-location candidates: %+v %+v", first, second)
	}
}

func TestParamsForProbeMethodUsesNameLocationForSymbolLookups(t *testing.T) {
	session := &probeSession{
		Snapshot:  "snapshot-1",
		ProjectID: "project-1",
		SampleLocations: []string{
			"decl-location-1",
		},
		SampleNameLocs: []string{
			"name-location-1",
		},
	}

	got := paramsForProbeMethod("getSymbolsAtLocations", session)
	params, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected params map, got %T", got)
	}
	locations, ok := params["locations"].([]string)
	if !ok {
		t.Fatalf("expected locations slice, got %+v", params)
	}
	if len(locations) != 1 || locations[0] != "name-location-1" {
		t.Fatalf("unexpected symbol-location params: %+v", locations)
	}
}

func TestParseProbeSourceFileNodesFindsNameLocationAtPosition(t *testing.T) {
	blob := encodeProbeSourceFileNodeBlob(
		[]uint32{0, 0, 0, 0, 0, 0, 0},
		[]uint32{probeSourceFileSyntaxKind, 0, 20, 0, 0, 0, 0},
		[]uint32{303, 4, 10, 0, 1, 0, 0},
		[]uint32{probeIdentifierSyntaxKind, 4, 6, 0, 2, 0, 0},
		[]uint32{211, 6, 10, 0, 2, 0, 0},
	)

	nodes, err := parseProbeSourceFileNodes(blob, 20)
	if err != nil {
		t.Fatalf("parseProbeSourceFileNodes() error = %v", err)
	}

	got := probeNameLocationHandleAtPosition(nodes, 5, "/users/example/logic.ts")
	want := "4.6.79./users/example/logic.ts"
	if got != want {
		t.Fatalf("probeNameLocationHandleAtPosition() = %q, want %q", got, want)
	}
}

func TestParseProbeSourceFileNodesFindsUnalignedNodeTable(t *testing.T) {
	blob := encodeProbeSourceFileNodeBlobWithPadding(3,
		[]uint32{0, 0, 0, 0, 0, 0, 0},
		[]uint32{probeSourceFileSyntaxKind, 0, 20, 0, 0, 0, 0},
		[]uint32{303, 4, 10, 0, 1, 0, 0},
		[]uint32{probeIdentifierSyntaxKind, 4, 6, 0, 2, 0, 0},
		[]uint32{211, 6, 10, 0, 2, 0, 0},
	)

	nodes, err := parseProbeSourceFileNodes(blob, 20)
	if err != nil {
		t.Fatalf("parseProbeSourceFileNodes() error = %v", err)
	}

	got := probeNameLocationHandleAtPosition(nodes, 5, "/users/example/connect.ts")
	want := "4.6.79./users/example/connect.ts"
	if got != want {
		t.Fatalf("probeNameLocationHandleAtPosition() = %q, want %q", got, want)
	}
}

func TestParseProbeSourceFileNodesUsesUTF16SourceLength(t *testing.T) {
	source := "const note = '👈'\n"
	utf16Len := len(utf16.Encode([]rune(source)))
	blob := encodeProbeSourceFileNodeBlob(
		[]uint32{0, 0, 0, 0, 0, 0, 0},
		[]uint32{probeSourceFileSyntaxKind, 0, uint32(utf16Len), 0, 0, 0, 0},
		[]uint32{probeIdentifierSyntaxKind, 6, 10, 0, 1, 0, 0},
	)

	if _, err := parseProbeSourceFileNodes(blob, len(source)); err == nil {
		t.Fatalf("expected byte-length parse to fail for unicode source")
	}

	nodes, err := parseProbeSourceFileNodes(blob, utf16Len)
	if err != nil {
		t.Fatalf("parseProbeSourceFileNodes() error = %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 parsed nodes, got %d", len(nodes))
	}
}

func TestProbeNodeLocationHandleForRangePrefersExactMatch(t *testing.T) {
	nodes := []probeSourceFileNode{
		{Kind: probeSourceFileSyntaxKind, Pos: 0, End: 30},
		{Kind: 303, Pos: 4, End: 20},
		{Kind: 220, Pos: 6, End: 18},
		{Kind: probeIdentifierSyntaxKind, Pos: 6, End: 10},
	}

	got := probeNodeLocationHandleForRange(nodes, 6, 18, "/users/example/logic.ts")
	want := "6.18.220./users/example/logic.ts"
	if got != want {
		t.Fatalf("probeNodeLocationHandleForRange() = %q, want %q", got, want)
	}
}

func TestProbePositionCandidatesCollectPropertyNames(t *testing.T) {
	source := `
import { kea } from 'kea'

export const logic = kea({
    props: {},
    selectors: () => ({
        upperCaseName: [() => [], () => 'x'],
    }),
})
`

	got, err := probePositionCandidates(source)
	if err != nil {
		t.Fatalf("probePositionCandidates() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(got))
	}
	if got[0].propertyName != "props" || got[1].propertyName != "selectors" {
		t.Fatalf("unexpected candidate order: %+v", got)
	}
	if got[0].namePosition <= 0 || got[0].valuePosition <= 0 {
		t.Fatalf("expected positions on first candidate, got %+v", got[0])
	}
}

func TestSelectProbePositionCandidatesUsesRequestedProperty(t *testing.T) {
	candidates := []probePositionCandidate{
		{propertyName: "props", namePosition: 1, valuePosition: 2},
		{propertyName: "selectors", namePosition: 3, valuePosition: 4},
		{propertyName: "selectors", namePosition: 5, valuePosition: 6},
	}

	got, err := selectProbePositionCandidates(candidates, "selectors")
	if err != nil {
		t.Fatalf("selectProbePositionCandidates() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 selected candidates, got %d", len(got))
	}
	for _, candidate := range got {
		if candidate.propertyName != "selectors" {
			t.Fatalf("unexpected candidate selection: %+v", got)
		}
	}
}

func TestSelectProbePositionCandidatesRejectsUnknownProperty(t *testing.T) {
	candidates := []probePositionCandidate{
		{propertyName: "selectors"},
		{propertyName: "props"},
		{propertyName: "selectors"},
	}

	_, err := selectProbePositionCandidates(candidates, "listeners")
	if err == nil {
		t.Fatalf("expected selection error")
	}
	if !strings.Contains(err.Error(), `available properties: props, selectors`) {
		t.Fatalf("unexpected selection error: %v", err)
	}
}

func TestProbeMemberPositionCandidatesSelectsNestedProperty(t *testing.T) {
	source := `
import { kea } from 'kea'

export const logic = kea({
    actions: () => ({
        updateName: (name: string) => ({ name }),
        setAge: (age: number) => ({ age }),
    }),
})
`

	candidates, err := probePositionCandidates(source)
	if err != nil {
		t.Fatalf("probePositionCandidates() error = %v", err)
	}
	candidates, err = selectProbePositionCandidates(candidates, "actions")
	if err != nil {
		t.Fatalf("selectProbePositionCandidates() error = %v", err)
	}

	got, err := probeMemberPositionCandidates(source, candidates, "updateName")
	if err != nil {
		t.Fatalf("probeMemberPositionCandidates() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 member candidate, got %d", len(got))
	}
	if got[0].propertyName != "actions" || got[0].memberName != "updateName" {
		t.Fatalf("unexpected member candidate: %+v", got[0])
	}
	if got[0].namePosition <= 0 || got[0].valuePosition <= 0 || got[0].valueEnd <= got[0].valuePosition {
		t.Fatalf("expected nested positions on member candidate, got %+v", got[0])
	}
}

func TestProbeMemberPositionCandidatesRejectsUnknownMember(t *testing.T) {
	source := `
import { kea } from 'kea'

export const logic = kea({
    actions: () => ({
        updateName: (name: string) => ({ name }),
        setAge: (age: number) => ({ age }),
    }),
})
`

	candidates, err := probePositionCandidates(source)
	if err != nil {
		t.Fatalf("probePositionCandidates() error = %v", err)
	}
	candidates, err = selectProbePositionCandidates(candidates, "actions")
	if err != nil {
		t.Fatalf("selectProbePositionCandidates() error = %v", err)
	}

	_, err = probeMemberPositionCandidates(source, candidates, "missing")
	if err == nil {
		t.Fatalf("expected member selection error")
	}
	if !strings.Contains(err.Error(), `available members: setAge, updateName`) {
		t.Fatalf("unexpected member selection error: %v", err)
	}
}

func TestProbeElementPositionCandidatesSelectsLastArrayElement(t *testing.T) {
	source := `
import { kea } from 'kea'

export const logic = kea({
    selectors: () => ({
        capitalizedName: [
            (s) => [s.name, s.number],
            (name, number) => name.toUpperCase() + number.toString(),
        ],
    }),
})
`

	candidates, err := probePositionCandidates(source)
	if err != nil {
		t.Fatalf("probePositionCandidates() error = %v", err)
	}
	candidates, err = selectProbePositionCandidates(candidates, "selectors")
	if err != nil {
		t.Fatalf("selectProbePositionCandidates() error = %v", err)
	}
	candidates, err = probeMemberPositionCandidates(source, candidates, "capitalizedName")
	if err != nil {
		t.Fatalf("probeMemberPositionCandidates() error = %v", err)
	}

	got, err := probeElementPositionCandidates(source, candidates, "last")
	if err != nil {
		t.Fatalf("probeElementPositionCandidates() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 element candidate, got %d", len(got))
	}
	if got[0].elementName != "last" {
		t.Fatalf("unexpected element label: %+v", got[0])
	}
	selected := strings.TrimSpace(source[got[0].valuePosition:got[0].valueEnd])
	if selected != "(name, number) => name.toUpperCase() + number.toString()" {
		t.Fatalf("unexpected selected element: %q", selected)
	}
}

func TestProbeElementPositionCandidatesSelectsLastCallableArrayElementBeforeTrailingOptionsObject(t *testing.T) {
	source := `
import { kea } from 'kea'

export const logic = kea({
    selectors: () => ({
        ipAddresses: [
            (s) => [s.logs],
            (logs) => logs.map((log) => log.ip).filter(Boolean),
            { resultEqualityCheck: (a, b) => JSON.stringify(a) === JSON.stringify(b) },
        ],
    }),
})
`

	candidates, err := probePositionCandidates(source)
	if err != nil {
		t.Fatalf("probePositionCandidates() error = %v", err)
	}
	candidates, err = selectProbePositionCandidates(candidates, "selectors")
	if err != nil {
		t.Fatalf("selectProbePositionCandidates() error = %v", err)
	}
	candidates, err = probeMemberPositionCandidates(source, candidates, "ipAddresses")
	if err != nil {
		t.Fatalf("probeMemberPositionCandidates() error = %v", err)
	}

	got, err := probeElementPositionCandidates(source, candidates, "last")
	if err != nil {
		t.Fatalf("probeElementPositionCandidates() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 element candidate, got %d", len(got))
	}
	if got[0].elementName != "last" {
		t.Fatalf("unexpected element label: %+v", got[0])
	}
	selected := strings.TrimSpace(source[got[0].valuePosition:got[0].valueEnd])
	if selected != "(logs) => logs.map((log) => log.ip).filter(Boolean)" {
		t.Fatalf("unexpected selected element: %q", selected)
	}
}

func TestProbeElementPositionCandidatesSelectsIndexedArrayElement(t *testing.T) {
	source := `
import { kea } from 'kea'

export const logic = kea({
    selectors: () => ({
        capitalizedName: [
            (s) => [s.name, s.number],
            (name, number) => name.toUpperCase() + number.toString(),
        ],
    }),
})
`

	candidates, err := probePositionCandidates(source)
	if err != nil {
		t.Fatalf("probePositionCandidates() error = %v", err)
	}
	candidates, err = selectProbePositionCandidates(candidates, "selectors")
	if err != nil {
		t.Fatalf("selectProbePositionCandidates() error = %v", err)
	}
	candidates, err = probeMemberPositionCandidates(source, candidates, "capitalizedName")
	if err != nil {
		t.Fatalf("probeMemberPositionCandidates() error = %v", err)
	}

	got, err := probeElementPositionCandidates(source, candidates, "0")
	if err != nil {
		t.Fatalf("probeElementPositionCandidates() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 element candidate, got %d", len(got))
	}
	if got[0].elementName != "0" {
		t.Fatalf("unexpected element label: %+v", got[0])
	}
	selected := strings.TrimSpace(source[got[0].valuePosition:got[0].valueEnd])
	if selected != "(s) => [s.name, s.number]" {
		t.Fatalf("unexpected selected element: %q", selected)
	}
}

func TestProbeElementPositionCandidatesRejectsUnknownElement(t *testing.T) {
	source := `
import { kea } from 'kea'

export const logic = kea({
    selectors: () => ({
        capitalizedName: [
            (s) => [s.name, s.number],
            (name, number) => name.toUpperCase() + number.toString(),
        ],
    }),
})
`

	candidates, err := probePositionCandidates(source)
	if err != nil {
		t.Fatalf("probePositionCandidates() error = %v", err)
	}
	candidates, err = selectProbePositionCandidates(candidates, "selectors")
	if err != nil {
		t.Fatalf("selectProbePositionCandidates() error = %v", err)
	}
	candidates, err = probeMemberPositionCandidates(source, candidates, "capitalizedName")
	if err != nil {
		t.Fatalf("probeMemberPositionCandidates() error = %v", err)
	}

	_, err = probeElementPositionCandidates(source, candidates, "2")
	if err == nil {
		t.Fatalf("expected element selection error")
	}
	if !strings.Contains(err.Error(), `available elements: first, 0, 1, last`) {
		t.Fatalf("unexpected element selection error: %v", err)
	}
}

func TestProbePropertyPositionCandidatesSelectsNestedProperty(t *testing.T) {
	source := `
import { kea } from 'kea'

export const logic = kea({
    reducers: () => ({
        selectedActionId: [
            null as number | null,
            {
                selectAction: (_, { id }) => (id ? parseInt(id) : null),
                newAction: () => null,
            },
        ],
    }),
})
`

	candidates, err := probePositionCandidates(source)
	if err != nil {
		t.Fatalf("probePositionCandidates() error = %v", err)
	}
	candidates, err = selectProbePositionCandidates(candidates, "reducers")
	if err != nil {
		t.Fatalf("selectProbePositionCandidates() error = %v", err)
	}
	candidates, err = probeMemberPositionCandidates(source, candidates, "selectedActionId")
	if err != nil {
		t.Fatalf("probeMemberPositionCandidates() error = %v", err)
	}
	candidates, err = probeElementPositionCandidates(source, candidates, "last")
	if err != nil {
		t.Fatalf("probeElementPositionCandidates() error = %v", err)
	}

	got, err := probePropertyPositionCandidates(source, candidates, "selectAction")
	if err != nil {
		t.Fatalf("probePropertyPositionCandidates() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 property candidate, got %d", len(got))
	}
	if got[0].nestedName != "selectAction" {
		t.Fatalf("unexpected nested property candidate: %+v", got[0])
	}
	selected := strings.TrimSpace(source[got[0].valuePosition:got[0].valueEnd])
	if selected != "(_, { id }) => (id ? parseInt(id) : null)" {
		t.Fatalf("unexpected selected property value: %q", selected)
	}
	if len(got[0].typePositions) == 0 {
		t.Fatalf("expected callback probe positions on nested property candidate: %+v", got[0])
	}
}

func TestProbePropertyPositionCandidatesRejectsUnknownProperty(t *testing.T) {
	source := `
import { kea } from 'kea'

export const logic = kea({
    loaders: {
        dashboard: {
            __default: null,
            addDashboard: ({ name }) => ({ name }),
        },
    },
})
`

	candidates, err := probePositionCandidates(source)
	if err != nil {
		t.Fatalf("probePositionCandidates() error = %v", err)
	}
	candidates, err = selectProbePositionCandidates(candidates, "loaders")
	if err != nil {
		t.Fatalf("selectProbePositionCandidates() error = %v", err)
	}
	candidates, err = probeMemberPositionCandidates(source, candidates, "dashboard")
	if err != nil {
		t.Fatalf("probeMemberPositionCandidates() error = %v", err)
	}

	_, err = probePropertyPositionCandidates(source, candidates, "missing")
	if err == nil {
		t.Fatalf("expected property selection error")
	}
	if !strings.Contains(err.Error(), `available properties: __default, addDashboard`) {
		t.Fatalf("unexpected property selection error: %v", err)
	}
	if !strings.Contains(err.Error(), `sample "loaders" member "dashboard"`) {
		t.Fatalf("expected target context in property selection error: %v", err)
	}
}

func TestParseProbeElementSelectorRejectsInvalidValue(t *testing.T) {
	_, _, err := parseProbeElementSelector("projector")
	if err == nil {
		t.Fatalf("expected invalid element selector error")
	}
	if !strings.Contains(err.Error(), "use first, last, or a zero-based index") {
		t.Fatalf("unexpected selector parse error: %v", err)
	}
}

func TestProbeCandidateTypePositionsDedupesAndKeepsValuePositionFirst(t *testing.T) {
	candidate := probePositionCandidate{
		valuePosition: 7,
		typePositions: []int{7, 11, 7, 13},
	}

	got := probeCandidateTypePositions(candidate)
	want := []int{7, 11, 13}
	if len(got) != len(want) {
		t.Fatalf("unexpected type positions: got %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("unexpected type positions: got %v, want %v", got, want)
		}
	}
}

func TestNormalizeProbePositionCandidatesUsesUTF16Offsets(t *testing.T) {
	source := "const note = '👈'\nafterMount: () => 'ok'\n"
	byteName := strings.Index(source, "afterMount")
	byteValue := strings.Index(source, "() =>")
	byteEnd := byteValue + len("() => 'ok'")
	byteInner := byteValue + len("() => ")
	if byteName == -1 || byteValue == -1 {
		t.Fatalf("expected test positions inside source")
	}

	got := normalizeProbePositionCandidates(newProbeSourceOffsetMap(source), []probePositionCandidate{
		{
			namePosition:  byteName,
			valuePosition: byteValue,
			valueEnd:      byteEnd,
			typePositions: []int{byteValue, byteInner},
		},
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 normalized candidate, got %d", len(got))
	}

	wantName := probeTestUTF16Offset(source, byteName)
	wantValue := probeTestUTF16Offset(source, byteValue)
	wantEnd := probeTestUTF16Offset(source, byteEnd)
	wantInner := probeTestUTF16Offset(source, byteInner)
	if got[0].namePosition != wantName || got[0].valuePosition != wantValue || got[0].valueEnd != wantEnd {
		t.Fatalf("unexpected normalized positions: got %+v want name=%d value=%d end=%d", got[0], wantName, wantValue, wantEnd)
	}
	if len(got[0].typePositions) != 2 || got[0].typePositions[0] != wantValue || got[0].typePositions[1] != wantInner {
		t.Fatalf("unexpected normalized type positions: %+v", got[0].typePositions)
	}
	if newProbeSourceOffsetMap(source).utf16Length() != len(utf16.Encode([]rune(source))) {
		t.Fatalf("expected UTF-16 source length for %q", source)
	}
}

func TestProbeLocationHandlesForMethodPrefersSameFileLocations(t *testing.T) {
	session := &probeSession{
		File: "/Users/example/logic.ts",
		SampleLocations: []string{
			"10.20.303./users/example/logic.ts",
			"30.40.211./users/example/logic.ts",
			"0.100.265./users/example/lib.es5.d.ts",
		},
	}

	got := probeLocationHandlesForMethod("getTypeAtLocation", session)
	if len(got) != 2 {
		t.Fatalf("expected 2 same-file handles, got %d", len(got))
	}
	for _, handle := range got {
		if !strings.Contains(handle, "/users/example/logic.ts") {
			t.Fatalf("unexpected fallback handles: %+v", got)
		}
	}
}

func TestDefaultProbeMethodsIncludeTypeToString(t *testing.T) {
	methods := defaultProbeMethods()
	for _, method := range methods {
		if method == "typeToString" {
			return
		}
	}
	t.Fatalf("expected default probe methods to include typeToString, got %+v", methods)
}

func TestProbeShouldUseFallbackTypePrefersNonWeakType(t *testing.T) {
	current := probeFallbackType{
		ID:    "type-any",
		Flags: 1,
		Rank:  probeFallbackRankContextual,
	}
	next := probeFallbackType{
		ID:          "type-array",
		Flags:       1048576,
		ObjectFlags: 136986628,
		Rank:        probeFallbackRankLocation,
	}

	if !probeShouldUseFallbackType(current, next) {
		t.Fatalf("expected non-weak fallback type to replace weak fallback type")
	}
}

func TestProbeShouldUseFallbackTypePrefersHigherRankWhenStrengthMatches(t *testing.T) {
	current := probeFallbackType{
		ID:          "type-position",
		Flags:       1048576,
		ObjectFlags: 136986628,
		Rank:        probeFallbackRankPosition,
	}
	next := probeFallbackType{
		ID:          "type-location",
		Flags:       1048576,
		ObjectFlags: 136986628,
		Rank:        probeFallbackRankLocation,
	}

	if !probeShouldUseFallbackType(current, next) {
		t.Fatalf("expected higher-rank fallback type to replace lower-rank fallback type")
	}
}

func TestProbeShouldUseFallbackTypeKeepsStrongTypeOverWeakFallback(t *testing.T) {
	current := probeFallbackType{
		ID:          "type-array",
		Flags:       1048576,
		ObjectFlags: 136986628,
		Rank:        probeFallbackRankLocation,
	}
	next := probeFallbackType{
		ID:    "type-any",
		Flags: 1,
		Rank:  probeFallbackRankContextual,
	}

	if probeShouldUseFallbackType(current, next) {
		t.Fatalf("expected weak fallback type not to replace stronger fallback type")
	}
}

func TestProbeTypeIDFromParamsReturnsTypeHandle(t *testing.T) {
	got := probeTypeIDFromParams(map[string]any{
		"type": "type-1",
	})
	if got != "type-1" {
		t.Fatalf("probeTypeIDFromParams() = %q, want %q", got, "type-1")
	}
}

func TestProbeTypeIDFromParamsIgnoresMissingType(t *testing.T) {
	if got := probeTypeIDFromParams(map[string]any{"project": "project-1"}); got != "" {
		t.Fatalf("probeTypeIDFromParams() = %q, want empty string", got)
	}
}

func TestProbeSymbolIDFromParamsReturnsSymbolHandle(t *testing.T) {
	got := probeSymbolIDFromParams(map[string]any{
		"symbol": "symbol-1",
	})
	if got != "symbol-1" {
		t.Fatalf("probeSymbolIDFromParams() = %q, want %q", got, "symbol-1")
	}
}

func TestProbeSymbolIDFromParamsIgnoresMissingSymbol(t *testing.T) {
	if got := probeSymbolIDFromParams(map[string]any{"project": "project-1"}); got != "" {
		t.Fatalf("probeSymbolIDFromParams() = %q, want empty string", got)
	}
}

func TestProbeSignatureIDFromParamsReturnsSignatureHandle(t *testing.T) {
	got := probeSignatureIDFromParams(map[string]any{
		"signature": "signature-1",
	})
	if got != "signature-1" {
		t.Fatalf("probeSignatureIDFromParams() = %q, want %q", got, "signature-1")
	}
}

func TestProbeSignatureIDFromParamsIgnoresMissingSignature(t *testing.T) {
	if got := probeSignatureIDFromParams(map[string]any{"project": "project-1"}); got != "" {
		t.Fatalf("probeSignatureIDFromParams() = %q, want empty string", got)
	}
}

func TestProbeSelectSignaturePrefersMatchingID(t *testing.T) {
	signatures := []*tsgoapi.SignatureResponse{
		{ID: "signature-1"},
		{ID: "signature-2"},
	}

	got, ok := probeSelectSignature(signatures, "signature-2")
	if !ok || got == nil {
		t.Fatalf("expected matching signature")
	}
	if got.ID != "signature-2" {
		t.Fatalf("unexpected signature: %+v", got)
	}
}

func TestProbeSelectSignatureFallsBackToFirst(t *testing.T) {
	signatures := []*tsgoapi.SignatureResponse{
		nil,
		{ID: "signature-1"},
		{ID: "signature-2"},
	}

	got, ok := probeSelectSignature(signatures, "")
	if !ok || got == nil {
		t.Fatalf("expected first available signature")
	}
	if got.ID != "signature-1" {
		t.Fatalf("unexpected signature: %+v", got)
	}
}

func TestProbeSelectSignatureRejectsMissingID(t *testing.T) {
	signatures := []*tsgoapi.SignatureResponse{
		{ID: "signature-1"},
	}

	got, ok := probeSelectSignature(signatures, "signature-2")
	if ok || got != nil {
		t.Fatalf("expected missing signature selection to fail, got %+v", got)
	}
}

func TestProbePropertyDetailsFallbackTypeUsesReturnTypeHandle(t *testing.T) {
	signature := &tsgoapi.SignatureResponse{ID: "signature-1"}
	returnType := &tsgoapi.TypeResponse{ID: "type-return"}

	got := probePropertyDetailsFallbackType("type-current", signature, returnType)
	if got != "type-return" {
		t.Fatalf("probePropertyDetailsFallbackType() = %q, want %q", got, "type-return")
	}
}

func TestProbePropertyDetailsFallbackTypeRejectsSameType(t *testing.T) {
	signature := &tsgoapi.SignatureResponse{ID: "signature-1"}
	returnType := &tsgoapi.TypeResponse{ID: "type-current"}

	if got := probePropertyDetailsFallbackType("type-current", signature, returnType); got != "" {
		t.Fatalf("probePropertyDetailsFallbackType() = %q, want empty string", got)
	}
}

func TestProbePropertyDetailsFallbackTypeRejectsMissingSignature(t *testing.T) {
	returnType := &tsgoapi.TypeResponse{ID: "type-return"}

	if got := probePropertyDetailsFallbackType("type-current", nil, returnType); got != "" {
		t.Fatalf("probePropertyDetailsFallbackType() = %q, want empty string", got)
	}
}

func TestProbeIsTupleLikeTypeText(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "[(state: any) => string, (value: string) => string]", want: true},
		{text: "readonly [string, number]", want: true},
		{text: "Array<string>", want: false},
		{text: "{ name: string }", want: false},
	}

	for _, tt := range tests {
		if got := probeIsTupleLikeTypeText(tt.text); got != tt.want {
			t.Fatalf("probeIsTupleLikeTypeText(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestProbeTrimTupleSymbolSurfaceKeepsTupleEntriesOnly(t *testing.T) {
	symbols := []*tsgoapi.SymbolResponse{
		{Name: "push", ID: "symbol-push"},
		{Name: "1", ID: "symbol-1"},
		{Name: "length", ID: "symbol-length"},
		{Name: "0", ID: "symbol-0"},
		{Name: "map", ID: "symbol-map"},
	}

	got, ok := probeTrimTupleSymbolSurface(symbols)
	if !ok {
		t.Fatalf("expected tuple surface trim to succeed")
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 tuple entries, got %d", len(got))
	}
	if got[0].Name != "0" || got[1].Name != "1" || got[2].Name != "length" {
		t.Fatalf("unexpected trimmed tuple surface: %+v", got)
	}
}

func TestProbeTrimTupleSymbolSurfaceRejectsNonTupleMembers(t *testing.T) {
	symbols := []*tsgoapi.SymbolResponse{
		{Name: "push", ID: "symbol-push"},
		{Name: "length", ID: "symbol-length"},
		{Name: "map", ID: "symbol-map"},
	}

	got, ok := probeTrimTupleSymbolSurface(symbols)
	if ok || got != nil {
		t.Fatalf("expected non-tuple surface trim to fail, got %+v", got)
	}
}

func TestParseProbeLocationHandle(t *testing.T) {
	got, ok := parseProbeLocationHandle("12.34.220./tmp/example.ts")
	if !ok {
		t.Fatalf("expected location handle parse to succeed")
	}
	if got.Start != 12 || got.End != 34 || got.Kind != 220 || got.File != "/tmp/example.ts" {
		t.Fatalf("unexpected parsed location handle: %+v", got)
	}
}

func TestProbeTextForLocationHandleUsesUTF16Offsets(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "logic.ts")
	source := "const marker = '👈'\nconst fn = (payload: string) => payload\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	start := strings.Index(source, "(payload: string)")
	end := start + len("(payload: string)")
	offsets := newProbeSourceOffsetMap(source)
	handle := probeLocationHandle{
		Start: offsets.utf16Offset(start),
		End:   offsets.utf16Offset(end),
		File:  path,
	}

	got, ok := probeTextForLocationHandle(handle)
	if !ok {
		t.Fatalf("expected text lookup to succeed")
	}
	if got != "(payload: string)" {
		t.Fatalf("probeTextForLocationHandle() = %q, want %q", got, "(payload: string)")
	}
}

func TestProbeResolveLocationHandlePathMatchesCaseInsensitively(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "AutoImportLogic.ts")
	if err := os.WriteFile(path, []byte("export const x = 1\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	got, ok := probeResolveLocationHandlePath(filepath.Join(tempDir, "autoimportlogic.ts"))
	if !ok {
		t.Fatalf("expected case-insensitive path resolution to succeed")
	}
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("os.Stat(%q) error = %v", got, err)
	}
	wantInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat(%q) error = %v", path, err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("probeResolveLocationHandlePath() = %q, want same file as %q", got, path)
	}
}

func TestProbeSignatureParameterNamesFromDeclaration(t *testing.T) {
	text := `(payload: { name: string }, breakpoint: BreakPointFunction, action: { type: string; payload: { name: string } }, previousState: any) => void | Promise<void>`

	got, thisName, ok := probeSignatureParameterNamesFromDeclaration(text, false)
	if !ok {
		t.Fatalf("expected parameter-name parse to succeed")
	}
	if thisName != "" {
		t.Fatalf("unexpected this parameter name: %q", thisName)
	}
	want := []string{"payload", "breakpoint", "action", "previousState"}
	if len(got) != len(want) {
		t.Fatalf("unexpected parameter names: got %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("unexpected parameter names: got %v, want %v", got, want)
		}
	}
}

func TestProbeSignatureParameterNamesFromDeclarationHandlesThisAndDestructuring(t *testing.T) {
	text := `(this: void, [_, error]: LoadResult, { id }: { id: string }, ...rest: string[]) => void`

	got, thisName, ok := probeSignatureParameterNamesFromDeclaration(text, true)
	if !ok {
		t.Fatalf("expected parameter-name parse to succeed")
	}
	if thisName != "this" {
		t.Fatalf("unexpected this parameter name: %q", thisName)
	}
	want := []string{"[_, error]", "{ id }", "rest"}
	if len(got) != len(want) {
		t.Fatalf("unexpected parameter names: got %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("unexpected parameter names: got %v, want %v", got, want)
		}
	}
}

func TestProbeSignatureParameterDetailJSONIncludesDeclaredType(t *testing.T) {
	raw, err := json.Marshal(probeSignatureParameterDetail{
		Index:  1,
		Name:   "filter",
		Symbol: "symbol-1",
		Type: probeTypeSummary{
			ID:   "type-1",
			Text: "{ name: string }",
		},
		DeclaredType: probeTypeSummary{
			ID:   "type-2",
			Text: "A5",
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"name":"filter"`) {
		t.Fatalf("expected name in JSON, got %s", text)
	}
	if !strings.Contains(text, `"declaredType":{"id":"type-2","text":"A5"}`) {
		t.Fatalf("expected declaredType in JSON, got %s", text)
	}
}

func TestProbeSignatureDetailJSONIncludesDeclarationText(t *testing.T) {
	raw, err := json.Marshal(probeSignatureDetail{
		Signature:       "signature-1",
		Declaration:     "10.20.220./tmp/example.ts",
		DeclarationText: "(filter: A5) => ({ a6: filter.a6 })",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"declarationText":"(filter: A5) =\u003e ({ a6: filter.a6 })"`) {
		t.Fatalf("expected declarationText in JSON, got %s", text)
	}
}

func TestProbeResultHasValue(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "empty", raw: "", want: false},
		{name: "null", raw: "null", want: false},
		{name: "null array", raw: "[null]", want: false},
		{name: "mixed array", raw: "[null,{\"id\":\"t1\"}]", want: true},
		{name: "object", raw: "{\"id\":\"t1\"}", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := probeResultHasValue([]byte(tt.raw)); got != tt.want {
				t.Fatalf("probeResultHasValue(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestClassifyProbeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "unknown", err: simpleProbeError("unknown API method \"foo\""), want: "unknown-method"},
		{name: "invalid", err: simpleProbeError("invalid node handle"), want: "known-method-bad-params"},
		{name: "panic", err: simpleProbeError("panic: runtime error: invalid memory address or nil pointer dereference"), want: "call-error"},
		{name: "other", err: simpleProbeError("connection closed"), want: "call-error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyProbeError(tt.err); got != tt.want {
				t.Fatalf("classifyProbeError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

type simpleProbeError string

func (e simpleProbeError) Error() string {
	return string(e)
}

func encodeProbeSourceFileNodeBlob(records ...[]uint32) []byte {
	return encodeProbeSourceFileNodeBlobWithPadding(0, records...)
}

func encodeProbeSourceFileNodeBlobWithPadding(padding int, records ...[]uint32) []byte {
	var buf bytes.Buffer
	buf.Write(make([]byte, 16))
	buf.Write(make([]byte, padding))
	for _, record := range records {
		for _, value := range record {
			_ = binary.Write(&buf, binary.LittleEndian, value)
		}
	}
	return buf.Bytes()
}

func probeTestUTF16Offset(source string, byteOffset int) int {
	return len(utf16.Encode([]rune(source[:byteOffset])))
}
