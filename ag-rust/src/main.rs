//! and-ag: AND'ın ağ katmanının Rust denemesi (ölçüm aşaması).
//!
//!   and-ag yerel [ileti_sayisi] [boyut]   yerel gossip ölçümü (iki düğüm, loopback)
//!   and-ag canli [saniye]                 gerçek ağda keşif ölçümü (duyuru yapmaz)
//!   and-ag uyum <tohum-hex> <konu>        Go ile uyumluluk sınaması (Go testi başlatır)
//!
//! Sonuçlar tek satırlık JSON olarak yazılır; Go tarafının aynı ölçümleri
//! internal/network/olcum_test.go'dadır (derleme etiketi: olcum).

// libp2p 0.57'nin NetworkBehaviour türetmesi "unreachable call" uyarısı üretiyor.
#![allow(unreachable_code)]

mod canli;
mod olcum;
mod ortak;
mod uyum;
mod yerel;

use std::process::ExitCode;
use std::time::Duration;

type Sonuc = Result<(), Box<dyn std::error::Error + Send + Sync>>;

fn main() -> ExitCode {
    // libp2p'nin async durum makineleri büyüktür; Windows'un 1 MB'lık ana iş
    // parçacığı yığınına sığmaz. İş, heap'e alınmış (Box::pin) bir gelecek
    // olarak, yığını geniş çalışan iş parçacıklarında yürütülür.
    let rt = match tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .thread_stack_size(8 * 1024 * 1024)
        .build()
    {
        Ok(rt) => rt,
        Err(e) => {
            eprintln!("and-ag: {e}");
            return ExitCode::FAILURE;
        }
    };
    let sonuc = rt.block_on(async {
        match tokio::spawn(async { calistir().await.map_err(|e| e.to_string()) }).await {
            Ok(s) => s,
            Err(e) => Err(e.to_string()),
        }
    });
    match sonuc {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            eprintln!("and-ag: {e}");
            ExitCode::FAILURE
        }
    }
}

async fn calistir() -> Sonuc {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let sayi = |i: usize, varsayilan: u64| args.get(i).and_then(|s| s.parse().ok()).unwrap_or(varsayilan);
    match args.first().map(String::as_str) {
        Some("yerel") => Box::pin(yerel::calistir(sayi(1, 20_000) as usize, sayi(2, 1024) as usize)).await,
        Some("canli") => Box::pin(canli::calistir(Duration::from_secs(sayi(1, 120)))).await,
        Some("uyum") => match (args.get(1), args.get(2)) {
            (Some(t), Some(k)) => match uyum::tohum_coz(t) {
                Ok(tohum) => Box::pin(uyum::calistir(tohum, k)).await,
                Err(e) => Err(e),
            },
            _ => Err("kullanım: and-ag uyum <tohum-hex> <konu>".into()),
        },
        _ => Err("kullanım: and-ag yerel [ileti_sayisi] [boyut] | canli [saniye] | uyum <tohum-hex> <konu>".into()),
    }
}
