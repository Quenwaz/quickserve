package main

import (
	"bytes"
	"errors"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    options
		wantErr error
	}{
		{"defaults", nil, options{port: 8000, dir: "."}, nil},
		{"positional port", []string{"9999"}, options{port: 9999, dir: "."}, nil},
		{"positional port and dir", []string{"9999", "./public"}, options{port: 9999, dir: "./public"}, nil},
		{"positional dir only", []string{"./public"}, options{port: 8000, dir: "./public"}, nil},
		{"flag port", []string{"-port", "9000"}, options{port: 9000, dir: "."}, nil},
		{"flag port equals", []string{"--port=9000"}, options{port: 9000, dir: "."}, nil},
		{"short port equals", []string{"-p=9000"}, options{port: 9000, dir: "."}, nil},
		{"flag dir", []string{"-dir", "./pub"}, options{port: 8000, dir: "./pub"}, nil},
		{"dir equals", []string{"--directory=pub"}, options{port: 8000, dir: "pub"}, nil},
		{"upload flag", []string{"-u"}, options{port: 8000, dir: ".", upload: true}, nil},
		{"upload long", []string{"--upload"}, options{port: 8000, dir: ".", upload: true}, nil},
		{"cors flag", []string{"-c"}, options{port: 8000, dir: ".", cors: true}, nil},
		{"combined", []string{"-u", "-c", "-p", "9000", "-d", "pub"},
			options{port: 9000, dir: "pub", upload: true, cors: true}, nil},
		{"help", []string{"-h"}, options{port: 8000, dir: "."}, errHelp},
		{"help long", []string{"--help"}, options{port: 8000, dir: "."}, errHelp},
		{"version", []string{"-v"}, options{port: 8000, dir: "."}, errVersion},
		{"invalid port value", []string{"-port", "abc"}, options{}, errors.New("anything")},
		{"missing port value", []string{"-p"}, options{}, errors.New("anything")},
		{"missing dir value", []string{"-d"}, options{}, errors.New("anything")},
		{"unknown option", []string{"--nope"}, options{}, errors.New("anything")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("parseArgs(%v) expected error, got nil", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs(%v) unexpected error: %v", tt.args, err)
			}
			if got != tt.want {
				t.Errorf("parseArgs(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestPrintUsageWrites(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)
	if buf.Len() == 0 {
		t.Fatal("printUsage wrote nothing")
	}
}
