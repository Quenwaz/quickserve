package upload

import (
	"errors"
	"path"
	"strings"
	"unicode/utf8"
)

// maxSegmentBytes 限制单个路径段的字节长度，避免超长文件名在
// Windows（MAX_PATH=260）或极端目录深度下创建失败。留出余量取 200。
const maxSegmentBytes = 200

// errBadSegment 表示路径段被安全策略拒绝（而非清洗后可用）。
var errBadSegment = errors.New("path segment rejected")

// windows 保留设备名（不区分大小写，带任意扩展名同样保留）。
var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// sanitizeRelPath 把 URL 路径（已解码）清洗为相对 root 的安全相对路径。
// 规则：
//   - 拒绝包含 ".." 的段（403 语义，由调用方转换）；
//   - 拒绝 Windows 保留设备名；
//   - 替换 Windows 非法字符与控制字符为 "_"；
//   - 去掉段尾的点与空格（Windows 会静默丢弃，造成访问歧义）；
//   - 每段按 UTF-8 边界截断到 maxSegmentBytes。
//
// 返回 "/" 分隔的相对路径（已 Clean），空段被折叠。
func sanitizeRelPath(urlPath string) (string, error) {
	var parts []string
	for _, seg := range strings.Split(urlPath, "/") {
		if seg == "" {
			continue
		}
		if seg == ".." {
			return "", errBadSegment
		}
		clean, err := sanitizeSegment(seg)
		if err != nil {
			return "", err
		}
		if clean != "" {
			parts = append(parts, clean)
		}
	}
	if len(parts) == 0 {
		return "", errBadSegment
	}
	return path.Join(parts...), nil
}

// sanitizeFilename 清洗 multipart 文件名。浏览器通常只发基础名，
// 但旧版 IE 会带完整路径，这里统一取最后一段（兼容 / 和 \），再按
// sanitizeSegment 清洗。返回空串表示无有效文件名。
func sanitizeFilename(name string) string {
	// 统一两种分隔符后取最后一段，防止 "C:\path\evil" 或 "/etc/passwd"。
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	clean, err := sanitizeSegment(name)
	if err != nil {
		return ""
	}
	return clean
}

// sanitizeSegment 清洗单个路径段。拒绝保留名；控制字符、Windows 非法
// 字符替换为 "_"；尾部点/空格去除；UTF-8 安全截断。
func sanitizeSegment(seg string) (string, error) {
	seg = strings.TrimFunc(seg, func(r rune) bool {
		return r <= 0x1F || r == 0x7F // 控制字符直接去除
	})
	seg = strings.Map(func(r rune) rune {
		switch {
		case r < 0x20 || r == 0x7F:
			return '_'
		case strings.ContainsRune(`<>:"|?*`, r):
			return '_'
		}
		return r
	}, seg)
	// Windows 会丢弃尾部点与空格，提前去除避免 "file." 与 "file" 互相覆盖。
	seg = strings.TrimRight(seg, ". ")
	if seg == "" {
		return "", nil
	}
	// 保留设备名（含 "CON.txt" 形式）一律拒绝，避免 Windows 设备对象。
	if reservedNames[strings.ToUpper(seg)] {
		return "", errBadSegment
	}
	if i := strings.IndexByte(seg, '.'); i > 0 && reservedNames[strings.ToUpper(seg[:i])] {
		return "", errBadSegment
	}
	if utf8.RuneCountInString(seg) > maxSegmentBytes {
		seg = truncateRunes(seg, maxSegmentBytes)
		// 截断后可能以点结尾，再次清理。
		seg = strings.TrimRight(seg, ". ")
		if seg == "" {
			return "", nil
		}
	}
	return seg, nil
}

// truncateRunes 按 rune 边界截断字符串，不产生半个 UTF-8 序列。
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}
