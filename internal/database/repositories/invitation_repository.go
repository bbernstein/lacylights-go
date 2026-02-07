// Package repositories provides data access layer implementations.
package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// InvitationRepository handles group invitation data access.
type InvitationRepository struct {
	db *gorm.DB
}

// NewInvitationRepository creates a new InvitationRepository.
func NewInvitationRepository(db *gorm.DB) *InvitationRepository {
	return &InvitationRepository{db: db}
}

// FindByID returns an invitation by ID.
func (r *InvitationRepository) FindByID(ctx context.Context, id string) (*models.GroupInvitation, error) {
	var invitation models.GroupInvitation
	if err := r.db.WithContext(ctx).First(&invitation, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &invitation, nil
}

// FindPendingByGroupAndEmail returns a pending invitation for a specific group and email.
func (r *InvitationRepository) FindPendingByGroupAndEmail(ctx context.Context, groupID, email string) (*models.GroupInvitation, error) {
	var invitation models.GroupInvitation
	if err := r.db.WithContext(ctx).
		Where("group_id = ? AND email = ? AND status = ?", groupID, email, models.InvitationStatusPending).
		First(&invitation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &invitation, nil
}

// Create creates a new group invitation.
func (r *InvitationRepository) Create(ctx context.Context, groupID, email, invitedByID, role string, expiresAt time.Time) (*models.GroupInvitation, error) {
	invitation := &models.GroupInvitation{
		ID:          cuid.New(),
		GroupID:     groupID,
		Email:       email,
		InvitedByID: invitedByID,
		Role:        role,
		Status:      models.InvitationStatusPending,
		ExpiresAt:   expiresAt,
	}
	if err := r.db.WithContext(ctx).Create(invitation).Error; err != nil {
		return nil, err
	}
	return invitation, nil
}

// UpdateStatus updates an invitation's status.
func (r *InvitationRepository) UpdateStatus(ctx context.Context, id, status string, acceptedAt *time.Time) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if acceptedAt != nil {
		updates["accepted_at"] = acceptedAt
	}
	result := r.db.WithContext(ctx).
		Model(&models.GroupInvitation{}).
		Where("id = ?", id).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// FindByGroupID returns all invitations for a group.
func (r *InvitationRepository) FindByGroupID(ctx context.Context, groupID string) ([]models.GroupInvitation, error) {
	var invitations []models.GroupInvitation
	if err := r.db.WithContext(ctx).
		Where("group_id = ?", groupID).
		Order("created_at DESC").
		Find(&invitations).Error; err != nil {
		return nil, err
	}
	return invitations, nil
}

// FindPendingByEmail returns all pending invitations for an email address.
func (r *InvitationRepository) FindPendingByEmail(ctx context.Context, email string) ([]models.GroupInvitation, error) {
	var invitations []models.GroupInvitation
	if err := r.db.WithContext(ctx).
		Where("email = ? AND status = ?", email, models.InvitationStatusPending).
		Order("created_at DESC").
		Find(&invitations).Error; err != nil {
		return nil, err
	}
	return invitations, nil
}

// ExpireOverdueInvitations marks invitations past their expiry as EXPIRED.
// Returns the number of expired invitations.
func (r *InvitationRepository) ExpireOverdueInvitations(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&models.GroupInvitation{}).
		Where("status = ? AND expires_at < ?", models.InvitationStatusPending, time.Now()).
		Update("status", models.InvitationStatusExpired)
	return result.RowsAffected, result.Error
}

// FindByIDForUpdate returns an invitation by ID for use inside a transaction.
// Caller is responsible for wrapping in a transaction.
func (r *InvitationRepository) FindByIDForUpdate(tx *gorm.DB, id string) (*models.GroupInvitation, error) {
	var invitation models.GroupInvitation
	if err := tx.First(&invitation, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &invitation, nil
}

// DB returns the underlying database connection for use in transactions.
func (r *InvitationRepository) DB() *gorm.DB {
	return r.db
}

// GroupInvitationInput holds the parameters for creating an invitation.
type GroupInvitationInput struct {
	GroupID     string
	Email       string
	InvitedByID string
	Role        string
	TTL         time.Duration // How long until the invitation expires
}

// CreateWithDefaults creates an invitation with sensible defaults.
func (r *InvitationRepository) CreateWithDefaults(ctx context.Context, input GroupInvitationInput) (*models.GroupInvitation, error) {
	ttl := input.TTL
	if ttl == 0 {
		ttl = 7 * 24 * time.Hour // Default: 7 days
	}
	role := input.Role
	if role == "" {
		role = models.GroupRoleMember
	}

	return r.Create(ctx, input.GroupID, input.Email, input.InvitedByID, role, time.Now().Add(ttl))
}

// NewInvitationID generates a new invitation ID.
func NewInvitationID() string {
	return cuid.New()
}
