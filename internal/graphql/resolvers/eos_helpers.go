package resolvers

import (
	"sort"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	importeos "github.com/bbernstein/lacylights-go/internal/services/import_eos"
)

// severityToGraphQL maps an importeos severity to its generated GraphQL
// enum constant. A typed switch instead of a raw string cast surfaces enum
// drift (e.g. a new severity added to the service layer without a schema
// update) at compile time rather than emitting an invalid enum to clients.
func severityToGraphQL(s importeos.Severity) generated.EosWarningSeverity {
	switch s {
	case importeos.SeverityInfo:
		return generated.EosWarningSeverityInfo
	case importeos.SeverityWarn:
		return generated.EosWarningSeverityWarn
	}
	// Unknown severity → fall back to WARN so the message is at least
	// surfaced rather than dropped, but a future linter run will catch
	// the unhandled case if Severity gains a new constant.
	return generated.EosWarningSeverityWarn
}

// maxEosContentBytes caps the size of an inbound EOS ASCII payload. The
// largest production showfile we have on hand is the OTBPA fixture at ~500 KB;
// the cap is generous enough for very large shows but small enough that a
// misconfigured or malicious client can't consume arbitrary memory before the
// parser sees it.
//
// This check is application-layer: by the time it runs, the GraphQL runtime
// has already decoded the JSON body into a Go string. For complete protection
// against oversized requests, the HTTP server should also cap the request
// body via http.MaxBytesReader. That is currently configured at the server
// boundary; the limit here adds a documented contract at the resolver entry.
//
// Lives in this file (not schema.resolvers.go) so gqlgen regeneration won't
// strip it.
const maxEosContentBytes = 50 << 20 // 50 MiB


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
			Severity: severityToGraphQL(w.Severity),
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
