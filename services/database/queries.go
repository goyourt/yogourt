package database

import (
	"errors"
	"reflect"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/providers"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DataWriter struct {
	db          *gorm.DB
	CurrentUser any
}

func CreateDataWriter(c *gin.Context) DataWriter {
	if c == nil {
		return DataWriter{}
	}
	return DataWriter{CurrentUser: providers.GetCurrentUser(c)}
}

func (dw DataWriter) connection() (*gorm.DB, error) {
	if dw.db != nil {
		return dw.db, nil
	}
	return providers.GetDB()
}

func (dw DataWriter) withDB(db *gorm.DB) DataWriter {
	return DataWriter{db: db, CurrentUser: dw.CurrentUser}
}

func GetAll[T any](objs *[]T, values map[string]any) error {
	return GetAllPaginated(objs, values, 0, 0)
}

func GetAllPaginated[T any](objs *[]T, values map[string]any, page int, pageSize int) error {
	query, err := SearchQuery(objs, values, page, pageSize)
	if err != nil {
		return err
	}
	return query.Distinct().Find(objs).Error
}

// SearchQuery builds the paginated query GetAllPaginated runs, and is the
// entry point for the applications refining it with the GORM API.
func SearchQuery[T any](objs *[]T, values map[string]any, page int, pageSize int) (*gorm.DB, error) {
	db, err := providers.GetDB()
	if err != nil {
		return nil, err
	}
	query, err := buildQuery(db, new(T), values)
	if err != nil {
		return nil, err
	}
	return Paginate(query, page, pageSize), nil
}

// GetOneBy loads the first record matching values into obj. The query runs
// against a fresh probe: on gorm.ErrRecordNotFound or any error, obj is left
// intact.
func GetOneBy(obj any, values map[string]any) error {
	db, err := providers.GetDB()
	if err != nil {
		return err
	}
	return getOneBy(db, obj, values)
}

func getOneBy(db *gorm.DB, obj any, values map[string]any) error {
	rv, err := structPointer(obj)
	if err != nil {
		return err
	}
	probe := reflect.New(rv.Elem().Type())
	query, err := buildQuery(db, probe.Interface(), values)
	if err != nil {
		return err
	}
	if err := query.First(probe.Interface()).Error; err != nil {
		return err
	}
	rv.Elem().Set(probe.Elem())
	return nil
}

func (dw DataWriter) Create(obj any) error {
	if _, err := structPointer(obj); err != nil {
		return err
	}
	if resetter, ok := capability[interfaces.GeneratedIdentityResetter](obj); ok {
		resetter.ResetGeneratedIdentity()
	}
	if id, ok := actorId(dw.CurrentUser); ok {
		if setter, ok := capability[interfaces.CreatedBySetter](obj); ok {
			setter.SetCreatedBy(id)
		}
		if setter, ok := capability[interfaces.UpdatedBySetter](obj); ok {
			setter.SetUpdatedBy(id)
		}
	}
	db, err := dw.connection()
	if err != nil {
		return err
	}
	// Associations are never written implicitly: GORM's save-associations
	// callbacks would insert non-nil relation fields — including rows carrying
	// a client-chosen public id. Relations go through UpsertRelations.
	return db.Omit(clause.Associations).Create(obj).Error
}

func (dw DataWriter) Update(obj any) error {
	db, err := dw.connection()
	if err != nil {
		return err
	}
	if err := requireCompletePrimaryKey(db, obj); err != nil {
		return err
	}
	if setter, ok := capability[interfaces.UpdatedAtSetter](obj); ok {
		setter.SetUpdatedAt(time.Now())
	}
	if id, ok := actorId(dw.CurrentUser); ok {
		if setter, ok := capability[interfaces.UpdatedBySetter](obj); ok {
			setter.SetUpdatedBy(id)
		}
	}
	// UpdateColumns(struct) only writes non-zero fields: Update patches, it
	// cannot clear a column. Associations are omitted, see Create.
	if err := db.Model(obj).Omit(clause.Associations).UpdateColumns(obj).Error; err != nil {
		return err
	}
	// RowsAffected == 0 does not distinguish a no-op from a missing row: the
	// reload by primary key settles it and returns the stored state.
	return db.First(obj).Error
}

func (dw DataWriter) Upsert(obj any, values map[string]any) error {
	rv, err := structPointer(obj)
	if err != nil {
		return err
	}
	db, err := dw.connection()
	if err != nil {
		return err
	}
	probe := reflect.New(rv.Elem().Type()).Interface()
	switch err := getOneBy(db, probe, values); {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return dw.Create(obj)
	case err != nil:
		return err
	}
	if err := copyPrimaryKeyFields(db, probe, obj); err != nil {
		return err
	}
	return dw.Update(obj)
}

func (dw DataWriter) Delete(obj any) error {
	db, err := dw.connection()
	if err != nil {
		return err
	}
	if err := requireCompletePrimaryKey(db, obj); err != nil {
		return err
	}

	auditor, hasAuditor := capability[interfaces.DeleteAuditor](obj)
	id, hasActor := actorId(dw.CurrentUser)
	if !hasAuditor || !hasActor {
		return deleteOne(db, obj)
	}

	column, value := auditor.DeleteAuditAssignment(id)
	if err := requireSchemaColumn(db, obj, column); err != nil {
		return err
	}
	// One transaction: a row can never end up deleted without its author, nor
	// attributed to an author without being deleted.
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(obj).Omit(clause.Associations).UpdateColumn(column, value)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return deleteOne(tx, obj)
	})
}

func HardDelete(obj any) error {
	db, err := providers.GetDB()
	if err != nil {
		return err
	}
	if err := requireCompletePrimaryKey(db, obj); err != nil {
		return err
	}
	return hardDelete(db, obj)
}

func hardDelete(db *gorm.DB, obj any) error {
	return deleteOne(db.Unscoped(), obj)
}

func deleteOne(db *gorm.DB, obj any) error {
	result := db.Delete(obj)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func Paginate(query *gorm.DB, page int, pageSize int) *gorm.DB {
	if page < 1 || pageSize < 1 {
		return query
	}
	offset := (page - 1) * pageSize
	return query.Limit(pageSize).Offset(offset)
}
