package models

import (
	"time"

	"github.com/google/uuid"
)

type Org struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	CreatedAt time.Time
	UpdatedAt time.Time
}
