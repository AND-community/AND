// Package e2e, AND düğümlerini cmd/and'in kurduğu gibi (libp2p düğümü,
// GossipSub, forum, DM aracısı, eklenti API'si) gerçek ağ bağlantılarıyla
// birlikte çalıştırır. Paket testleri her parçayı tek başına sınar; burada
// parçaların birbirine bağlandığı yerler sınanır: izinli eklenti anahtarı →
// API → DM akışı → karşı düğümün geçmişi, forum konusunun gossip ile
// yayılması ve sonradan katılan düğümün senkronla yakalaması.
package e2e

import (
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/dmmgr"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/pluginapi"
)

type dugum struct {
	id    *stdcrypto.Identity
	node  *network.Node
	konu  *network.Topic
	forum *forum.Forum
	dm    *dmmgr.Broker
	api   *pluginapi.Server
	addr  string
}

// kimlikArka ve forumArka, cmd/and'deki bağdaştırıcıların bu testin
// ihtiyacı kadarıdır.
type kimlikArka struct{ d *dugum }

func (k kimlikArka) Name() string                         { return k.d.id.Name() }
func (k kimlikArka) PubKeyHex() string                    { return hex.EncodeToString(k.d.id.PublicKey()) }
func (k kimlikArka) PeerIDStr() string                    { return k.d.node.Host.ID().String() }
func (k kimlikArka) IsFounder() bool                      { return false }
func (k kimlikArka) IsModerator() bool                    { return false }
func (k kimlikArka) ConnectedPeers() []pluginapi.PeerInfo { return nil }

type forumArka struct{ f *forum.Forum }

func (a forumArka) AllPosts() ([]pluginapi.PostSummary, error) { return nil, nil }
func (a forumArka) DeletePost(context.Context, string, string) error {
	return errors.New("bu testte yok")
}
func (a forumArka) CreatePost(ctx context.Context, kategori, baslik, govde string) error {
	_, err := a.f.CreatePost(ctx, kategori, baslik, govde)
	return err
}
func (a forumArka) GetPost(string) (*pluginapi.PostDetail, error) {
	return nil, errors.New("bu testte yok")
}
func (a forumArka) CreateReply(context.Context, string, string) error {
	return errors.New("bu testte yok")
}

func yeniDugum(t *testing.T, ctx context.Context, ad string) *dugum {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	id.SetName(ad)
	node, err := network.New(id, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = node.Close() })
	ps, err := network.NewPubSub(ctx, node.Host)
	if err != nil {
		t.Fatal(err)
	}
	konu, err := network.JoinTopic(ps, node.Host, network.ForumTopic)
	if err != nil {
		t.Fatal(err)
	}
	f, err := forum.New(id, konu, filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.DB().Close() })
	go f.Run(ctx)
	f.RegisterSync(node.Host)

	d := &dugum{id: id, node: node, konu: konu, forum: f, dm: dmmgr.New(node)}
	d.api = pluginapi.NewServer(kimlikArka{d}, forumArka{f}, nil, d.dm, nil, nil, nil, "", nil)
	addr, _, err := d.api.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	d.addr = addr
	return d
}

// eklenti, düğümün API'sine yalnızca verilen izinlerle bağlanan bir eklenti
// istemcisidir (AND'ın eklenti başlatırken yaptığı gibi).
func (d *dugum) eklenti(t *testing.T, izinler ...string) *pluginapi.Client {
	t.Helper()
	anahtar, geriAl, err := d.api.EklentiAnahtari("e2e", izinler)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(geriAl)
	t.Setenv("AND_API_ADDR", d.addr)
	t.Setenv("AND_API_TOKEN", anahtar)
	c, err := pluginapi.NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func bagla(t *testing.T, ctx context.Context, a, b *dugum) {
	t.Helper()
	if err := a.node.Host.Connect(ctx, peer.AddrInfo{ID: b.node.Host.ID(), Addrs: b.node.Host.Addrs()}); err != nil {
		t.Fatal(err)
	}
}

// bekle, kosul doğru olana kadar (en fazla süre kadar) bekler.
func bekle(t *testing.T, sure time.Duration, ne string, kosul func() bool) {
	t.Helper()
	son := time.Now().Add(sure)
	for !kosul() {
		if time.Now().After(son) {
			t.Fatalf("zaman aşımı: %s", ne)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestIkiDugum_DMEklentiAPIsiUzerinden(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a, b := yeniDugum(t, ctx, "ayse"), yeniDugum(t, ctx, "burak")
	bagla(t, ctx, a, b)

	izinsiz := a.eklenti(t)
	if err := izinsiz.SendDM(b.node.Host.ID().String(), "izinsiz"); !pluginapi.IsForbidden(err) {
		t.Fatalf("dm izni olmayan eklenti DM gönderememeli: %v", err)
	}

	gonderen := a.eklenti(t, pluginapi.IzinDM)
	if err := gonderen.SendDM(b.node.Host.ID().String(), "merhaba Burak"); err != nil {
		t.Fatalf("SendDM: %v", err)
	}

	alici := b.eklenti(t, pluginapi.IzinDM)
	var gelen pluginapi.DMRecord
	bekle(t, 10*time.Second, "DM B'nin geçmişine düşmeli", func() bool {
		kayitlar, err := alici.DMHistory()
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range kayitlar {
			if !k.Outgoing && k.Text == "merhaba Burak" {
				gelen = k
				return true
			}
		}
		return false
	})
	if gelen.PeerID != a.node.Host.ID().String() {
		t.Fatalf("gönderen, bağlantının doğrulanmış Peer ID'si olmalı: %q", gelen.PeerID)
	}
	if !strings.Contains(gelen.Name, "ayse") {
		t.Fatalf("gönderenin adı taşınmalı: %q", gelen.Name)
	}

	giden, err := gonderen.DMHistory()
	if err != nil || len(giden) != 1 || !giden[0].Outgoing || giden[0].PeerID != b.node.Host.ID().String() {
		t.Fatalf("gönderen tarafta giden mesaj kayıtlı olmalı: %+v %v", giden, err)
	}
}

func TestIkiDugum_ForumGossipVeSonradanKatilanSenkron(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a, b := yeniDugum(t, ctx, "ayse"), yeniDugum(t, ctx, "burak")
	bagla(t, ctx, a, b)
	if err := a.konu.AboneyiBekle(ctx, b.node.Host.ID()); err != nil {
		t.Fatal(err)
	}

	yazar := a.eklenti(t, pluginapi.IzinForum)
	if err := yazar.CreatePost("Go", "Uçtan uca", "İki düğüm arasında yayılan konu."); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	bekle(t, 10*time.Second, "konu gossip ile B'ye ulaşmalı", func() bool { return b.forum.TotalPosts() == 1 })
	konu := b.forum.PostsByCategory("Go")
	if len(konu) != 1 || konu[0].Title != "Uçtan uca" || konu[0].AuthorKey != hex.EncodeToString(a.id.PublicKey()) {
		t.Fatalf("B'deki konu A'nın imzalı konusu olmalı: %+v", konu)
	}

	// C, konu yayımlandıktan sonra katılır: gossip'i kaçırmıştır, senkronla yakalar.
	c := yeniDugum(t, ctx, "cem")
	bagla(t, ctx, c, a)
	if c.forum.TotalPosts() != 0 {
		t.Fatal("C senkrondan önce konuyu bilmemeli")
	}
	if err := c.forum.SyncWithPeer(ctx, c.node.Host, a.node.Host.ID()); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}
	if c.forum.TotalPosts() != 1 {
		t.Fatalf("C senkronla konuyu almalı, %d konu", c.forum.TotalPosts())
	}
}

func TestIkiDugum_GidenKutusu(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a, c := yeniDugum(t, ctx, "ayse"), yeniDugum(t, ctx, "cem")

	// C henüz bağlı değil: mesaj hata vermeden giden kutusuna girer.
	gonderen := a.eklenti(t, pluginapi.IzinDM)
	if err := gonderen.SendDMQueued(c.node.Host.ID().String(), "bağlanınca oku"); err != nil {
		t.Fatalf("SendDMQueued: %v", err)
	}
	if g, _ := gonderen.DMHistory(); len(g) != 1 || !g[0].Pending {
		t.Fatalf("mesaj bekliyor görünmeli: %+v", g)
	}

	bagla(t, ctx, a, c)
	alici := c.eklenti(t, pluginapi.IzinDM)
	bekle(t, 10*time.Second, "bekleyen mesaj C bağlanınca ulaşmalı", func() bool {
		g, err := alici.DMHistory()
		return err == nil && len(g) == 1 && g[0].Text == "bağlanınca oku"
	})
	bekle(t, 5*time.Second, "gönderilen mesaj artık bekliyor görünmemeli", func() bool {
		g, err := gonderen.DMHistory()
		return err == nil && len(g) == 1 && !g[0].Pending
	})
}
