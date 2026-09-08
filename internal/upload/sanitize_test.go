package upload

import (
	"errors"
	"strings"
	"testing"
)

func TestSanitizeRelPath(t *testing.T) {
	tests := []struct {
		in    string
		want  string
		isErr bool
	}{
		{"/a/b/c.txt", "a/b/c.txt", false},
		{"/single.txt", "single.txt", false},
		{"", "", true},
		{"/", "", true},
		{"/../x", "", true},                   // 穿越
		{"/a/../../x", "", true},              // 穿越
		{"/CON", "", true},                    // Windows 保留名
		{"/CON.txt", "", true},                // 带扩展名的保留名
		{"/a/CON/x", "", true},                // 中间段保留名
		{"/a//b///c.txt", "a/b/c.txt", false}, // 空段折叠
		{"/trailing./x ", "trailing/x", false},
	}
	for _, tt := range tests {
		got, err := sanitizeRelPath(tt.in)
		if tt.isErr {
			if err == nil {
				t.Errorf("sanitizeRelPath(%q) expected error, got %q", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("sanitizeRelPath(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("sanitizeRelPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSanitizeRelPathRejectsDotDot(t *testing.T) {
	_, err := sanitizeRelPath("/a/../b")
	if !errors.Is(err, errBadSegment) {
		t.Errorf("err = %v, want errBadSegment", err)
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"photo.jpg", "photo.jpg"},
		{`C:\Users\evp\evil.exe`, "evil.exe"},
		{"/etc/passwd", "passwd"},
		{"../../evil", "evil"},
		{"", ""},
		{"...", ""}, // 全部为点 → 清洗后为空
		{"CON", ""}, // 保留名 → 拒绝
		{"a<b>.txt", "a_b_.txt"},
		{"设置.ini", "设置.ini"},
		{"weird\tname.png", "weird_name.png"}, // 内部控制字符替换为 _
	}
	for _, tt := range tests {
		got := sanitizeFilename(tt.in)
		if got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSanitizeSegmentLongName(t *testing.T) {
	long := strings.Repeat("字", 300)
	got := sanitizeFilename(long)
	if n := len([]rune(got)); n > maxSegmentBytes {
		t.Errorf("rune count = %d, want <= %d", n, maxSegmentBytes)
	}
	// 截断后不应以点结尾（Windows 歧义）。
	if strings.HasSuffix(got, ".") {
		t.Error("truncated name ends with dot")
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("héllo", 3); got != "hél" {
		t.Errorf("truncateRunes = %q", got)
	}
	if got := truncateRunes("ab", 5); got != "ab" {
		t.Errorf("truncateRunes = %q", got)
	}
	if got := truncateRunes("ab", 0); got != "" {
		t.Errorf("truncateRunes = %q", got)
	}
}
