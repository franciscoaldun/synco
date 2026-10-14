package motor

// Tarjetas perforadas de 80 columnas (código Hollerith del perforador IBM 029)
// y su equivalente EBCDIC en la memoria del 360. En ECOM los datos que llegaban
// por télex se pasaban de la cinta de papel a tarjetas: tres tipos de
// transacción, TR 01, 02 y 03, leídas en un orden estricto.

import (
	"fmt"
	"strings"
)

// Filas de una tarjeta, de arriba abajo: 12, 11, 0, 1, ..., 9.
var FilasTarjeta = []string{"12", "11", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}

var hollerith = map[rune][]string{
	'&': {"12"}, '-': {"11"}, '/': {"0", "1"},
	'.': {"12", "3", "8"}, '<': {"12", "4", "8"}, '(': {"12", "5", "8"}, '+': {"12", "6", "8"},
	'$': {"11", "3", "8"}, '*': {"11", "4", "8"}, ')': {"11", "5", "8"}, ';': {"11", "6", "8"},
	',': {"0", "3", "8"}, '%': {"0", "4", "8"}, '_': {"0", "5", "8"}, '>': {"0", "6", "8"}, '?': {"0", "7", "8"},
	':': {"2", "8"}, '#': {"3", "8"}, '@': {"4", "8"}, '\'': {"5", "8"}, '=': {"6", "8"}, '"': {"7", "8"},
}

func init() {
	for d := '0'; d <= '9'; d++ {
		hollerith[d] = []string{string(d)}
	}
	for i, r := range "ABCDEFGHI" {
		hollerith[r] = []string{"12", fmt.Sprint(i + 1)}
	}
	for i, r := range "JKLMNOPQR" {
		hollerith[r] = []string{"11", fmt.Sprint(i + 1)}
	}
	for i, r := range "STUVWXYZ" {
		hollerith[r] = []string{"0", fmt.Sprint(i + 2)}
	}
}

// Perforaciones de un carácter (filas perforadas).
func Perforaciones(r rune) []string { return hollerith[r] }

// EBCDIC de un carácter, como lo vería un volcado del 360.
func EBCDIC(r rune) byte {
	switch {
	case r >= 'A' && r <= 'I':
		return 0xC1 + byte(r-'A')
	case r >= 'J' && r <= 'R':
		return 0xD1 + byte(r-'J')
	case r >= 'S' && r <= 'Z':
		return 0xE2 + byte(r-'S')
	case r >= '0' && r <= '9':
		return 0xF0 + byte(r-'0')
	}
	switch r {
	case ' ':
		return 0x40
	case '.':
		return 0x4B
	case '<':
		return 0x4C
	case '(':
		return 0x4D
	case '+':
		return 0x4E
	case '&':
		return 0x50
	case '$':
		return 0x5B
	case '*':
		return 0x5C
	case ')':
		return 0x5D
	case ';':
		return 0x5E
	case '-':
		return 0x60
	case '/':
		return 0x61
	case ',':
		return 0x6B
	case '%':
		return 0x6C
	case '_':
		return 0x6D
	case '>':
		return 0x6E
	case '?':
		return 0x6F
	case ':':
		return 0x7A
	case '#':
		return 0x7B
	case '@':
		return 0x7C
	case '\'':
		return 0x7D
	case '=':
		return 0x7E
	case '"':
		return 0x7F
	}
	return 0x40
}

// HexEBCDIC de una cadena: "C3C9D4..."
func HexEBCDIC(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, "%02X", EBCDIC(r))
	}
	return b.String()
}

// Tarjeta es una transacción perforada. El diseño de columnas es una
// reconstrucción: el "Manual de perforación TS-07" (ECOM, junio 1972) fijaba el
// orden código-fecha-valor, la fecha DDMMAA con ceros a la izquierda, el punto
// decimal y las cifras alineadas a la derecha, que es lo que se respeta aquí.
//
//	col  1-2   tipo de transacción (01 ACT, 02 nueva CAP, 03 nueva POT)
//	col  3-24  código del proceso (22 letras: sector + planta + índice)
//	col 25-30  fecha DDMMAA
//	col 31-42  valor, alineado a la derecha, con punto decimal
//	col 73-80  secuencia
type Tarjeta struct {
	Tipo      string `json:"tipo"`
	Codigo    string `json:"codigo"`
	Fecha     string `json:"fecha"`
	Valor     string `json:"valor"`
	Secuencia int    `json:"secuencia"`
	Texto     string `json:"texto"` // las 80 columnas
}

func NuevaTarjeta(tipo, codigo, fecha, valor string, seq int) Tarjeta {
	t := Tarjeta{Tipo: tipo, Codigo: codigo, Fecha: fecha, Valor: valor, Secuencia: seq}
	col := []rune(strings.Repeat(" ", 80))
	put := func(desde int, s string) {
		for i, r := range s {
			if desde+i < 80 {
				col[desde+i] = r
			}
		}
	}
	put(0, tipo)
	put(2, padDer(codigo, 22))
	put(24, fecha)
	put(30, padIzq(valor, 12))
	put(72, fmt.Sprintf("CYB%05d", seq))
	t.Texto = string(col)
	return t
}

func padDer(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}

func padIzq(s string, n int) string {
	if len(s) >= n {
		return s[len(s)-n:]
	}
	return strings.Repeat(" ", n-len(s)) + s
}
