package exporteos

// bundle is the writer-ready representation of a project.
//
// loadBundle (loader.go) populates a bundle from the LacyLights repositories;
// Service.Export then feeds the sections to the writer in a fixed order.
type bundle struct {
	ProjectName   string
	ParamTypes    []ParamTypeOut
	Personalities []PersonalityIn
	Patch         []PatchEntryOut
	Palettes      []PaletteOut
	CueLists      []CueListOut
	Groups        []GroupOut
	Sidecar       SidecarOut
}
