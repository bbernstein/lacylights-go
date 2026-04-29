// Package importeos parses ETC Eos ASCII showfiles into LacyLights domain objects.
package importeos

// Severity is the severity of a non-fatal warning produced by the importer or exporter.
type Severity string

const (
	SeverityInfo Severity = "INFO"
	SeverityWarn Severity = "WARN"
)

// WarningCode classifies a structured warning so the UI/MCP can group them.
type WarningCode string

const (
	WarnSynthesizedFixture WarningCode = "SYNTHESIZED_FIXTURE"
	WarnEffectSkipped      WarningCode = "EFFECT_SKIPPED"
	WarnSubmasterSkipped   WarningCode = "SUBMASTER_SKIPPED"
	WarnMagicSheetSkipped  WarningCode = "MAGIC_SHEET_SKIPPED"
	WarnPartitionSkipped   WarningCode = "PARTITION_SKIPPED"
	WarnActionSkipped      WarningCode = "ACTION_SKIPPED"
	WarnCurveSkipped       WarningCode = "CURVE_SKIPPED"
	WarnUnknownDirective   WarningCode = "UNKNOWN_DIRECTIVE"
	WarnFadeBehaviorLost   WarningCode = "FADE_BEHAVIOR_LOST"
	WarnUTextDecode        WarningCode = "UTEXT_DECODE"
	WarnSidecarInvalid     WarningCode = "SIDECAR_INVALID"
	WarnSidecarUnresolved  WarningCode = "SIDECAR_UNRESOLVED"
	WarnUnpatchedChannel   WarningCode = "UNPATCHED_CHANNEL"
	WarnUnpatchedInstance  WarningCode = "UNPATCHED_INSTANCE"
	WarnLookValuesInvalid  WarningCode = "LOOK_VALUES_INVALID"
	WarnGroupsSkipped      WarningCode = "GROUPS_SKIPPED"
	WarnAddressConflict    WarningCode = "ADDRESS_CONFLICT"
	// WarnGroupAutoAssigned is emitted by the GraphQL resolver layer
	// (not the parser/mapper) when a multi-group user gets their new
	// imported project silently assigned to groupIDs[0]. Listed here
	// so the registry of every code a client may receive lives in one
	// canonical place, even though the warning is produced outside
	// import_eos.
	WarnGroupAutoAssigned WarningCode = "GROUP_AUTO_ASSIGNED"
)

// Warning is a structured non-fatal warning emitted during import or export.
type Warning struct {
	Code     WarningCode
	Severity Severity
	Message  string
	Context  map[string]string
}

// Collector accumulates warnings during a single import or export run.
type Collector struct {
	warnings []Warning
}

// Add records a new warning.
func (c *Collector) Add(code WarningCode, severity Severity, message string, context map[string]string) {
	c.warnings = append(c.warnings, Warning{
		Code:     code,
		Severity: severity,
		Message:  message,
		Context:  context,
	})
}

// All returns all warnings collected so far.
func (c *Collector) All() []Warning {
	out := make([]Warning, len(c.warnings))
	copy(out, c.warnings)
	return out
}
