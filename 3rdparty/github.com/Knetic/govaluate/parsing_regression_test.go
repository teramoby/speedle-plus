package govaluate

import "testing"

func TestRejectDanglingEscapes(t *testing.T) {
	for _, text := range []string{`\`, `abc\`, `[abc\`, `'abc\`, `"abc\`} {
		t.Run(text, func(t *testing.T) {
			if _, err := NewEvaluableExpression(text); err == nil {
				t.Fatalf("accepted dangling escape %q", text)
			}
		})
	}
}
