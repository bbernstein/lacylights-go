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
		if sub.Directive == "Text" {
			pe.Label = strings.Join(sub.Fields, " ")
		}
		p.advance()
	}
	p.show.Patch = append(p.show.Patch, pe)
	return nil
}
