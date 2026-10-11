package airlock

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ScanTag is the blob index tag Microsoft Defender for Storage writes its verdict to.
const ScanTag = "Malware Scanning scan result"

// ScanClean is Defender's verdict for a clean file.
const ScanClean = "No threats found"

// ErrNotFound means a container does not exist.
var ErrNotFound = errors.New("container not found")

// Store is the airlock's storage.
type Store interface {
	// EnsureContainer creates a container if it does not exist.
	EnsureContainer(ctx context.Context, acct Account, container string) error
	// DeleteContainer deletes a container; a missing one is not an error.
	DeleteContainer(ctx context.Context, acct Account, container string) error
	// ListFiles lists a container's blobs, or returns ErrNotFound.
	ListFiles(ctx context.Context, acct Account, container string) ([]File, error)
	// Copy copies every blob from one container to another, creating the destination.
	Copy(ctx context.Context, from Account, fromContainer string, to Account, toContainer string) error
	// ScanVerdicts returns Defender's verdict per blob; a blob not yet scanned is absent.
	ScanVerdicts(ctx context.Context, acct Account, container string) (map[string]string, error)
	// ContainerURL returns a link to a container, valid for the given time.
	ContainerURL(ctx context.Context, acct Account, container string, perm Permissions, valid time.Duration) (string, error)
}

// File is a blob in a container.
type File struct {
	Name string
	Size int64
}

// MemoryStore is an in-memory Store for tests.
type MemoryStore struct {
	mu         sync.Mutex
	containers map[string]map[string][]byte
	verdicts   map[string]string
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{containers: map[string]map[string][]byte{}, verdicts: map[string]string{}}
}

func key(acct Account, container string) string { return string(acct) + "/" + container }

// Put adds a blob, as a user uploading through a link would.
func (m *MemoryStore) Put(acct Account, container, name string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.containers[key(acct, container)]
	if !ok {
		c = map[string][]byte{}
		m.containers[key(acct, container)] = c
	}
	c[name] = data
}

// SetVerdict records a scan verdict, as Defender would.
func (m *MemoryStore) SetVerdict(acct Account, container, name, verdict string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verdicts[key(acct, container)+"/"+name] = verdict
}

// Exists reports whether a container exists.
func (m *MemoryStore) Exists(acct Account, container string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.containers[key(acct, container)]
	return ok
}

func (m *MemoryStore) EnsureContainer(_ context.Context, acct Account, container string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.containers[key(acct, container)]; !ok {
		m.containers[key(acct, container)] = map[string][]byte{}
	}
	return nil
}

func (m *MemoryStore) DeleteContainer(_ context.Context, acct Account, container string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.containers, key(acct, container))
	return nil
}

func (m *MemoryStore) ListFiles(_ context.Context, acct Account, container string) ([]File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.containers[key(acct, container)]
	if !ok {
		return nil, ErrNotFound
	}
	out := []File{}
	for name, data := range c {
		out = append(out, File{Name: name, Size: int64(len(data))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *MemoryStore) Copy(_ context.Context, from Account, fromContainer string, to Account, toContainer string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	src, ok := m.containers[key(from, fromContainer)]
	if !ok {
		return ErrNotFound
	}
	dst, ok := m.containers[key(to, toContainer)]
	if !ok {
		dst = map[string][]byte{}
		m.containers[key(to, toContainer)] = dst
	}
	for name, data := range src {
		dst[name] = append([]byte(nil), data...)
	}
	return nil
}

func (m *MemoryStore) ScanVerdicts(_ context.Context, acct Account, container string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.containers[key(acct, container)]; !ok {
		return nil, ErrNotFound
	}
	out := map[string]string{}
	prefix := key(acct, container) + "/"
	for k, v := range m.verdicts {
		if name, ok := strings.CutPrefix(k, prefix); ok {
			out[name] = v
		}
	}
	return out, nil
}

func (m *MemoryStore) ContainerURL(_ context.Context, acct Account, container string, perm Permissions, valid time.Duration) (string, error) {
	return fmt.Sprintf("https://%s.blob.core.windows.net/%s?sp=%s&se=%s", acct, container, perm.String(),
		time.Now().Add(valid).UTC().Format(time.RFC3339)), nil
}

// String renders permissions in Azure's canonical order.
func (p Permissions) String() string {
	s := ""
	if p.Read {
		s += "r"
	}
	if p.Write {
		s += "w"
	}
	if p.Delete {
		s += "d"
	}
	if p.List {
		s += "l"
	}
	return s
}
