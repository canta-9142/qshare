package share

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	MaxDirectoryFiles   = 1000
	MaxDirectoryEntries = 2000
	MaxDirectoryDepth   = 20
)

type NodeKind uint8

const (
	NodeDirectory NodeKind = iota
	NodeFile
)

type Node struct {
	id       ResourceID
	name     string
	kind     NodeKind
	size     int64
	modTime  time.Time
	identity os.FileInfo
	children []*Node
	parent   *Node
	pinned   *os.File
}

func (n *Node) ID() ResourceID     { return n.id }
func (n *Node) Name() string       { return n.name }
func (n *Node) Kind() NodeKind     { return n.kind }
func (n *Node) Size() int64        { return n.size }
func (n *Node) ModTime() time.Time { return n.modTime }
func (n *Node) Children() []*Node  { return append([]*Node(nil), n.children...) }
func (n *Node) Parent() *Node      { return n.parent }

type Directory struct {
	root     *os.File
	rootPath string
	node     *Node
	byID     map[ResourceID]*Node
}

func OpenDirectory(path string) (*Directory, error) {
	return openDirectory(path, newResourceID)
}

func openDirectory(path string, makeID func() (ResourceID, error)) (_ *Directory, resultErr error) {
	// Preserve intermediate components while removing trailing separators.
	if trimmed := strings.TrimRight(path, string(filepath.Separator)); trimmed != "" {
		path = trimmed
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("lstat shared directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("shared directory is not a directory: %s", path)
	}
	root, err := openDirectoryNoFollow(path)
	if err != nil {
		return nil, fmt.Errorf("open shared directory: %w", err)
	}
	d := &Directory{root: root, rootPath: path, byID: make(map[ResourceID]*Node)}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, d.Close())
		}
	}()

	rootInfo, err := root.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat shared directory: %w", err)
	}
	rootID, err := d.uniqueID(makeID)
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve shared directory name: %w", err)
	}
	d.rootPath = absPath
	d.node = &Node{id: rootID, name: filepath.Base(absPath), kind: NodeDirectory, modTime: info.ModTime(), identity: rootInfo}
	d.byID[rootID] = d.node
	files, entries := 0, 0
	if err := d.walk(root, d.node, 0, &files, &entries, makeID); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Directory) walk(dir *os.File, parent *Node, depth int, files, entries *int, makeID func() (ResourceID, error)) error {
	remaining := MaxDirectoryEntries - *entries
	items, err := dir.ReadDir(remaining + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read directory %q: %w", dir.Name(), err)
	}
	if len(items) > remaining {
		return fmt.Errorf("directory contains too many entries: maximum is %d", MaxDirectoryEntries)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name() < items[j].Name() })
	for _, item := range items {
		*entries++
		if *entries > MaxDirectoryEntries {
			return fmt.Errorf("directory contains too many entries: maximum is %d", MaxDirectoryEntries)
		}
		name := item.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		child, info, included, err := openDirectoryEntryNoFollow(dir, name)
		if err != nil {
			return err
		}
		if !included {
			continue
		}
		childDepth := depth + 1
		if childDepth > MaxDirectoryDepth {
			child.Close()
			return fmt.Errorf("directory depth exceeds maximum of %d", MaxDirectoryDepth)
		}
		id, err := d.uniqueID(makeID)
		if err != nil {
			child.Close()
			return err
		}
		node := &Node{id: id, name: name, identity: info, size: info.Size(), modTime: info.ModTime(), parent: parent, pinned: child}
		if info.Mode().IsRegular() {
			node.kind = NodeFile
			*files++
			if *files > MaxDirectoryFiles {
				child.Close()
				return fmt.Errorf("directory contains too many regular files: maximum is %d", MaxDirectoryFiles)
			}
		} else {
			node.kind = NodeDirectory
		}
		parent.children = append(parent.children, node)
		// Register before descending so startup failures close this handle too.
		d.byID[id] = node
		if node.kind == NodeDirectory {
			if err := d.walk(child, node, childDepth, files, entries, makeID); err != nil {
				return err
			}
		}
	}
	sort.SliceStable(parent.children, func(i, j int) bool {
		if parent.children[i].kind != parent.children[j].kind {
			return parent.children[i].kind == NodeDirectory
		}
		return parent.children[i].name < parent.children[j].name
	})
	return nil
}

func (d *Directory) uniqueID(makeID func() (ResourceID, error)) (ResourceID, error) {
	id, err := makeID()
	if err != nil {
		return "", fmt.Errorf("generate resource ID: %w", err)
	}
	if _, exists := d.byID[id]; exists {
		return "", errors.New("generate resource ID: duplicate ID")
	}
	return id, nil
}

func (d *Directory) Root() *Node                        { return d.node }
func (d *Directory) Lookup(id ResourceID) (*Node, bool) { n, ok := d.byID[id]; return n, ok }

func (d *Directory) OpenFile(node *Node) (*File, error) {
	if node == nil || node.kind != NodeFile || d.byID[node.id] != node {
		return nil, errors.New("node is not an authorized file")
	}
	file, info, err := d.openAuthorizedNode(node)
	if err != nil {
		return nil, err
	}
	return &File{file: file, name: node.name, size: info.Size(), modTime: info.ModTime()}, nil
}

func (d *Directory) VerifyDirectory(node *Node) error {
	if node == nil || node.kind != NodeDirectory || d.byID[node.id] != node {
		return errors.New("node is not an authorized directory")
	}
	file, _, err := d.openAuthorizedNode(node)
	if err != nil {
		return err
	}
	return file.Close()
}

// Reopen descendants only through handles to their verified authorized parents.
func (d *Directory) openAuthorizedNode(node *Node) (*os.File, os.FileInfo, error) {
	var file *os.File
	var err error
	if node == d.node {
		file, err = openDirectoryNoFollow(d.rootPath)
	} else {
		parent, _, openErr := d.openAuthorizedNode(node.parent)
		if openErr != nil {
			return nil, nil, openErr
		}
		file, err = reopenDirectoryEntryNoFollow(parent, node.name, node.kind == NodeDirectory)
		parent.Close()
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reopen authorized node: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, fmt.Errorf("verify authorized node: %w", err)
	}
	if info.Mode().Type() != node.identity.Mode().Type() || !os.SameFile(info, node.identity) {
		file.Close()
		return nil, nil, errors.New("authorized node was replaced")
	}
	return file, info, nil
}

func (d *Directory) Close() error {
	if d == nil || d.root == nil {
		return nil
	}
	var err error
	for _, node := range d.byID {
		if node.pinned != nil {
			err = errors.Join(err, node.pinned.Close())
			node.pinned = nil
		}
	}
	err = errors.Join(err, d.root.Close())
	d.root = nil
	return err
}
