package interfaces

import (
	"time"

	"gorm.io/gorm"
)

type BaseInterface interface {
	GetID() int
	SetID(id int)
	GetUuid() string
	SetUuid(uuid string)
	GetCreatedAt() time.Time
	SetCreatedAt(time.Time)
	GetCreatedById() int
	SetCreatedById(currentUser BaseInterface)
	GetUpdatedAt() time.Time
	SetUpdatedAt(time.Time)
	GetUpdatedById() int
	SetUpdatedById(currentUser BaseInterface)
	GetDeletedAt() gorm.DeletedAt
	SetDeletedAt(gorm.DeletedAt)
	GetDeletedById() int
	SetDeletedById(currentUser BaseInterface)
}

type Base struct {
	ID          *int           `gorm:"primaryKey;autoIncrement;not null;unique" json:"-"`
	Uuid        *string        `gorm:"type:uuid;default:gen_random_uuid();not null;unique" json:"uuid"`
	CreatedAt   time.Time      `json:"createdAt" gorm:"autoCreateTime" `
	CreatedById *int           `json:"-"`
	UpdatedAt   time.Time      `json:"updatedAt" gorm:"autoUpdateTime" `
	UpdatedById *int           `json:"-"`
	DeletedAt   gorm.DeletedAt `json:"-"`
	DeletedById *int           `json:"-"`
}

func (b *Base) GetID() int {
	if nil == b.ID {
		b.ID = new(int)
	}
	return *b.ID
}

func (b *Base) SetID(id int) {
	if nil == b.ID {
		b.ID = new(int)
	}
	*b.ID = id
}

func (b *Base) GetUuid() string {
	if nil == b.Uuid {
		b.Uuid = new(string)
	}
	return *b.Uuid
}

func (b *Base) SetUuid(uuid string) {
	if nil == b.Uuid {
		b.Uuid = new(string)
	}
	*b.Uuid = uuid
}

func (b *Base) GetCreatedAt() time.Time { return b.CreatedAt }

func (b *Base) SetCreatedAt(createdAt time.Time) { b.CreatedAt = createdAt }

func (b *Base) GetCreatedById() int {
	if nil == b.CreatedById {
		b.CreatedById = new(int)
	}
	return *b.CreatedById
}

func (b *Base) SetCreatedById(currentUser BaseInterface) {
	if currentUser == nil {
		return
	}
	if nil == b.CreatedById {
		b.CreatedById = new(int)
	}
	*b.CreatedById = currentUser.GetID()
}

func (b *Base) GetUpdatedAt() time.Time { return b.UpdatedAt }

func (b *Base) SetUpdatedAt(updatedAt time.Time) { b.UpdatedAt = updatedAt }

func (b *Base) GetUpdatedById() int {
	if nil == b.UpdatedById {
		b.UpdatedById = new(int)
	}
	return *b.UpdatedById
}

func (b *Base) SetUpdatedById(currentUser BaseInterface) {
	if currentUser == nil {
		return
	}
	if nil == b.UpdatedById {
		b.UpdatedById = new(int)
	}
	*b.UpdatedById = currentUser.GetID()
}

func (b *Base) GetDeletedAt() gorm.DeletedAt { return b.DeletedAt }

func (b *Base) SetDeletedAt(deletedAt gorm.DeletedAt) { b.DeletedAt = deletedAt }

func (b *Base) GetDeletedById() int {
	if nil == b.DeletedById {
		b.DeletedById = new(int)
	}
	return *b.DeletedById
}

func (b *Base) SetDeletedById(currentUser BaseInterface) {
	if currentUser == nil {
		return
	}
	if nil == b.DeletedById {
		b.DeletedById = new(int)
	}
	*b.DeletedById = currentUser.GetID()
}
