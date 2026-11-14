//! Go tarafıyla birebir aynı tutulması gereken ağ ayarları ve yardımcılar.

use std::net::{Ipv4Addr, Ipv6Addr};
use std::time::Duration;

use libp2p::gossipsub;
use libp2p::identity::Keypair;
use libp2p::multiaddr::Protocol;
use libp2p::{Multiaddr, PeerId};
use sha2::{Digest, Sha256};

/// AND'ın DHT'deki buluşma anahtarı (protokol.Rendezvous).
pub const BULUSMA: &str = "and-community/1.0.0";

/// Röle hizmeti sunan düğümlerin protokolü.
pub const ROLE_PROTOKOLU: &str = "/libp2p/circuit/relay/0.2.0/hop";

/// go-libp2p-kad-dht v0.40'ın varsayılan bootstrap düğümleri (Go tarafıyla aynı).
pub const BOOTSTRAP: &[&str] = &[
    "/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
    "/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
    "/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
    "/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
    "/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
];

/// Go'nun drouting.nsToCid'i gibi: buluşma adının SHA-256 multihash'i. Go DHT
/// sağlayıcı kaydını bu multihash'in baytlarıyla tutar.
pub fn bulusma_anahtari() -> Vec<u8> {
    let ozet = Sha256::digest(BULUSMA.as_bytes());
    let mut mh = vec![0x12, 0x20];
    mh.extend_from_slice(&ozet);
    mh
}

/// go-libp2p-pubsub'ın DefaultMsgIdFn'i: göndericinin Peer ID baytları ve
/// 8 baytlık sıra numarası. IHAVE/IWANT iletileri bu kimlikleri taşıdığı için
/// Go düğümleriyle aynı olmalıdır.
fn go_mesaj_kimligi(m: &gossipsub::Message) -> gossipsub::MessageId {
    let mut kimlik = m.source.map(|p| p.to_bytes()).unwrap_or_default();
    kimlik.extend_from_slice(&m.sequence_number.unwrap_or(0).to_be_bytes());
    gossipsub::MessageId::from(kimlik)
}

/// AND'ın Go'daki NewPubSub ayarlarının karşılığı: 64 KiB sınır, imzalı
/// iletiler, sıkı imza denetimi, kendi iletisini bütün abonelere gönderme.
pub fn gossipsub(anahtar: &Keypair) -> Result<gossipsub::Behaviour, Box<dyn std::error::Error + Send + Sync>> {
    let ayar = gossipsub::ConfigBuilder::default()
        .max_transmit_size(64 * 1024)
        .validation_mode(gossipsub::ValidationMode::Strict)
        .flood_publish(true)
        .heartbeat_interval(Duration::from_secs(1))
        .message_id_fn(go_mesaj_kimligi)
        .build()?;
    Ok(gossipsub::Behaviour::new(gossipsub::MessageAuthenticity::Signed(anahtar.clone()), ayar)?)
}

/// Bir adresin internetten erişilebilir, doğrudan (röle olmayan) bir IP
/// adresi olup olmadığı (Go'daki adresinTuru == adresAcik).
pub fn acik_mi(a: &Multiaddr) -> bool {
    let mut acik = false;
    for p in a.iter() {
        match p {
            Protocol::P2pCircuit => return false,
            Protocol::Ip4(ip) => acik = acik_ip4(ip),
            Protocol::Ip6(ip) => acik = acik_ip6(ip),
            Protocol::Dns(_) | Protocol::Dns4(_) | Protocol::Dns6(_) | Protocol::Dnsaddr(_) => acik = true,
            _ => {}
        }
    }
    acik
}

fn acik_ip4(ip: Ipv4Addr) -> bool {
    !(ip.is_private()
        || ip.is_loopback()
        || ip.is_link_local()
        || ip.is_unspecified()
        || ip.is_multicast()
        || ip.is_broadcast()
        || ip.is_documentation()
        || ip.octets()[0] == 100 && (ip.octets()[1] & 0xC0) == 64) // 100.64/10 operatör NAT'ı
}

fn acik_ip6(ip: Ipv6Addr) -> bool {
    let s = ip.segments();
    !(ip.is_loopback()
        || ip.is_unspecified()
        || ip.is_multicast()
        || (s[0] & 0xfe00) == 0xfc00 // benzersiz yerel
        || (s[0] & 0xffc0) == 0xfe80) // bağlantı-yerel
}

/// Adresin TCP taşıyıcısı kullanıp kullanmadığı (deneme yalnızca TCP konuşur).
pub fn tcp_mi(a: &Multiaddr) -> bool {
    a.iter().any(|p| matches!(p, Protocol::Tcp(_)))
        && !a.iter().any(|p| matches!(p, Protocol::Ws(_) | Protocol::Wss(_) | Protocol::Tls | Protocol::P2pCircuit))
}

/// Adresin sonundaki /p2p/<kimlik>'i ayırır.
pub fn kimligi_ayir(a: &Multiaddr) -> Option<(PeerId, Multiaddr)> {
    let mut adres = a.clone();
    match adres.pop() {
        Some(Protocol::P2p(p)) => Some((p, adres)),
        _ => None,
    }
}
