package motor

// La "realidad" simulada: lo que pasa en el piso de la fábrica y que Cyberstride
// sólo conoce a través de los índices que llegan por télex.

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
)

// Registro es una línea del archivo histórico (más lo que calculó PRO20).
type Registro struct {
	Dia     int               `json:"dia"`
	Fecha   string            `json:"fecha"` // DDMMAA
	ACT     float64           `json:"act"`
	P       float64           `json:"p"`
	L       float64           `json:"l"`
	PERF    float64           `json:"perf"`
	Prob    [NEstados]float64 `json:"prob"`
	Retro   [NEstados]float64 `json:"retro"`
	Media   float64           `json:"media"`
	Pend    float64           `json:"pend"`
	Pron    float64           `json:"pron"` // pronóstico para el día siguiente
	DesvPron float64          `json:"desv"`
	Clas    Clasificacion     `json:"clas"`
	Previo  bool              `json:"previo,omitempty"` // análisis previo (antes de iniciar la sesión)
}

// Indice es un proceso vivo: su verdad de terreno y su ficha en el archivo maestro.
type Indice struct {
	DefIndice
	Fabrica    string      `json:"fabrica"`
	Sector     string      `json:"sector"`
	Estacional [12]float64 `json:"-"` // SYSVAR(44..55)

	// verdad de terreno (no la ve Cyberstride)
	ar       float64
	HoyACT   float64 `json:"-"`
	HoyDia   int     `json:"-"`

	// archivo maestro
	HS        *MSM       `json:"-"`
	Banda     Banda      `json:"-"`
	RefMedia  float64    `json:"-"`
	racha     int
	SinDatos  int        `json:"-"` // corridas seguidas sin transacción
	Historia  []Registro `json:"-"`
	CAPNueva  float64    `json:"-"` // TR 02 pendiente
	POTNueva  float64    `json:"-"` // TR 03 pendiente
}

func (ix *Indice) Ultimo() *Registro {
	if len(ix.Historia) == 0 {
		return nil
	}
	return &ix.Historia[len(ix.Historia)-1]
}

// Fabrica en tiempo de ejecución.
type Fabrica struct {
	ID        string    `json:"id"`
	Sector    string    `json:"sector"`
	Nombre    string    `json:"nombre"`
	Corto     string    `json:"corto"`
	Lugar     string    `json:"lugar"`
	Lat       float64   `json:"lat"`
	Central   string    `json:"central"`
	Telex     string    `json:"telex"`
	Respuesta string    `json:"respuesta"` // tambor de respuesta del T-100
	Indices   []*Indice `json:"indices"`
	Humano    string    `json:"humano"`    // participante a cargo ("" = automática)
	Equipo    string    `json:"equipo"`    // nombre del equipo de estudiantes
	Enviado   int       `json:"enviado"`   // último día en que transmitió sus índices
}

// Evento en el piso de la fábrica.
type Evento struct {
	ID        int     `json:"id"`
	Fabrica   string  `json:"fabrica"`
	Codigo    string  `json:"codigo"`
	Tipo      string  `json:"tipo"`  // escalon | pendiente | transitorio | digitacion
	Causa     string  `json:"causa"` // materia_prima | mantencion | ausentismo | transporte | demanda | mejora | digitacion
	Magnitud  float64 `json:"magnitud"`
	Inicio    int     `json:"inicio"`
	Duracion  int     `json:"duracion"` // escalón: días hasta que se arregla solo (0 = no se arregla solo); pendiente: días de caída
	Resuelto  bool    `json:"resuelto"`
	FinDia    int     `json:"finDia"`
	Origen    string  `json:"origen"` // azar | docente | escenario
	Novedad   string  `json:"novedad"`
}

func (e *Evento) activoEn(dia int) bool {
	if dia < e.Inicio {
		return false
	}
	if e.Resuelto {
		return dia < e.FinDia
	}
	switch e.Tipo {
	case "transitorio", "digitacion":
		return dia == e.Inicio
	case "escalon":
		return e.Duracion == 0 || dia < e.Inicio+e.Duracion
	}
	return true
}

// efecto sobre la eficiencia latente en un día.
func (e *Evento) efecto(dia int) float64 {
	if e.Tipo == "digitacion" || dia < e.Inicio {
		return 0
	}
	recup := 1.0
	if e.Resuelto || (e.Tipo == "escalon" && e.Duracion > 0 && dia >= e.Inicio+e.Duracion) {
		fin := e.FinDia
		if !e.Resuelto {
			fin = e.Inicio + e.Duracion
		}
		switch d := dia - fin; {
		case d < 0:
		case d == 0:
			recup = 0.4
		default:
			return 0
		}
	}
	switch e.Tipo {
	case "transitorio":
		if dia == e.Inicio {
			return e.Magnitud
		}
		return 0
	case "escalon":
		return e.Magnitud * recup
	case "pendiente":
		n := dia - e.Inicio + 1
		if e.Duracion > 0 && n > e.Duracion {
			n = e.Duracion
		}
		v := e.Magnitud * float64(n)
		if v < -0.45 {
			v = -0.45
		}
		if v > 0.2 {
			v = 0.2
		}
		return v * recup
	}
	return 0
}

// Vocabulario del piso de fábrica por sector, para las novedades del turno.
var insumos = map[string][]string{
	"TEX": {"algodón", "lana sucia", "rayón", "colorantes"},
	"CIM": {"caliza", "petróleo para los hornos", "yeso", "puzolana"},
	"FOR": {"madera en trozos", "soda cáustica", "azufre"},
	"ELE": {"cinescopios importados", "componentes electrónicos", "plancha de acero"},
	"CAU": {"caucho natural importado", "negro de humo", "alambre de acero"},
}
var maquinas = map[string][]string{
	"TEX": {"una carda", "el telar Sulzer N°3", "la continua de hilar", "la caldera"},
	"CIM": {"el horno rotatorio N°2", "el molino de clínker", "la chancadora primaria"},
	"FOR": {"el digestor", "la máquina papelera", "el descortezador"},
	"ELE": {"la línea de montaje", "la soldadora por ola", "el banco de pruebas"},
	"CAU": {"la vulcanizadora", "el mezclador Banbury", "la extrusora"},
}

func elegir(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

// NovedadTurno: lo que el jefe de turno le cuenta al equipo de la fábrica.
func NovedadTurno(r *rand.Rand, sector, causa, tipo string, mag float64) string {
	switch causa {
	case "materia_prima":
		return fmt.Sprintf("Llegó sólo una parte del pedido de %s. El proveedor dice que no tiene divisas para importar.", elegir(r, insumos[sector]))
	case "mantencion":
		if tipo == "pendiente" {
			return fmt.Sprintf("%s viene fallando cada vez más; los mecánicos piden repuestos que no llegan.", capitalizar(elegir(r, maquinas[sector])))
		}
		return fmt.Sprintf("Se detuvo %s por una falla. Se trabaja con capacidad reducida.", elegir(r, maquinas[sector]))
	case "ausentismo":
		return "Mucha inasistencia: problemas de locomoción colectiva y asambleas largas en el turno de noche."
	case "transporte":
		return "Los camiones no salen: paro de los dueños de camiones. No llegan insumos ni salen despachos."
	case "demanda":
		return "La bodega está llena: los distribuidores no retiran la producción y hubo que bajar el ritmo."
	case "mejora":
		return fmt.Sprintf("El comité de producción reorganizó los turnos y volvió a andar %s después de la reparación: se está produciendo más que nunca.", elegir(r, maquinas[sector]))
	}
	return ""
}

func capitalizar(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// eficienciaHoy calcula la eficiencia latente de un índice en un día.
func (m *Motor) eficienciaHoy(ix *Indice, dia int, r *rand.Rand) float64 {
	ix.ar = 0.35*ix.ar + r.NormFloat64()*ix.Ruido
	e := ix.Base + ix.ar
	for _, ev := range m.Eventos {
		if ev.Codigo == ix.Codigo {
			e += ev.efecto(dia)
		}
	}
	return math.Max(0.05, math.Min(1.15, e))
}

// medirACT convierte la eficiencia en el valor que se anota en el formulario.
func medirACT(ix *Indice, e float64, mes int) float64 {
	s := ix.Estacional[mes]
	if s <= 0 {
		s = 1
	}
	var act float64
	if ix.Inverso {
		act = ix.CAP / (e * s)
	} else {
		act = ix.CAP * e * s
	}
	f := math.Pow(10, float64(ix.Decim))
	return math.Round(act*f) / f
}

// FormatoValor como lo escribe la fábrica en el formulario ECOM: punto decimal.
func FormatoValor(v float64, decim int) string {
	return strconv.FormatFloat(v, 'f', decim, 64)
}

// errorDigitacion produce el típico error de transcripción entre el
// formulario y el teleimpresor: coma corrida o dos cifras traspuestas.
func errorDigitacion(r *rand.Rand, v string) string {
	if r.Intn(2) == 0 {
		if i := strings.IndexByte(v, '.'); i > 1 {
			return v[:i-1] + "." + v[i-1:i] + v[i+1:]
		}
		return v + "0"
	}
	b := []byte(v)
	for k := 0; k < 6; k++ {
		i := r.Intn(len(b) - 1)
		if b[i] != '.' && b[i+1] != '.' && b[i] != b[i+1] {
			b[i], b[i+1] = b[i+1], b[i]
			return string(b)
		}
	}
	return v + "0"
}
