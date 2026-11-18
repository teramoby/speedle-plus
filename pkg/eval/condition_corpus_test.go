// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The files below are the corpus that pkg/condexpr's compile-mode
// differential test reads. This test owns them: it derives the expressions
// from the conformance cases and from the policy fixtures in this package, and
// fails when a file drifts. Regenerate with UPDATE_CONDITION_CORPUS=1.

const (
	conformanceCorpusFile = "testdata/condition-conformance-expressions.txt"
	policyCorpusFile      = "testdata/engine-policy-conditions.txt"
)

// fixtureConditionPattern matches a JSON condition value inside a Go source
// file. Conditions are single-line, so one match never spans a line.
var fixtureConditionPattern = regexp.MustCompile(`"condition"\s*:\s*"((?:[^"\\]|\\.)*)"`)

func TestConditionCorpusFilesAreCurrent(t *testing.T) {
	conformance := make(map[string]bool)
	for _, c := range conditionCases() {
		conformance[c.expression] = true
	}
	checkCorpusFile(t, conformanceCorpusFile, sortedCorpus(conformance))

	checkCorpusFile(t, policyCorpusFile, fixturePolicyConditions(t))
}

// fixturePolicyConditions collects every JSON condition literal from the test
// sources of this package, which is the policy set the evaluator tests use.
func fixturePolicyConditions(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	conditions := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range fixtureConditionPattern.FindAllSubmatch(source, -1) {
			condition, err := strconv.Unquote(`"` + string(match[1]) + `"`)
			if err != nil {
				t.Fatalf("%s: malformed condition literal %q: %v", name, match[1], err)
			}
			if condition != "" {
				conditions[condition] = true
			}
		}
	}
	return sortedCorpus(conditions)
}

func sortedCorpus(set map[string]bool) []string {
	conditions := make([]string, 0, len(set))
	for condition := range set {
		conditions = append(conditions, condition)
	}
	sort.Strings(conditions)
	return conditions
}

// checkCorpusFile rewrites the file when UPDATE_CONDITION_CORPUS=1, and
// otherwise fails when the committed content differs from the derived corpus.
func checkCorpusFile(t *testing.T, path string, want []string) {
	t.Helper()
	content := strings.Join(want, "\n") + "\n"
	if os.Getenv("UPDATE_CONDITION_CORPUS") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d conditions to %s", len(want), path)
		return
	}
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./pkg/eval -run TestConditionCorpusFilesAreCurrent with UPDATE_CONDITION_CORPUS=1)", err)
	}
	if string(committed) == content {
		return
	}
	have := make(map[string]bool)
	for _, line := range strings.Split(string(committed), "\n") {
		if line != "" {
			have[line] = true
		}
	}
	wantSet := make(map[string]bool, len(want))
	for _, condition := range want {
		wantSet[condition] = true
	}
	var missing, extra []string
	for _, condition := range want {
		if !have[condition] {
			missing = append(missing, condition)
		}
	}
	for condition := range have {
		if !wantSet[condition] {
			extra = append(extra, condition)
		}
	}
	sort.Strings(extra)
	t.Fatalf("%s is stale: %d new, %d removed\nnew: %q\nremoved: %q\nRegenerate with UPDATE_CONDITION_CORPUS=1",
		path, len(missing), len(extra), missing, extra)
}
