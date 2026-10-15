package models

import "testing"

func TestFixtureDefinitionBeforeCreate_DefaultsSourceToUser(t *testing.T) {
	d := &FixtureDefinition{}
	if err := d.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if d.Source != FixtureSourceUser {
		t.Errorf("Source = %q, want %q", d.Source, FixtureSourceUser)
	}
}

func TestFixtureDefinitionBeforeCreate_KeepsExplicitSource(t *testing.T) {
	d := &FixtureDefinition{Source: FixtureSourceGDTF}
	if err := d.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if d.Source != FixtureSourceGDTF {
		t.Errorf("Source = %q, want %q", d.Source, FixtureSourceGDTF)
	}
}

func TestDefaultFixtureSourcePrecedence_ListsEverySourceOnce(t *testing.T) {
	want := []string{FixtureSourceUser, FixtureSourceGDTF, FixtureSourceOFL,
		FixtureSourceBuiltIn, FixtureSourceEOS, FixtureSourceSynthesized}
	if len(DefaultFixtureSourcePrecedence) != len(want) {
		t.Fatalf("len = %d, want %d", len(DefaultFixtureSourcePrecedence), len(want))
	}
	for i, s := range want {
		if DefaultFixtureSourcePrecedence[i] != s {
			t.Errorf("[%d] = %q, want %q", i, DefaultFixtureSourcePrecedence[i], s)
		}
	}
}
