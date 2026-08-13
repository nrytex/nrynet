package main

import (
	"context"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

type fakeRunner struct {
	cfg         updater.Config
	initCalls   int
	checks      int
	downloads   int
	restarts    int
	release     *updater.Release
	downloadErr error
	restartErr  error
}

func (f *fakeRunner) Init(cfg updater.Config) error {
	f.cfg = cfg
	f.initCalls++
	return nil
}

func (f *fakeRunner) Check(context.Context) (*updater.Release, error) {
	f.checks++
	return f.release, nil
}

func (f *fakeRunner) DownloadAndInstall(context.Context) error {
	f.downloads++
	return f.downloadErr
}

func (f *fakeRunner) Restart(context.Context) error {
	f.restarts++
	return f.restartErr
}

func TestUpdateServiceConfiguresGitHubOnce(t *testing.T) {
	runner := &fakeRunner{}
	svc := NewUpdateService(runner)
	if err := svc.ConfigureAutomatic(); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CheckForUpdate(); err != nil {
		t.Fatal(err)
	}
	if runner.initCalls != 1 || runner.checks != 1 {
		t.Fatalf("init=%d checks=%d", runner.initCalls, runner.checks)
	}
	if runner.cfg.CurrentVersion != appVersion || runner.cfg.CheckInterval != 0 {
		t.Fatalf("unexpected updater config: version=%q interval=%v", runner.cfg.CurrentVersion, runner.cfg.CheckInterval)
	}
	if runner.cfg.Window != updater.WindowNone {
		t.Fatalf("unexpected updater window mode: %#v", runner.cfg.Window)
	}
	if len(runner.cfg.Providers) != 1 || runner.cfg.Providers[0].Name() != "github-release" {
		t.Fatalf("unexpected providers: %#v", runner.cfg.Providers)
	}
	if len(runner.cfg.PublicKey) != 0 {
		t.Fatal("GitHub checksum updates should not require a user-supplied public key")
	}
}

func TestUpdateServiceReportsAvailableRelease(t *testing.T) {
	runner := &fakeRunner{release: &updater.Release{
		Version:  "1.1.0",
		Metadata: map[string]any{downloadURLKey: "https://github.com/nrytex/nrynet/releases/download/v1.1.0/nrynet-desktop-windows-amd64.zip"},
	}}
	svc := NewUpdateService(runner)
	if err := svc.ConfigureAutomatic(); err != nil {
		t.Fatal(err)
	}
	result, err := svc.CheckForUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Available || result.LatestVersion != "1.1.0" {
		t.Fatalf("unexpected update result: %+v", result)
	}
	if result.DownloadURL == "" || result.Message == "" || !result.Ready || result.DownloadState != "ready" {
		t.Fatalf("update result missing download details: %+v", result)
	}
	if runner.downloads != 1 {
		t.Fatalf("downloads=%d, want 1", runner.downloads)
	}
}

func TestUpdateServiceAppliesDownloadedUpdate(t *testing.T) {
	runner := &fakeRunner{release: &updater.Release{
		Version:  "2.0.0",
		Metadata: map[string]any{downloadURLKey: "https://example.com/nrynet.zip"},
	}}
	svc := NewUpdateService(runner)
	if _, err := svc.CheckForUpdate(); err != nil {
		t.Fatal(err)
	}
	if err := svc.ApplyUpdate(); err != nil {
		t.Fatal(err)
	}
	if runner.restarts != 1 {
		t.Fatalf("restarts=%d, want 1", runner.restarts)
	}
}

func TestUpdateServiceReportsUpToDate(t *testing.T) {
	runner := &fakeRunner{}
	svc := NewUpdateService(runner)
	if err := svc.ConfigureAutomatic(); err != nil {
		t.Fatal(err)
	}
	result, err := svc.CheckForUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if result.Available || !result.Checked {
		t.Fatalf("unexpected up-to-date result: %+v", result)
	}
}

func TestUpdateServiceDownloadsOnlyOnceForSameRelease(t *testing.T) {
	runner := &fakeRunner{release: &updater.Release{
		Version:  "2.0.0",
		Metadata: map[string]any{downloadURLKey: "https://example.com/nrynet.zip"},
	}}
	svc := NewUpdateService(runner)
	if _, err := svc.CheckForUpdate(); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CheckForUpdate(); err != nil {
		t.Fatal(err)
	}
	if runner.downloads != 1 {
		t.Fatalf("downloads=%d, want 1", runner.downloads)
	}
}

func TestUpdateServiceLastResultFeedsSnapshot(t *testing.T) {
	runner := &fakeRunner{release: &updater.Release{
		Version:  "1.1.0",
		Metadata: map[string]any{downloadURLKey: "https://github.com/nrytex/nrynet/releases/download/v1.1.0/nrynet-desktop-windows-amd64.zip"},
	}}
	svc := NewUpdateService(runner)
	if err := svc.ConfigureAutomatic(); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CheckForUpdate(); err != nil {
		t.Fatal(err)
	}
	last := svc.LastResult()
	if last == nil || !last.Available || last.LatestVersion != "1.1.0" {
		t.Fatalf("unexpected last result: %+v", last)
	}
	if last.DownloadURL == "" {
		t.Fatalf("last result missing download URL: %+v", last)
	}
}

func TestUpdateServiceLastResultNilWhenUpToDate(t *testing.T) {
	runner := &fakeRunner{}
	svc := NewUpdateService(runner)
	if err := svc.ConfigureAutomatic(); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CheckForUpdate(); err != nil {
		t.Fatal(err)
	}
	if last := svc.LastResult(); last != nil {
		t.Fatalf("expected no update banner, got %+v", last)
	}
}

func TestUpdateServiceReportsDownloadFailure(t *testing.T) {
	runner := &fakeRunner{
		release:     &updater.Release{Version: "2.0.0", Metadata: map[string]any{downloadURLKey: "https://example.com/nrynet.zip"}},
		downloadErr: context.DeadlineExceeded,
	}
	svc := NewUpdateService(runner)
	result, err := svc.CheckForUpdate()
	if err == nil || result.DownloadState != "error" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if svc.LastResult() != nil {
		t.Fatal("failed update should not remain as an actionable banner")
	}
}
