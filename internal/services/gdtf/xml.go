package gdtf

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// The structs below cover the parts of a GDTF 1.2 description.xml that the
// importer reads. Everything else (wheels, physical descriptions, models,
// relations, macros, presets, protocols) is ignored here; the whole
// document is kept gzipped in FixtureDefinition.SourcePayload.

type xmlGDTF struct {
	XMLName     xml.Name       `xml:"GDTF"`
	DataVersion string         `xml:"DataVersion,attr"`
	FixtureType xmlFixtureType `xml:"FixtureType"`
}

type xmlFixtureType struct {
	Name          string         `xml:"Name,attr"`
	ShortName     string         `xml:"ShortName,attr"`
	LongName      string         `xml:"LongName,attr"`
	Manufacturer  string         `xml:"Manufacturer,attr"`
	FixtureTypeID string         `xml:"FixtureTypeID,attr"`
	Attributes    []xmlAttribute `xml:"AttributeDefinitions>Attributes>Attribute"`
	Geometries    xmlGeometries  `xml:"Geometries"`
	DMXModes      []xmlDMXMode   `xml:"DMXModes>DMXMode"`
	Revisions     []xmlRevision  `xml:"Revisions>Revision"`
}

type xmlAttribute struct {
	Name         string `xml:"Name,attr"`
	Pretty       string `xml:"Pretty,attr"`
	Feature      string `xml:"Feature,attr"`
	PhysicalUnit string `xml:"PhysicalUnit,attr"`
}

// xmlGeometries is the <Geometries> collect. Geometry nodes come in many
// element types (Geometry, Axis, Beam, ..., GeometryReference), so they are
// read generically.
type xmlGeometries struct {
	Children []xmlGeometry `xml:",any"`
}

// xmlGeometry is any geometry node, or a <Break> child of a
// GeometryReference (which uses only DMXOffset and DMXBreak).
type xmlGeometry struct {
	XMLName   xml.Name
	Name      string        `xml:"Name,attr"`
	Geometry  string        `xml:"Geometry,attr"` // GeometryReference: referenced top-level geometry
	DMXOffset string        `xml:"DMXOffset,attr"`
	DMXBreak  string        `xml:"DMXBreak,attr"`
	Children  []xmlGeometry `xml:",any"`
}

func (g *xmlGeometry) isReference() bool { return g.XMLName.Local == "GeometryReference" }
func (g *xmlGeometry) isBreak() bool     { return g.XMLName.Local == "Break" }

type xmlDMXMode struct {
	Name        string          `xml:"Name,attr"`
	Description string          `xml:"Description,attr"`
	Geometry    string          `xml:"Geometry,attr"`
	Channels    []xmlDMXChannel `xml:"DMXChannels>DMXChannel"`
}

type xmlDMXChannel struct {
	DMXBreak        string              `xml:"DMXBreak,attr"`
	Offset          *string             `xml:"Offset,attr"` // nil: attribute absent (= "None")
	InitialFunction string              `xml:"InitialFunction,attr"`
	Geometry        string              `xml:"Geometry,attr"`
	Default         string              `xml:"Default,attr"` // legacy (GDTF 1.1)
	LogicalChannels []xmlLogicalChannel `xml:"LogicalChannel"`
}

type xmlLogicalChannel struct {
	Attribute string               `xml:"Attribute,attr"`
	Snap      string               `xml:"Snap,attr"`
	Functions []xmlChannelFunction `xml:"ChannelFunction"`
}

type xmlChannelFunction struct {
	Name       string          `xml:"Name,attr"`
	Attribute  string          `xml:"Attribute,attr"`
	DMXFrom    string          `xml:"DMXFrom,attr"`
	Default    string          `xml:"Default,attr"`
	ModeMaster string          `xml:"ModeMaster,attr"`
	ModeFrom   string          `xml:"ModeFrom,attr"`
	ModeTo     string          `xml:"ModeTo,attr"`
	Sets       []xmlChannelSet `xml:"ChannelSet"`
}

type xmlChannelSet struct {
	Name           string `xml:"Name,attr"`
	DMXFrom        string `xml:"DMXFrom,attr"`
	WheelSlotIndex string `xml:"WheelSlotIndex,attr"`
}

type xmlRevision struct {
	Text string `xml:"Text,attr"`
	Date string `xml:"Date,attr"`
}

// Version support.
const (
	supportedMajor = 1
	supportedMinor = 2
)

// ErrUnsupportedVersion is returned for GDTF versions the importer rejects.
var ErrUnsupportedVersion = errors.New("unsupported GDTF version")

// ErrInvalidDescription is returned for malformed or incomplete files.
var ErrInvalidDescription = errors.New("invalid GDTF description")

// parseDescription decodes description.xml and checks its version and
// required fields. Warnings are returned for versions imported on a
// best-effort basis.
func parseDescription(text string) (*xmlGDTF, []string, error) {
	data := bytes.TrimPrefix([]byte(text), []byte("\xef\xbb\xbf")) // UTF-8 BOM
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charsetReader
	var doc xmlGDTF
	if err := dec.Decode(&doc); err != nil {
		return nil, nil, fmt.Errorf("%w: XML: %v", ErrInvalidDescription, err)
	}
	warnings, err := checkVersion(doc.DataVersion)
	if err != nil {
		return nil, nil, err
	}
	ft := &doc.FixtureType
	switch {
	case strings.TrimSpace(ft.FixtureTypeID) == "":
		return nil, nil, fmt.Errorf("%w: FixtureType has no FixtureTypeID", ErrInvalidDescription)
	case strings.TrimSpace(ft.Name) == "" && strings.TrimSpace(ft.LongName) == "":
		return nil, nil, fmt.Errorf("%w: FixtureType has no Name", ErrInvalidDescription)
	case strings.TrimSpace(ft.Manufacturer) == "":
		return nil, nil, fmt.Errorf("%w: FixtureType has no Manufacturer", ErrInvalidDescription)
	case len(ft.DMXModes) == 0:
		return nil, nil, fmt.Errorf("%w: no DMX modes", ErrInvalidDescription)
	}
	return &doc, warnings, nil
}

// charsetReader accepts the encodings description.xml uses in practice.
// UTF-8 is the decoder default; ASCII is a subset of it.
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "utf-8", "utf8", "us-ascii", "ascii":
		return input, nil
	}
	return nil, fmt.Errorf("unsupported XML encoding %q", charset)
}

// checkVersion applies the version policy: 1.2 is supported, newer
// versions and 1.1 are imported with a warning, 1.0 and older rejected.
func checkVersion(v string) ([]string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, fmt.Errorf("%w: GDTF node has no DataVersion", ErrUnsupportedVersion)
	}
	majorStr, minorStr, ok := strings.Cut(v, ".")
	major, errMajor := strconv.Atoi(majorStr)
	minor, errMinor := strconv.Atoi(minorStr)
	if !ok || errMajor != nil || errMinor != nil || major < 0 || minor < 0 {
		return nil, fmt.Errorf("%w: DataVersion %q is not Major.Minor", ErrUnsupportedVersion, v)
	}
	switch {
	case major > supportedMajor || (major == supportedMajor && minor > supportedMinor):
		return []string{fmt.Sprintf("GDTF %s is newer than the supported version 1.2; imported on a best-effort basis", v)}, nil
	case major == supportedMajor && minor == supportedMinor:
		return nil, nil
	case major == 1 && minor == 1:
		return []string{"GDTF 1.1 file imported on a best-effort basis; re-export it as GDTF 1.2 for best results"}, nil
	}
	return nil, fmt.Errorf("%w: GDTF %s is not supported (1.1 or later required); re-export the file with a current GDTF editor", ErrUnsupportedVersion, v)
}
