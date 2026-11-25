package main

import (
	"testing"

	"github.com/lucian95511/and/dil/diltest"
)

func TestCeviriKapsami(t *testing.T) {
	diltest.Kapsam(t, "ceviri.json", ceviriVerisi, diltest.Tara(t, "."))
}
