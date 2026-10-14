// Paquete qr: codificador de códigos QR (modo byte, corrección M, versiones
// 1 a 10) para que los estudiantes entren desde el celular sin teclear la
// dirección. Sin dependencias externas.
package qr

import (
	"errors"
	"fmt"
	"strings"
)

type bloque struct{ n, total, datos int }

// Tabla de bloques para el nivel M (ISO/IEC 18004), versiones 1-10.
var bloquesM = [][]bloque{
	nil,
	{{1, 26, 16}},
	{{1, 44, 28}},
	{{1, 70, 44}},
	{{2, 50, 32}},
	{{2, 67, 43}},
	{{4, 43, 27}},
	{{4, 49, 31}},
	{{2, 60, 38}, {2, 61, 39}},
	{{3, 58, 36}, {2, 59, 37}},
	{{4, 69, 43}, {1, 70, 44}},
}

var alineacion = [][]int{nil, nil, {6, 18}, {6, 22}, {6, 26}, {6, 30}, {6, 34}, {6, 22, 38}, {6, 24, 42}, {6, 26, 46}, {6, 28, 50}}

var infoVersion = map[int]int{7: 0x07C94, 8: 0x085BC, 9: 0x09A99, 10: 0x0A4D3}

// ---------- GF(256) y Reed-Solomon ----------

var expT, logT [512]int

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		expT[i] = x
		logT[x] = i
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11D
		}
	}
	for i := 255; i < 512; i++ {
		expT[i] = expT[i-255]
	}
}

func gfMul(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	return expT[logT[a]+logT[b]]
}

func generador(n int) []int {
	g := []int{1}
	for i := 0; i < n; i++ {
		ng := make([]int, len(g)+1)
		for j, c := range g {
			ng[j] ^= c
			ng[j+1] ^= gfMul(c, expT[i])
		}
		g = ng
	}
	return g
}

func reedSolomon(datos []int, n int) []int {
	g := generador(n)
	resto := make([]int, n)
	for _, d := range datos {
		f := d ^ resto[0]
		copy(resto, resto[1:])
		resto[n-1] = 0
		for j := 0; j < n; j++ {
			resto[j] ^= gfMul(g[j+1], f)
		}
	}
	return resto
}

// ---------- codificación ----------

type bits struct{ b []bool }

func (w *bits) poner(v, n int) {
	for i := n - 1; i >= 0; i-- {
		w.b = append(w.b, (v>>i)&1 == 1)
	}
}

// Matriz es el código resultante: true = módulo oscuro.
type Matriz struct {
	N       int
	M       [][]bool
	Version int
}

// Codificar arma el QR más chico que alcance para el texto.
func Codificar(texto string) (*Matriz, error) {
	datos := []byte(texto)
	for v := 1; v <= 10; v++ {
		capacidad := 0
		for _, bl := range bloquesM[v] {
			capacidad += bl.n * bl.datos
		}
		cuenta := 8
		if v >= 10 {
			cuenta = 16
		}
		if 4+cuenta+8*len(datos) <= capacidad*8 {
			return construir(datos, v, capacidad, cuenta)
		}
	}
	return nil, errors.New("texto demasiado largo para un QR versión 10")
}

func construir(datos []byte, v, capacidad, cuenta int) (*Matriz, error) {
	var w bits
	w.poner(0b0100, 4)
	w.poner(len(datos), cuenta)
	for _, d := range datos {
		w.poner(int(d), 8)
	}
	for i := 0; i < 4 && len(w.b) < capacidad*8; i++ {
		w.b = append(w.b, false)
	}
	for len(w.b)%8 != 0 {
		w.b = append(w.b, false)
	}
	relleno := []int{0xEC, 0x11}
	for k := 0; len(w.b) < capacidad*8; k++ {
		w.poner(relleno[k%2], 8)
	}
	palabras := make([]int, capacidad)
	for i := range palabras {
		x := 0
		for j := 0; j < 8; j++ {
			x <<= 1
			if w.b[i*8+j] {
				x |= 1
			}
		}
		palabras[i] = x
	}
	// bloques y corrección
	var dbl, ebl [][]int
	pos := 0
	for _, bl := range bloquesM[v] {
		for i := 0; i < bl.n; i++ {
			d := palabras[pos : pos+bl.datos]
			pos += bl.datos
			dbl = append(dbl, d)
			ebl = append(ebl, reedSolomon(d, bl.total-bl.datos))
		}
	}
	var final []int
	for i := 0; ; i++ {
		algo := false
		for _, d := range dbl {
			if i < len(d) {
				final = append(final, d[i])
				algo = true
			}
		}
		if !algo {
			break
		}
	}
	for i := 0; i < len(ebl[0]); i++ {
		for _, e := range ebl {
			final = append(final, e[i])
		}
	}

	n := 17 + 4*v
	var mejor *matrizTrabajo
	mejorPen := -1
	for mascara := 0; mascara < 8; mascara++ {
		m := nuevaMatriz(n, v)
		m.colocarDatos(final, mascara)
		m.formato(mascara)
		if p := m.penalizacion(); mejorPen < 0 || p < mejorPen {
			mejor, mejorPen = m, p
		}
	}
	return &Matriz{N: n, M: mejor.mod, Version: v}, nil
}

type matrizTrabajo struct {
	n        int
	v        int
	mod      [][]bool
	reservado [][]bool
}

func nuevaMatriz(n, v int) *matrizTrabajo {
	m := &matrizTrabajo{n: n, v: v}
	m.mod = make([][]bool, n)
	m.reservado = make([][]bool, n)
	for i := range m.mod {
		m.mod[i] = make([]bool, n)
		m.reservado[i] = make([]bool, n)
	}
	buscador := func(f, c int) {
		for dy := -1; dy <= 7; dy++ {
			for dx := -1; dx <= 7; dx++ {
				y, x := f+dy, c+dx
				if y < 0 || x < 0 || y >= n || x >= n {
					continue
				}
				oscuro := dy >= 0 && dy <= 6 && dx >= 0 && dx <= 6 &&
					(dy == 0 || dy == 6 || dx == 0 || dx == 6 || (dy >= 2 && dy <= 4 && dx >= 2 && dx <= 4))
				m.mod[y][x] = oscuro
				m.reservado[y][x] = true
			}
		}
	}
	buscador(0, 0)
	buscador(0, n-7)
	buscador(n-7, 0)
	for i := 8; i < n-8; i++ {
		m.mod[6][i] = i%2 == 0
		m.mod[i][6] = i%2 == 0
		m.reservado[6][i] = true
		m.reservado[i][6] = true
	}
	al := alineacion[v]
	for i, f := range al {
		for j, c := range al {
			ult := len(al) - 1
			if (i == 0 && j == 0) || (i == 0 && j == ult) || (i == ult && j == 0) {
				continue // se toparía con un buscador
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					m.mod[f+dy][c+dx] = dy == -2 || dy == 2 || dx == -2 || dx == 2 || (dy == 0 && dx == 0)
					m.reservado[f+dy][c+dx] = true
				}
			}
		}
	}
	// zonas de formato
	for i := 0; i < 9; i++ {
		m.reservado[8][i] = true
		m.reservado[i][8] = true
	}
	for i := 0; i < 8; i++ {
		m.reservado[8][n-1-i] = true
		m.reservado[n-1-i][8] = true
	}
	m.mod[n-8][8] = true // módulo oscuro fijo
	m.reservado[n-8][8] = true
	if v >= 7 {
		info := infoVersion[v]
		for i := 0; i < 18; i++ {
			b := (info>>i)&1 == 1
			f, c := i/3, n-11+i%3
			m.mod[f][c], m.reservado[f][c] = b, true
			m.mod[c][f], m.reservado[c][f] = b, true
		}
	}
	return m
}

func aplicaMascara(mascara, f, c int) bool {
	switch mascara {
	case 0:
		return (f+c)%2 == 0
	case 1:
		return f%2 == 0
	case 2:
		return c%3 == 0
	case 3:
		return (f+c)%3 == 0
	case 4:
		return (f/2+c/3)%2 == 0
	case 5:
		return (f*c)%2+(f*c)%3 == 0
	case 6:
		return ((f*c)%2+(f*c)%3)%2 == 0
	default:
		return ((f+c)%2+(f*c)%3)%2 == 0
	}
}

func (m *matrizTrabajo) colocarDatos(palabras []int, mascara int) {
	n := m.n
	total := len(palabras) * 8
	k := 0
	subiendo := true
	for col := n - 1; col > 0; col -= 2 {
		if col == 6 {
			col--
		}
		for i := 0; i < n; i++ {
			f := i
			if subiendo {
				f = n - 1 - i
			}
			for dc := 0; dc < 2; dc++ {
				c := col - dc
				if m.reservado[f][c] {
					continue
				}
				b := false
				if k < total {
					b = (palabras[k/8]>>(7-k%8))&1 == 1
				}
				k++
				if aplicaMascara(mascara, f, c) {
					b = !b
				}
				m.mod[f][c] = b
			}
		}
		subiendo = !subiendo
	}
}

func (m *matrizTrabajo) formato(mascara int) {
	datos := 0<<3 | mascara // nivel M = 00
	r := datos
	for i := 0; i < 10; i++ {
		r = (r << 1) ^ ((r >> 9) * 0x537)
	}
	f := (datos<<10 | r) ^ 0x5412
	bit := func(i int) bool { return (f>>i)&1 == 1 }
	n := m.n
	// primera copia, junto al buscador superior izquierdo
	for i := 0; i <= 5; i++ {
		m.mod[i][8] = bit(i)
	}
	m.mod[7][8] = bit(6)
	m.mod[8][8] = bit(7)
	m.mod[8][7] = bit(8)
	for i := 9; i < 15; i++ {
		m.mod[8][14-i] = bit(i)
	}
	// segunda copia, repartida entre los otros dos buscadores
	for i := 0; i < 8; i++ {
		m.mod[8][n-1-i] = bit(i)
	}
	for i := 8; i < 15; i++ {
		m.mod[n-15+i][8] = bit(i)
	}
	m.mod[n-8][8] = true
}

func (m *matrizTrabajo) penalizacion() int {
	n := m.n
	p := 0
	// N1: corridas de 5 o más del mismo color
	for f := 0; f < n; f++ {
		for _, horizontal := range []bool{true, false} {
			corrida := 1
			for c := 1; c < n; c++ {
				var a, b bool
				if horizontal {
					a, b = m.mod[f][c], m.mod[f][c-1]
				} else {
					a, b = m.mod[c][f], m.mod[c-1][f]
				}
				if a == b {
					corrida++
				} else {
					if corrida >= 5 {
						p += 3 + corrida - 5
					}
					corrida = 1
				}
			}
			if corrida >= 5 {
				p += 3 + corrida - 5
			}
		}
	}
	// N2: bloques 2x2
	for f := 0; f < n-1; f++ {
		for c := 0; c < n-1; c++ {
			x := m.mod[f][c]
			if m.mod[f+1][c] == x && m.mod[f][c+1] == x && m.mod[f+1][c+1] == x {
				p += 3
			}
		}
	}
	// N3: patrón parecido al buscador
	pat1 := []bool{true, false, true, true, true, false, true, false, false, false, false}
	pat2 := []bool{false, false, false, false, true, false, true, true, true, false, true}
	for f := 0; f < n; f++ {
		for c := 0; c <= n-11; c++ {
			for _, pat := range [][]bool{pat1, pat2} {
				h, vv := true, true
				for k := 0; k < 11; k++ {
					if m.mod[f][c+k] != pat[k] {
						h = false
					}
					if m.mod[c+k][f] != pat[k] {
						vv = false
					}
				}
				if h {
					p += 40
				}
				if vv {
					p += 40
				}
			}
		}
	}
	// N4: proporción de oscuros
	oscuros := 0
	for f := 0; f < n; f++ {
		for c := 0; c < n; c++ {
			if m.mod[f][c] {
				oscuros++
			}
		}
	}
	desv := oscuros*100/(n*n) - 50
	if desv < 0 {
		desv = -desv
	}
	p += desv / 5 * 10
	return p
}

// SVG devuelve el código como imagen vectorial con margen de 4 módulos.
func (q *Matriz) SVG(color, fondo string) string {
	var b strings.Builder
	t := q.N + 8
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="%d" height="%d" fill="%s"/><path fill="%s" d="`, t, t, t, t, fondo, color)
	for f := 0; f < q.N; f++ {
		for c := 0; c < q.N; c++ {
			if q.M[f][c] {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", c+4, f+4)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String()
}

// Texto devuelve la matriz como filas de 0 y 1 (para las pruebas).
func (q *Matriz) Texto() string {
	var b strings.Builder
	for _, fila := range q.M {
		for _, x := range fila {
			if x {
				b.WriteByte('1')
			} else {
				b.WriteByte('0')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}
