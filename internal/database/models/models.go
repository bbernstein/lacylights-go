// Package models contains the database model definitions.
// These models map directly to the SQLite database tables.
package models

import (
	"time"
)

// User represents a user in the system.
// Table: users
type User struct {
	ID            string     `gorm:"column:id;primaryKey"`
	Email         string     `gorm:"column:email;uniqueIndex"`
	Name          *string    `gorm:"column:name"`
	Phone         *string    `gorm:"column:phone"`
	Role          string     `gorm:"column:role;default:USER"`
	EmailVerified bool       `gorm:"column:email_verified;default:false"`
	PhoneVerified bool       `gorm:"column:phone_verified;default:false"`
	IsActive      bool       `gorm:"column:is_active;default:true"`
	LastLoginAt   *time.Time `gorm:"column:last_login_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (User) TableName() string { return "users" }

// UserCredential stores password credentials for a user.
// Table: user_credentials
type UserCredential struct {
	ID                string     `gorm:"column:id;primaryKey"`
	UserID            string     `gorm:"column:user_id;uniqueIndex"`
	PasswordHash      string     `gorm:"column:password_hash"`
	PasswordUpdatedAt time.Time  `gorm:"column:password_updated_at"`
	FailedAttempts    int        `gorm:"column:failed_attempts;default:0"`
	LockedUntil       *time.Time `gorm:"column:locked_until"`
	CreatedAt         time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	User *User `gorm:"foreignKey:UserID"`
}

func (UserCredential) TableName() string { return "user_credentials" }

// UserOAuth stores OAuth provider connections for a user.
// Table: user_oauth
type UserOAuth struct {
	ID             string    `gorm:"column:id;primaryKey"`
	UserID         string    `gorm:"column:user_id;index"`
	Provider       string    `gorm:"column:provider"`
	ProviderUserID string    `gorm:"column:provider_user_id"`
	Email          *string   `gorm:"column:email"`
	RefreshToken   *string   `gorm:"column:refresh_token"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	User *User `gorm:"foreignKey:UserID"`
}

func (UserOAuth) TableName() string { return "user_oauth" }

// Session represents an active user session.
// Table: sessions
type Session struct {
	ID             string    `gorm:"column:id;primaryKey"`
	UserID         string    `gorm:"column:user_id;index"`
	TokenHash      string    `gorm:"column:token_hash;uniqueIndex"`
	DeviceID       *string   `gorm:"column:device_id;index"`
	IPAddress      *string   `gorm:"column:ip_address"`
	UserAgent      *string   `gorm:"column:user_agent"`
	ExpiresAt      time.Time `gorm:"column:expires_at"`
	LastActivityAt time.Time `gorm:"column:last_activity_at"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`

	// Relations
	User   *User   `gorm:"foreignKey:UserID"`
	Device *Device `gorm:"foreignKey:DeviceID"`
}

func (Session) TableName() string { return "sessions" }

// DeviceStatus represents the current status of a device.
const (
	DeviceStatusPending  = "PENDING"
	DeviceStatusApproved = "APPROVED"
	DeviceStatusRevoked  = "REVOKED"
)

// DevicePermissions represents the permission level for a device.
const (
	DevicePermissionsReadOnly = "READ_ONLY"
	DevicePermissionsOperator = "OPERATOR"
	DevicePermissionsAdmin    = "ADMIN"
)

// Device represents a pre-authorized device for device-based authentication.
// Table: devices
type Device struct {
	ID                         string     `gorm:"column:id;primaryKey"`
	Name                       string     `gorm:"column:name"`
	Fingerprint                string     `gorm:"column:fingerprint;uniqueIndex"`
	FingerprintComponents      *string    `gorm:"column:fingerprint_components"` // JSON
	Status                     string     `gorm:"column:status;default:PENDING;index"` // PENDING, APPROVED, REVOKED
	Permissions                string     `gorm:"column:permissions;default:READ_ONLY"` // READ_ONLY, OPERATOR, ADMIN
	IsAuthorized               bool       `gorm:"column:is_authorized;default:false"` // Legacy field - kept for backward compatibility. Use Status field for authoritative state.
	AuthorizationCode          *string    `gorm:"column:authorization_code"`
	AuthorizationCodeExpiresAt *time.Time `gorm:"column:authorization_code_expires_at"`
	DefaultUserID              *string    `gorm:"column:default_user_id;index"`
	DefaultRole                string     `gorm:"column:default_role;default:PLAYER"`
	LastSeenAt                 *time.Time `gorm:"column:last_seen_at"`
	LastIPAddress              *string    `gorm:"column:last_ip_address"`
	CreatedAt                  time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                  time.Time  `gorm:"column:updated_at;autoUpdateTime"`
	ApprovedAt                 *time.Time `gorm:"column:approved_at"`
	ApprovedByID               *string    `gorm:"column:approved_by;index"`

	// Relations
	DefaultUser *User `gorm:"foreignKey:DefaultUserID"`
	ApprovedBy  *User `gorm:"foreignKey:ApprovedByID"`
}

func (Device) TableName() string { return "devices" }

// VerificationToken stores tokens for email/phone verification and password reset.
// Table: verification_tokens
type VerificationToken struct {
	ID        string     `gorm:"column:id;primaryKey"`
	UserID    *string    `gorm:"column:user_id;index"`
	Email     *string    `gorm:"column:email"`
	Phone     *string    `gorm:"column:phone"`
	TokenHash string     `gorm:"column:token_hash"`
	TokenType string     `gorm:"column:token_type"` // EMAIL_VERIFY, PHONE_VERIFY, PASSWORD_RESET
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	UsedAt    *time.Time `gorm:"column:used_at"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime"`

	// Relations
	User *User `gorm:"foreignKey:UserID"`
}

func (VerificationToken) TableName() string { return "verification_tokens" }

// UserGroup represents a permission group for users.
// Table: user_groups
type UserGroup struct {
	ID          string    `gorm:"column:id;primaryKey"`
	Name        string    `gorm:"column:name;uniqueIndex"`
	Description *string   `gorm:"column:description"`
	Permissions *string   `gorm:"column:permissions"` // JSON array of permission strings
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	Members []UserGroupMember `gorm:"foreignKey:GroupID"`
}

func (UserGroup) TableName() string { return "user_groups" }

// UserGroupMember represents a user's membership in a group.
// Table: user_group_members
type UserGroupMember struct {
	ID        string    `gorm:"column:id;primaryKey"`
	UserID    string    `gorm:"column:user_id;index"`
	GroupID   string    `gorm:"column:group_id;index"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`

	// Relations
	User  *User      `gorm:"foreignKey:UserID"`
	Group *UserGroup `gorm:"foreignKey:GroupID"`
}

func (UserGroupMember) TableName() string { return "user_group_members" }

// AuthSetting stores global authentication configuration settings.
// Table: auth_settings
type AuthSetting struct {
	ID        string    `gorm:"column:id;primaryKey"`
	Key       string    `gorm:"column:key;uniqueIndex"`
	Value     string    `gorm:"column:value"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (AuthSetting) TableName() string { return "auth_settings" }

// Project represents a lighting project.
// Table: projects
type Project struct {
	ID          string    `gorm:"column:id;primaryKey"`
	Name        string    `gorm:"column:name"`
	Description *string   `gorm:"column:description"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Soft delete support - nil means not deleted
	DeletedAt *time.Time `gorm:"column:deleted_at;index"`

	// 2D Layout Canvas Configuration (for fixture layout editor)
	LayoutCanvasWidth  int `gorm:"column:layout_canvas_width;default:2000"`
	LayoutCanvasHeight int `gorm:"column:layout_canvas_height;default:2000"`

	// Relations (loaded separately)
	Fixtures   []FixtureInstance `gorm:"foreignKey:ProjectID"`
	Looks      []Look            `gorm:"foreignKey:ProjectID"`
	CueLists   []CueList         `gorm:"foreignKey:ProjectID"`
	LookBoards []LookBoard       `gorm:"foreignKey:ProjectID"`
	Effects    []Effect          `gorm:"foreignKey:ProjectID"`
}

func (Project) TableName() string { return "projects" }

// ProjectUser represents the many-to-many relationship between users and projects.
// Table: project_users
type ProjectUser struct {
	ID        string    `gorm:"column:id;primaryKey"`
	UserID    string    `gorm:"column:user_id;index"`
	ProjectID string    `gorm:"column:project_id;index"`
	Role      string    `gorm:"column:role;default:VIEWER"`
	JoinedAt  time.Time `gorm:"column:joined_at;autoCreateTime"`
}

func (ProjectUser) TableName() string { return "project_users" }

// FixtureDefinition represents a fixture type definition.
// Table: fixture_definitions
type FixtureDefinition struct {
	ID           string    `gorm:"column:id;primaryKey"`
	Manufacturer string    `gorm:"column:manufacturer"`
	Model        string    `gorm:"column:model"`
	Type         string    `gorm:"column:type"`
	IsBuiltIn    bool      `gorm:"column:is_built_in;default:false"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// OFL tracking fields for change detection
	OFLSourceHash *string `gorm:"column:ofl_source_hash"` // SHA256 of the original OFL JSON
	OFLVersion    *string `gorm:"column:ofl_version"`     // OFL commit/version when imported

	// Relations
	Channels []ChannelDefinition `gorm:"foreignKey:DefinitionID"`
	Modes    []FixtureMode       `gorm:"foreignKey:DefinitionID"`
}

func (FixtureDefinition) TableName() string { return "fixture_definitions" }

// ChannelDefinition represents a channel within a fixture definition.
// Table: channel_definitions
type ChannelDefinition struct {
	ID           string `gorm:"column:id;primaryKey"`
	Name         string `gorm:"column:name"`
	Type         string `gorm:"column:type"`
	Offset       int    `gorm:"column:offset"`
	MinValue     int    `gorm:"column:min_value;default:0"`
	MaxValue     int    `gorm:"column:max_value;default:255"`
	DefaultValue int    `gorm:"column:default_value;default:0"`
	FadeBehavior string `gorm:"column:fade_behavior;default:FADE"` // FadeBehavior enum: FADE, SNAP, SNAP_END
	IsDiscrete   bool   `gorm:"column:is_discrete;default:false"`  // True if channel has multiple discrete DMX ranges
	DefinitionID string `gorm:"column:definition_id;index"`
}

func (ChannelDefinition) TableName() string { return "channel_definitions" }

// FixtureMode represents a mode within a fixture definition.
// Table: fixture_modes
type FixtureMode struct {
	ID           string  `gorm:"column:id;primaryKey"`
	Name         string  `gorm:"column:name"`
	ShortName    *string `gorm:"column:short_name"`
	ChannelCount int     `gorm:"column:channel_count"`
	DefinitionID string  `gorm:"column:definition_id;index"`

	// Relations
	ModeChannels []ModeChannel `gorm:"foreignKey:ModeID"`
}

func (FixtureMode) TableName() string { return "fixture_modes" }

// ModeChannel represents the mapping of channels to modes.
// Table: mode_channels
type ModeChannel struct {
	ID        string `gorm:"column:id;primaryKey"`
	ModeID    string `gorm:"column:mode_id;index"`
	ChannelID string `gorm:"column:channel_id;index"`
	Offset    int    `gorm:"column:offset"`
}

func (ModeChannel) TableName() string { return "mode_channels" }

// FixtureInstance represents a physical fixture instance in a project.
// Table: fixture_instances
type FixtureInstance struct {
	ID           string  `gorm:"column:id;primaryKey"`
	Name         string  `gorm:"column:name"`
	Description  *string `gorm:"column:description"`
	DefinitionID string  `gorm:"column:definition_id;index"`
	Manufacturer *string `gorm:"column:manufacturer"` // Denormalized
	Model        *string `gorm:"column:model"`        // Denormalized
	Type         *string `gorm:"column:type"`         // Denormalized
	ModeName     *string `gorm:"column:mode_name"`
	ChannelCount *int    `gorm:"column:channel_count"`
	ProjectID    string  `gorm:"column:project_id;index"`
	Universe     int     `gorm:"column:universe"`
	StartChannel int     `gorm:"column:start_channel"`
	Tags         *string `gorm:"column:tags;default:[]"` // JSON array

	ProjectOrder   *int     `gorm:"column:project_order"`
	LayoutX        *float64 `gorm:"column:layout_x"`
	LayoutY        *float64 `gorm:"column:layout_y"`
	LayoutRotation *float64 `gorm:"column:layout_rotation"`

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	Definition *FixtureDefinition `gorm:"foreignKey:DefinitionID"`
	Channels   []InstanceChannel  `gorm:"foreignKey:FixtureID"`
}

func (FixtureInstance) TableName() string { return "fixture_instances" }

// InstanceChannel represents a channel on a fixture instance.
// Table: instance_channels
type InstanceChannel struct {
	ID           string `gorm:"column:id;primaryKey"`
	FixtureID    string `gorm:"column:fixture_id;index"`
	Offset       int    `gorm:"column:offset"`
	Name         string `gorm:"column:name"`
	Type         string `gorm:"column:type"`
	MinValue     int    `gorm:"column:min_value;default:0"`
	MaxValue     int    `gorm:"column:max_value;default:255"`
	DefaultValue int    `gorm:"column:default_value;default:0"`
	FadeBehavior string `gorm:"column:fade_behavior;default:FADE"` // FadeBehavior enum: FADE, SNAP, SNAP_END
	IsDiscrete   bool   `gorm:"column:is_discrete;default:false"`  // True if channel has multiple discrete DMX ranges
}

func (InstanceChannel) TableName() string { return "instance_channels" }

// Look represents a lighting look (formerly known as "scene").
// A look captures a snapshot of fixture channel values that can be recalled.
// Table: looks
type Look struct {
	ID          string    `gorm:"column:id;primaryKey"`
	Name        string    `gorm:"column:name"`
	Description *string   `gorm:"column:description"`
	ProjectID   string    `gorm:"column:project_id;index"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	FixtureValues []FixtureValue `gorm:"foreignKey:LookID"`
}

func (Look) TableName() string { return "looks" }

// ChannelValue represents a single channel's value in a look
type ChannelValue struct {
	Offset int `json:"offset"`
	Value  int `json:"value"`
}

// FixtureValue represents fixture channel values within a look.
// Table: fixture_values
type FixtureValue struct {
	ID        string `gorm:"column:id;primaryKey"`
	LookID    string `gorm:"column:look_id;index"`
	FixtureID string `gorm:"column:fixture_id;index"`
	Channels  string `gorm:"column:channels;default:[]"` // JSON array of ChannelValue
	LookOrder *int   `gorm:"column:look_order"`
}

func (FixtureValue) TableName() string { return "fixture_values" }

// CueList represents a cue list (sequence of cues).
// Table: cue_lists
type CueList struct {
	ID          string    `gorm:"column:id;primaryKey"`
	Name        string    `gorm:"column:name"`
	Description *string   `gorm:"column:description"`
	Loop        bool      `gorm:"column:loop;default:false"`
	ProjectID   string    `gorm:"column:project_id;index"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	Cues []Cue `gorm:"foreignKey:CueListID"`
}

func (CueList) TableName() string { return "cue_lists" }

// Cue represents a lighting cue within a cue list.
// Table: cues
type Cue struct {
	ID          string    `gorm:"column:id;primaryKey"`
	Name        string    `gorm:"column:name"`
	CueNumber   float64   `gorm:"column:cue_number"`
	CueListID   string    `gorm:"column:cue_list_id;index"`
	LookID      string    `gorm:"column:look_id;index"`
	FadeInTime  float64   `gorm:"column:fade_in_time;default:0"`
	FadeOutTime float64   `gorm:"column:fade_out_time;default:0"`
	FollowTime  *float64  `gorm:"column:follow_time"`
	EasingType  *string   `gorm:"column:easing_type"`
	Notes       *string   `gorm:"column:notes"`
	Skip        bool      `gorm:"column:skip;default:false"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	Look    *Look       `gorm:"foreignKey:LookID"`
	Effects []CueEffect `gorm:"foreignKey:CueID"`
}

func (Cue) TableName() string { return "cues" }

// PreviewSession represents a preview session.
// Table: preview_sessions
type PreviewSession struct {
	ID        string    `gorm:"column:id;primaryKey"`
	ProjectID string    `gorm:"column:project_id;index"`
	UserID    string    `gorm:"column:user_id;index"`
	IsActive  bool      `gorm:"column:is_active;default:true"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (PreviewSession) TableName() string { return "preview_sessions" }

// Setting represents a system setting.
// Table: settings
type Setting struct {
	ID        string    `gorm:"column:id;primaryKey"`
	Key       string    `gorm:"column:key;uniqueIndex"`
	Value     string    `gorm:"column:value"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (Setting) TableName() string { return "settings" }

// LookBoard represents a look board for organizing looks (formerly SceneBoard).
// Table: look_boards
type LookBoard struct {
	ID              string    `gorm:"column:id;primaryKey"`
	Name            string    `gorm:"column:name"`
	Description     *string   `gorm:"column:description"`
	ProjectID       string    `gorm:"column:project_id;index"`
	DefaultFadeTime float64   `gorm:"column:default_fade_time;default:3.0"`
	GridSize        *int      `gorm:"column:grid_size;default:50"`
	CanvasWidth     int       `gorm:"column:canvas_width;default:2000"`
	CanvasHeight    int       `gorm:"column:canvas_height;default:2000"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	Buttons []LookBoardButton `gorm:"foreignKey:LookBoardID"`
}

func (LookBoard) TableName() string { return "look_boards" }

// LookBoardButton represents a button on a look board (formerly SceneBoardButton).
// Table: look_board_buttons
type LookBoardButton struct {
	ID          string    `gorm:"column:id;primaryKey"`
	LookBoardID string    `gorm:"column:look_board_id;index"`
	LookID      string    `gorm:"column:look_id;index"`
	LayoutX     int       `gorm:"column:layout_x"`
	LayoutY     int       `gorm:"column:layout_y"`
	Width       *int      `gorm:"column:width;default:200"`
	Height      *int      `gorm:"column:height;default:120"`
	Color       *string   `gorm:"column:color"`
	Label       *string   `gorm:"column:label"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	Look *Look `gorm:"foreignKey:LookID"`
}

func (LookBoardButton) TableName() string { return "look_board_buttons" }

// Effect represents an FX Engine effect definition.
// Effects can be WAVEFORM (oscillating), CROSSFADE, STATIC, or MASTER types.
// Table: effects
type Effect struct {
	ID          string  `gorm:"column:id;primaryKey"`
	Name        string  `gorm:"column:name"`
	Description *string `gorm:"column:description"`
	ProjectID   string  `gorm:"column:project_id;index"`
	Project     *Project

	EffectType      string `gorm:"column:effect_type;default:WAVEFORM"`   // WAVEFORM, CROSSFADE, STATIC, MASTER
	PriorityBand    string `gorm:"column:priority_band;default:USER"`     // BASE, USER, CUE, SYSTEM
	PrioritySub     int    `gorm:"column:priority_sub;default:50"`        // 0-100
	CompositionMode string `gorm:"column:composition_mode;default:OVERRIDE"` // OVERRIDE, ADDITIVE, MULTIPLY
	OnCueChange     string `gorm:"column:on_cue_change;default:FADE_OUT"` // FADE_OUT, PERSIST, SNAP_OFF, CROSSFADE_PARAMS
	FadeDuration    *float64 `gorm:"column:fade_duration"`

	// For WAVEFORM effects
	EasingType  *string  `gorm:"column:easing_type"`
	Waveform    *string  `gorm:"column:waveform"`                  // SINE, COSINE, SQUARE, SAWTOOTH, TRIANGLE, RANDOM
	Frequency   float64  `gorm:"column:frequency;default:1.0"`     // Hz
	Amplitude   float64  `gorm:"column:amplitude;default:100.0"`   // Percentage
	Offset      float64  `gorm:"column:offset;default:50.0"`       // Percentage (center value)
	PhaseOffset float64  `gorm:"column:phase_offset;default:0.0"`  // Degrees

	// For MASTER effects
	MasterValue *float64 `gorm:"column:master_value"` // 0.0-1.0

	// Relations
	Fixtures []EffectFixture `gorm:"foreignKey:EffectID;constraint:OnDelete:CASCADE"`

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (Effect) TableName() string { return "effects" }

// EffectFixture links effects to fixtures with per-fixture settings.
// Table: effect_fixtures
type EffectFixture struct {
	ID        string           `gorm:"column:id;primaryKey"`
	EffectID  string           `gorm:"column:effect_id;index"`
	FixtureID string           `gorm:"column:fixture_id;index"`
	Fixture   *FixtureInstance `gorm:"foreignKey:FixtureID"`

	PhaseOffset    *float64 `gorm:"column:phase_offset"`    // Per-fixture phase offset
	AmplitudeScale *float64 `gorm:"column:amplitude_scale"` // Per-fixture amplitude multiplier
	EffectOrder    *int     `gorm:"column:effect_order"`    // Order in the effect

	// Relations
	Channels []EffectChannel `gorm:"foreignKey:EffectFixtureID;constraint:OnDelete:CASCADE"`

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (EffectFixture) TableName() string { return "effect_fixtures" }

// EffectChannel defines per-channel overrides within an EffectFixture.
// Table: effect_channels
type EffectChannel struct {
	ID              string `gorm:"column:id;primaryKey"`
	EffectFixtureID string `gorm:"column:effect_fixture_id;index"`

	ChannelOffset *int    `gorm:"column:channel_offset"` // Which channel (offset from fixture start)
	ChannelType   *string `gorm:"column:channel_type"`   // INTENSITY, COLOR, PAN, TILT, etc.

	AmplitudeScale *float64 `gorm:"column:amplitude_scale"` // Per-channel amplitude
	FrequencyScale *float64 `gorm:"column:frequency_scale"` // Per-channel frequency multiplier

	// Min/Max define oscillation range (0-100%). When set, internally convert to offset/amplitude:
	// offset = (min + max) / 2, amplitude = (max - min) / 2
	MinValue *float64 `gorm:"column:min_value"` // Minimum oscillation value (0-100%)
	MaxValue *float64 `gorm:"column:max_value"` // Maximum oscillation value (0-100%)

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (EffectChannel) TableName() string { return "effect_channels" }

// CueEffect links effects to cues with runtime parameters.
// Table: cue_effects
type CueEffect struct {
	ID       string  `gorm:"column:id;primaryKey"`
	CueID    string  `gorm:"column:cue_id;index"`
	Cue      *Cue    `gorm:"foreignKey:CueID"`
	EffectID string  `gorm:"column:effect_id;index"`
	Effect   *Effect `gorm:"foreignKey:EffectID"`

	Intensity   float64 `gorm:"column:intensity;default:100.0"` // 0-100
	Speed       float64 `gorm:"column:speed;default:1.0"`       // Frequency multiplier
	OnCueChange *string `gorm:"column:on_cue_change"`           // Override effect's default behavior

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (CueEffect) TableName() string { return "cue_effects" }

// OFLImportMeta tracks the history of OFL imports.
// Table: ofl_import_meta
type OFLImportMeta struct {
	ID                string    `gorm:"column:id;primaryKey"`
	OFLVersion        string    `gorm:"column:ofl_version"`        // Commit SHA or version tag
	StartedAt         time.Time `gorm:"column:started_at"`         // When import started
	CompletedAt       time.Time `gorm:"column:completed_at"`       // When import completed
	TotalFixtures     int       `gorm:"column:total_fixtures"`     // Total fixtures in OFL
	SuccessfulImports int       `gorm:"column:successful_imports"` // Successfully imported
	FailedImports     int       `gorm:"column:failed_imports"`     // Failed to import
	SkippedDuplicates int       `gorm:"column:skipped_duplicates"` // Already existed
	UpdatedFixtures   int       `gorm:"column:updated_fixtures"`   // Updated existing fixtures
	UsedBundledData   bool      `gorm:"column:used_bundled_data"`  // True if imported from bundle
	ErrorMessage      *string   `gorm:"column:error_message"`      // Error if import failed
}

func (OFLImportMeta) TableName() string { return "ofl_import_meta" }

// Operation represents a recorded operation for undo/redo functionality.
// Each operation captures the state before and after a mutation, enabling reversal.
// Table: operations
type Operation struct {
	ID            string    `gorm:"column:id;primaryKey"`
	ProjectID     string    `gorm:"column:project_id;index"`
	OperationType string    `gorm:"column:operation_type"` // CREATE, UPDATE, DELETE, BULK
	EntityType    string    `gorm:"column:entity_type"`    // Look, FixtureInstance, Cue, etc.
	EntityID      string    `gorm:"column:entity_id"`
	Description   string    `gorm:"column:description"`
	PreviousState string    `gorm:"column:previous_state"` // JSON snapshot before
	NewState      string    `gorm:"column:new_state"`      // JSON snapshot after
	RelatedIDs    *string   `gorm:"column:related_ids"`    // JSON array for bulk ops
	Sequence      int       `gorm:"column:sequence;index"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Operation) TableName() string { return "operations" }

// OperationPointer tracks the current position in each project's undo/redo stack.
// The CurrentSequence indicates the most recently applied operation.
// Operations with sequence > CurrentSequence are "redo" candidates.
// Table: operation_pointers
type OperationPointer struct {
	ID              string    `gorm:"column:id;primaryKey"`
	ProjectID       string    `gorm:"column:project_id;uniqueIndex"`
	CurrentSequence int       `gorm:"column:current_sequence;default:0"`
	MaxSequence     int       `gorm:"column:max_sequence;default:0"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (OperationPointer) TableName() string { return "operation_pointers" }
