package resolvers

import (
	"context"
	"fmt"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/services/gdtf"
)

// Per-request limits for importGdtfFixtures. Clients send batches of 25
// files and at most 64 MB; these leave 2x room or more while bounding the
// work one request can cause.
const (
	maxGdtfFilesPerRequest = 200
	maxGdtfRequestBytes    = 128 << 20
)

// importGdtfFixtures implements the importGdtfFixtures mutation.
func (r *mutationResolver) importGdtfFixtures(ctx context.Context, files []*generated.GdtfFileInput, options *generated.GdtfImportOptions) (*generated.GdtfImportResult, error) {
	// Definitions are global and an update overwrites one by FixtureTypeID.
	if err := r.requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	opts := gdtf.Options{OnExisting: gdtf.OnExistingUpdate}
	if options != nil {
		if p := options.OnExisting.Value(); p != nil && *p == generated.GdtfExistingPolicySkip {
			opts.OnExisting = gdtf.OnExistingSkip
		}
	}
	if len(files) > maxGdtfFilesPerRequest {
		return nil, fmt.Errorf("too many files in one request: %d (at most %d; send them in batches)", len(files), maxGdtfFilesPerRequest)
	}
	in := make([]gdtf.File, 0, len(files))
	total := 0
	for _, f := range files {
		if f == nil {
			continue
		}
		total += len(f.DescriptionXML)
		if total > maxGdtfRequestBytes {
			return nil, fmt.Errorf("request is larger than %d MB of GDTF descriptions; send the files in smaller batches", maxGdtfRequestBytes>>20)
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
