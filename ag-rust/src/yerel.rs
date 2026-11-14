//! Yerel gossip ölçümü: aynı süreçte, loopback üzerinden bağlı iki düğüm
//! (TCP + Noise + Yamux + imzalı gossipsub). İnternetin gecikmesi yoktur;
//! ölçülen, mesaj başına yapılan işin (imzalama, imza denetimi, protobuf,
//! şifreleme, çoğullama) maliyetidir: dilin ve kütüphanenin hızı.

use std::error::Error;
use std::time::{Duration, Instant};

use futures::StreamExt;
use libp2p::swarm::{NetworkBehaviour, SwarmEvent};
use libp2p::{gossipsub, identify, noise, ping, tcp, yamux, Multiaddr, Swarm};
use serde_json::json;
use tokio::sync::{mpsc, oneshot};

use crate::{olcum, ortak};

const KONU: &str = "and-olcum/1";

#[derive(NetworkBehaviour)]
pub(crate) struct Davranis {
    pub(crate) gossipsub: gossipsub::Behaviour,
    identify: identify::Behaviour,
    ping: ping::Behaviour,
}

/// Gossipsub, identify ve ping konuşan bir düğüm (Go'da libp2p.New'in
/// varsayılanları ve AND'ın NewPubSub'ı).
pub(crate) fn swarm_kur(anahtar: libp2p::identity::Keypair) -> Result<Swarm<Davranis>, Box<dyn Error + Send + Sync>> {
    Ok(libp2p::SwarmBuilder::with_existing_identity(anahtar)
        .with_tokio()
        .with_tcp(tcp::Config::default().nodelay(true), noise::Config::new, yamux::Config::default)?
        .with_behaviour(|anahtar| {
            Ok(Davranis {
                gossipsub: ortak::gossipsub(anahtar)?,
                identify: identify::Behaviour::new(identify::Config::new("ipfs/0.1.0".into(), anahtar.public())),
                ping: ping::Behaviour::default(),
            })
        })?
        .with_swarm_config(|c| c.with_idle_connection_timeout(Duration::from_secs(120)))
        .build())
}

/// Bir düğümden ana döngüye giden olay.
enum Olay {
    /// Karşı düğümün konuya abone olduğu görüldü.
    AboneGoruldu,
    /// i numaralı ileti geldi.
    Geldi(u64, Instant),
    /// Gönderilemeyen ileti.
    Gonderilemedi,
}

pub async fn calistir(n: usize, boyut: usize) -> Result<(), Box<dyn Error + Send + Sync>> {
    let konu = gossipsub::IdentTopic::new(KONU);
    let (olay_tx, mut olay_rx) = mpsc::unbounded_channel::<(char, Olay)>();

    // B: dinler, gelen iletileri bildirir.
    let mut b = swarm_kur(libp2p::identity::Keypair::generate_ed25519())?;
    b.behaviour_mut().gossipsub.subscribe(&konu)?;
    b.listen_on("/ip4/127.0.0.1/tcp/0".parse()?)?;
    let (adres_tx, adres_rx) = oneshot::channel::<Multiaddr>();
    let b_olay = olay_tx.clone();
    tokio::spawn(async move {
        let mut adres_tx = Some(adres_tx);
        loop {
            match b.select_next_some().await {
                SwarmEvent::NewListenAddr { address, .. } => {
                    if let Some(tx) = adres_tx.take() {
                        let _ = tx.send(address.with(libp2p::multiaddr::Protocol::P2p(*b.local_peer_id())));
                    }
                }
                SwarmEvent::Behaviour(DavranisEvent::Gossipsub(gossipsub::Event::Message { message, .. })) => {
                    let mut sira = [0u8; 8];
                    sira.copy_from_slice(&message.data[..8]);
                    let _ = b_olay.send(('B', Olay::Geldi(u64::from_be_bytes(sira), Instant::now())));
                }
                SwarmEvent::Behaviour(DavranisEvent::Gossipsub(gossipsub::Event::Subscribed { .. })) => {
                    let _ = b_olay.send(('B', Olay::AboneGoruldu));
                }
                _ => {}
            }
        }
    });

    // A: B'ye bağlanır, ana döngünün istediği iletileri yayımlar.
    let mut a = swarm_kur(libp2p::identity::Keypair::generate_ed25519())?;
    a.behaviour_mut().gossipsub.subscribe(&konu)?;
    a.dial(adres_rx.await?)?;
    let (gonder_tx, mut gonder_rx) = mpsc::unbounded_channel::<Vec<u8>>();
    let a_olay = olay_tx.clone();
    let a_konu = konu.clone();
    tokio::spawn(async move {
        loop {
            tokio::select! {
                olay = a.select_next_some() => {
                    if let SwarmEvent::Behaviour(DavranisEvent::Gossipsub(gossipsub::Event::Subscribed { .. })) = olay {
                        let _ = a_olay.send(('A', Olay::AboneGoruldu));
                    }
                }
                Some(veri) = gonder_rx.recv() => {
                    if a.behaviour_mut().gossipsub.publish(a_konu.clone(), veri).is_err() {
                        let _ = a_olay.send(('A', Olay::Gonderilemedi));
                    }
                }
            }
        }
    });

    // İki taraf da birbirinin aboneliğini görene kadar bekle (Go'da AboneyiBekle).
    let (mut a_hazir, mut b_hazir) = (false, false);
    let bitis = Instant::now() + Duration::from_secs(20);
    while !(a_hazir && b_hazir) {
        let kalan = bitis.saturating_duration_since(Instant::now());
        match tokio::time::timeout(kalan, olay_rx.recv()).await {
            Ok(Some(('A', Olay::AboneGoruldu))) => a_hazir = true,
            Ok(Some(('B', Olay::AboneGoruldu))) => b_hazir = true,
            Ok(Some(_)) => {}
            _ => return Err("düğümler birbirinin aboneliğini görmedi".into()),
        }
    }

    let ileti = |i: u64| {
        let mut v = vec![0x5a_u8; boyut];
        v[..8].copy_from_slice(&i.to_be_bytes());
        v
    };
    let mut sira: u64 = 0;

    // Isınma ve gecikme: her ileti gelene kadar bir sonrakini gönderme.
    let mut gecikmeler = Vec::with_capacity(1000);
    for tur in 0..1200 {
        let gonderim = Instant::now();
        gonder_tx.send(ileti(sira))?;
        loop {
            match tokio::time::timeout(Duration::from_secs(10), olay_rx.recv()).await {
                Ok(Some((_, Olay::Geldi(i, t)))) if i == sira => {
                    if tur >= 200 {
                        gecikmeler.push(t.duration_since(gonderim));
                    }
                    break;
                }
                Ok(Some((_, Olay::Gonderilemedi))) => return Err("gecikme ölçümünde ileti gönderilemedi".into()),
                Ok(Some(_)) => {}
                _ => return Err("gecikme ölçümünde ileti gelmedi".into()),
            }
        }
        sira += 1;
    }
    gecikmeler.sort();
    let yuzdelik = |p: f64| gecikmeler[((gecikmeler.len() as f64 - 1.0) * p) as usize].as_secs_f64() * 1e6;

    // Verim: en fazla 32 ileti yolda olacak şekilde n ileti (go-libp2p-pubsub
    // bir eşe giden kuyruğu 32 iletide dolunca iletiyi düşürür; Go tarafı da
    // bu pencereyle ölçülür).
    const PENCERE: u64 = 32;
    let ilk = sira;
    let (mut gonderilen, mut biten, mut kayip) = (0u64, 0u64, 0u64);
    let cpu_bas = olcum::islemci_sn();
    let bas = Instant::now();
    while biten < n as u64 {
        if gonderilen < n as u64 && gonderilen - biten < PENCERE {
            gonder_tx.send(ileti(ilk + gonderilen))?;
            gonderilen += 1;
            continue;
        }
        match tokio::time::timeout(Duration::from_secs(10), olay_rx.recv()).await {
            Ok(Some((_, Olay::Geldi(i, _)))) if i >= ilk => biten += 1,
            Ok(Some((_, Olay::Gonderilemedi))) => {
                biten += 1;
                kayip += 1;
            }
            Ok(Some(_)) => {}
            _ => return Err(format!("verim ölçümünde iletiler gelmedi ({biten}/{n})").into()),
        }
    }
    let sure = bas.elapsed().as_secs_f64();
    let cpu = olcum::islemci_sn() - cpu_bas;

    println!(
        "{}",
        json!({
            "dil": "rust",
            "olcum": "yerel",
            "n": n,
            "boyut": boyut,
            "gecikme_p50_us": yuzdelik(0.5),
            "gecikme_p99_us": yuzdelik(0.99),
            "verim_mesaj_sn": n as f64 / sure,
            "verim_mb_sn": (n * boyut) as f64 / sure / (1024.0 * 1024.0),
            "cpu_us_mesaj": cpu / n as f64 * 1e6,
            "kayip": kayip,
            "tepe_bellek_mb": olcum::tepe_bellek_mb(),
        })
    );
    Ok(())
}
