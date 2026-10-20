package main

// protokol.go — bir site paylaşım kodunu (bkz. index.go'nun
// SiteSayfasiBul'u) TIKLANABİLİR bir bağlantı haline getiren özel URI
// şeması (and-site://<yazarAnahtarı>/<slug>). İşletim sistemine bu şemayı
// (protokolKaydet ile) bir kez kaydettirdikten sonra, bu linki herhangi
// bir yerde (sohbet, e-posta, tarayıcı adres çubuğu) TIKLAMAK işletim
// sistemine bu programı o URI'yi tek argüman olarak vererek başlatmasını
// söyler — main()'deki siteURIYiCoz bunu -site bayrağıyla AYNI koda
// (siteCLICalistir) yönlendirir, TUI/ağ hiç açılmaz.
//
// Bu, "AND arama motoru/README.md"'nin Site sayfası paylaşımı bölümünde
// belgelenen "gerçek bir web adresi/tarayıcı linki DEĞİLDİR" sınırını
// KALDIRMAZ — hâlâ yalnızca bu programı (ya da AND'ı) kurmuş ve o içeriği
// zaten kendi indeksine senkronize etmiş biri için işler. Yaptığı tek şey,
// zaten var olan paylaşım kodunu elle kopyala-yapıştır yerine tek tıkla
// açılabilir hale getirmek.

import "strings"

// siteURIScheme, kayıtlı özel URI şemasının adıdır (RFC 3986 şema
// sözdizimine uygun: yalnızca harf/rakam/'-' — "://" HARİÇ).
const siteURIScheme = "and-site"

// siteURIOnEki, tam bir siteURI() çıktısının başındaki sabit kısımdır.
const siteURIOnEki = siteURIScheme + "://"

// siteURI, kod'u (bkz. SiteSayfasiBul'un "yazarAnahtarı/slug" biçimi)
// tıklanabilir bir and-site:// bağlantısına çevirir.
func siteURI(kod string) string {
	return siteURIOnEki + kod
}

// kodFromSiteURI, bir and-site:// URI'sinden kod'u (yazarAnahtarı/slug)
// çıkarır. uri bu şemayla başlamıyorsa ok=false döner — çağıran bunu "bu
// argüman bir and-site linki değil" anlamında kullanmalı, hata değil.
//
// Şema karşılaştırması BÜYÜK/küçük harf DUYARSIZDIR (RFC 3986 §3.1: URI
// şeması harf büyüklüğünden bağımsızdır — bazı istemciler/kabuklar bir
// linki panoya kopyalarken ya da bir e-posta istemcisi otomatik biçimlerken
// şemayı büyütebilir), ama kod'un kendisi (yazarAnahtarı hex'i büyük/küçük
// harfe DUYARLI olabileceğinden) OLDUĞU GİBİ, dönüştürülmeden döner.
// Baştaki/sondaki boşluk (ör. bir kabuğun argümanı aktarırken eklediği)
// TrimSpace ile temizlenir.
func kodFromSiteURI(uri string) (kod string, ok bool) {
	uri = strings.TrimSpace(uri)
	if len(uri) <= len(siteURIOnEki) || !strings.EqualFold(uri[:len(siteURIOnEki)], siteURIOnEki) {
		return "", false
	}
	return uri[len(siteURIOnEki):], true
}

// resolveSiteKod, main()'in -site bayrağı ile bir and-site:// URI
// argümanı arasındaki önceliği belirleyen SAF (yan etkisiz, test edilebilir)
// mantığıdır: -site AÇIKÇA verilmişse (bayrakSite boş değilse) HER ZAMAN
// kazanır — pozisyonel argüman yalnızca -site verilmemişse ve gerçekten
// geçerli bir and-site:// URI'siyse dikkate alınır. Diğer her durumda
// (URI değilse, boşsa) bayrakSite değişmeden döner.
func resolveSiteKod(bayrakSite string, pozisyonelArg string) string {
	if bayrakSite != "" {
		return bayrakSite
	}
	if kod, ok := kodFromSiteURI(pozisyonelArg); ok {
		return kod
	}
	return bayrakSite
}
