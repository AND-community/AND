package protokol

import "testing"

// TestAgSabitleriDegismez: bu değerler ağın kendisidir. Biri değişirse o
// sürümle eskiler birbirini göremez. Değiştirmek gerekiyorsa yeni bir sürüm
// (ör. /and/sync/2.0.0) eklenir ve eskisi bir süre birlikte konuşulur; bu
// test de bilinçli olarak güncellenir.
func TestAgSabitleriDegismez(t *testing.T) {
	for ad, tc := range map[string][2]string{
		"ForumKonusu":            {ForumKonusu, "github.com/lucian95511/and/forum"},
		"SohbetKonusu":           {SohbetKonusu, "github.com/lucian95511/and/chat"},
		"SiteKonusu":             {SiteKonusu, "github.com/lucian95511/and/site/1.0.0"},
		"ProfilKonusu":           {ProfilKonusu, "github.com/lucian95511/and/profile/1.0.0"},
		"ModerasyonKonusu":       {ModerasyonKonusu, "github.com/lucian95511/and/moderation/1.0.0"},
		"ForumSenkronProtokolu":  {ForumSenkronProtokolu, "/and/sync/1.0.0"},
		"SohbetSenkronProtokolu": {SohbetSenkronProtokolu, "/and/chatsync/1.0.0"},
		"SiteSenkronProtokolu":   {SiteSenkronProtokolu, "/and/sitesync/1.0.0"},
		"DMProtokolu":            {DMProtokolu, "/and/dm/1.0.0"},
		"DosyaProtokolu":         {DosyaProtokolu, "/and/file/2.0.0"},
		"AramaProtokolu":         {AramaProtokolu, "/and-arama-motoru/1.0.0"},
		"Rendezvous":             {Rendezvous, "and-community/1.0.0"},
		"MDNSEtiketi":            {MDNSEtiketi, "and-community-mdns"},
		"DeltaXOnEki":            {DeltaXOnEki, "AND-PROJEAG/"},
	} {
		if tc[0] != tc[1] {
			t.Errorf("%s = %q; ağdaki değer %q", ad, tc[0], tc[1])
		}
	}
}
