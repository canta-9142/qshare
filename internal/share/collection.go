package share

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

const MaxFiles = 100

// ResourceID is an opaque browser-visible identifier for one shared file.
type ResourceID string

// Resource associates an opaque ID with a validated, open file.
type Resource struct {
	id   ResourceID
	file *File
}

func (r Resource) ID() ResourceID { return r.id }
func (r Resource) Name() string   { return r.file.Name() }
func (r Resource) Size() int64    { return r.file.Size() }
func (r Resource) File() *File    { return r.file }

// Collection is an ordered, validated set of shared files.
type Collection struct {
	resources []Resource
	byID      map[ResourceID]*File
}

// OpenCollection validates and opens all paths. No path is opened if the
// selection is empty or exceeds MaxFiles. A partial selection is closed if
// validation fails.
func OpenCollection(paths []string) (*Collection, error) {
	return openCollection(paths, Open, newResourceID)
}

func openCollection(paths []string, open func(string) (*File, error), makeID func() (ResourceID, error)) (_ *Collection, resultErr error) {
	if len(paths) == 0 {
		return nil, errors.New("at least one shared file is required")
	}
	if len(paths) > MaxFiles {
		return nil, fmt.Errorf("too many shared files: got %d, maximum is %d", len(paths), MaxFiles)
	}

	collection := &Collection{
		resources: make([]Resource, 0, len(paths)),
		byID:      make(map[ResourceID]*File, len(paths)),
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, collection.Close())
		}
	}()

	for _, path := range paths {
		file, err := open(path)
		if err != nil {
			return nil, fmt.Errorf("open shared file %q: %w", path, err)
		}
		id, err := makeID()
		if err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("generate resource ID: %w", err)
		}
		if _, exists := collection.byID[id]; exists {
			_ = file.Close()
			return nil, errors.New("generate resource ID: duplicate ID")
		}
		collection.resources = append(collection.resources, Resource{id: id, file: file})
		collection.byID[id] = file
	}
	return collection, nil
}

func newResourceID() (ResourceID, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return ResourceID(base64.RawURLEncoding.EncodeToString(raw[:])), nil
}

// Resources returns the resources in CLI argument order.
func (c *Collection) Resources() []Resource {
	return append([]Resource(nil), c.resources...)
}

func (c *Collection) Lookup(id ResourceID) (*File, bool) {
	file, ok := c.byID[id]
	return file, ok
}

func (c *Collection) Close() error {
	var err error
	for _, resource := range c.resources {
		err = errors.Join(err, resource.file.Close())
	}
	return err
}
