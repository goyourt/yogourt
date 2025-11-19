package interfaces

import (
	"time"

	"gorm.io/gorm"
)

type BaseInterface interface {
	GetID() int
	GetUuid() string
	GetCreatedAt() time.Time
	SetCreatedAt(time.Time)
	GetCreatedBy() *Base
	SetCreatedBy(*Base)
	GetUpdatedAt() time.Time
	SetUpdatedAt(time.Time)
	GetUpdatedBy() *Base
	SetUpdatedBy(*Base)
	GetDeletedAt() gorm.DeletedAt
	SetDeletedAt(gorm.DeletedAt)
	GetDeletedBy() *Base
	SetDeletedBy(*Base)
}

type Base struct {
	ID          int            `gorm:"primaryKey;autoIncrement;not null;unique" json:"-"`
	Uuid        string         `gorm:"type:uuid;default:gen_random_uuid();not null;unique" json:"uuid"`
	CreatedAt   time.Time      `json:"-" gorm:"autoCreateTime" `
	CreatedById *int           `json:"-"`
	CreatedBy   *Base          `json:"-"`
	UpdatedAt   time.Time      `json:"-" gorm:"autoUpdateTime" `
	UpdatedById *int           `json:"-"`
	UpdatedBy   *Base          `json:"-"`
	DeletedAt   gorm.DeletedAt `json:"-"`
	DeletedById *int           `json:"-"`
	DeletedBy   *Base          `json:"-"`
}

func (b *Base) GetID() int { return b.ID }

func (b *Base) GetUuid() string { return b.Uuid }

func (b *Base) GetCreatedAt() time.Time { return b.CreatedAt }

func (b *Base) SetCreatedAt(createdAt time.Time) { b.CreatedAt = createdAt }

func (b *Base) GetCreatedBy() *Base { return b.CreatedBy }

func (b *Base) SetCreatedBy(createdBy *Base) { b.CreatedBy = createdBy }

func (b *Base) GetUpdatedAt() time.Time { return b.UpdatedAt }

func (b *Base) SetUpdatedAt(updatedAt time.Time) { b.UpdatedAt = updatedAt }

func (b *Base) GetUpdatedBy() *Base { return b.UpdatedBy }

func (b *Base) SetUpdatedBy(updatedBy *Base) { b.UpdatedBy = updatedBy }

func (b *Base) GetDeletedAt() gorm.DeletedAt { return b.DeletedAt }

func (b *Base) SetDeletedAt(deletedAt gorm.DeletedAt) { b.DeletedAt = deletedAt }

func (b *Base) GetDeletedBy() *Base { return b.DeletedBy }

func (b *Base) SetDeletedBy(deletedBy *Base) { b.DeletedBy = deletedBy }
