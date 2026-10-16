package ofl

// MapFadeBehavior returns the default fade behavior (FADE or SNAP) for a
// channel type, as the OFL importer assigns it. Other importers (GDTF) use
// it so the same channel type fades the same way whatever its source.
func MapFadeBehavior(channelType string, isDiscrete bool) string {
	return mapFadeBehavior(channelType, isDiscrete)
}
