// Package repositories provides data access layer implementations.
package repositories

import (
	"context"
	"errors"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// GroupRepository handles user group and group member data access.
type GroupRepository struct {
	db *gorm.DB
}

// NewGroupRepository creates a new GroupRepository.
func NewGroupRepository(db *gorm.DB) *GroupRepository {
	return &GroupRepository{db: db}
}

// FindGroupByID returns a group by ID.
func (r *GroupRepository) FindGroupByID(ctx context.Context, id string) (*models.UserGroup, error) {
	var group models.UserGroup
	if err := r.db.WithContext(ctx).First(&group, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &group, nil
}

// FindMember returns a specific group membership.
func (r *GroupRepository) FindMember(ctx context.Context, userID, groupID string) (*models.UserGroupMember, error) {
	var member models.UserGroupMember
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND group_id = ?", userID, groupID).
		First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &member, nil
}

// UpdateMemberRole updates a group member's role.
func (r *GroupRepository) UpdateMemberRole(ctx context.Context, userID, groupID, role string) error {
	result := r.db.WithContext(ctx).
		Model(&models.UserGroupMember{}).
		Where("user_id = ? AND group_id = ?", userID, groupID).
		Update("role", role)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// AddMember adds a user to a group with the specified role.
func (r *GroupRepository) AddMember(ctx context.Context, userID, groupID, role string) (*models.UserGroupMember, error) {
	member := &models.UserGroupMember{
		ID:      cuid.New(),
		UserID:  userID,
		GroupID: groupID,
		Role:    role,
	}
	if err := r.db.WithContext(ctx).Create(member).Error; err != nil {
		return nil, err
	}
	return member, nil
}

// RemoveMember removes a user from a group.
func (r *GroupRepository) RemoveMember(ctx context.Context, userID, groupID string) (bool, error) {
	result := r.db.WithContext(ctx).
		Delete(&models.UserGroupMember{}, "user_id = ? AND group_id = ?", userID, groupID)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// AddDeviceToGroup adds a device to a group.
func (r *GroupRepository) AddDeviceToGroup(ctx context.Context, deviceID, groupID string) (*models.DeviceGroupMember, error) {
	member := &models.DeviceGroupMember{
		ID:       cuid.New(),
		DeviceID: deviceID,
		GroupID:  groupID,
	}
	if err := r.db.WithContext(ctx).Create(member).Error; err != nil {
		return nil, err
	}
	return member, nil
}

// FindDeviceGroupMember returns a device's membership in a specific group.
func (r *GroupRepository) FindDeviceGroupMember(ctx context.Context, deviceID, groupID string) (*models.DeviceGroupMember, error) {
	var member models.DeviceGroupMember
	if err := r.db.WithContext(ctx).
		Where("device_id = ? AND group_id = ?", deviceID, groupID).
		First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &member, nil
}

// RemoveDeviceFromGroup removes a device from a group.
func (r *GroupRepository) RemoveDeviceFromGroup(ctx context.Context, deviceID, groupID string) (bool, error) {
	result := r.db.WithContext(ctx).
		Delete(&models.DeviceGroupMember{}, "device_id = ? AND group_id = ?", deviceID, groupID)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// FindDeviceByID returns a device by ID.
func (r *GroupRepository) FindDeviceByID(ctx context.Context, id string) (*models.Device, error) {
	var device models.Device
	if err := r.db.WithContext(ctx).First(&device, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &device, nil
}

// FindUserByEmail returns a user by email.
func (r *GroupRepository) FindUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}
