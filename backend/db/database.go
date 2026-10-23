package db

import (
	"context"
	"reflect"

	"godeb/backend/security"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// registerVaultCallbacks seals struct fields tagged vault:"secret" before
// GORM writes them and opens them again after a read or a successful write.
func registerVaultCallbacks(gdb *gorm.DB) {
	gdb.Callback().Create().Before("gorm:create").Register("godeb:vault_seal_create", func(tx *gorm.DB) {
		applyVault(tx, true)
	})
	gdb.Callback().Update().Before("gorm:update").Register("godeb:vault_seal_update", func(tx *gorm.DB) {
		applyVault(tx, true)
	})
	gdb.Callback().Create().After("gorm:create").Register("godeb:vault_open_create", func(tx *gorm.DB) {
		applyVault(tx, false)
	})
	gdb.Callback().Update().After("gorm:update").Register("godeb:vault_open_update", func(tx *gorm.DB) {
		applyVault(tx, false)
	})
	gdb.Callback().Query().After("gorm:query").Register("godeb:vault_open_query", func(tx *gorm.DB) {
		applyVault(tx, false)
	})
}

func applyVault(tx *gorm.DB, seal bool) {
	if tx.Error != nil || tx.Statement == nil || tx.Statement.Schema == nil {
		return
	}
	ctx := tx.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	forEachRecord(tx.Statement.ReflectValue, func(record reflect.Value) {
		if tx.Error != nil {
			return
		}
		for _, field := range tx.Statement.Schema.Fields {
			if field.StructField.Tag.Get("vault") != "secret" {
				continue
			}
			transformField(tx, ctx, field, record, seal)
		}
	})
}

func transformField(tx *gorm.DB, ctx context.Context, field *schema.Field, record reflect.Value, seal bool) {
	raw, isZero := field.ValueOf(ctx, record)
	if isZero {
		return
	}
	text, ok := raw.(string)
	if !ok || text == "" {
		return
	}
	var (
		next string
		err  error
	)
	if seal {
		next, err = security.Seal(text)
	} else {
		next, err = security.Open(text)
	}
	if err != nil {
		_ = tx.AddError(err)
		return
	}
	if next == text {
		return
	}
	if err := field.Set(ctx, record, next); err != nil {
		_ = tx.AddError(err)
	}
}

func forEachRecord(value reflect.Value, fn func(reflect.Value)) {
	if !value.IsValid() {
		return
	}
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Struct:
		fn(value)
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			forEachRecord(value.Index(i), fn)
		}
	}
}
