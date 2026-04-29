package resolvers

import (
	"sort"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	importeos "github.com/bbernstein/lacylights-go/internal/services/import_eos"
)

// toEosWarnings converts importeos warnings to GraphQL warning shapes.
// Context entries are sorted by key so output is deterministic across runs;
// Go's map iteration order would otherwise produce flaky tests and unstable
// client diffs. The returned Context is always a non-nil slice (possibly
// empty) to match the non-nullable list shape declared in the schema.
//
// This helper lives outside schema.resolvers.go because gqlgen rewrites that
// file on regeneration and would otherwise relocate this function to the
// trailing /* ... */ cruft block.
func toEosWarnings(in []importeos.Warning) []*generated.EosWarning {
	out := make([]*generated.EosWarning, 0, len(in))
	for _, w := range in {
		ew := &generated.EosWarning{
			Code:     string(w.Code),
			Severity: generated.EosWarningSeverity(string(w.Severity)),
			Message:  w.Message,
		}
		ew.Context = make([]*generated.EosWarningContextEntry, 0, len(w.Context))
		if len(w.Context) > 0 {
			keys := make([]string, 0, len(w.Context))
			for k := range w.Context {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				ew.Context = append(ew.Context, &generated.EosWarningContextEntry{Key: k, Value: w.Context[k]})
			}
		}
		out = append(out, ew)
	}
	return out
}
