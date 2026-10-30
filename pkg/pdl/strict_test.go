package pdl

import (
	"encoding/json"
	"fmt"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/testutil/pdltest"
	"reflect"
	"strings"
	"testing"
)

func TestGrammarCorpus(t *testing.T) {
	for _, c := range pdltest.Cases {
		t.Run(c.Name, func(t *testing.T) {
			var err error
			if c.Kind == "policy" {
				_, _, err = ParsePolicy(c.Text, "p")
			} else {
				_, _, err = ParseRolePolicy(c.Text, "p")
			}
			if (err == nil) != c.Valid {
				t.Fatalf("valid=%v, error=%v", c.Valid, err)
			}
		})
	}
}
func TestQuotedAndWhitespaceAST(t *testing.T) {
	p, reader, err := ParsePolicy("grant\t(user\talice\u2003from\tcorp, group devs), role admin\tread,write\t'book#1'\tif\tx > 0\r\n", "p")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Principals, [][]string{{"idd=corp:user:alice", "group:devs"}, {"role:admin"}}) || p.Condition != "x > 0" || p.Permissions[0].Resource != "book#1" {
		t.Fatalf("AST %+v", p)
	}
	var decoded pms.Policy
	if err := json.NewDecoder(reader).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*p, decoded) {
		t.Fatalf("JSON round trip: %+v != %+v", p, decoded)
	}
	p, _, err = ParsePolicy(`grant user "ali'ce" read 'book"one'`, "")
	if err != nil || p.Principals[0][0] != "user:ali'ce" || p.Permissions[0].Resource != `book"one` {
		t.Fatalf("escaped AST %+v %v", p, err)
	}
}
func FuzzParseStatements(f *testing.F) {
	for _, c := range pdltest.Cases {
		f.Add(c.Text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 8192 {
			return
		}
		if p, _, err := ParsePolicy(text, "p"); err == nil {
			if p == nil || len(p.Permissions) == 0 || len(p.Principals) == 0 {
				t.Fatalf("partial AST %+v", p)
			}
			if _, _, err := ParsePolicy(text+" ) trailing", "p"); p.Condition == "" && err == nil {
				t.Fatal("accepted trailing input")
			}
		}
		if p, _, err := ParseRolePolicy(text, "p"); err == nil && (p == nil || len(p.Roles) == 0) {
			t.Fatalf("partial role AST %+v", p)
		}
	})
}
func TestGeneratedNamesRoundTrip(t *testing.T) {
	for _, name := range []string{"book", "书籍", "a b", "a#b", "a,b", "a(b)", `a"b`, `a'b`, `a\b`} {
		quote := "'"
		if strings.Contains(name, quote) {
			quote = `"`
		}
		quoted := quote + name + quote
		p, _, err := ParsePolicy(fmt.Sprintf("grant user alice read %s", quoted), "")
		if err != nil || p.Permissions[0].Resource != name {
			t.Fatalf("%q: %+v %v", name, p, err)
		}
	}
}
