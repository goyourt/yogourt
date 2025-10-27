package interfaces

type BaseInterface interface {
	GetID() int
	GetUUID() string
	GetCreatedAt() string
	GetCreatedBy() string
	Create(int)
	GetUpdatedAt() string
	GetUpdatedBy() string
	Update(int)
	GetDeletedAt() string
	GetDeletedBy() string
	Delete(int)
}

type Base struct {
	ID        int    `gorm:"primaryKey;autoIncrement;not null;unique" json:"id"`
	UUID      string `gorm:"type:uuid;default:uuid_generate_v4();not null;unique" json:"uuid"`
	CreatedAt string `gorm:"autoCreateTime" json:"created_at"`
	CreatedBy string `json:"created_by"`
	UpdatedAt string `gorm:"autoUpdateTime" json:"updated_at"`
	UpdatedBy string `json:"updated_by"`
	DeletedAt string `gorm:"index" json:"deleted_at"`
	DeletedBy string `json:"deleted_by"`
}

func (b *Base) GetID() int { return b.ID }

func (b *Base) GetUUID() string { return b.UUID }

func (b *Base) GetCreatedAt() string { return b.CreatedAt }

func (b *Base) GetCreatedBy() string { return b.CreatedBy }

func (b *Base) Create(int) {
	//TODO implement me
	panic("implement me")

}

func (b *Base) GetUpdatedAt() string {
	//TODO implement me
	panic("implement me")
}

func (b *Base) GetUpdatedBy() string {
	//TODO implement me
	panic("implement me")
}

func (b *Base) Update(int) {
	//TODO implement me
	panic("implement me")
}

func (b *Base) GetDeletedAt() string {
	//TODO implement me
	panic("implement me")
}

func (b *Base) GetDeletedBy() string {
	//TODO implement me
	panic("implement me")
}

func (b *Base) Delete(int) {
	//TODO implement me
	panic("implement me")
}
