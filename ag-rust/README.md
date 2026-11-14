# and-ag: AND ağ katmanının Rust denemesi

Bu klasör, AND'ın ağ katmanının (libp2p) Rust'a taşınmasının **ölçüm
aşamasıdır**. AND henüz bu programı kullanmaz; AND'ın ağı hâlâ Go'dadır
(`internal/network`). Amaç, taşımaya karar vermeden önce iki dili aynı
koşullarda ölçmektir. Eklenti sistemi (eklenti API'si, `pluginapi`,
`Eklentiler/`) her durumda Go'da kalır.

## Ne ölçülür

| Komut | Go karşılığı | Ölçtüğü |
|---|---|---|
| `and-ag yerel [ileti] [boyut]` | `TestOlcum_Yerel` | Aynı süreçte, loopback üzerinden bağlı iki düğüm (TCP + Noise + Yamux + AND'ın imzalı gossipsub ayarları): gecikme, verim, ileti başına işlemci süresi, tepe bellek. İnternet yoktur; dilin ve kütüphanenin hızı ölçülür |
| `and-ag canli [saniye]` | `TestOlcum_Canli` | Gerçek libp2p ağında: ilk bağlantı, DHT'nin dolması, AutoNAT, röleden yer ayırtma, buluşma noktasında arama, toplam işlemci ve bellek. Go tarafı AND'ın gerçek ağ katmanıdır; iki taraf da yalnızca TCP kullanır ve buluşma noktasına duyuru yapmaz |
| `and-ag uyum <tohum> <konu>` | `TestOlcum_Uyum` | Go testi Rust düğümünü başlatır: aynı Ed25519 tohumundan aynı Peer ID, AND'ın forum konusunda iki yönde, iki tarafın da sıkı denetlediği imzalı gossip |

Rust tarafı AND'ın Go ayarlarını taklit eder: gossipsub (64 KiB sınır, imzalı
iletiler, sıkı denetim, kendi iletisini bütün abonelere gönderme, Go'nunkiyle
aynı ileti kimliği), aynı bootstrap düğümleri, aynı röle kuralları (AutoNAT
"NAT arkasında" deyince, en az iki aday ya da 10 sn, iki röle, 15 dk geri
çekilme) ve aynı arama aralıkları (30, 60, 120 sn).

## Derleme

Linux ve macOS'ta `cargo build --release` yeterlidir. Windows'ta Rust'ın GNU
araç zinciriyle, C derleyicisi ve Visual Studio C++ araçları olmadan derlenir:

```sh
rustup component add llvm-tools
./derle.sh build --release        # target/release/and-ag.exe
```

Bunun için iki ayar gerekir:

- `.cargo/config.toml`: bağlayıcı olarak Rust'ın kendi `rust-lld`'si. Rust'ın
  paketlediği GNU `ld`, raw-dylib içe aktarma kütüphanelerindeki bazı girişleri
  bağlamıyor; program `main`'e girmeden erişim ihlaliyle çöküyordu.
- `derle.sh`: raw-dylib için gereken `dlltool` olarak `llvm-tools`'taki
  `llvm-ar`'ı kullanır (GNU `dlltool`, pakette olmayan bir montajcı ister).

`yama/` altında iki küçük yama vardır; ikisi de `ring`'in (C kodu) yerine saf
Rust (RustCrypto) koyar ve teldeki baytları değiştirmez:

- `yama/libp2p-noise`: Noise şifrelemesi (ChaCha20-Poly1305, SHA-256).
- `yama/libp2p-identity`: uzak düğümlerin RSA anahtarlarının doğrulanması
  (libp2p'nin genel bootstrap düğümleri RSA kullanır). RSA özel anahtarları
  desteklenmez; AND kimlikleri Ed25519'dur.

## Çalıştırma

```sh
./target/release/and-ag yerel 20000 1024
./target/release/and-ag canli 120
AND_AG_ROLE=erken ./target/release/and-ag canli 120   # röleye AutoNAT'ı beklemeden başla

# Go tarafı (AND deposunun kökünden)
go test -tags olcum -run TestOlcum_Yerel -v -count=1 ./internal/network
OLCUM_SURE=120s go test -tags olcum -run TestOlcum_Canli -v -count=1 -timeout 300s ./internal/network
AND_AG=<and-ag.exe'nin tam yolu> go test -tags olcum -run TestOlcum_Uyum -v -count=1 ./internal/network
```

Her ölçüm tek satırlık bir JSON yazar; iki dilin alanları aynıdır.
