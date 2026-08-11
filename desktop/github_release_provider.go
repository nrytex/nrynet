package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"golang.org/x/mod/semver"
)

const (
	githubWebURL      = "https://github.com"
	checksumAssetName = "SHA256SUMS"
	downloadURLKey    = "github.release.downloadURL"
	userAgentPrefix   = "Nrynet-Desktop-Updater/"
)

type githubReleaseProvider struct {
	repository string
	baseURL    string
	client     *http.Client
}

func newGitHubReleaseProvider(repository string, client *http.Client) (*githubReleaseProvider, error) {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("github release repository must be in owner/name form: %q", repository)
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &githubReleaseProvider{repository: repository, baseURL: githubWebURL, client: client}, nil
}

func (p *githubReleaseProvider) Name() string {
	return "github-release"
}

func (p *githubReleaseProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	tag, err := p.latestTag(ctx)
	if err != nil {
		return nil, err
	}
	if !isNewerVersion(tag, req.CurrentVersion) {
		return nil, nil
	}
	filename, filetype, err := desktopReleaseAsset(req.Platform, req.Arch)
	if err != nil {
		return nil, err
	}
	digest, err := p.releaseChecksum(ctx, tag, filename)
	if err != nil {
		return nil, err
	}
	downloadURL := p.releaseAssetURL(tag, filename)
	return &updater.Release{
		Version: strings.TrimPrefix(tag, "v"), Channel: "stable", Name: "Nrynet " + tag,
		Artifact:     updater.Artifact{Filename: filename, Filetype: filetype, Platform: req.Platform, Arch: req.Arch},
		Verification: &updater.Verification{DigestAlgo: "sha256", Digest: digest},
		Metadata:     map[string]any{downloadURLKey: downloadURL},
	}, nil
}

func (p *githubReleaseProvider) Download(
	ctx context.Context,
	release *updater.Release,
	destination io.Writer,
	onProgress func(written, total int64),
) error {
	downloadURL, err := releaseDownloadURL(release)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("download GitHub release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download GitHub release: HTTP %d", response.StatusCode)
	}
	return copyWithProgress(destination, response.Body, response.ContentLength, onProgress)
}

func (p *githubReleaseProvider) latestTag(ctx context.Context) (string, error) {
	latestURL := p.baseURL + "/" + p.repository + "/releases/latest"
	var lastErr error
	userAgent := userAgentPrefix + appVersion
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodHead, latestURL, nil)
		if err != nil {
			return "", err
		}
		request.Header.Set("User-Agent", userAgent)
		response, err := p.client.Do(request)
		if err != nil {
			lastErr = fmt.Errorf("resolve latest GitHub release: %w", err)
			continue
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("resolve latest GitHub release: HTTP %d", response.StatusCode)
			continue
		}
		prefix := "/" + p.repository + "/releases/tag/"
		if !strings.HasPrefix(response.Request.URL.Path, prefix) {
			return "", errors.New("resolve latest GitHub release: redirect did not contain a tag")
		}
		tag, err := url.PathUnescape(strings.TrimPrefix(response.Request.URL.Path, prefix))
		if err != nil || tag == "" {
			return "", errors.New("resolve latest GitHub release: invalid tag")
		}
		return tag, nil
	}
	return "", lastErr
}

func (p *githubReleaseProvider) releaseChecksum(ctx context.Context, tag, filename string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.releaseAssetURL(tag, checksumAssetName), nil)
	if err != nil {
		return nil, err
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("load release checksums: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("load release checksums: HTTP %d", response.StatusCode)
	}
	return findSHA256(response.Body, filename)
}

func (p *githubReleaseProvider) releaseAssetURL(tag, filename string) string {
	return p.baseURL + "/" + p.repository + "/releases/download/" + url.PathEscape(tag) + "/" + url.PathEscape(filename)
}

func desktopReleaseAsset(platform, arch string) (string, string, error) {
	switch platform {
	case "windows":
		if arch == "amd64" {
			return "nrynet-desktop-windows-amd64.zip", "zip", nil
		}
	case "darwin":
		if arch == "amd64" || arch == "arm64" {
			return "nrynet-desktop-darwin-universal.tar.gz", "tar.gz", nil
		}
	}
	return "", "", fmt.Errorf("no desktop release asset for %s/%s", platform, arch)
}

func isNewerVersion(candidate, current string) bool {
	return semver.Compare(canonicalVersion(candidate), canonicalVersion(current)) > 0
}

func canonicalVersion(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	return value
}

func findSHA256(reader io.Reader, filename string) ([]byte, error) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || path.Base(strings.TrimPrefix(fields[1], "*")) != filename {
			continue
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != 32 {
			return nil, fmt.Errorf("invalid SHA-256 for %s", filename)
		}
		return digest, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("release checksums do not contain %s", filename)
}

func releaseDownloadURL(release *updater.Release) (string, error) {
	if release == nil || release.Metadata == nil {
		return "", errors.New("GitHub release metadata is missing")
	}
	downloadURL, ok := release.Metadata[downloadURLKey].(string)
	if !ok || downloadURL == "" {
		return "", errors.New("GitHub release download URL is missing")
	}
	return downloadURL, nil
}

func copyWithProgress(destination io.Writer, source io.Reader, total int64, progress func(int64, int64)) error {
	written := int64(0)
	buffer := make([]byte, 64*1024)
	for {
		count, readErr := source.Read(buffer)
		if count > 0 {
			writtenNow, err := destination.Write(buffer[:count])
			if err != nil {
				return err
			}
			if writtenNow != count {
				return io.ErrShortWrite
			}
			written += int64(writtenNow)
			progress(written, total)
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
