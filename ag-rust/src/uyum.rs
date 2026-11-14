//! Go ile uyumluluk sınaması: Go tarafındaki test bu programı başlatır, aynı
//! Ed25519 tohumundan aynı Peer ID'nin çıktığını denetler, AND'ın konusu
//! üzerinden imzalı bir ileti gönderir ve Rust'ın gönderdiğini alır. İki taraf
//! da imzaları sıkı denetler. Satır satır protokol (stdout):
//!
//!   KIMLIK <peer id>       bu düğümün kimliği
//!   ADRES <multiaddr>      Go'nun bağlanacağı adres
//!   GONDERDI               Go'nun aboneliği görüldü ve ileti yayımlandı
//!   ALDI <metin>           Go'nun iletisi geldi (imzası doğrulanmış)
//!   TAMAM                  iki yön de bitti

use std::error::Error;
use std::io::Write;
use std::time::{Duration, Instant};

use futures::StreamExt;
use libp2p::identity::{ed25519, Keypair};
use libp2p::multiaddr::Protocol;
use libp2p::swarm::SwarmEvent;
use libp2p::gossipsub;

use crate::yerel::{swarm_kur, DavranisEvent};

pub const RUST_ILETISI: &str = "rust'tan merhaba";

fn yaz(satir: String) {
    let mut out = std::io::stdout().lock();
    let _ = writeln!(out, "{satir}");
    let _ = out.flush();
}

pub async fn calistir(tohum: [u8; 32], konu_adi: &str) -> Result<(), Box<dyn Error + Send + Sync>> {
    let anahtar = Keypair::from(ed25519::Keypair::from(ed25519::SecretKey::try_from_bytes(tohum)?));
    yaz(format!("KIMLIK {}", anahtar.public().to_peer_id()));

    let mut swarm = swarm_kur(anahtar)?;
    let konu = gossipsub::IdentTopic::new(konu_adi);
    swarm.behaviour_mut().gossipsub.subscribe(&konu)?;
    swarm.listen_on("/ip4/127.0.0.1/tcp/0".parse()?)?;

    let bitis = Instant::now() + Duration::from_secs(30);
    let (mut gonderecek, mut gonderdi, mut aldi) = (false, false, false);
    let mut tik = tokio::time::interval(Duration::from_millis(100));
    while Instant::now() < bitis && !(gonderdi && aldi) {
        tokio::select! {
            _ = tik.tick() => {
                if gonderecek && !gonderdi
                    && swarm.behaviour_mut().gossipsub.publish(konu.clone(), RUST_ILETISI.as_bytes()).is_ok() {
                    gonderdi = true;
                    yaz("GONDERDI".into());
                }
            }
            olay = swarm.select_next_some() => match olay {
                SwarmEvent::NewListenAddr { address, .. } => {
                    yaz(format!("ADRES {}", address.with(Protocol::P2p(*swarm.local_peer_id()))));
                }
                SwarmEvent::Behaviour(DavranisEvent::Gossipsub(gossipsub::Event::Subscribed { .. })) => gonderecek = true,
                SwarmEvent::Behaviour(DavranisEvent::Gossipsub(gossipsub::Event::Message { message, .. })) => {
                    aldi = true;
                    yaz(format!("ALDI {}", String::from_utf8_lossy(&message.data)));
                }
                _ => {}
            }
        }
    }
    if !(gonderdi && aldi) {
        return Err(format!("uyum sınaması bitmedi (gönderdi={gonderdi}, aldı={aldi})").into());
    }
    yaz("TAMAM".into());
    // Gönderilen iletinin karşıya ulaşması için kısa bir süre daha çalış.
    let son = Instant::now() + Duration::from_secs(2);
    while Instant::now() < son {
        let _ = tokio::time::timeout(son.saturating_duration_since(Instant::now()), swarm.select_next_some()).await;
    }
    Ok(())
}

/// 64 haneli onaltılık metni 32 bayta çevirir.
pub fn tohum_coz(s: &str) -> Result<[u8; 32], Box<dyn Error + Send + Sync>> {
    if s.len() != 64 {
        return Err("tohum 64 onaltılık hane olmalı".into());
    }
    let mut out = [0u8; 32];
    for (i, parca) in s.as_bytes().chunks(2).enumerate() {
        out[i] = u8::from_str_radix(std::str::from_utf8(parca)?, 16)?;
    }
    Ok(out)
}
