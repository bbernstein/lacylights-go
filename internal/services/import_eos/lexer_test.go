package importeos

import (
	"strings"
	"testing"
)

func TestLexer_TokenizesBasicDirectives(t *testing.T) {
	src := `Ident 3:0
Manufacturer ETC
Console Eos
$$Format 3.20
! a comment line
$Personality 11896
   $$Manuf Chauvet
   $$Model Colorado_1_Solo_SSP
`

	got, err := tokenize(strings.NewReader(src))
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	want := []Line{
		{Lineno: 1, Kind: LineDirective, Directive: "Ident", Fields: []string{"3:0"}, Indent: 0, Raw: "Ident 3:0"},
		{Lineno: 2, Kind: LineDirective, Directive: "Manufacturer", Fields: []string{"ETC"}, Indent: 0, Raw: "Manufacturer ETC"},
		{Lineno: 3, Kind: LineDirective, Directive: "Console", Fields: []string{"Eos"}, Indent: 0, Raw: "Console Eos"},
		{Lineno: 4, Kind: LineDirective, Directive: "$$Format", Fields: []string{"3.20"}, Indent: 0, Raw: "$$Format 3.20"},
		{Lineno: 5, Kind: LineComment, Raw: "! a comment line"},
		{Lineno: 6, Kind: LineDirective, Directive: "$Personality", Fields: []string{"11896"}, Indent: 0, Raw: "$Personality 11896"},
		{Lineno: 7, Kind: LineDirective, Directive: "$$Manuf", Fields: []string{"Chauvet"}, Indent: 3, Raw: "   $$Manuf Chauvet"},
		{Lineno: 8, Kind: LineDirective, Directive: "$$Model", Fields: []string{"Colorado_1_Solo_SSP"}, Indent: 3, Raw: "   $$Model Colorado_1_Solo_SSP"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Lineno != want[i].Lineno || got[i].Kind != want[i].Kind ||
			got[i].Directive != want[i].Directive || got[i].Indent != want[i].Indent {
			t.Errorf("line %d mismatch: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLexer_BlankLine(t *testing.T) {
	got, err := tokenize(strings.NewReader("\n   \n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Kind != LineBlank || got[1].Kind != LineBlank {
		t.Errorf("got %+v, want two blank lines", got)
	}
}
