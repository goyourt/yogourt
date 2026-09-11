package database

import (
	"fmt"
	"reflect"

	"github.com/goyourt/yogourt/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func structPointer(obj any) (reflect.Value, error) {
	rv := reflect.ValueOf(obj)
	if rv.Kind() != reflect.Ptr || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("expected a non-nil pointer to a struct, got %T", obj)
	}
	return rv, nil
}

// capability asserts obj to T, refusing typed-nil pointers hidden in an
// interface value: calling a method on them would panic.
func capability[T any](obj any) (T, bool) {
	var zero T
	if obj == nil {
		return zero, false
	}
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Ptr && rv.IsNil() {
		return zero, false
	}
	t, ok := obj.(T)
	return t, ok
}

func actorId(currentUser any) (int, bool) {
	provider, ok := capability[interfaces.AuditActorProvider](currentUser)
	if !ok {
		return 0, false
	}
	id, ok := provider.AuditActorID()
	if !ok || id == 0 {
		return 0, false
	}
	return id, true
}

func parseSchema(db *gorm.DB, obj any) (*schema.Schema, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(obj); err != nil {
		return nil, err
	}
	return stmt.Schema, nil
}

func requireCompletePrimaryKey(db *gorm.DB, obj any) error {
	rv, err := structPointer(obj)
	if err != nil {
		return err
	}
	sch, err := parseSchema(db, obj)
	if err != nil {
		return err
	}
	if len(sch.PrimaryFields) == 0 {
		return fmt.Errorf("%T has no primary key", obj)
	}
	for _, field := range sch.PrimaryFields {
		if _, isZero := field.ValueOf(db.Statement.Context, rv); isZero {
			return fmt.Errorf("%T needs its full primary key: %s is empty", obj, field.Name)
		}
	}
	return nil
}

func copyPrimaryKeyFields(db *gorm.DB, from any, to any) error {
	fromRv, err := structPointer(from)
	if err != nil {
		return err
	}
	toRv, err := structPointer(to)
	if err != nil {
		return err
	}
	sch, err := parseSchema(db, to)
	if err != nil {
		return err
	}
	for _, field := range sch.PrimaryFields {
		value, isZero := field.ValueOf(db.Statement.Context, fromRv)
		if isZero {
			continue
		}
		if err := field.Set(db.Statement.Context, toRv, value); err != nil {
			return err
		}
	}
	return nil
}

func requireSchemaColumn(db *gorm.DB, obj any, column string) error {
	sch, err := parseSchema(db, obj)
	if err != nil {
		return err
	}
	if sch.LookUpField(column) == nil {
		return fmt.Errorf("%T has no column %q", obj, column)
	}
	return nil
}
