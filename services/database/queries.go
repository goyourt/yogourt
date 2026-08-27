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
// error, or the connection error when the database is unreachable.
func GetAllPaginated[T interfaces.BaseInterface](objs *[]T, values map[string]any, page int, pageSize int) error {
	query, err := SearchQuery(values, objs, page, pageSize)
	if err != nil {
		return err
	}

	return query.Distinct().Find(objs).Error
}

// GetOneBy loads the first record matching values into obj. It returns
// GORM's error, including gorm.ErrRecordNotFound when nothing matches.
func GetOneBy(obj interfaces.BaseInterface, values map[string]any) error {
	if obj.GetID() == 0 {
		resetId(obj)
	}

	query, err := JoinTables(values, &obj)
	if err != nil {
		return err
	}

	return query.First(obj).Error
}

func (dw DataWriter) Create(obj interfaces.BaseInterface) error {
	resetId(obj)
	obj.SetCreatedById(dw.CurrentUser)
	obj.SetUpdatedById(dw.CurrentUser)

	db, err := providers.GetDB()
	if err != nil {
		return err
	}

	return db.Create(obj).Error
}

func (dw DataWriter) Update(obj interfaces.BaseInterface) error {
	obj.SetUpdatedById(dw.CurrentUser)
	obj.SetUpdatedAt(time.Now())

	db, err := providers.GetDB()
	if err != nil {
		return err
	}

	if err := db.Model(obj).Where("uuid = ?", obj.GetUuid()).UpdateColumns(obj).Error; err != nil {
		return err
	}

	return db.First(obj, "uuid = ?", obj.GetUuid()).Error
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
	db, err := providers.GetDB()
	if err != nil {
		return err
	}

	return dw.softDelete(db, obj)
}

// softDelete soft deletes obj and really persists its deleted_by_id audit
// column.
//
// GORM's soft-delete callback only writes the deleted_at column
// (UPDATE ... SET deleted_at = ?): it never carries the other columns of the
// model, so the value set by SetDeletedById used to be silently dropped and
// deleted_by_id stayed NULL in database — unlike created_by_id and
// updated_by_id, which Create and Update write along with every other column.
// The audit column therefore needs its own explicit statement, targeting the
// row by uuid exactly like Update does.
//
// Both statements share one explicit transaction: a row can never end up
// soft deleted without its author, nor attributed to an author without being
// deleted.
func (dw DataWriter) softDelete(db *gorm.DB, obj interfaces.BaseInterface) error {
	obj.SetDeletedById(dw.CurrentUser)

	// No authenticated user: there is no audit column to write, the plain
	// soft delete is enough and needs no transaction.
	if dw.CurrentUser == nil {
		return db.Delete(obj).Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Model(obj).
			Where("uuid = ?", obj.GetUuid()).
			UpdateColumn("deleted_by_id", obj.GetDeletedById()).Error
		if err != nil {
			return err
		}

		return tx.Delete(obj).Error
	})
}

func HardDelete(obj interfaces.BaseInterface) error {
	db, err := providers.GetDB()
	if err != nil {
		return err
	}

	return hardDelete(db, obj)
}

func hardDelete(db *gorm.DB, obj interfaces.BaseInterface) error {
	return db.Unscoped().Delete(obj).Error
}

// SearchQuery builds the paginated query GetAllPaginated runs, and is the
// entry point for the applications refining it with the GORM API. It returns
// the connection error instead of a query when the database is unreachable:
// there is no *gorm.DB to hang a failure on before a connection exists.
func SearchQuery[T interfaces.BaseInterface](values map[string]any, objs *[]T, page int, pageSize int) (*gorm.DB, error) {
	query, err := JoinTables(values, new(T))
	if err != nil {
		return nil, err
	}

	return Paginate(query, page, pageSize), nil
}

func Paginate(query *gorm.DB, page int, pageSize int) *gorm.DB {
	if page < 1 || pageSize < 1 {
		return query
	}
	offset := (page - 1) * pageSize
	return query.Limit(pageSize).Offset(offset)
}
