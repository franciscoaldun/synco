package dynamo

import (
	"math"
	"testing"
)

func TestModelosIncluidos(t *testing.T) {
	for _, f := range Modelos() {
		m, err := Compilar(f.Texto)
		if err != nil {
			t.Fatalf("%s: %v", f.ID, err)
		}
		r, err := m.Correr(nil, 1)
		if err != nil {
			t.Fatalf("%s: %v", f.ID, err)
		}
		t.Logf("=== %s (%d pasos)\n%s\n%s", f.ID, r.Pasos, r.TextoPrint, r.TextoPlot)
	}
}

// La relación (DT)(X) es una multiplicación y el nivel integra con Euler.
func TestNivelSimple(t *testing.T) {
	m, err := Compilar("* PRUEBA\nL X.K=X.J+(DT)(R.JK)\nN X=0\nR R.KL=2\nSPEC DT=0.5/LENGTH=4/PRTPER=1/PLTPER=1\nPLOT X=X")
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.Correr(nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	x := r.Series["X"]
	if math.Abs(x[len(x)-1]-8) > 1e-9 {
		t.Errorf("X(4) = %v, esperaba 8", x[len(x)-1])
	}
}

func TestErrores(t *testing.T) {
	casos := []string{
		"L X.K=X.J+(DT)(R.JK)\nR R.KL=1",           // falta N
		"A Y.K=Z.K+1\nSPEC DT=1/LENGTH=2",            // Z no definida
		"A Y.K=W.K\nA W.K=Y.K",                      // circular
		"Q X=1",                                     // tipo desconocido
	}
	for _, c := range casos {
		m, err := Compilar(c)
		if err == nil {
			_, err = m.Correr(nil, 1)
		}
		if err == nil {
			t.Errorf("debería fallar: %q", c)
		} else {
			t.Logf("ok: %v", err)
		}
	}
}
