package main

// cli.go — TUI hiç açılmadan, betikleme/otomasyon için tek seferlik
// komut satırı araması (-search). Profesyonel bir CLI aracının klasik
// izleği: aynı ikili hem etkileşimli (TUI) hem de betiklenebilir (tek
// komut, stdout'a yaz, çık) çalışabilmeli — ör. bir cron job'ın ya da
// başka bir aracın çıktısını `| jq` ile işleyebilmesi için.
//
// Yalnızca YEREL indekste arar: host/DHT/pubsub kurmanın (saniyeler
// süren) gecikmesi, hızlı/betiklenebilir bir sorgu için gereksiz ve
// istenmeyen bir maliyet olurdu — yerel indeks zaten önceki
// çalıştırmalardan senkronize edilmiş içeriği taşır.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// tipDenetle, -tip bayrağının değerini doğrular ("" = tümü).
func tipDenetle(s string) (SonucTip, error) {
	switch SonucTip(s) {
	case "", TipKonu, TipYanit, TipSohbet, TipSite:
		return SonucTip(s), nil
	default:
		return "", fmt.Errorf("geçersiz -tip değeri %q (konu|yanit|sohbet|site olmalı)", s)
	}
}

// cliSonuc, -search çıktısının (hem düz metin hem -json) taşıdığı temiz
// alan kümesidir — AramaSonucu'nun tamamını (ör. YazarKey gibi dahili
// alanları, vurgu sentinel'lerini) olduğu gibi dışa sızdırmak yerine.
type cliSonuc struct {
	Tip      SonucTip `json:"tip"`
	ID       string   `json:"id"`
	Baslik   string   `json:"baslik,omitempty"`
	Kategori string   `json:"kategori,omitempty"`
	Yazar    string   `json:"yazar,omitempty"`
	// YazarKey, -yazar bayrağıyla sonraki bir aramayı bu yazara
	// filtrelemek için kullanılabilecek gerçek yazar anahtarıdır (YazarAd'
	// ın aksine sahtesi mümkün olmayan kimlik) — keşfedilebilirlik için
	// çıktıya dahil edilir.
	YazarKey string `json:"yazar_key,omitempty"`
	Zaman    string `json:"zaman,omitempty"`
	Govde    string `json:"govde"`
}

func cliSonucaCevir(s AramaSonucu) cliSonuc {
	// Komut satırı da terminale yazar: ağdan gelmiş metin önce temizlenir.
	s = s.ekranIcin()
	zaman := ""
	if !s.OlusturmaTS.IsZero() {
		zaman = s.OlusturmaTS.Local().Format("2006-01-02 15:04")
	}
	return cliSonuc{
		Tip: s.Tip, ID: s.KaynakID, Baslik: s.Baslik, Kategori: s.Kategori,
		Yazar: s.YazarAd, YazarKey: s.YazarKey, Zaman: zaman, Govde: duzMetin(s.Govde),
	}
}

func (c cliSonuc) oncelikliBaslik() string {
	if c.Baslik != "" {
		return c.Baslik
	}
	return kirp(c.Govde, 60)
}

// aramaCLICalistir, sorgu'yu yerel indekste arayıp sonuçları stdout'a
// yazar. jsonCikti true ise girintili bir JSON dizisi, değilse insan
// tarafından okunabilir bir liste yazılır. sorgu boş ama yazarKey doluysa,
// bir anahtar kelime gerekmeden o yazarın tüm (en yeniden eskiye sıralı)
// içeriği listelenir — bkz. index.go'nun AraGelismis'i. sayfa 1'den
// başlar (kullanıcı-dostu; dahili offset'e çevrilir). Sonuç sıfırsa VE bir
// sorgu terimi verilmişse, bir "şunu mu demek istediniz?" önerisi stderr'e
// yazılır (stdout betiklerle işlendiğinden — ör. `| jq` — önerinin oraya
// karışmaması için).
func aramaCLICalistir(depo *Depo, sorgu string, tip SonucTip, yazarKey string, limit, sayfa int, jsonCikti bool) error {
	if sayfa < 1 {
		sayfa = 1
	}
	offset := (sayfa - 1) * limit
	aramaSayfasi, err := depo.AraGelismis(sorgu, tip, yazarKey, limit, offset)
	if err != nil {
		return fmt.Errorf("arama başarısız: %w", err)
	}
	sonuclar := aramaSayfasi.Sonuclar

	if jsonCikti {
		out := make([]cliSonuc, 0, len(sonuclar))
		for _, s := range sonuclar {
			out = append(out, cliSonucaCevir(s))
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return err
		}
		if len(sonuclar) == 0 && strings.TrimSpace(sorgu) != "" {
			if oneri, bulunduMu := depo.YazimOnerisiUret(sorgu); bulunduMu {
				fmt.Fprintf(os.Stderr, "Şunu mu demek istediniz: %s\n", oneri)
			}
		}
		return nil
	}

	if len(sonuclar) == 0 {
		fmt.Println("Sonuç yok.")
		if strings.TrimSpace(sorgu) != "" {
			if oneri, bulunduMu := depo.YazimOnerisiUret(sorgu); bulunduMu {
				fmt.Fprintf(os.Stderr, "Şunu mu demek istediniz: %s\n", oneri)
			}
		}
		return nil
	}
	for _, s := range sonuclar {
		c := cliSonucaCevir(s)
		fmt.Printf("[%s] %s\n", strings.ToUpper(string(c.Tip)), c.oncelikliBaslik())
		if c.Yazar != "" || c.Zaman != "" {
			fmt.Printf("  %s  %s\n", c.Yazar, c.Zaman)
		}
		fmt.Printf("  %s\n", kirp(c.Govde, 200))
		fmt.Printf("  id: %s\n\n", c.ID)
	}
	fmt.Printf("%d sonuç (sayfa %d)", len(sonuclar), sayfa)
	if aramaSayfasi.SonrakiVarMi {
		fmt.Printf(" — daha fazlası için -sayfa %d\n", sayfa+1)
	} else {
		fmt.Println(".")
	}
	return nil
}

// siteCLICalistir, kod (bkz. index.go'nun SiteSayfasiBul'u — bir site
// sayfasının "yazarAnahtarı/slug" paylaşım kodu) ile TAM eşleşen tek
// sayfayı yerel indekste arayıp stdout'a yazar. Birinin sana gönderdiği
// bir paylaşım koduyla doğrudan sayfayı açmanın betiklenebilir karşılığı —
// TUI'deki "Kodla Git" ekranıyla AYNI YEREL sorguyu kullanır (ağa çıkmaz).
func siteCLICalistir(depo *Depo, kod string, jsonCikti bool) error {
	s, bulunduMu, err := depo.SiteSayfasiBul(kod)
	if err != nil {
		return err
	}
	if !bulunduMu {
		if jsonCikti {
			fmt.Println("null")
			return nil
		}
		fmt.Println("Bu kod yerel indekste bulunamadı.")
		return nil
	}
	c := cliSonucaCevir(s)
	if jsonCikti {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(c)
	}
	fmt.Printf("[SİTE] %s\n", c.oncelikliBaslik())
	if c.Yazar != "" || c.Zaman != "" {
		fmt.Printf("  %s  %s\n", c.Yazar, c.Zaman)
	}
	fmt.Printf("\n%s\n\n", c.Govde)
	fmt.Printf("kod: %s\n", c.ID)
	return nil
}
