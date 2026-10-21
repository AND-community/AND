package protokol

import "testing"

func TestMakineIletisiMi(t *testing.T) {
	for metin, beklenen := range map[string]bool{
		"AND-PROJEAG/1|eyJ0aXAiOiJkYXZldCJ9": true,
		"AND-PROJEAG/2|":                     true, // sonraki sürümler de makine mesajıdır
		"merhaba":                            false,
		"":                                   false,
		" AND-PROJEAG/1|x":                   false, // başında boşluk: bir insan yazmış
		"and-projeag/1|x":                    false,
		"AND-PROJEAG":                        false,
	} {
		if got := MakineIletisiMi(metin); got != beklenen {
			t.Errorf("MakineIletisiMi(%q) = %v; %v beklenirdi", metin, got, beklenen)
		}
	}
}
