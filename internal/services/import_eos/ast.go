package importeos

// Show is the root of the parsed Eos ASCII AST.
type Show struct {
	Ident          string            // e.g. "3:0"
	Manufacturer   string            // e.g. "ETC"
	Console        string            // e.g. "Eos"
	Format         string            // e.g. "3.20"
	SoftwareString string            // raw text from $$Software ... line
	Title          string            // $$Title value
	ParamTypes     []ParamType
	Personalities  []Personality
	Patch          []PatchEntry
	CueLists       []CueList
	ColorPalettes  []Palette
	BeamPalettes   []Palette
	FocusPalettes  []Palette
	IntensPalettes []Palette
	Presets        []Palette
	Groups         []Group
	UnknownLines   []int    // 1-based line numbers we skipped
	SidecarLines   []string // raw "$$ LACYLIGHTS:..." lines for the sidecar reader
}

// ParamType is one row of the $ParamType table.
type ParamType struct {
	ID        int
	Category  int
	LongName  string
	ShortName string
}

// Personality is one $Personality block.
type Personality struct {
	ID        int
	Manuf     string
	Model     string
	Dcid      string
	Footprint int
	Channels  []PersChannel
}

// PersChannel describes one channel of a personality.
type PersChannel struct {
	ParamID   int
	Size      int    // 1 = 8-bit, 2 = 16-bit
	Offset    int    // 1-based MSB offset
	Offset16  int    // 1-based LSB offset (for 16-bit), else 0
	HomeValue int
	Flags     string // e.g. "S" for Snap
}

// PatchEntry is a single $Patch line plus its sub-directives.
type PatchEntry struct {
	Channel       int
	AddressRaw    string  // e.g. "1.512", "513", or "1/512"
	PersonalityID int
	Label         string  // from "Text" sub-directive
	UnicodeText   *string // from $$UText, decoded
}

// CueList holds one cue list and its cues.
type CueList struct {
	Number int
	Label  string
	Cues   []Cue
}

// Cue is a single cue (or cue part) within a list.
type Cue struct {
	Number      string  // "0.5", "5/2", etc. — without the part suffix once parsed
	Part        int     // 0 if not a part
	Label       string
	UnicodeText *string
	UpFade      float64
	UpDelay     float64
	DownFade    float64
	DownDelay   float64
	Follow      *float64
	Hang        *float64
	Block       bool
	IntBlock    bool
	ChanMoves   []ChanMove  // intensity moves (8-bit values)
	ParamMoves  []ParamMove // attribute moves with per-parameter values
}

// ChanMove is a single tracked-intensity move within a cue.
// Format in source: "1@H00" → channel 1, hex 0x00.
type ChanMove struct {
	Channel int
	Value   int
}

// ParamMove is one channel's attribute moves within a cue.
// Format in source: "$$Param 41 1@0 12@255 13@201 14@236 204@0".
// First field is channel, remaining are paramID@value pairs.
type ParamMove struct {
	Channel int
	Values  []ParamValue
}

// ParamValue is one paramID/value pair.
type ParamValue struct {
	ParamID int
	Value   int
}

// Palette is a Color/Beam/Focus/Intensity palette or a Preset.
type Palette struct {
	Number      string
	Label       string
	UnicodeText *string
	ChanMoves   []ChanMove
	ParamMoves  []ParamMove
}

// Group is one $Group block.
type Group struct {
	Number      string
	Label       string
	UnicodeText *string
	Channels    []int
}
