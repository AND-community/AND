//! Canlı ölçüm: geçici bir kimlikle gerçek libp2p ağına bağlanır ve AND'ın Go
//! tarafındaki keşfin (internal/network) zamanlamalarını taklit eder:
//! bootstrap, DHT, erişilebilirlik (AutoNAT), röleden yer ayırtma (aynı aday,
//! bekleme ve geri çekilme kuralları) ve buluşma noktasında arama (aynı
//! aralıklar). Buluşma noktasına DUYURU YAPMAZ: gerçek AND kullanıcıları bu
//! geçici düğümü bulmaz.

use std::collections::{HashMap, HashSet};
use std::error::Error;
use std::time::{Duration, Instant};

use futures::StreamExt;
use libp2p::core::transport::ListenerId;
use libp2p::multiaddr::Protocol;
use libp2p::swarm::{NetworkBehaviour, SwarmEvent};
use libp2p::{
    autonat, dcutr, identify, kad, noise, ping, relay, tcp, yamux, Multiaddr, PeerId, StreamProtocol,
    Swarm,
};
use serde_json::json;

use crate::{olcum, ortak};

#[derive(NetworkBehaviour)]
struct Davranis {
    relay: relay::client::Behaviour,
    identify: identify::Behaviour,
    kad: kad::Behaviour<kad::store::MemoryStore>,
    autonat: autonat::v1::Behaviour,
    dcutr: dcutr::Behaviour,
    ping: ping::Behaviour,
}

// AND'ın Go tarafındaki ayarları (internal/network/node.go, discovery.go).
const ROLE_BEKLEME: Duration = Duration::from_secs(10); // autorelay.WithBootDelay
const ROLE_EN_AZ_ADAY: usize = 2; // autorelay.WithMinCandidates
const ROLE_ISTENEN: usize = 2; // autorelay.WithNumRelays
const ROLE_GERI_CEKILME: Duration = Duration::from_secs(15 * 60); // autorelay.WithBackoff
const DHT_BEKLEME: Duration = Duration::from_secs(30); // dhtBekleme
const ARAMA_ARALIKLARI: [u64; 4] = [30, 60, 120, 240]; // aramaAraligi: hiç AND eşi yokken 30 sn, sonra katlanarak

fn swarm_kur() -> Result<Swarm<Davranis>, Box<dyn Error + Send + Sync>> {
    Ok(libp2p::SwarmBuilder::with_new_identity()
        .with_tokio()
        .with_tcp(tcp::Config::default().nodelay(true), noise::Config::new, yamux::Config::default)?
        .with_dns()?
        .with_relay_client(noise::Config::new, yamux::Config::default)?
        .with_behaviour(|anahtar, relay| {
            let kimlik = anahtar.public().to_peer_id();
            let mut kad_ayar = kad::Config::new(kad::PROTOCOL_NAME);
            kad_ayar.set_query_timeout(Duration::from_secs(60));
            Ok(Davranis {
                relay,
                identify: identify::Behaviour::new(
                    identify::Config::new_with_signed_peer_record("ipfs/0.1.0".into(), anahtar)
                        .with_agent_version("and-ag-olcum/0.1".into()),
                ),
                kad: kad::Behaviour::with_config(kimlik, kad::store::MemoryStore::new(kimlik), kad_ayar),
                autonat: autonat::v1::Behaviour::new(kimlik, Default::default()),
                dcutr: dcutr::Behaviour::new(kimlik),
                ping: ping::Behaviour::default(),
            })
        })?
        .with_swarm_config(|c| c.with_idle_connection_timeout(Duration::from_secs(60)))
        .build())
}

fn sn(t: Option<Instant>, bas: Instant) -> f64 {
    t.map(|t| t.duration_since(bas).as_secs_f64()).unwrap_or(-1.0)
}

pub async fn calistir(sure: Duration) -> Result<(), Box<dyn Error + Send + Sync>> {
    let mut swarm = swarm_kur()?;
    let ben = *swarm.local_peer_id();
    swarm.listen_on("/ip4/0.0.0.0/tcp/0".parse()?)?;
    swarm.listen_on("/ip6/::/tcp/0".parse()?)?;
    for a in ortak::BOOTSTRAP {
        let adres: Multiaddr = a.parse()?;
        if let Some((kimlik, yalin)) = ortak::kimligi_ayir(&adres) {
            swarm.behaviour_mut().kad.add_address(&kimlik, yalin);
        }
        let _ = swarm.dial(adres);
    }
    let _ = swarm.behaviour_mut().kad.bootstrap();

    let kad_protokolu = kad::PROTOCOL_NAME;
    let role_protokolu = StreamProtocol::new(ortak::ROLE_PROTOKOLU);
    let anahtar = kad::RecordKey::new(&ortak::bulusma_anahtari());

    let bas = Instant::now();
    let bitis = bas + sure;
    let (mut ilk_baglanti, mut dht10, mut dht20, mut erisim_t, mut ilk_role) = (None, None, None, None, None);
    let mut erisim = autonat::v1::NatStatus::Unknown;

    // Röle adayları: röle hizmeti sunan ve açık bir TCP adresi olan eşler.
    let mut adaylar: HashMap<PeerId, Multiaddr> = HashMap::new();
    let mut geri_cekilen: HashMap<PeerId, Instant> = HashMap::new();
    let mut bekleyen: HashMap<ListenerId, PeerId> = HashMap::new();
    let mut roleler: HashSet<PeerId> = HashSet::new();

    // Buluşma noktası araması.
    let mut arama: Option<(kad::QueryId, Instant, usize)> = None;
    let mut sonraki_arama: Option<Instant> = None;
    let (mut arama_sayisi, mut ilk_arama_sure, mut bulunan) = (0usize, None::<f64>, HashSet::<PeerId>::new());

    // Go'nun autorelay'ı röle aramaya ancak AutoNAT erişilebilirliği belirleyince
    // (NAT arkasında) başlar; karşılaştırma için aynısı yapılır. AND_AG_ROLE=erken
    // ile beklemeden başlanır (Go'da da uygulanabilecek bir strateji farkı).
    let erken_role = std::env::var("AND_AG_ROLE").as_deref() == Ok("erken");

    let mut tik = tokio::time::interval(Duration::from_millis(200));
    while Instant::now() < bitis {
        tokio::select! {
            _ = tik.tick() => {
                let simdi = Instant::now();
                let dht: usize = swarm.behaviour_mut().kad.kbuckets().map(|k| k.num_entries()).sum();
                if dht >= 10 && dht10.is_none() { dht10 = Some(simdi); }
                if dht >= 20 && dht20.is_none() { dht20 = Some(simdi); }

                // Arama: DHT bir eş tanıyınca (en geç 30 sn sonra) başlar.
                if arama.is_none() {
                    let zamani = match sonraki_arama {
                        None => dht >= 1 || simdi.duration_since(bas) >= DHT_BEKLEME,
                        Some(t) => simdi >= t,
                    };
                    if zamani {
                        let q = swarm.behaviour_mut().kad.get_providers(anahtar.clone());
                        arama = Some((q, simdi, 0));
                    }
                }

                // Röle: erişilebilir değilken; en az iki aday ya da bekleme dolunca.
                let nat_arkasinda = matches!(erisim, autonat::v1::NatStatus::Private);
                if (erken_role || nat_arkasinda) && roleler.len() + bekleyen.len() < ROLE_ISTENEN {
                    geri_cekilen.retain(|_, t| simdi.duration_since(*t) < ROLE_GERI_CEKILME);
                    let uygun: Vec<(PeerId, Multiaddr)> = adaylar
                        .iter()
                        .filter(|(p, _)| !roleler.contains(*p) && !geri_cekilen.contains_key(*p) && !bekleyen.values().any(|b| b == *p))
                        .map(|(p, a)| (*p, a.clone()))
                        .collect();
                    let basla = uygun.len() >= ROLE_EN_AZ_ADAY || simdi.duration_since(bas) >= ROLE_BEKLEME;
                    if basla {
                        for (p, a) in uygun.into_iter().take(ROLE_ISTENEN - roleler.len() - bekleyen.len()) {
                            let devre = a.with(Protocol::P2p(p)).with(Protocol::P2pCircuit);
                            match swarm.listen_on(devre) {
                                Ok(id) => { bekleyen.insert(id, p); }
                                Err(_) => { geri_cekilen.insert(p, simdi); }
                            }
                        }
                    }
                }
            }
            olay = swarm.select_next_some() => match olay {
                SwarmEvent::OutgoingConnectionError { peer_id, error, .. } if std::env::var_os("AND_AG_IZ").is_some() => {
                    eprintln!("[iz] bağlanılamadı {peer_id:?}: {error}");
                }
                SwarmEvent::ConnectionEstablished { .. } => {
                    if ilk_baglanti.is_none() { ilk_baglanti = Some(Instant::now()); }
                }
                SwarmEvent::Behaviour(DavranisEvent::Identify(identify::Event::Received { peer_id, info, .. })) => {
                    if info.protocols.contains(&kad_protokolu) {
                        for a in &info.listen_addrs {
                            swarm.behaviour_mut().kad.add_address(&peer_id, a.clone());
                        }
                    }
                    if info.protocols.contains(&role_protokolu) {
                        if let Some(a) = info.listen_addrs.iter().find(|a| ortak::acik_mi(a) && ortak::tcp_mi(a)) {
                            adaylar.insert(peer_id, a.clone());
                        }
                    }
                }
                SwarmEvent::Behaviour(DavranisEvent::Autonat(autonat::v1::Event::StatusChanged { new, .. })) => {
                    if !matches!(new, autonat::v1::NatStatus::Unknown) && erisim_t.is_none() {
                        erisim_t = Some(Instant::now());
                    }
                    erisim = new;
                }
                SwarmEvent::Behaviour(DavranisEvent::Relay(relay::client::Event::ReservationReqAccepted { relay_peer_id, renewal: false, .. })) => {
                    if ilk_role.is_none() { ilk_role = Some(Instant::now()); }
                    roleler.insert(relay_peer_id);
                    bekleyen.retain(|_, p| *p != relay_peer_id);
                }
                SwarmEvent::ListenerClosed { listener_id, .. } => {
                    if let Some(p) = bekleyen.remove(&listener_id) {
                        roleler.remove(&p);
                        geri_cekilen.insert(p, Instant::now());
                    }
                }
                SwarmEvent::Behaviour(DavranisEvent::Kad(kad::Event::OutboundQueryProgressed { id, result, step, .. })) => {
                    if let Some((q, baslangic, _)) = arama {
                        if q == id {
                            if let kad::QueryResult::GetProviders(Ok(kad::GetProvidersOk::FoundProviders { providers, .. })) = result {
                                bulunan.extend(providers.into_iter().filter(|p| *p != ben));
                            }
                            if step.last {
                                let gecen = baslangic.elapsed().as_secs_f64();
                                if ilk_arama_sure.is_none() { ilk_arama_sure = Some(gecen); }
                                let aralik = ARAMA_ARALIKLARI[arama_sayisi.min(ARAMA_ARALIKLARI.len() - 1)];
                                arama_sayisi += 1;
                                arama = None;
                                sonraki_arama = Some(Instant::now() + Duration::from_secs(aralik));
                            }
                        }
                    }
                }
                _ => {}
            }
        }
    }

    let erisim_adi = match erisim {
        autonat::v1::NatStatus::Public(_) => "Public",
        autonat::v1::NatStatus::Private => "Private",
        autonat::v1::NatStatus::Unknown => "bilinmiyor",
    };
    let dht: usize = swarm.behaviour_mut().kad.kbuckets().map(|k| k.num_entries()).sum();
    println!(
        "{}",
        json!({
            "dil": if erken_role { "rust-erken-role" } else { "rust" },
            "olcum": "canli",
            "sure_sn": sure.as_secs(),
            "ilk_baglanti_sn": sn(ilk_baglanti, bas),
            "dht10_sn": sn(dht10, bas),
            "dht20_sn": sn(dht20, bas),
            "erisim_sn": sn(erisim_t, bas),
            "erisim": erisim_adi,
            "ilk_role_sn": sn(ilk_role, bas),
            "role_sayisi": roleler.len(),
            "role_adayi": adaylar.len(),
            "ilk_arama_sure_sn": ilk_arama_sure.unwrap_or(-1.0),
            "arama_sayisi": arama_sayisi,
            "bulunan_saglayici": bulunan.len(),
            "son_dht": dht,
            "son_baglanti": swarm.connected_peers().count(),
            "cpu_sn": olcum::islemci_sn(),
            "tepe_bellek_mb": olcum::tepe_bellek_mb(),
        })
    );
    Ok(())
}
