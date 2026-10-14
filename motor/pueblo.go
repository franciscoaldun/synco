package motor

// Cyberfolk: el medidor algedónico del pueblo (Beer, "Project Cyberfolk",
// marzo de 1972; Brain of the Firm, 2ª ed., p. 283). Quien sostiene el medidor
// mueve la aguja en una escala continua entre "total intranquilidad" y "total
// satisfacción", sin tener que explicar nada. Los medidores de un local se
// suman eléctricamente y un medidor en el estudio de televisión muestra el
// total mientras se anuncia una política. Nunca se construyó: aquí sí.

import (
	"sort"
	"time"
)

type Medidor struct {
	ID    string    `json:"id"`
	Valor float64   `json:"valor"` // -1 intranquilidad total, +1 satisfacción total
	visto time.Time
	orden int
}

type PuntoPueblo struct {
	T    int64   `json:"t"`
	Prom float64 `json:"prom"`
	N    int     `json:"n"`
}

type Pueblo struct {
	medidores map[string]*Medidor
	orden     int
	Politica  string
	desde     time.Time
	Serie     []PuntoPueblo
	ultimo    time.Time
	pendiente bool
	ultimoN   int
}

type VistaPueblo struct {
	Politica string        `json:"politica"`
	Desde    int64         `json:"desde"`
	N        int           `json:"n"`
	Prom     float64       `json:"prom"`
	Hist     [11]int       `json:"hist"`
	Locales  []float64     `json:"locales"` // un medidor cada tres personas, sumado por local
	Serie    []PuntoPueblo `json:"serie"`
}

func NuevoPueblo() *Pueblo { return &Pueblo{medidores: map[string]*Medidor{}} }

func (p *Pueblo) activos() []*Medidor {
	var xs []*Medidor
	for _, x := range p.medidores {
		if time.Since(x.visto) < 20*time.Second {
			xs = append(xs, x)
		}
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i].orden < xs[j].orden })
	return xs
}

func (p *Pueblo) Vista() VistaPueblo {
	v := VistaPueblo{Politica: p.Politica, Serie: p.Serie}
	if !p.desde.IsZero() {
		v.Desde = p.desde.UnixMilli()
	}
	xs := p.activos()
	v.N = len(xs)
	var suma float64
	var local float64
	enLocal := 0
	for _, x := range xs {
		suma += x.Valor
		b := int((x.Valor + 1) / 2 * 10)
		if b < 0 {
			b = 0
		}
		if b > 10 {
			b = 10
		}
		v.Hist[b]++
		local += x.Valor
		enLocal++
		if enLocal == 3 {
			v.Locales = append(v.Locales, local/3)
			local, enLocal = 0, 0
		}
	}
	if enLocal > 0 {
		v.Locales = append(v.Locales, local/float64(enLocal))
	}
	if v.N > 0 {
		v.Prom = suma / float64(v.N)
	}
	return v
}

// Mover registra la posición de la aguja de un medidor.
func (m *Motor) MoverMedidor(id string, valor float64) {
	m.mu.Lock()
	defer m.flush()
	p := m.Pueblo
	if valor < -1 {
		valor = -1
	}
	if valor > 1 {
		valor = 1
	}
	x := p.medidores[id]
	if x == nil {
		p.orden++
		x = &Medidor{ID: id, orden: p.orden}
		p.medidores[id] = x
	}
	x.Valor = valor
	x.visto = time.Now()
	m.muestrearPueblo(false)
}

func (m *Motor) muestrearPueblo(forzar bool) {
	p := m.Pueblo
	if !forzar && time.Since(p.ultimo) < 400*time.Millisecond {
		p.pendiente = true // lo despacha el reloj en la próxima vuelta
		return
	}
	p.pendiente = false
	p.ultimo = time.Now()
	v := p.Vista()
	p.ultimoN = v.N
	if v.N > 0 {
		p.Serie = append(p.Serie, PuntoPueblo{time.Now().UnixMilli(), v.Prom, v.N})
		if len(p.Serie) > 900 {
			p.Serie = p.Serie[len(p.Serie)-900:]
		}
	}
	v.Serie = nil
	m.emitir("pueblo", v)
}

// AnunciarPolitica: el estudio de televisión anuncia una medida.
func (m *Motor) AnunciarPolitica(texto string) {
	m.mu.Lock()
	defer m.flush()
	m.Pueblo.Politica = texto
	m.Pueblo.desde = time.Now()
	m.Pueblo.Serie = nil
	m.log("Se anuncia por televisión: «%s»", texto)
	m.emitir("politica", map[string]any{"politica": texto, "desde": m.Pueblo.desde.UnixMilli()})
	m.muestrearPueblo(true)
}

func (m *Motor) SeriePueblo() []PuntoPueblo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]PuntoPueblo(nil), m.Pueblo.Serie...)
}

// revisarCaducados avisa cuando un medidor dejó de responder (el celular se apagó).
func (m *Motor) revisarCaducados() {
	p := m.Pueblo
	if len(p.medidores) == 0 || time.Since(p.ultimo) < 2*time.Second {
		return
	}
	if len(p.activos()) != p.ultimoN {
		m.muestrearPueblo(true)
	}
}
