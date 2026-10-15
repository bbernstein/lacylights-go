package resolvers

import (
	"context"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/services/gdtf"
)

// importGdtfFixtures implements the importGdtfFixtures mutation.
func (r *mutationResolver) importGdtfFixtures(ctx context.Context, files []*generated.GdtfFileInput, options *generated.GdtfImportOptions) (*generated.GdtfImportResult, error) {
	opts := gdtf.Options{OnExisting: gdtf.OnExistingUpdate}
	if options != nil {
		if p := options.OnExisting.Value(); p != nil && *p == generated.GdtfExistingPolicySkip {
			opts.OnExisting = gdtf.OnExistingSkip
		}
	}
	in := make([]gdtf.File, 0, len(files))
	for _, f := range files {
		if f == nil {
			continue
		}
		in = append(in, gdtf.File{FileName: f.FileName, DescriptionXML: f.DescriptionXML})
	}

	res := r.GDTFService.Import(ctx, in, opts)
	out := &generated.GdtfImportResult{
		Results:        make([]*generated.GdtfImportFileResult, 0, len(res.Results)),
		CreatedCount:   res.CreatedCount,
		UpdatedCount:   res.UpdatedCount,
		UnchangedCount: res.UnchangedCount,
		SkippedCount:   res.SkippedCount,
		FailedCount:    res.FailedCount,
	}
	for i := range res.Results {
		fr := &res.Results[i]
		item := &generated.GdtfImportFileResult{
			FileName:     fr.FileName,
			Status:       generated.GdtfImportStatus(fr.Status),
			Definition:   fr.Definition,
			Manufacturer: nonEmpty(fr.Manufacturer),
			Model:        nonEmpty(fr.Model),
			Warnings:     fr.Warnings,
			Error:        nonEmpty(fr.Error),
		}
		if item.Warnings == nil {
			item.Warnings = []string{}
		}
		if fr.Status != gdtf.StatusFailed {
			count := fr.ModeCount
			item.ModeCount = &count
		}
		out.Results = append(out.Results, item)
	}
	return out, nil
}

// nonEmpty returns nil for "" and a pointer to s otherwise.
func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
