package fbq

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"github.com/google/uuid"
	liberr "github.com/konveyor/controller/pkg/error"
	"io"
	"os"
	pathlib "path"
	"reflect"
)

//
// Default working directory.
var WorkingDir = "/tmp"

//
// New file-based queue.
func New() (q *Queue, err error) {
	uid, _ := uuid.NewUUID()
	name := uid.String() + ".fbq"
	path := pathlib.Join(WorkingDir, name)
	q, err = NewAt(path)
	return
}

//
// New file-based queue at path.
func NewAt(path string) (q *Queue, err error) {
	writer, err := os.Create(path)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}
	reader, err := os.Open(path)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}
	q = &Queue{
		path:   path,
		writer: writer,
		reader: reader,
	}

	return
}

//
// File-based queue.
type Queue struct {
	path    string
	catalog []interface{}
	writer  *os.File
	reader  *os.File
}

//
// Enqueue object.
func (q *Queue) Put(object interface{}) (err error) {
	file := q.writer
	// Encode object and add to catalog.
	var bfr bytes.Buffer
	encoder := gob.NewEncoder(&bfr)
	err = encoder.Encode(object)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}
	kind := q.add(object)
	// Write object kind.
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, kind)
	_, err = file.Write(b)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}
	// Write object encoded length.
	n := bfr.Len()
	b = make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(n))
	_, err = file.Write(b)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}
	// Write encoded object.
	nWrite, err := file.Write(bfr.Bytes())
	if err != nil {
		err = liberr.Wrap(err)
		return
	}
	if n != nWrite {
		err = liberr.New("Write failed.")
	}

	return
}

//
// Dequeue object.
func (q *Queue) Next() (object interface{}, end bool, err error) {
	file := q.reader
	// Read object kind.
	b := make([]byte, 2)
	_, err = file.Read(b)
	if err != nil {
		if err == io.EOF {
			end = true
			err = nil
		} else {
			err = liberr.Wrap(err)
		}
		return
	}
	// Read object encoded length.
	kind := binary.LittleEndian.Uint16(b)
	b = make([]byte, 8)
	_, err = file.Read(b)
	if err != nil {
		if err == io.EOF {
			end = true
			err = nil
		} else {
			err = liberr.Wrap(err)
		}
		return
	}
	// Read encoded object.
	n := int64(binary.LittleEndian.Uint64(b))
	b = make([]byte, n)
	_, err = file.Read(b)
	if err != nil {
		if err == io.EOF {
			end = true
			err = nil
		} else {
			err = liberr.Wrap(err)
		}
		return
	}
	// Decode object.
	bfr := bytes.NewBuffer(b)
	decoder := gob.NewDecoder(bfr)
	object, found := q.find(kind)
	if !found {
		err = liberr.New("unknown kind.")
		return
	}
	err = decoder.Decode(object)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}

	return
}

//
// Close the queue.
func (q *Queue) Close() {
	_ = q.writer.Close()
	_ = q.reader.Close()
	_ = os.Remove(q.path)
}

//
// Add object (proto) to the catelog.
func (q *Queue) add(object interface{}) (kind uint16) {
	t := reflect.TypeOf(object)
	for i, f := range q.catalog {
		if t == reflect.TypeOf(f) {
			kind = uint16(i)
			return
		}
	}

	kind = uint16(len(q.catalog))
	q.catalog = append(q.catalog, object)
	return
}

//
// Find object (kind) in the catalog.
func (q *Queue) find(kind uint16) (object interface{}, found bool) {
	i := int(kind)
	if i < len(q.catalog) {
		object = q.catalog[i]
		object = reflect.New(reflect.TypeOf(object)).Interface()
		found = true
	}

	return
}
