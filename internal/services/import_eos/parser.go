package importeos

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ParseError is returned for fatal parser failures.
type ParseError struct {
	Lineno int
	Msg    string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("eos parse error at line %d: %s", e.Lineno, e.Msg)
}

// Parse reads r as an Eos ASCII file and returns the AST plus a Collector
// containing any non-fatal warnings (e.g. UNKNOWN_DIRECTIVE).
func Parse(r io.Reader) (*Show, *Collector, error) {
	lines, err := tokenize(r)
	if err != nil {
		return nil, nil, err
	}
	p := &parser{
		lines:          lines,
		show:           &Show{},
		warn:           &Collector{},
		personalityIDs: map[int]struct{}{},
	}
	if err := p.run(); err != nil {
		return nil, nil, err
	}
	return p.show, p.warn, nil
}

type parser struct {
	lines []Line
	pos   int
	show  *Show
	warn  *Collector
	// personalityIDs is a set of $Personality IDs seen so far; used by
	// $Patch field-order autodetection in O(1) per lookup. Populated by
	// parsePersonality and consumed by fieldIsKnownPersonality.
	personalityIDs map[int]struct{}
}

func (p *parser) cur() *Line {
	if p.pos >= len(p.lines) {
		return nil
	}
	return &p.lines[p.pos]
}

func (p *parser) advance() { p.pos++ }

func (p *parser) run() error {
	for p.pos < len(p.lines) {
		line := p.cur()
		if line.Kind == LineSidecar {
			p.show.SidecarLines = append(p.show.SidecarLines, line.Raw)
			p.advance()
			continue
		}
		if line.Kind != LineDirective || line.Indent > 0 {
			p.advance()
			continue
		}
		if err := p.handleTopLevel(line); err != nil {
			return err
		}
	}
	return nil
}

func (p *parser) handleTopLevel(line *Line) error {
	switch line.Directive {
	case "Ident":
		p.show.Ident = strings.Join(line.Fields, " ")
		p.advance()
	case "Manufacturer":
		p.show.Manufacturer = strings.Join(line.Fields, " ")
		p.advance()
	case "Console":
		p.show.Console = strings.Join(line.Fields, " ")
		p.advance()
	case "$$Format":
		p.show.Format = strings.Join(line.Fields, " ")
		p.advance()
	case "$$Software":
		p.show.SoftwareString = line.Raw
		p.advance()
	case "$$Title":
		p.show.Title = strings.Join(line.Fields, " ")
		p.advance()
	case "$ParamType":
		return p.parseParamType()
	case "$Personality":
		return p.parsePersonality()
	case "$Patch":
		return p.parsePatch()
	case "$CueList":
		return p.parseCueList()
	case "Cue":
		return p.parseCue()
	case "$Cue":
		return p.parseCue()
	case "$ColorPalette":
		return p.parsePalette(&p.show.ColorPalettes)
	case "$BeamPalette":
		return p.parsePalette(&p.show.BeamPalettes)
	case "$FocusPalette":
		return p.parsePalette(&p.show.FocusPalettes)
	case "$IntensPalette":
		return p.parsePalette(&p.show.IntensPalettes)
	case "$Preset":
		return p.parsePalette(&p.show.Presets)
	case "$Group":
		return p.parseGroup()
	case "$Effect":
		return p.skipBlockWithWarning(WarnEffectSkipped, "EOS effect not modeled")
	case "$FaderDef", "$Sub":
		return p.skipBlockWithWarning(WarnSubmasterSkipped, "EOS submaster/fader not modeled")
	case "$Partition":
		return p.skipBlockWithWarning(WarnPartitionSkipped, "EOS partition not modeled")
	case "$MagicSheet":
		return p.skipBlockWithWarning(WarnMagicSheetSkipped, "EOS magic sheet not modeled")
	case "$Action":
		return p.skipBlockWithWarning(WarnActionSkipped, "EOS action trigger not modeled")
	case "$Curve":
		return p.skipBlockWithWarning(WarnCurveSkipped, "EOS curve not modeled")
	default:
		// All other top-level directives become "skipped" until later tasks teach
		// the parser to handle them. We emit UNKNOWN_DIRECTIVE only for $-prefixed
		// directives we don't recognize so that ordinary keywords like "Clear"
		// or "Set" are silently passed over.
		if strings.HasPrefix(line.Directive, "$") {
			p.warn.Add(WarnUnknownDirective, SeverityInfo,
				fmt.Sprintf("skipping unknown directive %s", line.Directive),
				map[string]string{"line": strconv.Itoa(line.Lineno)})
		}
		p.advance()
	}
	return nil
}

func (p *parser) parseParamType() error {
	line := p.cur()
	if len(line.Fields) < 3 {
		return &ParseError{Lineno: line.Lineno, Msg: "$ParamType needs at least Id Cat Name"}
	}
	id, err := strconv.Atoi(line.Fields[0])
	if err != nil {
		return &ParseError{Lineno: line.Lineno, Msg: "$ParamType id not an integer"}
	}
	cat, err := strconv.Atoi(line.Fields[1])
	if err != nil {
		return &ParseError{Lineno: line.Lineno, Msg: "$ParamType cat not an integer"}
	}
	pt := ParamType{ID: id, Category: cat, LongName: strings.Join(line.Fields[2:], " ")}
	p.advance()
	for p.cur() != nil && p.cur().Indent > 0 {
		sub := p.cur()
		if sub.Directive == "$$ShortName" {
			pt.ShortName = strings.Join(sub.Fields, " ")
		}
		p.advance()
	}
	p.show.ParamTypes = append(p.show.ParamTypes, pt)
	return nil
}

func (p *parser) parsePersonality() error {
	line := p.cur()
	if len(line.Fields) < 1 {
		return &ParseError{Lineno: line.Lineno, Msg: "$Personality missing id"}
	}
	id, err := strconv.Atoi(line.Fields[0])
	if err != nil {
		return &ParseError{Lineno: line.Lineno, Msg: "$Personality id not an integer"}
	}
	pers := Personality{ID: id}
	p.advance()
	for p.cur() != nil && p.cur().Indent > 0 {
		sub := p.cur()
		switch sub.Directive {
		case "$$Manuf":
			pers.Manuf = strings.Join(sub.Fields, " ")
		case "$$Model":
			pers.Model = strings.Join(sub.Fields, " ")
		case "$$Dcid":
			pers.Dcid = strings.Join(sub.Fields, " ")
		case "$$Footprint":
			if len(sub.Fields) >= 1 {
				if v, err := strconv.Atoi(sub.Fields[0]); err == nil {
					pers.Footprint = v
				}
			}
		case "$$PersChan":
			if pc, ok := parsePersChan(sub); ok {
				pers.Channels = append(pers.Channels, pc)
			}
		}
		p.advance()
	}
	p.show.Personalities = append(p.show.Personalities, pers)
	p.personalityIDs[id] = struct{}{}
	// LacyLights synthesizes IDs starting at 90001 on export; warn if an
	// incoming file already uses an ID in that range so a future re-export
	// collision is at least visible to the operator.
	const synthesizedPersIDFloor = 90001
	if id >= synthesizedPersIDFloor {
		p.warn.Add(WarnUnknownDirective, SeverityInfo,
			fmt.Sprintf("$Personality %d uses an ID in the LacyLights synthesized range (>= %d); re-export may collide",
				id, synthesizedPersIDFloor),
			map[string]string{"line": strconv.Itoa(line.Lineno), "personalityId": strconv.Itoa(id)})
	}
	return nil
}

func parsePersChan(line *Line) (PersChannel, bool) {
	// Fields: paramID size offset (offset16) home (flags)
	// All numeric fields are strictly parsed: a malformed value drops the
	// whole channel rather than silently substituting zero, since a wrong
	// offset would corrupt patch addressing and a wrong home value would
	// silently change fixture defaults.
	if len(line.Fields) < 4 {
		return PersChannel{}, false
	}
	paramID, err := strconv.Atoi(line.Fields[0])
	if err != nil {
		return PersChannel{}, false
	}
	size, err := strconv.Atoi(line.Fields[1])
	if err != nil {
		return PersChannel{}, false
	}
	offset, err := strconv.Atoi(line.Fields[2])
	if err != nil {
		return PersChannel{}, false
	}
	pc := PersChannel{ParamID: paramID, Size: size, Offset: offset}
	idx := 3
	if size == 2 && len(line.Fields) > idx {
		v, err := strconv.Atoi(line.Fields[idx])
		if err != nil {
			return PersChannel{}, false
		}
		pc.Offset16 = v
		idx++
	}
	if idx < len(line.Fields) {
		v, err := strconv.Atoi(line.Fields[idx])
		if err != nil {
			return PersChannel{}, false
		}
		pc.HomeValue = v
		idx++
	}
	if idx < len(line.Fields) {
		pc.Flags = line.Fields[idx]
	}
	return pc, true
}

// fieldIsKnownPersonality reports whether the textual field is an integer that
// matches an already-parsed $Personality block. Backed by p.personalityIDs so
// each call is O(1) — important for files like OTBPA with hundreds of
// $Patch entries and dozens of personalities.
func (p *parser) fieldIsKnownPersonality(s string) bool {
	v, err := strconv.Atoi(s)
	if err != nil {
		return false
	}
	_, ok := p.personalityIDs[v]
	return ok
}

func (p *parser) parsePatch() error {
	line := p.cur()
	if len(line.Fields) < 3 {
		return &ParseError{Lineno: line.Lineno, Msg: "$Patch needs Channel Address PersID"}
	}
	channel, err := strconv.Atoi(line.Fields[0])
	if err != nil {
		return &ParseError{Lineno: line.Lineno, Msg: "$Patch channel not an integer"}
	}
	// EOS exports use two field orderings depending on the source:
	//   user-authored:  $Patch <chan> <addr> <persID>                 (3 fields)
	//   library-driven: $Patch <chan> <persID> <addr> <wheel> <count> (5+ fields)
	//
	// We resolve by:
	//   1. Requiring at least 5 fields for the library-driven form. This
	//      avoids a false positive when a user-authored 3-field line
	//      happens to have a flat absolute address that matches a
	//      synthesized personality ID (e.g. addr 9001 on a 4-universe rig,
	//      where 9001 is also our persIDBase). Lines with 6+ fields are
	//      treated as library-driven too (forward-compatible with future
	//      EOS revisions that may append optional fields), with a
	//      diagnostic so anyone reading warnings notices the variance.
	//   2. Within a library-driven line, swapping only when fields[1] is
	//      a declared personality ID and fields[2] is not — both fields
	//      being valid personalities (or neither) is ambiguous, so we
	//      leave the user-authored ordering as the safe default.
	addrIdx, persIdx := 1, 2
	if len(line.Fields) >= 5 {
		f1Pers := p.fieldIsKnownPersonality(line.Fields[1])
		f2Pers := p.fieldIsKnownPersonality(line.Fields[2])
		if f1Pers && !f2Pers {
			addrIdx, persIdx = 2, 1
			if len(line.Fields) > 5 {
				p.warn.Add(WarnUnknownDirective, SeverityInfo,
					fmt.Sprintf("$Patch line has %d fields (expected 5 for library-driven form); extra fields ignored",
						len(line.Fields)),
					map[string]string{"line": strconv.Itoa(line.Lineno)})
			}
		} else if f1Pers && f2Pers {
			// Both fields parse as known personality IDs — the line is
			// genuinely ambiguous. Default to user-authored ordering
			// (fields[1] = address, fields[2] = persID) so synthetic
			// fixtures keep working, but warn so an operator can review.
			p.warn.Add(WarnUnknownDirective, SeverityWarn,
				fmt.Sprintf("$Patch line is ambiguous (fields[1]=%s and fields[2]=%s both parse as known personality IDs); defaulting to user-authored ordering",
					line.Fields[1], line.Fields[2]),
				map[string]string{"line": strconv.Itoa(line.Lineno)})
		}
	}
	persID, err := strconv.Atoi(line.Fields[persIdx])
	if err != nil {
		return &ParseError{Lineno: line.Lineno, Msg: "$Patch personality not an integer"}
	}
	pe := PatchEntry{Channel: channel, AddressRaw: line.Fields[addrIdx], PersonalityID: persID}
	p.advance()
	for p.cur() != nil && p.cur().Indent > 0 {
		sub := p.cur()
		switch sub.Directive {
		case "Text":
			pe.Label = strings.Join(sub.Fields, " ")
		case "$$UText":
			if s, ok := decodeUText(sub.Fields); ok {
				pe.UnicodeText = &s
			} else {
				p.warn.Add(WarnUTextDecode, SeverityWarn,
					"failed to decode $$UText",
					map[string]string{"line": strconv.Itoa(sub.Lineno)})
			}
		}
		p.advance()
	}
	p.show.Patch = append(p.show.Patch, pe)
	return nil
}

func (p *parser) parseCueList() error {
	line := p.cur()
	if len(line.Fields) < 1 {
		return &ParseError{Lineno: line.Lineno, Msg: "$CueList needs number"}
	}
	num, err := strconv.Atoi(line.Fields[0])
	if err != nil {
		return &ParseError{Lineno: line.Lineno, Msg: "$CueList number not an integer"}
	}
	cl := CueList{Number: num}
	p.advance()
	for p.cur() != nil && p.cur().Indent > 0 {
		sub := p.cur()
		if sub.Directive == "Text" {
			cl.Label = strings.Join(sub.Fields, " ")
		}
		p.advance()
	}
	p.show.CueLists = append(p.show.CueLists, cl)
	return nil
}

func (p *parser) parseCue() error {
	line := p.cur()
	if len(line.Fields) < 2 {
		return &ParseError{Lineno: line.Lineno, Msg: "Cue needs Number ListNum"}
	}
	listNum, err := strconv.Atoi(line.Fields[1])
	if err != nil {
		return &ParseError{Lineno: line.Lineno, Msg: "Cue listNum not an integer"}
	}
	cue := Cue{Number: line.Fields[0]}
	if slash := strings.IndexByte(cue.Number, '/'); slash >= 0 {
		if part, err := strconv.Atoi(cue.Number[slash+1:]); err == nil {
			cue.Part = part
			cue.Number = cue.Number[:slash]
		}
	}
	p.advance()
	for p.cur() != nil && p.cur().Indent > 0 {
		sub := p.cur()
		switch sub.Directive {
		case "Text":
			cue.Label = strings.Join(sub.Fields, " ")
		case "Up":
			if len(sub.Fields) >= 1 {
				cue.UpFade, _ = strconv.ParseFloat(sub.Fields[0], 64)
			}
		case "Down":
			if len(sub.Fields) >= 1 {
				cue.DownFade, _ = strconv.ParseFloat(sub.Fields[0], 64)
			}
		case "$$Follow":
			if len(sub.Fields) >= 1 {
				if v, err := strconv.ParseFloat(sub.Fields[0], 64); err == nil {
					cue.Follow = &v
				}
			}
		case "$$Hang":
			if len(sub.Fields) >= 1 {
				if v, err := strconv.ParseFloat(sub.Fields[0], 64); err == nil {
					cue.Hang = &v
				}
			}
		case "$$Block":
			cue.Block = true
		case "$$IntBlock":
			cue.IntBlock = true
		case "$$ChanMove":
			cue.ChanMoves = append(cue.ChanMoves, parseChanMoves(sub.Fields)...)
		case "$$Param":
			if pm, ok := parseParamMove(sub.Fields); ok {
				cue.ParamMoves = append(cue.ParamMoves, pm)
			}
		case "$$UText":
			if s, ok := decodeUText(sub.Fields); ok {
				cue.UnicodeText = &s
			} else {
				p.warn.Add(WarnUTextDecode, SeverityWarn,
					"failed to decode $$UText",
					map[string]string{"line": strconv.Itoa(sub.Lineno)})
			}
		}
		p.advance()
	}
	for i := range p.show.CueLists {
		if p.show.CueLists[i].Number == listNum {
			p.show.CueLists[i].Cues = append(p.show.CueLists[i].Cues, cue)
			return nil
		}
	}
	p.show.CueLists = append(p.show.CueLists, CueList{Number: listNum, Cues: []Cue{cue}})
	return nil
}

// parseChanMoves accepts tokens like "1@HFF" or "12@H00" and returns ChanMove entries.
func parseChanMoves(fields []string) []ChanMove {
	var out []ChanMove
	for _, f := range fields {
		at := strings.IndexByte(f, '@')
		if at < 0 {
			continue
		}
		ch, err := strconv.Atoi(f[:at])
		if err != nil {
			continue
		}
		raw := f[at+1:]
		val, err := parseLevel(raw)
		if err != nil {
			continue
		}
		out = append(out, ChanMove{Channel: ch, Value: val})
	}
	return out
}

// parseLevel decodes EOS level encoding: H<hex> for hex, or decimal otherwise.
// Returns a value in the range 0..255; returns an error for out-of-range or
// otherwise unparseable inputs so callers do not silently wrap when the value
// is later cast to byte for DMX output.
func parseLevel(raw string) (int, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("empty level")
	}
	if raw[0] == 'H' || raw[0] == 'h' {
		v, err := strconv.ParseInt(raw[1:], 16, 32)
		if err != nil {
			return 0, err
		}
		if v < 0 || v > 255 {
			return 0, fmt.Errorf("level out of range: %d", v)
		}
		return int(v), nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if v < 0 || v > 255 {
		return 0, fmt.Errorf("level out of range: %d", v)
	}
	return v, nil
}

// parseParamMove parses tokens like "41 1@0 12@255 13@201".
func parseParamMove(fields []string) (ParamMove, bool) {
	if len(fields) < 2 {
		return ParamMove{}, false
	}
	ch, err := strconv.Atoi(fields[0])
	if err != nil {
		return ParamMove{}, false
	}
	pm := ParamMove{Channel: ch}
	for _, f := range fields[1:] {
		at := strings.IndexByte(f, '@')
		if at < 0 {
			continue
		}
		paramID, err := strconv.Atoi(f[:at])
		if err != nil {
			continue
		}
		val, err := parseLevel(f[at+1:])
		if err != nil {
			continue
		}
		pm.Values = append(pm.Values, ParamValue{ParamID: paramID, Value: val})
	}
	return pm, true
}

func (p *parser) parsePalette(target *[]Palette) error {
	line := p.cur()
	if len(line.Fields) < 1 {
		return &ParseError{Lineno: line.Lineno, Msg: "palette needs number"}
	}
	pal := Palette{Number: line.Fields[0]}
	p.advance()
	for p.cur() != nil && p.cur().Indent > 0 {
		sub := p.cur()
		switch sub.Directive {
		case "Text":
			pal.Label = strings.Join(sub.Fields, " ")
		case "$$ChanMove":
			pal.ChanMoves = append(pal.ChanMoves, parseChanMoves(sub.Fields)...)
		case "$$Param":
			if pm, ok := parseParamMove(sub.Fields); ok {
				pal.ParamMoves = append(pal.ParamMoves, pm)
			}
		case "$$UText":
			if s, ok := decodeUText(sub.Fields); ok {
				pal.UnicodeText = &s
			} else {
				p.warn.Add(WarnUTextDecode, SeverityWarn,
					"failed to decode $$UText",
					map[string]string{"line": strconv.Itoa(sub.Lineno)})
			}
		}
		p.advance()
	}
	*target = append(*target, pal)
	return nil
}

func (p *parser) parseGroup() error {
	line := p.cur()
	if len(line.Fields) < 1 {
		return &ParseError{Lineno: line.Lineno, Msg: "$Group needs number"}
	}
	g := Group{Number: line.Fields[0]}
	p.advance()
	for p.cur() != nil && p.cur().Indent > 0 {
		sub := p.cur()
		switch sub.Directive {
		case "Text":
			g.Label = strings.Join(sub.Fields, " ")
		case "$$UText":
			if s, ok := decodeUText(sub.Fields); ok {
				g.UnicodeText = &s
			} else {
				p.warn.Add(WarnUTextDecode, SeverityWarn,
					"failed to decode $$UText",
					map[string]string{"line": strconv.Itoa(sub.Lineno)})
			}
		default:
			tokens := append([]string{sub.Directive}, sub.Fields...)
			g.Channels = append(g.Channels, expandChannelTokens(tokens)...)
		}
		p.advance()
	}
	p.show.Groups = append(p.show.Groups, g)
	return nil
}

func expandChannelTokens(tokens []string) []int {
	var out []int
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if strings.EqualFold(t, "Thru") && len(out) > 0 && i+1 < len(tokens) {
			endN, err := strconv.Atoi(tokens[i+1])
			if err == nil {
				start := out[len(out)-1] + 1
				for v := start; v <= endN; v++ {
					out = append(out, v)
				}
				i++
				continue
			}
		}
		if v, err := strconv.Atoi(t); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// ErrAddressUnpatched indicates the EOS patch entry has no DMX address (raw "0").
// Callers may choose to skip such entries rather than fail the whole import.
var ErrAddressUnpatched = errors.New("eos: address is 0 (unpatched)")

// NormalizeAddress converts an EOS patch address string to a (universe, address) tuple.
// Accepts flat absolute ("1024"), dotted ("2.512"), and slashed ("3/100") forms.
// Universes are 1-based, addresses are 1-based and 1..512. Flat values are
// allowed to span multiple universes (e.g. flat=1024 → universe 2, address 512;
// flat=1025 → universe 3, address 1).
// Returns ErrAddressUnpatched for the literal value "0", which EOS uses to mark
// channels that exist in the show but are not patched to any DMX output.
func NormalizeAddress(raw string) (universe int, address int, err error) {
	for _, sep := range []byte{'.', '/'} {
		if i := strings.IndexByte(raw, sep); i >= 0 {
			u, e1 := strconv.Atoi(raw[:i])
			a, e2 := strconv.Atoi(raw[i+1:])
			if e1 != nil || e2 != nil || u < 1 || a < 1 || a > 512 {
				return 0, 0, fmt.Errorf("invalid eos address %q", raw)
			}
			return u, a, nil
		}
	}
	flat, err := strconv.Atoi(raw)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid eos address %q", raw)
	}
	if flat == 0 {
		return 0, 0, ErrAddressUnpatched
	}
	if flat < 0 {
		return 0, 0, fmt.Errorf("invalid eos address %q", raw)
	}
	universe = (flat-1)/512 + 1
	address = (flat-1)%512 + 1
	return universe, address, nil
}

func (p *parser) skipBlockWithWarning(code WarningCode, msg string) error {
	line := p.cur()
	p.warn.Add(code, SeverityInfo, msg,
		map[string]string{"line": strconv.Itoa(line.Lineno), "directive": line.Directive})
	p.advance()
	for p.cur() != nil && p.cur().Indent > 0 {
		p.advance()
	}
	return nil
}
