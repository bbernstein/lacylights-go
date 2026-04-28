package exporteos

import "fmt"

// PersonalityIn is the input shape for one personality emission.
type PersonalityIn struct {
	ID        int
	Manuf     string
	Model     string
	Dcid      string
	Footprint int
	Channels  []PersonalityChannelIn
}

// PersonalityChannelIn describes one $$PersChan emission.
type PersonalityChannelIn struct {
	ParamID   int
	Size      int
	Offset    int
	Offset16  int
	HomeValue int
	Snap      bool
}

func (w *writer) writePersonality(p PersonalityIn) {
	w.line(fmt.Sprintf("$Personality %d", p.ID))
	w.line(fmt.Sprintf("   $$Manuf %s", p.Manuf))
	w.line(fmt.Sprintf("   $$Model %s", p.Model))
	if p.Dcid != "" {
		w.line(fmt.Sprintf("   $$Dcid %s", p.Dcid))
	}
	w.line(fmt.Sprintf("   $$Footprint %d", p.Footprint))
	for _, c := range p.Channels {
		flags := ""
		if c.Snap {
			flags = " S"
		}
		offset16 := c.Offset16
		if c.Size != 2 {
			offset16 = 0
		}
		w.line(fmt.Sprintf("   $$PersChan %5d %5d %5d %5d %5d%s",
			c.ParamID, c.Size, c.Offset, offset16, c.HomeValue, flags))
	}
}
