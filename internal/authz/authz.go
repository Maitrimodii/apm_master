// Package authz defines the permission vocabulary shared by route
// registration, permission seeding and the permission middleware.
package authz

import (
	"context"

	"github.com/google/uuid"
)

// Action is a permission verb. Values match the permission_action DB enum.
type Action string

const (
	Read    Action = "read"
	Write   Action = "write"
	Update  Action = "update"
	Delete  Action = "delete"
	Approve Action = "approve"
	Reject  Action = "reject"
)

// Resource is the thing a permission applies to. Names are plural and
// match the API route they protect.
type Resource string

const (
	Invites        Resource = "invites"
	Permissions    Resource = "permissions"
	Roles          Resource = "roles"
	APIKeys        Resource = "api_keys"
	Traces         Resource = "traces"
	Logs           Resource = "logs"
	Metrics        Resource = "metrics"
	Alerts         Resource = "alerts"
	Investigations Resource = "investigations"
)

// Checker reports whether a user may perform action on resource in an org.
type Checker interface {
	CheckPermission(ctx context.Context, userID, orgID uuid.UUID, resource Resource, action Action) (bool, error)
}

// Permission is one entry in the default catalogue.
type Permission struct {
	Resource    Resource
	Action      Action
	Description string
}

// Catalogue lists every permission the API checks. It is seeded and granted
// to the admin role when an organization is registered.
var Catalogue = []Permission{
	{Invites, Write, "Invite users to the organization"},
	{Invites, Read, "View organization invites"},
	{Invites, Delete, "Cancel pending invites"},
	{Permissions, Read, "View permissions"},
	{Permissions, Write, "Create permissions"},
	{Permissions, Update, "Update permissions"},
	{Permissions, Delete, "Delete permissions"},
	{Roles, Read, "View roles"},
	{Roles, Write, "Create roles"},
	{Roles, Update, "Update roles"},
	{Roles, Delete, "Delete roles"},
	{Traces, Read, "View traces"},
	{Logs, Read, "View logs"},
	{Metrics, Read, "View metrics"},
	{Alerts, Read, "View alerts"},
	{Alerts, Write, "Create and share alerts"},
	{Alerts, Update, "Update alerts"},
	{Alerts, Delete, "Delete alerts"},
	{APIKeys, Read, "View API keys"},
	{APIKeys, Write, "Create API keys"},
	{APIKeys, Delete, "Revoke API keys"},
	{Investigations, Read, "View incident investigations, reports, and evidence"},
	{Investigations, Write, "Create, retry, and cancel incident investigations"},
}
