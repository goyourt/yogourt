package database

import (
	"errors"
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

// GetAll loads every record matching values into objs. It returns GORM's
// error: a database outage is no longer indistinguishable from an empty
// result.
func GetAll[T interfaces.BaseInterface](objs *[]T, values map[string]any) error {
	return GetAllPaginated(objs, values, 0, 0)
}

// GetAllPaginated behaves like GetAll with pagination. It returns GORM's
// error.
func GetAllPaginated[T interfaces.BaseInterface](objs *[]T, values map[string]any, page int, pageSize int) error {
	return SearchQuery(values, objs, page, pageSize).Distinct().Find(objs).Error
}

// GetOneBy loads the first record matching values into obj. It returns
// GORM's error, including gorm.ErrRecordNotFound when nothing matches.
func GetOneBy(obj interfaces.BaseInterface, values map[string]any) error {
	if obj.GetID() == 0 {
		resetId(obj)
	}
	return JoinTables(values, &obj).First(obj).Error
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

func (dw DataWriter) Upsert(obj interfaces.BaseInterface, values map[string]any) error {
	if err := GetOneBy(obj, values); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if obj.GetID() == 0 {
		return dw.Create(obj)
	}
	return dw.Update(obj)
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
