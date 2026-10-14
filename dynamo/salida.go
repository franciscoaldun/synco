package dynamo

// Salidas al estilo de la impresora de línea: el gráfico de DYNAMO con el
// tiempo hacia abajo y una letra por variable (así se ve el gráfico del
// "Simulation Model of the Chilean Economy", CHECO, septiembre de 1972).

import (
	"fmt"
	"math"
	"strings"
)

const anchoPlot = 101

func bonito(x float64) float64 {
	if x <= 0 {
		return 1
	}
	e := math.Pow(10, math.Floor(math.Log10(x)))
	f := x / e
	switch {
	case f <= 1:
		f = 1
	case f <= 2:
		f = 2
	case f <= 2.5:
		f = 2.5
	case f <= 5:
		f = 5
	default:
		f = 10
	}
	return f * e
}

// EscalaGrupo calcula los límites redondos de un grupo de variables.
func EscalaGrupo(g GrupoPlot, series map[string][]float64) (float64, float64) {
	if g.Fijo {
		return g.Min, g.Max
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range g.Vars {
		for _, x := range series[v.Nombre] {
			lo = math.Min(lo, x)
			hi = math.Max(hi, x)
		}
	}
	if math.IsInf(lo, 0) {
		return 0, 1
	}
	if lo > 0 && lo < hi*0.5 {
		lo = 0
	}
	if hi == lo {
		hi = lo + 1
	}
	paso := bonito((hi - lo) / 4)
	lo = math.Floor(lo/paso) * paso
	hi = lo + 4*paso
	for hi < math.Max(hi, 0) {
		hi += paso
	}
	maxSerie := math.Inf(-1)
	for _, v := range g.Vars {
		for _, x := range series[v.Nombre] {
			maxSerie = math.Max(maxSerie, x)
		}
	}
	for hi < maxSerie {
		paso = bonito(paso * 1.5)
		lo = math.Floor(lo/paso) * paso
		hi = lo + 4*paso
	}
	return lo, hi
}

func numCorto(x float64) string {
	ax := math.Abs(x)
	switch {
	case x == 0:
		return "0."
	case ax >= 1e6 || ax < 1e-3:
		return strings.ReplaceAll(fmt.Sprintf("%.2E", x), "E+0", "E")
	case ax >= 100:
		return fmt.Sprintf("%.0f.", x)
	case ax >= 1:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", x), "0"), ".") + map[bool]string{true: ".", false: ""}[math.Mod(x, 1) == 0]
	}
	return strings.TrimLeft(fmt.Sprintf("%.3f", x), "0")
}

func textoPlot(r *Resultado) string {
	grupos := r.Plot
	if len(grupos) == 0 {
		return ""
	}
	var b strings.Builder
	titulo := r.Titulo
	if titulo == "" {
		titulo = "MODELO"
	}
	fmt.Fprintf(&b, "PAGE 1   %s\n\n", strings.ToUpper(titulo))
	for _, g := range grupos {
		var nombres []string
		for _, v := range g.Vars {
			nombres = append(nombres, v.Nombre+"="+v.Simbolo)
		}
		fmt.Fprintf(&b, "         %s\n", strings.Join(nombres, ","))
	}
	type esc struct{ lo, hi float64 }
	escalas := make([]esc, len(grupos))
	for i, g := range grupos {
		lo, hi := EscalaGrupo(g, r.Series)
		escalas[i] = esc{lo, hi}
		linea := []rune(strings.Repeat(" ", anchoPlot+12))
		for k := 0; k <= 4; k++ {
			s := numCorto(lo + (hi-lo)*float64(k)/4)
			col := 8 + k*25 - len(s)/2
			if k == 4 {
				col = 8 + 100 - len(s) + 1
			}
			for j, ch := range s {
				if col+j >= 0 && col+j < len(linea) {
					linea[col+j] = ch
				}
			}
		}
		var sims []string
		for _, v := range g.Vars {
			sims = append(sims, v.Simbolo)
		}
		b.WriteString(strings.TrimRight(string(linea), " ") + "   " + strings.Join(sims, "") + "\n")
	}
	b.WriteString(" TIME\n")
	pltper := r.Spec["PLTPER"]
	prtper := r.Spec["PRTPER"]
	dt := r.Spec["DT"]
	paso := int(math.Max(1, math.Round(pltper/dt)))
	pasoEtiqueta := int(math.Max(1, math.Round(prtper/dt)))
	for k := 0; k < len(r.Tiempos); k += paso {
		fila := []rune(strings.Repeat(" ", anchoPlot))
		marca := k%pasoEtiqueta == 0
		for c := 0; c < anchoPlot; c += 25 {
			fila[c] = '.'
		}
		if k == 0 {
			for c := range fila {
				fila[c] = '-'
			}
		}
		var choques []string
		for gi, g := range grupos {
			lo, hi := escalas[gi].lo, escalas[gi].hi
			for _, v := range g.Vars {
				s := r.Series[v.Nombre]
				if k >= len(s) {
					continue
				}
				pos := int(math.Round((s[k] - lo) / (hi - lo) * float64(anchoPlot-1)))
				if pos < 0 {
					pos = 0
				}
				if pos >= anchoPlot {
					pos = anchoPlot - 1
				}
				ch := []rune(v.Simbolo)[0]
				if fila[pos] != ' ' && fila[pos] != '.' && fila[pos] != '-' {
					choques = append(choques, string(fila[pos])+string(ch))
					continue
				}
				fila[pos] = ch
			}
		}
		etiqueta := "        "
		if marca {
			etiqueta = fmt.Sprintf("%7s ", numCorto(r.Tiempos[k]))
		}
		linea := etiqueta + string(fila)
		if len(choques) > 0 {
			linea += "  " + strings.Join(choques, ",")
		}
		b.WriteString(strings.TrimRight(linea, " ") + "\n")
	}
	return b.String()
}

func textoPrint(r *Resultado) string {
	vars := r.Print
	if len(vars) == 0 {
		for _, g := range r.Plot {
			for _, v := range g.Vars {
				vars = append(vars, v.Nombre)
			}
		}
	}
	if len(vars) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%10s", "TIME")
	for _, v := range vars {
		fmt.Fprintf(&b, "%12s", v)
	}
	b.WriteString("\n")
	paso := int(math.Max(1, math.Round(r.Spec["PRTPER"]/r.Spec["DT"])))
	for k := 0; k < len(r.Tiempos); k += paso {
		fmt.Fprintf(&b, "%10.3f", r.Tiempos[k])
		for _, v := range vars {
			s := r.Series[v]
			if k < len(s) {
				fmt.Fprintf(&b, "%12.4g", s[k])
			} else {
				fmt.Fprintf(&b, "%12s", "?")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
