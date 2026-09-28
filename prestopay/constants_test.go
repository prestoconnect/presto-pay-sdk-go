package prestopay

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// TestConstants_NameIsPrefixPlusValue guards the one invariant that matters
// for the wire-vocabulary constants: nobody "tidies" a value like
// PaymentMethodPmPgCard or PaymentMethodUnionPayQR into something more
// readable. ErrorCode is excluded — a four-digit code is not a name.
func TestConstants_NameIsPrefixPlusValue(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "constants.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing constants.go: %v", err)
	}

	prefixes := []string{"TxnType", "PaymentStatus", "ReversalStatus", "RefundStatus", "PaymentMethod"}
	checked := 0

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vspec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vspec.Names {
				if i >= len(vspec.Values) {
					continue
				}
				lit, ok := vspec.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquoting %s: %v", name.Name, err)
				}
				prefix, matched := longestMatchingPrefix(name.Name, prefixes)
				if !matched {
					continue
				}
				checked++
				if want := prefix + value; name.Name != want {
					t.Errorf("%s = %q: name should be %q", name.Name, value, want)
				}
			}
		}
	}

	if checked == 0 {
		t.Fatal("no constants matched any of the checked prefixes; the test is not exercising anything")
	}
}

func longestMatchingPrefix(name string, prefixes []string) (string, bool) {
	best := ""
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) && len(p) > len(best) {
			best = p
		}
	}
	return best, best != ""
}
