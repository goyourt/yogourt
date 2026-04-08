package database

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/providers"
	"gorm.io/gorm"
)

type DataWriter struct {
	CurrentUser interfaces.BaseInterface
}

func CreateDataWriter(c *gin.Context) DataWriter {
	if c == nil {
		return DataWriter{nil}
	}

	currentUser := providers.GetCurrentUser(c)

	if currentUser == nil {
		return DataWriter{nil}
	}

	return DataWriter{currentUser}
}

func GetAll[T interfaces.BaseInterface](objs *[]T, values map[string]any) {
	GetAllPaginated(objs, values, 0, 0)
}

func GetAllPaginated[T interfaces.BaseInterface](objs *[]T, values map[string]any, page int, pageSize int) {
	SearchQuery(values, objs, page, pageSize).Find(objs)
}

func GetOneBy(obj interfaces.BaseInterface, values map[string]any) {
	if obj.GetID() == 0 {
		resetId(obj)
	}
	JoinTables(values, &obj).First(obj)
}

func (dw DataWriter) Create(obj interfaces.BaseInterface) error {
	resetId(obj)
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

func SearchQuery[T interfaces.BaseInterface](values map[string]any, objs *[]T, page int, pageSize int) *gorm.DB {
	return Paginate(JoinTables(values, new(T)), page, pageSize)
}

func Paginate(query *gorm.DB, page int, pageSize int) *gorm.DB {
	if page < 1 || pageSize < 1 {
		return query
	}
	offset := (page - 1) * pageSize
	return query.Limit(pageSize).Offset(offset)
}

func Like(s string) string {
	return likePatern + "%" + s + "%"
}
