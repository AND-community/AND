# AND yaması: libp2p-noise 0.47.0

Kaynak: crates.io'daki `libp2p-noise` 0.47.0 (MIT lisanslı, rust-libp2p).

Değişiklik yalnızca kriptografi çözümleyicisidir:

- `src/protocol.rs`: `resolve_hash` ve `resolve_cipher` her platformda
  `snow::resolvers::DefaultResolver` kullanır. Özgün kod wasm dışında
  `RingResolver` kullanır.
- `Cargo.toml`: `snow` her platformda `default-resolver` ve
  `default-resolver-crypto` özellikleriyle gelir (özgünde wasm dışında
  `ring-resolver`).

Neden: `ring` C ve assembly derler; Windows GNU araç zincirinde C derleyicisi
olmadan derlenmez. `DefaultResolver` aynı algoritmaları (ChaCha20-Poly1305,
SHA-256; X25519 zaten x25519-dalek ile yapılıyordu) saf Rust (RustCrypto) ile
uygular; Noise el sıkışması ve teldeki baytlar değişmez, Go düğümleriyle
uyumludur.
