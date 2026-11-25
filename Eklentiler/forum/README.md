# and-plugin-forum — Forum

Kategoriler, konular ve yanıtlar; yeni konu açma ve taslaklar.

---

## Genel Bakış

`and-plugin-forum`, AND'ın forumunun arayüzüdür. Forumun verisi, imzaları ve
ağ eşitlemesi AND'da kalır; eklenti konuları ve yanıtları eklenti API'si
(`pluginapi`) üzerinden okur ve yazar, hiçbir zaman özel anahtar görmez.

- **Terminalde:** kategoriler (konu sayılarıyla) → konular (son etkinliğe göre)
  → konu (gövde ve yanıtlar) → yanıt yazma. `n` yeni konu açar, `p` yazarın
  profilini gösterir, kendi konunda `x` siler.
- **AND masaüstünde:** terminal yerine AND'ın kendi arayüzüyle açılır (bkz.
  `arayuz.go`).
- **Canlı:** yeni konu ve yanıtlar `GET /api/v1/forum/changes` ile gelir; eski
  bir AND'da liste `r` ile yenilenir.
- **Taslaklar:** yanıt taslakları (`yanit_taslaklari.json`) ve kategori başına
  konu taslakları (`taslaklar_<kategori>.json`) AND'ın veri dizininde durur;
  eski forum ekranının ve konu_ac eklentisinin taslakları aynen geçerlidir.

Forum yalnızca `forum` iznini ister. Yasaklama Yönetici Paneli'ndedir.
Konu açma eskiden ayrı `konu_ac` eklentisiydi; artık forumun bir parçasıdır.

## Kurulum

```bash
go build -o Eklentiler/forum/and-plugin-forum.exe ./Eklentiler/forum   # Windows
go build -o Eklentiler/forum/and-plugin-forum ./Eklentiler/forum
```

İkili, `plugin.json` ile aynı dizinde olmalıdır.
