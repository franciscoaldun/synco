// Paquete dynamo: intérprete del lenguaje DYNAMO de Jay W. Forrester (MIT),
// el que usó el subproyecto Futuro / CHECO (CHilean ECOnomy) de Cybersyn en su
// versión DYNAMOP, escrita en Fortran por P. Williams en el Queen Mary College.
//
// Sintaxis soportada (DYNAMO II):
//
//	* TITULO
//	NOTE comentario
//	L NIVEL.K=NIVEL.J+(DT)(ENTRA.JK-SALE.JK)    ecuación de nivel
//	A AUX.K=...                                  auxiliar
//	S SUP.K=...                                  suplementaria (sólo para mostrar)
//	R TASA.KL=...                                tasa
//	C CONST=1.5/OTRA=2                           constantes
//	N NIVEL=...                                  valor inicial
//	T TABLA=0/.5/1/1.5                           tabla
//	X ...                                        continuación de la línea anterior
//	SPEC DT=.25/LENGTH=10/PRTPER=1/PLTPER=.5
//	PRINT A,B
//	PLOT A=A,B=B(0,100)/C=*
//
// Como en el original, (DT)(X) es una multiplicación implícita.
// Funciones: TABLE, TABHL, CLIP, SWITCH, MAX, MIN, ABS, SQRT, EXP, LOGN, SIN,
// COS, STEP, RAMP, PULSE, NOISE, SMOOTH, DELAY1, DELAY3.
package dynamo

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type Ecuacion struct {
	Tipo    string `json:"tipo"`
	Nombre  string `json:"nombre"`
	Texto   string `json:"texto"`
	Linea   int    `json:"linea"`
	expr    nodo
	tabla   []float64
}

type GrupoPlot struct {
	Vars []VarPlot `json:"vars"`
	Min  float64   `json:"min"`
	Max  float64   `json:"max"`
	Fijo bool      `json:"fijo"`
}

type VarPlot struct {
	Nombre string `json:"nombre"`
	Simbolo string `json:"simbolo"`
}

type Modelo struct {
	Titulo   string
	Notas    []string
	Ecs      []*Ecuacion
	porNombre map[string]*Ecuacion
	Spec     map[string]float64
	Print    []string
	Plot     []GrupoPlot
	slots    int
}

type Resultado struct {
	Titulo      string               `json:"titulo"`
	Notas       []string             `json:"notas"`
	Tiempos     []float64            `json:"tiempos"`
	Series      map[string][]float64 `json:"series"`
	Plot        []GrupoPlot          `json:"plot"`
	Print       []string             `json:"print"`
	TextoPlot   string               `json:"textoPlot"`
	TextoPrint  string               `json:"textoPrint"`
	Constantes  map[string]float64   `json:"constantes"`
	Ecuaciones  []*Ecuacion          `json:"ecuaciones"`
	Spec        map[string]float64   `json:"spec"`
	Pasos       int                  `json:"pasos"`
}

type ErrorDynamo struct {
	Linea int
	Msg   string
}

func (e ErrorDynamo) Error() string {
	if e.Linea > 0 {
		return fmt.Sprintf("línea %d: %s", e.Linea, e.Msg)
	}
	return e.Msg
}

// ---------- análisis léxico de líneas ----------

// Compilar lee el texto fuente de un modelo.
func Compilar(fuente string) (*Modelo, error) {
	m := &Modelo{porNombre: map[string]*Ecuacion{}, Spec: map[string]float64{"DT": 0.25, "LENGTH": 10, "PRTPER": 1, "PLTPER": 0.5, "TIME": 0}}
	type lineaF struct {
		n     int
		texto string
	}
	var lineas []lineaF
	for i, raw := range strings.Split(strings.ReplaceAll(fuente, "\r\n", "\n"), "\n") {
		l := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(l) == "" {
			continue
		}
		up := strings.ToUpper(strings.TrimSpace(l))
		if strings.HasPrefix(up, "X ") && len(lineas) > 0 {
			lineas[len(lineas)-1].texto += strings.TrimSpace(up[2:])
			continue
		}
		lineas = append(lineas, lineaF{i + 1, strings.TrimSpace(l)})
	}
	for _, lf := range lineas {
		l := lf.texto
		up := strings.ToUpper(l)
		if strings.HasPrefix(up, "*") {
			if m.Titulo == "" {
				m.Titulo = strings.TrimSpace(l[1:])
			} else {
				m.Notas = append(m.Notas, strings.TrimSpace(l[1:]))
			}
			continue
		}
		campos := strings.SplitN(up, " ", 2)
		cmd := campos[0]
		resto := ""
		if len(campos) > 1 {
			resto = strings.TrimSpace(campos[1])
		}
		if cmd == "NOTE" {
			m.Notas = append(m.Notas, strings.TrimSpace(l[4:]))
			continue
		}
		// los comentarios al final de una ecuación van separados por dos espacios o más
		if i := strings.Index(resto, "  "); i >= 0 && cmd != "PLOT" && cmd != "PRINT" {
			resto = strings.TrimSpace(resto[:i])
		}
		resto = strings.ReplaceAll(resto, " ", "")
		switch cmd {
		case "SPEC":
			for _, p := range strings.Split(resto, "/") {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) != 2 {
					continue
				}
				v, err := strconv.ParseFloat(kv[1], 64)
				if err != nil {
					return nil, ErrorDynamo{lf.n, "SPEC con valor no numérico: " + p}
				}
				m.Spec[kv[0]] = v
			}
		case "PRINT":
			for _, p := range strings.FieldsFunc(resto, func(r rune) bool { return r == ',' || r == '/' }) {
				m.Print = append(m.Print, strings.TrimSpace(strings.SplitN(p, "=", 2)[0]))
			}
		case "PLOT":
			for _, g := range strings.Split(resto, "/") {
				grupo := GrupoPlot{}
				for _, p := range splitPlot(g) {
					escala := ""
					if i := strings.Index(p, "("); i >= 0 {
						escala = strings.Trim(p[i:], "()")
						p = p[:i]
					}
					kv := strings.SplitN(p, "=", 2)
					sim := "*"
					if len(kv) == 2 && kv[1] != "" {
						sim = kv[1][:1]
					} else if len(kv[0]) > 0 {
						sim = kv[0][:1]
					}
					grupo.Vars = append(grupo.Vars, VarPlot{kv[0], sim})
					if escala != "" {
						lim := strings.Split(escala, ",")
						if len(lim) == 2 {
							a, e1 := strconv.ParseFloat(lim[0], 64)
							b, e2 := strconv.ParseFloat(lim[1], 64)
							if e1 == nil && e2 == nil {
								grupo.Min, grupo.Max, grupo.Fijo = a, b, true
							}
						}
					}
				}
				if len(grupo.Vars) > 0 {
					m.Plot = append(m.Plot, grupo)
				}
			}
		case "RUN":
		case "L", "A", "S", "R", "N", "C", "T":
			partes := []string{resto}
			if cmd == "C" {
				partes = strings.Split(resto, "/")
			}
			for _, p := range partes {
				if err := m.agregarEcuacion(cmd, p, lf.n); err != nil {
					return nil, err
				}
			}
		default:
			return nil, ErrorDynamo{lf.n, fmt.Sprintf("tipo de ecuación desconocido %q (use L, A, R, C, N, T, S, NOTE, SPEC, PRINT o PLOT)", cmd)}
		}
	}
	if err := m.verificar(); err != nil {
		return nil, err
	}
	return m, nil
}

func splitPlot(g string) []string {
	var out []string
	prof := 0
	ini := 0
	for i, r := range g {
		switch r {
		case '(':
			prof++
		case ')':
			prof--
		case ',':
			if prof == 0 {
				out = append(out, g[ini:i])
				ini = i + 1
			}
		}
	}
	return append(out, g[ini:])
}

func (m *Modelo) agregarEcuacion(tipo, texto string, linea int) error {
	i := strings.Index(texto, "=")
	if i <= 0 {
		return ErrorDynamo{linea, "falta el signo = en " + texto}
	}
	izq, der := texto[:i], texto[i+1:]
	nombre := izq
	if j := strings.Index(izq, "."); j >= 0 {
		nombre = izq[:j]
		suf := izq[j+1:]
		esperado := map[string]string{"L": "K", "A": "K", "S": "K", "R": "KL"}[tipo]
		if esperado != "" && suf != esperado {
			return ErrorDynamo{linea, fmt.Sprintf("una ecuación %s debe definir %s.%s, no %s", tipo, nombre, esperado, izq)}
		}
	}
	if !esNombre(nombre) {
		return ErrorDynamo{linea, "nombre inválido: " + nombre}
	}
	ec := &Ecuacion{Tipo: tipo, Nombre: nombre, Texto: tipo + " " + texto, Linea: linea}
	if tipo == "T" {
		for _, v := range strings.Split(der, "/") {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return ErrorDynamo{linea, "valor de tabla no numérico: " + v}
			}
			ec.tabla = append(ec.tabla, f)
		}
	} else {
		p := &parser{src: der, linea: linea, modelo: m}
		e, err := p.parse()
		if err != nil {
			return err
		}
		ec.expr = e
	}
	clave := nombre
	if tipo == "N" {
		clave = "N:" + nombre
	}
	if previa, ok := m.porNombre[clave]; ok {
		return ErrorDynamo{linea, fmt.Sprintf("%s ya fue definida en la línea %d", nombre, previa.Linea)}
	}
	m.porNombre[clave] = ec
	m.Ecs = append(m.Ecs, ec)
	return nil
}

func esNombre(s string) bool {
	if s == "" || len(s) > 12 {
		return false
	}
	for i, r := range s {
		if !(unicode.IsUpper(r) || (i > 0 && unicode.IsDigit(r))) {
			return false
		}
	}
	return true
}

func (m *Modelo) verificar() error {
	for _, e := range m.Ecs {
		if e.Tipo == "L" {
			if _, ok := m.porNombre["N:"+e.Nombre]; !ok {
				return ErrorDynamo{e.Linea, fmt.Sprintf("el nivel %s no tiene valor inicial (falta N %s=...)", e.Nombre, e.Nombre)}
			}
		}
	}
	var err error
	for _, e := range m.Ecs {
		if e.expr == nil {
			continue
		}
		e.expr.recorrer(func(n nodo) {
			if v, ok := n.(*nVar); ok && err == nil {
				if _, existe := m.porNombre[v.nombre]; !existe && !esPredefinida(v.nombre) {
					err = ErrorDynamo{e.Linea, fmt.Sprintf("%s usa %s, que no está definida", e.Nombre, v.nombre)}
				}
			}
			if t, ok := n.(*nFunc); ok && err == nil && (t.nombre == "TABLE" || t.nombre == "TABHL") {
				if len(t.args) > 0 {
					if v, ok := t.args[0].(*nVar); ok {
						if ec := m.porNombre[v.nombre]; ec == nil || ec.Tipo != "T" {
							err = ErrorDynamo{e.Linea, fmt.Sprintf("%s no es una tabla (falta T %s=...)", v.nombre, v.nombre)}
						}
					}
				}
			}
		})
	}
	return err
}

func esPredefinida(n string) bool { return n == "TIME" || n == "DT" }

// ---------- expresiones ----------

type nodo interface {
	eval(c *contexto) float64
	recorrer(func(nodo))
}

type nNum struct{ v float64 }
type nVar struct {
	nombre string
	suf    string
}
type nBin struct {
	op   byte
	a, b nodo
}
type nNeg struct{ a nodo }
type nFunc struct {
	nombre string
	args   []nodo
	slot   int
}

func (n *nNum) recorrer(f func(nodo)) { f(n) }
func (n *nVar) recorrer(f func(nodo)) { f(n) }
func (n *nBin) recorrer(f func(nodo)) { f(n); n.a.recorrer(f); n.b.recorrer(f) }
func (n *nNeg) recorrer(f func(nodo)) { f(n); n.a.recorrer(f) }
func (n *nFunc) recorrer(f func(nodo)) {
	f(n)
	for _, a := range n.args {
		a.recorrer(f)
	}
}

var funciones = map[string]int{
	"TABLE": 5, "TABHL": 5, "CLIP": 4, "SWITCH": 3, "MAX": 2, "MIN": 2, "ABS": 1, "SQRT": 1,
	"EXP": 1, "LOGN": 1, "SIN": 1, "COS": 1, "STEP": 2, "RAMP": 2, "PULSE": 3, "NOISE": 0,
	"SMOOTH": 2, "DELAY1": 2, "DELAY3": 2,
}

type parser struct {
	src    string
	pos    int
	linea  int
	modelo *Modelo
}

func (p *parser) err(msg string) error {
	return ErrorDynamo{p.linea, fmt.Sprintf("%s (en «%s»)", msg, p.src)}
}

func (p *parser) mirar() byte {
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return 0
}

func (p *parser) parse() (nodo, error) {
	n, err := p.suma()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.src) {
		return nil, p.err(fmt.Sprintf("sobra «%s»", p.src[p.pos:]))
	}
	return n, nil
}

func (p *parser) suma() (nodo, error) {
	a, err := p.producto()
	if err != nil {
		return nil, err
	}
	for {
		c := p.mirar()
		if c != '+' && c != '-' {
			return a, nil
		}
		p.pos++
		b, err := p.producto()
		if err != nil {
			return nil, err
		}
		a = &nBin{c, a, b}
	}
}

func (p *parser) producto() (nodo, error) {
	a, err := p.potencia()
	if err != nil {
		return nil, err
	}
	for {
		c := p.mirar()
		switch {
		case c == '*' && !(p.pos+1 < len(p.src) && p.src[p.pos+1] == '*'):
			p.pos++
		case c == '/':
			p.pos++
		case c == '(':
			// multiplicación implícita: (DT)(X)
			c = '*'
		default:
			return a, nil
		}
		b, err := p.potencia()
		if err != nil {
			return nil, err
		}
		a = &nBin{c, a, b}
	}
}

func (p *parser) potencia() (nodo, error) {
	a, err := p.unario()
	if err != nil {
		return nil, err
	}
	if p.mirar() == '*' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '*' {
		p.pos += 2
		b, err := p.potencia()
		if err != nil {
			return nil, err
		}
		return &nBin{'^', a, b}, nil
	}
	return a, nil
}

func (p *parser) unario() (nodo, error) {
	if p.mirar() == '-' {
		p.pos++
		a, err := p.unario()
		if err != nil {
			return nil, err
		}
		return &nNeg{a}, nil
	}
	if p.mirar() == '+' {
		p.pos++
	}
	return p.primario()
}

func (p *parser) primario() (nodo, error) {
	c := p.mirar()
	switch {
	case c == '(':
		p.pos++
		n, err := p.suma()
		if err != nil {
			return nil, err
		}
		if p.mirar() != ')' {
			return nil, p.err("falta un paréntesis de cierre")
		}
		p.pos++
		return n, nil
	case c == '.' || (c >= '0' && c <= '9'):
		ini := p.pos
		for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9' || p.src[p.pos] == '.') {
			p.pos++
		}
		if p.pos < len(p.src) && p.src[p.pos] == 'E' {
			p.pos++
			if p.mirar() == '-' || p.mirar() == '+' {
				p.pos++
			}
			for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
				p.pos++
			}
		}
		v, err := strconv.ParseFloat(p.src[ini:p.pos], 64)
		if err != nil {
			return nil, p.err("número mal escrito " + p.src[ini:p.pos])
		}
		return &nNum{v}, nil
	case c >= 'A' && c <= 'Z':
		ini := p.pos
		for p.pos < len(p.src) && (p.src[p.pos] >= 'A' && p.src[p.pos] <= 'Z' || p.src[p.pos] >= '0' && p.src[p.pos] <= '9') {
			p.pos++
		}
		nombre := p.src[ini:p.pos]
		if aridad, esF := funciones[nombre]; esF && p.mirar() == '(' {
			p.pos++
			var args []nodo
			if p.mirar() != ')' {
				for {
					a, err := p.suma()
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					if p.mirar() == ',' {
						p.pos++
						continue
					}
					break
				}
			}
			if p.mirar() != ')' {
				return nil, p.err("falta ) en la función " + nombre)
			}
			p.pos++
			if len(args) != aridad {
				return nil, p.err(fmt.Sprintf("%s lleva %d argumentos", nombre, aridad))
			}
			f := &nFunc{nombre: nombre, args: args}
			if nombre == "SMOOTH" || nombre == "DELAY1" || nombre == "DELAY3" {
				f.slot = p.modelo.slots
				p.modelo.slots++
			}
			return f, nil
		}
		suf := ""
		if p.mirar() == '.' {
			p.pos++
			ini2 := p.pos
			for p.pos < len(p.src) && (p.src[p.pos] == 'J' || p.src[p.pos] == 'K' || p.src[p.pos] == 'L') {
				p.pos++
			}
			suf = p.src[ini2:p.pos]
		}
		return &nVar{nombre, suf}, nil
	}
	if c == 0 {
		return nil, p.err("la expresión termina antes de tiempo")
	}
	return nil, p.err(fmt.Sprintf("carácter inesperado «%c»", c))
}

// ---------- evaluación ----------

type estadoRetardo struct {
	init    bool
	niveles [3]float64
	entrada float64
	demora  float64
	tipo    string
}

type contexto struct {
	m       *Modelo
	val     map[string]float64 // valores actuales (K / KL)
	prev    map[string]float64 // valores de niveles en J
	enNivel bool
	tiempo  float64
	dt      float64
	rng     *rand.Rand
	slots   []estadoRetardo
	pila    map[string]bool
	err     error
}

func (n *nNum) eval(c *contexto) float64 { return n.v }
func (n *nNeg) eval(c *contexto) float64 { return -n.a.eval(c) }

func (n *nVar) eval(c *contexto) float64 {
	switch n.nombre {
	case "TIME":
		return c.tiempo
	case "DT":
		return c.dt
	}
	if c.enNivel && n.suf == "J" {
		if v, ok := c.prev[n.nombre]; ok {
			return v
		}
	}
	if v, ok := c.val[n.nombre]; ok {
		return v
	}
	// durante la inicialización se calcula lo que falte, en orden de dependencias
	if c.pila != nil {
		return c.inicializar(n.nombre)
	}
	return 0
}

func (n *nBin) eval(c *contexto) float64 {
	a, b := n.a.eval(c), n.b.eval(c)
	switch n.op {
	case '+':
		return a + b
	case '-':
		return a - b
	case '*':
		return a * b
	case '/':
		if b == 0 {
			if c.err == nil {
				c.err = fmt.Errorf("división por cero en el tiempo %g", c.tiempo)
			}
			return 0
		}
		return a / b
	case '^':
		return math.Pow(a, b)
	}
	return 0
}

func (n *nFunc) eval(c *contexto) float64 {
	arg := func(i int) float64 { return n.args[i].eval(c) }
	switch n.nombre {
	case "TABLE", "TABHL":
		t := c.m.porNombre[n.args[0].(*nVar).nombre].tabla
		x, lo, hi, inc := arg(1), arg(2), arg(3), arg(4)
		return interpolar(t, x, lo, hi, inc)
	case "CLIP":
		if arg(2) >= arg(3) {
			return arg(0)
		}
		return arg(1)
	case "SWITCH":
		if arg(2) == 0 {
			return arg(0)
		}
		return arg(1)
	case "MAX":
		return math.Max(arg(0), arg(1))
	case "MIN":
		return math.Min(arg(0), arg(1))
	case "ABS":
		return math.Abs(arg(0))
	case "SQRT":
		return math.Sqrt(math.Max(0, arg(0)))
	case "EXP":
		return math.Exp(arg(0))
	case "LOGN":
		return math.Log(math.Max(1e-300, arg(0)))
	case "SIN":
		return math.Sin(arg(0))
	case "COS":
		return math.Cos(arg(0))
	case "STEP":
		if c.tiempo >= arg(1)-1e-9 {
			return arg(0)
		}
		return 0
	case "RAMP":
		if c.tiempo > arg(1) {
			return arg(0) * (c.tiempo - arg(1))
		}
		return 0
	case "PULSE":
		h, t0, int_ := arg(0), arg(1), arg(2)
		if c.tiempo < t0-1e-9 {
			return 0
		}
		if int_ <= 0 {
			if math.Abs(c.tiempo-t0) < c.dt/2 {
				return h / c.dt
			}
			return 0
		}
		k := math.Mod(c.tiempo-t0, int_)
		if k < c.dt/2 || int_-k < c.dt/2 {
			return h / c.dt
		}
		return 0
	case "NOISE":
		return c.rng.Float64() - 0.5
	case "SMOOTH", "DELAY1", "DELAY3":
		s := &c.slots[n.slot]
		x, d := arg(0), arg(1)
		if d <= 0 {
			d = c.dt
		}
		s.entrada, s.demora, s.tipo = x, d, n.nombre
		if !s.init {
			s.init = true
			switch n.nombre {
			case "SMOOTH":
				s.niveles[0] = x
			case "DELAY1":
				s.niveles[0] = x * d
			case "DELAY3":
				for i := 0; i < 3; i++ {
					s.niveles[i] = x * d / 3
				}
			}
		}
		switch n.nombre {
		case "SMOOTH":
			return s.niveles[0]
		case "DELAY1":
			return s.niveles[0] / d
		default:
			return s.niveles[2] / (d / 3)
		}
	}
	return 0
}

// integrarRetardos avanza los niveles internos de SMOOTH y DELAY.
func (c *contexto) integrarRetardos() {
	for i := range c.slots {
		s := &c.slots[i]
		if !s.init {
			continue
		}
		dt, d := c.dt, s.demora
		switch s.tipo {
		case "SMOOTH":
			s.niveles[0] += dt * (s.entrada - s.niveles[0]) / d
		case "DELAY1":
			s.niveles[0] += dt * (s.entrada - s.niveles[0]/d)
		case "DELAY3":
			d3 := d / 3
			r0, r1, r2 := s.niveles[0]/d3, s.niveles[1]/d3, s.niveles[2]/d3
			s.niveles[0] += dt * (s.entrada - r0)
			s.niveles[1] += dt * (r0 - r1)
			s.niveles[2] += dt * (r1 - r2)
		}
	}
}

func interpolar(t []float64, x, lo, hi, inc float64) float64 {
	if len(t) == 0 {
		return 0
	}
	if inc <= 0 {
		return t[0]
	}
	if x <= lo {
		return t[0]
	}
	if x >= hi {
		return t[len(t)-1]
	}
	pos := (x - lo) / inc
	i := int(math.Floor(pos))
	if i >= len(t)-1 {
		return t[len(t)-1]
	}
	f := pos - float64(i)
	return t[i] + f*(t[i+1]-t[i])
}

// inicializar calcula un valor faltante en el tiempo 0 (constantes, N, A, R).
func (c *contexto) inicializar(nombre string) float64 {
	if c.pila[nombre] {
		if c.err == nil {
			c.err = fmt.Errorf("definición circular al inicializar %s", nombre)
		}
		return 0
	}
	c.pila[nombre] = true
	defer delete(c.pila, nombre)
	m := c.m
	if ec, ok := m.porNombre["N:"+nombre]; ok {
		v := ec.expr.eval(c)
		c.val[nombre] = v
		return v
	}
	if ec, ok := m.porNombre[nombre]; ok && ec.expr != nil {
		v := ec.expr.eval(c)
		c.val[nombre] = v
		return v
	}
	return 0
}

// ordenAux ordena las auxiliares según sus dependencias.
func (m *Modelo) ordenAux() ([]*Ecuacion, error) {
	aux := map[string]*Ecuacion{}
	for _, e := range m.Ecs {
		if e.Tipo == "A" || e.Tipo == "S" {
			aux[e.Nombre] = e
		}
	}
	var orden []*Ecuacion
	estado := map[string]int{}
	var visitar func(n string) error
	visitar = func(n string) error {
		switch estado[n] {
		case 1:
			return ErrorDynamo{aux[n].Linea, "ecuaciones auxiliares circulares en " + n}
		case 2:
			return nil
		}
		estado[n] = 1
		var err error
		aux[n].expr.recorrer(func(x nodo) {
			if v, ok := x.(*nVar); ok && err == nil {
				if _, es := aux[v.nombre]; es {
					err = visitar(v.nombre)
				}
			}
		})
		if err != nil {
			return err
		}
		estado[n] = 2
		orden = append(orden, aux[n])
		return nil
	}
	var nombres []string
	for n := range aux {
		nombres = append(nombres, n)
	}
	sort.Strings(nombres)
	for _, n := range nombres {
		if err := visitar(n); err != nil {
			return nil, err
		}
	}
	return orden, nil
}

// Correr simula el modelo. Las constantes en «ajustes» reemplazan a las del texto.
func (m *Modelo) Correr(ajustes map[string]float64, semilla int64) (*Resultado, error) {
	dt, largo := m.Spec["DT"], m.Spec["LENGTH"]
	if dt <= 0 || largo <= 0 {
		return nil, fmt.Errorf("SPEC necesita DT y LENGTH positivos")
	}
	pasos := int(math.Round(largo / dt))
	if pasos > 20000 {
		return nil, fmt.Errorf("demasiados pasos (%d): aumente DT o reduzca LENGTH", pasos)
	}
	c := &contexto{m: m, val: map[string]float64{}, prev: map[string]float64{}, dt: dt,
		tiempo: m.Spec["TIME"], rng: rand.New(rand.NewSource(semilla)), slots: make([]estadoRetardo, m.slots),
		pila: map[string]bool{}}
	res := &Resultado{Titulo: m.Titulo, Notas: m.Notas, Series: map[string][]float64{}, Plot: m.Plot,
		Print: m.Print, Constantes: map[string]float64{}, Ecuaciones: m.Ecs, Spec: m.Spec, Pasos: pasos}

	// constantes (con ajustes)
	for _, e := range m.Ecs {
		if e.Tipo == "C" {
			v := e.expr.eval(c)
			if a, ok := ajustes[e.Nombre]; ok {
				v = a
			}
			c.val[e.Nombre] = v
			res.Constantes[e.Nombre] = v
		}
	}
	for k, v := range ajustes {
		if _, ok := c.val[k]; !ok {
			c.val[k] = v
		}
	}
	aux, err := m.ordenAux()
	if err != nil {
		return nil, err
	}
	var niveles, tasas []*Ecuacion
	for _, e := range m.Ecs {
		switch e.Tipo {
		case "L":
			niveles = append(niveles, e)
		case "R":
			tasas = append(tasas, e)
		}
	}
	// tiempo inicial: niveles desde N, después auxiliares y tasas
	for _, e := range niveles {
		c.inicializar(e.Nombre)
	}
	for _, e := range aux {
		c.val[e.Nombre] = e.expr.eval(c)
	}
	for _, e := range tasas {
		c.inicializar(e.Nombre)
	}
	c.pila = nil
	if c.err != nil {
		return nil, c.err
	}
	vars := m.variablesSalida()
	guardar := func() {
		res.Tiempos = append(res.Tiempos, c.tiempo)
		for _, v := range vars {
			res.Series[v] = append(res.Series[v], c.val[v])
		}
	}
	guardar()
	for k := 1; k <= pasos; k++ {
		for _, e := range niveles {
			c.prev[e.Nombre] = c.val[e.Nombre]
		}
		c.tiempo += dt
		c.enNivel = true
		nuevos := make([]float64, len(niveles))
		for i, e := range niveles {
			nuevos[i] = e.expr.eval(c)
		}
		c.enNivel = false
		for i, e := range niveles {
			c.val[e.Nombre] = nuevos[i]
		}
		c.integrarRetardos()
		for _, e := range aux {
			c.val[e.Nombre] = e.expr.eval(c)
		}
		for _, e := range tasas {
			c.val[e.Nombre] = e.expr.eval(c)
		}
		if c.err != nil {
			return nil, c.err
		}
		for _, v := range vars {
			if x := c.val[v]; math.IsNaN(x) || math.IsInf(x, 0) {
				return nil, fmt.Errorf("%s se volvió infinita en el tiempo %g", v, c.tiempo)
			}
		}
		guardar()
	}
	res.TextoPlot = textoPlot(res)
	res.TextoPrint = textoPrint(res)
	return res, nil
}

func (m *Modelo) variablesSalida() []string {
	vistos := map[string]bool{}
	var out []string
	for _, e := range m.Ecs {
		if (e.Tipo == "L" || e.Tipo == "A" || e.Tipo == "S" || e.Tipo == "R") && !vistos[e.Nombre] {
			vistos[e.Nombre] = true
			out = append(out, e.Nombre)
		}
	}
	return out
}
