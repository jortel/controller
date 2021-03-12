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
func New() *Queue {
	uid, _ := uuid.NewUUID()
	name := uid.String() + ".fbq"
	path := pathlib.Join(WorkingDir, name)
	return NewAt(path)
}

//
// New file-based queue at the specified path.
func NewAt(path string) *Queue {
	return &Queue{
		path: path,
		writer: Writer{
			path: path,
		},
		reader: Reader{
			path: path,
		},
	}
}

//
// File-based queue.
type Queue struct {
	// File path.
	path string
	// Queue writer.
	writer Writer
	// Queue Reader.
	reader Reader
}

//
// Enqueue object.
func (q *Queue) Put(object interface{}) (err error) {
	err = q.writer.Put(object)
	return
}

//
// Dequeue object.
func (q *Queue) Get() (object interface{}, end bool, err error) {
	q.reader.catalog = q.writer.catalog
	object, end, err = q.reader.Get()
	return
}

//
// Get a new reader.
func (q *Queue) NewReader() (r *Reader, err error) {
	uid, _ := uuid.NewUUID()
	name := uid.String() + ".fbq"
	path := pathlib.Join(WorkingDir, name)
	err = os.Link(q.path, path)
	if err == nil {
		r = &Reader{
			catalog: q.writer.catalog,
			path:    path,
		}
	} else {
		err = liberr.Wrap(err)
	}

	return
}

//
// Close the queue.
func (q *Queue) Close(delete bool) {
	q.writer.Close(delete)
	q.reader.Close()
}

//
// Writer.
type Writer struct {
	// File path.
	path string
	// Catalog of object types.
	catalog []interface{}
	// File.
	file *os.File
}

//
// Enqueue object.
func (w *Writer) Put(object interface{}) (err error) {
	// Lazy open.
	if w.file == nil {
		err = w.open()
		if err != nil {
			return
		}
	}
	file := w.file
	// Encode object and add to catalog.
	var bfr bytes.Buffer
	encoder := gob.NewEncoder(&bfr)
	err = encoder.Encode(object)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}
	kind := w.add(object)
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

	_ = file.Sync()

	return
}

//
// Close the writer.
func (w *Writer) Close(delete bool) {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
		if delete {
			_ = os.Remove(w.path)
		}
	}
}

//
// Open the writer.
func (w *Writer) open() (err error) {
	if w.file != nil {
		return
	}
	w.file, err = os.Create(w.path)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}

	return
}

//
// Add object (proto) to the catalog.
func (w *Writer) add(object interface{}) (kind uint16) {
	t := reflect.TypeOf(object)
	for i, f := range w.catalog {
		if t == reflect.TypeOf(f) {
			kind = uint16(i)
			return
		}
	}

	kind = uint16(len(w.catalog))
	w.catalog = append(w.catalog, object)
	return
}

//
// Reader.
type Reader struct {
	// File path.
	path string
	// Catalog of object types.
	catalog []interface{}
	// File.
	file *os.File
}

//
// Dequeue object.
func (r *Reader) Get() (object interface{}, end bool, err error) {
	// Lazy open.
	if r.file == nil {
		err = r.open()
		if err != nil {
			return
		}
	}
	file := r.file
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
	object, found := r.find(kind)
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
// Close the reader.
func (r *Reader) Close() {
	if r.file != nil {
		_ = r.file.Close()
		_ = os.Remove(r.path)
		r.file = nil
	}
}

//
// Open the reader.
func (r *Reader) open() (err error) {
	if r.file != nil {
		return
	}
	r.file, err = os.Open(r.path)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}

	return
}

//
// Find object (kind) in the catalog.
func (r *Reader) find(kind uint16) (object interface{}, found bool) {
	i := int(kind)
	if i < len(r.catalog) {
		object = r.catalog[i]
		object = reflect.New(reflect.TypeOf(object)).Interface()
		found = true
	}

	return
}
