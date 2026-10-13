package ofl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// This file turns an OFL fixture into an importPlan: an ordered list of
// channel definitions plus, for each mode, the channel used at every DMX
// offset. It implements the parts of the OFL fixture format
// (https://github.com/OpenLightingProject/open-fixture-library/blob/master/docs/fixture-format.md)
// that affect which channel sits at which DMX offset:
//
//   - fine channel aliases ("Dimmer fine")
//   - switching channel aliases (capability switchChannels)
//   - null mode entries (unused DMX slots)
//   - matrix template channels ("Red $pixelKey") referenced directly or
//     through matrix insert blocks ({"insert": "matrixChannels", ...})
//
// After insert blocks are expanded, every resulting mode channel (including
// null slots) occupies exactly one DMX offset, so offsets match the
// fixture's documented layout.

const (
	// maxMatrixPixels bounds pixelCount so a malformed fixture cannot make
	// the importer allocate unbounded memory. The largest OFL matrix has a
	// few hundred pixels.
	maxMatrixPixels = 65536
	// maxModeChannels is the most channels a mode can have: one DMX
	// universe. Insert blocks are checked against the remaining budget
	// before they are expanded.
	maxModeChannels = 512

	pixelKeyVar = "$pixelKey"
	// unusedChannelName names the placeholder channel used for null mode
	// entries and switching aliases that switch to no channel.
	unusedChannelName = "No function"
)

// planMode is a mode whose channels have been resolved to plan channel
// indexes; channelIdx[i] is the channel at DMX offset i.
type planMode struct {
	Name       string
	ShortName  string
	channelIdx []int
}

// importPlan is the database-ready form of an OFL fixture.
type importPlan struct {
	Channels []ChannelDefinition
	Modes    []planMode
}

// parseOFLFixture unmarshals OFL JSON and records key order information
// that Go maps lose.
func parseOFLFixture(data string) (*OFLFixture, error) {
	var f OFLFixture
	if err := json.Unmarshal([]byte(data), &f); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidJSON, err)
	}
	var order struct {
		AvailableChannels json.RawMessage `json:"availableChannels"`
		Matrix            *struct {
			PixelGroups json.RawMessage `json:"pixelGroups"`
		} `json:"matrix"`
	}
	if err := json.Unmarshal([]byte(data), &order); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidJSON, err)
	}
	f.availableChannelOrder = orderedObjectKeys(order.AvailableChannels)
	if f.Matrix != nil && order.Matrix != nil {
		f.Matrix.pixelGroupOrder = orderedObjectKeys(order.Matrix.PixelGroups)
	}
	return &f, nil
}

// orderedObjectKeys returns the top-level keys of a JSON object in document
// order. It returns nil for anything that is not an object.
func orderedObjectKeys(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return keys
		}
		key, ok := tok.(string)
		if !ok {
			return keys
		}
		keys = append(keys, key)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return keys
		}
	}
	return keys
}

// validateOFLFixture validates required OFL fixture fields. Redirect files
// and fixtures without channels return sentinel errors so bulk importers
// can count them as skipped rather than failed.
func validateOFLFixture(fixture *OFLFixture) error {
	if fixture.Name == "" {
		return fmt.Errorf("OFL fixture must have a \"name\" field")
	}

	if fixture.RedirectTo != "" {
		return fmt.Errorf("%w to %s", ErrFixtureRedirect, fixture.RedirectTo)
	}

	if len(fixture.Categories) == 0 {
		return fmt.Errorf("OFL fixture must have a \"categories\" array with at least one category")
	}

	if len(fixture.AvailableChannels) == 0 && len(fixture.TemplateChannels) == 0 {
		return fmt.Errorf("%w: \"availableChannels\" and \"templateChannels\" are empty", ErrNoChannels)
	}

	if len(fixture.Modes) == 0 {
		return fmt.Errorf("OFL fixture must have a \"modes\" array with at least one mode")
	}

	return nil
}

// planBuilder resolves channel keys to plan channel indexes, creating
// channel definitions on demand for aliases and template channels.
type planBuilder struct {
	fixture  *OFLFixture
	channels []ChannelDefinition
	byKey    map[string]int
	// switching maps a switching alias to its default target key; a nil
	// target means the alias defaults to an unused channel.
	switching map[string]*string
	templates []templatePattern
	pixelSet  map[string]bool // pixel keys + group keys; nil if no matrix
	resolving map[string]bool // guards against alias cycles
	unusedIdx int             // placeholder for null entries; -1 until needed
}

type templatePattern struct {
	re      *regexp.Regexp
	literal int // length of the non-variable part, used to rank matches
	tmplKey string
	fineIdx int // 0 = coarse channel, n = n-th fine alias
	// isSwitch marks a templated switching alias; switchTarget is its
	// default target key (still containing $pixelKey), nil = unused.
	isSwitch     bool
	switchTarget *string
}

// compilePattern turns a key containing $pixelKey into a matcher.
func compilePattern(n string) (*regexp.Regexp, int) {
	parts := strings.Split(n, pixelKeyVar)
	quoted := make([]string, len(parts))
	for j, p := range parts {
		quoted[j] = regexp.QuoteMeta(p)
	}
	re := regexp.MustCompile("^" + strings.Join(quoted, "(.+?)") + "$")
	return re, len(n) - len(pixelKeyVar)*(len(parts)-1)
}

// buildImportPlan validates the fixture and resolves every mode.
func buildImportPlan(f *OFLFixture) (*importPlan, error) {
	if err := validateOFLFixture(f); err != nil {
		return nil, err
	}
	b := &planBuilder{
		fixture:   f,
		byKey:     map[string]int{},
		switching: map[string]*string{},
		resolving: map[string]bool{},
		unusedIdx: -1,
	}
	b.addAvailableChannels()
	b.collectSwitchingAliases()
	b.compileTemplates()
	if f.Matrix != nil {
		if c := f.Matrix.pixelCount(); !validPixelCount(c) {
			return nil, fmt.Errorf("%w: matrix pixelCount %v exceeds %d pixels", ErrUnsupportedModeChannel, c, maxMatrixPixels)
		}
		b.pixelSet = map[string]bool{}
		for _, k := range f.Matrix.pixelKeysABC() {
			b.pixelSet[k] = true
		}
		for _, k := range f.Matrix.pixelGroupKeys() {
			b.pixelSet[k] = true
		}
	}

	plan := &importPlan{}
	for _, m := range f.Modes {
		keys, err := expandModeChannels(m, f.Matrix)
		if err != nil {
			return nil, err
		}
		pm := planMode{Name: m.Name, ShortName: m.ShortName}
		for _, key := range keys {
			idx, err := b.resolve(key)
			if err != nil {
				return nil, fmt.Errorf("mode %q: %w", m.Name, err)
			}
			pm.channelIdx = append(pm.channelIdx, idx)
		}
		plan.Modes = append(plan.Modes, pm)
	}
	plan.Channels = b.channels
	return plan, nil
}

// addAvailableChannels adds every available channel and its fine aliases,
// in JSON order, so definition offsets are deterministic.
func (b *planBuilder) addAvailableChannels() {
	order := b.fixture.availableChannelOrder
	if len(order) != len(b.fixture.AvailableChannels) {
		// Fixture built in code rather than parsed: fall back to sorted keys.
		order = make([]string, 0, len(b.fixture.AvailableChannels))
		for k := range b.fixture.AvailableChannels {
			order = append(order, k)
		}
		sort.Strings(order)
	}
	for _, key := range order {
		ch := b.fixture.AvailableChannels[key]
		b.addChannel(key, ch)
		for i, alias := range ch.FineChannelAliases {
			b.addFine(alias, ch, i+1)
		}
	}
}

func (b *planBuilder) add(def ChannelDefinition) int {
	idx := b.addUnlisted(def)
	if _, exists := b.byKey[def.Name]; !exists {
		b.byKey[def.Name] = idx
	}
	return idx
}

// addUnlisted adds a channel that mode keys cannot resolve to by name
// (the shared placeholder for null slots).
func (b *planBuilder) addUnlisted(def ChannelDefinition) int {
	def.Offset = len(b.channels)
	b.channels = append(b.channels, def)
	return len(b.channels) - 1
}

func (b *planBuilder) addChannel(name string, ch OFLChannel) int {
	return b.add(channelDefFromOFL(name, ch))
}

// addFine adds the fineIdx-th fine channel (1 = first fine alias) of
// parent. Its default is the matching byte of the parent's defaultValue.
func (b *planBuilder) addFine(name string, parent OFLChannel, fineIdx int) int {
	coarse := channelDefFromOFL(name, parent)
	defaultByte := 0
	r := resolutionOf(parent)
	if v, ok := parseDMXValue(parent.DefaultValue, r.valueBytes); ok {
		defaultByte = r.byteAt(r.toFull(v), fineIdx)
	}
	return b.add(ChannelDefinition{
		Name:         name,
		Type:         coarse.Type,
		MinValue:     0,
		MaxValue:     255,
		DefaultValue: defaultByte,
		FadeBehavior: coarse.FadeBehavior,
		IsDiscrete:   false, // fine channels are always continuous
	})
}

// channelDefFromOFL converts one OFL channel to a channel definition
// (without offset), auto-detecting type and fade behavior. DMX values of
// multi-byte (coarse + fine) channels are reduced to the coarse byte.
func channelDefFromOFL(name string, ch OFLChannel) ChannelDefinition {
	var capability *OFLCapability
	if ch.Capability != nil {
		capability = ch.Capability
	} else if len(ch.Capabilities) > 0 {
		capability = &ch.Capabilities[0]
	}
	r := resolutionOf(ch)
	explicitDefault, hasDefault := parseDMXValue(ch.DefaultValue, r.valueBytes)
	if capability == nil {
		def := ChannelDefinition{
			Name: name, Type: "OTHER", MinValue: 0, MaxValue: 255,
			DefaultValue: 0, FadeBehavior: "FADE",
		}
		if hasDefault {
			def.DefaultValue = r.byteAt(r.toFull(explicitDefault), 0)
		}
		return def
	}
	isDiscrete := len(ch.Capabilities) > 1
	channelType := mapChannelType(capability)
	// dmxRange-derived values are in the value resolution; without a
	// dmxRange the helpers return 8-bit values that need no reduction.
	minVal, maxVal := getMinMaxValues(capability)
	defaultVal := getDefaultValue(capability)
	if capability.DMXRange != nil {
		minVal, maxVal = r.coarseRange(minVal, maxVal)
		defaultVal = r.coarse(defaultVal)
	}
	if hasDefault {
		defaultVal = r.byteAt(r.toFull(explicitDefault), 0)
	}
	return ChannelDefinition{
		Name:         name,
		Type:         channelType,
		MinValue:     minVal,
		MaxValue:     maxVal,
		DefaultValue: defaultVal,
		FadeBehavior: mapFadeBehavior(channelType, isDiscrete),
		IsDiscrete:   isDiscrete,
	}
}

// resolution describes how an OFL channel's DMX values map to bytes.
type resolution struct {
	channelBytes int // 1 + number of fine channel aliases
	valueBytes   int // resolution DMX values are written in
}

func resolutionOf(ch OFLChannel) resolution {
	r := resolution{channelBytes: 1 + len(ch.FineChannelAliases)}
	switch ch.DMXValueResolution {
	case "8bit":
		r.valueBytes = 1
	case "16bit":
		r.valueBytes = 2
	case "24bit":
		r.valueBytes = 3
	default:
		r.valueBytes = r.channelBytes
	}
	if r.valueBytes > r.channelBytes {
		r.valueBytes = r.channelBytes
	}
	return r
}

// toFull scales a value from the value resolution to the full channel
// resolution the way OFL's scaleDmxValue does: each added byte repeats the
// least-significant byte (an 8-bit 132 on a 16-bit channel is 0x8484).
func (r resolution) toFull(v int) int {
	lsb := v & 0xff
	for i := r.valueBytes; i < r.channelBytes; i++ {
		v = v<<8 | lsb
	}
	return v
}

// byteAt returns byte i (0 = coarse) of a full-resolution value.
func (r resolution) byteAt(full, i int) int {
	if i < 0 || i >= r.channelBytes {
		return 0
	}
	return (full >> (8 * (r.channelBytes - 1 - i))) & 0xff
}

// coarse reduces a value in the value resolution to its coarse byte.
func (r resolution) coarse(v int) int {
	return clampByte(v >> (8 * (r.valueBytes - 1)))
}

// coarseRange reduces a DMX range to the coarse byte like OFL's
// scaleDmxRangeIndividually: the end drops its fine bytes, and the start is
// rounded up when a dropped byte was non-zero (unless that would pass the
// end), so the stored range never widens.
func (r resolution) coarseRange(start, end int) (int, int) {
	end = r.coarse(end)
	for res := r.valueBytes; res > 1; res-- {
		dropped := start & 0xff
		start >>= 8
		if dropped > 0 && start < end {
			start++
		}
	}
	return clampByte(start), end
}

func clampByte(v int) int {
	return min(max(v, 0), 255)
}

// parseDMXValue parses an OFL DMX value: a number, or a percentage string
// ("50%") scaled to the given byte resolution.
func parseDMXValue(raw json.RawMessage, valueBytes int) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	maxVal := (1 << (8 * valueBytes)) - 1
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		return min(max(int(n), 0), maxVal), true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil || !strings.HasSuffix(s, "%") {
		return 0, false
	}
	pct, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "%")), 64)
	if err != nil {
		return 0, false
	}
	pct = min(max(pct, 0), 100)
	// Math.floor, matching the OFL reference implementation.
	return int(math.Floor(pct / 100 * float64(maxVal))), true
}

func unusedChannelDef(name string) ChannelDefinition {
	return ChannelDefinition{
		Name: name, Type: "OTHER", MinValue: 0, MaxValue: 255,
		DefaultValue: 0, FadeBehavior: "SNAP",
	}
}

// collectSwitchingAliases records, for each switching alias defined by an
// available channel, the channel it switches to by default.
func (b *planBuilder) collectSwitchingAliases() {
	keys := make([]string, 0, len(b.fixture.AvailableChannels))
	for k := range b.fixture.AvailableChannels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for alias, target := range switchDefaults(b.fixture.AvailableChannels[key]) {
			if _, seen := b.switching[alias]; !seen {
				b.switching[alias] = target
			}
		}
	}
}

// switchDefaults returns the default target of every switching alias a
// trigger channel defines: the target of the capability containing the
// channel's defaultValue, else of the first capability naming the alias.
// A nil target means the alias switches to an unused channel.
func switchDefaults(ch OFLChannel) map[string]*string {
	caps := ch.Capabilities
	if ch.Capability != nil {
		caps = []OFLCapability{*ch.Capability}
	}
	out := map[string]*string{}
	if def, ok := parseDMXValue(ch.DefaultValue, resolutionOf(ch).valueBytes); ok {
		for _, c := range caps {
			if c.DMXRange != nil && def >= c.DMXRange[0] && def <= c.DMXRange[1] {
				for alias, target := range c.SwitchChannels {
					out[alias] = target
				}
				break
			}
		}
	}
	for _, c := range caps {
		for alias, target := range c.SwitchChannels {
			if _, seen := out[alias]; !seen {
				out[alias] = target
			}
		}
	}
	return out
}

// compileTemplates builds a matcher for each template channel key and each
// of its fine aliases.
func (b *planBuilder) compileTemplates() {
	keys := make([]string, 0, len(b.fixture.TemplateChannels))
	for k := range b.fixture.TemplateChannels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ch := b.fixture.TemplateChannels[k]
		names := append([]string{k}, ch.FineChannelAliases...)
		for i, n := range names {
			if !strings.Contains(n, pixelKeyVar) {
				continue
			}
			re, literal := compilePattern(n)
			b.templates = append(b.templates, templatePattern{
				re: re, literal: literal, tmplKey: k, fineIdx: i,
			})
		}
		for alias, target := range switchDefaults(ch) {
			if !strings.Contains(alias, pixelKeyVar) {
				continue
			}
			re, literal := compilePattern(alias)
			b.templates = append(b.templates, templatePattern{
				re: re, literal: literal, tmplKey: k, isSwitch: true, switchTarget: target,
			})
		}
	}
}

// resolve returns the plan channel index for a mode channel key. A nil key
// (JSON null) is an unused DMX slot.
func (b *planBuilder) resolve(key *string) (int, error) {
	if key == nil {
		if b.unusedIdx < 0 {
			b.unusedIdx = b.addUnlisted(unusedChannelDef(unusedChannelName))
		}
		return b.unusedIdx, nil
	}
	k := *key
	if idx, ok := b.byKey[k]; ok {
		return idx, nil
	}
	if b.resolving[k] {
		return 0, fmt.Errorf("%w: %q (alias cycle)", ErrUnknownModeChannel, k)
	}
	b.resolving[k] = true
	defer delete(b.resolving, k)

	if target, ok := b.switching[k]; ok {
		return b.addSwitching(k, target)
	}
	if idx, ok, err := b.resolveTemplate(k); ok || err != nil {
		return idx, err
	}
	// Legacy fallback for "A / B" keys whose first part is a channel.
	if strings.Contains(k, " / ") {
		primary := strings.Split(k, " / ")[0]
		if idx, ok := b.byKey[primary]; ok {
			return idx, nil
		}
	}
	return 0, fmt.Errorf("%w: %q not found in availableChannels or templateChannels", ErrUnknownModeChannel, k)
}

// addSwitching adds a channel for switching alias k that mirrors its
// default target channel (or an unused placeholder for a nil target).
func (b *planBuilder) addSwitching(k string, target *string) (int, error) {
	if target == nil {
		return b.add(unusedChannelDef(k)), nil
	}
	tIdx, err := b.resolve(target)
	if err != nil {
		return 0, err
	}
	def := b.channels[tIdx]
	def.Name = k
	return b.add(def), nil
}

// resolveTemplate matches k against the template channel patterns and, if
// one matches, adds the resolved channel (and remembers it under k). OFL
// template keys contain $pixelKey once; for switching targets the first
// captured pixel key is substituted. When several patterns match, the one
// with the longest literal part wins (ties: first in sorted template order).
func (b *planBuilder) resolveTemplate(k string) (int, bool, error) {
	best := -1
	var bestPixel string
	for i, t := range b.templates {
		m := t.re.FindStringSubmatch(k)
		if m == nil {
			continue
		}
		valid := true
		for _, pk := range m[1:] {
			if b.pixelSet != nil && !b.pixelSet[pk] {
				valid = false
			}
		}
		if !valid {
			continue
		}
		if best < 0 || t.literal > b.templates[best].literal {
			best = i
			bestPixel = m[1]
		}
	}
	if best < 0 {
		return 0, false, nil
	}
	t := b.templates[best]
	tmpl := b.fixture.TemplateChannels[t.tmplKey]
	switch {
	case t.isSwitch:
		var target *string
		if t.switchTarget != nil {
			resolved := strings.ReplaceAll(*t.switchTarget, pixelKeyVar, bestPixel)
			target = &resolved
		}
		idx, err := b.addSwitching(k, target)
		return idx, true, err
	case t.fineIdx == 0:
		return b.addChannel(k, tmpl), true, nil
	default:
		return b.addFine(k, tmpl, t.fineIdx), true, nil
	}
}

// expandModeChannels flattens a mode's raw channel list into keys (nil for
// unused slots), expanding matrix insert blocks.
func expandModeChannels(m OFLMode, matrix *OFLMatrix) ([]*string, error) {
	var keys []*string
	for i, raw := range m.Channels {
		if len(keys) >= maxModeChannels {
			return nil, fmt.Errorf("%w: mode %q has more than %d channels", ErrUnsupportedModeChannel, m.Name, maxModeChannels)
		}
		trimmed := bytes.TrimSpace(raw)
		switch {
		case len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")):
			keys = append(keys, nil)
		case trimmed[0] == '"':
			var s string
			if err := json.Unmarshal(trimmed, &s); err != nil {
				return nil, fmt.Errorf("%w: mode %q entry %d: %v", ErrUnsupportedModeChannel, m.Name, i, err)
			}
			keys = append(keys, &s)
		case trimmed[0] == '{':
			expanded, err := expandInsertBlock(trimmed, matrix, maxModeChannels-len(keys))
			if err != nil {
				return nil, fmt.Errorf("%w: mode %q entry %d: %v", ErrUnsupportedModeChannel, m.Name, i, err)
			}
			keys = append(keys, expanded...)
		default:
			return nil, fmt.Errorf("%w: mode %q entry %d: %s", ErrUnsupportedModeChannel, m.Name, i, trimmed)
		}
	}
	return keys, nil
}

type insertBlock struct {
	Insert           string          `json:"insert"`
	RepeatFor        json.RawMessage `json:"repeatFor"`
	ChannelOrder     string          `json:"channelOrder"`
	TemplateChannels []*string       `json:"templateChannels"`
}

// expandInsertBlock expands a matrix insert block. budget is the number of
// channels the mode can still take; larger blocks are rejected before any
// allocation.
func expandInsertBlock(raw []byte, matrix *OFLMatrix, budget int) ([]*string, error) {
	var blk insertBlock
	if err := json.Unmarshal(raw, &blk); err != nil {
		return nil, err
	}
	if blk.Insert != "matrixChannels" {
		return nil, fmt.Errorf("unknown insert %q", blk.Insert)
	}
	pixelKeys, err := repeatForKeys(blk.RepeatFor, matrix)
	if err != nil {
		return nil, err
	}
	// Compare by division so the product cannot overflow on 32-bit builds.
	if t := len(blk.TemplateChannels); t > 0 && len(pixelKeys) > budget/t {
		return nil, fmt.Errorf("insert block of %d pixels x %d template channels exceeds the %d channels left in the mode",
			len(pixelKeys), t, budget)
	}
	resolve := func(tmpl *string, pk string) *string {
		if tmpl == nil {
			return nil
		}
		s := strings.ReplaceAll(*tmpl, pixelKeyVar, pk)
		return &s
	}
	keys := make([]*string, 0, len(pixelKeys)*len(blk.TemplateChannels))
	switch blk.ChannelOrder {
	case "perPixel":
		for _, pk := range pixelKeys {
			for _, t := range blk.TemplateChannels {
				keys = append(keys, resolve(t, pk))
			}
		}
	case "perChannel":
		for _, t := range blk.TemplateChannels {
			for _, pk := range pixelKeys {
				keys = append(keys, resolve(t, pk))
			}
		}
	default:
		return nil, fmt.Errorf("unknown channelOrder %q", blk.ChannelOrder)
	}
	return keys, nil
}

func repeatForKeys(raw json.RawMessage, matrix *OFLMatrix) ([]string, error) {
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var mode string
	if err := json.Unmarshal(raw, &mode); err != nil {
		return nil, fmt.Errorf("invalid repeatFor %s", raw)
	}
	if matrix == nil {
		return nil, fmt.Errorf("repeatFor %q needs a matrix", mode)
	}
	switch {
	case mode == "eachPixelGroup":
		return matrix.pixelGroupKeys(), nil
	case mode == "eachPixelABC":
		return matrix.pixelKeysABC(), nil
	case strings.HasPrefix(mode, "eachPixel") && len(mode) == len("eachPixel")+3:
		axes := mode[len("eachPixel"):]
		if !isAxisPermutation(axes) {
			return nil, fmt.Errorf("invalid repeatFor %q", mode)
		}
		return matrix.pixelKeysByOrder(axes), nil
	}
	return nil, fmt.Errorf("invalid repeatFor %q", mode)
}

func isAxisPermutation(s string) bool {
	return len(s) == 3 && strings.ContainsRune(s, 'X') && strings.ContainsRune(s, 'Y') && strings.ContainsRune(s, 'Z')
}

// validPixelCount checks the bound before each multiplication so the
// product cannot overflow (int is 32 bits on 32-bit ARM builds).
func validPixelCount(c [3]int) bool {
	total := 1
	for _, n := range c {
		if n < 0 || n > maxMatrixPixels {
			return false
		}
		if n > 0 && total > maxMatrixPixels/n {
			return false
		}
		total *= n
	}
	return true
}

// pixelCount returns [x, y, z], derived from pixelKeys when pixelCount is
// not given.
func (m *OFLMatrix) pixelCount() [3]int {
	if len(m.PixelCount) == 3 {
		return [3]int{m.PixelCount[0], m.PixelCount[1], m.PixelCount[2]}
	}
	xyz := [3]int{1, 1, len(m.PixelKeys)}
	for _, ys := range m.PixelKeys {
		if len(ys) > xyz[1] {
			xyz[1] = len(ys)
		}
		for _, xs := range ys {
			if len(xs) > xyz[0] {
				xyz[0] = len(xs)
			}
		}
	}
	return xyz
}

// pixelPositions maps each pixel key to its 1-based [x, y, z] position.
func (m *OFLMatrix) pixelPositions() map[string][3]int {
	pos := map[string][3]int{}
	if len(m.PixelKeys) > 0 {
		for z, ys := range m.PixelKeys {
			for y, xs := range ys {
				for x, k := range xs {
					if k != nil {
						pos[*k] = [3]int{x + 1, y + 1, z + 1}
					}
				}
			}
		}
		return pos
	}
	c := m.pixelCount()
	var axes []int
	for i, n := range c {
		if n > 1 {
			axes = append(axes, i)
		}
	}
	for z := 1; z <= c[2]; z++ {
		for y := 1; y <= c[1]; y++ {
			for x := 1; x <= c[0]; x++ {
				p := [3]int{x, y, z}
				var key string
				switch len(axes) {
				case 0, 1:
					key = strconv.Itoa(max(x, y, z))
				case 2:
					key = fmt.Sprintf("(%d, %d)", p[axes[0]], p[axes[1]])
				default:
					key = fmt.Sprintf("(%d, %d, %d)", x, y, z)
				}
				pos[key] = p
			}
		}
	}
	return pos
}

// pixelKeysABC returns pixel keys in natural alphanumeric order.
func (m *OFLMatrix) pixelKeysABC() []string {
	pos := m.pixelPositions()
	keys := make([]string, 0, len(pos))
	for k := range pos {
		keys = append(keys, k)
	}
	// Numeric collation can tie ("1" vs "01"); break ties on the raw
	// string so the order (and thus DMX offsets) is deterministic.
	sort.Slice(keys, func(i, j int) bool {
		if naturalLess(keys[i], keys[j]) {
			return true
		}
		if naturalLess(keys[j], keys[i]) {
			return false
		}
		return keys[i] < keys[j]
	})
	return keys
}

// pixelKeysByOrder sorts pixel keys by position: axes[0] varies fastest,
// axes[2] slowest (e.g. "XYZ" reads like a book).
func (m *OFLMatrix) pixelKeysByOrder(axes string) []string {
	idx := func(r byte) int { return int(r - 'X') }
	first, second, third := idx(axes[0]), idx(axes[1]), idx(axes[2])
	pos := m.pixelPositions()
	keys := m.pixelKeysABC()
	sort.SliceStable(keys, func(i, j int) bool {
		a, b := pos[keys[i]], pos[keys[j]]
		if a[third] != b[third] {
			return a[third] < b[third]
		}
		if a[second] != b[second] {
			return a[second] < b[second]
		}
		return a[first] < b[first]
	})
	return keys
}

// pixelGroupKeys returns pixel group keys in JSON order.
func (m *OFLMatrix) pixelGroupKeys() []string {
	if len(m.pixelGroupOrder) == len(m.PixelGroups) {
		return m.pixelGroupOrder
	}
	keys := make([]string, 0, len(m.PixelGroups))
	for k := range m.PixelGroups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// naturalLess compares strings case-insensitively, treating digit runs as
// numbers (approximating JavaScript localeCompare with numeric: true).
func naturalLess(a, b string) bool {
	ar, br := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ar) && j < len(br) {
		if unicode.IsDigit(ar[i]) && unicode.IsDigit(br[j]) {
			si := i
			for i < len(ar) && unicode.IsDigit(ar[i]) {
				i++
			}
			sj := j
			for j < len(br) && unicode.IsDigit(br[j]) {
				j++
			}
			na := strings.TrimLeft(string(ar[si:i]), "0")
			nb := strings.TrimLeft(string(br[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		ca, cb := unicode.ToLower(ar[i]), unicode.ToLower(br[j])
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	return len(ar)-i < len(br)-j
}
