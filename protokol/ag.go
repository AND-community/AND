package protokol

// GossipSub konuları. Bir konunun adı değişirse eski ve yeni sürümler
// birbirinin iletilerini hiç görmez: değiştirmek yerine yeni bir sürüm ekle.
const (
	ForumKonusu      = "github.com/lucian95511/and/forum"
	SohbetKonusu     = "github.com/lucian95511/and/chat"
	SiteKonusu       = "github.com/lucian95511/and/site/1.0.0"
	ProfilKonusu     = "github.com/lucian95511/and/profile/1.0.0"
	ModerasyonKonusu = "github.com/lucian95511/and/moderation/1.0.0"
)

// Eşler arası doğrudan akış (stream) protokolleri.
const (
	ForumSenkronProtokolu  = "/and/sync/1.0.0"
	SohbetSenkronProtokolu = "/and/chatsync/1.0.0"
	SiteSenkronProtokolu   = "/and/sitesync/1.0.0"
	DMProtokolu            = "/and/dm/1.0.0"
	DosyaProtokolu         = "/and/file/2.0.0"
	// AramaProtokolu, arama motorlarının birbirine sorgu ilettiği protokoldür;
	// AND düğümleri konuşmaz.
	AramaProtokolu = "/and-arama-motoru/1.0.0"
)

// Eş keşfi: DHT'deki buluşma anahtarı ve yerel ağdaki mDNS servis etiketi.
const (
	Rendezvous  = "and-community/1.0.0"
	MDNSEtiketi = "and-community-mdns"
)
