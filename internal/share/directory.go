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
	rel      []string
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
	items, err := dir.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("read directory %q: %w", strings.Join(parent.rel, string(filepath.Separator)), err)
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
		node := &Node{id: id, name: name, rel: append(append([]string(nil), parent.rel...), name), identity: info, size: info.Size(), modTime: info.ModTime(), parent: parent}
		if info.Mode().IsRegular() {
			node.kind = NodeFile
			*files++
			if *files > MaxDirectoryFiles {
				child.Close()
				return fmt.Errorf("directory contains too many regular files: maximum is %d", MaxDirectoryFiles)
			}
			node.pinned = child
		} else {
			node.kind = NodeDirectory
			err = d.walk(child, node, childDepth, files, entries, makeID)
			closeErr := child.Close()
			if err != nil || closeErr != nil {
				return errors.Join(err, closeErr)
			}
		}
		parent.children = append(parent.children, node)
		d.byID[id] = node
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

func (d *Directory) OpenFile(node *Node) (*DirectoryFile, error) {
	if node == nil || node.kind != NodeFile || d.byID[node.id] != node {
		return nil, errors.New("node is not an authorized file")
	}
	file, err := d.openAuthorizedRoot()
	if err != nil {
		return nil, err
	}
	for i, part := range node.rel {
		next, openErr := reopenDirectoryEntryNoFollow(file, part, i < len(node.rel)-1)
		file.Close()
		if openErr != nil {
			return nil, fmt.Errorf("reopen authorized file: %w", openErr)
		}
		file = next
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("verify authorized file: %w", err)
	}
	if !info.Mode().IsRegular() || !os.SameFile(info, node.identity) {
		file.Close()
		return nil, errors.New("authorized file was replaced")
	}
	return &DirectoryFile{file: file, name: node.name, size: info.Size(), modTime: info.ModTime()}, nil
}

func (d *Directory) VerifyDirectory(node *Node) error {
	if node == nil || node.kind != NodeDirectory || d.byID[node.id] != node {
		return errors.New("node is not an authorized directory")
	}
	file, err := d.openAuthorizedRoot()
	if err != nil {
		return err
	}
	for _, part := range node.rel {
		next, openErr := reopenDirectoryEntryNoFollow(file, part, true)
		file.Close()
		if openErr != nil {
			return fmt.Errorf("reopen authorized directory: %w", openErr)
		}
		file = next
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("verify authorized directory: %w", err)
	}
	if !info.IsDir() || !os.SameFile(info, node.identity) {
		return errors.New("authorized directory was replaced")
	}
	return nil
}

func (d *Directory) openAuthorizedRoot() (*os.File, error) {
	file, err := openDirectoryNoFollow(d.rootPath)
	if err != nil {
		return nil, fmt.Errorf("reopen shared root: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("verify shared root: %w", err)
	}
	if !os.SameFile(info, d.node.identity) {
		file.Close()
		return nil, errors.New("shared root was replaced")
	}
	return file, nil
}

type DirectoryFile struct {
	file    *os.File
	name    string
	size    int64
	modTime time.Time
}

func (f *DirectoryFile) Name() string          { return f.name }
func (f *DirectoryFile) Size() int64           { return f.size }
func (f *DirectoryFile) ModTime() time.Time    { return f.modTime }
func (f *DirectoryFile) Reader() io.ReadSeeker { return io.NewSectionReader(f.file, 0, f.size) }
func (f *DirectoryFile) Close() error          { return f.file.Close() }
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
