package motor

// Cyberstride: el cálculo de índices de Beer y el análisis de PRO20.
//
// Del diagrama de flujo de PRO20 (suite temporal, Dunsmuir, feb. 1972):
//
//	ACT  = VALUE
//	P    = ACT/CAP(I)
//	PERF = ACT/POT(I)
//	IF (SYSVAR(56) .NE. 0)  P = 1./P ; PERF = 1./PERF      (índices "al revés")
//	XL   = PERF/P
//	XL   = ARSIN(AMIN1(1.,XL))/1.570796
//	P    = ARSIN(AMIN1(1.,P))/1.570796
//	PERF = ARSIN(AMIN1(1.,PERF))/1.570796
//	CALL STATS(I)
//
// Productividad P = actualidad/capacidad, latencia L = capacidad/potencialidad,
// rendimiento PERF = actualidad/potencialidad = P·L (Beer, Brain of the Firm, 164).
// El arcoseno es la "normalización trigonométrica" que dejaba las series cerca
// de una normal.

import "math"

const medioPi = 1.570796 // el literal de PRO20

// Indices calcula P, L y PERF exactamente como el diagrama de PRO20.
func Indices(act, cap, pot float64, inverso bool) (p, l, perf float64) {
	act, cap, pot = Real4(act), Real4(cap), Real4(pot)
	if cap <= 0 || pot <= 0 || act <= 0 {
		return 0, 0, 0
	}
	p = act / cap
	perf = act / pot
	if inverso {
		p = 1 / p
		perf = 1 / perf
	}
	xl := perf / p
	xl = math.Asin(math.Min(1, xl)) / medioPi
	p = math.Asin(math.Min(1, p)) / medioPi
	perf = math.Asin(math.Min(1, perf)) / medioPi
	return Real4(p), Real4(xl), Real4(perf)
}

// Banda taxonómica: rango en que "la variable (en rigor, su tendencia) se
// comporta satisfactoriamente y no es preciso tomar acción correctiva alguna"
// (Gabella, Técnica de la flujogramación cuantificada, CORFO, marzo 1973).
type Banda struct {
	Inf float64 `json:"inf"`
	Med float64 `json:"med"`
	Sup float64 `json:"sup"`
}

// Clasificación de un día de PRO20 para una serie.
type Clasificacion struct {
	Estado      int     `json:"estado"`      // estado más probable del dato de hoy
	ProbCambio  float64 `json:"probCambio"`  // P(escalón)+P(transitorio) hoy
	TransAyer   bool    `json:"transAyer"`   // el dato de ayer fue transitorio
	EscalonAyer bool    `json:"escalonAyer"` // ayer hubo un cambio de escalón
	Pendiente   bool    `json:"pendiente"`   // cambio de pendiente sostenido
	Anomalia    bool    `json:"anomalia"`    // el dato de hoy es improbable (por clasificar mañana)
	FueraBanda  int     `json:"fueraBanda"`  // -1 bajo la banda, +1 sobre, 0 dentro
	MovMedia    bool    `json:"movMedia"`    // la media actualizada se alejó de la original
	Direccion   int     `json:"direccion"`   // -1 baja, +1 sube (en escala de P)
}

// UmbralPendiente: la tendencia debe superar esta fracción de la desviación
// del ruido por día, con el mismo signo, durante DiasPendiente días seguidos.
const (
	UmbralPendiente = 0.35
	DiasPendiente   = 3
)

// Clasificar aplica las reglas de excepción después de actualizar el filtro.
// rachaPend lleva la cuenta de días seguidos con pendiente marcada (con signo).
func Clasificar(m *MSM, p float64, b Banda, refMedia float64, rachaPend *int) Clasificacion {
	c := Clasificacion{Estado: m.EstadoMasProbable()}
	c.ProbCambio = m.Prob[EstEscalon] + m.Prob[EstTransitorio]
	c.Anomalia = c.ProbCambio > 0.8
	c.TransAyer = m.N > 1 && m.Retro[EstTransitorio] > 0.5
	c.EscalonAyer = m.N > 1 && m.Retro[EstEscalon] > 0.5
	sd := math.Sqrt(m.V)
	z := 0.0
	if m.VarPendiente > 0 {
		z = m.Pendiente / math.Sqrt(m.VarPendiente)
	}
	switch {
	case m.Pendiente < -UmbralPendiente*sd && z < -1.2:
		if *rachaPend > 0 {
			*rachaPend = 0
		}
		*rachaPend--
	case m.Pendiente > UmbralPendiente*sd && z > 1.2:
		if *rachaPend < 0 {
			*rachaPend = 0
		}
		*rachaPend++
	default:
		*rachaPend = 0
	}
	c.Pendiente = *rachaPend <= -DiasPendiente || *rachaPend >= DiasPendiente
	if p < b.Inf {
		c.FueraBanda = -1
	} else if p > b.Sup {
		c.FueraBanda = 1
	}
	// Segunda fase de PRO20: "la media actualizada se compara con la media que
	// originalmente tomó el proceso para ver si hubo un movimiento significativo".
	c.MovMedia = math.Abs(m.Media-refMedia) > 3*sd
	switch {
	case c.Pendiente:
		c.Direccion = signo(m.Pendiente)
	case c.FueraBanda != 0:
		c.Direccion = c.FueraBanda
	default:
		c.Direccion = signo(m.Media - refMedia)
		if c.Anomalia {
			c.Direccion = signo(m.ErrorPron)
		}
	}
	return c
}

func signo(x float64) int {
	if x < 0 {
		return -1
	}
	if x > 0 {
		return 1
	}
	return 0
}

// BandaDesdeHistoria arma la banda con la media y la desviación de la serie de
// P del período de análisis previo (tres meses, como pedía Beer).
func BandaDesdeHistoria(ps []float64) (Banda, float64, float64) {
	if len(ps) == 0 {
		return Banda{0, 0.5, 1}, 0.5, 0.01
	}
	var s, s2 float64
	for _, x := range ps {
		s += x
		s2 += x * x
	}
	n := float64(len(ps))
	media := s / n
	varz := math.Max(s2/n-media*media, 1e-6)
	// ruido: la mitad de la varianza de las primeras diferencias
	var dif float64
	for i := 1; i < len(ps); i++ {
		d := ps[i] - ps[i-1]
		dif += d * d
	}
	ruido := varz
	if len(ps) > 2 {
		ruido = math.Max(dif/(2*float64(len(ps)-1)), 1e-6)
	}
	sd := math.Sqrt(varz)
	med := mediana(ps)
	return Banda{
		Inf: math.Max(0, med-3*sd),
		Med: med,
		Sup: math.Min(1, med+3*sd),
	}, media, ruido
}

func mediana(xs []float64) float64 {
	c := append([]float64(nil), xs...)
	// inserción: las series son cortas
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j] < c[j-1]; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}
