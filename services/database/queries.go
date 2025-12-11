package database

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/providers"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DataWriter struct {
	CurrentUser interfaces.BaseInterface
}

func CreateDataWriter(c *gin.Context) DataWriter {
	currentUser := providers.GetCurrentUser(c)

	if currentUser == nil {
		return DataWriter{nil}
	}

	return DataWriter{currentUser}
}

func GetAll(objs []*interfaces.BaseInterface, query *gorm.DB) {
	query.Find(&objs)
}

func GetOneBy(obj interfaces.BaseInterface, values map[string]any) {
	JoinTables(values).Preload(clause.Associations).First(obj)
}

func (dw DataWriter) Create(obj interfaces.BaseInterface) error {
	obj.SetCreatedById(dw.CurrentUser)
	obj.SetUpdatedById(dw.CurrentUser)
	return providers.GetDB().Create(obj).Error
}

func (dw DataWriter) Update(obj interfaces.BaseInterface) error {
	obj.SetUpdatedById(dw.CurrentUser)
	obj.SetUpdatedAt(time.Now())

	if err := providers.GetDB().Model(obj).Where("uuid = ?", obj.GetUuid()).UpdateColumns(obj).Error; err != nil {
		return err
	}

	return providers.GetDB().First(obj, "uuid = ?", obj.GetUuid()).Error
}

func (dw DataWriter) Delete(obj interfaces.BaseInterface) error {
	obj.SetDeletedById(dw.CurrentUser)
	return providers.GetDB().Delete(obj).Error
}

func HardDelete(obj interfaces.BaseInterface) error {
	return providers.GetDB().Unscoped().Delete(obj).Error
}
