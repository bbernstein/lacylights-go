// Package models contains the database model definitions.
// These models map directly to the SQLite database tables
// and are compatible with the lacylights-node Prisma schema.
package models

import (
	"time"
)

// User represents a user in the system.
// Table: users
type User struct {
	ID        string    `gorm:"column:id;primaryKey"`
	Email     string    `gorm:"column:email;uniqueIndex"`
	Name      *string   `gorm:"column:name"`
	Role      string    `gorm:"column:role;default:USER"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (User) TableName() string { return "users" }

// Project represents a lighting project.
// Table: projects
type Project struct {
	ID          string    `gorm:"column:id;primaryKey"`
	Name        string    `gorm:"column:name"`
	Description *string   `gorm:"column:description"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations (loaded separately)
	Fixtures  []FixtureInstance `gorm:"foreignKey:ProjectID"`
	Scenes    []Scene           `gorm:"foreignKey:ProjectID"`
	CueLists  []CueList         `gorm:"foreignKey:ProjectID"`
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

// Scene represents a lighting scene.
// Table: scenes
type Scene struct {
	ID          string    `gorm:"column:id;primaryKey"`
	Name        string    `gorm:"column:name"`
	Description *string   `gorm:"column:description"`
	ProjectID   string    `gorm:"column:project_id;index"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	FixtureValues []FixtureValue `gorm:"foreignKey:SceneID"`
}

func (Scene) TableName() string { return "scenes" }

// ChannelValue represents a single channel's value in a scene
type ChannelValue struct {
	Offset int `json:"offset"`
	Value  int `json:"value"`
}

// FixtureValue represents fixture channel values within a scene.
// Table: fixture_values
type FixtureValue struct {
	ID            string `gorm:"column:id;primaryKey"`
	SceneID       string `gorm:"column:scene_id;index"`
	FixtureID     string `gorm:"column:fixture_id;index"`
	Channels      string `gorm:"column:channels;default:[]"`         // JSON array of ChannelValue
	ChannelValues string `gorm:"column:channelValues"`               // DEPRECATED - kept for migration
	SceneOrder    *int   `gorm:"column:scene_order"`
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
	SceneID     string    `gorm:"column:scene_id;index"`
	FadeInTime  float64   `gorm:"column:fade_in_time;default:0"`
	FadeOutTime float64   `gorm:"column:fade_out_time;default:0"`
	FollowTime  *float64  `gorm:"column:follow_time"`
	EasingType  *string   `gorm:"column:easing_type"`
	Notes       *string   `gorm:"column:notes"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	Scene *Scene `gorm:"foreignKey:SceneID"`
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

// SceneBoard represents a scene board for organizing scenes.
// Table: scene_boards
type SceneBoard struct {
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
	Buttons []SceneBoardButton `gorm:"foreignKey:SceneBoardID"`
}

func (SceneBoard) TableName() string { return "scene_boards" }

// SceneBoardButton represents a button on a scene board.
// Table: scene_board_buttons
type SceneBoardButton struct {
	ID           string    `gorm:"column:id;primaryKey"`
	SceneBoardID string    `gorm:"column:scene_board_id;index"`
	SceneID      string    `gorm:"column:scene_id;index"`
	LayoutX      int       `gorm:"column:layout_x"`
	LayoutY      int       `gorm:"column:layout_y"`
	Width        *int      `gorm:"column:width;default:200"`
	Height       *int      `gorm:"column:height;default:120"`
	Color        *string   `gorm:"column:color"`
	Label        *string   `gorm:"column:label"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	Scene *Scene `gorm:"foreignKey:SceneID"`
}

func (SceneBoardButton) TableName() string { return "scene_board_buttons" }

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
