package motor

import (
	"strings"
	"testing"
	"time"
)

func correrDias(t *testing.T, m *Motor, dias int) {
	t.Helper()
	limite := time.Now().Add(20 * time.Second)
	for m.dia < dias || fasesDia[m.fase].Nombre != "noche" {
		m.Avanzar(80 * time.Millisecond)
		time.Sleep(time.Millisecond)
		if time.Now().After(limite) {
			t.Fatalf("la jornada se quedó pegada en el día %d, fase %s, esperando %v", m.dia, fasesDia[m.fase].Nombre, m.esperando)
		}
	}
}

func motorDePrueba(esc string) *Motor {
	cfg := ConfigPorDefecto()
	cfg.Velocidad = 400
	cfg.VelTelex = 100000
	cfg.Turbulencia = 0.25
	cfg.ErroresDigitacion = 0.05
	return Nuevo(cfg, esc, 42)
}

func TestJornadasCompletas(t *testing.T) {
	m := motorDePrueba("abril1973")
	correrDias(t, m, 12)
	if m.Ecom.Ultima == nil || m.Ecom.Ultima.Dia != 12 {
		t.Fatalf("ECOM no corrió el día 12")
	}
	if m.Ecom.Ultima.CCValid != 0 || !m.Ecom.Ultima.Stats {
		t.Errorf("con revisión de CORFO, PRO10 debería terminar en 0: %v", m.Ecom.Ultima.Mensajes)
	}
	if got := len(m.Ecom.Ultima.Resultados); got != len(m.porCodigo) {
		t.Errorf("PRO20 procesó %d de %d índices", got, len(m.porCodigo))
	}
	var tipos = map[string]int{}
	for _, tx := range m.Telex {
		tipos[tx.Tipo]++
	}
	if tipos["indices"] < 12*len(m.Fabricas) || tipos["reenvio"] < 12 {
		t.Errorf("faltan télex: %v", tipos)
	}
	niveles := map[int]int{}
	for _, s := range m.Senales {
		niveles[s.Nivel]++
	}
	if len(m.Senales) == 0 {
		t.Errorf("con turbulencia alta debería haber señales")
	}
	t.Logf("télex: %v · señales por nivel: %v · eventos: %d", tipos, niveles, len(m.Eventos))
	for _, l := range m.Ecom.Ultima.Log {
		if strings.Contains(l, "IEF142I") {
			t.Log(l)
		}
	}
	for _, li := range m.Ecom.Ultima.Listados {
		if strings.HasPrefix(li.Titulo, "Informe") {
			for _, x := range li.Lineas {
				t.Log(x)
			}
		}
	}
	for _, tx := range m.Telex {
		if tx.Tipo == "algedonico" && tx.A == TelexCORFO {
			t.Log("\n" + tx.Texto)
			break
		}
	}
}

// Sin el control de consistencia de CORFO, un código mal tecleado llega a
// ECOM, PRO10 devuelve código 8 y COND=(7,LT) se salta el paso STATS.
func TestSinControlCORFOElErrorDetieneTodo(t *testing.T) {
	m := motorDePrueba("marzo1972")
	m.Cfg.RevisaCORFO = false
	correrDias(t, m, 2)
	f := m.porID["FABRILAN"]
	f.Humano = "equipo-1"
	// avanzar hasta la transmisión del día 3
	for !(m.dia == 3 && fasesDia[m.fase].Nombre == "transmision") {
		m.Avanzar(80 * time.Millisecond)
		time.Sleep(time.Millisecond)
	}
	texto := "TEXFABRILANAUSENTISMO " + m.fecha.Format("020106") + " 479\n" + // 21 letras
		"TEXFABRILANEFICHILCARD " + m.fecha.Format("020106") + " 80.5\n" +
		"TEXFABRILANPRODHILADOS " + m.fecha.Format("020106") + " 4300\n"
	if _, err := m.EnviarTelexHumano("FABRILAN", TelexCORFO, texto); err != nil {
		t.Fatal(err)
	}
	correrDias(t, m, 3)
	c := m.Ecom.Ultima
	if c.CCValid != 8 || c.Stats {
		t.Fatalf("esperaba código 8 y STATS sin ejecutar; cc=%d stats=%v mensajes=%v", c.CCValid, c.Stats, c.Mensajes)
	}
	t.Log(strings.Join(c.Mensajes, "\n"))
}

// El paro de octubre: el transporte cae en casi todas las fábricas y los
// camiones leales alcanzan para pocas intervenciones.
func TestParoDeOctubre(t *testing.T) {
	m := motorDePrueba("octubre1972")
	m.Cfg.Turbulencia = 0
	m.Cfg.Autonomia = 0 // nadie corrige solo: todo depende de los camiones
	correrDias(t, m, 4)
	paro := 0
	for _, ev := range m.Eventos {
		if ev.Causa == "transporte" {
			paro++
		}
	}
	if paro < len(m.Fabricas)-1 {
		t.Errorf("el paro alcanzó a %d de %d fábricas", paro, len(m.Fabricas))
	}
	m.mu.Lock()
	var activas []*Senal
	for _, s := range m.Senales {
		if s.Estado == "activa" && s.Esperando == "" {
			activas = append(activas, s)
		}
	}
	m.mu.Unlock()
	if len(activas) < 5 {
		t.Fatalf("esperaba varias señales por el paro, hay %d", len(activas))
	}
	ok, sinCamiones := 0, 0
	for _, s := range activas {
		if err := m.Decidir(s.ID, "transporte", "SALA DE OPERACIONES", ""); err != nil {
			sinCamiones++
		} else {
			ok++
		}
	}
	if ok != 8 || m.Camiones != 0 || sinCamiones == 0 {
		t.Errorf("con 200 camiones caben 8 envíos de 25: ok=%d sin camiones=%d quedan=%d", ok, sinCamiones, m.Camiones)
	}
	correrDias(t, m, 9)
	if m.Camiones != 200 {
		t.Errorf("los camiones deberían haber vuelto: hay %d", m.Camiones)
	}
	t.Logf("fábricas con paro: %d · señales: %d · envíos: %d · rechazados por falta de camiones: %d", paro, len(activas), ok, sinCamiones)
}
