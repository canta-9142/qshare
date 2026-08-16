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

	"golang.org/x/sys/unix"
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
	identity fileIdentity
	children []*Node
	parent   *Node
}

func (n *Node) ID() ResourceID     { return n.id }
func (n *Node) Name() string       { return n.name }
func (n *Node) Kind() NodeKind     { return n.kind }
func (n *Node) Size() int64        { return n.size }
func (n *Node) ModTime() time.Time { return n.modTime }
func (n *Node) Children() []*Node  { return append([]*Node(nil), n.children...) }
func (n *Node) Parent() *Node      { return n.parent }

type fileIdentity struct{ dev, ino uint64 }

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
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open shared directory: %w", err)
	}
	root := os.NewFile(uintptr(fd), path)
	if root == nil {
		unix.Close(fd)
		return nil, errors.New("create shared directory handle")
	}
	d := &Directory{root: root, rootPath: path, byID: make(map[ResourceID]*Node)}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, d.Close())
		}
	}()

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
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
	d.node = &Node{id: rootID, name: filepath.Base(absPath), kind: NodeDirectory, modTime: info.ModTime(), identity: identityOf(&stat)}
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
		var observed unix.Stat_t
		if err := unix.Fstatat(int(dir.Fd()), name, &observed, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return fmt.Errorf("stat directory entry %q: %w", name, err)
		}
		observedMode := uint32(observed.Mode) & unix.S_IFMT
		if observedMode != unix.S_IFREG && observedMode != unix.S_IFDIR {
			continue
		}
		fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return fmt.Errorf("open directory entry %q: %w", name, err)
		}
		var stat unix.Stat_t
		err = unix.Fstat(fd, &stat)
		if err != nil {
			unix.Close(fd)
			return fmt.Errorf("stat directory entry %q: %w", name, err)
		}
		mode := uint32(stat.Mode) & unix.S_IFMT
		if mode != unix.S_IFREG && mode != unix.S_IFDIR {
			unix.Close(fd)
			continue
		}
		childDepth := depth + 1
		if childDepth > MaxDirectoryDepth {
			unix.Close(fd)
			return fmt.Errorf("directory depth exceeds maximum of %d", MaxDirectoryDepth)
		}
		id, err := d.uniqueID(makeID)
		if err != nil {
			unix.Close(fd)
			return err
		}
		node := &Node{id: id, name: name, rel: append(append([]string(nil), parent.rel...), name), identity: identityOf(&stat), size: stat.Size, modTime: time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec), parent: parent}
		if mode == unix.S_IFREG {
			node.kind = NodeFile
			*files++
			if *files > MaxDirectoryFiles {
				unix.Close(fd)
				return fmt.Errorf("directory contains too many regular files: maximum is %d", MaxDirectoryFiles)
			}
			unix.Close(fd)
		} else {
			node.kind = NodeDirectory
			child := os.NewFile(uintptr(fd), name)
			if child == nil {
				unix.Close(fd)
				return errors.New("create directory entry handle")
			}
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

func identityOf(stat *unix.Stat_t) fileIdentity {
	return fileIdentity{dev: uint64(stat.Dev), ino: stat.Ino}
}
func (d *Directory) Root() *Node                        { return d.node }
func (d *Directory) Lookup(id ResourceID) (*Node, bool) { n, ok := d.byID[id]; return n, ok }

func (d *Directory) OpenFile(node *Node) (*DirectoryFile, error) {
	if node == nil || node.kind != NodeFile || d.byID[node.id] != node {
		return nil, errors.New("node is not an authorized file")
	}
	fd, err := d.openAuthorizedRoot()
	if err != nil {
		return nil, err
	}
	for i, part := range node.rel {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if i < len(node.rel)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, openErr := unix.Openat(fd, part, flags, 0)
		unix.Close(fd)
		if openErr != nil {
			return nil, fmt.Errorf("reopen authorized file: %w", openErr)
		}
		fd = next
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("verify authorized file: %w", err)
	}
	if uint32(stat.Mode)&unix.S_IFMT != unix.S_IFREG || identityOf(&stat) != node.identity {
		unix.Close(fd)
		return nil, errors.New("authorized file was replaced")
	}
	f := os.NewFile(uintptr(fd), node.name)
	if f == nil {
		unix.Close(fd)
		return nil, errors.New("create authorized file handle")
	}
	return &DirectoryFile{file: f, name: node.name, size: stat.Size, modTime: time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec)}, nil
}

func (d *Directory) VerifyDirectory(node *Node) error {
	if node == nil || node.kind != NodeDirectory || d.byID[node.id] != node {
		return errors.New("node is not an authorized directory")
	}
	fd, err := d.openAuthorizedRoot()
	if err != nil {
		return err
	}
	for _, part := range node.rel {
		next, openErr := unix.Openat(fd, part, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
		unix.Close(fd)
		if openErr != nil {
			return fmt.Errorf("reopen authorized directory: %w", openErr)
		}
		fd = next
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("verify authorized directory: %w", err)
	}
	if uint32(stat.Mode)&unix.S_IFMT != unix.S_IFDIR || identityOf(&stat) != node.identity {
		return errors.New("authorized directory was replaced")
	}
	return nil
}

func (d *Directory) openAuthorizedRoot() (int, error) {
	fd, err := unix.Open(d.rootPath, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, fmt.Errorf("reopen shared root: %w", err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		unix.Close(fd)
		return -1, fmt.Errorf("verify shared root: %w", err)
	}
	if identityOf(&stat) != d.node.identity {
		unix.Close(fd)
		return -1, errors.New("shared root was replaced")
	}
	return fd, nil
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
	return d.root.Close()
}
