package updater

import (
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"v0.1.7", "0.1.7", 0},
		{"0.2.0", "0.1.7", 1},
		{"0.1.7", "0.2.0", -1},
		{"v0.2.1", "v0.2.0", 1},
		{"1.0.0", "0.20.0", 1},
		{"0.1.20", "0.1.19", 1},
		{"0.1.10", "0.1.2", 1},
		{"0.2.0-beta", "0.2.0", 0},
	}

	for _, tt := range tests {
		got := CompareVersions(tt.v1, tt.v2)
		if got != tt.expected {
			t.Errorf("CompareVersions(%q, %q) = %d; want %d", tt.v1, tt.v2, got, tt.expected)
		}
	}
}

func TestMatchAsset(t *testing.T) {
	m := NewManager("voidvon", "Takealot-Autobest", "0.1.7", nil)
	assets := []ghAsset{
		{Name: "Takealot-v0.1.7-linux-amd64.zip", BrowserDownloadURL: "https://example.com/linux.zip"},
		{Name: "Takealot-v0.1.7-macOS-arm64.zip", BrowserDownloadURL: "https://example.com/mac.zip"},
		{Name: "Takealot-v0.1.7-windows-amd64.zip", BrowserDownloadURL: "https://example.com/win.zip"},
	}

	matched := m.matchAsset(assets)
	if matched == nil {
		t.Fatalf("matchAsset returned nil")
	}
	t.Logf("Matched asset for current OS/Arch: %s", matched.Name)
}
