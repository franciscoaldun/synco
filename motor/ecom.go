package motor

// ECOM: la Empresa Nacional de Computación e Informática y su IBM System/360
// modelo 50H (262.144 bytes de memoria principal, ciclo de 2,0 µs, discos 2314,
// lectora de cinta de papel 2671, lectora-perforadora de tarjetas, impresoras).
//
// Proceso normal diario según el "Manual de procesos computacionales TS-09"
// (Isaquino Benadof, ECOM, c. junio 1972):
//
//	DATOS (TR 01, 02, 03) → PRO10 (validación) → hook ASSEMBLER (código de
//	condición 8 si PRO10 detecta error, 0 si no) → PRO20 (análisis estadístico)
//	↔ archivos ÍNDICE, MAESTRO e HISTÓRICO → informes.
//
// El JCL de abajo copia el procedimiento en línea que se conserva en el
// archivo Beer (caja 57); con COND=(7,LT) el paso STATS no se ejecuta si
// VALID terminó con código 8.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type Listado struct {
	Titulo string   `json:"titulo"`
	Lineas []string `json:"lineas"` // registros FBA de 133: control ASA + 132 columnas
}

type ResultadoPRO20 struct {
	Codigo string            `json:"codigo"`
	ACT    float64           `json:"act"`
	P      float64           `json:"p"`
	L      float64           `json:"l"`
	PERF   float64           `json:"perf"`
	Prob   [NEstados]float64 `json:"prob"`
	Retro  [NEstados]float64 `json:"retro"`
	Clas   Clasificacion     `json:"clas"`
}

type Corrida struct {
	Dia        int              `json:"dia"`
	Fecha      string           `json:"fecha"`
	Juliana    string           `json:"juliana"`
	Cinta      []string         `json:"cinta"`
	Tarjetas   []Tarjeta        `json:"tarjetas"`
	JCL        []string         `json:"jcl"`
	Log        []string         `json:"log"`
	CCValid    int              `json:"ccValid"`
	Stats      bool             `json:"stats"`
	Mensajes   []string         `json:"mensajes"`
	Listados   []Listado        `json:"listados"`
	CPU        float64          `json:"cpu"`
	Resultados []ResultadoPRO20 `json:"resultados"`
}

type ECOM struct {
	Entrada    []string
	Pendientes []Tarjeta
	Ultima     *Corrida
	Previas    []ResumenCorrida
	seq        int
}

type ResumenCorrida struct {
	Dia     int     `json:"dia"`
	Fecha   string  `json:"fecha"`
	TR      int     `json:"tr"`
	CC      int     `json:"cc"`
	Stats   bool    `json:"stats"`
	CPU     float64 `json:"cpu"`
	Excepc  int     `json:"excepciones"`
}

func NuevoECOM() *ECOM { return &ECOM{} }

// JCL del procedimiento CYBER (las líneas marcadas son reconstrucción).
func JCL(juliana string) []string {
	return []string{
		"//CYBERDIA JOB (ECOM,CORFO),'CYBERSTRIDE',CLASS=A,MSGCLASS=A,     * RECONSTRUIDA",
		"// XXXXYYYYYY,  COND=(7,LT),",
		"//CYBER PROC",
		"//VALID EXEC  PGM=PRASMY01",
		"//STEPLIB DD DSN=USER.CYBER.TEMPORAL,UNIT=2314,DISP=(OLD,PASS),",
		"//  VOL=(PRIVATE,RETAIN,SER=BIER01).",
		"//SYSUDUMP DD SYSOUT=A",
		"//FT03F001 DD SYSOUT=A",
		"//FT06F001 DD SYSOUT=A,DCB=(RECFM=FBA,LRECL=133,BLKSIZE=665,",
		"//  BUFNO=1),SPACE=(TRK,(5,1))",
		"//FT10F001 DD SYSOUT=A,DCB=(RECFM=FBA,LRECL=133,BLKSIZE=665,",
		"//  BUFNO=1),SPACE=(TRK,(5,1))",
		"//FT08F001 DD DSN=USER.CYBER.INDEX,UNIT=2314,DISP=(OLD,PASS),",
		"//  VOL=(PRIVATE,RETAIN,SER=BIER01)",
		"//FT09F001 DD DSN=USER.CYBER.TRANFILE,UNIT=2314,DISP=(OLD,PASS),",
		"//  VOL=(PRIVATE,RETAIN,SER=BIER01)",
		"//STATS EXEC PGM=PRO20",
		"//STEPLIB DD DSN=*.VALID.STEPLIB,DISP=OLD",
		"//SYSUDUMP DD SYSOUT=A",
		"//FT03F001 DD SYSOUT=A",
		"//FT11F001 DD SYSOUT=A,DCB=(RECFM=FBA,LRECL=133,BLKSIZE=665,",
		"//  BUFNO=1),SPACE=(TRK,(5,1))",
		"//FT12F001 DD SYSOUT=A,DCB=(RECFM=FBA,LRECL=133,BLKSIZE=665,",
		"//  BUFNO=1),SPACE=(TRK,(5,1))",
		"//FT16F001 DD SYSOUT=A,DCB=(RECFM=FBA,LRECL=133,BLKSIZE=665,",
		"//  BUFNO=1),SPACE=(TRK,(5,1))",
		"//FT13F001 DD SYSOUT=A,DCB=(RECFM=FBA,LRECL=133,BLKSIZE=665,",
		"//  BUFNO=1),SPACE=(TRK,(5,1))",
		"//FT15F001 DD SYSOUT=A,DCB=(RECFM=FBA,LRECL=133,BLKSIZE=665,",
		"//  BUFNO=1),SPACE=(TRK,(5,1))",
		"//FT14F001 DD SYSOUT=A,DCB=(RECFM=FBA,LRECL=133,BLKSIZE=665,",
		"//  BUFNO=1),SPACE=(TRK,(5,1))",
		"//FT20F001 DD DSN=USER.CYBER.MASTER,UNIT=2314,DISP=(OLD,KEEP),   * RECONSTRUIDA",
		"//  VOL=(PRIVATE,RETAIN,SER=BIER01)",
		"//FT21F001 DD DSN=USER.CYBER.HISTORY,UNIT=2314,DISP=(MOD,KEEP),  * RECONSTRUIDA",
		"//  VOL=(PRIVATE,RETAIN,SER=BIER01)",
		"//      PEND",
		"//DIARIO EXEC CYBER                                               * RECONSTRUIDA",
		"//VALID.FT05F001 DD *                                              * TARJETAS TR 01-03",
	}
}

// CrearArchivos corresponde a PRO40 (creación de archivos, se corre una vez).
func (e *ECOM) CrearArchivos(m *Motor) {
	e.Ultima = &Corrida{Dia: 0, Fecha: m.fecha.Format("020106"), JCL: JCL(""),
		Log: []string{
			"PRO40 - CREACION DE ARCHIVOS CYBERSTRIDE",
			fmt.Sprintf("USER.CYBER.INDEX     %4d PROCESOS", len(m.porCodigo)),
			fmt.Sprintf("USER.CYBER.MASTER    %4d REGISTROS (ANALISIS PREVIO DE 65 DIAS HABILES)", len(m.porCodigo)),
			"USER.CYBER.HISTORY   CREADO",
			"PESOS A PRIORI DE LOS ESTADOS (DUNSMUIR, FEB-1972): 75.0  8.5  14.5  2.0",
		}}
}

// Correr ejecuta el JOB CYBERDIA del día.
func (e *ECOM) Correr(m *Motor) {
	c := &Corrida{Dia: m.dia, Fecha: m.fecha.Format("020106"), Juliana: m.reloj().Juliana, JCL: JCL(m.reloj().Juliana)}
	jul := c.Juliana
	c.Cinta = append([]string(nil), e.Entrada...)

	// Cinta de papel → lectora 2671 → tarjetas (la perforista respeta el orden TR 01, 02, 03)
	var tarjetas []Tarjeta
	for _, l := range c.Cinta {
		f := strings.Fields(l)
		if len(f) < 3 {
			f = append(f, "", "", "")
		}
		e.seq += 10
		tarjetas = append(tarjetas, NuevaTarjeta("01", f[0], f[1], f[2], e.seq))
	}
	for _, t := range e.Pendientes {
		e.seq += 10
		t.Secuencia = e.seq
		t = NuevaTarjeta(t.Tipo, t.Codigo, t.Fecha, t.Valor, e.seq)
		tarjetas = append(tarjetas, t)
	}
	e.Pendientes = nil
	sort.SliceStable(tarjetas, func(i, j int) bool { return tarjetas[i].Tipo < tarjetas[j].Tipo })
	c.Tarjetas = tarjetas

	// ---- paso VALID: PRASMY01 + PRO10 (MAIN, VALHD, VALID, INIT, CODEX)
	log := []string{
		fmt.Sprintf("IEF236I ALLOC. FOR CYBERDIA VALID"),
		"IEF237I 131  ALLOCATED TO STEPLIB",
		"IEF237I 131  ALLOCATED TO FT08F001",
		"IEF237I 131  ALLOCATED TO FT09F001",
	}
	var validas []Tarjeta
	for _, t := range tarjetas {
		if msg := m.validarTarjeta(t); msg != "" {
			c.Mensajes = append(c.Mensajes, fmt.Sprintf("TR %s SEC %05d  %-22s  RECHAZADA: %s", t.Tipo, t.Secuencia, t.Codigo, msg))
			continue
		}
		validas = append(validas, t)
	}
	if len(c.Mensajes) > 0 {
		c.CCValid = 8
	}
	cpuValid := 0.30 + 0.012*float64(len(tarjetas))
	log = append(log,
		fmt.Sprintf("IEF142I CYBERDIA VALID - STEP WAS EXECUTED - COND CODE %04d", c.CCValid),
		"IEF285I   USER.CYBER.TEMPORAL                          PASSED",
		"IEF285I   VOL SER NOS= BIER01.",
		"IEF285I   USER.CYBER.INDEX                             PASSED",
		"IEF285I   USER.CYBER.TRANFILE                          PASSED",
		fmt.Sprintf("IEF373I STEP /VALID   / START %s.1800", jul),
		fmt.Sprintf("IEF374I STEP /VALID   / STOP  %s.1801 CPU    0MIN %05.2fSEC MAIN  96K", jul, cpuValid),
	)
	c.Listados = append(c.Listados, m.listadoValidacion(c, validas))

	// ---- paso STATS: PRO20 (SETUP, STATS, IREAD, WRITE, TRANSF, REPORT)
	cpuStats := 0.0
	if c.CCValid > 7 {
		log = append(log, "IEF272I CYBERDIA STATS - STEP WAS NOT EXECUTED.")
		m.log("ECOM: PRO10 terminó con código 8; el paso STATS no se ejecutó (COND=(7,LT))")
		for _, ix := range m.porCodigo {
			ix.SinDatos++
		}
	} else {
		c.Stats = true
		procesados := map[string]bool{}
		var maestro []string
		for _, t := range validas {
			ix := m.porCodigo[t.Codigo]
			v, _ := strconv.ParseFloat(strings.TrimSpace(t.Valor), 64)
			switch t.Tipo {
			case "01":
				r := m.pro20(ix, v, t.Fecha)
				c.Resultados = append(c.Resultados, r)
				procesados[ix.Codigo] = true
				cpuStats += 0.05 // "1/20 seg de CPU en un IBM 360/50 por actualización de un modelo de cuatro estados"
			case "02":
				maestro = append(maestro, fmt.Sprintf(" %-22s  CAP   ANTERIOR %s   NUEVA %s", ix.Codigo, FormatoF(ix.CAP, 12, 5), FormatoF(v, 12, 5)))
				ix.CAP = v
			case "03":
				maestro = append(maestro, fmt.Sprintf(" %-22s  POT   ANTERIOR %s   NUEVA %s", ix.Codigo, FormatoF(ix.POT, 12, 5), FormatoF(v, 12, 5)))
				ix.POT = v
			}
		}
		for _, ix := range m.porCodigo {
			if procesados[ix.Codigo] {
				ix.SinDatos = 0
			} else {
				ix.SinDatos++
			}
		}
		cpuStats += 0.40 + 0.02*float64(len(m.porCodigo))
		log = append(log,
			"IEF236I ALLOC. FOR CYBERDIA STATS",
			fmt.Sprintf("IEF142I CYBERDIA STATS - STEP WAS EXECUTED - COND CODE 0000"),
			"IEF285I   USER.CYBER.MASTER                            KEPT",
			"IEF285I   USER.CYBER.HISTORY                           KEPT",
			"IEF285I   VOL SER NOS= BIER01.",
			fmt.Sprintf("IEF373I STEP /STATS   / START %s.1801", jul),
			fmt.Sprintf("IEF374I STEP /STATS   / STOP  %s.1803 CPU    0MIN %05.2fSEC MAIN 120K", jul, cpuStats),
		)
		c.Listados = append(c.Listados, m.listadosPRO20(c, maestro)...)
	}
	c.CPU = cpuValid + cpuStats
	log = append(log, fmt.Sprintf("IEF375I  JOB /CYBERDIA/ START %s.1800", jul),
		fmt.Sprintf("IEF376I  JOB /CYBERDIA/ STOP  %s.1803 CPU    0MIN %05.2fSEC", jul, c.CPU))
	c.Log = log
	e.Ultima = c
	exc := 0
	for _, r := range c.Resultados {
		if r.Clas.FueraBanda != 0 || r.Clas.EscalonAyer || r.Clas.TransAyer || r.Clas.Pendiente || r.Clas.MovMedia || r.Clas.Anomalia {
			exc++
		}
	}
	e.Previas = append(e.Previas, ResumenCorrida{c.Dia, c.Fecha, len(tarjetas), c.CCValid, c.Stats, c.CPU, exc})
	if len(e.Previas) > 40 {
		e.Previas = e.Previas[len(e.Previas)-40:]
	}
	m.log("ECOM corrió CYBERDIA: %d tarjetas, código %d, %d excepciones, CPU %.2f s", len(tarjetas), c.CCValid, exc, c.CPU)
	m.emitir("ecom", e.Vista())
	var res []ResumenIndice
	for _, f := range m.Fabricas {
		for _, ix := range f.Indices {
			res = append(res, m.resumenIndice(ix))
		}
	}
	tot, por := m.agregados()
	m.emitir("indices", map[string]any{"indices": res, "total": tot, "porSector": por})
}

// validarTarjeta replica lo que revisaba PRO10.
func (m *Motor) validarTarjeta(t Tarjeta) string {
	if t.Tipo != "01" && t.Tipo != "02" && t.Tipo != "03" {
		return "TIPO DE TRANSACCION INVALIDO"
	}
	if len(strings.TrimSpace(t.Codigo)) != 22 {
		return "CODIGO DE PROCESO NO TIENE 22 POSICIONES"
	}
	if m.porCodigo[t.Codigo] == nil {
		return "CODIGO NO ESTA EN USER.CYBER.INDEX"
	}
	if len(t.Fecha) != 6 {
		return "FECHA NO ES DDMMAA"
	}
	d, err1 := strconv.Atoi(t.Fecha[:2])
	mm, err2 := strconv.Atoi(t.Fecha[2:4])
	if err1 != nil || err2 != nil || d < 1 || d > 31 || mm < 1 || mm > 12 {
		return "FECHA INVALIDA"
	}
	v := strings.TrimSpace(t.Valor)
	if strings.Contains(v, ",") {
		return "COMA DECIMAL (DEBE SER PUNTO)"
	}
	if _, err := strconv.ParseFloat(v, 64); err != nil || v == "" {
		return "VALOR NO NUMERICO"
	}
	return ""
}

// pro20 procesa una transacción TR 01.
func (m *Motor) pro20(ix *Indice, valor float64, fecha string) ResultadoPRO20 {
	act := Real4(valor)
	p, l, perf := Indices(act, ix.CAP, ix.POT, ix.Inverso)
	mes := int(m.fecha.Month()) - 1
	s := ix.Estacional[mes]
	if s <= 0 {
		s = 1
	}
	pd := p / s
	ix.HS.Actualizar(pd)
	clas := Clasificar(ix.HS, pd, ix.Banda, ix.RefMedia, &ix.racha)
	reg := Registro{Dia: m.dia, Fecha: fecha, ACT: act, P: p, L: l, PERF: perf,
		Prob: ix.HS.Prob, Retro: ix.HS.Retro, Media: ix.HS.Media, Pend: ix.HS.Pendiente,
		Pron: ix.HS.SigPronSig, DesvPron: ix.HS.DesvPron, Clas: clas}
	ix.Historia = append(ix.Historia, reg)
	if len(ix.Historia) > 400 {
		ix.Historia = ix.Historia[len(ix.Historia)-400:]
	}
	return ResultadoPRO20{Codigo: ix.Codigo, ACT: act, P: p, L: l, PERF: perf, Prob: ix.HS.Prob, Retro: ix.HS.Retro, Clas: clas}
}

// ---------- listados de impresora (133 posiciones, FBA) ----------

func cabecera(pag int, programa, fecha string) []string {
	f := fmt.Sprintf("%s-%s-%s", fecha[:2], fecha[2:4], fecha[4:6])
	return []string{
		"1" + fmt.Sprintf("%-50s%-60s%s", "EMPRESA NACIONAL DE COMPUTACION E INFORMATICA", "SISTEMA CYBERSTRIDE - SUITE PERMANENTE", fmt.Sprintf("PAG. %3d", pag)),
		" " + fmt.Sprintf("%-50s%-60s%s", "ECOM - IBM 360/50H", programa, "FECHA "+f),
		"0",
	}
}

func (m *Motor) listadoValidacion(c *Corrida, validas []Tarjeta) Listado {
	l := cabecera(1, "PRO10 - VALIDACION DE TRANSACCIONES", c.Fecha)
	l = append(l, "0MENSAJES")
	if len(c.Mensajes) == 0 {
		l = append(l, "  NO SE DETECTARON ERRORES DE VALIDACION")
	}
	for _, s := range c.Mensajes {
		l = append(l, " "+s)
	}
	l = append(l, "0LISTA DE TRANSACCIONES VALIDAS", "0  SEC   TR  CODIGO DEL PROCESO        FECHA          VALOR")
	for _, t := range validas {
		v, _ := strconv.ParseFloat(strings.TrimSpace(t.Valor), 64)
		l = append(l, fmt.Sprintf(" %05d   %s  %-22s  %s  %s", t.Secuencia, t.Tipo, t.Codigo, t.Fecha, FormatoF(v, 12, 5)))
	}
	l = append(l, fmt.Sprintf("0TOTAL TARJETAS LEIDAS %4d   VALIDAS %4d   RECHAZADAS %4d   CODIGO DE CONDICION %d",
		len(c.Tarjetas), len(validas), len(c.Tarjetas)-len(validas), c.CCValid))
	return Listado{"PRO10 · Validación", l}
}

func (m *Motor) listadosPRO20(c *Corrida, maestro []string) []Listado {
	var out []Listado
	// Resumen de transacciones
	r := cabecera(2, "PRO20 - ANALISIS ESTADISTICO", c.Fecha)
	r = append(r, "0RESUMEN DE TRANSACCIONES",
		"0 CODIGO DEL PROCESO              ACT          P       L     PERF   P(1)  P(2)  P(3)  P(4)  PRONOSTICO  ESTADO")
	for _, x := range c.Resultados {
		est := NombreEstado[x.Clas.Estado]
		r = append(r, fmt.Sprintf("  %-22s %s %s %s %s  %5.3f %5.3f %5.3f %5.3f  %s  %s",
			x.Codigo, FormatoF(x.ACT, 12, 5), FormatoF(x.P, 7, 5), FormatoF(x.L, 7, 5), FormatoF(x.PERF, 7, 5),
			x.Prob[0], x.Prob[1], x.Prob[2], x.Prob[3], FormatoF(m.porCodigo[x.Codigo].HS.SigPronSig, 9, 5), est))
	}
	r = append(r, "0P(1) SIN CAMBIO  P(2) ESCALON  P(3) PENDIENTE  P(4) TRANSITORIO   (HARRISON-STEVENS, 4 ESTADOS)")
	out = append(out, Listado{"PRO20 · Resumen de transacciones", r})

	// Informe algedónico - taxonómico (sólo si hay anomalías)
	est := []string{}
	tax := []string{}
	for _, x := range c.Resultados {
		ix := m.porCodigo[x.Codigo]
		dir := direccionTelex(x.Clas.Direccion, ix.Inverso)
		switch {
		case x.Clas.TransAyer:
			est = append(est, fmt.Sprintf("  %-22s  TRANSITORIO EN EL DATO ANTERIOR (POSIBLE ERROR)   PROB. %5.3f", x.Codigo, x.Retro[EstTransitorio]))
		}
		if x.Clas.EscalonAyer {
			est = append(est, fmt.Sprintf("  %-22s  CAMBIO DE ESCALON  %-5s                          PROB. %5.3f", x.Codigo, dir, x.Retro[EstEscalon]))
		}
		if x.Clas.Pendiente {
			est = append(est, fmt.Sprintf("  %-22s  CAMBIO DE PENDIENTE %-5s  TENDENCIA %8.5f/DIA", x.Codigo, dir, ix.HS.Pendiente))
		}
		if x.Clas.Anomalia {
			est = append(est, fmt.Sprintf("  %-22s  DATO IMPROBABLE HOY - SE CLASIFICA MANANA      PROB. %5.3f", x.Codigo, x.Clas.ProbCambio))
		}
		if x.Clas.MovMedia {
			est = append(est, fmt.Sprintf("  %-22s  MOVIMIENTO SIGNIFICATIVO DE LA MEDIA  %-5s  MEDIA %7.5f ORIGINAL %7.5f", x.Codigo, dir, ix.HS.Media, ix.RefMedia))
		}
		if x.Clas.FueraBanda != 0 {
			tax = append(tax, fmt.Sprintf("  %-22s  P %s   BANDA %s %s %s   %s", x.Codigo, FormatoF(x.P, 7, 5),
				FormatoF(ix.Banda.Inf, 7, 5), FormatoF(ix.Banda.Med, 7, 5), FormatoF(ix.Banda.Sup, 7, 5), dir))
		}
	}
	if len(est)+len(tax) > 0 {
		a := cabecera(3, "INFORME ALGEDONICO - TAXONOMICO", c.Fecha)
		if len(est) > 0 {
			a = append(a, "0INFORME DE ANALISIS ESTADISTICO ANOMALO (*)")
			a = append(a, est...)
		}
		if len(tax) > 0 {
			a = append(a, "0INFORME DE ANALISIS TAXONOMICO ANOMALO (*)", "0 CODIGO DEL PROCESO         VALOR P      INF.    MED.    SUP.")
			a = append(a, tax...)
		}
		a = append(a, "0(*) SE IMPRIME SOLO EN CASO DE ANOMALIA")
		out = append(out, Listado{"Informe algedónico-taxonómico", a})
	}
	if len(maestro) > 0 {
		a := cabecera(4, "ACTUALIZACION DEL ARCHIVO MAESTRO (**)", c.Fecha)
		a = append(a, maestro...)
		a = append(a, "0(**) SE IMPRIME SOLO SI HAY TRANSACCIONES TIPO 02 Y 03")
		out = append(out, Listado{"Actualización del maestro", a})
	}
	return out
}

// direccionTelex: ALTO/BAJO se refiere al valor del índice, no a P.
func direccionTelex(dirP int, inverso bool) string {
	if inverso {
		dirP = -dirP
	}
	if dirP > 0 {
		return "ALTO"
	}
	if dirP < 0 {
		return "BAJO"
	}
	return "-"
}

// ---------- vistas del computador ----------

type VistaECOM struct {
	Ultima  *Corrida         `json:"ultima"`
	Previas []ResumenCorrida `json:"previas"`
	Memoria []BloqueMemoria  `json:"memoria"`
}

type BloqueMemoria struct {
	Desde  int    `json:"desde"`
	Hasta  int    `json:"hasta"`
	Nombre string `json:"nombre"`
}

// Mapa ilustrativo de los 256 KB del 360/50H bajo OS/360 MVT.
var mapaMemoria = []BloqueMemoria{
	{0, 24 * 1024, "Núcleo de OS/360 (supervisor, PSA)"},
	{24 * 1024, 64 * 1024, "Área de colas del sistema y rutinas residentes"},
	{64 * 1024, 120 * 1024, "Región de otros trabajos (contabilidad, nóminas)"},
	{120 * 1024, 240 * 1024, "Región CYBERDIA: PRO20 + archivo MAESTRO en memoria (120 K)"},
	{240 * 1024, 262144, "Lectora/escritora de colas (spool) e impresión"},
}

func (e *ECOM) Vista() VistaECOM {
	return VistaECOM{Ultima: e.Ultima, Previas: e.Previas, Memoria: mapaMemoria}
}

// ArchivoMaestro arma el contenido de USER.CYBER.MASTER para el volcado.
type RegistroMaestro struct {
	Codigo  string   `json:"codigo"`
	CAP     float64  `json:"cap"`
	POT     float64  `json:"pot"`
	Sysvar  []float64 `json:"sysvar"`
	Hex     []string `json:"hex"`
	EBCDIC  string   `json:"ebcdic"`
}

// SYSVAR (reconstrucción): 1-20 las cuatro componentes (m, b, vμμ, vμβ, vββ),
// 21-24 sus pesos, 25 varianza base, 26-28 banda, 29 media original, 30 N,
// 44-55 factores estacionales (documentado), 56 índice inverso (documentado).
func (m *Motor) ArchivoMaestro() []RegistroMaestro {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []RegistroMaestro
	for _, f := range m.Fabricas {
		for _, ix := range f.Indices {
			sv := make([]float64, 56)
			for j := 0; j < NEstados; j++ {
				c := ix.HS.Comps[j]
				sv[j*5+0], sv[j*5+1] = c.M[0], c.M[1]
				sv[j*5+2], sv[j*5+3], sv[j*5+4] = c.C[0][0], c.C[0][1], c.C[1][1]
				sv[20+j] = c.Peso
			}
			sv[24] = ix.HS.V
			sv[25], sv[26], sv[27] = ix.Banda.Inf, ix.Banda.Med, ix.Banda.Sup
			sv[28] = ix.RefMedia
			sv[29] = float64(ix.HS.N)
			for k := 0; k < 12; k++ {
				sv[43+k] = ix.Estacional[k]
			}
			if ix.Inverso {
				sv[55] = 1
			}
			hex := make([]string, len(sv))
			for i, v := range sv {
				hex[i] = HexHFP(v)
				sv[i] = Real4(v)
			}
			out = append(out, RegistroMaestro{ix.Codigo, Real4(ix.CAP), Real4(ix.POT), sv, hex, HexEBCDIC(ix.Codigo)})
		}
	}
	return out
}

// HistoricoCSV exporta el archivo histórico completo para analizarlo en clase.
func (m *Motor) HistoricoCSV() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	b.WriteString("codigo,fabrica,sector,dia,fecha,act,p,l,perf,prob_sin_cambio,prob_escalon,prob_pendiente,prob_transitorio,media,pendiente,pronostico,banda_inf,banda_sup,previo\n")
	for _, f := range m.Fabricas {
		for _, ix := range f.Indices {
			for _, h := range ix.Historia {
				fmt.Fprintf(&b, "%s,%s,%s,%d,%s,%s,%.5f,%.5f,%.5f,%.4f,%.4f,%.4f,%.4f,%.5f,%.6f,%.5f,%.5f,%.5f,%v\n",
					ix.Codigo, f.ID, f.Sector, h.Dia, h.Fecha, strconv.FormatFloat(h.ACT, 'f', -1, 64), h.P, h.L, h.PERF,
					h.Prob[0], h.Prob[1], h.Prob[2], h.Prob[3], h.Media, h.Pend, h.Pron, ix.Banda.Inf, ix.Banda.Sup, h.Previo)
			}
		}
	}
	return b.String()
}

var _ = math.Abs
