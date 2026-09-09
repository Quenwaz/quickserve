package config

import (
	"bytes"
	"os"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    Options
		wantErr bool
	}{
		{"defaults", nil, opts(8000, "."), false},
		{"positional port", []string{"9999"}, opts(9999, "."), false},
		{"positional port and dir", []string{"9999", "./public"}, opts(9999, "./public"), false},
		{"positional dir only", []string{"./public"}, opts(8000, "./public"), false},
		{"flag port", []string{"-port", "9000"}, opts(9000, "."), false},
		{"flag port equals", []string{"--port=9000"}, opts(9000, "."), false},
		{"short port equals", []string{"-p=9000"}, opts(9000, "."), false},
		{"flag dir", []string{"-dir", "./pub"}, opts(8000, "./pub"), false},
		{"dir equals", []string{"--directory=pub"}, opts(8000, "pub"), false},
		{"upload flag", []string{"-u"}, withUpload(opts(8000, ".")), false},
		{"upload long", []string{"--upload"}, withUpload(opts(8000, ".")), false},
		{"cors flag", []string{"-c"}, withCORS(opts(8000, ".")), false},
		{"combined", []string{"-u", "-c", "-p", "9000", "-d", "pub"},
			withCORS(withUpload(opts(9000, "pub"))), false},
		{"token", []string{"-t", "s3cr3t"}, withToken(opts(8000, "."), "s3cr3t"), false},
		{"token equals", []string{"--token=abc"}, withToken(opts(8000, "."), "abc"), false},
		{"max upload", []string{"-m", "100MB"}, withMax(opts(8000, "."), 100<<20), false},
		{"max upload equals", []string{"--max-upload=512K"}, withMax(opts(8000, "."), 512<<10), false},
		{"max upload zero", []string{"-m", "0"}, withMax(opts(8000, "."), 0), false},
		{"max upload plain bytes", []string{"-m", "1024"}, withMax(opts(8000, "."), 1024), false},
		{"rate limit", []string{"-r", "30"}, withRate(opts(8000, "."), 30), false},
		{"rate limit zero", []string{"-r", "0"}, withRate(opts(8000, "."), 0), false},
		{"read only", []string{"-ro"}, withReadOnly(opts(8000, ".")), false},
		{"base path", []string{"-b", "/apps"}, withBase(opts(8000, "."), "/apps"), false},
		{"base path equals", []string{"--base-path=/apps/"}, withBase(opts(8000, "."), "/apps"), false},
		{"base path no slash", []string{"-b=apps"}, withBase(opts(8000, "."), "/apps"), false},
		{"base path root forms", []string{"-b=/"}, withBase(opts(8000, "."), ""), false},
		{"base path empty", []string{"-b="}, withBase(opts(8000, "."), ""), false},
		{"base path traversal", []string{"-b=/../etc"}, Options{}, true},
		{"base path dot", []string{"-b=/."}, Options{}, true},
		{"base path whitespace", []string{"-b=/my apps"}, Options{}, true},
		{"base path missing value", []string{"-b"}, Options{}, true},
		{"help", []string{"-h"}, opts(8000, "."), true},
		{"help long", []string{"--help"}, opts(8000, "."), true},
		{"version", []string{"-v"}, opts(8000, "."), true},
		{"invalid port value", []string{"-port", "abc"}, Options{}, true},
		{"port out of range", []string{"-port", "70000"}, Options{}, true},
		{"negative rate", []string{"-r", "-5"}, Options{}, true},
		{"bad size", []string{"-m", "abc"}, Options{}, true},
		{"missing port value", []string{"-p"}, Options{}, true},
		{"missing dir value", []string{"-d"}, Options{}, true},
		{"missing token value", []string{"-t"}, Options{}, true},
		{"unknown option", []string{"--nope"}, Options{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseArgs(%v) expected error, got nil", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseArgs(%v) unexpected error: %v", tt.args, err)
			}
			if !sameOptions(got, tt.want) {
				t.Errorf("ParseArgs(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

// opts / withX 构造期望值；默认 maxSize/ratePM 与生产默认保持一致。
func opts(port int, dir string) Options {
	o := NewOptions()
	o.port, o.dir = port, dir
	return o
}
func withUpload(o Options) Options          { o.upload = true; return o }
func withCORS(o Options) Options            { o.cors = true; return o }
func withReadOnly(o Options) Options        { o.readonly = true; return o }
func withToken(o Options, s string) Options { o.token = s; return o }
func withMax(o Options, n int64) Options    { o.maxSize = n; return o }
func withRate(o Options, n int) Options     { o.ratePM = n; return o }
func withBase(o Options, p string) Options  { o.basePath = p; return o }

// sameOptions 逐字段比较（Options 含未导出字段，不能直接 == 之外还依赖默认值一致性）。
func sameOptions(a, b Options) bool {
	return a.port == b.port && a.dir == b.dir && a.upload == b.upload &&
		a.cors == b.cors && a.token == b.token && a.maxSize == b.maxSize &&
		a.ratePM == b.ratePM && a.readonly == b.readonly && a.basePath == b.basePath
}

func TestEnvVars(t *testing.T) {
	t.Run("all vars", func(t *testing.T) {
		setenvs(t, map[string]string{
			"QUICKSERVE_PORT":       "9100",
			"QUICKSERVE_DIR":        "/srv/pub",
			"QUICKSERVE_UPLOAD":     "1",
			"QUICKSERVE_CORS":       "true",
			"QUICKSERVE_TOKEN":      "env-secret",
			"QUICKSERVE_MAX_UPLOAD": "50MB",
			"QUICKSERVE_RATE_LIMIT": "10",
			"QUICKSERVE_READ_ONLY":  "no",
			"QUICKSERVE_BASE_PATH":  "apps/",
		})
		got, err := ParseArgs(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := withBase(withRate(withMax(withToken(withCORS(withUpload(opts(9100, "/srv/pub"))), "env-secret"), 50<<20), 10), "/apps")
		if !sameOptions(got, want) {
			t.Errorf("env opts = %+v, want %+v", got, want)
		}
	})
	t.Run("flags override env", func(t *testing.T) {
		setenvs(t, map[string]string{
			"QUICKSERVE_PORT":      "9100",
			"QUICKSERVE_TOKEN":     "env-secret",
			"QUICKSERVE_BASE_PATH": "/from-env",
		})
		got, err := ParseArgs([]string{"-p", "9200", "-b", "/from-flag"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Port() != 9200 || got.BasePath() != "/from-flag" {
			t.Errorf("flags did not override env: port=%d base=%q", got.Port(), got.BasePath())
		}
		if got.Token() != "env-secret" {
			t.Errorf("env token lost: %q", got.Token())
		}
	})
	t.Run("invalid env", func(t *testing.T) {
		setenvs(t, map[string]string{"QUICKSERVE_PORT": "abc"})
		if _, err := ParseArgs(nil); err == nil {
			t.Error("invalid QUICKSERVE_PORT should fail")
		}
		setenvs(t, map[string]string{"QUICKSERVE_UPLOAD": "maybe"})
		if _, err := ParseArgs(nil); err == nil {
			t.Error("invalid QUICKSERVE_UPLOAD should fail")
		}
		setenvs(t, map[string]string{"QUICKSERVE_BASE_PATH": "/../x"})
		if _, err := ParseArgs(nil); err == nil {
			t.Error("traversal QUICKSERVE_BASE_PATH should fail")
		}
	})
}

// setenvs 设置环境变量并在测试结束后恢复。
func setenvs(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		old, had := os.LookupEnv(k)
		os.Setenv(k, v)
		t.Cleanup(func() {
			if had {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		})
	}
}

func TestNormalizeBasePath(t *testing.T) {
	tests := []struct {
		in    string
		want  string
		isErr bool
	}{
		{"", "", false},
		{"/", "", false},
		{"apps", "/apps", false},
		{"/apps", "/apps", false},
		{"/apps/", "/apps", false},
		{"/a/b", "/a/b", false},
		{"/a//b", "/a/b", false},
		{"/..", "", true},
		{"/a/../b", "", true},
		{"/.", "", true},
		{"/my apps", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeBasePath(tt.in)
		if tt.isErr {
			if err == nil {
				t.Errorf("NormalizeBasePath(%q) expected error, got %q", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeBasePath(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("NormalizeBasePath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPrintUsageWrites(t *testing.T) {
	var buf bytes.Buffer
	PrintUsage(&buf)
	if buf.Len() == 0 {
		t.Fatal("PrintUsage wrote nothing")
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		err  bool
	}{
		{"100", 100, false},
		{"1KB", 1 << 10, false},
		{"512K", 512 << 10, false},
		{"100MB", 100 << 20, false},
		{"2G", 2 << 30, false},
		{"1.5MB", 1572864, false},
		{"0", 0, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-5MB", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseSize(tt.in)
		if tt.err {
			if err == nil {
				t.Errorf("ParseSize(%q) expected error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSize(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseSize(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
