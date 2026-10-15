package gdtf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustPlan(t *testing.T, xmlText string) *importPlan {
	t.Helper()
	doc, warns, err := parseDescription(xmlText)
	if err != nil {
		t.Fatalf("parseDescription: %v", err)
	}
	plan, err := buildPlan(doc, warns)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	return plan
}

func (p *importPlan) mode(t *testing.T, name string) plannedMode {
	t.Helper()
	for _, m := range p.Modes {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("mode %q not found; have %v", name, p.modeNames())
	return plannedMode{}
}

func (p *importPlan) modeNames() []string {
	out := make([]string, len(p.Modes))
	for i, m := range p.Modes {
		out[i] = m.Name
	}
	return out
}

// channels returns the planned channels of a mode, in offset order.
func (p *importPlan) channels(m plannedMode) []plannedChannel {
	out := make([]plannedChannel, len(m.channelIdx))
	for i, idx := range m.channelIdx {
		out[i] = p.Channels[idx]
	}
	return out
}

func channelNames(chs []plannedChannel) []string {
	out := make([]string, len(chs))
	for i, c := range chs {
		out[i] = c.Name
	}
	return out
}

func hasWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// testAttributes is an AttributeDefinitions block for inline test XML.
const testAttributes = `<AttributeDefinitions><Attributes>
  <Attribute Name="Dimmer" Pretty="Dimmer" Feature="Dimmer.Dimmer"/>
  <Attribute Name="ColorAdd_R" Pretty="Red" Feature="Color.RGB"/>
  <Attribute Name="ColorAdd_G" Pretty="Green" Feature="Color.RGB"/>
  <Attribute Name="Pan" Pretty="Pan" Feature="Position.PanTilt"/>
  <Attribute Name="NoFeature" Pretty="NoFeature" Feature="Control.Control"/>
</Attributes></AttributeDefinitions>`

// inlineXML builds a minimal description with one top-level geometry
// "Body" and the given DMXChannels in mode "Mode".
func inlineXML(version, fixtureTypeID, channels string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<GDTF DataVersion="%s">
  <FixtureType Name="Inline" LongName="Inline Test" Manufacturer="Synthetic Lighting Co" FixtureTypeID="%s">
    %s
    <Geometries><Geometry Name="Body"><Beam Name="Beam"/></Geometry></Geometries>
    <DMXModes><DMXMode Name="Mode" Geometry="Body"><DMXChannels>%s</DMXChannels></DMXMode></DMXModes>
  </FixtureType>
</GDTF>`, version, fixtureTypeID, testAttributes, channels)
}

// dmxChannel builds one DMXChannel with a single logical channel.
func dmxChannel(offset, attr, extraAttrs, functions string) string {
	off := ""
	if offset != "-" {
		off = fmt.Sprintf(` Offset="%s"`, offset)
	}
	return fmt.Sprintf(`<DMXChannel DMXBreak="1"%s Geometry="Beam" %s><LogicalChannel Attribute="%s">%s</LogicalChannel></DMXChannel>`,
		off, extraAttrs, attr, functions)
}
