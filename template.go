package main

import (
	"fmt"
	"regexp"
	"strings"
)

// segment is one piece of a compiled template: either literal text or the
// name of a capture group to substitute.
type segment struct {
	literal string
	group   string
}

// placeholderName matches the names accepted inside ${...}. Named groups in
// Go regular expressions use word characters, so this mirrors that rule.
var placeholderName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// parseTemplate splits a template into literal and placeholder segments.
//
// ${name} is a placeholder, $$ is an escaped dollar sign, and a $ that does
// not start one of those is treated as literal text.
func parseTemplate(tmpl string) ([]segment, error) {
	var segs []segment
	var lit strings.Builder

	flush := func() {
		if lit.Len() > 0 {
			segs = append(segs, segment{literal: lit.String()})
			lit.Reset()
		}
	}

	for i := 0; i < len(tmpl); {
		c := tmpl[i]
		if c != '$' || i+1 >= len(tmpl) {
			lit.WriteByte(c)
			i++
			continue
		}

		switch tmpl[i+1] {
		case '$':
			lit.WriteByte('$')
			i += 2
		case '{':
			end := strings.IndexByte(tmpl[i+2:], '}')
			if end < 0 {
				return nil, fmt.Errorf("unterminated placeholder starting at offset %d", i)
			}
			name := tmpl[i+2 : i+2+end]
			if !placeholderName.MatchString(name) {
				return nil, fmt.Errorf("invalid placeholder name %q at offset %d", name, i)
			}
			flush()
			segs = append(segs, segment{group: name})
			i += 2 + end + 1
		default:
			lit.WriteByte('$')
			i++
		}
	}
	flush()

	return segs, nil
}

// render expands the parsed template using the supplied values. Placeholders
// without a value (including groups that did not take part in the match)
// expand to the empty string.
func render(segs []segment, values map[string]string) string {
	var b strings.Builder
	for _, s := range segs {
		if s.group == "" {
			b.WriteString(s.literal)
		} else {
			b.WriteString(values[s.group])
		}
	}
	return b.String()
}
