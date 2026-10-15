package gdtf

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bbernstein/lacylights-go/internal/services/ofl"
)

// placeholderName names the channel used for unused addresses (offset gaps
// and NoFeature filler channels).
const placeholderName = "No Function"

// plannedChannel is a channel definition to create (without IDs).
type plannedChannel struct {
	Name         string
	Type         string
	DefaultValue int
	FadeBehavior string
	IsDiscrete   bool

	fine        bool // byte 2..n of a multi-byte channel
	placeholder bool // unused address
}

// plannedMode is a mode to create: channelIdx[offset] indexes plan.Channels.
type plannedMode struct {
	Name       string
	channelIdx []int
}

// importPlan is the LacyLights view of one GDTF fixture type.
type importPlan struct {
	Manufacturer  string
	Model         string
	Type          string
	SourceID      string
	SourceVersion string
	Channels      []plannedChannel
	Modes         []plannedMode
	Warnings      []string
}

// buildPlan maps a parsed description to an import plan. versionWarnings
// are carried into the plan's warnings.
func buildPlan(doc *xmlGDTF, versionWarnings []string) (*importPlan, error) {
	ft := &doc.FixtureType
	b := &planBuilder{
		attrs:    map[string]*xmlAttribute{},
		byKey:    map[string]int{},
		warnings: append([]string(nil), versionWarnings...),
		modeName: map[string]bool{},
	}
	for i := range ft.Attributes {
		a := &ft.Attributes[i]
		key := strings.ToLower(a.Name)
		if _, dup := b.attrs[key]; !dup {
			b.attrs[key] = a
		}
	}
	geoms := newGeometryIndex(&ft.Geometries)
	for i := range ft.DMXModes {
		if err := b.addMode(geoms, &ft.DMXModes[i], i); err != nil {
			return nil, err
		}
	}
	if len(b.modes) == 0 {
		return nil, fmt.Errorf("%w: no DMX mode has patchable channels", ErrInvalidDescription)
	}

	plan := &importPlan{
		Manufacturer:  strings.TrimSpace(ft.Manufacturer),
		Model:         modelName(ft),
		SourceID:      strings.ToUpper(strings.TrimSpace(ft.FixtureTypeID)),
		SourceVersion: sourceVersion(doc),
		Channels:      b.channels,
		Modes:         b.modes,
		Warnings:      b.warnings,
	}
	var types []string
	for _, ch := range b.channels {
		if !ch.fine && !ch.placeholder {
			types = append(types, ch.Type)
		}
	}
	plan.Type = fixtureTypeFor(types)
	return plan, nil
}

// modelName is the fixture type's LongName, falling back to Name.
func modelName(ft *xmlFixtureType) string {
	if s := strings.TrimSpace(ft.LongName); s != "" {
		return s
	}
	return strings.TrimSpace(ft.Name)
}

// sourceVersion is the DataVersion plus the last revision's date and text,
// e.g. "1.2 (2024-03-01T10:00:00 Added mode)".
func sourceVersion(doc *xmlGDTF) string {
	v := strings.TrimSpace(doc.DataVersion)
	revs := doc.FixtureType.Revisions
	if len(revs) == 0 {
		return v
	}
	last := revs[len(revs)-1]
	var parts []string
	for _, s := range []string{last.Date, last.Text} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return v
	}
	return v + " (" + strings.Join(parts, " ") + ")"
}

type planBuilder struct {
	attrs          map[string]*xmlAttribute
	channels       []plannedChannel
	byKey          map[string]int
	placeholderIdx int // index+1 of the shared placeholder; 0 = none yet
	modes          []plannedMode
	modeName       map[string]bool
	warnings       []string
}

func (b *planBuilder) warn(format string, args ...any) {
	b.warnings = append(b.warnings, fmt.Sprintf(format, args...))
}

// addMode adds one LacyLights mode per DMX break of a GDTF mode.
func (b *planBuilder) addMode(geoms *geometryIndex, mode *xmlDMXMode, index int) error {
	byBreak, warns, err := expandMode(geoms, mode)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidDescription, err)
	}
	b.warnings = append(b.warnings, warns...)

	name := strings.TrimSpace(mode.Name)
	if name == "" {
		name = fmt.Sprintf("Mode %d", index+1)
	}
	if len(byBreak) == 0 {
		b.warn("mode %q has no patchable channels; skipped", name)
		return nil
	}
	breaks := make([]int, 0, len(byBreak))
	for brk := range byBreak {
		breaks = append(breaks, brk)
	}
	sort.Ints(breaks)
	if len(breaks) > 1 {
		b.warn("mode %q uses %d DMX breaks; imported as one mode per break", name, len(breaks))
	}
	for _, brk := range breaks {
		modeName := name
		if len(breaks) > 1 {
			modeName = fmt.Sprintf("%s (Break %d)", name, brk)
		}
		b.addBreak(b.uniqueModeName(modeName), byBreak[brk])
	}
	return nil
}

func (b *planBuilder) uniqueModeName(name string) string {
	out := name
	for i := 2; b.modeName[out]; i++ {
		out = fmt.Sprintf("%s (%d)", name, i)
	}
	b.modeName[out] = true
	return out
}

// addBreak lays out one break's channel instances as a mode.
func (b *planBuilder) addBreak(modeName string, instances []channelInstance) {
	sort.SliceStable(instances, func(i, j int) bool { return instances[i].offsets[0] < instances[j].offsets[0] })

	// Attributes that occur more than once get a geometry prefix.
	attrCount := map[string]int{}
	size := 0
	for _, inst := range instances {
		attrCount[strings.ToLower(firstLogical(inst.ch).Attribute)]++
		for _, o := range inst.offsets {
			size = max(size, o)
		}
	}

	slots := make([]int, size)
	for i := range slots {
		slots[i] = -1
	}
	usedNames := map[string]bool{}
	for _, inst := range instances {
		lc := firstLogical(inst.ch)
		attrKey := strings.ToLower(lc.Attribute)
		if normalizeAttribute(lc.Attribute) == "nofeature" {
			for _, o := range inst.offsets {
				b.place(modeName, slots, o, b.placeholder(), inst)
			}
			continue
		}
		base := b.displayName(lc.Attribute)
		if attrCount[attrKey] > 1 && inst.geometry != "" {
			base = inst.geometry + " " + base
		}
		base = uniqueName(base, usedNames)

		coarse := b.channelFor(inst, lc)
		full := b.defaultValue(modeName, inst, len(inst.offsets))
		byteVals := splitBytes(full, len(inst.offsets))
		for i, o := range inst.offsets {
			ch := coarse
			ch.Name = fineName(base, i)
			ch.DefaultValue = byteVals[i]
			if i > 0 {
				ch.fine = true
				ch.IsDiscrete = false // fine channels are continuous
			}
			b.place(modeName, slots, o, b.add(ch), inst)
		}
	}
	for i, idx := range slots {
		if idx < 0 {
			slots[i] = b.placeholder()
		}
	}
	b.modes = append(b.modes, plannedMode{Name: modeName, channelIdx: slots})
}

// place assigns a channel to a 1-based offset, keeping the first channel
// when two claim the same address.
func (b *planBuilder) place(modeName string, slots []int, offset, idx int, inst channelInstance) {
	if slots[offset-1] >= 0 {
		b.warn("mode %q: address %d is used by more than one channel; %s on %q ignored there",
			modeName, offset, channelName(inst.ch), inst.geometry)
		return
	}
	slots[offset-1] = idx
}

// fineName names byte i of a channel: "Pan", "Pan Fine", "Pan Fine 2", ...
func fineName(base string, i int) string {
	switch i {
	case 0:
		return base
	case 1:
		return base + " Fine"
	}
	return fmt.Sprintf("%s Fine %d", base, i)
}

func uniqueName(name string, used map[string]bool) string {
	out := name
	for i := 2; used[out]; i++ {
		out = fmt.Sprintf("%s %d", name, i)
	}
	used[out] = true
	return out
}

// displayName is a readable channel name for an attribute; see
// channelDisplayName.
func (b *planBuilder) displayName(attr string) string {
	if a := b.attrs[strings.ToLower(attr)]; a != nil {
		name := a.Name
		if strings.TrimSpace(name) == "" {
			name = attr
		}
		return channelDisplayName(name, a.Pretty)
	}
	return channelDisplayName(attr, "")
}

// channelFor returns the coarse channel of an instance without name or
// default: type, discreteness and fade behaviour.
func (b *planBuilder) channelFor(inst channelInstance, lc *xmlLogicalChannel) plannedChannel {
	feature := ""
	if a := b.attrs[strings.ToLower(lc.Attribute)]; a != nil {
		feature = a.Feature
	}
	ch := plannedChannel{
		Type:       classifyAttribute(lc.Attribute, feature),
		IsDiscrete: isDiscrete(lc),
	}
	if isSnap(lc.Snap) {
		ch.FadeBehavior = "SNAP"
	} else {
		ch.FadeBehavior = ofl.MapFadeBehavior(ch.Type, ch.IsDiscrete)
	}
	return ch
}

func isSnap(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "on":
		return true
	}
	return false
}

// isDiscrete reports whether a logical channel has more than one channel
// function in effect at once (ModeMaster-gated alternatives are counted per
// mode range), or wheel-slot channel sets.
func isDiscrete(lc *xmlLogicalChannel) bool {
	ungated := 0
	groups := map[string]int{}
	largest := 0
	for i := range lc.Functions {
		cf := &lc.Functions[i]
		for _, set := range cf.Sets {
			if s := strings.TrimSpace(set.WheelSlotIndex); s != "" && s != "0" {
				return true
			}
		}
		if strings.TrimSpace(cf.ModeMaster) == "" {
			ungated++
			continue
		}
		key := cf.ModeMaster + "|" + cf.ModeFrom + "|" + cf.ModeTo
		groups[key]++
		largest = max(largest, groups[key])
	}
	if ungated > 0 {
		return ungated > 1
	}
	return largest > 1
}

// defaultValue returns the instance's default at its full resolution
// (n bytes): the InitialFunction's Default, else the legacy
// DMXChannel@Default, else the function's DMXFrom, else 0.
func (b *planBuilder) defaultValue(modeName string, inst channelInstance, n int) uint64 {
	ch := inst.ch
	cf := initialFunction(ch)
	candidates := []string{ch.Default}
	if cf != nil {
		candidates = []string{cf.Default, ch.Default, cf.DMXFrom}
	}
	for _, s := range candidates {
		v, ok, err := parseDMXValue(s)
		if err != nil {
			b.warn("mode %q: channel %s: %v; ignored", modeName, channelName(ch), err)
			continue
		}
		if ok {
			return v.to(n)
		}
	}
	return 0
}

// initialFunction resolves DMXChannel@InitialFunction, a link
// "<DMXChannel>.<LogicalChannel>.<ChannelFunction>". The default is the
// first channel function of the first logical channel.
func initialFunction(ch *xmlDMXChannel) *xmlChannelFunction {
	if link := strings.TrimSpace(ch.InitialFunction); link != "" {
		for li := range ch.LogicalChannels {
			lc := &ch.LogicalChannels[li]
			for fi := range lc.Functions {
				suffix := lc.Attribute + "." + functionName(lc, fi)
				if link == suffix || strings.HasSuffix(link, "."+suffix) {
					return &lc.Functions[fi]
				}
			}
		}
	}
	for li := range ch.LogicalChannels {
		if lc := &ch.LogicalChannels[li]; len(lc.Functions) > 0 {
			return &lc.Functions[0]
		}
	}
	return nil
}

// functionName is a channel function's Name, defaulting to its attribute
// and 1-based number ("Dimmer 1").
func functionName(lc *xmlLogicalChannel, i int) string {
	cf := &lc.Functions[i]
	if cf.Name != "" {
		return cf.Name
	}
	attr := cf.Attribute
	if attr == "" {
		attr = lc.Attribute
	}
	return fmt.Sprintf("%s %d", attr, i+1)
}

// add returns the index of an equal channel already planned (same name,
// type, default, fade behaviour and discreteness), adding it if new.
func (b *planBuilder) add(ch plannedChannel) int {
	key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%t", ch.Name, ch.Type, ch.DefaultValue, ch.FadeBehavior, ch.IsDiscrete)
	if idx, ok := b.byKey[key]; ok {
		return idx
	}
	b.channels = append(b.channels, ch)
	idx := len(b.channels) - 1
	b.byKey[key] = idx
	return idx
}

// placeholder returns the shared "No Function" channel.
func (b *planBuilder) placeholder() int {
	if b.placeholderIdx == 0 {
		b.channels = append(b.channels, plannedChannel{
			Name:         placeholderName,
			Type:         typeOther,
			FadeBehavior: "FADE",
			placeholder:  true,
		})
		b.placeholderIdx = len(b.channels)
	}
	return b.placeholderIdx - 1
}
