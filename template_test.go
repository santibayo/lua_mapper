package main

import "testing"

func TestParseTemplateAndRender(t *testing.T) {
	tests := []struct {
		name   string
		tmpl   string
		values map[string]string
		want   string
	}{
		{
			name:   "basic",
			tmpl:   `{"name":"${name}","surname":"${surname}"}`,
			values: map[string]string{"name": "Ada", "surname": "Lovelace"},
			want:   `{"name":"Ada","surname":"Lovelace"}`,
		},
		{
			name:   "missing value renders empty",
			tmpl:   "${a}-${b}",
			values: map[string]string{"a": "x"},
			want:   "x-",
		},
		{
			name:   "repeated placeholder",
			tmpl:   "${a}${a}",
			values: map[string]string{"a": "x"},
			want:   "xx",
		},
		{
			name:   "escaped dollar",
			tmpl:   "$${a} costs $$5",
			values: map[string]string{"a": "it"},
			want:   "${a} costs $5",
		},
		{
			name:   "lone dollars stay literal",
			tmpl:   "a $ b $",
			values: map[string]string{},
			want:   "a $ b $",
		},
		{
			name:   "trailing newline literal",
			tmpl:   "${a}\n",
			values: map[string]string{"a": "x"},
			want:   "x\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segs, err := parseTemplate(tt.tmpl)
			if err != nil {
				t.Fatalf("parseTemplate(%q): %v", tt.tmpl, err)
			}
			if got := render(segs, tt.values); got != tt.want {
				t.Fatalf("render(%q) = %q, want %q", tt.tmpl, got, tt.want)
			}
		})
	}
}

func TestParseTemplateErrors(t *testing.T) {
	for _, tmpl := range []string{
		"${",
		"${name",
		"${}",
		"${a b}",
		"${a-b}",
	} {
		if _, err := parseTemplate(tmpl); err == nil {
			t.Errorf("parseTemplate(%q) succeeded, want error", tmpl)
		}
	}
}
