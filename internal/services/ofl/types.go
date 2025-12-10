// Package ofl provides OFL (Open Fixture Library) import functionality
package ofl

// OFLCapability describes what a channel can do
type OFLCapability struct {
	Type             string     `json:"type"`
	Color            string     `json:"color,omitempty"`
	BrightnessStart  string     `json:"brightnessStart,omitempty"`
	BrightnessEnd    string     `json:"brightnessEnd,omitempty"`
	DMXRange         *[2]int    `json:"dmxRange,omitempty"`
	Comment          string     `json:"comment,omitempty"`
	SpeedStart       string     `json:"speedStart,omitempty"`
	SpeedEnd         string     `json:"speedEnd,omitempty"`
	ShutterEffect    string     `json:"shutterEffect,omitempty"`
	EffectName       string     `json:"effectName,omitempty"`
	ColorTemperature string     `json:"colorTemperature,omitempty"`
	Colors           []string   `json:"colors,omitempty"`
	ColorsStart      []string   `json:"colorsStart,omitempty"`
	ColorsEnd        []string   `json:"colorsEnd,omitempty"`
}

// OFLChannel represents a channel in OFL format
type OFLChannel struct {
	Capability        *OFLCapability  `json:"capability,omitempty"`
	Capabilities      []OFLCapability `json:"capabilities,omitempty"`
	FineChannelAliases []string       `json:"fineChannelAliases,omitempty"`
}

// OFLMode represents an operating mode
type OFLMode struct {
	Name      string   `json:"name"`
	ShortName string   `json:"shortName,omitempty"`
	Channels  []string `json:"channels"`
}

// OFLMeta contains fixture metadata
type OFLMeta struct {
	Authors        []string `json:"authors,omitempty"`
	CreateDate     string   `json:"createDate,omitempty"`
	LastModifyDate string   `json:"lastModifyDate,omitempty"`
}

// OFLFixture represents the complete OFL fixture JSON structure
type OFLFixture struct {
	Name              string                `json:"name"`
	ShortName         string                `json:"shortName,omitempty"`
	Categories        []string              `json:"categories"`
	Meta              *OFLMeta              `json:"meta,omitempty"`
	Modes             []OFLMode             `json:"modes"`
	AvailableChannels map[string]OFLChannel `json:"availableChannels"`
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
