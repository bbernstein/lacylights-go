package resolvers

import (
	"context"
	"errors"
	"fmt"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/middleware"
	importqlc "github.com/bbernstein/lacylights-go/internal/services/import_qlc"
)

// maxQLCContentBytes caps inbound QLC+ workspace XML at the resolver entry,
// matching maxEosContentBytes. Defined here (not in schema.resolvers.go) to
// survive gqlgen regeneration.
const maxQLCContentBytes = 50 << 20 // 50 MiB

// importProjectFromQLC implements Mutation.importProjectFromQLC: it creates a
// new project from a QLC+ workspace (.qxw). It lives outside
// schema.resolvers.go so gqlgen regeneration leaves it alone.
func (r *Resolver) importProjectFromQLC(ctx context.Context, xmlContent, originalFileName string) (*generated.QLCImportResult, error) {
	if r.QLCImportService == nil {
		return nil, errors.New("QLC+ import not available on this platform")
	}
	if len(xmlContent) > maxQLCContentBytes {
		return nil, fmt.Errorf("xmlContent exceeds the %d MiB maximum", maxQLCContentBytes>>20)
	}

	opts := importqlc.Options{
		ProjectName: importqlc.ProjectNameFromFileName(originalFileName),
		Description: "Imported from QLC+ workspace " + originalFileName,
	}
	var groupWarnings []string
	if r.AuthService != nil && r.AuthService.IsEnabled() {
		// Same rule as importProject/importProjectFromEos: the new project
		// goes to the caller's first group (the API has no groupId option).
		groupIDs := middleware.GetUserGroupIDs(ctx)
		if len(groupIDs) >= 1 {
			gid := groupIDs[0]
			opts.GroupID = &gid
			if len(groupIDs) > 1 {
				groupWarnings = append(groupWarnings, "The imported project was assigned to your first group")
			}
		} else if !middleware.IsAdmin(ctx) {
			return nil, fmt.Errorf("cannot import project: user does not belong to any groups")
		}
	}

	res, err := r.QLCImportService.Import(ctx, xmlContent, opts)
	if err != nil {
		return nil, err
	}
	project, err := r.ProjectRepo.FindByID(ctx, res.ProjectID)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, fmt.Errorf("imported project %s not found", res.ProjectID)
	}

	warnings := append(res.Warnings, groupWarnings...)
	if warnings == nil {
		warnings = []string{}
	}
	return &generated.QLCImportResult{
		Project:          *project,
		OriginalFileName: originalFileName,
		FixtureCount:     res.FixtureCount,
		LookCount:        res.LookCount,
		CueListCount:     res.CueListCount,
		Warnings:         warnings,
	}, nil
}
