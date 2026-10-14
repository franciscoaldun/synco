package motor

import (
	"math/rand"
	"strings"
	"testing"
)

// Los valores del listado de la suite temporal (ECOM, 21-01-1972) tienen que
// salir idénticos al pasar por el punto flotante hexadecimal del 360.
func TestHFPListadoSuiteTemporal(t *testing.T) {
	casos := []struct {
		tecleado float64
		impreso  string
	}{
		{1225.0, "1225.00000"},
		{25.4, "25.39999"},
		{479, "479.00000"},
		{11656.9, "11656.89844"},
		{14918.03, "14918.02734"},
	}
	for _, c := range casos {
		got := strings.TrimSpace(FormatoF(c.tecleado, 12, 5))
		if got != c.impreso {
			t.Errorf("%v: impreso %q, el listado de 1972 dice %q (palabra %s)", c.tecleado, got, c.impreso, HexHFP(c.tecleado))
		}
	}
	if HexHFP(25.4) != "42196666" {
		t.Errorf("25.4 en HFP = %s", HexHFP(25.4))
	}
}

// La cinta de la tabla 2 de Gómez-Venegas (télex de ECOM, abril 1973).
func TestITA2CintaECOM(t *testing.T) {
	tr := CodificarITA2("GA\n40750\n40506 ECOM CL")
	var bits []string
	for _, x := range tr {
		bits = append(bits, x.Bits)
	}
	esperado := []string{
		"11111", "01011", "11000", "00010", "01000", // LTRS G A CR LF
		"11011", "01010", "01101", "11100", "00001", "01101", "00010", "01000", // FIGS 4 0 7 5 0 CR LF
		"01010", "01101", "00001", "01101", "10101", "00100", // 4 0 5 0 6 espacio
		"11111", "10000", "01110", "00011", "00111", "00100", "01110", "01001", // LTRS E C O M esp C L
	}
	if strings.Join(bits, " ") != strings.Join(esperado, " ") {
		t.Errorf("cinta distinta:\n got %v\nwant %v", bits, esperado)
	}
	if DecodificarITA2(tr) != "GA\n40750\n40506 ECOM CL" {
		t.Errorf("ida y vuelta: %q", DecodificarITA2(tr))
	}
}

func TestNormalizarTelex(t *testing.T) {
	if got := NormalizarTelex("Señales algedónicas: Caña 3,5%"); got != "SENALES ALGEDONICAS: CANA 3,5" {
		t.Errorf("%q", got)
	}
}

// El filtro tiene que reconocer los cuatro tipos de cambio del gráfico de
// Harrison y Stevens: transitorios, escalón y cambio de pendiente.
func TestMSMReconoceCambios(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	sd := 0.02
	m := NuevoMSM(0.7, sd*sd)
	banda := Banda{0.6, 0.7, 0.8}
	nivel, pend := 0.7, 0.0
	racha := 0
	var transitorio, escalon, pendiente, falsas int
	for k := 0; k < 140; k++ {
		if k == 80 {
			nivel -= 0.15 // escalón hacia abajo
		}
		if k == 110 {
			pend = -0.01 // empieza a caer
		}
		nivel += pend
		d := nivel + rng.NormFloat64()*sd
		if k == 50 {
			d = nivel + 0.25 // dato aislado
		}
		m.Actualizar(d)
		c := Clasificar(m, d, banda, 0.7, &racha)
		switch {
		case c.TransAyer && k == 51:
			transitorio++
		case c.EscalonAyer && k == 81:
			escalon++
		case c.Pendiente && k > 110 && k < 122:
			pendiente++
		case k < 48 && (c.TransAyer || c.EscalonAyer || c.Pendiente):
			falsas++
		}
	}
	if transitorio == 0 {
		t.Error("no reconoció el transitorio")
	}
	if escalon == 0 {
		t.Error("no reconoció el escalón")
	}
	if pendiente == 0 {
		t.Error("no reconoció el cambio de pendiente a tiempo")
	}
	if falsas > 1 {
		t.Errorf("%d falsas alarmas en el tramo tranquilo", falsas)
	}
}

func TestIndicesPRO20(t *testing.T) {
	p, l, perf := Indices(900, 1000, 1200, false)
	// P = asin(0.9)/(pi/2), L = asin(1000/1200)/(pi/2)
	if p < 0.712 || p > 0.714 || l < 0.626 || l > 0.628 || perf < 0.53 || perf > 0.54 {
		t.Errorf("P=%v L=%v PERF=%v", p, l, perf)
	}
	// índice al revés (ausentismo): más es peor
	p2, _, _ := Indices(500, 400, 300, true)
	if p2 >= 1 || p2 < 0.5 {
		t.Errorf("ausentismo P=%v", p2)
	}
}
