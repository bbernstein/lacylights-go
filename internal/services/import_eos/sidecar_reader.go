package importeos

import (
	"encoding/json"
	"strings"
)

// Sidecar holds parsed `$$ LACYLIGHTS:` records from an Eos ASCII file.
type Sidecar struct {
	Version       int
	LookBoards    []SidecarLookBoard
	FadeBehaviors []SidecarFadeBehavior
	SynthDefs     []SidecarSynthDef
}

// SidecarLookBoard describes a LacyLights look board.
type SidecarLookBoard struct {
	RefID   string                   `json:"refId"`
	Name    string                   `json:"name"`
	Buttons []SidecarLookBoardButton `json:"buttons"`
}

// SidecarLookBoardButton describes one button on a look board.
type SidecarLookBoardButton struct {
	LookRefID string `json:"lookRefId"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Color     string `json:"color"`
}

// SidecarFadeBehavior captures non-default per-channel fade behaviors.
type SidecarFadeBehavior struct {
	InstanceRefID string                       `json:"instanceRefId"`
	Channels      []SidecarFadeBehaviorChannel `json:"channels"`
}

// SidecarFadeBehaviorChannel describes one channel's behavior.
type SidecarFadeBehaviorChannel struct {
	Offset   int    `json:"offset"`
	Behavior string `json:"behavior"` // "FADE" | "SNAP" | "SNAP_END"
}

// SidecarSynthDef marks a fixture definition that was synthesized on a prior import.
type SidecarSynthDef struct {
	DefRefID           string `json:"defRefId"`
	Manufacturer       string `json:"manufacturer"`
	Model              string `json:"model"`
	ChannelFingerprint string `json:"channelFingerprint"`
}

// ReadSidecar parses raw sidecar lines into a Sidecar struct.
// Malformed records are skipped and reported via the supplied collector.
func ReadSidecar(rawLines []string, warn *Collector) Sidecar {
	out := Sidecar{}
	for _, line := range rawLines {
		const prefix = "$$ LACYLIGHTS:"
		idx := strings.Index(line, prefix)
		if idx < 0 {
			continue
		}
		body := strings.TrimSpace(line[idx+len(prefix):])
		spaceAt := strings.IndexAny(body, " \t")
		var key, payload string
		if spaceAt < 0 {
			key, payload = body, ""
		} else {
			key, payload = body[:spaceAt], strings.TrimSpace(body[spaceAt+1:])
		}
		switch key {
		case "version":
			if v, ok := parseSidecarInt(payload); ok {
				out.Version = v
			} else {
				warn.Add(WarnSidecarInvalid, SeverityWarn, "invalid sidecar version", nil)
			}
		case "look_board":
			var lb SidecarLookBoard
			if err := json.Unmarshal([]byte(payload), &lb); err != nil {
				warn.Add(WarnSidecarInvalid, SeverityWarn, "invalid look_board JSON",
					map[string]string{"err": err.Error()})
				continue
			}
			out.LookBoards = append(out.LookBoards, lb)
		case "fade_behavior":
			var fb SidecarFadeBehavior
			if err := json.Unmarshal([]byte(payload), &fb); err != nil {
				warn.Add(WarnSidecarInvalid, SeverityWarn, "invalid fade_behavior JSON",
					map[string]string{"err": err.Error()})
				continue
			}
			out.FadeBehaviors = append(out.FadeBehaviors, fb)
		case "synth_def":
			var sd SidecarSynthDef
			if err := json.Unmarshal([]byte(payload), &sd); err != nil {
				warn.Add(WarnSidecarInvalid, SeverityWarn, "invalid synth_def JSON",
					map[string]string{"err": err.Error()})
				continue
			}
			out.SynthDefs = append(out.SynthDefs, sd)
		default:
			warn.Add(WarnSidecarInvalid, SeverityInfo,
				"unknown sidecar key: "+key, nil)
		}
	}
	return out
}

func parseSidecarInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		v = v*10 + int(r-'0')
	}
	return v, true
}
