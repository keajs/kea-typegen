package main

import "testing"

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
