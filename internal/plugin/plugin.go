// Package plugin defines the AND plugin interface and the shared environment
// that the main app passes to every plugin at startup.
//
// Dependency direction: plugin → crypto, network (no upward deps).
// Forum and other feature packages import plugin, not the other way around.
package plugin

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	stdcrypto "and/internal/crypto"
	"and/internal/network"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

// PendingPost is a forum post awaiting admin/moderator approval.
type PendingPost struct {
	ID         string
	Title      string
	AuthorName string
	Category   string
	ExpiresAt  time.Time
}

// Env is the set of app-level resources the main app makes available to every
// plugin during Init. Plugins should store what they need from Env and use it
// inside NewModel's returned tea.Model.
type Env struct {
	Ctx      context.Context
	Identity *stdcrypto.Identity
	Node     *network.Node
	// DataDir is the user's AND config directory (e.g. %APPDATA%\and on
	// Windows). Plugins may create their own files here.
	DataDir string
	// JoinTopic joins a named GossipSub topic. Plugins that need network
	// messaging call this during Init instead of touching pubsub directly.
	JoinTopic func(topicName string) (*network.Topic, error)
	// Routing, AND'nin DHT'si üzerine kurulu RoutingDiscovery; eklentiler
	// bağımsız DHT başlatmadan advertise/find yapabilir.
	Routing *drouting.RoutingDiscovery

	// Forum onay sistemi — nil ise forum hazır değil.
	PendingForumPosts  func() []PendingPost  // bekleyen konular
	LocalApprovePost   func(postID string)   // yerel DB'de onayla
	LocalApproveAuthor func(authorKey string) // yazarın tüm postlarını onayla
	// PublishApproval imzalayıp ağda yayınlar; nil → kullanıcı onaylayamaz.
	PublishApproval func(postID string) error

	// CreatePost yeni bir forum konusu oluşturur ve ağda yayınlar.
	// Nil ise bu düğüm salt okunur modda çalışıyor.
	CreatePost func(ctx context.Context, category, title, body string, permanentReq bool) error
}

// BackMsg is returned by a plugin's tea.Model when the user wants to go back
// to the main menu. The app's Update function catches it and switches screens.
type BackMsg struct{}

// OpenPostMsg is returned by a plugin when the user wants to navigate to a
// specific forum post. The app catches it, switches to the forum screen, and
// opens that post directly.
type OpenPostMsg struct{ PostID string }

// Plugin is the interface every AND plugin must satisfy.
//
//	Name       – unique lowercase identifier, used in log messages
//	MenuLabel  – text shown as a menu item in the main app menu
//	Init       – called once after login; plugin stores env for later use
//	NewModel   – factory called each time the user enters this plugin's screen
type Plugin interface {
	Name() string
	MenuLabel() string
	Init(env Env) error
	NewModel() tea.Model
}

// Registry holds registered plugins and the shared Env they were initialised with.
type Registry struct {
	env     Env
	plugins []Plugin
}

// New creates a Registry with the given Env. Call Register for each plugin
// before handing the registry to tui.Run.
func New(env Env) *Registry {
	return &Registry{env: env}
}

// Register calls p.Init and, on success, appends p to the registry.
// Returns an error if Init fails so main can surface it before the TUI starts.
func (r *Registry) Register(p Plugin) error {
	if err := p.Init(r.env); err != nil {
		return fmt.Errorf("plugin %q: init: %w", p.Name(), err)
	}
	r.plugins = append(r.plugins, p)
	return nil
}

// All returns all successfully registered plugins in registration order.
func (r *Registry) All() []Plugin {
	return r.plugins
}

// Env returns the shared environment the registry was initialised with.
func (r *Registry) Env() Env { return r.env }

// autoFactories holds plugin factories registered via AutoRegister.
// Populated by plugin packages' init() functions before main starts.
var autoFactories []func(Env) (Plugin, error)

// AutoRegister enqueues factory so NewWithAuto picks it up automatically.
// Plugin packages call this from their init() functions.
func AutoRegister(factory func(Env) (Plugin, error)) {
	autoFactories = append(autoFactories, factory)
}

// NewWithAuto creates a Registry, calls every auto-registered factory in
// registration order, and registers the resulting plugins. Returns an error
// if any factory or Init fails.
func NewWithAuto(env Env) (*Registry, error) {
	r := New(env)
	for _, factory := range autoFactories {
		p, err := factory(env)
		if err != nil {
			return nil, err
		}
		if err := r.Register(p); err != nil {
			return nil, err
		}
	}
	return r, nil
}
