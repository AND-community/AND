package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestLisansDosyasiMi(t *testing.T) {
	for ad, want := range map[string]bool{
		"LICENSE":          true,
		"LICENSE.md":       true,
		"LICENSE.txt":      true,
		"LICENSE-MIT":      true,
		"LICENSE-APACHE":   true,
		"license":          true,
		"LICENCE":          true,
		"COPYING":          true,
		"COPYING.LESSER":   true,
		"COPYRIGHT":        true,
		"NOTICE":           true,
		"NOTICE.txt":       true,
		"PATENTS":          true,
		"UNLICENSE":        true,
		"README.md":        false,
		"license.go":       false,
		"licenses_test.go": false,
		"LICENSEE.md":      false,
		"go.mod":           false,
	} {
		if got := lisansDosyasiMi(ad); got != want {
			t.Errorf("lisansDosyasiMi(%q) = %v, beklenen %v", ad, got, want)
		}
	}
}

const (
	mitOrnek = `The MIT License (MIT)

Copyright (c) 2014 Juan Batiz-Benet

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction.`

	apacheOrnek = `
                                 Apache License
                           Version 2.0, January 2004
                        http://www.apache.org/licenses/

   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION`

	bsd3Ornek = `Copyright 2009 The Go Authors.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:
   * Neither the name of Google LLC nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.`

	bsd2Ornek = `Copyright (c) 2013, Patrick Mezard
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:
    Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.`

	iscOrnek = `ISC License

Copyright (c) 2012-2016 Dave Collins <dave@davec.name>

Permission to use, copy, modify, and/or distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.`

	// MPL'nin "Secondary License" tanımı GNU lisanslarını adıyla anar;
	// bu yüzden GPL ailesi olarak sınıflanmamalı.
	mplOrnek = `Mozilla Public License, version 2.0

1. Definitions
1.12. "Secondary License"
    means either the GNU General Public License, Version 2.0, the GNU
    Lesser General Public License, Version 2.1, the GNU Affero General
    Public License, Version 3.0, or any later versions of those licenses.`

	gplOrnek = `                    GNU GENERAL PUBLIC LICENSE
                       Version 3, 29 June 2007

 Copyright (C) 2007 Free Software Foundation, Inc. <https://fsf.org/>`

	unlicenseOrnek = `This is free and unencumbered software released into the public domain.`

	noticeOrnek = `This product includes software developed at
The Example Foundation (http://www.example.org/).`

	// github.com/multiformats/go-base36'nın LICENSE.md'si: lisans metni yerine
	// SPDX tanımlayıcısı taşır.
	spdxOrnek = "The software contents of this repository are Copyright (c) Protocol Labs,\n" +
		"Licensed under the `Permissive License Stack`, meaning either of:\n\n" +
		"- Apache-2.0 Software License: https://www.apache.org/licenses/LICENSE-2.0\n" +
		"- MIT Software License: https://opensource.org/licenses/MIT\n\n" +
		"<!--- SPDX-License-Identifier: Apache-2.0 OR MIT -->\n" +
		"`SPDX-License-Identifier: Apache-2.0 OR MIT`"
)

func TestLisansTurleri(t *testing.T) {
	for _, tt := range []struct {
		ad    string
		metin string
		want  []string
	}{
		{"MIT", mitOrnek, []string{"MIT"}},
		{"Apache", apacheOrnek, []string{"Apache-2.0"}},
		{"BSD-3", bsd3Ornek, []string{"BSD-3-Clause"}},
		{"BSD-2", bsd2Ornek, []string{"BSD-2-Clause"}},
		{"ISC", iscOrnek, []string{"ISC"}},
		{"MPL, GNU adlarını andığı halde", mplOrnek, []string{"MPL-2.0"}},
		{"GPL", gplOrnek, []string{"GPL-3.0"}},
		{"Unlicense", unlicenseOrnek, []string{"Unlicense"}},
		{"aynı dosyada iki lisans", mitOrnek + "\n\n" + apacheOrnek, []string{"Apache-2.0", "MIT"}},
		{"Windows satır sonları", strings.ReplaceAll(bsd3Ornek, "\n", "\r\n"), []string{"BSD-3-Clause"}},
		{"SPDX tanımlayıcısı", spdxOrnek, []string{"Apache-2.0", "MIT"}},
		{"SPDX ifadesi parantez ve WITH içerir", "SPDX-License-Identifier: (MIT AND BSD-3-Clause) OR Apache-2.0 WITH LLVM-exception",
			[]string{"Apache-2.0", "BSD-3-Clause", "MIT"}},
		{"NOTICE lisans değildir", noticeOrnek, nil},
		{"tanınmayan metin", "Bu yazılımı istediğiniz gibi kullanın.", nil},
	} {
		if got := lisansTurleri(tt.metin); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: lisansTurleri = %q, beklenen %q", tt.ad, got, tt.want)
		}
	}
}

func TestModulLisansiBirlesimdir(t *testing.T) {
	m := modul{Yol: "github.com/ornek/cift", Surum: "v1.0.0", Dosyalar: []lisansDosyasi{
		{Ad: "LICENSE-MIT", Metin: mitOrnek},
		{Ad: "LICENSE-APACHE", Metin: apacheOrnek},
		{Ad: "NOTICE", Metin: noticeOrnek},
	}}
	if got := m.lisans(); got != "Apache-2.0, MIT" {
		t.Errorf("lisans() = %q, beklenen %q", got, "Apache-2.0, MIT")
	}
	if got := (modul{Yol: "x", Dosyalar: []lisansDosyasi{{Ad: "NOTICE", Metin: noticeOrnek}}}).lisans(); got != "" {
		t.Errorf("yalnızca NOTICE taşıyan modülün lisansı %q, boş beklenirdi", got)
	}
}

func TestGruplaAyniMetniBirKezYazar(t *testing.T) {
	bsd := []lisansDosyasi{{Ad: "LICENSE", Metin: bsd3Ornek}, {Ad: "PATENTS", Metin: "Additional IP Rights Grant (Patents)"}}
	moduller := []modul{
		{Yol: "golang.org/x/sys", Surum: "v0.48.0", Dosyalar: bsd},
		{Yol: "github.com/ornek/mit", Surum: "v1.2.3", Dosyalar: []lisansDosyasi{{Ad: "LICENSE", Metin: mitOrnek}}},
		{Yol: "golang.org/x/crypto", Surum: "v0.57.0", Dosyalar: bsd},
	}
	gruplar := grupla(moduller)
	var basliklar [][]string
	for _, g := range gruplar {
		var adlar []string
		for _, m := range g.Moduller {
			adlar = append(adlar, m.Yol)
		}
		basliklar = append(basliklar, adlar)
	}
	want := [][]string{{"github.com/ornek/mit"}, {"golang.org/x/crypto", "golang.org/x/sys"}}
	if !reflect.DeepEqual(basliklar, want) {
		t.Fatalf("gruplar %q, beklenen %q", basliklar, want)
	}
	if len(gruplar[1].Dosyalar) != 2 {
		t.Errorf("BSD grubunda %d dosya, 2 (LICENSE, PATENTS) beklenirdi", len(gruplar[1].Dosyalar))
	}
}

func TestGruplaFarkliTelifSatiriniAyriTutar(t *testing.T) {
	// MIT metinleri telif satırında ayrışır; her modül kendi telif satırını taşımalı.
	baskaMIT := strings.Replace(mitOrnek, "Juan Batiz-Benet", "Protocol Labs", 1)
	gruplar := grupla([]modul{
		{Yol: "a", Surum: "v1", Dosyalar: []lisansDosyasi{{Ad: "LICENSE", Metin: mitOrnek}}},
		{Yol: "b", Surum: "v1", Dosyalar: []lisansDosyasi{{Ad: "LICENSE", Metin: baskaMIT}}},
	})
	if len(gruplar) != 2 {
		t.Fatalf("%d grup, 2 beklenirdi", len(gruplar))
	}
}

func TestBildirimMetni(t *testing.T) {
	moduller := []modul{
		{Yol: "go", Surum: "go1.26.9", Aciklama: "Go standard library", Dosyalar: []lisansDosyasi{{Ad: "LICENSE", Metin: bsd3Ornek}}},
		{Yol: "github.com/mattn/go-localereader", Surum: "v0.0.1", Not: "No license file in the module.",
			Dosyalar: []lisansDosyasi{{Ad: "LICENSE (README)", Metin: mitOrnek}}},
		{Yol: "golang.org/x/sys", Surum: "v0.48.0", Dosyalar: []lisansDosyasi{{Ad: "LICENSE", Metin: bsd3Ornek}}},
	}
	metin := bildirimMetni(moduller)

	for _, satir := range []string{
		"go (Go standard library)",
		"github.com/mattn/go-localereader",
		"golang.org/x/sys",
	} {
		if !strings.Contains(metin, satir) {
			t.Errorf("özet %q satırını içermiyor", satir)
		}
	}
	// go ve x/sys aynı BSD metnini taşır: metin bir kez yazılır, iki modül aynı başlıkta.
	if n := strings.Count(metin, "Neither the name of Google LLC"); n != 1 {
		t.Errorf("BSD metni %d kez yazıldı, 1 beklenirdi", n)
	}
	if !strings.Contains(metin, "go go1.26.9\ngolang.org/x/sys v0.48.0\n") {
		t.Errorf("aynı metni taşıyan modüller tek başlık altında listelenmeli:\n%s", metin)
	}
	if !strings.Contains(metin, "Note: No license file in the module.") {
		t.Error("elle doğrulanan modülün notu yazılmalı")
	}
	if strings.Contains(metin, "\r") {
		t.Error("bildirim yalnızca LF satır sonu içermeli")
	}
	if !strings.HasSuffix(metin, "\n") || strings.HasSuffix(metin, "\n\n") {
		t.Error("bildirim tek bir satır sonuyla bitmeli")
	}
}

func TestGoModGereksinimleri(t *testing.T) {
	gomod := `module github.com/ornek/and

go 1.26.0

toolchain go1.26.9

require (
	github.com/charmbracelet/bubbletea v1.3.10
	golang.org/x/sys v0.48.0 // indirect
)

require github.com/tek/satir v0.1.0

// require github.com/yorum/satiri v9.9.9
`
	gereksinimler, toolchain, err := goModGereksinimleri([]byte(strings.ReplaceAll(gomod, "\n", "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	want := []surumluModul{
		{"github.com/charmbracelet/bubbletea", "v1.3.10"},
		{"github.com/tek/satir", "v0.1.0"},
		{"golang.org/x/sys", "v0.48.0"},
	}
	if !reflect.DeepEqual(gereksinimler, want) {
		t.Errorf("gereksinimler %v, beklenen %v", gereksinimler, want)
	}
	if toolchain != "go1.26.9" {
		t.Errorf("toolchain %q, beklenen go1.26.9", toolchain)
	}
}

func TestModullerGoKaynagi(t *testing.T) {
	src, err := modullerGoKaynagi([]modul{
		{Yol: "go", Surum: "go1.26.9", Dosyalar: []lisansDosyasi{{Ad: "LICENSE", Metin: bsd3Ornek}}},
		{Yol: "github.com/ornek/cift", Surum: "v1.0.0", Dosyalar: []lisansDosyasi{{Ad: "LICENSE-MIT", Metin: mitOrnek}, {Ad: "LICENSE-APACHE", Metin: apacheOrnek}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, parca := range []string{
		"// Code generated",
		"package lisans",
		`{Yol: "go", Surum: "go1.26.9", Lisans: "BSD-3-Clause"}`,
		`{Yol: "github.com/ornek/cift", Surum: "v1.0.0", Lisans: "Apache-2.0, MIT"}`,
	} {
		if !strings.Contains(s, parca) {
			t.Errorf("üretilen kaynak %q içermiyor:\n%s", parca, s)
		}
	}
}
