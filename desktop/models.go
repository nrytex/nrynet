package main

import (
	"time"

	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/model"
)

var appVersion = "1.0.0"

type AppConfig struct {
	ServerURL          string `json:"serverUrl"`
	DataAddress        string `json:"dataAddress"`
	Transport          string `json:"transport"`
	QUICAddress        string `json:"quicAddress"`
	CAFile             string `json:"caFile"`
	Token              string `json:"token"`
	Name               string `json:"name"`
	DeviceID           string `json:"deviceId"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	AutoStart          bool   `json:"autoStart"`
}

type AppConfigPatch struct {
	ServerURL          *string `json:"serverUrl"`
	DataAddress        *string `json:"dataAddress"`
	Transport          *string `json:"transport"`
	QUICAddress        *string `json:"quicAddress"`
	CAFile             *string `json:"caFile"`
	Token              *string `json:"token"`
	Name               *string `json:"name"`
	DeviceID           *string `json:"deviceId"`
	InsecureSkipVerify *bool   `json:"insecureSkipVerify"`
	AutoStart          *bool   `json:"autoStart"`
}

func (p AppConfigPatch) Apply(base AppConfig) AppConfig {
	applyPatchValue(&base.ServerURL, p.ServerURL)
	applyPatchValue(&base.DataAddress, p.DataAddress)
	applyPatchValue(&base.Transport, p.Transport)
	applyPatchValue(&base.QUICAddress, p.QUICAddress)
	applyPatchValue(&base.CAFile, p.CAFile)
	applyPatchValue(&base.Token, p.Token)
	applyPatchValue(&base.Name, p.Name)
	applyPatchValue(&base.DeviceID, p.DeviceID)
	applyPatchValue(&base.InsecureSkipVerify, p.InsecureSkipVerify)
	applyPatchValue(&base.AutoStart, p.AutoStart)
	return base
}

func applyPatchValue[T any](target *T, value *T) {
	if value != nil {
		*target = *value
	}
}

type RuntimeStatus struct {
	Connected     bool      `json:"connected"`
	State         string    `json:"state"`
	Message       string    `json:"message"`
	Version       string    `json:"version"`
	UploadBytes   int64     `json:"uploadBytes"`
	DownloadBytes int64     `json:"downloadBytes"`
	LastStartedAt time.Time `json:"lastStartedAt,omitempty"`
	LastStoppedAt time.Time `json:"lastStoppedAt,omitempty"`
}

type LogEntry struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields"`
}

type UpdateResult struct {
	Started bool   `json:"started"`
	Message string `json:"message"`
}

type DesktopSnapshot struct {
	Config  AppConfig      `json:"config"`
	Status  RuntimeStatus  `json:"status"`
	Tunnels []model.Tunnel `json:"tunnels"`
	Logs    []LogEntry     `json:"logs"`
}

func (c AppConfig) toClientConfig() config.ClientConfig {
	return config.ClientConfig{
		ServerURL: c.ServerURL, DataAddress: c.DataAddress,
		Transport: c.Transport, QUICAddress: c.QUICAddress,
		CAFile: c.CAFile,
		Token:  c.Token, Name: c.Name, DeviceID: c.DeviceID,
		InsecureSkipVerify: c.InsecureSkipVerify,
	}
}

func configFromClient(c config.ClientConfig) AppConfig {
	return AppConfig{
		ServerURL: c.ServerURL, DataAddress: c.DataAddress,
		Transport: c.Transport, QUICAddress: c.QUICAddress,
		CAFile: c.CAFile,
		Token:  c.Token, Name: c.Name, DeviceID: c.DeviceID,
		InsecureSkipVerify: c.InsecureSkipVerify,
	}
}
