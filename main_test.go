package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"strings"
	"testing"
	"time"
)

func mustMapper(t *testing.T, cfg config) *mapper {
	t.Helper()
	m, err := newMapper(cfg)
	if err != nil {
		t.Fatalf("newMapper(%+v): %v", cfg, err)
	}
	return m
}

func TestRunTransformsLines(t *testing.T) {
	m := mustMapper(t, config{
		expr: `^(?P<name>[A-Za-z]+)\s+(?P<surname>[A-Za-z]+)$`,
		tmpl: `{"name":"${name}","surname":"${surname}"}`,
	})

	input := "john doe\njane roe\nnot a match\n\nbob\nzoe zed"
	var out, log bytes.Buffer

	if err := run(context.Background(), m, strings.NewReader(input), &out, &log, false, false); err != nil {
		t.Fatalf("run: %v", err)
	}

	want := "" +
		`{"name":"john","surname":"doe"}` + "\n" +
		`{"name":"jane","surname":"roe"}` + "\n" +
		`{"name":"zoe","surname":"zed"}` + "\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if log.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", log.String())
	}
}

func TestRunVerboseReportsUnmatched(t *testing.T) {
	m := mustMapper(t, config{expr: `^(?P<id>\d+)$`, tmpl: "${id}"})

	var out, log bytes.Buffer
	if err := run(context.Background(), m, strings.NewReader("42\nnope\n"), &out, &log, true, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "42\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if !strings.Contains(log.String(), "no match: nope") {
		t.Fatalf("stderr = %q, want a no-match report", log.String())
	}
}

func TestRunDoesNotDoubleNewline(t *testing.T) {
	m := mustMapper(t, config{expr: `^(?P<x>\d+)$`, tmpl: "${x}\n"})

	var out bytes.Buffer
	if err := run(context.Background(), m, strings.NewReader("42\n"), &out, io.Discard, false, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "42\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRunNonParticipatingGroupIsEmpty(t *testing.T) {
	m := mustMapper(t, config{expr: `^(?P<a>x)(?P<b>y)?$`, tmpl: "[${a}][${b}]"})

	var out bytes.Buffer
	if err := run(context.Background(), m, strings.NewReader("x\n"), &out, io.Discard, false, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "[x][]\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	m := mustMapper(t, config{expr: `(?P<x>.)`, tmpl: "${x}"})

	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, m, pr, io.Discard, io.Discard, false, false)
	}()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not stop after context cancellation")
	}
}

func TestRunReadError(t *testing.T) {
	m := mustMapper(t, config{expr: `(?P<x>.)`, tmpl: "${x}"})

	err := run(context.Background(), m, &failingReader{}, io.Discard, io.Discard, false, false)
	if err == nil || !strings.Contains(err.Error(), "read stdin") {
		t.Fatalf("run error = %v, want a stdin read error", err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestAngleBracketNamedGroups(t *testing.T) {
	m := mustMapper(t, config{expr: `^(?<name>\w+)$`, tmpl: "${name}"})

	var out bytes.Buffer
	if err := run(context.Background(), m, strings.NewReader("hello\n"), &out, io.Discard, false, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "hello\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestNewMapperErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  config
	}{
		{"missing expr", config{tmpl: "${x}"}},
		{"missing tmpl", config{expr: `(?P<x>a)`}},
		{"invalid expr", config{expr: `(`, tmpl: "${x}"}},
		{"no named groups", config{expr: `(a)`, tmpl: "${x}"}},
		{"unknown template group", config{expr: `(?P<x>a)`, tmpl: "${nope}"}},
		{"unterminated template", config{expr: `(?P<x>a)`, tmpl: "${x"}},
		{"invalid template name", config{expr: `(?P<x>a)`, tmpl: "${}"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := newMapper(tt.cfg); err == nil {
				t.Fatalf("newMapper(%+v) succeeded, want error", tt.cfg)
			}
		})
	}
}

func TestParseFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	cfg, err := parseFlags(fs, []string{
		"-expr", `(?P<x>\d+)`,
		"-tmpl", "${x}",
		"-v",
	})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.expr != `(?P<x>\d+)` || cfg.tmpl != "${x}" || !cfg.verbose {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}
