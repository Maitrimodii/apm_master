package models

import (
	"github.com/google/uuid"
)

type PermissionAction string

const (
	Read    PermissionAction = "read"
	Write   PermissionAction = "write"
	Update  PermissionAction = "update"
	Delete  PermissionAction = "delete"
	Approve PermissionAction = "approve"
	Reject  PermissionAction = "reject"
)

type Permission struct {
	ID          uuid.UUID
	Name        string
	Action      PermissionAction
	Description string
}
