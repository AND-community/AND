# AND yaması: libp2p-identity 0.3.1

Kaynak: crates.io'daki `libp2p-identity` 0.3.1 (MIT lisanslı, rust-libp2p).

Değişiklik yalnızca RSA desteğidir (`rsa` özelliği):

- `src/rsa.rs`: RSA açık anahtar doğrulaması `ring` yerine RustCrypto `rsa`
  0.10 ile yapılır; kurallar aynıdır (PKCS#1 v1.5, SHA-256, 2048–8192 bitlik
  modül). RSA özel anahtarları desteklenmez: `Keypair::try_decode_*` hata döner,
  `Keypair` türü hiç oluşturulamaz. Özgün dosyadaki ring'e bağlı testler
  kaldırıldı.
- `Cargo.toml`: `rsa` özelliği `ring` yerine `rsa` ve `sha2` (`oid`
  özelliğiyle) crate'lerini açar; `ring` bağımlılıkları kaldırıldı.

Neden: libp2p'nin genel bootstrap düğümleri (Qm… kimlikli) RSA anahtarı
kullanır; onlarla el sıkışmak için RSA doğrulaması şarttır. `ring` C derler ve
Windows GNU araç zincirinde C derleyicisi olmadan derlenmez. AND kimlikleri
Ed25519'dur, bu yüzden RSA imzalamaya hiç gerek yoktur. RustCrypto `rsa`'nın
bilinen zamanlama açığı (RUSTSEC-2023-0071) özel anahtar işlemleriyle
ilgilidir; yalnızca doğrulama yapan bu kullanımı etkilemez.
