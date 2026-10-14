package motor

// Aritmética de punto flotante hexadecimal del IBM System/360 (formato corto, 32 bits).
//
// El IBM 360/50H de ECOM guardaba los REAL*4 de Fortran así:
//   bit 0      signo
//   bits 1-7   característica (exponente en base 16, exceso 64)
//   bits 8-31  fracción de 24 bits (6 dígitos hexadecimales), normalizada y TRUNCADA
//
// La truncación es la que explica los valores raros del listado de la suite
// temporal de Cyberstride (marzo de 1972): un 25.4 tecleado en la fábrica se
// imprimía 25.39999 y un 11656.9 salía 11656.89844.

import (
	"fmt"
	"math"
)

// AHFP convierte un número a palabra HFP corta, truncando como el 360.
func AHFP(x float64) uint32 {
	if x == 0 || math.IsNaN(x) {
		return 0
	}
	var signo uint32
	if x < 0 {
		signo = 1 << 31
		x = -x
	}
	exp := 0
	for x >= 1 {
		x /= 16
		exp++
	}
	for x < 1.0/16 {
		x *= 16
		exp--
	}
	car := exp + 64
	if car > 127 {
		return signo | 0x7FFFFFFF
	}
	if car < 0 {
		return 0
	}
	frac := uint32(math.Floor(x * (1 << 24)))
	return signo | uint32(car)<<24 | frac
}

// DeHFP devuelve el valor representado por una palabra HFP corta.
func DeHFP(w uint32) float64 {
	frac := float64(w&0x00FFFFFF) / (1 << 24)
	car := int((w >> 24) & 0x7F)
	v := frac * math.Pow(16, float64(car-64))
	if w&(1<<31) != 0 {
		v = -v
	}
	return v
}

// Real4 hace el viaje de ida y vuelta por la memoria del 360.
func Real4(x float64) float64 { return DeHFP(AHFP(x)) }

// FormatoF imita un FORMAT(Fw.d) de Fortran IV sobre un REAL*4 del 360.
func FormatoF(x float64, w, d int) string {
	s := fmt.Sprintf("%*.*f", w, d, Real4(x))
	if len(s) > w {
		return stringsRepeat("*", w)
	}
	return s
}

// HexHFP muestra la palabra como la vería un volcado de memoria: "42196666".
func HexHFP(x float64) string { return fmt.Sprintf("%08X", AHFP(x)) }

func stringsRepeat(s string, n int) string {
	b := make([]byte, 0, n*len(s))
	for i := 0; i < n; i++ {
		b = append(b, s...)
	}
	return string(b)
}
