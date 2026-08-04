package main

import (
	"context"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

type fakeRunner struct {
	cfg       updater.Config
	initCalls int
	checks    int
}

func (f *fakeRunner) Init(cfg updater.Config) error {
	f.cfg = cfg
	f.initCalls++
	return nil
}

func (f *fakeRunner) CheckAndInstall(context.Context) error {
	f.checks++
	return nil
}

func TestUpdateServiceConfiguresGitHubOnce(t *testing.T) {
	runner := &fakeRunner{}
	svc := NewUpdateService(runner)
	if err := svc.ConfigureAutomatic(); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CheckAndInstall(); err != nil {
		t.Fatal(err)
	}
	if runner.initCalls != 1 || runner.checks != 1 {
		t.Fatalf("init=%d checks=%d", runner.initCalls, runner.checks)
	}
	if runner.cfg.CurrentVersion != appVersion || runner.cfg.CheckInterval != automaticUpdateInterval {
		t.Fatalf("unexpected updater config: version=%q interval=%v", runner.cfg.CurrentVersion, runner.cfg.CheckInterval)
	}
	if len(runner.cfg.Providers) != 1 || runner.cfg.Providers[0].Name() != "github-release" {
		t.Fatalf("unexpected providers: %#v", runner.cfg.Providers)
	}
	if len(runner.cfg.PublicKey) != 0 {
		t.Fatal("GitHub checksum updates should not require a user-supplied public key")
	}
}
