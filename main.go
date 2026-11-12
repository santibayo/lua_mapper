// Command lua-mapper maps lines from standard input to a template using
// named capture groups from a regular expression.
//
// For every line read from stdin it applies -expr, extracts the named
// groups and renders -tmpl replacing ${name} with the captured text. It
// keeps going until stdin reaches EOF or the process receives SIGINT
// (usually Ctrl+C) or SIGTERM.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
)

const usageText = `Usage: lua-mapper -expr REGEX -tmpl TEMPLATE

Reads lines from standard input, extracts named capture groups from each
line with REGEX, and writes TEMPLATE with every ${name} replaced by the
captured value. Processing stops at EOF or on Ctrl+C / SIGTERM.

Named groups use Go regular expression syntax, for example:
  (?P<name>...)   or   (?<name>...)

A group that did not take part in the match renders as an empty string.
Use $$ for a literal dollar sign. Lines that do not match REGEX are
skipped (use -v to report them on stderr).

Flags:
`

type config struct {
	expr    string
	tmpl    string
	verbose bool
}

// capture links a named group to its index in a match slice.
type capture struct {
	name  string
	index int
}

type mapper struct {
	re   *regexp.Regexp
	caps []capture
	segs []segment
}

func newMapper(cfg config) (*mapper, error) {
	if cfg.expr == "" {
		return nil, errors.New("-expr is required")
	}
	if cfg.tmpl == "" {
		return nil, errors.New("-tmpl is required")
	}

	re, err := regexp.Compile(cfg.expr)
	if err != nil {
		return nil, fmt.Errorf("invalid -expr: %w", err)
	}

	var caps []capture
	known := make(map[string]bool)
	for i, name := range re.SubexpNames() {
		if name == "" {
			continue
		}
		caps = append(caps, capture{name: name, index: i})
		known[name] = true
	}
	if len(caps) == 0 {
		return nil, errors.New("-expr must contain at least one named capture group, e.g. (?P<name>...)")
	}

	segs, err := parseTemplate(cfg.tmpl)
	if err != nil {
		return nil, fmt.Errorf("invalid -tmpl: %w", err)
	}
	for _, seg := range segs {
		if seg.group != "" && !known[seg.group] {
			names := make([]string, 0, len(caps))
			for _, c := range caps {
				names = append(names, c.name)
			}
			return nil, fmt.Errorf("invalid -tmpl: unknown capture group %q (available: %s)",
				seg.group, strings.Join(names, ", "))
		}
	}

	return &mapper{re: re, caps: caps, segs: segs}, nil
}

// run reads lines from in until EOF or ctx cancellation, writing one rendered
// result per matching line to out. When flushEach is true, buffered output is
// flushed after every line so interactive terminals see results immediately.
func run(ctx context.Context, m *mapper, in io.Reader, out io.Writer, errOut io.Writer, verbose, flushEach bool) error {
	lines := make(chan string)
	errs := make(chan error, 1)
	go readLines(ctx, in, lines, errs)

	values := make(map[string]string, len(m.caps))

	for {
		select {
		case <-ctx.Done():
			return flush(out)
		case line := <-lines:
			match := m.re.FindStringSubmatch(line)
			if match == nil {
				if verbose {
					fmt.Fprintf(errOut, "no match: %s\n", line)
				}
				continue
			}

			clear(values)
			for _, c := range m.caps {
				values[c.name] = match[c.index]
			}

			result := render(m.segs, values)
			if _, err := io.WriteString(out, result); err != nil {
				return fmt.Errorf("write stdout: %w", err)
			}
			if !strings.HasSuffix(result, "\n") {
				if _, err := io.WriteString(out, "\n"); err != nil {
					return fmt.Errorf("write stdout: %w", err)
				}
			}
			if flushEach {
				if err := flush(out); err != nil {
					return fmt.Errorf("flush stdout: %w", err)
				}
			}
		case err := <-errs:
			if errors.Is(err, io.EOF) {
				return flush(out)
			}
			return fmt.Errorf("read stdin: %w", err)
		}
	}
}

// readLines turns an arbitrarily long-lived stream into a channel of lines
// without the trailing newline. ReadString is used instead of Scanner so
// lines are not limited to 64 KiB.
func readLines(ctx context.Context, r io.Reader, lines chan<- string, errs chan<- error) {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			select {
			case lines <- line:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			errs <- err
			return
		}
	}
}

type flusher interface {
	Flush() error
}

func flush(w io.Writer) error {
	if f, ok := w.(flusher); ok {
		return f.Flush()
	}
	return nil
}

func parseFlags(fs *flag.FlagSet, args []string) (config, error) {
	var cfg config
	fs.StringVar(&cfg.expr, "expr", "", "regular expression with named capture groups (required)")
	fs.StringVar(&cfg.tmpl, "tmpl", "", "output template with ${name} placeholders (required)")
	fs.BoolVar(&cfg.verbose, "v", false, "report lines that do not match on stderr")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() > 0 {
		return cfg, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return cfg, nil
}

func printUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprint(w, usageText)
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func main() {
	fs := flag.NewFlagSet("lua-mapper", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	cfg, err := parseFlags(fs, os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(os.Stdout, fs)
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "lua-mapper: %v\n\n", err)
		printUsage(os.Stderr, fs)
		os.Exit(2)
	}

	m, err := newMapper(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lua-mapper: %v\n", err)
		os.Exit(2)
	}

	// Report broken pipes as write errors instead of dying from SIGPIPE, so
	// pipelines such as `lua-mapper ... | head -n 1` exit quietly.
	signal.Ignore(syscall.SIGPIPE)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	out := bufio.NewWriterSize(os.Stdout, 64*1024)
	err = run(ctx, m, os.Stdin, out, os.Stderr, cfg.verbose, isTerminal(os.Stdout))
	if err == nil {
		err = out.Flush()
	}
	if err != nil {
		if errors.Is(err, syscall.EPIPE) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "lua-mapper: %v\n", err)
		os.Exit(1)
	}
}
