// Package ofl provides OFL (Open Fixture Library) import functionality
package ofl

import "encoding/json"

// OFLCapability describes what a channel can do
type OFLCapability struct {
	Type             string   `json:"type"`
	Color            string   `json:"color,omitempty"`
	BrightnessStart  string   `json:"brightnessStart,omitempty"`
	BrightnessEnd    string   `json:"brightnessEnd,omitempty"`
	DMXRange         *[2]int  `json:"dmxRange,omitempty"`
	Comment          string   `json:"comment,omitempty"`
	SpeedStart       string   `json:"speedStart,omitempty"`
	SpeedEnd         string   `json:"speedEnd,omitempty"`
	ShutterEffect    string   `json:"shutterEffect,omitempty"`
	EffectName       string   `json:"effectName,omitempty"`
	ColorTemperature string   `json:"colorTemperature,omitempty"`
	Colors           []string `json:"colors,omitempty"`
	ColorsStart      []string `json:"colorsStart,omitempty"`
	ColorsEnd        []string `json:"colorsEnd,omitempty"`
	// SwitchChannels maps a switching channel alias to the available
	// channel key it resolves to while this capability is active. A nil
	// value switches to an unused channel.
	SwitchChannels map[string]*string `json:"switchChannels,omitempty"`
}

// OFLChannel represents a channel in OFL format
type OFLChannel struct {
	Capability         *OFLCapability  `json:"capability,omitempty"`
	Capabilities       []OFLCapability `json:"capabilities,omitempty"`
	FineChannelAliases []string        `json:"fineChannelAliases,omitempty"`
	// DefaultValue is a DMX value (number) or a percentage string.
	DefaultValue json.RawMessage `json:"defaultValue,omitempty"`
}

// OFLMode represents an operating mode
type OFLMode struct {
	Name      string `json:"name"`
	ShortName string `json:"shortName,omitempty"`
	// Channels holds the raw mode channel entries. Each entry is a channel
	// key (string), null (an unused DMX slot) or a matrix insert block
	// object. See resolveModeChannels.
	Channels []json.RawMessage `json:"channels"`
}

// OFLMatrix describes the pixel layout of a matrix fixture.
type OFLMatrix struct {
	PixelCount  []int                      `json:"pixelCount,omitempty"`
	PixelKeys   [][][]*string              `json:"pixelKeys,omitempty"`
	PixelGroups map[string]json.RawMessage `json:"pixelGroups,omitempty"`

	// pixelGroupOrder lists pixelGroups keys in JSON order (needed for
	// repeatFor "eachPixelGroup"). Filled by parseOFLFixture.
	pixelGroupOrder []string
}

// OFLMeta contains fixture metadata
type OFLMeta struct {
	Authors        []string `json:"authors,omitempty"`
	CreateDate     string   `json:"createDate,omitempty"`
	LastModifyDate string   `json:"lastModifyDate,omitempty"`
}

// OFLFixture represents the complete OFL fixture JSON structure
type OFLFixture struct {
	Name string `json:"name"`
	// RedirectTo is set on OFL redirect files ("<manufacturer>/<fixture>")
	// that point at a renamed fixture or the same fixture under another
	// brand. They carry no channel data.
	RedirectTo        string                `json:"redirectTo,omitempty"`
	ShortName         string                `json:"shortName,omitempty"`
	Categories        []string              `json:"categories"`
	Meta              *OFLMeta              `json:"meta,omitempty"`
	Modes             []OFLMode             `json:"modes"`
	AvailableChannels map[string]OFLChannel `json:"availableChannels"`
	TemplateChannels  map[string]OFLChannel `json:"templateChannels,omitempty"`
	Matrix            *OFLMatrix            `json:"matrix,omitempty"`

	// availableChannelOrder lists availableChannels keys in JSON order so
	// definition channel offsets are deterministic. Filled by parseOFLFixture.
	availableChannelOrder []string
}

// ChannelDefinition represents a processed channel ready for database storage
type ChannelDefinition struct {
	Name         string
	Type         string
	Offset       int
	MinValue     int
	MaxValue     int
	DefaultValue int
	FadeBehavior string // FADE, SNAP, or SNAP_END
	IsDiscrete   bool   // True if channel has multiple discrete DMX ranges
}

// ModeDefinition represents a processed mode
type ModeDefinition struct {
	Name         string
	ShortName    string
	ChannelCount int
	ChannelNames []string
}
