package motor

// El motor: reloj de la jornada, escenarios y estado compartido de la sesión.
//
// Una jornada de Cybersyn (días hábiles):
//   08:00-15:00  producción en las fábricas; los interventores actúan sobre las señales
//   15:00-17:00  cada fábrica transmite sus índices por télex a CORFO (40750)
//   17:00-18:00  la central de tráfico de CORFO revisa consistencia, archiva y
//                reenvía todo por télex a ECOM (40506)
//   18:00-18:30  ECOM: cinta → tarjetas → JOB CYBER en el IBM 360/50 (PRASMY01, PRO10, PRO20)
//   18:30-20:00  informes algedónico-taxonómicos por télex; escalamiento
//   20:00-08:00  noche

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"
)

type Config struct {
	SegundosPorDia    float64 `json:"segundosPorDia"`    // duración real de una jornada a velocidad 1
	Velocidad         float64 `json:"velocidad"`
	VelTelex          float64 `json:"velTelex"`          // 1 = 50 baudios reales
	EsperarHumanos    bool    `json:"esperarHumanos"`    // detener el reloj hasta que transmitan los equipos
	RevisaCORFO       bool    `json:"revisaCORFO"`       // control de consistencia en la central de tráfico
	VentanaDias       int     `json:"ventanaDias"`       // días en un nivel antes de escalar
	Autonomia         float64 `json:"autonomia"`         // probabilidad diaria de que un nivel automático resuelva
	Turbulencia       float64 `json:"turbulencia"`       // probabilidad diaria de un imprevisto por fábrica
	ErroresDigitacion float64 `json:"erroresDigitacion"` // probabilidad de error al teclear en las fábricas automáticas
	SalaAutomatica    bool    `json:"salaAutomatica"`    // si nadie decide en la Sala, decide el motor
	EsperaMaxima      float64 `json:"esperaMaxima"`      // segundos reales que se espera a un equipo antes de seguir
}

func ConfigPorDefecto() Config {
	return Config{
		SegundosPorDia:    75,
		Velocidad:         1,
		VelTelex:          10,
		EsperarHumanos:    true,
		RevisaCORFO:       true,
		VentanaDias:       2,
		Autonomia:         0.45,
		Turbulencia:       0.05,
		ErroresDigitacion: 0.02,
		SalaAutomatica:    false,
		EsperaMaxima:      120,
	}
}

type faseDef struct {
	Nombre string
	Titulo string
	Desde  int // minutos desde medianoche
	Hasta  int
	Peso   float64
}

var fasesDia = []faseDef{
	{"produccion", "Producción en las fábricas", 8 * 60, 15 * 60, 0.27},
	{"transmision", "Transmisión de índices por télex", 15 * 60, 17 * 60, 0.30},
	{"corfo", "Central de tráfico de CORFO", 17 * 60, 18 * 60, 0.09},
	{"ecom", "Proceso Cyberstride en ECOM", 18 * 60, 18*60 + 30, 0.13},
	{"informes", "Informes algedónicos y escalamiento", 18*60 + 30, 20 * 60, 0.13},
	{"noche", "Noche", 20 * 60, 32 * 60, 0.08},
}

// Escenario de partida.
type Escenario struct {
	ID          string `json:"id"`
	Titulo      string `json:"titulo"`
	Descripcion string `json:"descripcion"`
	Inicio      string `json:"inicio"` // AAAA-MM-DD, primer día hábil
	Desde       string `json:"-"`
	Camiones    int    `json:"camiones"`
}

var Escenarios = []Escenario{
	{"abril1973", "Abril de 1973 · suite permanente",
		"Cyberstride procesa a diario los índices de 16 empresas del Área Social. Es la semana del télex real de ECOM a la Sala de Operaciones (3 al 6 de abril).",
		"1973-04-02", "1973", 0},
	{"marzo1972", "Marzo de 1972 · suite temporal",
		"Las cinco primeras fábricas conectadas: Cemento Melón, Inforsa, Fabrilana, Textil Progreso y Said Maipú. Alan Dunsmuir instala en ECOM la suite temporal.",
		"1972-03-20", "1972", 0},
	{"octubre1972", "Octubre de 1972 · el paro de los camioneros",
		"Cuarenta mil dueños de camiones paran el país. El gobierno cuenta con unos 200 camiones leales y la red de télex para coordinarlos.",
		"1972-10-11", "1973", 200},
}

type Reloj struct {
	Dia       int      `json:"dia"`
	Fecha     string   `json:"fecha"`     // AAAA-MM-DD
	FechaCorta string  `json:"fechaCorta"` // DDMMAA, como en los formularios
	Juliana   string   `json:"juliana"`   // AADDD, como en los mensajes del OS/360
	Hora      string   `json:"hora"`
	Minuto    int      `json:"minuto"`
	Fase      string   `json:"fase"`
	TituloFase string  `json:"tituloFase"`
	Progreso  float64  `json:"progreso"`
	Pausa     bool     `json:"pausa"`
	Esperando []string `json:"esperando"`
}

type Evt struct {
	T string `json:"t"`
	D any    `json:"d"`
}

type Motor struct {
	mu   sync.Mutex
	Cfg  Config
	Esc  Escenario
	rng  *rand.Rand

	fecha    time.Time
	dia      int
	fase     int
	progreso float64
	pausa    bool
	esperando []string
	esperaDesde time.Time

	Fabricas  []*Fabrica
	porID     map[string]*Fabrica
	porCodigo map[string]*Indice

	Eventos   []*Evento
	sigEvento int
	Senales   []*Senal
	sigSenal  int

	Telex      []*Mensaje
	sigTelex   int
	recibidos  map[string][]*Mensaje // telex de índices recibidos hoy por CORFO, por fábrica
	porEnviar  map[string]float64    // progreso de la fase en que transmite cada fábrica automática
	planificadas map[string]bool
	corregir   map[string]bool       // fábricas a las que CORFO pidió corrección
	reenvio    []string              // transacciones válidas que CORFO mandó a ECOM hoy
	reenviado  bool

	Ecom   *ECOM
	Sala   *Sala
	Pueblo *Pueblo

	Camiones      int
	camionesFuera []int // día en que vuelve cada grupo

	Bitacora []string

	salida []Evt
	Emitir func([]Evt)
	ultimoReloj time.Time
}

func Nuevo(cfg Config, escenario string, semilla int64) *Motor {
	m := &Motor{Cfg: cfg}
	m.Reiniciar(escenario, semilla)
	return m
}

// Reiniciar arma una sesión nueva con el escenario elegido.
func (m *Motor) Reiniciar(escenario string, semilla int64) {
	if semilla == 0 {
		semilla = time.Now().UnixNano()
	}
	esc := Escenarios[0]
	for _, e := range Escenarios {
		if e.ID == escenario {
			esc = e
		}
	}
	emitir := m.Emitir
	cfg := m.Cfg
	sala, pueblo := m.Sala, m.Pueblo
	*m = Motor{Cfg: cfg, Esc: esc, Emitir: emitir}
	m.rng = rand.New(rand.NewSource(semilla))
	m.porID = map[string]*Fabrica{}
	m.porCodigo = map[string]*Indice{}
	m.recibidos = map[string][]*Mensaje{}
	m.porEnviar = map[string]float64{}
	m.corregir = map[string]bool{}
	m.Camiones = esc.Camiones
	m.fecha, _ = time.Parse("2006-01-02", esc.Inicio)
	m.Ecom = NuevoECOM()
	if sala == nil {
		sala = NuevaSala()
	}
	sala.Reiniciar()
	m.Sala = sala
	if pueblo == nil {
		pueblo = NuevoPueblo()
	}
	m.Pueblo = pueblo

	for _, df := range CatalogoFabricas {
		if esc.Desde == "1972" && df.Desde != "1972" {
			continue
		}
		f := &Fabrica{
			ID: df.ID, Sector: df.Sector, Nombre: df.Nombre, Corto: df.Corto, Lugar: df.Lugar,
			Lat: df.Lat, Central: df.Central, Telex: df.Telex,
			Respuesta: df.Telex + " " + abreviarRespuesta(df.ID) + " CL",
		}
		for _, di := range df.Indices {
			ix := &Indice{DefIndice: di, Fabrica: f.ID, Sector: f.Sector}
			for k := range ix.Estacional {
				ix.Estacional[k] = 1
			}
			if f.Sector == "FOR" || f.Sector == "CIM" {
				// la construcción y la madera bajan en el invierno austral
				for k := 0; k < 12; k++ {
					ix.Estacional[k] = 1 + 0.03*math.Cos(2*math.Pi*float64(k)/12)
				}
				if ix.Inverso {
					for k := 0; k < 12; k++ {
						ix.Estacional[k] = 1
					}
				}
			}
			f.Indices = append(f.Indices, ix)
			m.porCodigo[ix.Codigo] = ix
		}
		m.Fabricas = append(m.Fabricas, f)
		m.porID[f.ID] = f
	}
	m.analisisPrevio()
	m.dia = 0
	m.fase = len(fasesDia) - 1 // parte en la noche anterior al primer día
	m.progreso = 0.9
	m.fecha = m.fecha.AddDate(0, 0, -1)
	m.log("Sesión iniciada: %s", esc.Titulo)
	m.emitir("estado", nil)
}

func abreviarRespuesta(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// analisisPrevio genera tres meses de historia (lo que Beer llamó distribuciones
// gaussianas de tres meses) y deja entrenado el filtro, como hacían los
// estadísticos de ECOM con el "Prior Analysis of Time-Series of Processes".
func (m *Motor) analisisPrevio() {
	const dias = 65
	inicio := m.fecha
	fechas := make([]time.Time, 0, dias)
	f := inicio
	for len(fechas) < dias {
		f = f.AddDate(0, 0, -1)
		if f.Weekday() != time.Saturday && f.Weekday() != time.Sunday {
			fechas = append(fechas, f)
		}
	}
	// orden cronológico
	for i, j := 0, len(fechas)-1; i < j; i, j = i+1, j-1 {
		fechas[i], fechas[j] = fechas[j], fechas[i]
	}
	for _, ix := range m.porCodigo {
		ps := make([]float64, 0, dias)
		var regs []Registro
		for k, fe := range fechas {
			e := ix.Base + 0
			ix.ar = 0.35*ix.ar + m.rng.NormFloat64()*ix.Ruido
			e += ix.ar
			act := medirACT(ix, math.Max(0.05, e), int(fe.Month())-1)
			p, l, perf := Indices(act, ix.CAP, ix.POT, ix.Inverso)
			pd := p / ix.Estacional[int(fe.Month())-1]
			ps = append(ps, pd)
			regs = append(regs, Registro{Dia: k - dias, Fecha: fe.Format("020106"), ACT: act, P: p, L: l, PERF: perf, Previo: true})
		}
		banda, media, ruido := BandaDesdeHistoria(ps)
		ix.Banda, ix.RefMedia = banda, media
		ix.HS = NuevoMSM(media, ruido)
		for i := range regs {
			pd := ps[i]
			ix.HS.Actualizar(pd)
			regs[i].Prob, regs[i].Retro = ix.HS.Prob, ix.HS.Retro
			regs[i].Media, regs[i].Pend, regs[i].Pron = ix.HS.Media, ix.HS.Pendiente, ix.HS.SigPronSig
			regs[i].DesvPron = ix.HS.DesvPron
		}
		ix.Historia = regs
		ix.racha = 0
	}
	m.Ecom.CrearArchivos(m)
}

// ---------- reloj ----------

// Avanzar mueve el reloj según el tiempo real transcurrido.
func (m *Motor) Avanzar(dt time.Duration) {
	m.mu.Lock()
	m.avanzar(dt.Seconds())
	evts := m.tomarSalida()
	m.mu.Unlock()
	if len(evts) > 0 && m.Emitir != nil {
		m.Emitir(evts)
	}
}

func (m *Motor) avanzar(seg float64) {
	m.actualizarTelex()
	if m.Pueblo.pendiente {
		m.muestrearPueblo(false)
	}
	m.revisarCaducados()
	if m.pausa {
		m.emitirReloj(false)
		return
	}
	f := fasesDia[m.fase]
	if f.Nombre == "transmision" {
		m.transmitirAutomaticas()
	}
	dur := m.Cfg.SegundosPorDia * f.Peso / math.Max(m.Cfg.Velocidad, 0.05)
	nuevo := m.progreso + seg/dur
	if nuevo >= 1 {
		if !m.puedeTerminarFase() {
			m.progreso = 0.999
			m.emitirReloj(false)
			return
		}
		m.esperando = nil
		m.fase = (m.fase + 1) % len(fasesDia)
		m.progreso = 0
		m.entrarFase()
	} else {
		m.progreso = nuevo
	}
	m.emitirReloj(false)
}

// puedeTerminarFase decide si hay que esperar a los equipos humanos.
func (m *Motor) puedeTerminarFase() bool {
	f := fasesDia[m.fase].Nombre
	var faltan []string
	switch f {
	case "transmision":
		for _, fa := range m.Fabricas {
			if fa.Humano != "" && fa.Enviado != m.dia && m.Cfg.EsperarHumanos {
				faltan = append(faltan, fa.Corto)
			}
			if fa.Humano == "" && fa.Enviado != m.dia {
				faltan = append(faltan, "télex de "+fa.Corto) // automáticas aún transmitiendo
			}
		}
		for _, t := range m.Telex {
			if t.Estado == "transmitiendo" && t.Tipo == "indices" {
				faltan = append(faltan, "télex en línea")
				break
			}
		}
	case "corfo":
		for id := range m.corregir {
			if fa := m.porID[id]; fa != nil && fa.Humano != "" && m.Cfg.EsperarHumanos {
				faltan = append(faltan, fa.Corto+" (corrección)")
			}
		}
		for _, t := range m.Telex {
			if t.Estado == "transmitiendo" && (t.Tipo == "reenvio" || t.Tipo == "indices") {
				faltan = append(faltan, "télex a ECOM en línea")
				break
			}
		}
	case "informes":
		for _, t := range m.Telex {
			if t.Estado == "transmitiendo" && t.Tipo == "algedonico" {
				faltan = append(faltan, "informes en línea")
				break
			}
		}
	}
	sort.Strings(faltan)
	m.esperando = faltan
	if len(faltan) == 0 {
		m.esperaDesde = time.Time{}
		return true
	}
	if m.esperaDesde.IsZero() {
		m.esperaDesde = time.Now()
	}
	// a los equipos humanos se los espera un rato; los télex en línea siempre terminan
	soloHumanos := true
	for _, x := range faltan {
		if strings.HasPrefix(x, "télex") || strings.HasPrefix(x, "informes") {
			soloHumanos = false
		}
	}
	if soloHumanos && m.Cfg.EsperaMaxima > 0 && time.Since(m.esperaDesde).Seconds() > m.Cfg.EsperaMaxima {
		m.log("Se cerró la espera: %s no alcanzó a responder", strings.Join(faltan, ", "))
		m.aviso("CORFO cerró la recepción sin: %s", strings.Join(faltan, ", "))
		m.esperaDesde = time.Time{}
		if f := fasesDia[m.fase].Nombre; f == "corfo" {
			m.corregir = map[string]bool{}
			m.reenviarAECOM()
		}
		return true
	}
	return false
}

func (m *Motor) entrarFase() {
	switch fasesDia[m.fase].Nombre {
	case "produccion":
		m.nuevoDia()
	case "transmision":
		m.planificarTransmisiones()
	case "corfo":
		m.centralCORFO()
	case "ecom":
		if !m.reenviado {
			m.reenviarAECOM()
		}
		m.Ecom.Correr(m)
	case "informes":
		m.evaluarSenales()
	case "noche":
		m.emitir("noche", map[string]any{"dia": m.dia})
	}
}

func (m *Motor) emitirReloj(forzar bool) {
	if !forzar && time.Since(m.ultimoReloj) < 250*time.Millisecond {
		return
	}
	m.ultimoReloj = time.Now()
	m.emitir("reloj", m.reloj())
}

func (m *Motor) reloj() Reloj {
	f := fasesDia[m.fase]
	min := f.Desde + int(float64(f.Hasta-f.Desde)*m.progreso)
	fecha := m.fecha
	if min >= 24*60 {
		min -= 24 * 60
		fecha = fecha.AddDate(0, 0, 1)
	}
	return Reloj{
		Dia: m.dia, Fecha: fecha.Format("2006-01-02"), FechaCorta: m.fecha.Format("020106"),
		Juliana: fmt.Sprintf("%02d%03d", m.fecha.Year()%100, m.fecha.YearDay()),
		Hora: fmt.Sprintf("%02d:%02d", min/60, min%60), Minuto: min,
		Fase: f.Nombre, TituloFase: f.Titulo, Progreso: m.progreso, Pausa: m.pausa,
		Esperando: m.esperando,
	}
}

// ---------- jornada ----------

func (m *Motor) nuevoDia() {
	m.dia++
	m.fecha = m.fecha.AddDate(0, 0, 1)
	for m.fecha.Weekday() == time.Saturday || m.fecha.Weekday() == time.Sunday {
		m.fecha = m.fecha.AddDate(0, 0, 1)
	}
	m.recibidos = map[string][]*Mensaje{}
	m.corregir = map[string]bool{}
	m.reenvio = nil
	m.reenviado = false
	// los imprevistos viejos y ya terminados se olvidan
	vigentes := m.Eventos[:0]
	for _, ev := range m.Eventos {
		if ev.Inicio >= m.dia-30 || ev.activoEn(m.dia) {
			vigentes = append(vigentes, ev)
		}
	}
	m.Eventos = vigentes
	m.eventosDelEscenario()
	m.eventosAlAzar()
	mes := int(m.fecha.Month()) - 1
	for _, ix := range m.porCodigo {
		e := m.eficienciaHoy(ix, m.dia, m.rng)
		ix.HoyACT = medirACT(ix, e, mes)
		ix.HoyDia = m.dia
	}
	// vuelven los camiones
	var quedan []int
	for _, d := range m.camionesFuera {
		if d <= m.dia {
			m.Camiones += 25
		} else {
			quedan = append(quedan, d)
		}
	}
	m.camionesFuera = quedan
	m.actuanNivelesAutomaticos()
	m.log("Día %d · %s", m.dia, m.fecha.Format("02-01-2006"))
	m.emitir("dia", map[string]any{"dia": m.dia, "fecha": m.fecha.Format("2006-01-02")})
}

func (m *Motor) eventosAlAzar() {
	for _, f := range m.Fabricas {
		if m.rng.Float64() >= m.Cfg.Turbulencia {
			continue
		}
		ix := f.Indices[m.rng.Intn(len(f.Indices))]
		m.nuevoEventoAzar(f, ix, "azar")
	}
}

func (m *Motor) nuevoEventoAzar(f *Fabrica, ix *Indice, origen string) *Evento {
	r := m.rng.Float64()
	ev := &Evento{Fabrica: f.ID, Codigo: ix.Codigo, Inicio: m.dia, Origen: origen}
	switch {
	case r < 0.50:
		ev.Tipo = "escalon"
		causas := []string{"materia_prima", "materia_prima", "mantencion", "mantencion", "ausentismo", "demanda", "mejora"}
		ev.Causa = causas[m.rng.Intn(len(causas))]
		ev.Magnitud = -(0.10 + 0.15*m.rng.Float64())
		if ev.Causa == "mejora" {
			ev.Magnitud = 0.06 + 0.05*m.rng.Float64()
		}
		if m.rng.Float64() < 0.5 {
			ev.Duracion = 6 + m.rng.Intn(10)
		}
	case r < 0.72:
		ev.Tipo = "pendiente"
		ev.Causa = "mantencion"
		ev.Magnitud = -(0.012 + 0.012*m.rng.Float64())
		ev.Duracion = 8 + m.rng.Intn(6)
	default:
		ev.Tipo = "transitorio"
		ev.Causa = "mantencion"
		ev.Magnitud = -(0.25 + 0.2*m.rng.Float64())
	}
	ev.Novedad = NovedadTurno(m.rng, f.Sector, ev.Causa, ev.Tipo, ev.Magnitud)
	if ev.Tipo == "transitorio" {
		ev.Novedad = "Corte de luz de varias horas en la planta; mañana se normaliza."
	}
	m.agregarEvento(ev)
	return ev
}

func (m *Motor) agregarEvento(ev *Evento) {
	m.sigEvento++
	ev.ID = m.sigEvento
	m.Eventos = append(m.Eventos, ev)
	m.emitir("evento", ev)
}

// InyectarEvento lo usa el docente desde el panel de control.
func (m *Motor) InyectarEvento(codigo, tipo, causa string, magnitud float64, duracion int) (*Evento, error) {
	m.mu.Lock()
	defer m.flush()
	ix := m.porCodigo[codigo]
	if ix == nil {
		return nil, fmt.Errorf("no existe el índice %s", codigo)
	}
	f := m.porID[ix.Fabrica]
	ev := &Evento{Fabrica: f.ID, Codigo: codigo, Tipo: tipo, Causa: causa, Magnitud: magnitud,
		Inicio: m.dia + 1, Duracion: duracion, Origen: "docente"}
	if fasesDia[m.fase].Nombre == "produccion" && m.progreso < 0.5 {
		ev.Inicio = m.dia // todavía alcanza a afectar el día de hoy
		defer func() {
			e := m.eficienciaHoy(ix, m.dia, m.rng)
			ix.HoyACT = medirACT(ix, e, int(m.fecha.Month())-1)
		}()
	}
	if tipo == "digitacion" {
		ev.Causa = "digitacion"
	}
	ev.Novedad = NovedadTurno(m.rng, f.Sector, causa, tipo, magnitud)
	m.agregarEvento(ev)
	m.log("El docente provocó un %s en %s (%s)", tipo, f.Corto, ix.Abrev)
	return ev, nil
}

// eventosDelEscenario: el paro de octubre empieza el primer día.
func (m *Motor) eventosDelEscenario() {
	if m.Esc.ID != "octubre1972" {
		return
	}
	switch m.dia {
	case 1, 2, 3:
		// el paro alcanza a casi todas las fábricas en los primeros días
		for i, f := range m.Fabricas {
			if i%3 != m.dia-1 {
				continue
			}
			ix := f.Indices[0]
			ev := &Evento{Fabrica: f.ID, Codigo: ix.Codigo, Tipo: "escalon", Causa: "transporte",
				Magnitud: -(0.18 + 0.12*m.rng.Float64()), Inicio: m.dia, Duracion: 17, Origen: "escenario"}
			ev.Novedad = NovedadTurno(m.rng, f.Sector, "transporte", "escalon", ev.Magnitud)
			m.agregarEvento(ev)
		}
		if m.dia == 1 {
			m.log("Comienza el paro de octubre: 40.000 dueños de camiones detienen el transporte")
			m.aviso("Comienza el paro de octubre. Los camiones leales son pocos: hay %d disponibles.", m.Camiones)
		}
	case 17:
		m.aviso("El paro termina. Los camiones vuelven a circular.")
	}
}

// ---------- utilidades ----------

func (m *Motor) emitir(t string, d any) { m.salida = append(m.salida, Evt{t, d}) }

func (m *Motor) tomarSalida() []Evt {
	s := m.salida
	m.salida = nil
	return s
}

// flush desbloquea el motor y despacha lo acumulado.
func (m *Motor) flush() {
	evts := m.tomarSalida()
	m.mu.Unlock()
	if len(evts) > 0 && m.Emitir != nil {
		m.Emitir(evts)
	}
}

func (m *Motor) log(f string, a ...any) {
	s := fmt.Sprintf(f, a...)
	m.Bitacora = append(m.Bitacora, fmt.Sprintf("[%s %s] %s", m.fecha.Format("02-01-06"), m.reloj().Hora, s))
	if len(m.Bitacora) > 300 {
		m.Bitacora = m.Bitacora[len(m.Bitacora)-300:]
	}
	m.emitir("bitacora", m.Bitacora[len(m.Bitacora)-1])
}

func (m *Motor) aviso(f string, a ...any) { m.emitir("aviso", fmt.Sprintf(f, a...)) }

func (m *Motor) fechaTelex() string {
	return fmt.Sprintf("%d-%d-%02d", m.fecha.Day(), int(m.fecha.Month()), m.fecha.Year()%100)
}

// ---------- control ----------

func (m *Motor) Pausar(p bool) {
	m.mu.Lock()
	defer m.flush()
	m.pausa = p
	m.emitirReloj(true)
}

func (m *Motor) Configurar(c Config) {
	m.mu.Lock()
	defer m.flush()
	if c.SegundosPorDia < 10 {
		c.SegundosPorDia = 10
	}
	if c.Velocidad <= 0 {
		c.Velocidad = 1
	}
	if c.VelTelex < 1 {
		c.VelTelex = 1
	}
	if c.VentanaDias < 1 {
		c.VentanaDias = 1
	}
	m.Cfg = c
	m.emitir("config", c)
}

// SaltarFase fuerza el término de la fase actual (el docente no quiere esperar).
func (m *Motor) SaltarFase() {
	m.mu.Lock()
	defer m.flush()
	m.esperando = nil
	m.fase = (m.fase + 1) % len(fasesDia)
	m.progreso = 0
	m.entrarFase()
	m.emitirReloj(true)
}

func (m *Motor) ReiniciarSesion(escenario string) {
	m.mu.Lock()
	defer m.flush()
	m.Reiniciar(escenario, 0)
	m.emitirReloj(true)
}

// ---------- vistas ----------

type ResumenIndice struct {
	Codigo   string        `json:"codigo"`
	Fabrica  string        `json:"fabrica"`
	Sector   string        `json:"sector"`
	Nombre   string        `json:"nombre"`
	Abrev    string        `json:"abrev"`
	Unidad   string        `json:"unidad"`
	CAP      float64       `json:"cap"`
	POT      float64       `json:"pot"`
	Inverso  bool          `json:"inverso"`
	Decim    int           `json:"decim"`
	Real     bool          `json:"real"`
	Banda    Banda         `json:"banda"`
	Ultimo   *Registro     `json:"ultimo"`
	Serie    []float64     `json:"serie"` // últimos P
	SinDatos int           `json:"sinDatos"`
}

func (m *Motor) resumenIndice(ix *Indice) ResumenIndice {
	r := ResumenIndice{Codigo: ix.Codigo, Fabrica: ix.Fabrica, Sector: ix.Sector, Nombre: ix.Nombre,
		Abrev: ix.Abrev, Unidad: ix.Unidad, CAP: ix.CAP, POT: ix.POT, Inverso: ix.Inverso,
		Decim: ix.Decim, Real: ix.Real, Banda: ix.Banda, Ultimo: ix.Ultimo(), SinDatos: ix.SinDatos}
	desde := len(ix.Historia) - 30
	if desde < 0 {
		desde = 0
	}
	for _, h := range ix.Historia[desde:] {
		r.Serie = append(r.Serie, h.P)
	}
	return r
}

type Estado struct {
	Escenario   Escenario         `json:"escenario"`
	Escenarios  []Escenario       `json:"escenarios"`
	Config      Config            `json:"config"`
	Reloj       Reloj             `json:"reloj"`
	Sectores    []*Sector         `json:"sectores"`
	Fabricas    []*Fabrica        `json:"fabricas"`
	Indices     []ResumenIndice   `json:"indices"`
	Senales     []*Senal          `json:"senales"`
	Telex       []*Mensaje        `json:"telex"`
	Eventos     []*Evento         `json:"eventos"`
	Centrales   []Central         `json:"centrales"`
	Sala        *Sala             `json:"sala"`
	Pueblo      VistaPueblo       `json:"pueblo"`
	Ecom        VistaECOM         `json:"ecom"`
	Camiones    int               `json:"camiones"`
	Bitacora    []string          `json:"bitacora"`
	Acciones    []Accion          `json:"acciones"`
	Niveles     []string          `json:"niveles"`
	Total       Agregado          `json:"total"`
	PorSector   map[string]Agregado `json:"porSector"`
}

func (m *Motor) Estado() Estado {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.estado()
}

func (m *Motor) estado() Estado {
	m.armarMenu()
	e := Estado{
		Escenario: m.Esc, Escenarios: Escenarios, Config: m.Cfg, Reloj: m.reloj(),
		Sectores: Sectores, Fabricas: m.Fabricas, Senales: m.senalesVista(), Centrales: Centrales,
		Sala: m.Sala, Pueblo: m.Pueblo.Vista(), Ecom: m.Ecom.Vista(), Camiones: m.Camiones,
		Bitacora: m.Bitacora, Acciones: Acciones, Niveles: NombresNivel[:],
	}
	desde := len(m.Telex) - 60
	if desde < 0 {
		desde = 0
	}
	e.Telex = m.Telex[desde:]
	desdeE := len(m.Eventos) - 60
	if desdeE < 0 {
		desdeE = 0
	}
	e.Eventos = m.Eventos[desdeE:]
	for _, f := range m.Fabricas {
		for _, ix := range f.Indices {
			e.Indices = append(e.Indices, m.resumenIndice(ix))
		}
	}
	e.Total, e.PorSector = m.agregados()
	return e
}

// senalesVista: todas las activas y las últimas 30 resueltas.
func (m *Motor) senalesVista() []*Senal {
	var activas, resueltas []*Senal
	for _, s := range m.Senales {
		if s.Estado == "activa" {
			activas = append(activas, s)
		} else {
			resueltas = append(resueltas, s)
		}
	}
	if len(resueltas) > 30 {
		resueltas = resueltas[len(resueltas)-30:]
	}
	return append(resueltas, activas...)
}

// Historia completa de un índice (pantallas del Datafeed y del computador).
func (m *Motor) Historia(codigo string) (ResumenIndice, []Registro, *MSM, []*Evento, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ix := m.porCodigo[codigo]
	if ix == nil {
		return ResumenIndice{}, nil, nil, nil, fmt.Errorf("no existe %s", codigo)
	}
	var evs []*Evento
	for _, ev := range m.Eventos {
		if ev.Codigo == codigo {
			evs = append(evs, ev)
		}
	}
	h := append([]Registro(nil), ix.Historia...)
	hs := *ix.HS
	return m.resumenIndice(ix), h, &hs, evs, nil
}

// Agregado de la "Industria Total": promedios de las latencias,
// productividades y rendimientos (pantalla Staffy).
type Agregado struct {
	P    float64 `json:"p"`
	L    float64 `json:"l"`
	PERF float64 `json:"perf"`
	N    int     `json:"n"`
	Alertas int  `json:"alertas"`
}

func (m *Motor) agregados() (Agregado, map[string]Agregado) {
	por := map[string]Agregado{}
	var tot Agregado
	for _, f := range m.Fabricas {
		for _, ix := range f.Indices {
			u := ix.Ultimo()
			if u == nil {
				continue
			}
			a := por[f.Sector]
			a.P += u.P
			a.L += u.L
			a.PERF += u.PERF
			a.N++
			por[f.Sector] = a
			tot.P += u.P
			tot.L += u.L
			tot.PERF += u.PERF
			tot.N++
		}
	}
	for _, s := range m.Senales {
		if s.Estado == "activa" {
			a := por[s.Sector]
			a.Alertas++
			por[s.Sector] = a
			tot.Alertas++
		}
	}
	for k, a := range por {
		if a.N > 0 {
			a.P /= float64(a.N)
			a.L /= float64(a.N)
			a.PERF /= float64(a.N)
		}
		por[k] = a
	}
	if tot.N > 0 {
		tot.P /= float64(tot.N)
		tot.L /= float64(tot.N)
		tot.PERF /= float64(tot.N)
	}
	return tot, por
}

// ParteDelDia es lo que ve el equipo de una fábrica: el formulario con lo que
// midieron los supervisores y las novedades del turno.
type ParteDelDia struct {
	Fabrica  *Fabrica      `json:"fabrica"`
	Fecha    string        `json:"fecha"`
	Valores  []ValorParte  `json:"valores"`
	Novedades []string     `json:"novedades"`
	Senales  []*Senal      `json:"senales"`
	Telex    []*Mensaje    `json:"telex"`
	Enviado  bool          `json:"enviado"`
	Corregir bool          `json:"corregir"`
}

type ValorParte struct {
	Codigo string  `json:"codigo"`
	Nombre string  `json:"nombre"`
	Unidad string  `json:"unidad"`
	Valor  string  `json:"valor"`
}

func (m *Motor) Parte(id string) (ParteDelDia, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.porID[id]
	if f == nil {
		return ParteDelDia{}, fmt.Errorf("no existe la fábrica %s", id)
	}
	p := ParteDelDia{Fabrica: f, Fecha: m.fecha.Format("020106"), Enviado: f.Enviado == m.dia, Corregir: m.corregir[id]}
	for _, ix := range f.Indices {
		p.Valores = append(p.Valores, ValorParte{ix.Codigo, ix.Nombre, ix.Unidad, FormatoValor(ix.HoyACT, ix.Decim)})
	}
	for _, ev := range m.Eventos {
		if ev.Fabrica == id && ev.activoEn(m.dia) && ev.Novedad != "" {
			p.Novedades = append(p.Novedades, ev.Novedad)
		}
	}
	if len(p.Novedades) == 0 {
		p.Novedades = []string{"Turno sin novedades."}
	}
	for _, s := range m.Senales {
		if s.Fabrica == id && s.Estado == "activa" {
			p.Senales = append(p.Senales, s)
		}
	}
	for _, t := range m.Telex {
		if t.A == f.Telex || t.De == f.Telex {
			p.Telex = append(p.Telex, t)
		}
	}
	if len(p.Telex) > 30 {
		p.Telex = p.Telex[len(p.Telex)-30:]
	}
	return p, nil
}

// TomarFabrica asigna una fábrica a un equipo humano (o la libera con id vacío).
func (m *Motor) TomarFabrica(fabrica, participante, equipo string) error {
	m.mu.Lock()
	defer m.flush()
	f := m.porID[fabrica]
	if f == nil {
		return fmt.Errorf("no existe la fábrica %s", fabrica)
	}
	if participante != "" && f.Humano != "" && f.Humano != participante {
		return fmt.Errorf("%s ya está a cargo de otro equipo (%s)", f.Corto, f.Equipo)
	}
	f.Humano = participante
	f.Equipo = strings.TrimSpace(equipo)
	if participante == "" {
		f.Equipo = ""
		m.log("%s vuelve a ser automática", f.Corto)
	} else {
		m.log("El equipo «%s» se hace cargo de %s", f.Equipo, f.Corto)
	}
	m.emitir("fabricas", m.Fabricas)
	return nil
}

func (m *Motor) Bloquear()   { m.mu.Lock() }
func (m *Motor) Liberar()    { m.flush() }
func (m *Motor) EstadoSinBloqueo() Estado { return m.estado() }
