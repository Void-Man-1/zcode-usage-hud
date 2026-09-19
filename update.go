package main

// Updater: check the repo's latest GitHub release, semver-compare, ask,
// then update silently (download setup asset to %TEMP%, run /VERYSILENT,
// exit so the installer can replace files).

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const releasesAPIURL = "https://api.github.com/repos/Void-Man-1/zcode-usage-hud/releases/latest"

type releaseInfo struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// parseSemver extracts major/minor/patch from a "v1.2.3" string. Pure -
// unit-tested.
func parseSemver(tag string) ([3]int, bool) {
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, "v")
	parts := strings.SplitN(tag, ".", 3)
	if len(parts) < 2 {
		return [3]int{}, false
	}
	var out [3]int
	for i := 0; i < 3; i++ {
		if i >= len(parts) {
			out[i] = 0
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// isNewerVersion reports whether candidate beats current. Pure -
// unit-tested.
func isNewerVersion(current, candidate string) bool {
	cur, okc := parseSemver(current)
	cand, okn := parseSemver(candidate)
	if !okc || !okn {
		return false
	}
	for i := 0; i < 3; i++ {
		if cand[i] != cur[i] {
			return cand[i] > cur[i]
		}
	}
	return false
}

// fetchLatestRelease queries the GitHub releases API.
func fetchLatestRelease() (*releaseInfo, error) {
	req, err := http.NewRequest("GET", releasesAPIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var rel releaseInfo
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("no tag in release response")
	}
	return &rel, nil
}

// setupAssetURL picks the ZCode-Usage-HUD-*-Setup.exe asset.
func setupAssetURL(rel *releaseInfo) string {
	for _, a := range rel.Assets {
		if strings.HasPrefix(a.Name, "ZCode-Usage-HUD-") && strings.HasSuffix(a.Name, "-Setup.exe") {
			return a.URL
		}
	}
	return ""
}

// messageBoxYesNo asks a Yes/No question; returns true on Yes.
func messageBoxYesNo(owner uintptr, title, text string) bool {
	const mbYesNo = 0x04
	const mbIconQ = 0x20
	const idYes = 6
	hwnd := owner
	if hwnd == 0 {
		hwnd = hwndMain
	}
	r, _, _ := procMessageBoxW.Call(hwnd,
		uintptr(unsafe.Pointer(wideString(title))),
		uintptr(unsafe.Pointer(wideString(text))),
		uintptr(mbYesNo|mbIconQ))
	return r == idYes
}

func wideString(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// checkForUpdatesInteractive runs the flow: fetch, compare, ask, install.
// Returns the outcome string for the tray/log diagnostics.
func checkForUpdatesInteractive(owner uintptr) string {
	rel, err := fetchLatestRelease()
	if err != nil {
		logDiagnostic("update check failed: %v", err)
		messageBox("Check for updates", "Could not check for updates:\n"+err.Error())
		return "error"
	}
	if !isNewerVersion(appVersion, rel.TagName) {
		messageBox("Check for updates", "You are running the latest version ("+appVersion+").")
		return "uptodate"
	}
	if !messageBoxYesNo(owner, "Update available",
		"Version "+rel.TagName+" is available (you have v"+appVersion+").\n\nDownload and install silently now?") {
		return "declined"
	}
	return installUpdate(rel)
}

// installUpdate downloads the setup asset and runs it silently.
func installUpdate(rel *releaseInfo) string {
	url := setupAssetURL(rel)
	if url == "" {
		messageBox("Update", "The release has no installer asset to download.")
		return "no-asset"
	}
	tmp, err := os.CreateTemp("", "zcode-hud-update-*.exe")
	if err != nil {
		messageBox("Update", "Could not prepare download: "+err.Error())
		return "error"
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath) // no-op if the updater already renamed/ran it

	if err := downloadToFile(url, tmpPath); err != nil {
		logDiagnostic("update download failed: %v", err)
		messageBox("Update", "Download failed: "+err.Error())
		return "error"
	}
	// Silent update: run the installer in replace mode, then exit.
	if err := execAsync(tmpPath, "/VERYSILENT", "/NORESTART", "/CLOSEAPPLICATIONS", "/RESTARTAPPLICATIONS"); err != nil {
		messageBox("Update", "Could not start the installer: "+err.Error())
		return "error"
	}
	gracefulExit()
	return "updating"
}

// downloadToFile streams url into path.
func downloadToFile(url, path string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download status %d", resp.StatusCode)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(resp.Body, 1<<30))
	return err
}

// execAsync starts a process without waiting.
func execAsync(path string, args ...string) error {
	cmd := filepath.Clean(path) + " " + strings.Join(args, " ")
	vc, err := syscall.UTF16PtrFromString(cmd)
	if err != nil {
		return err
	}
	const swHide = 0
	r, _, _ := procShellExecuteW.Call(0, 0, uintptr(unsafe.Pointer(vc)), 0, 0, uintptr(swHide))
	if r != 1 {
		return fmt.Errorf("ShellExecute failed")
	}
	return nil
}

// gracefulExit posts the silent exit path on the UI thread and gives the
// installer a moment to take over.
func gracefulExit() {
	if hwndMain != 0 {
		procPostMessageW.Call(hwndMain, WM_APP_EXIT, 0, 0)
	}
	time.Sleep(300 * time.Millisecond)
	os.Exit(0)
}
