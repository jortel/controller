package model

import (
	"database/sql"
	"errors"
	liberr "github.com/konveyor/controller/pkg/error"
	"os"
	"reflect"
	"sync"
)

const (
	Pragma = "PRAGMA foreign_keys = ON"
)

//
// Tx.Commit()
// Tx.End()
// Called and the transaction is not in progress by
// the associated Client.
var TxInvalidError = errors.New("transaction not valid")

//
// Database client.
type DB interface {
	// Open and build the schema.
	Open(bool) error
	// Close.
	Close(bool) error
	// Get the specified model.
	Get(Model) error
	// List models based on the type of slice.
	List(interface{}, ListOptions) error
	// Count based on the specified model.
	Count(Model, Predicate) (int64, error)
	// Begin a transaction.
	Begin() (*Tx, error)
	// Insert a model.
	Insert(Model) error
	// Update a model.
	Update(Model) error
	// Delete a model.
	Delete(Model) error
	// Watch a model collection.
	Watch(Model, EventHandler) (*Watch, error)
	// The journal
	Journal() *Journal
}

//
// Database client.
type Client struct {
	// Protect internal state.
	sync.RWMutex
	// The sqlite3 database will not support
	// concurrent write operations.
	dbMutex sync.Mutex
	// file path.
	path string
	// Model
	models []interface{}
	// Database connection.
	db *sql.DB
	// Current database transaction.
	tx *Tx
	// Journal
	journal Journal
}

//
// Create the database.
// Build the schema to support the specified models.
// Optionally `purge` (delete) the DB first.
func (r *Client) Open(purge bool) error {
	if purge {
		os.Remove(r.path)
	}
	db, err := sql.Open("sqlite3", r.path)
	if err != nil {
		panic(err)
	}
	statements := []string{Pragma}
	r.models = append(r.models, &Label{})
	for _, m := range r.models {
		ddl, err := Table{}.DDL(m)
		if err != nil {
			panic(err)
		}
		statements = append(statements, ddl...)
	}
	for _, ddl := range statements {
		_, err = db.Exec(ddl)
		if err != nil {
			db.Close()
			return liberr.Wrap(err)
		}
	}

	r.db = db

	return nil
}

//
// Close the database.
// Optionally purge (delete) the DB.
func (r *Client) Close(purge bool) error {
	if r.db == nil {
		return nil
	}
	err := r.db.Close()
	if err != nil {
		return liberr.Wrap(err)
	}
	r.db = nil
	if purge {
		os.Remove(r.path)
	}

	return nil
}

//
// Get the model.
func (r *Client) Get(model Model) error {
	return Table{r.db}.Get(model)
}

//
// List models.
// The `list` must be: *[]Model.
func (r *Client) List(list interface{}, options ListOptions) error {
	return Table{r.db}.List(list, options)
}

//
// Count models.
func (r *Client) Count(model Model, predicate Predicate) (int64, error) {
	return Table{r.db}.Count(model, predicate)
}

//
// Begin a transaction.
// Example:
//   tx, _ := client.Begin()
//   defer tx.End()
//   client.Insert(model)
//   client.Insert(model)
//   tx.Commit()
func (r *Client) Begin() (*Tx, error) {
	r.Lock()
	defer r.Unlock()
	r.dbMutex.Lock()
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	r.tx = &Tx{client: r, ref: tx}
	return r.tx, nil
}

//
// Insert the model.
func (r *Client) Insert(model Model) error {
	r.Lock()
	defer r.Unlock()
	table := Table{}
	if r.tx == nil {
		r.dbMutex.Lock()
		defer r.dbMutex.Unlock()
		table.DB = r.db
	} else {
		table.DB = r.tx.ref
	}
	err := table.Insert(model)
	if err != nil {
		return liberr.Wrap(err)
	}
	err = r.insertLabels(table, model)
	if err != nil {
		return liberr.Wrap(err)
	}
	r.journal.Created(r.origin(), model)
	if r.tx == nil {
		r.journal.Commit()
	}

	return nil
}

//
// Update the model.
func (r *Client) Update(model Model) error {
	r.Lock()
	defer r.Unlock()
	table := Table{}
	if r.tx == nil {
		r.dbMutex.Lock()
		defer r.dbMutex.Unlock()
		table.DB = r.db
	} else {
		table.DB = r.tx.ref
	}
	current := r.journal.copy(model)
	err := table.Get(current)
	if err != nil {
		return liberr.Wrap(err)
	}
	err = table.Update(model)
	if err != nil {
		return liberr.Wrap(err)
	}
	err = r.replaceLabels(table, model)
	if err != nil {
		return liberr.Wrap(err)
	}
	r.journal.Updated(r.tx, current, model)
	if r.tx == nil {
		r.journal.Commit()
	}

	return nil
}

//
// Delete the model.
func (r *Client) Delete(model Model) error {
	r.Lock()
	defer r.Unlock()
	table := Table{}
	if r.tx == nil {
		r.dbMutex.Lock()
		defer r.dbMutex.Unlock()
		table.DB = r.db
	} else {
		table.DB = r.tx.ref
	}
	err := table.Delete(model)
	if err != nil {
		return liberr.Wrap(err)
	}
	err = r.deleteLabels(table, model)
	if err != nil {
		return liberr.Wrap(err)
	}
	r.journal.Deleted(r.tx, model)
	if r.tx == nil {
		r.journal.Commit()
	}

	return nil
}

//
// Watch model events.
func (r *Client) Watch(model Model, handler EventHandler) (*Watch, error) {
	r.Lock()
	defer r.Unlock()
	mt := reflect.TypeOf(model)
	switch mt.Kind() {
	case reflect.Ptr:
		mt = mt.Elem()
	}
	watch, err := r.journal.Watch(model, handler)
	if err != nil {
		return nil, liberr.Wrap(err)
	}
	listPtr := reflect.New(reflect.SliceOf(mt))
	err = Table{r.db}.List(listPtr.Interface(), ListOptions{})
	if err != nil {
		return nil, liberr.Wrap(err)
	}
	list := listPtr.Elem()
	for i := 0; i < list.Len(); i++ {
		m := list.Index(i).Addr().Interface()
		watch.notify(
			&Event{
				Model:  m.(Model),
				Action: Created,
			})
	}

	watch.Start()

	return watch, nil
}

//
// The associated journal.
func (r *Client) Journal() *Journal {
	return &r.journal
}

//
// Insert labels for the model into the DB.
func (r *Client) insertLabels(table Table, model Model) error {
	if labeled, cast := model.(Labeled); cast {
		for l, v := range labeled.Labels() {
			label := &Label{
				Parent: model.Pk(),
				Kind:   table.Name(model),
				Name:   l,
				Value:  v,
			}
			err := table.Insert(label)
			if err != nil {
				return liberr.Wrap(err)
			}
		}
	}

	return nil
}

//
// Delete labels for a model in the DB.
func (r *Client) deleteLabels(table Table, model Model) error {
	list := []Label{}
	err := table.List(
		&list,
		ListOptions{
			Predicate: And(
				Eq("Kind", table.Name(model)),
				Eq("Parent", model.Pk())),
		})
	if err != nil {
		return liberr.Wrap(err)
	}
	for _, label := range list {
		err := table.Delete(&label)
		if err != nil {
			return liberr.Wrap(err)
		}
	}

	return nil
}

//
// Replace labels.
func (r *Client) replaceLabels(table Table, model Model) error {
	err := r.deleteLabels(table, model)
	if err != nil {
		return liberr.Wrap(err)
	}
	err = r.insertLabels(table, model)
	if err != nil {
		return liberr.Wrap(err)
	}

	return nil
}

//
// Commit a transaction.
// This MUST be preceeded by Begin() which returns
// the `tx` transaction.  This will end the transaction.
func (r *Client) commit(tx *Tx) error {
	r.Lock()
	defer r.Unlock()
	if r.tx == nil || r.tx.ref != tx.ref {
		return liberr.Wrap(TxInvalidError)
	}
	defer func() {
		r.dbMutex.Unlock()
		r.tx = nil
	}()
	err := r.tx.ref.Commit()
	if err != nil {
		return liberr.Wrap(err)
	}

	r.journal.Commit()

	return nil
}

//
// End a transaction.
// This MUST be preceeded by Begin() which returns
// the `tx` transaction.
func (r *Client) end(tx *Tx) error {
	r.Lock()
	defer r.Unlock()
	if r.tx == nil || r.tx.ref != tx.ref {
		return liberr.Wrap(TxInvalidError)
	}
	defer func() {
		r.dbMutex.Unlock()
		r.tx = nil
	}()
	err := r.tx.ref.Rollback()
	if err != nil {
		return liberr.Wrap(err)
	}

	r.journal.Unstage()

	return nil
}

//
// Transaction origin.
func (r *Client) origin() (origin interface{}) {
	if r.tx != nil {
		origin = r.tx.Origin
	}

	return
}

//
// Database transaction.
type Tx struct {
	// Transaction origin.
	Origin interface{}
	// Associated client.
	client *Client
	// Reference to sql.Tx.
	ref *sql.Tx
}

//
// Commit a transaction.
// Staged changes are committed in the DB.
// This will end the transaction.
func (r *Tx) Commit() error {
	return r.client.commit(r)
}

//
// End a transaction.
// Staged changes are discarded.
// See: Commit().
func (r *Tx) End() error {
	return r.client.end(r)
}
