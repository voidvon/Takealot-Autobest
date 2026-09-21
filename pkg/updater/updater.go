package updater

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	repoOwner       string
	repoName        string
	currentVersion  string
	onBeforeRestart func()

	mu            sync.RWMutex
	progress      Progress
	lastCheck     *ReleaseInfo
	lastCheckTime time.Time
	isUpdating    bool
}

func NewManager(repoOwner, repoName, currentVersion string, onBeforeRestart func()) *Manager {
	if repoOwner == "" {
		repoOwner = "voidvon"
	}
	if repoName == "" {
		repoName = "Takealot-Autobest"
	}
	return &Manager{
		repoOwner:       repoOwner,
		repoName:        repoName,
		currentVersion:  strings.TrimSpace(currentVersion),
		onBeforeRestart: onBeforeRestart,
		progress: Progress{
			Status: "idle",
		},
	}
}

type ghReleaseResponse struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Assets      []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckForUpdate queries GitHub for the latest release
func (m *Manager) CheckForUpdate(force bool) (*ReleaseInfo, error) {
	m.mu.Lock()
	if !force && m.lastCheck != nil && time.Since(m.lastCheckTime) < 60*time.Second {
		cached := m.lastCheck
		m.mu.Unlock()
		return cached, nil
	}
	m.mu.Unlock()

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", m.repoOwner, m.repoName)
	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", fmt.Sprintf("Takealot-Autobest-Updater/%s", m.currentVersion))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 GitHub 检查更新失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("GitHub API 请求频次受限，请稍后再试或访问 Releases 页面")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回错误状态码: %d", resp.StatusCode)
	}

	var ghRel ghReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&ghRel); err != nil {
		return nil, fmt.Errorf("解析 GitHub 版本数据失败: %w", err)
	}

	latestVer := strings.TrimPrefix(strings.TrimSpace(ghRel.TagName), "v")
	currentVer := strings.TrimPrefix(strings.TrimSpace(m.currentVersion), "v")
	hasUpdate := CompareVersions(latestVer, currentVer) > 0

	// 查找匹配当前平台架构的资产
	matchedAsset := m.matchAsset(ghRel.Assets)

	info := &ReleaseInfo{
		Version:      latestVer,
		TagName:      ghRel.TagName,
		Title:        ghRel.Name,
		ReleaseNotes: ghRel.Body,
		PublishedAt:  ghRel.PublishedAt.Format("2006-01-02 15:04:05"),
		HTMLURL:      ghRel.HTMLURL,
		HasUpdate:    hasUpdate,
		CurrentVer:   m.currentVersion,
	}

	if matchedAsset != nil {
		info.AssetName = matchedAsset.Name
		info.AssetSize = matchedAsset.Size
		info.AssetURL = matchedAsset.BrowserDownloadURL
	}

	m.mu.Lock()
	m.lastCheck = info
	m.lastCheckTime = time.Now()
	m.mu.Unlock()

	return info, nil
}

// matchAsset selects the best release asset based on current OS and Arch
func (m *Manager) matchAsset(assets []ghAsset) *ghAsset {
	osName := runtime.GOOS     // darwin, windows, linux
	archName := runtime.GOARCH // arm64, amd64

	var candidates []ghAsset
	for _, a := range assets {
		lowerName := strings.ToLower(a.Name)
		if !strings.HasSuffix(lowerName, ".zip") {
			continue
		}
		switch osName {
		case "darwin":
			if strings.Contains(lowerName, "macos") || strings.Contains(lowerName, "darwin") {
				candidates = append(candidates, a)
			}
		case "windows":
			if strings.Contains(lowerName, "windows") || strings.Contains(lowerName, "win") {
				candidates = append(candidates, a)
			}
		case "linux":
			if strings.Contains(lowerName, "linux") {
				candidates = append(candidates, a)
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// 优先匹配具体架构
	for _, c := range candidates {
		lower := strings.ToLower(c.Name)
		if strings.Contains(lower, archName) {
			return &c
		}
		if archName == "amd64" && (strings.Contains(lower, "x86_64") || strings.Contains(lower, "x64")) {
			return &c
		}
		if archName == "arm64" && strings.Contains(lower, "aarch64") {
			return &c
		}
	}

	// 兜底返回第一个同平台的资产包
	return &candidates[0]
}

// CompareVersions compares two semver strings: returns 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func CompareVersions(v1, v2 string) int {
	c1 := strings.TrimPrefix(strings.TrimSpace(v1), "v")
	c2 := strings.TrimPrefix(strings.TrimSpace(v2), "v")

	parts1 := strings.Split(strings.Split(c1, "-")[0], ".")
	parts2 := strings.Split(strings.Split(c2, "-")[0], ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}
		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}
	return 0
}

func (m *Manager) GetProgress() Progress {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.progress
}

func (m *Manager) setProgress(status string, percent float64, current, total int64, speed, msg, errStr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress = Progress{
		Status:     status,
		Percent:    percent,
		Downloaded: current,
		Total:      total,
		Speed:      speed,
		Message:    msg,
		Error:      errStr,
	}
}

// StartApply begins downloading the update and executing replacement
func (m *Manager) StartApply(assetURL string, useProxy bool) error {
	m.mu.Lock()
	if m.isUpdating {
		m.mu.Unlock()
		return fmt.Errorf("更新任务正在进行中，请勿重复触发")
	}
	m.isUpdating = true
	m.mu.Unlock()

	go func() {
		defer func() {
			m.mu.Lock()
			m.isUpdating = false
			m.mu.Unlock()
		}()
		m.runUpdate(assetURL, useProxy)
	}()

	return nil
}

func (m *Manager) runUpdate(assetURL string, useProxy bool) {
	if assetURL == "" {
		info, err := m.CheckForUpdate(true)
		if err != nil {
			m.setProgress("error", 0, 0, 0, "", "获取更新文件地址失败", err.Error())
			return
		}
		assetURL = info.AssetURL
	}

	if assetURL == "" {
		m.setProgress("error", 0, 0, 0, "", "未找到适配当前平台的安装包", "缺少对应平台的资产文件")
		return
	}

	finalURL := assetURL
	if useProxy {
		// 使用开源加速代理镜像
		finalURL = "https://ghproxy.net/" + assetURL
	}

	log.Printf("[Updater] 开始下载更新: %s (代理: %v)", finalURL, useProxy)
	m.setProgress("downloading", 0, 0, 0, "", "正在连接下载服务器...", "")

	// 1. 创建临时 zip 文件
	tmpFile, err := os.CreateTemp("", "takealot_update_*.zip")
	if err != nil {
		m.setProgress("error", 0, 0, 0, "", "创建临时文件失败", err.Error())
		return
	}
	tmpFilePath := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFilePath)
	}()

	// 2. 发起下载
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequest("GET", finalURL, nil)
	if err != nil {
		m.setProgress("error", 0, 0, 0, "", "创建下载请求失败", err.Error())
		return
	}
	req.Header.Set("User-Agent", fmt.Sprintf("Takealot-Autobest-Updater/%s", m.currentVersion))

	resp, err := client.Do(req)
	if err != nil {
		// 如果代理失败且使用了代理，尝试直接下载
		if useProxy {
			log.Printf("[Updater] 代理下载失败，尝试官方直连: %v", err)
			m.setProgress("downloading", 0, 0, 0, "", "加速通道失败，正在切换官方直连...", "")
			finalURL = assetURL
			req, _ = http.NewRequest("GET", finalURL, nil)
			req.Header.Set("User-Agent", fmt.Sprintf("Takealot-Autobest-Updater/%s", m.currentVersion))
			resp, err = client.Do(req)
		}
	}
	if err != nil {
		m.setProgress("error", 0, 0, 0, "", "下载更新包失败", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		m.setProgress("error", 0, 0, 0, "", fmt.Sprintf("服务器返回异常状态码: %d", resp.StatusCode), "")
		return
	}

	totalBytes := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 64*1024)
	startTime := time.Now()
	lastReport := time.Now()

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := tmpFile.Write(buf[:n]); writeErr != nil {
				m.setProgress("error", 0, 0, 0, "", "写入临时文件失败", writeErr.Error())
				return
			}
			downloaded += int64(n)
		}

		now := time.Now()
		if now.Sub(lastReport) >= 150*time.Millisecond || readErr != nil {
			lastReport = now
			var percent float64
			if totalBytes > 0 {
				percent = float64(downloaded) / float64(totalBytes) * 100.0
			}
			elapsed := now.Sub(startTime).Seconds()
			speedStr := ""
			if elapsed > 0 {
				bytesPerSec := float64(downloaded) / elapsed
				if bytesPerSec > 1024*1024 {
					speedStr = fmt.Sprintf("%.2f MB/s", bytesPerSec/(1024*1024))
				} else {
					speedStr = fmt.Sprintf("%.1f KB/s", bytesPerSec/1024)
				}
			}
			msg := fmt.Sprintf("正在下载更新包 (%.1f%%)", percent)
			m.setProgress("downloading", percent, downloaded, totalBytes, speedStr, msg, "")
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			m.setProgress("error", 0, 0, 0, "", "下载流中断", readErr.Error())
			return
		}
	}
	_ = tmpFile.Close()

	// 3. 解压缩更新包
	m.setProgress("extracting", 100, downloaded, totalBytes, "", "正在解压并校验更新包...", "")
	unpackDir, err := os.MkdirTemp("", "takealot_unpack_*")
	if err != nil {
		m.setProgress("error", 0, 0, 0, "", "创建解压临时目录失败", err.Error())
		return
	}

	if err := unzip(tmpFilePath, unpackDir); err != nil {
		_ = os.RemoveAll(unpackDir)
		m.setProgress("error", 0, 0, 0, "", "解压更新包失败", err.Error())
		return
	}

	// 4. 准备就绪，执行重启与替换
	m.setProgress("ready", 100, downloaded, totalBytes, "", "更新就绪，软件即将自动重启并加载新版...", "")
	time.Sleep(1200 * time.Millisecond)

	m.setProgress("restarting", 100, downloaded, totalBytes, "", "正在重启应用...", "")
	if err := m.applyAndRestart(unpackDir); err != nil {
		_ = os.RemoveAll(unpackDir)
		m.setProgress("error", 0, 0, 0, "", "自动替换并重启失败", err.Error())
	}
}

// unzip unpacks a zip archive into destDir preserving permissions
func unzip(srcZip, destDir string) error {
	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// Prevent Zip Slip vulnerability
		filePath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(filePath), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("非法压缩文件路径: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}

		mode := f.Mode()
		outFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}

		// Ensure execution bit for binary executables
		if strings.Contains(filePath, "Contents/MacOS/") || strings.HasSuffix(filePath, ".exe") || (mode&0111 != 0) {
			_ = os.Chmod(filePath, 0755)
		}
	}
	return nil
}

// Cleanup removes leftover temporary updater or backup files
func Cleanup() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	dir := filepath.Dir(exePath)
	oldExe := filepath.Join(dir, filepath.Base(exePath)+".old")
	_ = os.Remove(oldExe)
}
