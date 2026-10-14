package dynamo

import (
	"embed"
	"sort"
	"strings"
)

//go:embed modelos/*.dyn
var archivos embed.FS

type Fuente struct {
	ID     string `json:"id"`
	Titulo string `json:"titulo"`
	Texto  string `json:"texto"`
}

// Modelos incluidos en el emulador.
func Modelos() []Fuente {
	ents, _ := archivos.ReadDir("modelos")
	var out []Fuente
	for _, e := range ents {
		b, _ := archivos.ReadFile("modelos/" + e.Name())
		txt := string(b)
		titulo := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(txt, "\n", 2)[0], "*"))
		out = append(out, Fuente{strings.TrimSuffix(e.Name(), ".dyn"), titulo, txt})
	}
	orden := map[string]int{"checo": 0, "futuro": 1, "taller": 2}
	sort.Slice(out, func(i, j int) bool { return orden[out[i].ID] < orden[out[j].ID] })
	return out
}
