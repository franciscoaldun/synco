package motor

// Señales algedónicas (de algos, dolor, y hedone, placer) y su escalamiento por
// los niveles de recursión: si un nivel no corrige el problema en el plazo
// fijado, la señal sube al siguiente. Es el mecanismo que protegía la
// autonomía de las fábricas: el nivel superior sólo se entera cuando el de
// abajo no pudo.

import (
	"fmt"
	"strings"
)

var NombresNivel = [5]string{"", "EMPRESA", "COMITE SECTORIAL", "CORFO", "SALA DE OPERACIONES"}

type Senal struct {
	ID          int      `json:"id"`
	Codigo      string   `json:"codigo"`
	Fabrica     string   `json:"fabrica"`
	Sector      string   `json:"sector"`
	Abrev       string   `json:"abrev"`
	Tipo        string   `json:"tipo"` // ESCALON | PENDIENTE | FUERA DE BANDA | MOVIMIENTO DE MEDIA | SIN DATOS
	Direccion   string   `json:"direccion"`
	Nivel       int      `json:"nivel"`
	DiasNivel   int      `json:"diasNivel"`
	Inicio      int      `json:"inicio"`
	FechaInicio string   `json:"fechaInicio"`
	Estado      string   `json:"estado"` // activa | resuelta
	P           float64  `json:"p"`
	Banda       Banda    `json:"banda"`
	Causa       string   `json:"causa"` // conocida por quien preguntó
	Historial   []string `json:"historial"`
	Esperando   string   `json:"esperando"` // acción en curso (télex en línea)
	okSeguidos  int
	verifPedida bool
}

func (s *Senal) anotar(m *Motor, f string, a ...any) {
	s.Historial = append(s.Historial, fmt.Sprintf("%s %s · %s", m.fecha.Format("02-01"), m.reloj().Hora, fmt.Sprintf(f, a...)))
}

// Accion correctiva disponible para empresas, comités, CORFO y la Sala.
type Accion struct {
	ID       string   `json:"id"`
	Nombre   string   `json:"nombre"`
	Resuelve []string `json:"resuelve"`
	Telex    string   `json:"telex"`  // cuando decide un nivel superior
	Propia   string   `json:"propia"` // cuando la empresa informa lo que hizo
}

var Acciones = []Accion{
	{"verificar", "Verificar el dato y preguntar la causa", []string{"digitacion"}, "SE SOLICITA VERIFICAR EL DATO E INFORMAR LA CAUSA", "SE VERIFICO EL DATO EN LA PLANTA"},
	{"mantencion", "Enviar brigada de mantención", []string{"mantencion"}, "SE ENVIA BRIGADA DE MANTENCION", "LA BRIGADA DE MANTENCION DE LA PLANTA REPARA LA FALLA"},
	{"insumos", "Conseguir materias primas o autorizar importación", []string{"materia_prima"}, "SE ASIGNAN MATERIAS PRIMAS DE LA RESERVA", "SE CONSIGUEN MATERIAS PRIMAS CON OTROS PROVEEDORES"},
	{"transporte", "Asignar camiones de la red leal", []string{"transporte"}, "SE ASIGNAN CAMIONES PARA INSUMOS Y DESPACHOS", "SE CONSIGUEN CAMIONES PARA INSUMOS Y DESPACHOS"},
	{"personal", "Asamblea con los trabajadores y reorganizar turnos", []string{"ausentismo"}, "SE CONVOCA A ASAMBLEA Y SE REORGANIZAN TURNOS", "SE REALIZO ASAMBLEA Y SE REORGANIZARON LOS TURNOS"},
	{"demanda", "Redistribuir la producción", []string{"demanda"}, "SE REDISTRIBUYE LA PRODUCCION A OTRAS ZONAS", "SE BUSCAN NUEVOS DISTRIBUIDORES PARA LA PRODUCCION"},
	{"capacidad", "Revisar la capacidad (tarjeta TR 02)", []string{"mejora"}, "SE REVISA LA CAPACIDAD DEL PROCESO (TR 02)", "SE SOLICITA REVISAR LA CAPACIDAD DEL PROCESO (TR 02)"},
	{"autonomia", "Devolver a la empresa: respetar su autonomía", nil, "SE DEVUELVE EL PROBLEMA A LA EMPRESA CON PLAZO ADICIONAL", "LA EMPRESA PIDE PLAZO ADICIONAL"},
}

func accionPorID(id string) *Accion {
	for i := range Acciones {
		if Acciones[i].ID == id {
			return &Acciones[i]
		}
	}
	return nil
}

var accionPorCausa = map[string]string{
	"digitacion": "verificar", "mantencion": "mantencion", "materia_prima": "insumos",
	"transporte": "transporte", "ausentismo": "personal", "demanda": "demanda", "mejora": "capacidad",
}

var textoCausa = map[string]string{
	"digitacion": "ERROR DE TRANSCRIPCION DEL DATO", "mantencion": "FALLA DE MAQUINARIA",
	"materia_prima": "FALTA DE MATERIAS PRIMAS", "transporte": "PARO DEL TRANSPORTE",
	"ausentismo": "AUSENTISMO", "demanda": "BAJA DEMANDA", "mejora": "MEJORA EN LA PRODUCCION",
}

func (m *Motor) senalActiva(codigo string) *Senal {
	for _, s := range m.Senales {
		if s.Codigo == codigo && s.Estado == "activa" {
			return s
		}
	}
	return nil
}

func (m *Motor) SenalPorID(id int) *Senal {
	for _, s := range m.Senales {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// evaluarSenales corre después de PRO20: crea, sostiene, escala o cierra señales
// y manda los télex de ECOM, como el del 3 al 6 de abril de 1973.
func (m *Motor) evaluarSenales() {
	if m.Ecom.Ultima == nil || m.Ecom.Ultima.Dia != m.dia {
		return
	}
	porNivel := map[int][]lineaInforme{}
	verificar := map[string][]string{}
	nuevas := 0

	res := map[string]ResultadoPRO20{}
	for _, r := range m.Ecom.Ultima.Resultados {
		res[r.Codigo] = r
	}
	for _, f := range m.Fabricas {
		for _, ix := range f.Indices {
			r, ok := res[ix.Codigo]
			s := m.senalActiva(ix.Codigo)
			if !ok {
				// sin transacción hoy
				if ix.SinDatos >= 2 && s == nil {
					s = m.nuevaSenal(ix, "SIN DATOS", "-", 0)
					nuevas++
				}
				if s != nil {
					m.envejecer(s)
				}
				continue
			}
			c := r.Clas
			if c.TransAyer {
				verificar[f.ID] = append(verificar[f.ID], fmt.Sprintf("%s DEL DIA ANTERIOR", ix.Abrev))
			}
			condicion := c.FueraBanda != 0 || c.MovMedia || c.Pendiente || c.EscalonAyer
			if s != nil && s.Tipo == "SIN DATOS" {
				m.cerrar(s, "llegaron los datos")
				s = nil
			}
			switch {
			case s == nil && condicion:
				tipo := "FUERA DE BANDA"
				switch {
				case c.Pendiente:
					tipo = "PENDIENTE"
				case c.EscalonAyer:
					tipo = "ESCALON"
				case c.MovMedia && c.FueraBanda == 0:
					tipo = "MOVIMIENTO DE MEDIA"
				}
				s = m.nuevaSenal(ix, tipo, direccionTelex(c.Direccion, ix.Inverso), r.P)
				nuevas++
				porNivel[1] = append(porNivel[1], lineaInforme{f.ID, m.lineaSenal(s)})
			case s != nil && condicion:
				s.okSeguidos = 0
				s.P = r.P
				if c.Direccion != 0 {
					s.Direccion = direccionTelex(c.Direccion, ix.Inverso)
				}
				if m.envejecer(s) {
					porNivel[s.Nivel] = append(porNivel[s.Nivel], lineaInforme{f.ID, m.lineaSenal(s)})
				}
			case s != nil && !condicion:
				s.okSeguidos++
				s.P = r.P
				if s.okSeguidos >= 2 {
					m.cerrar(s, "el índice volvió a su banda")
				}
			}
		}
	}
	m.telexInformes(porNivel, verificar)
	if nuevas > 0 {
		m.log("%d señales algedónicas nuevas", nuevas)
	}
	m.emitir("senales", m.senalesVista())
}

func (m *Motor) nuevaSenal(ix *Indice, tipo, dir string, p float64) *Senal {
	m.sigSenal++
	s := &Senal{ID: m.sigSenal, Codigo: ix.Codigo, Fabrica: ix.Fabrica, Sector: ix.Sector, Abrev: ix.Abrev,
		Tipo: tipo, Direccion: dir, Nivel: 1, Inicio: m.dia, FechaInicio: m.fechaTelex(), Estado: "activa",
		P: p, Banda: ix.Banda}
	s.anotar(m, "Cyberstride detecta %s %s en %s", tipo, dir, ix.Abrev)
	m.Senales = append(m.Senales, s)
	if len(m.Senales) > 300 {
		m.Senales = m.Senales[len(m.Senales)-300:]
	}
	return s
}

// envejecer suma un día y escala si se cumplió el plazo. Devuelve true si escaló.
func (m *Motor) envejecer(s *Senal) bool {
	s.DiasNivel++
	if s.Nivel < 4 && s.DiasNivel > m.Cfg.VentanaDias {
		s.Nivel++
		s.DiasNivel = 0
		s.anotar(m, "sin corrección en %d días: escala a %s", m.Cfg.VentanaDias, NombresNivel[s.Nivel])
		m.log("Señal %s de %s escala a %s", s.Abrev, m.porID[s.Fabrica].Corto, NombresNivel[s.Nivel])
		return true
	}
	return false
}

func (m *Motor) cerrar(s *Senal, motivo string) {
	s.Estado = "resuelta"
	s.anotar(m, "resuelta: %s", motivo)
	m.emitir("senal", s)
}

func (m *Motor) lineaSenal(s *Senal) string {
	f := m.porID[s.Fabrica]
	puntos := 26 - len(s.Abrev)
	if puntos < 3 {
		puntos = 3
	}
	return fmt.Sprintf("   AL %s:  %s:\n%s%s%s", m.fechaTelex(), f.Corto, s.Abrev, strings.Repeat(".", puntos), s.Direccion)
}

// telexInformes manda los avisos del día a cada nivel.
type lineaInforme struct{ fabrica, texto string }

func (m *Motor) telexInformes(porNivel map[int][]lineaInforme, verificar map[string][]string) {
	// nivel 1: a cada empresa
	porFab := map[string][]string{}
	for _, l := range porNivel[1] {
		porFab[l.fabrica] = append(porFab[l.fabrica], l.texto)
	}
	for _, f := range m.Fabricas {
		lineas := porFab[f.ID]
		ver := verificar[f.ID]
		if len(lineas) == 0 && len(ver) == 0 {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "DE:ECOM.\nA :SR. INTERVENTOR %s.\n\n", f.Corto)
		if len(lineas) > 0 {
			b.WriteString("   SE HAN DETECTADO LAS SIGUIENTES SENALES ALGEDONICAS:\n\n")
			b.WriteString(strings.Join(lineas, "\n\n"))
			fmt.Fprintf(&b, "\n\nSI NO SE CORRIGEN EN %d DIAS SE INFORMARA AL COMITE.\n", m.Cfg.VentanaDias)
		}
		for _, v := range ver {
			fmt.Fprintf(&b, "\nSE SOLICITA VERIFICAR EL DATO %s (TRANSITORIO).\n", v)
		}
		b.WriteString("\n   ATENTAMENTE\n")
		m.enviar(TelexECOM, f.Telex, b.String(), "algedonico", false)
	}
	// nivel 2: a cada comité sectorial
	porSec := map[string][]string{}
	for _, l := range porNivel[2] {
		sec := m.porID[l.fabrica].Sector
		porSec[sec] = append(porSec[sec], l.texto)
	}
	for _, sec := range Sectores {
		lineas := porSec[sec.ID]
		if len(lineas) == 0 {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "DE:ECOM.\nA :SR. SECRETARIO EJECUTIVO %s.\n\n   SE HAN DETECTADO LAS SIGUIENTES SENALES ALGEDONICAS:\n\n%s\n\n   ATENTAMENTE\n",
			sec.Comite, strings.Join(lineas, "\n\n"))
		m.enviar(TelexECOM, sec.Telex, b.String(), "algedonico", false)
	}
	// niveles 3 y 4: a CORFO, con copia a la Sala de Operaciones
	var altos []string
	for _, n := range []int{3, 4} {
		for _, l := range porNivel[n] {
			altos = append(altos, l.texto+fmt.Sprintf("   (%s)", NombresNivel[n]))
		}
	}
	if len(altos) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "DE:ECOM.\nA :SR. ROBERTO CANETE-SALA DE OPERACIONES.\n\nSE HAN DETECTADO LOS SIGUIENTES INFORMES ALGEDONICOS A NIVEL CORFO:\nDEL %s:\n\n%s\n\nINFORMES ENVIADOS A LAS DIFERENTES EMPRESAS Y COMITES.\n\nC/C A DON RAUL ESPEJO.\n",
			m.fechaTelex(), strings.Join(altos, "\n\n"))
		m.enviar(TelexECOM, TelexCORFO, b.String(), "algedonico", false)
	}
}

// ---------- decisiones ----------

// actuanNivelesAutomaticos: cada mañana, interventores, comités y CORFO
// automáticos intentan corregir lo que está en su nivel.
func (m *Motor) actuanNivelesAutomaticos() {
	for _, s := range m.Senales {
		if s.Estado != "activa" || s.Esperando != "" {
			continue
		}
		f := m.porID[s.Fabrica]
		if f == nil {
			continue
		}
		switch s.Nivel {
		case 1:
			if f.Humano != "" {
				continue
			}
		case 4:
			if !m.Cfg.SalaAutomatica && m.Sala.HayPersonas() {
				continue
			}
		}
		prob := m.Cfg.Autonomia
		if s.Nivel > 1 {
			prob *= 0.8
		}
		if m.rng.Float64() >= prob {
			continue
		}
		// el nivel automático conoce (o averigua) la causa
		accion := "verificar"
		if ev := m.eventoDe(s); ev != nil {
			accion = accionPorCausa[ev.Causa]
		}
		if s.Tipo == "SIN DATOS" {
			accion = "verificar"
		}
		quien := NombresNivel[s.Nivel]
		switch s.Nivel {
		case 1:
			quien = "INTERVENTOR " + f.Corto
		case 2:
			quien = "SECRETARIO EJECUTIVO " + SectorPorID(f.Sector).Comite
		}
		m.pedirAccion(s, accion, quien, false)
	}
}

// eventoDe busca el imprevisto activo detrás de una señal.
func (m *Motor) eventoDe(s *Senal) *Evento {
	var mejor *Evento
	for _, ev := range m.Eventos {
		if ev.Codigo == s.Codigo && ev.activoEn(m.dia) && !ev.Resuelto && ev.Tipo != "transitorio" {
			mejor = ev
		}
	}
	return mejor
}

// pedirAccion manda por télex la decisión y la ejecuta al llegar.
func (m *Motor) pedirAccion(s *Senal, accion, quien string, humano bool) error {
	a := accionPorID(accion)
	if a == nil {
		return fmt.Errorf("acción desconocida %s", accion)
	}
	f := m.porID[s.Fabrica]
	if accion == "transporte" && m.Esc.Camiones > 0 {
		if m.Camiones < 25 {
			return fmt.Errorf("no quedan camiones disponibles (hay %d, se necesitan 25)", m.Camiones)
		}
		m.Camiones -= 25
		m.camionesFuera = append(m.camionesFuera, m.dia+4)
		m.emitir("camiones", m.Camiones)
	}
	de := TelexCORFO
	switch s.Nivel {
	case 1:
		de = f.Telex
	case 2:
		de = SectorPorID(f.Sector).Telex
	}
	para := f.Telex
	if de == f.Telex {
		para = TelexCORFO
	}
	cuerpo := fmt.Sprintf("DE:%s.\nA :%s.\nREF. SENAL ALGEDONICA %s (%s %s).\n\n%s.\n\nATENTAMENTE\n",
		quien, map[bool]string{true: "CORFO. CENTRAL DE TRAFICO", false: "SR. INTERVENTOR " + f.Corto}[para == TelexCORFO],
		s.Abrev, s.Tipo, s.Direccion, map[bool]string{true: a.Propia, false: a.Telex}[de == f.Telex])
	msg := m.enviar(de, para, cuerpo, "directiva", humano)
	msg.accion = &pedidoAccion{senal: s.ID, accion: accion, quien: quien}
	s.Esperando = a.Nombre
	s.anotar(m, "%s decide: %s", quien, strings.ToLower(a.Nombre))
	m.emitir("senal", s)
	return nil
}

// ejecutarPedido aplica la acción cuando el télex llega a destino.
func (m *Motor) ejecutarPedido(p *pedidoAccion, t *Mensaje) {
	s := m.SenalPorID(p.senal)
	if s == nil {
		return
	}
	s.Esperando = ""
	a := accionPorID(p.accion)
	f := m.porID[s.Fabrica]
	switch p.accion {
	case "autonomia":
		s.Nivel = 1
		s.DiasNivel = -1 // un día extra de plazo
		s.anotar(m, "vuelve a la empresa con plazo adicional")
		m.emitir("senal", s)
		return
	case "verificar":
		if ev := m.eventoDe(s); ev != nil {
			s.Causa = textoCausa[ev.Causa]
			s.anotar(m, "la empresa informa la causa: %s", strings.ToLower(s.Causa))
		} else {
			// ¿fue un error de transcripción?
			resuelto := false
			for _, ev := range m.Eventos {
				if ev.Codigo == s.Codigo && ev.Tipo == "digitacion" && !ev.Resuelto && m.dia-ev.Inicio <= 2 {
					ev.Resuelto, ev.FinDia = true, m.dia
					resuelto = true
				}
			}
			if resuelto {
				s.Causa = textoCausa["digitacion"]
				s.anotar(m, "era un error de transcripción: el dato se corrige")
			} else {
				s.Causa = "SIN CAUSA APARENTE"
				s.anotar(m, "la empresa no encuentra una causa: puede ser una variación normal")
			}
		}
		if f != nil && f.Humano == "" {
			m.enviar(f.Telex, TelexCORFO, fmt.Sprintf("DE:INTERVENTOR %s.\nA :CORFO.\nREF. %s: CAUSA %s.\n", f.Corto, s.Abrev, s.Causa), "respuesta", false)
		}
		m.emitir("senal", s)
		return
	}
	ev := m.eventoDe(s)
	if ev == nil || !contiene(a.Resuelve, ev.Causa) {
		s.anotar(m, "%s: sin efecto (la causa era otra)", strings.ToLower(a.Nombre))
		m.emitir("senal", s)
		return
	}
	ev.Resuelto = true
	ev.FinDia = m.dia + 1
	s.Causa = textoCausa[ev.Causa]
	if ev.Causa == "mejora" {
		// la mejora se consolida como nueva capacidad: tarjeta TR 02 para mañana
		if ix := m.porCodigo[s.Codigo]; ix != nil {
			f := (ix.Base + ev.Magnitud) / ix.Base
			nueva := ix.CAP * f
			if ix.Inverso {
				nueva = ix.CAP / f // en ausentismo o consumos la capacidad es el mínimo alcanzable
			}
			fecha := m.fecha.Format("020106")
			m.Ecom.Pendientes = append(m.Ecom.Pendientes, NuevaTarjeta("02", ix.Codigo, fecha, FormatoValor(nueva, ix.Decim), 0))
			ix.CAP = nueva
			// la capacidad no puede superar a la potencialidad: se revisa también POT (TR 03)
			if (!ix.Inverso && ix.CAP > ix.POT) || (ix.Inverso && ix.CAP < ix.POT) {
				pot := ix.CAP * 1.05
				if ix.Inverso {
					pot = ix.CAP / 1.05
				}
				m.Ecom.Pendientes = append(m.Ecom.Pendientes, NuevaTarjeta("03", ix.Codigo, fecha, FormatoValor(pot, ix.Decim), 0))
				ix.POT = pot
			}
		}
	}
	s.anotar(m, "%s: corrige la causa (%s)", strings.ToLower(a.Nombre), strings.ToLower(s.Causa))
	m.log("%s corrige %s en %s", p.quien, strings.ToLower(s.Causa), f.Corto)
	m.emitir("senal", s)
}

func contiene(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// Decidir es la entrada pública: un equipo de fábrica, una silla de la Sala o
// el docente toman una acción sobre una señal.
func (m *Motor) Decidir(senal int, accion, quien string, fabricaHumana string) error {
	m.mu.Lock()
	defer m.flush()
	s := m.SenalPorID(senal)
	if s == nil || s.Estado != "activa" {
		return fmt.Errorf("la señal %d no está activa", senal)
	}
	if s.Esperando != "" {
		return fmt.Errorf("ya hay una decisión en camino: %s", s.Esperando)
	}
	if fabricaHumana != "" {
		if s.Fabrica != fabricaHumana {
			return fmt.Errorf("esa señal no es de su empresa")
		}
		if s.Nivel > 1 {
			// la empresa puede seguir actuando aunque la señal haya escalado
			quien = "INTERVENTOR " + m.porID[fabricaHumana].Corto
		} else {
			quien = "INTERVENTOR " + m.porID[fabricaHumana].Corto
		}
	}
	return m.pedirAccion(s, accion, quien, true)
}

// LlevarALaSala: el docente sube una señal directamente a la Sala.
func (m *Motor) LlevarALaSala(senal int) error {
	m.mu.Lock()
	defer m.flush()
	s := m.SenalPorID(senal)
	if s == nil {
		return fmt.Errorf("no existe la señal %d", senal)
	}
	s.Nivel = 4
	s.DiasNivel = 0
	s.anotar(m, "el docente la lleva a la Sala de Operaciones")
	m.emitir("senal", s)
	return nil
}
