package motor

// Cybernet: la red de télex. Teleimpresores Siemens T-100 a 50 baudios,
// centrales LM Ericsson ARM/ARB, la central de tráfico de CORFO (40750) y el
// computador de ECOM (40506).

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Mensaje struct {
	ID         int      `json:"id"`
	De         string   `json:"de"`
	A          string   `json:"a"`
	DeNombre   string   `json:"deNombre"`
	ANombre    string   `json:"aNombre"`
	Texto      string   `json:"texto"`
	Caracteres int      `json:"caracteres"`
	Inicio     int64    `json:"inicio"`     // ms (reloj real del servidor)
	DuracionMs int      `json:"duracionMs"` // a la velocidad de esta transmisión
	Dia        int      `json:"dia"`
	Hora       string   `json:"hora"`
	Tipo       string   `json:"tipo"` // indices | reenvio | correccion | algedonico | verificacion | respuesta | directiva | libre
	Estado     string   `json:"estado"`
	Ruta       []string `json:"ruta"`
	Humano     bool     `json:"humano"`
	fabrica    string
	accion     *pedidoAccion
}

type pedidoAccion struct {
	senal  int
	accion string
	quien  string
}

// Directorio de abonados para el encabezado y la ruta.
func (m *Motor) abonado(numero string) (nombre, central string) {
	switch numero {
	case TelexCORFO:
		return "CORFO CL", "SCL"
	case TelexECOM:
		return "ECOM CL", "SCL"
	}
	for _, s := range Sectores {
		if s.Telex == numero {
			return s.Comite, "SCL"
		}
	}
	for _, f := range m.Fabricas {
		if f.Telex == numero {
			return abreviarRespuesta(f.ID) + " CL", f.Central
		}
	}
	return "", ""
}

func centralPorID(id string) *Central {
	for i := range Centrales {
		if Centrales[i].ID == id {
			return &Centrales[i]
		}
	}
	return nil
}

// ruta entre dos centrales subiendo por el árbol hasta Santiago.
func ruta(desde, hasta string) []string {
	subir := func(id string) []string {
		var r []string
		for id != "" {
			r = append(r, id)
			c := centralPorID(id)
			if c == nil {
				break
			}
			id = c.Padre
		}
		return r
	}
	a, b := subir(desde), subir(hasta)
	// quitar el tramo común
	for len(a) > 1 && len(b) > 1 && a[len(a)-1] == b[len(b)-1] && a[len(a)-2] == b[len(b)-2] {
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	out := append([]string{}, a...)
	for i := len(b) - 2; i >= 0; i-- {
		out = append(out, b[i])
	}
	if len(out) == 0 {
		out = []string{desde}
	}
	return out
}

// Encabezado como el del télex de ECOM de abril de 1973:
//
//	GA
//	40750
//	40506 ECOM CL
//	40750 CORFO CL
func (m *Motor) encabezado(de, a string) string {
	dn, _ := m.abonado(de)
	an, _ := m.abonado(a)
	return fmt.Sprintf("GA\n%s\n%s %s\n%s %s\n\n", a, de, dn, a, an)
}

// enviar pone un mensaje en la línea.
func (m *Motor) enviar(de, a, cuerpo, tipo string, humano bool) *Mensaje {
	texto := NormalizarTelex(m.encabezado(de, a) + cuerpo)
	dn, dc := m.abonado(de)
	an, ac := m.abonado(a)
	vel := m.Cfg.VelTelex * m.Cfg.Velocidad
	if vel < 1 {
		vel = 1
	}
	tr := len(CodificarITA2(texto))
	m.sigTelex++
	msg := &Mensaje{
		ID: m.sigTelex, De: de, A: a, DeNombre: dn, ANombre: an, Texto: texto,
		Caracteres: tr, Inicio: time.Now().UnixMilli(),
		DuracionMs: int(float64(DuracionTransmision(tr)) / vel),
		Dia: m.dia, Hora: m.reloj().Hora, Tipo: tipo, Estado: "transmitiendo",
		Ruta: ruta(dc, ac), Humano: humano,
	}
	m.Telex = append(m.Telex, msg)
	if len(m.Telex) > 600 {
		m.Telex = m.Telex[len(m.Telex)-600:]
	}
	m.emitir("telex", msg)
	return msg
}

// actualizarTelex entrega los mensajes cuya transmisión terminó.
func (m *Motor) actualizarTelex() {
	ahora := time.Now().UnixMilli()
	for _, t := range m.Telex {
		if t.Estado != "transmitiendo" || ahora < t.Inicio+int64(t.DuracionMs) {
			continue
		}
		t.Estado = "entregado"
		m.emitir("telexFin", map[string]any{"id": t.ID, "estado": t.Estado})
		m.alRecibir(t)
	}
}

func (m *Motor) alRecibir(t *Mensaje) {
	switch {
	case t.A == TelexCORFO && t.Tipo == "indices":
		if f := m.porID[t.fabrica]; f != nil {
			m.recibidos[f.ID] = append(m.recibidos[f.ID], t)
			f.Enviado = m.dia
			if fasesDia[m.fase].Nombre == "corfo" {
				// llegó una corrección durante la revisión
				m.revisarFabrica(f)
				m.revisarPendientesCORFO()
			}
			m.emitir("fabricas", m.Fabricas)
		}
	case t.accion != nil:
		m.ejecutarPedido(t.accion, t)
	}
}

// ---------- fábricas automáticas ----------

func (m *Motor) planificarTransmisiones() {
	m.porEnviar = map[string]float64{}
	m.planificadas = map[string]bool{}
	for _, f := range m.Fabricas {
		if f.Humano == "" {
			m.porEnviar[f.ID] = 0.03 + 0.55*m.rng.Float64()
			m.planificadas[f.ID] = true
		}
	}
}

func (m *Motor) transmitirAutomaticas() {
	// una fábrica que un equipo soltó a media tarde pasa a transmitir sola
	for _, f := range m.Fabricas {
		if f.Humano == "" && f.Enviado != m.dia && !m.planificadas[f.ID] {
			if m.planificadas == nil {
				m.planificadas = map[string]bool{}
			}
			m.planificadas[f.ID] = true
			m.porEnviar[f.ID] = m.progreso
		}
	}
	for id, umbral := range m.porEnviar {
		if m.progreso < umbral {
			continue
		}
		delete(m.porEnviar, id)
		f := m.porID[id]
		if f == nil || f.Humano != "" {
			continue
		}
		var b strings.Builder
		for _, ix := range f.Indices {
			v := FormatoValor(ix.HoyACT, ix.Decim)
			if m.rng.Float64() < m.Cfg.ErroresDigitacion {
				// error de la secretaria al pasar el formulario al teleimpresor
				v = errorDigitacion(m.rng, v)
				ev := &Evento{Fabrica: f.ID, Codigo: ix.Codigo, Tipo: "digitacion", Causa: "digitacion", Inicio: m.dia, Origen: "azar",
					Novedad: ""}
				m.agregarEvento(ev)
			}
			fmt.Fprintf(&b, "%s %s %12s\n", ix.Codigo, m.fecha.Format("020106"), v)
		}
		msg := m.enviar(f.Telex, TelexCORFO, b.String(), "indices", false)
		msg.fabrica = f.ID
	}
}

// ---------- télex de los equipos humanos ----------

// EnviarTelexHumano recibe lo que el equipo tecleó en su T-100.
func (m *Motor) EnviarTelexHumano(fabrica, destino, texto string) (*Mensaje, error) {
	m.mu.Lock()
	defer m.flush()
	f := m.porID[fabrica]
	if f == nil {
		return nil, fmt.Errorf("no existe la fábrica %s", fabrica)
	}
	if nombre, _ := m.abonado(destino); nombre == "" {
		return nil, fmt.Errorf("NP: el número %s no está en servicio", destino)
	}
	tipo := "libre"
	if destino == TelexCORFO && reTransaccion.MatchString(NormalizarTelex(texto)) {
		tipo = "indices"
		if fasesDia[m.fase].Nombre != "transmision" && fasesDia[m.fase].Nombre != "corfo" && fasesDia[m.fase].Nombre != "produccion" {
			return nil, fmt.Errorf("CORFO ya cerró la recepción de hoy: transmita mañana después de las 15:00")
		}
	}
	msg := m.enviar(f.Telex, destino, texto, tipo, true)
	msg.fabrica = f.ID
	if tipo == "indices" {
		delete(m.corregir, f.ID)
	}
	m.log("%s transmite un télex a %s (%d caracteres)", f.Corto, destino, msg.Caracteres)
	return msg, nil
}

// ---------- central de tráfico de CORFO ----------

// Formato del formulario ECOM: código de 22 letras, fecha DDMMAA, valor con punto.
var reTransaccion = regexp.MustCompile(`(?m)^\s*([A-Z]{20,24})\s+([0-9]{4,8})\s+([0-9.,\-]+)\s*$`)
var reCasiTransaccion = regexp.MustCompile(`(?m)^\s*([A-Z]{8,30})\s+\S+.*$`)

type problema struct {
	linea int
	texto string
}

// revisarTelex aplica el control de consistencia del manual de operaciones
// administrativas de ECOM a un télex de índices.
func (m *Motor) revisarTelex(f *Fabrica, texto string) (validas []string, problemas []problema) {
	vistos := map[string]bool{}
	cuerpo := texto
	// las líneas se cuentan desde el cuerpo, sin el encabezado GA / números / tambores
	if i := strings.Index(cuerpo, "\n\n"); i >= 0 && strings.HasPrefix(cuerpo, "GA\n") {
		cuerpo = cuerpo[i+2:]
	}
	lineas := strings.Split(cuerpo, "\n")
	for n, l := range lineas {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		sm := reTransaccion.FindStringSubmatch(l)
		if sm == nil {
			if reCasiTransaccion.MatchString(l) && strings.ContainsAny(l, "0123456789") && !strings.HasPrefix(l, "DE") && !strings.HasPrefix(l, "A ") && !strings.Contains(l, " CL") {
				problemas = append(problemas, problema{n + 1, "LINEA SIN EL FORMATO CODIGO FECHA VALOR"})
			}
			continue
		}
		codigo, fecha, valor := sm[1], sm[2], sm[3]
		ix := m.porCodigo[codigo]
		switch {
		case len(codigo) != 22:
			problemas = append(problemas, problema{n + 1, fmt.Sprintf("CODIGO %s NO TIENE 22 LETRAS", codigo)})
			continue
		case ix == nil:
			problemas = append(problemas, problema{n + 1, fmt.Sprintf("CODIGO %s NO EXISTE", codigo)})
			continue
		case ix.Fabrica != f.ID:
			problemas = append(problemas, problema{n + 1, fmt.Sprintf("CODIGO %s NO CORRESPONDE A ESTA EMPRESA", codigo)})
			continue
		case len(fecha) != 6:
			problemas = append(problemas, problema{n + 1, fmt.Sprintf("FECHA %s DEBE SER DDMMAA CON CEROS", fecha)})
			continue
		case fecha != m.fecha.Format("020106"):
			problemas = append(problemas, problema{n + 1, fmt.Sprintf("FECHA %s NO ES LA DE HOY (%s)", fecha, m.fecha.Format("020106"))})
			continue
		case strings.Contains(valor, ","):
			problemas = append(problemas, problema{n + 1, fmt.Sprintf("VALOR %s: USE PUNTO DECIMAL, NO COMA", valor)})
			continue
		case strings.Count(valor, ".") > 1 || strings.Contains(valor, "-"):
			problemas = append(problemas, problema{n + 1, fmt.Sprintf("VALOR %s NO ES NUMERICO", valor)})
			continue
		}
		if vistos[codigo] {
			continue
		}
		vistos[codigo] = true
		validas = append(validas, fmt.Sprintf("%s %s %12s", codigo, fecha, valor))
	}
	for _, ix := range f.Indices {
		if !vistos[ix.Codigo] {
			problemas = append(problemas, problema{0, fmt.Sprintf("FALTA EL INDICE %s (%s)", ix.Codigo, ix.Abrev)})
		}
	}
	return
}

func (m *Motor) revisarFabrica(f *Fabrica) {
	msgs := m.recibidos[f.ID]
	if len(msgs) == 0 {
		return
	}
	ultimo := msgs[len(msgs)-1]
	validas, problemas := m.revisarTelex(f, ultimo.Texto)
	if !m.Cfg.RevisaCORFO {
		// sin control de consistencia todo pasa tal cual a ECOM (y PRO10 se encarga)
		var crudas []string
		for _, l := range strings.Split(ultimo.Texto, "\n") {
			if sm := reCasiTransaccion.FindString(l); sm != "" && strings.ContainsAny(l, "0123456789") && len(strings.Fields(l)) == 3 {
				crudas = append(crudas, strings.TrimSpace(l))
			}
		}
		m.agregarReenvio(f, crudas)
		return
	}
	if len(problemas) > 0 && f.Humano != "" && m.corregir[f.ID] == false {
		var b strings.Builder
		fmt.Fprintf(&b, "DE:CORFO. CENTRAL DE TRAFICO.\nA :SR. INTERVENTOR %s.\n\nCONTROL DE CONSISTENCIA DEL TELEX DE INDICES DE HOY:\n", f.Corto)
		for _, p := range problemas {
			if p.linea > 0 {
				fmt.Fprintf(&b, "LINEA %d: %s\n", p.linea, p.texto)
			} else {
				fmt.Fprintf(&b, "%s\n", p.texto)
			}
		}
		b.WriteString("FAVOR CORREGIR Y RETRANSMITIR EL TELEX COMPLETO.\nATENTAMENTE\n")
		m.enviar(TelexCORFO, f.Telex, b.String(), "correccion", false)
		m.corregir[f.ID] = true
		m.log("CORFO pide a %s corregir su télex (%d problemas)", f.Corto, len(problemas))
		return
	}
	if len(problemas) > 0 {
		m.log("CORFO archiva el télex de %s con %d problemas; reenvía sólo las líneas válidas", f.Corto, len(problemas))
	}
	delete(m.corregir, f.ID)
	m.agregarReenvio(f, validas)
}

func (m *Motor) agregarReenvio(f *Fabrica, lineas []string) {
	// reemplaza lo que hubiera de esa fábrica (si retransmitió)
	var quedan []string
	for _, l := range m.reenvio {
		if ix := m.porCodigo[strings.Fields(l)[0]]; ix != nil && ix.Fabrica == f.ID {
			continue
		}
		quedan = append(quedan, l)
	}
	m.reenvio = append(quedan, lineas...)
}

// centralCORFO revisa todo lo recibido, lo archiva y lo reenvía a ECOM.
func (m *Motor) centralCORFO() {
	ids := make([]string, 0, len(m.recibidos))
	for id := range m.recibidos {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		m.revisarFabrica(m.porID[id])
	}
	for _, f := range m.Fabricas {
		if f.Enviado != m.dia {
			m.log("CORFO: %s no transmitió sus índices hoy", f.Corto)
		}
	}
	m.emitir("corfo", map[string]any{"lineas": len(m.reenvio), "corregir": len(m.corregir)})
	if len(m.corregir) == 0 || !m.Cfg.EsperarHumanos {
		m.reenviarAECOM()
	}
}

func (m *Motor) reenviarAECOM() {
	if m.reenviado {
		return
	}
	m.reenviado = true
	m.Ecom.Entrada = nil
	if len(m.reenvio) == 0 {
		return
	}
	sort.Strings(m.reenvio)
	cuerpo := fmt.Sprintf("DE:CORFO. CENTRAL DE TRAFICO.\nA :ECOM. CENTRO DE COMPUTO.\nINDICES DEL %s PARA CYBERSTRIDE. %d LINEAS.\n\n%s\nFIN\n",
		m.fechaTelex(), len(m.reenvio), strings.Join(m.reenvio, "\n"))
	m.enviar(TelexCORFO, TelexECOM, cuerpo, "reenvio", false)
	m.Ecom.Entrada = append([]string(nil), m.reenvio...)
	m.log("CORFO reenvía a ECOM %d transacciones", len(m.reenvio))
}

// CorregidoTodo se llama cuando la última fábrica corrigió durante la fase CORFO.
func (m *Motor) revisarPendientesCORFO() {
	if fasesDia[m.fase].Nombre == "corfo" && len(m.corregir) == 0 {
		m.reenviarAECOM()
	}
}
