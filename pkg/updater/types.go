package updater

// ReleaseInfo contains information about a release retrieved from GitHub
type ReleaseInfo struct {
	Version      string `json:"version"`       // e.g. "0.2.1"
	TagName      string `json:"tag_name"`      // e.g. "v0.2.1"
	Title        string `json:"title"`         // e.g. "v0.2.1 发布"
	ReleaseNotes string `json:"release_notes"` // markdown notes
	PublishedAt  string `json:"published_at"`  // ISO timestamp or date
	AssetURL     string `json:"asset_url"`     // Download URL
	AssetName    string `json:"asset_name"`    // Asset file name
	AssetSize    int64  `json:"asset_size"`    // File size in bytes
	HTMLURL      string `json:"html_url"`      // Release page on GitHub
	HasUpdate    bool   `json:"has_update"`    // True if newer than current version
	CurrentVer   string `json:"current_version"`
}

// Progress tracks the live download and installation state
type Progress struct {
	Status     string  `json:"status"`               // "idle", "checking", "downloading", "extracting", "ready", "restarting", "error"
	Percent    float64 `json:"percent"`              // 0.0 to 100.0
	Downloaded int64   `json:"downloaded"`           // Bytes downloaded so far
	Total      int64   `json:"total"`                // Total bytes
	Speed      string  `json:"speed"`                // e.g. "2.4 MB/s"
	Message    string  `json:"message"`              // Human-readable status message
	Error      string  `json:"error,omitempty"`      // Error message if any
}

// ApplyRequest specifies options for applying an update
type ApplyRequest struct {
	Proxy       bool   `json:"proxy"`                  // Use download proxy mirror
	DownloadURL string `json:"download_url,omitempty"` // Override download URL
}
