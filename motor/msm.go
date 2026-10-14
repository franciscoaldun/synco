package motor

// Modelo multiestado de Harrison y Stevens (1971), "A Bayesian Approach to
// Short-term Forecasting", Operational Research Quarterly 22(4): 341-362.
// Es el corazón estadístico de Cyberstride (programa PRO20).
//
// Proceso generador (modelo de crecimiento lineal, sin estacionalidad porque
// el índice P ya llega desestacionalizado):
//
//	d_t = μ_t + ε_t            ε ~ N(0, Vε(j))
//	μ_t = μ_{t-1} + β_t + γ_t   γ ~ N(0, Vγ(j))
//	β_t = β_{t-1} + δ_t         δ ~ N(0, Vδ(j))
//
// En cada observación el proceso está en uno de N=4 estados j, cada uno con sus
// propias varianzas. En la implementación chilena (Dunsmuir, "Prior Analysis of
// Time-Series of Processes", febrero 1972) los estados y sus pesos fueron:
//
//	1 sin cambio 75,0   2 cambio de escalón 8,5   3 cambio de pendiente 14,5   4 transitorio 2,0
//
// La posterior es una mezcla de N² normales bivariadas que se condensa a N
// componentes después de cada dato (si no, "absurdo desde el punto de vista
// computacional"):  v(j) = Σ_i p(i,j)[v(i,j) + (m(i,j) - m(j))²] / q(j).

import "math"

const (
	EstSinCambio = iota
	EstEscalon
	EstPendiente
	EstTransitorio
	NEstados
)

var NombreEstado = [NEstados]string{"SIN CAMBIO", "ESCALON", "PENDIENTE", "TRANSITORIO"}

// PriorDunsmuir son las probabilidades a priori de cada estado (marzo 1972).
var PriorDunsmuir = [NEstados]float64{0.750, 0.085, 0.145, 0.020}

// Multiplicadores de la varianza base V del ruido de observación para cada
// estado: {Vε, Vγ, Vδ}. Valores en la línea de los ejemplos de Harrison y
// Stevens; los originales de Cyberstride no se conservan.
var VarianzasEstado = [NEstados][3]float64{
	{1, 0.02, 0.0002}, // sin cambio: ruido normal, nivel y pendiente casi fijos
	{1, 25, 0.0002},   // escalón: salto grande del nivel μ
	{1, 0.02, 0.5},    // pendiente: cambio de la tendencia β
	{40, 0.02, 0.0002}, // transitorio: un dato aislado y lejano
}

// Comp es una componente normal bivariada de (μ, β).
type Comp struct {
	M    [2]float64    `json:"m"` // E(μ), E(β)
	C    [2][2]float64 `json:"c"` // covarianzas vμμ, vμβ / vβμ, vββ
	Peso float64       `json:"w"`
}

// MSM es el estado del filtro para una serie (un proceso del archivo maestro).
type MSM struct {
	V     float64          `json:"v"`     // varianza base del ruido
	Comps [NEstados]Comp   `json:"comps"` // posterior condensada, una por estado
	Prob  [NEstados]float64 `json:"prob"`  // probabilidad posterior de cada estado en el último dato
	Retro [NEstados]float64 `json:"retro"` // estado del dato ANTERIOR, revisado con el último (así clasificaba H-S)
	VarPendiente float64 `json:"varPendiente"`
	Prior [NEstados]float64 `json:"prior"`
	N     int              `json:"n"`
	// Últimos resultados, para los informes
	Pronostico  float64 `json:"pronostico"`  // pronóstico a un paso hecho ANTES del dato
	ErrorPron   float64 `json:"errorPron"`   // dato - pronóstico
	DesvPron    float64 `json:"desvPron"`    // desviación estándar predictiva
	Media       float64 `json:"media"`       // E(μ) posterior
	Pendiente   float64 `json:"pendiente"`   // E(β) posterior
	SigPronSig  float64 `json:"sigPronSig"`  // pronóstico para mañana
}

func NuevoMSM(nivel, varBase float64) *MSM {
	m := &MSM{V: varBase, Prior: PriorDunsmuir}
	for j := 0; j < NEstados; j++ {
		m.Comps[j] = Comp{
			M:    [2]float64{nivel, 0},
			C:    [2][2]float64{{varBase * 4, 0}, {0, varBase * 0.01}},
			Peso: m.Prior[j],
		}
	}
	m.Prob = m.Prior
	m.Media = nivel
	m.SigPronSig = nivel
	return m
}

func normalPDF(x, media, varianza float64) float64 {
	if varianza <= 0 {
		varianza = 1e-12
	}
	d := x - media
	return math.Exp(-d*d/(2*varianza)) / math.Sqrt(2*math.Pi*varianza)
}

// Pronostico a un paso según la mezcla actual.
func (m *MSM) pronostico() (media, varianza float64) {
	var mu, m2 float64
	for i := 0; i < NEstados; i++ {
		c := m.Comps[i]
		a := c.M[0] + c.M[1]
		// varianza del nivel evolucionado con el estado normal, más el ruido normal
		vr := c.C[0][0] + 2*c.C[0][1] + c.C[1][1] + m.V*(VarianzasEstado[0][1]+VarianzasEstado[0][2]) + m.V
		mu += c.Peso * a
		m2 += c.Peso * (vr + a*a)
	}
	return mu, m2 - mu*mu
}

// Actualizar incorpora un dato y devuelve la probabilidad posterior de cada estado.
func (m *MSM) Actualizar(d float64) [NEstados]float64 {
	pron, varPron := m.pronostico()
	m.Pronostico = pron
	m.DesvPron = math.Sqrt(math.Max(varPron, 0))
	m.ErrorPron = d - m.Pronostico

	type post struct {
		m [2]float64
		c [2][2]float64
		p float64
	}
	var pij [NEstados][NEstados]post
	var total float64
	for i := 0; i < NEstados; i++ {
		ci := m.Comps[i]
		if ci.Peso <= 0 {
			continue
		}
		for j := 0; j < NEstados; j++ {
			ve := m.V * VarianzasEstado[j][0]
			vg := m.V * VarianzasEstado[j][1]
			vd := m.V * VarianzasEstado[j][2]
			// Evolución: a = G m, R = G C G' + W, con G = [[1,1],[0,1]]
			a := [2]float64{ci.M[0] + ci.M[1], ci.M[1]}
			c := ci.C
			r00 := c[0][0] + 2*c[0][1] + c[1][1]
			r01 := c[0][1] + c[1][1]
			r11 := c[1][1]
			// W según μ_t = μ_{t-1} + β_{t-1} + δ + γ, β_t = β_{t-1} + δ
			r00 += vg + vd
			r01 += vd
			r11 += vd
			q := r00 + ve
			lik := normalPDF(d, a[0], q)
			k0, k1 := r00/q, r01/q
			e := d - a[0]
			pp := post{
				m: [2]float64{a[0] + k0*e, a[1] + k1*e},
				c: [2][2]float64{
					{r00 - k0*k0*q, r01 - k0*k1*q},
					{r01 - k0*k1*q, r11 - k1*k1*q},
				},
				p: ci.Peso * m.Prior[j] * lik,
			}
			pij[i][j] = pp
			total += pp.p
		}
	}
	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		// Dato imposible para todas las componentes: se reinicia en el dato.
		*m = *NuevoMSM(d, m.V)
		m.Prob = [NEstados]float64{0, 1, 0, 0}
		m.N++
		return m.Prob
	}
	// Probabilidad retrospectiva del estado del dato anterior: Σ_j p(i,j)
	for i := 0; i < NEstados; i++ {
		var r float64
		for j := 0; j < NEstados; j++ {
			r += pij[i][j].p / total
		}
		m.Retro[i] = r
	}
	// Condensación N² -> N
	for j := 0; j < NEstados; j++ {
		var qj float64
		var mj [2]float64
		for i := 0; i < NEstados; i++ {
			p := pij[i][j].p / total
			qj += p
			mj[0] += p * pij[i][j].m[0]
			mj[1] += p * pij[i][j].m[1]
		}
		var cj [2][2]float64
		if qj > 0 {
			mj[0] /= qj
			mj[1] /= qj
			for i := 0; i < NEstados; i++ {
				p := pij[i][j].p / total
				dm0 := pij[i][j].m[0] - mj[0]
				dm1 := pij[i][j].m[1] - mj[1]
				cj[0][0] += p * (pij[i][j].c[0][0] + dm0*dm0)
				cj[0][1] += p * (pij[i][j].c[0][1] + dm0*dm1)
				cj[1][1] += p * (pij[i][j].c[1][1] + dm1*dm1)
			}
			cj[0][0] /= qj
			cj[0][1] /= qj
			cj[1][1] /= qj
			cj[1][0] = cj[0][1]
		} else {
			mj = m.Comps[j].M
			cj = m.Comps[j].C
		}
		m.Comps[j] = Comp{M: mj, C: cj, Peso: qj}
		m.Prob[j] = qj
	}
	m.N++
	m.Media, m.Pendiente = 0, 0
	for j := 0; j < NEstados; j++ {
		m.Media += m.Comps[j].Peso * m.Comps[j].M[0]
		m.Pendiente += m.Comps[j].Peso * m.Comps[j].M[1]
	}
	m.VarPendiente = 0
	for j := 0; j < NEstados; j++ {
		c := m.Comps[j]
		db := c.M[1] - m.Pendiente
		m.VarPendiente += c.Peso * (c.C[1][1] + db*db)
	}
	m.SigPronSig = m.Media + m.Pendiente
	return m.Prob
}

// EstadoMasProbable devuelve el índice del estado con mayor probabilidad.
func (m *MSM) EstadoMasProbable() int {
	mejor := 0
	for j := 1; j < NEstados; j++ {
		if m.Prob[j] > m.Prob[mejor] {
			mejor = j
		}
	}
	return mejor
}
