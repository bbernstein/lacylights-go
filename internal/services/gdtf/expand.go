package gdtf

import (
	"fmt"
	"strconv"
	"strings"
)

// Expansion limits.
const (
	// maxBreakChannels is the largest offset accepted within one DMX break
	// (a break is one patch footprint in one universe).
	maxBreakChannels = 512
	// maxGeometryDepth bounds the geometry tree walk.
	maxGeometryDepth = 64
	// maxModeInstances bounds the channel instances one mode may expand to.
	// Geometry references multiply template channels, so a small
	// description could otherwise allocate millions of instances.
	maxModeInstances = 8192
	// maxDMXUniverse bounds the universe of a "universe.address" DMXOffset.
	maxDMXUniverse = 1 << 15
)

// channelInstance is one DMX channel as patched: a DMXChannel of the mode,
// possibly instantiated once per GeometryReference.
type channelInstance struct {
	ch       *xmlDMXChannel
	geometry string // geometry the instance controls: the reference name, or the channel's Geometry
	dmxBreak int
	offsets  []int // 1-based addresses within the break, most significant byte first
}

// geometryIndex holds the top-level geometries of a fixture type.
type geometryIndex struct {
	top   map[string]*xmlGeometry
	order []*xmlGeometry
}

func newGeometryIndex(g *xmlGeometries) *geometryIndex {
	idx := &geometryIndex{top: map[string]*xmlGeometry{}}
	for i := range g.Children {
		n := &g.Children[i]
		if n.isBreak() {
			continue
		}
		if _, dup := idx.top[n.Name]; !dup {
			idx.top[n.Name] = n
		}
		idx.order = append(idx.order, n)
	}
	return idx
}

// modeExpander instantiates the DMX channels of one mode by walking the
// geometry tree from the mode's root geometry.
type modeExpander struct {
	geoms      *geometryIndex
	mode       *xmlDMXMode
	byGeometry map[string][]int // geometry name -> indexes into mode.Channels
	reached    []bool
	instances  []channelInstance
	warnings   []string
	warned     map[string]bool
	tooMany    bool // instance budget exceeded; expansion stops
}

// expandMode returns the patched channel instances of a mode grouped by
// DMX break. Virtual channels (Offset "None") are skipped.
func expandMode(geoms *geometryIndex, mode *xmlDMXMode) (map[int][]channelInstance, []string, error) {
	e := &modeExpander{
		geoms:      geoms,
		mode:       mode,
		byGeometry: map[string][]int{},
		reached:    make([]bool, len(mode.Channels)),
		warned:     map[string]bool{},
	}
	for i := range mode.Channels {
		g := mode.Channels[i].Geometry
		e.byGeometry[g] = append(e.byGeometry[g], i)
	}

	root := geoms.top[mode.Geometry]
	if root == nil {
		if len(geoms.order) != 1 {
			return nil, nil, fmt.Errorf("mode %q: geometry %q is not a top-level geometry", mode.Name, mode.Geometry)
		}
		root = geoms.order[0]
		e.warn(fmt.Sprintf("mode %q: geometry %q not found; using %q", mode.Name, mode.Geometry, root.Name))
	}
	e.walk(root, 0)
	if e.tooMany {
		return nil, nil, fmt.Errorf("mode %q expands to more than %d DMX channels", mode.Name, maxModeInstances)
	}

	// Channels outside the mode's geometry tree are invalid GDTF, but their
	// offsets are explicit, so use them as declared rather than lose them.
	for i := range mode.Channels {
		if e.reached[i] {
			continue
		}
		ch := &mode.Channels[i]
		if isVirtual(ch) {
			continue
		}
		e.warn(fmt.Sprintf("mode %q: channel %s uses geometry %q outside the mode geometry; used at its declared offset",
			mode.Name, channelName(ch), ch.Geometry))
		e.declared(ch)
	}

	byBreak := map[int][]channelInstance{}
	for _, inst := range e.instances {
		byBreak[inst.dmxBreak] = append(byBreak[inst.dmxBreak], inst)
	}
	return byBreak, e.warnings, nil
}

func (e *modeExpander) warn(msg string) {
	if !e.warned[msg] {
		e.warned[msg] = true
		e.warnings = append(e.warnings, msg)
	}
}

func (e *modeExpander) walk(node *xmlGeometry, depth int) {
	if e.tooMany || depth > maxGeometryDepth || node.isBreak() {
		return
	}
	if node.isReference() {
		e.reference(node)
		return
	}
	for _, i := range e.byGeometry[node.Name] {
		e.reached[i] = true
		e.declared(&e.mode.Channels[i])
	}
	for i := range node.Children {
		e.walk(&node.Children[i], depth+1)
	}
}

// reference instantiates the channels of a referenced top-level geometry
// (and of the plain geometries below it) once for this reference.
func (e *modeExpander) reference(ref *xmlGeometry) {
	tmpl := e.geoms.top[ref.Geometry]
	if tmpl == nil {
		e.warn(fmt.Sprintf("mode %q: geometry reference %q points at unknown geometry %q", e.mode.Name, ref.Name, ref.Geometry))
		return
	}
	breaks := referenceBreaks(ref)
	var visit func(n *xmlGeometry, depth int)
	visit = func(n *xmlGeometry, depth int) {
		if e.tooMany || depth > maxGeometryDepth || n.isBreak() || n.isReference() {
			return
		}
		for _, i := range e.byGeometry[n.Name] {
			e.reached[i] = true
			e.instantiate(&e.mode.Channels[i], ref, breaks)
		}
		for i := range n.Children {
			visit(&n.Children[i], depth+1)
		}
	}
	visit(tmpl, 0)
}

// refBreak is a <Break> of a GeometryReference.
type refBreak struct {
	dmxBreak  int
	dmxOffset int
}

func referenceBreaks(ref *xmlGeometry) []refBreak {
	var out []refBreak
	for i := range ref.Children {
		c := &ref.Children[i]
		if !c.isBreak() {
			continue
		}
		b := refBreak{dmxBreak: 1, dmxOffset: 1}
		if n, err := strconv.Atoi(strings.TrimSpace(c.DMXBreak)); err == nil && n >= 1 {
			b.dmxBreak = n
		}
		if n, ok := parseDMXAddress(c.DMXOffset); ok {
			b.dmxOffset = n
		}
		out = append(out, b)
	}
	return out
}

// parseDMXAddress parses a DMXAddress: an absolute address, or
// "universe.address".
func parseDMXAddress(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if u, a, ok := strings.Cut(s, "."); ok {
		un, err1 := strconv.Atoi(u)
		an, err2 := strconv.Atoi(a)
		if err1 != nil || err2 != nil || un < 1 || an < 1 || un > maxDMXUniverse || an > maxBreakChannels {
			return 0, false
		}
		return (un-1)*512 + an, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// declared adds a channel at its declared break and offsets.
func (e *modeExpander) declared(ch *xmlDMXChannel) {
	brk, overwrite, ok := parseBreak(ch.DMXBreak)
	if !ok {
		e.warn(fmt.Sprintf("mode %q: channel %s has invalid DMXBreak %q; using break 1", e.mode.Name, channelName(ch), ch.DMXBreak))
	}
	if overwrite {
		// Only valid on channels of referenced geometries.
		e.warn(fmt.Sprintf("mode %q: channel %s has DMXBreak \"Overwrite\" outside a geometry reference; using break 1", e.mode.Name, channelName(ch)))
	}
	e.add(ch, ch.Geometry, brk, 0)
}

// instantiate adds one instance of a template channel for a reference. The
// instance offset is Offset + (Break.DMXOffset - 1), using the Break for the
// channel's DMXBreak, or the last Break when the channel's break is
// "Overwrite".
func (e *modeExpander) instantiate(ch *xmlDMXChannel, ref *xmlGeometry, breaks []refBreak) {
	brk, overwrite, ok := parseBreak(ch.DMXBreak)
	if !ok {
		e.warn(fmt.Sprintf("mode %q: channel %s has invalid DMXBreak %q; using break 1", e.mode.Name, channelName(ch), ch.DMXBreak))
	}
	shift := 0
	switch {
	case overwrite && len(breaks) > 0:
		last := breaks[len(breaks)-1]
		brk, shift = last.dmxBreak, last.dmxOffset-1
	case overwrite:
		brk = 1
	default:
		found := false
		for _, b := range breaks {
			if b.dmxBreak == brk {
				shift, found = b.dmxOffset-1, true
				break
			}
		}
		if !found && !isVirtual(ch) {
			e.warn(fmt.Sprintf("mode %q: geometry reference %q has no Break for DMX break %d; no offset applied", e.mode.Name, ref.Name, brk))
		}
	}
	e.add(ch, ref.Name, brk, shift)
}

func (e *modeExpander) add(ch *xmlDMXChannel, geometry string, brk, shift int) {
	offsets, err := parseOffsets(ch.Offset)
	if err != nil {
		e.warn(fmt.Sprintf("mode %q: channel %s: %v; channel skipped", e.mode.Name, channelName(ch), err))
		return
	}
	if offsets == nil {
		return // virtual channel
	}
	if len(e.instances) >= maxModeInstances {
		e.tooMany = true
		return
	}
	shifted := make([]int, len(offsets))
	for i, o := range offsets {
		// Compare before adding so a huge Offset or DMXOffset cannot
		// overflow into an accepted address. shift is never negative.
		if o > maxBreakChannels-shift {
			e.warn(fmt.Sprintf("mode %q: channel %s on %q is beyond address %d; channel skipped", e.mode.Name, channelName(ch), geometry, maxBreakChannels))
			return
		}
		shifted[i] = o + shift
	}
	e.instances = append(e.instances, channelInstance{ch: ch, geometry: geometry, dmxBreak: brk, offsets: shifted})
}

// parseBreak parses DMXChannel@DMXBreak: an integer (default 1) or
// "Overwrite". ok is false when the value is invalid (break 1 is used).
func parseBreak(s string) (brk int, overwrite, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 1, false, true
	}
	if strings.EqualFold(s, "overwrite") {
		return 1, true, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1, false, false
	}
	return n, false, true
}

// parseOffsets parses DMXChannel@Offset: "1", "2,3" (coarse first), ...
// A missing value or "None" marks a virtual channel and yields nil.
func parseOffsets(s *string) ([]int, error) {
	if s == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*s)
	if v == "" || strings.EqualFold(v, "none") {
		return nil, nil
	}
	parts := strings.Split(v, ",")
	if len(parts) > maxValueBytes {
		return nil, fmt.Errorf("offset %q has more than %d bytes", v, maxValueBytes)
	}
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid offset %q", v)
		}
		out[i] = n
	}
	return out, nil
}

func isVirtual(ch *xmlDMXChannel) bool {
	offsets, err := parseOffsets(ch.Offset)
	return err == nil && offsets == nil
}

// channelName is the GDTF DMX channel name: "<Geometry>_<Attribute>" of
// the first logical channel.
func channelName(ch *xmlDMXChannel) string {
	return ch.Geometry + "_" + firstLogical(ch).Attribute
}

// firstLogical returns the first logical channel; a channel without one is
// treated as NoFeature.
func firstLogical(ch *xmlDMXChannel) *xmlLogicalChannel {
	if len(ch.LogicalChannels) == 0 {
		return &xmlLogicalChannel{Attribute: "NoFeature"}
	}
	return &ch.LogicalChannels[0]
}
