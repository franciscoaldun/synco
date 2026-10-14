package motor

// La Sala de Operaciones (Opsroom), diseñada por el Grupo de Diseño Industrial
// de INTEC bajo Gui Bonsiepe (1972-73): sala hexagonal, siete sillones
// giratorios de fibra de vidrio sin mesas ni papel, y en las paredes el
// Datafeed (una pantalla índice grande sobre tres pantallas de datos), dos
// pantallas algedónicas, el modelo de sistema viable ("Staffy"), el Panel del
// Futuro y una pared vacía reservada para el medidor algedónico.
//
// En cada brazo del sillón había botones geométricos grandes ("big hand"),
// pensados para usarse sin saber teclear, y un botón de retención (HOLD) que le
// daba a esa persona el control de las pantallas hasta soltarlo. La pantalla
// índice mostraba qué combinación de botones lleva a cada diapositiva.

import (
	"fmt"
	"time"
)

var Paredes = []string{"datafeed", "algedonica", "staffy", "futuro", "pueblo"}

var NombrePared = map[string]string{
	"datafeed":   "Datafeed",
	"algedonica": "Pantallas algedónicas",
	"staffy":     "Sistema viable (Staffy)",
	"futuro":     "Panel del Futuro",
	"pueblo":     "Medidor algedónico del pueblo",
}

// Botones geométricos del brazo del sillón.
var Botones = []string{"circulo", "cuadrado", "triangulo", "rombo", "cruz"}

type Silla struct {
	N            int    `json:"n"`
	Ocupada      bool   `json:"ocupada"`
	Participante string `json:"p"`
	Nombre       string `json:"nombre"`
	visto        time.Time
}

type OpcionMenu struct {
	Combo []int  `json:"combo"`
	Texto string `json:"texto"`
	Tipo  string `json:"tipo"` // sector | fabrica | indice | volver | pared
	Arg   string `json:"arg"`
}

type Seleccion struct {
	Nivel   string `json:"nivel"` // total | sector | fabrica | indice
	Sector  string `json:"sector"`
	Fabrica string `json:"fabrica"`
	Codigo  string `json:"codigo"`
}

type Sala struct {
	Pared       string       `json:"pared"`
	Sel         Seleccion    `json:"sel"`
	Sillas      [7]Silla     `json:"sillas"`
	Hold        int          `json:"hold"` // silla con el control (-1 nadie)
	Menu        []OpcionMenu `json:"menu"`
	Carrusel    int          `json:"carrusel"` // posición del carrusel de diapositivas
	Ultimo      string       `json:"ultimo"`   // último cambio
	SenalFoco   int          `json:"senalFoco"`
}

func NuevaSala() *Sala {
	s := &Sala{}
	s.Reiniciar()
	return s
}

func (s *Sala) Reiniciar() {
	s.Pared = "datafeed"
	s.Sel = Seleccion{Nivel: "total"}
	s.Hold = -1
	for i := range s.Sillas {
		s.Sillas[i].N = i + 1
	}
	s.Menu = nil
	s.Carrusel = 0
	s.SenalFoco = 0
}

func (s *Sala) HayPersonas() bool {
	for _, x := range s.Sillas {
		if x.Ocupada && time.Since(x.visto) < 2*time.Minute {
			return true
		}
	}
	return false
}

// combinaciones: primero botones solos, luego pares, luego tríos.
func combinaciones() [][]int {
	var out [][]int
	for i := 0; i < 5; i++ {
		out = append(out, []int{i})
	}
	for i := 0; i < 5; i++ {
		for j := i + 1; j < 5; j++ {
			out = append(out, []int{i, j})
		}
	}
	for i := 0; i < 5; i++ {
		for j := i + 1; j < 5; j++ {
			for k := j + 1; k < 5; k++ {
				out = append(out, []int{i, j, k})
			}
		}
	}
	return out
}

var todasCombinaciones = combinaciones()

// armarMenu construye la pantalla índice del Datafeed para la selección actual.
func (m *Motor) armarMenu() {
	s := m.Sala
	var ops []OpcionMenu
	add := func(texto, tipo, arg string) {
		ops = append(ops, OpcionMenu{Texto: texto, Tipo: tipo, Arg: arg})
	}
	switch s.Sel.Nivel {
	case "total":
		for _, sec := range Sectores {
			if m.tieneSector(sec.ID) {
				add(sec.Nombre, "sector", sec.ID)
			}
		}
	case "sector":
		for _, f := range m.Fabricas {
			if f.Sector == s.Sel.Sector {
				add(f.Corto, "fabrica", f.ID)
			}
		}
		add("Industria total", "volver", "total")
	case "fabrica":
		if f := m.porID[s.Sel.Fabrica]; f != nil {
			for _, ix := range f.Indices {
				add(ix.Abrev, "indice", ix.Codigo)
			}
		}
		add("Volver al sector", "volver", "sector")
	case "indice":
		if ix := m.porCodigo[s.Sel.Codigo]; ix != nil {
			for _, o := range m.porID[ix.Fabrica].Indices {
				if o.Codigo != ix.Codigo {
					add(o.Abrev, "indice", o.Codigo)
				}
			}
		}
		add("Volver a la empresa", "volver", "fabrica")
	}
	for _, p := range Paredes {
		if p != s.Pared {
			add("Pared: "+NombrePared[p], "pared", p)
		}
	}
	for i := range ops {
		if i < len(todasCombinaciones) {
			ops[i].Combo = todasCombinaciones[i]
		}
	}
	s.Menu = ops
}

func (m *Motor) tieneSector(id string) bool {
	for _, f := range m.Fabricas {
		if f.Sector == id {
			return true
		}
	}
	return false
}

func igualCombo(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	ma := map[int]bool{}
	for _, x := range a {
		ma[x] = true
	}
	for _, x := range b {
		if !ma[x] {
			return false
		}
	}
	return true
}

func (m *Motor) aplicarOpcion(o OpcionMenu) {
	s := m.Sala
	switch o.Tipo {
	case "sector":
		s.Sel = Seleccion{Nivel: "sector", Sector: o.Arg}
	case "fabrica":
		f := m.porID[o.Arg]
		s.Sel = Seleccion{Nivel: "fabrica", Sector: f.Sector, Fabrica: f.ID}
	case "indice":
		ix := m.porCodigo[o.Arg]
		s.Sel = Seleccion{Nivel: "indice", Sector: ix.Sector, Fabrica: ix.Fabrica, Codigo: ix.Codigo}
	case "volver":
		switch o.Arg {
		case "total":
			s.Sel = Seleccion{Nivel: "total"}
		case "sector":
			s.Sel = Seleccion{Nivel: "sector", Sector: s.Sel.Sector}
		case "fabrica":
			s.Sel = Seleccion{Nivel: "fabrica", Sector: s.Sel.Sector, Fabrica: s.Sel.Fabrica}
		}
	case "pared":
		s.Pared = o.Arg
	}
	s.Carrusel++
}

// ---------- entradas públicas ----------

func (m *Motor) emitirSala() {
	m.armarMenu()
	m.emitir("sala", m.Sala)
}

func (m *Motor) Sentarse(n int, participante, nombre string) error {
	m.mu.Lock()
	defer m.flush()
	if n < 1 || n > 7 {
		return fmt.Errorf("la sala tiene siete sillones")
	}
	s := &m.Sala.Sillas[n-1]
	if s.Ocupada && s.Participante != participante && time.Since(s.visto) < 2*time.Minute {
		return fmt.Errorf("el sillón %d está ocupado por %s", n, s.Nombre)
	}
	for i := range m.Sala.Sillas {
		if m.Sala.Sillas[i].Participante == participante && i != n-1 {
			m.Sala.Sillas[i] = Silla{N: i + 1}
			if m.Sala.Hold == i {
				m.Sala.Hold = -1
			}
		}
	}
	*s = Silla{N: n, Ocupada: true, Participante: participante, Nombre: nombre, visto: time.Now()}
	m.log("%s se sienta en el sillón %d de la Sala", nombre, n)
	m.emitirSala()
	return nil
}

func (m *Motor) Levantarse(participante string) {
	m.mu.Lock()
	defer m.flush()
	for i := range m.Sala.Sillas {
		if m.Sala.Sillas[i].Participante == participante {
			m.Sala.Sillas[i] = Silla{N: i + 1}
			if m.Sala.Hold == i {
				m.Sala.Hold = -1
			}
		}
	}
	m.emitirSala()
}

// Latido mantiene viva la ocupación de un sillón.
func (m *Motor) LatidoSilla(participante string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Sala.Sillas {
		if m.Sala.Sillas[i].Participante == participante {
			m.Sala.Sillas[i].visto = time.Now()
		}
	}
}

func (m *Motor) BotonHold(n int, participante string, activo bool) error {
	m.mu.Lock()
	defer m.flush()
	if n < 1 || n > 7 || m.Sala.Sillas[n-1].Participante != participante {
		return fmt.Errorf("no está sentado en ese sillón")
	}
	if activo {
		if m.Sala.Hold >= 0 && m.Sala.Hold != n-1 {
			return fmt.Errorf("el sillón %d tiene el control", m.Sala.Hold+1)
		}
		m.Sala.Hold = n - 1
	} else if m.Sala.Hold == n-1 {
		m.Sala.Hold = -1
	}
	m.emitirSala()
	return nil
}

// Combinacion: alguien apretó botones en su sillón (n=0 es el docente).
func (m *Motor) Combinacion(n int, participante string, combo []int) error {
	m.mu.Lock()
	defer m.flush()
	if n > 0 {
		if n > 7 || m.Sala.Sillas[n-1].Participante != participante {
			return fmt.Errorf("no está sentado en ese sillón")
		}
		if m.Sala.Hold >= 0 && m.Sala.Hold != n-1 {
			return fmt.Errorf("el sillón %d tiene el control (HOLD)", m.Sala.Hold+1)
		}
	}
	m.armarMenu()
	for _, o := range m.Sala.Menu {
		if igualCombo(o.Combo, combo) {
			m.aplicarOpcion(o)
			quien := "el docente"
			if n > 0 {
				quien = fmt.Sprintf("sillón %d (%s)", n, m.Sala.Sillas[n-1].Nombre)
			}
			m.Sala.Ultimo = fmt.Sprintf("%s → %s", quien, o.Texto)
			m.emitirSala()
			return nil
		}
	}
	return fmt.Errorf("esa combinación no lleva a ninguna diapositiva")
}

// Seleccionar lo usa el proyector o el docente para ir directo a algo.
func (m *Motor) Seleccionar(pared string, sel Seleccion, senal int) {
	m.mu.Lock()
	defer m.flush()
	if pared != "" {
		m.Sala.Pared = pared
	}
	if sel.Nivel != "" {
		m.Sala.Sel = sel
	}
	if senal >= 0 {
		m.Sala.SenalFoco = senal
	}
	m.Sala.Carrusel++
	m.emitirSala()
}

func (m *Motor) SalaActual() *Sala {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.armarMenu()
	return m.Sala
}
