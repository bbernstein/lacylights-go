package importeos

import (
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
	p := &parser{lines: lines, show: &Show{}, warn: &Collector{}}
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
	return nil
}

func parsePersChan(line *Line) (PersChannel, bool) {
	// Fields: paramID size offset (offset16) home (flags)
	if len(line.Fields) < 4 {
		return PersChannel{}, false
	}
	paramID, err := strconv.Atoi(line.Fields[0])
	if err != nil {
		return PersChannel{}, false
	}
	size, _ := strconv.Atoi(line.Fields[1])
	offset, _ := strconv.Atoi(line.Fields[2])
	pc := PersChannel{ParamID: paramID, Size: size, Offset: offset}
	idx := 3
	if size == 2 && len(line.Fields) > idx {
		pc.Offset16, _ = strconv.Atoi(line.Fields[idx])
		idx++
	}
	if idx < len(line.Fields) {
		pc.HomeValue, _ = strconv.Atoi(line.Fields[idx])
		idx++
	}
	if idx < len(line.Fields) {
		pc.Flags = line.Fields[idx]
	}
	return pc, true
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
	persID, err := strconv.Atoi(line.Fields[2])
	if err != nil {
		return &ParseError{Lineno: line.Lineno, Msg: "$Patch personality not an integer"}
	}
	pe := PatchEntry{Channel: channel, AddressRaw: line.Fields[1], PersonalityID: persID}
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

// NormalizeAddress converts an EOS patch address string to a (universe, address) tuple.
// Accepts flat absolute ("1024"), dotted ("2.512"), and slashed ("3/100") forms.
// Universes are 1-based, addresses are 1-based and 1..512.
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
	if err != nil || flat < 1 {
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
