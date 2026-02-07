package resolvers

// This file contains implementations for group admin management and device
// scoping GraphQL resolvers (Phase 5+6).
// The actual resolver stubs are generated in schema.resolvers.go by gqlgen.
// These helper functions are called from those stubs.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lucsky/cuid"
	"gorm.io/gorm"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/middleware"
)

// Errors specific to group admin operations.
var (
	ErrInvitationNotFound = errors.New("invitation not found")
	ErrInvitationExpired  = errors.New("invitation has expired")
	ErrInvitationNotPending = errors.New("invitation is not in pending status")
	ErrAlreadyMember      = errors.New("user is already a member of this group")
	ErrDeviceAlreadyInGroup = errors.New("device is already in this group")
	ErrEmailMismatch      = errors.New("invitation email does not match your account")
	ErrCannotDemoteOwner  = errors.New("cannot change the role of the group owner")
)

// updateGroupMemberRole updates a group member's role.
// Requires GROUP_ADMIN or system admin.
func (r *Resolver) updateGroupMemberRole(ctx context.Context, groupID, userID string, role generated.GroupMemberRole) (bool, error) {
	if err := r.requireGroupAdmin(ctx, groupID); err != nil {
		return false, err
	}

	// Verify the group exists
	group, err := r.GroupRepo.FindGroupByID(ctx, groupID)
	if err != nil {
		return false, err
	}
	if group == nil {
		return false, ErrGroupNotFound
	}

	// Prevent changing the role of the group owner
	if group.OwnerID != nil && *group.OwnerID == userID {
		return false, ErrCannotDemoteOwner
	}

	// Verify the member exists
	member, err := r.GroupRepo.FindMember(ctx, userID, groupID)
	if err != nil {
		return false, err
	}
	if member == nil {
		return false, fmt.Errorf("user is not a member of this group")
	}

	// Update the role
	if err := r.GroupRepo.UpdateMemberRole(ctx, userID, groupID, string(role)); err != nil {
		return false, err
	}

	return true, nil
}

// inviteToGroup creates an invitation for a user to join a group.
// Requires GROUP_ADMIN or system admin.
func (r *Resolver) inviteToGroup(ctx context.Context, groupID, email string, role *generated.GroupMemberRole) (*models.GroupInvitation, error) {
	if err := r.requireGroupAdmin(ctx, groupID); err != nil {
		return nil, err
	}

	// Verify the group exists
	group, err := r.GroupRepo.FindGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, ErrGroupNotFound
	}

	// Check if the user is already a member by email
	existingUser, err := r.GroupRepo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existingUser != nil {
		existing, err := r.GroupRepo.FindMember(ctx, existingUser.ID, groupID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return nil, ErrAlreadyMember
		}
	}

	// Check for an existing pending invitation
	existingInvitation, err := r.InvitationRepo.FindPendingByGroupAndEmail(ctx, groupID, email)
	if err != nil {
		return nil, err
	}
	if existingInvitation != nil {
		// Return the existing invitation rather than creating a duplicate
		return existingInvitation, nil
	}

	// Determine the role (default to MEMBER)
	memberRole := models.GroupRoleMember
	if role != nil {
		memberRole = string(*role)
	}

	// Get the inviting user's ID
	invitedByID := middleware.GetUserIDFromContext(ctx)
	if invitedByID == "" {
		// If auth is disabled (no user in context), use a placeholder
		invitedByID = "system"
	}

	invitation, err := r.InvitationRepo.CreateWithDefaults(ctx, repositories.GroupInvitationInput{
		GroupID:     groupID,
		Email:       email,
		InvitedByID: invitedByID,
		Role:        memberRole,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create invitation: %w", err)
	}

	return invitation, nil
}


// acceptInvitation accepts a group invitation and adds the user to the group.
// The authenticated user's email must match the invitation email.
func (r *Resolver) acceptInvitation(ctx context.Context, invitationID string) (bool, error) {
	// Get the authenticated user's email
	email := middleware.GetUserEmailFromContext(ctx)
	userID := middleware.GetUserIDFromContext(ctx)

	// Auth-disabled mode: allow acceptance without email verification
	authDisabled := r.AuthService == nil || !r.AuthService.IsEnabled()

	// Use a transaction to atomically validate, accept the invitation, and add the member.
	// This prevents race conditions where concurrent accepts could both pass validation.
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Load invitation inside the transaction for consistency
		invitation, err := r.InvitationRepo.FindByIDForUpdate(tx, invitationID)
		if err != nil {
			return err
		}
		if invitation == nil {
			return ErrInvitationNotFound
		}

		if invitation.Status != models.InvitationStatusPending {
			return ErrInvitationNotPending
		}

		if invitation.ExpiresAt.Before(time.Now()) {
			// Mark as expired
			_ = tx.Model(&models.GroupInvitation{}).
				Where("id = ?", invitationID).
				Update("status", models.InvitationStatusExpired)
			return ErrInvitationExpired
		}

		// Verify email match (skip if auth is disabled)
		if !authDisabled && email != invitation.Email {
			return ErrEmailMismatch
		}

		// Mark invitation as accepted
		now := time.Now()
		if err := tx.Model(&models.GroupInvitation{}).
			Where("id = ?", invitationID).
			Updates(map[string]interface{}{
				"status":      models.InvitationStatusAccepted,
				"accepted_at": &now,
			}).Error; err != nil {
			return err
		}

		// Determine the user ID for the new member
		memberUserID := userID
		if memberUserID == "" {
			// Look up user by invitation email (use tx to stay within the transaction)
			var user models.User
			if err := tx.Where("email = ?", invitation.Email).First(&user).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("no user found for email %s", invitation.Email)
				}
				return err
			}
			memberUserID = user.ID
		}

		// Check if already a member (idempotent)
		var count int64
		if err := tx.Model(&models.UserGroupMember{}).
			Where("user_id = ? AND group_id = ?", memberUserID, invitation.GroupID).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil // Already a member, just accept the invitation
		}

		// Add the user as a member with the invitation's role
		member := &models.UserGroupMember{
			ID:      cuid.New(),
			UserID:  memberUserID,
			GroupID: invitation.GroupID,
			Role:    invitation.Role,
		}
		return tx.Create(member).Error
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

// declineInvitation declines a group invitation.
func (r *Resolver) declineInvitation(ctx context.Context, invitationID string) (bool, error) {
	invitation, err := r.InvitationRepo.FindByID(ctx, invitationID)
	if err != nil {
		return false, err
	}
	if invitation == nil {
		return false, ErrInvitationNotFound
	}

	if invitation.Status != models.InvitationStatusPending {
		return false, ErrInvitationNotPending
	}

	// Verify the user is allowed to decline (their email matches or auth is disabled)
	authDisabled := r.AuthService == nil || !r.AuthService.IsEnabled()
	if !authDisabled {
		email := middleware.GetUserEmailFromContext(ctx)
		if email != invitation.Email {
			return false, ErrEmailMismatch
		}
	}

	if err := r.InvitationRepo.UpdateStatus(ctx, invitationID, models.InvitationStatusDeclined, nil); err != nil {
		return false, err
	}

	return true, nil
}

// cancelInvitation cancels a pending group invitation.
// Requires GROUP_ADMIN of the invitation's group or system admin.
func (r *Resolver) cancelInvitation(ctx context.Context, invitationID string) (bool, error) {
	invitation, err := r.InvitationRepo.FindByID(ctx, invitationID)
	if err != nil {
		return false, err
	}
	if invitation == nil {
		return false, ErrInvitationNotFound
	}

	// Require group admin for the invitation's group
	if err := r.requireGroupAdmin(ctx, invitation.GroupID); err != nil {
		return false, err
	}

	if invitation.Status != models.InvitationStatusPending {
		return false, ErrInvitationNotPending
	}

	// We reuse the "DECLINED" status to indicate cancellation, but we could also
	// add a CANCELLED status. For now, declined is sufficient.
	if err := r.InvitationRepo.UpdateStatus(ctx, invitationID, models.InvitationStatusDeclined, nil); err != nil {
		return false, err
	}

	return true, nil
}

// addDeviceToGroup adds a device to a group.
// Requires GROUP_ADMIN or system admin.
func (r *Resolver) addDeviceToGroup(ctx context.Context, deviceID, groupID string) (bool, error) {
	if err := r.requireGroupAdmin(ctx, groupID); err != nil {
		return false, err
	}

	// Verify the group exists
	group, err := r.GroupRepo.FindGroupByID(ctx, groupID)
	if err != nil {
		return false, err
	}
	if group == nil {
		return false, ErrGroupNotFound
	}

	// Verify the device exists
	device, err := r.GroupRepo.FindDeviceByID(ctx, deviceID)
	if err != nil {
		return false, err
	}
	if device == nil {
		return false, ErrDeviceNotFound
	}

	// Check if already in the group
	existing, err := r.GroupRepo.FindDeviceGroupMember(ctx, deviceID, groupID)
	if err != nil {
		return false, err
	}
	if existing != nil {
		return false, ErrDeviceAlreadyInGroup
	}

	_, err = r.GroupRepo.AddDeviceToGroup(ctx, deviceID, groupID)
	if err != nil {
		return false, fmt.Errorf("failed to add device to group: %w", err)
	}

	return true, nil
}

// removeDeviceFromGroup removes a device from a group.
// Requires GROUP_ADMIN or system admin.
func (r *Resolver) removeDeviceFromGroup(ctx context.Context, deviceID, groupID string) (bool, error) {
	if err := r.requireGroupAdmin(ctx, groupID); err != nil {
		return false, err
	}

	removed, err := r.GroupRepo.RemoveDeviceFromGroup(ctx, deviceID, groupID)
	if err != nil {
		return false, err
	}

	return removed, nil
}

// addUserToGroupWithRole adds a user to a group with a specific role.
// Requires GROUP_ADMIN or system admin.
func (r *Resolver) addUserToGroupWithRole(ctx context.Context, userID, groupID string, role *generated.GroupMemberRole) (bool, error) {
	if err := r.requireGroupAdmin(ctx, groupID); err != nil {
		return false, err
	}

	// Determine the role (default to MEMBER)
	memberRole := models.GroupRoleMember
	if role != nil {
		memberRole = string(*role)
	}

	// Check if already a member
	existing, err := r.GroupRepo.FindMember(ctx, userID, groupID)
	if err != nil {
		return false, err
	}
	if existing != nil {
		return false, ErrAlreadyMember
	}

	_, err = r.GroupRepo.AddMember(ctx, userID, groupID, memberRole)
	if err != nil {
		return false, fmt.Errorf("failed to add user to group: %w", err)
	}

	return true, nil
}

// approveDeviceWithGroup approves a device and optionally assigns it to a group.
func (r *Resolver) approveDeviceWithGroup(ctx context.Context, deviceID string, permissions generated.DevicePermissions, groupID *string) (*models.Device, error) {
	device, err := r.approveDevice(ctx, deviceID, permissions)
	if err != nil {
		return nil, err
	}

	// If a group ID is provided, add the device to that group
	if groupID != nil && *groupID != "" {
		// Verify the group exists
		group, err := r.GroupRepo.FindGroupByID(ctx, *groupID)
		if err != nil {
			return device, fmt.Errorf("device approved but failed to add to group: %w", err)
		}
		if group == nil {
			return device, fmt.Errorf("device approved but group not found: %s", *groupID)
		}

		// Check if already in the group
		existing, err := r.GroupRepo.FindDeviceGroupMember(ctx, deviceID, *groupID)
		if err != nil {
			return device, fmt.Errorf("device approved but failed to check group membership: %w", err)
		}
		if existing == nil {
			_, err = r.GroupRepo.AddDeviceToGroup(ctx, deviceID, *groupID)
			if err != nil {
				return device, fmt.Errorf("device approved but failed to add to group: %w", err)
			}
		}
	}

	return device, nil
}
