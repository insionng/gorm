package gorm

import (
	"fmt"
	"reflect"
	"strings"
)

func BeginTransaction(scope *Scope) {
	scope.Begin()
}

func CommitOrRollbackTransaction(scope *Scope) {
	scope.CommitOrRollback()
}

func SaveBeforeAssociations(scope *Scope) {
	for _, field := range scope.Fields() {
		if field.BeforeAssociation && !field.IsBlank && !field.IsIgnored {
			value := reflect.ValueOf(field.Value)
			newDB := scope.NewDB()

			if value.CanAddr() {
				scope.Err(newDB.Save(value.Addr().Interface()).Error)
			} else {
				// If can't take address, then clone the value and set it back
				value = reflect.New(reflect.ValueOf(field.Value).Type()).Elem()
				for _, f := range newDB.NewScope(field.Value).Fields() {
					value.FieldByName(f.Name).Set(reflect.ValueOf(f.Value))
				}
				scope.Err(newDB.Save(value.Addr().Interface()).Error)
				scope.SetColumn(field.Name, value.Interface())
			}

			if field.Relationship != nil && field.Relationship.foreignKey != "" {
				scope.SetColumn(field.Relationship.foreignKey, newDB.NewScope(value.Interface()).PrimaryKeyValue())
			}
		}
	}
}

func SaveAfterAssociations(scope *Scope) {
	for _, field := range scope.Fields() {
		if field.AfterAssociation && !field.IsBlank && !field.IsIgnored {
			value := reflect.ValueOf(field.Value)

			switch value.Kind() {
			case reflect.Slice:
				for i := 0; i < value.Len(); i++ {
					newDB := scope.NewDB()
					elem := value.Index(i).Addr().Interface()

					if field.Relationship != nil && field.Relationship.joinTable == "" && field.Relationship.foreignKey != "" {
						newDB.NewScope(elem).SetColumn(field.Relationship.foreignKey, scope.PrimaryKeyValue())
					}

					scope.Err(newDB.Save(elem).Error)

					if field.Relationship != nil && field.Relationship.joinTable != "" {
						newScope := scope.New(elem)
						joinTable := field.Relationship.joinTable
						foreignKey := ToSnake(field.Relationship.foreignKey)
						foreignValue := fmt.Sprintf("%v", scope.PrimaryKeyValue())
						associationForeignKey := ToSnake(field.Relationship.associationForeignKey)
						associationForeignValue := fmt.Sprintf("%v", newScope.PrimaryKeyValue())

						newScope.Raw(fmt.Sprintf(
							"INSERT INTO %v (%v) SELECT %v %v WHERE NOT EXISTS (SELECT * FROM %v WHERE %v = %v AND %v = %v);",
							joinTable,
							strings.Join([]string{scope.Quote(foreignKey), scope.Quote(associationForeignKey)}, ","),
							strings.Join([]string{newScope.AddToVars(foreignValue), newScope.AddToVars(associationForeignValue)}, ","),
							scope.Dialect().SelectFromDummyTable(),
							joinTable,
							scope.Quote(foreignKey),
							newScope.AddToVars(foreignValue),
							scope.Quote(associationForeignKey),
							newScope.AddToVars(associationForeignValue),
						))
						if _, err := scope.DB().Exec(newScope.Sql, newScope.SqlVars...); err != nil {
							scope.Err(err)
						}
					}
				}
			default:
				newDB := scope.NewDB()
				if value.CanAddr() {
					if field.Relationship != nil {
						newDB.NewScope(field.Value).SetColumn(field.Relationship.foreignKey, scope.PrimaryKeyValue())
					}
					scope.Err(newDB.Save(field.Value).Error)
				} else {
					destValue := reflect.New(reflect.TypeOf(field.Value)).Elem()

					for _, f := range newDB.NewScope(field.Value).Fields() {
						destValue.FieldByName(f.Name).Set(reflect.ValueOf(f.Value))
					}

					elem := destValue.Addr().Interface()
					if field.Relationship != nil {
						newDB.NewScope(elem).SetColumn(field.Relationship.foreignKey, scope.PrimaryKeyValue())
					}
					scope.Err(newDB.Save(elem).Error)
					scope.SetColumn(field.Name, destValue.Interface())
				}
			}
		}
	}
}
