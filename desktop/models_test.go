package main

import (
	"encoding/json"
	"testing"
)

func TestAppConfigPatchPreservesUnsubmittedSettings(t *testing.T) {
	base := AppConfig{
		ServerURL:   "wss://server.example/agent/connect",
		DataAddress: "server.example:7001",
		Transport:   "websocket",
	}
	var patch AppConfigPatch
	if err := json.Unmarshal([]byte(`{"token":"agent-token"}`), &patch); err != nil {
		t.Fatal(err)
	}
	got := patch.Apply(base)
	if got.ServerURL != base.ServerURL || got.DataAddress != base.DataAddress || got.Token != "agent-token" {
		t.Fatalf("partial save lost settings: %+v", got)
	}
}

func TestAppConfigPatchCanClearSubmittedSetting(t *testing.T) {
	var patch AppConfigPatch
	if err := json.Unmarshal([]byte(`{"serverUrl":""}`), &patch); err != nil {
		t.Fatal(err)
	}
	got := patch.Apply(AppConfig{ServerURL: "wss://old.example/agent/connect"})
	if got.ServerURL != "" {
		t.Fatalf("submitted empty value was ignored: %+v", got)
	}
}
