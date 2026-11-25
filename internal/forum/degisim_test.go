package forum

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"
)

func konu(id string) *Post {
	return &Post{ID: id, Category: "Go", AuthorKey: "yazar", AuthorName: "yazar", Title: id, Body: "b", CreatedAt: time.Now()}
}

func TestSurum_YeniKonuYanitSilmeArtirir(t *testing.T) {
	f := newTestForum(t)
	if f.Surum() != 0 {
		t.Fatalf("başlangıç sürümü 0 olmalı: %d", f.Surum())
	}
	f.storePost(konu("p1"))
	if f.Surum() != 1 {
		t.Fatalf("yeni konudan sonra 1: %d", f.Surum())
	}
	f.storePost(konu("p1")) // aynı konu tekrar gelirse değişmez
	if f.Surum() != 1 {
		t.Fatalf("yinelenen konu sürümü artırmamalı: %d", f.Surum())
	}
	f.storeReply(&Reply{ID: "r1", PostID: "p1", AuthorKey: "b", AuthorName: "b", Body: "y", CreatedAt: time.Now()})
	if f.Surum() != 2 {
		t.Fatalf("yanıttan sonra 2: %d", f.Surum())
	}
	if err := f.DeletePost("p1"); err != nil {
		t.Fatal(err)
	}
	if f.Surum() != 3 {
		t.Fatalf("silmeden sonra 3: %d", f.Surum())
	}
}

func TestBekle_HemenDoner(t *testing.T) {
	f := newTestForum(t)
	f.storePost(konu("p1"))
	f.storePost(konu("p2"))
	surum, konular, hepsi := f.Bekle(context.Background(), 0)
	if surum != 2 || hepsi || !slices.Equal(konular, []string{"p1", "p2"}) {
		t.Fatalf("Bekle(0) = %d %v %v", surum, konular, hepsi)
	}
	surum, konular, _ = f.Bekle(context.Background(), 1)
	if surum != 2 || !slices.Equal(konular, []string{"p2"}) {
		t.Fatalf("Bekle(1) = %d %v", surum, konular)
	}
}

func TestBekle_DegisiklikteUyanir(t *testing.T) {
	f := newTestForum(t)
	go func() {
		time.Sleep(100 * time.Millisecond)
		f.storePost(konu("p1"))
	}()
	ctx, iptal := context.WithTimeout(context.Background(), 5*time.Second)
	defer iptal()
	basla := time.Now()
	surum, konular, _ := f.Bekle(ctx, 0)
	if surum != 1 || !slices.Equal(konular, []string{"p1"}) || time.Since(basla) > 3*time.Second {
		t.Fatalf("Bekle = %d %v (%v)", surum, konular, time.Since(basla))
	}
}

func TestBekle_CtxBitince(t *testing.T) {
	f := newTestForum(t)
	ctx, iptal := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer iptal()
	surum, konular, hepsi := f.Bekle(ctx, 0)
	if surum != 0 || len(konular) != 0 || hepsi {
		t.Fatalf("zaman aşımında değişiklik yok dönmeli: %d %v %v", surum, konular, hepsi)
	}
}

func TestBekle_HalkaGeridenHepsi(t *testing.T) {
	f := newTestForum(t)
	for i := range halkaBoyu + 10 {
		f.storePost(konu(fmt.Sprintf("p%d", i)))
	}
	if _, _, hepsi := f.Bekle(context.Background(), 1); !hepsi {
		t.Fatal("halkanın gerisinde kalan istemci hepsi=true almalı")
	}
	if _, konular, hepsi := f.Bekle(context.Background(), uint64(halkaBoyu+9)); hepsi || len(konular) != 1 {
		t.Fatalf("son değişiklik: %v %v", konular, hepsi)
	}
}

// AND yeniden başlayınca sayaç sıfırlanır; eski bir sürümle gelen istemci
// beklemeden "hepsi" almalı.
func TestBekle_SayacGerideysHepsi(t *testing.T) {
	f := newTestForum(t)
	f.storePost(konu("p1"))
	surum, _, hepsi := f.Bekle(context.Background(), 99)
	if surum != 1 || !hepsi {
		t.Fatalf("Bekle(99) = %d %v", surum, hepsi)
	}
}
