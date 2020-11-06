package model

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"reflect"
)

//
// Errors.
var NotFound = sql.ErrNoRows

//
// Database client interface.
// Support model methods taking either sql.DB or sql.Tx.
type DBTX interface {
	Exec(string, ...interface{}) (sql.Result, error)
	Query(string, ...interface{}) (*sql.Rows, error)
	QueryRow(string, ...interface{}) *sql.Row
}

//
// Database interface.
// Support model `Scan` taking either sql.Row or sql.Rows.
type Row interface {
	Scan(...interface{}) error
}

//
// Page.
// Support pagination.
type Page struct {
	// The page offset.
	Offset int
	// The number of items per/page.
	Limit int
}

//
// Slice the collection according to the page definition.
// The `collection` must be a pointer to a `Slice` which is
// modified as needed.
func (p *Page) Slice(collection interface{}) {
	v := reflect.ValueOf(collection)
	switch v.Kind() {
	case reflect.Ptr:
		v = v.Elem()
	default:
		return
	}
	switch v.Kind() {
	case reflect.Slice:
		sliced := reflect.MakeSlice(v.Type(), 0, 0)
		for i := 0; i < v.Len(); i++ {
			if i < p.Offset {
				continue
			}
			if sliced.Len() == p.Limit {
				break
			}
			sliced = reflect.Append(sliced, v.Index(i))
		}
		v.Set(sliced)
	}
}

//
// Model
// Each model represents a table in the DB.
type Model interface {
	// Get the primary key.
	Pk() string
	// Get description of the model.
	String() string
	// Equal comparison.
	Equals(other Model) bool
}

//
// Labeled
type Labeled interface {
	// Get labels.
	// Optional and may return nil.
	Labels() Labels
}
