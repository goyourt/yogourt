package database

import (
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/providers"
	"gorm.io/gorm"
)

type DataWriter struct {
	CurrentUser *interfaces.Base
}

func CreateDataWriter(c *gin.Context) DataWriter {
	ctx := c.Request.Context()
	currentUser, exist := ctx.Value("currentUser").(*interfaces.Base)

	if !exist {
		return DataWriter{nil}
	}

	return DataWriter{currentUser}
}

func GetAll(objs []*interfaces.Base, query *gorm.DB) {
	query.Find(&objs)
}

func GetOneBy(obj interfaces.BaseInterface, values map[string]any) {
	JoinTables(values).First(obj)
}

func (dw DataWriter) Create(obj interfaces.BaseInterface) error {
	obj.SetCreatedBy(dw.CurrentUser)
	obj.SetUpdatedBy(dw.CurrentUser)
	return providers.GetDB().Create(obj).Error
}

func (dw DataWriter) Update(obj interfaces.BaseInterface) error {
	obj.SetUpdatedBy(dw.CurrentUser)
	return providers.GetDB().Save(obj).Error
}

func (dw DataWriter) Delete(obj interfaces.BaseInterface) error {
	obj.SetDeletedBy(dw.CurrentUser)
	return providers.GetDB().Delete(obj).Error
}

func HardDelete(obj interfaces.BaseInterface) error {
	return providers.GetDB().Delete(obj).Error
}
