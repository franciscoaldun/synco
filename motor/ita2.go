package motor

// Código Baudot-Murray ITA2 (CCITT, 1925/1932), el que usaban los teleimpresores
// Siemens T-100 de la red télex chilena. Cinco bits por carácter y dos "casos":
// LETRAS y CIFRAS, que se cambian con los caracteres LTRS (11111) y FIGS (11011).
//
// A 50 baudios cada carácter ocupa 7,5 unidades de 20 ms (1 de partida, 5 de datos
// y 1,5 de parada): 150 ms por carácter, unos 6,7 caracteres por segundo.

import (
	"strings"
	"unicode"
)

const (
	ITA2Nulo  = 0x00
	ITA2CR    = 0x02 // retorno de carro
	ITA2LF    = 0x08 // avance de línea
	ITA2Esp   = 0x04
	ITA2Cifra = 0x1B // FIGS
	ITA2Letra = 0x1F // LTRS

	MsPorCaracter = 150 // a 50 baudios
	Baudios       = 50
)

// Los cinco bits se escriben de izquierda a derecha como los numera la norma
// (bit 1 primero). Así "11000" es la A, igual que en la tabla de la cinta.
var ita2Letras = map[rune]string{
	'A': "11000", 'B': "10011", 'C': "01110", 'D': "10010", 'E': "10000",
	'F': "10110", 'G': "01011", 'H': "00101", 'I': "01100", 'J': "11010",
	'K': "11110", 'L': "01001", 'M': "00111", 'N': "00110", 'O': "00011",
	'P': "01101", 'Q': "11101", 'R': "01010", 'S': "10100", 'T': "00001",
	'U': "11100", 'V': "01111", 'W': "11001", 'X': "10111", 'Y': "10101",
	'Z': "10001",
}

var ita2Cifras = map[rune]string{
	'-': "11000", '?': "10011", ':': "01110", '3': "10000", '8': "01100",
	'(': "11110", ')': "01001", '.': "00111", ',': "00110", '9': "00011",
	'0': "01101", '1': "11101", '4': "01010", '\'': "10100", '5': "00001",
	'7': "11100", '=': "01111", '2': "11001", '/': "10111", '6': "10101",
	'+': "10001",
	'\a': "11010", // campanilla (CIFRAS J)
	'\x05': "10010", // ¿QUIÉN ES? / WRU (CIFRAS D)
}

var ita2Comunes = map[rune]string{
	' ': "00100", '\r': "00010", '\n': "01000",
}

// Trama es un carácter ya codificado, listo para perforar en la cinta.
type Trama struct {
	Bits   string `json:"b"` // "11000"
	Simbolo string `json:"s"` // lo que se imprime o el nombre de la función
	Caso   string `json:"c"` // "L", "C" o "" si vale en ambos
}

// NormalizarTelex deja un texto en el repertorio que un T-100 puede transmitir:
// mayúsculas, sin tildes, la Ñ como N y nada que no exista en ITA2.
func NormalizarTelex(s string) string {
	reemplazos := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
		"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ü", "U", "Ñ", "N",
		"\t", " ", "°", "O", "º", "O", "\"", "'", ";", ",", "!", ".", "_", "-",
	)
	s = reemplazos.Replace(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var b strings.Builder
	for _, r := range s {
		r = unicode.ToUpper(r)
		if _, ok := ita2Letras[r]; ok {
			b.WriteRune(r)
		} else if _, ok := ita2Cifras[r]; ok {
			b.WriteRune(r)
		} else if r == ' ' || r == '\n' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CodificarITA2 convierte texto (ya normalizado o no) en la secuencia de tramas
// que perfora la cinta, insertando los cambios de caso necesarios. Cada salto
// de línea se transmite como CR + LF, como en el télex real.
func CodificarITA2(texto string) []Trama {
	texto = NormalizarTelex(texto)
	var out []Trama
	caso := "" // desconocido al comienzo: el primer carácter fija el caso
	for _, r := range texto {
		if r == '\n' {
			out = append(out, Trama{"00010", "CR", ""}, Trama{"01000", "LF", ""})
			continue
		}
		if bits, ok := ita2Comunes[r]; ok {
			out = append(out, Trama{bits, string(r), ""})
			continue
		}
		if bits, ok := ita2Letras[r]; ok {
			if caso != "L" {
				out = append(out, Trama{"11111", "LTRS", ""})
				caso = "L"
			}
			out = append(out, Trama{bits, string(r), "L"})
			continue
		}
		if bits, ok := ita2Cifras[r]; ok {
			if caso != "C" {
				out = append(out, Trama{"11011", "FIGS", ""})
				caso = "C"
			}
			nombre := string(r)
			switch r {
			case '\a':
				nombre = "BELL"
			case '\x05':
				nombre = "WRU"
			}
			out = append(out, Trama{bits, nombre, "C"})
		}
	}
	return out
}

// DuracionTransmision en milisegundos a 50 baudios.
func DuracionTransmision(tramas int) int { return tramas * MsPorCaracter }

// DecodificarITA2 recorre una cinta y devuelve el texto impreso, para las pruebas.
func DecodificarITA2(tr []Trama) string {
	inv := func(m map[rune]string) map[string]rune {
		r := map[string]rune{}
		for k, v := range m {
			r[v] = k
		}
		return r
	}
	letras, cifras := inv(ita2Letras), inv(ita2Cifras)
	caso := 'L'
	var b strings.Builder
	for _, t := range tr {
		switch t.Bits {
		case "11111":
			caso = 'L'
		case "11011":
			caso = 'C'
		case "00010":
			b.WriteByte('\r')
		case "01000":
			b.WriteByte('\n')
		case "00100":
			b.WriteByte(' ')
		case "00000":
		default:
			if caso == 'L' {
				b.WriteRune(letras[t.Bits])
			} else {
				b.WriteRune(cifras[t.Bits])
			}
		}
	}
	return strings.ReplaceAll(b.String(), "\r\n", "\n")
}
