package main

// updatemsg_test.go — updateInner'ın agAramaSonucuMsg/baglantiAcildiMsg
// dallarının DOĞRUDAN (mesajı elle inşa edip m.Update'e vererek) testleri.
// Önceden bu dallar hiçbir testte üretilmiyordu (bkz. denetim,
// updateInner %28.9 kapsamdaydı).

import (
	"errors"
	"strings"
	"testing"
)

func TestModel_AgAramaSonucuMsg_Hata(t *testing.T) {
	m := testModel(t)
	m.agAraniyor = true

	next, _ := m.Update(agAramaSonucuMsg{err: errors.New("bağlantı koptu")})
	m = next.(model)

	if m.agAraniyor {
		t.Fatal("agAraniyor false olmalıydı")
	}
	if !m.isErr || !strings.Contains(m.notice, "bağlantı koptu") {
		t.Fatalf("hata bildirimi bekleniyordu, got isErr=%v notice=%q", m.isErr, m.notice)
	}
}

func TestModel_AgAramaSonucuMsg_BosSonuc(t *testing.T) {
	m := testModel(t)
	m.agAraniyor = true

	next, _ := m.Update(agAramaSonucuMsg{sonuclar: nil})
	m = next.(model)

	if m.agAraniyor {
		t.Fatal("agAraniyor false olmalıydı")
	}
	if m.isErr {
		t.Fatal("boş sonuç bir hata değildir")
	}
	if !strings.Contains(m.notice, "Ağdan ek sonuç gelmedi") {
		t.Fatalf("beklenmeyen bildirim: %q", m.notice)
	}
}

func TestModel_AgAramaSonucuMsg_SonucVar(t *testing.T) {
	m := testModel(t)
	m.agAraniyor = true

	sonuclar := []AramaSonucu{{KaynakID: "a"}, {KaynakID: "b"}}
	next, _ := m.Update(agAramaSonucuMsg{sonuclar: sonuclar})
	m = next.(model)

	if len(m.agSonuclari) != 2 {
		t.Fatalf("agSonuclari 2 eleman olmalıydı, got %d", len(m.agSonuclari))
	}
	if !strings.Contains(m.notice, "2 ek sonuç") {
		t.Fatalf("bildirim sonuç sayısını içermeliydi: %q", m.notice)
	}
}

func TestModel_BaglantiAcildiMsg_Hata(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(baglantiAcildiMsg{url: "https://example.com", err: errors.New("tarayıcı bulunamadı")})
	m = next.(model)

	if !m.isErr || !strings.Contains(m.notice, "tarayıcı bulunamadı") {
		t.Fatalf("hata bildirimi bekleniyordu, got isErr=%v notice=%q", m.isErr, m.notice)
	}
}

func TestModel_BaglantiAcildiMsg_Basarili(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(baglantiAcildiMsg{url: "https://example.com"})
	m = next.(model)

	if m.isErr {
		t.Fatal("başarılı açılışta hata bildirimi olmamalıydı")
	}
	if !strings.Contains(m.notice, "Tarayıcıda açıldı") || !strings.Contains(m.notice, "example.com") {
		t.Fatalf("beklenmeyen bildirim: %q", m.notice)
	}
}
