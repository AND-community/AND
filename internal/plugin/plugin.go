package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/network"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

type PluginMeta struct {
	Version     string
	Description string
	Author      string
}

type MetaProvider interface {
	Meta() PluginMeta
}

type Lifecycle interface {
	OnEnable() error
	OnDisable()
}

type DependencyProvider interface {
	Requires() []string
}

type Capabilities struct {
	Network bool
	Forum   bool
	Admin   bool
}

type CapabilityProvider interface {
	Capabilities() Capabilities
}

type SettingsProvider interface {
	NewSettingsModel() tea.Model
}

type Command struct {
	Name        string
	Description string
	Usage       string
	Handler     func(args []string) string
}

type CommandProvider interface {
	Commands() []Command
}

type EventBus struct {
	mu        sync.RWMutex
	listeners map[string][]func(data any)
}

func NewEventBus() *EventBus {
	return &EventBus{listeners: make(map[string][]func(any))}
}

func (b *EventBus) Subscribe(event string, handler func(data any)) {
	b.mu.Lock()
	b.listeners[event] = append(b.listeners[event], handler)
	b.mu.Unlock()
}

func (b *EventBus) Publish(event string, data any) {
	b.mu.RLock()
	src := b.listeners[event]
	handlers := make([]func(any), len(src))
	copy(handlers, src)
	b.mu.RUnlock()
	for _, h := range handlers {
		h(data)
	}
}

type KVStore struct {
	mu   sync.RWMutex
	path string
	data map[string]string
}

func OpenKVStore(path string) *KVStore {
	s := &KVStore{path: path, data: make(map[string]string)}
	if path == "" {
		return s
	}
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &s.data)
	}
	return s
}

func (s *KVStore) Get(key string) (string, bool) {
	s.mu.RLock()
	v, ok := s.data[key]
	s.mu.RUnlock()
	return v, ok
}

func (s *KVStore) GetDefault(key, def string) string {
	if v, ok := s.Get(key); ok {
		return v
	}
	return def
}

func (s *KVStore) Set(key, val string) error {
	s.mu.Lock()
	s.data[key] = val
	s.mu.Unlock()
	return s.flush()
}

func (s *KVStore) Delete(key string) error {
	s.mu.Lock()
	delete(s.data, key)
	s.mu.Unlock()
	return s.flush()
}

func (s *KVStore) All() map[string]string {
	s.mu.RLock()
	out := make(map[string]string, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	s.mu.RUnlock()
	return out
}

func (s *KVStore) flush() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	s.mu.RLock()
	data, err := json.MarshalIndent(s.data, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

type PendingPost struct {
	ID         string
	Title      string
	AuthorName string
	AuthorKey  string
	Category   string
	Body       string
	ExpiresAt  time.Time
}

type Env struct {
	Ctx      context.Context
	Identity *stdcrypto.Identity
	Node     *network.Node

	DataDir string

	JoinTopic func(topicName string) (*network.Topic, error)

	Routing *drouting.RoutingDiscovery

	Events *EventBus

	PendingForumPosts  func() []PendingPost
	LocalApprovePost   func(postID string)
	LocalApproveAuthor func(authorKey string)
	PublishApproval    func(postID string) error
	RejectPost         func(postID string) error
	CreatePost         func(ctx context.Context, category, title, body string, permanentReq bool) error
}

func (e Env) OpenStore(pluginName string) *KVStore {
	if e.DataDir == "" {
		return OpenKVStore("")
	}
	dir := filepath.Join(e.DataDir, "plugins", pluginName)
	_ = os.MkdirAll(dir, 0o700)
	return OpenKVStore(filepath.Join(dir, "kv.json"))
}

func (e Env) PluginDir(pluginName string) string {
	if e.DataDir == "" {
		return os.TempDir()
	}
	dir := filepath.Join(e.DataDir, "plugins", pluginName)
	_ = os.MkdirAll(dir, 0o700)
	return dir
}

type BackMsg struct {
	ReturnToForum  bool
	ReturnCategory string
	Notice         string
}

type OpenPostMsg struct{ PostID string }

type OpenPluginMsg struct {
	Name     string
	Category string
}

type Plugin interface {
	Name() string
	MenuLabel() string
	Init(env Env) error
	NewModel() tea.Model
}

type CategoryStarter interface {
	Plugin
	NewModelForCategory(category string) tea.Model
}

type LoadError struct {
	Name string
	Err  error
}

func (e LoadError) Error() string { return fmt.Sprintf("plugin %q: %v", e.Name, e.Err) }

type Registry struct {
	mu           sync.RWMutex
	env          Env
	plugins      []Plugin
	byName       map[string]Plugin
	userDisabled map[string]bool
	folderAbsent map[string]bool
	loadErrors   []LoadError
	dataDir      string
}

type pluginState struct {
	Disabled []string `json:"disabled"`
}

func New(env Env) *Registry {
	return &Registry{
		env:          env,
		byName:       make(map[string]Plugin),
		userDisabled: make(map[string]bool),
		folderAbsent: make(map[string]bool),
	}
}

func (r *Registry) Register(p Plugin) error {
	r.mu.Lock()
	_, exists := r.byName[p.Name()]
	r.mu.Unlock()
	if exists {
		return fmt.Errorf("plugin %q: already registered", p.Name())
	}
	if err := p.Init(r.env); err != nil {
		return fmt.Errorf("plugin %q: init: %w", p.Name(), err)
	}
	r.mu.Lock()
	r.plugins = append(r.plugins, p)
	r.byName[p.Name()] = p
	r.mu.Unlock()
	return nil
}

func (r *Registry) All() []Plugin {
	r.mu.RLock()
	out := make([]Plugin, len(r.plugins))
	copy(out, r.plugins)
	r.mu.RUnlock()
	return out
}

func (r *Registry) Get(name string) (Plugin, bool) {
	r.mu.RLock()
	p, ok := r.byName[name]
	r.mu.RUnlock()
	return p, ok
}

func (r *Registry) Env() Env { return r.env }

func (r *Registry) LoadErrors() []LoadError { return r.loadErrors }

func (r *Registry) IsEnabled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return !r.userDisabled[name] && !r.folderAbsent[name]
}

func (r *Registry) Toggle(name string) {
	var (
		nowEnabled bool
		target     Plugin
	)
	r.mu.Lock()
	if !r.folderAbsent[name] {
		r.userDisabled[name] = !r.userDisabled[name]
		nowEnabled = !r.userDisabled[name]
		target = r.byName[name]
	}
	r.mu.Unlock()

	if target != nil {
		if lc, ok := target.(Lifecycle); ok {
			if nowEnabled {
				if err := lc.OnEnable(); err != nil {
					r.mu.Lock()
					r.userDisabled[name] = true
					r.mu.Unlock()
				}
			} else {
				lc.OnDisable()
			}
		}
	}
	_ = r.saveState()
}

func (r *Registry) SetFolderState(name string, present bool) {
	r.mu.Lock()
	r.folderAbsent[name] = !present
	r.mu.Unlock()
}

func (r *Registry) LoadState(dataDir string) {
	r.mu.Lock()
	r.dataDir = dataDir
	r.mu.Unlock()

	data, err := os.ReadFile(filepath.Join(dataDir, "plugins_state.json"))
	if err != nil {
		return
	}
	var s pluginState
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	r.mu.Lock()
	for _, name := range s.Disabled {
		r.userDisabled[name] = true
	}
	r.mu.Unlock()
}

func (r *Registry) saveState() error {
	r.mu.RLock()
	dataDir := r.dataDir
	var names []string
	for name, off := range r.userDisabled {
		if off {
			names = append(names, name)
		}
	}
	r.mu.RUnlock()

	if dataDir == "" {
		return nil
	}
	sort.Strings(names)
	s := pluginState{Disabled: names}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "plugins_state.json"), data, 0o600)
}

func (r *Registry) Commands() []Command {
	var cmds []Command
	for _, p := range r.plugins {
		if !r.IsEnabled(p.Name()) {
			continue
		}
		if cp, ok := p.(CommandProvider); ok {
			cmds = append(cmds, cp.Commands()...)
		}
	}
	return cmds
}

func (r *Registry) Dispatch(name string, args []string) (string, bool) {
	if name == "help" {
		return r.helpText(), true
	}
	for _, p := range r.plugins {
		if !r.IsEnabled(p.Name()) {
			continue
		}
		cp, ok := p.(CommandProvider)
		if !ok {
			continue
		}
		for _, cmd := range cp.Commands() {
			if cmd.Name == name {
				return cmd.Handler(args), true
			}
		}
	}
	return "", false
}

func (r *Registry) helpText() string {
	cmds := r.Commands()
	if len(cmds) == 0 {
		return "Kayıtlı slash komutu yok.  /help — bu mesaj"
	}
	var b strings.Builder
	b.WriteString("Kullanılabilir komutlar:\n")
	b.WriteString("  /help          — bu listeyi göster\n")
	for _, c := range cmds {
		usage := c.Usage
		if usage == "" {
			usage = c.Name
		}
		b.WriteString(fmt.Sprintf("  /%-14s — %s\n", usage, c.Description))
	}
	return strings.TrimRight(b.String(), "\n")
}

var autoFactories []func(Env) (Plugin, error)

func AutoRegister(factory func(Env) (Plugin, error)) {
	autoFactories = append(autoFactories, factory)
}

func NewWithAuto(env Env) (*Registry, error) {
	r := New(env)
	for _, factory := range autoFactories {
		r.tryRegisterFactory(factory)
	}
	return r, nil
}

func (r *Registry) tryRegisterFactory(factory func(Env) (Plugin, error)) {
	p, err := factory(r.env)
	if err != nil {
		r.loadErrors = append(r.loadErrors, LoadError{Name: "?", Err: err})
		return
	}

	if dp, ok := p.(DependencyProvider); ok {
		for _, dep := range dp.Requires() {
			if _, found := r.byName[dep]; !found {
				r.loadErrors = append(r.loadErrors, LoadError{
					Name: p.Name(),
					Err:  fmt.Errorf("bağımlı eklenti kayıtlı değil: %q (all.go'da bu eklentiden önce gelmeli)", dep),
				})
				return
			}
		}
	}

	if err := r.Register(p); err != nil {
		r.loadErrors = append(r.loadErrors, LoadError{Name: p.Name(), Err: err})
	}
}
