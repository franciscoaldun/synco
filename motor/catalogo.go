package motor

// Catálogo de sectores, empresas e índices.
//
// Las empresas son reales del Área de Propiedad Social (1971-1973). Cinco de
// los códigos de índice son los del listado de la suite temporal de Cyberstride
// (ECOM, 21-01-1972): CIMMELONPRODUCCCLINKER, FORINFORSAEFICPORSULFI,
// TEXFABRILANAUSENTISMOO, TEXPROGRESOEFICTELARES y TEXSAIDMAIPEXISTERAYON.
// Las abreviaturas EFIC.HIL.CARDADA (Fabrilana) y EFIC.EST.GLOBAL (Sumar Nylon)
// salen del télex que ECOM mandó a la Sala de Operaciones en abril de 1973.
// El resto de los códigos sigue la misma regla (22 letras: 3 del sector, la
// planta y el proceso), pero son reconstrucciones, igual que todas las cifras
// de capacidad y producción, que son simuladas.

import "fmt"

type Sector struct {
	ID     string `json:"id"`     // CIM, TEX, ...
	Nombre string `json:"nombre"` // como en la pantalla Staffy de la Sala
	Comite string `json:"comite"` // comité sectorial de CORFO que recibía los télex
	Telex  string `json:"telex"`
	Color  string `json:"color"`
}

var Sectores = []*Sector{
	{"CIM", "Materiales de construcción", "COMITE MATERIALES DE CONSTRUCCION", "40761", "#c8a96e"},
	{"CAU", "Caucho", "COMITE DEL CAUCHO", "40762", "#8c8c8c"},
	{"TEX", "Textil", "COMITE TEXTIL", "40763", "#e0533d"},
	{"ELE", "Electrónica", "COMITE ELECTRONICO", "40764", "#4c9be8"},
	{"FOR", "Forestal", "COMITE FORESTAL", "40765", "#4caf6a"},
}

// DefIndice es la definición de un proceso del archivo maestro.
type DefIndice struct {
	Codigo  string  `json:"codigo"`  // 22 letras
	Nombre  string  `json:"nombre"`  // nombre largo
	Abrev   string  `json:"abrev"`   // como se imprimía en los télex de ECOM: EFIC.HIL.CARDADA
	Unidad  string  `json:"unidad"`
	CAP     float64 `json:"cap"`     // capacidad
	POT     float64 `json:"pot"`     // potencialidad
	Inverso bool    `json:"inverso"` // SYSVAR(56): más es peor (ausentismo, consumos)
	Base    float64 `json:"-"`       // eficiencia típica (ACT/CAP, o CAP/ACT si es inverso)
	Ruido   float64 `json:"-"`       // desviación diaria de esa eficiencia
	Decim   int     `json:"decim"`   // decimales con que se informa
	Real    bool    `json:"real"`    // el código aparece en el archivo
}

type DefFabrica struct {
	ID      string
	Sector  string
	Nombre  string
	Corto   string // como la nombraban los télex
	Lugar   string
	Lat     float64
	Central string // central télex a la que se conecta
	Telex   string
	Desde   string // escenarios en que existe: "1972" (las cinco de la suite temporal) o "1973"
	Indices []DefIndice
}

var CatalogoFabricas = []DefFabrica{
	// --- Las cinco fábricas conectadas en marzo de 1972 (suite temporal) ---
	{"FABRILAN", "TEX", "Fábrica de Tejidos de Lanas El Salto (Fabrilana)", "FABRILANA", "Recoleta, Santiago", -33.41, "SCL", "40211", "1972", []DefIndice{
		{"TEXFABRILANAUSENTISMOO", "Ausentismo", "AUSENTISMO", "horas-hombre/día", 380, 330, true, 0.80, 0.030, 0, true},
		{"TEXFABRILANEFICHILCARD", "Eficiencia hilandería cardada", "EFIC.HIL.CARDADA", "%", 92, 96, false, 0.87, 0.020, 1, false},
		{"TEXFABRILANPRODHILADOS", "Producción de hilados", "PROD.HILADOS", "kg/día", 5200, 6000, false, 0.83, 0.025, 0, false},
	}},
	{"PROGRESO", "TEX", "Textil Progreso", "TEXTIL PROGRESO", "Vicuña Mackenna, Santiago", -33.50, "SCL", "40217", "1972", []DefIndice{
		{"TEXPROGRESOEFICTELARES", "Eficiencia de telares", "EFIC.TELARES", "pasadas/h", 13500, 15000, false, 0.86, 0.020, 1, true},
		{"TEXPROGRESOPRODTEJIDOS", "Producción de tejidos", "PROD.TEJIDOS", "m/día", 21000, 24000, false, 0.85, 0.025, 0, false},
		{"TEXPROGRESOCONSENERGIA", "Consumo de energía", "CONS.ENERGIA", "kWh/día", 9500, 8800, true, 0.88, 0.020, 0, false},
	}},
	{"SAIDMAIP", "TEX", "Textil Said, planta Maipú", "SAID MAIPU", "Maipú, Santiago", -33.51, "SCL", "40223", "1972", []DefIndice{
		{"TEXSAIDMAIPEXISTERAYON", "Existencia de rayón", "EXIST.RAYON", "kg", 17000, 18000, false, 0.88, 0.025, 2, true},
		{"TEXSAIDMAIPPRODRAYONES", "Producción de rayón", "PROD.RAYON", "kg/día", 6500, 7200, false, 0.86, 0.022, 0, false},
		{"TEXSAIDMAIPEFICTEJEDUR", "Eficiencia tejeduría", "EFIC.TEJEDURIA", "%", 91, 95, false, 0.87, 0.020, 1, false},
	}},
	{"MELON", "CIM", "Cemento Melón", "CEMENTO MELON", "La Calera", -32.79, "VAL", "33150", "1972", []DefIndice{
		{"CIMMELONPRODUCCCLINKER", "Producción de clínker", "PROD.CLINKER", "t/día", 1350, 1450, false, 0.90, 0.020, 0, true},
		{"CIMMELONPRODUCCCEMENTO", "Producción de cemento", "PROD.CEMENTO", "t/día", 1700, 1900, false, 0.86, 0.022, 0, false},
		{"CIMMELONCONSUMOPETROLE", "Consumo de petróleo en hornos", "CONS.PETROLEO", "m3/día", 260, 240, true, 0.90, 0.018, 1, false},
	}},
	{"INFORSA", "FOR", "Industrias Forestales S.A. (Inforsa)", "INFORSA", "Nacimiento", -37.50, "LAN", "36410", "1972", []DefIndice{
		{"FORINFORSAEFICPORSULFI", "Rendimiento del proceso al sulfito", "EFIC.SULFITO", "%", 29, 31, false, 0.88, 0.020, 2, true},
		{"FORINFORSAPRODPAPELDIA", "Producción de papel", "PROD.PAPEL", "t/día", 210, 240, false, 0.86, 0.024, 0, false},
		{"FORINFORSAEXISTMADERAS", "Existencia de madera", "EXIST.MADERA", "m3", 30000, 32000, false, 0.87, 0.020, 0, false},
	}},
	// --- Las que se fueron sumando hasta septiembre de 1973 (~20 según Benadof) ---
	{"SUMAR", "TEX", "Sumar Nylon", "SUMAR NYLON", "San Joaquín, Santiago", -33.48, "SCL", "40231", "1973", []DefIndice{
		{"TEXSUMARNYLEFICESTGLOB", "Eficiencia estándar global", "EFIC.EST.GLOBAL", "%", 90, 95, false, 0.87, 0.020, 1, false},
		{"TEXSUMARNYLPRODFILAMEN", "Producción de filamento", "PROD.FILAMENTO", "kg/día", 9000, 11000, false, 0.84, 0.024, 0, false},
		{"TEXSUMARNYLAUSENTISMOO", "Ausentismo", "AUSENTISMO", "horas-hombre/día", 520, 450, true, 0.81, 0.030, 0, false},
	}},
	{"EXYARUR", "TEX", "Textil Ex-Yarur", "EX-YARUR", "Chuchunco, Santiago", -33.45, "SCL", "40239", "1973", []DefIndice{
		{"TEXEXYARURPRODTELAALGO", "Producción de tela de algodón", "PROD.TELA ALG.", "m/día", 48000, 55000, false, 0.84, 0.024, 0, false},
		{"TEXEXYARUREFICHILANDER", "Eficiencia hilandería", "EFIC.HILANDERIA", "%", 90, 94, false, 0.86, 0.020, 1, false},
		{"TEXEXYARURINASISTENCIA", "Inasistencia laboral", "INASISTENCIA", "horas-hombre/día", 1400, 1200, true, 0.80, 0.030, 0, false},
	}},
	{"CAUPOLIC", "TEX", "Textil Caupolicán Chiguayante", "CAUPOLICAN", "Chiguayante", -36.92, "CON", "26120", "1973", []DefIndice{
		{"TEXCAUPOLICPRODTEJIDOS", "Producción de tejidos", "PROD.TEJIDOS", "m/día", 26000, 30000, false, 0.84, 0.024, 0, false},
		{"TEXCAUPOLICEFICTELARES", "Eficiencia de telares", "EFIC.TELARES", "%", 89, 93, false, 0.86, 0.020, 1, false},
	}},
	{"BELLAVIS", "TEX", "Paños Bellavista Tomé", "BELLAVISTA TOME", "Tomé", -36.62, "CON", "26135", "1973", []DefIndice{
		{"TEXBELLAVISPRODPANOSLN", "Producción de paños de lana", "PROD.PANOS", "m/día", 7800, 9000, false, 0.84, 0.025, 0, false},
		{"TEXBELLAVISAUSENTISMOO", "Ausentismo", "AUSENTISMO", "horas-hombre/día", 300, 260, true, 0.80, 0.030, 0, false},
	}},
	{"POLPAICO", "CIM", "Cemento Cerro Blanco de Polpaico", "POLPAICO", "Til Til", -33.08, "SCL", "40245", "1973", []DefIndice{
		{"CIMPOLPAICOPRODCLINKER", "Producción de clínker", "PROD.CLINKER", "t/día", 1500, 1600, false, 0.89, 0.020, 0, false},
		{"CIMPOLPAICOPRODCEMENTO", "Producción de cemento", "PROD.CEMENTO", "t/día", 1900, 2100, false, 0.86, 0.022, 0, false},
	}},
	{"BIOBIO", "CIM", "Cementos Bío-Bío", "CEMENTOS BIO-BIO", "Talcahuano", -36.72, "CON", "26150", "1973", []DefIndice{
		{"CIMBIOBIOPRODUCCLINKER", "Producción de clínker", "PROD.CLINKER", "t/día", 900, 1000, false, 0.89, 0.022, 0, false},
		{"CIMBIOBIOINASISTENCIAS", "Inasistencia laboral", "INASISTENCIA", "horas-hombre/día", 160, 140, true, 0.82, 0.030, 0, false},
	}},
	{"ARAUCO", "FOR", "Celulosa Arauco", "ARAUCO", "Arauco", -37.25, "CON", "26171", "1973", []DefIndice{
		{"FORARAUCOPRODCELULOSAA", "Producción de celulosa", "PROD.CELULOSA", "t/día", 500, 560, false, 0.86, 0.022, 0, false},
		{"FORARAUCOCONSUMOMADERA", "Consumo de madera por tonelada", "CONS.MADERA", "m3/t", 4.6, 4.3, true, 0.90, 0.018, 2, false},
	}},
	{"EASTON", "FOR", "Easton (muebles)", "EASTON", "San Miguel, Santiago", -33.49, "SCL", "40251", "1973", []DefIndice{
		{"FOREASTONPRODMUEBLESUN", "Producción de muebles", "PROD.MUEBLES", "unid./día", 140, 170, false, 0.82, 0.030, 0, false},
		{"FOREASTONINASISTENCIAS", "Inasistencia laboral", "INASISTENCIA", "horas-hombre/día", 90, 80, true, 0.80, 0.030, 0, false},
	}},
	{"IRT", "ELE", "Industria de Radio y Televisión (IRT)", "IRT ARICA", "Arica", -18.48, "ARI", "21050", "1973", []DefIndice{
		{"ELEIRTPRODUCCTELEVISOR", "Producción de televisores", "PROD.TELEVISORES", "unid./día", 380, 450, false, 0.84, 0.026, 0, false},
		{"ELEIRTEXISTCINESCOPIOS", "Existencia de cinescopios importados", "EXIST.CINESCOPIOS", "unid.", 9000, 10000, false, 0.85, 0.022, 0, false},
	}},
	{"MADEMSA", "ELE", "Manufacturas de Metales (Mademsa)", "MADEMSA", "San Miguel, Santiago", -33.47, "SCL", "40257", "1973", []DefIndice{
		{"ELEMADEMSAPRODREFRIGER", "Producción de refrigeradores", "PROD.REFRIGERAD.", "unid./día", 300, 360, false, 0.84, 0.025, 0, false},
		{"ELEMADEMSAEFICLINEAMON", "Eficiencia línea de montaje", "EFIC.LINEA MONT.", "%", 90, 94, false, 0.86, 0.020, 1, false},
	}},
	{"INSA", "CAU", "Industria Nacional de Neumáticos (INSA)", "INSA", "Maipú, Santiago", -33.52, "SCL", "40263", "1973", []DefIndice{
		{"CAUINSAPRODUCCNEUMATIC", "Producción de neumáticos", "PROD.NEUMATICOS", "unid./día", 2400, 2800, false, 0.85, 0.024, 0, false},
		{"CAUINSAEXISTCAUCHONATU", "Existencia de caucho natural importado", "EXIST.CAUCHO NAT.", "t", 1200, 1300, false, 0.86, 0.022, 0, false},
		{"CAUINSAINASISTENCIALAB", "Inasistencia laboral", "INASISTENCIA", "horas-hombre/día", 700, 600, true, 0.81, 0.030, 0, false},
	}},
}

// Centrales de la red télex de ENTEL/Correos (LM Ericsson, 1967): una ARM 201/2
// en Santiago (hasta 4.000 abonados), dos ARM 503 en Valparaíso y Concepción, y
// una docena de terminales ARB 11. Las ubicaciones de las ARB son ilustrativas.
type Central struct {
	ID     string  `json:"id"`
	Nombre string  `json:"nombre"`
	Tipo   string  `json:"tipo"`
	Lat    float64 `json:"lat"`
	Padre  string  `json:"padre"`
	Radio  bool    `json:"radio"` // enlace de radio de alta frecuencia
}

var Centrales = []Central{
	{"SCL", "Santiago", "ARM 201/2", -33.45, "", false},
	{"VAL", "Valparaíso", "ARM 503", -33.05, "SCL", false},
	{"CON", "Concepción", "ARM 503", -36.82, "SCL", false},
	{"ARI", "Arica", "ARB 11", -18.48, "ANF", true},
	{"ANF", "Antofagasta", "ARB 11", -23.65, "SCL", true},
	{"LSR", "La Serena", "ARB 11", -29.90, "SCL", false},
	{"SFE", "San Fernando", "ARB 11", -34.58, "SCL", false},
	{"TAL", "Talca", "ARB 11", -35.43, "SCL", false},
	{"CHI", "Chillán", "ARB 11", -36.61, "CON", false},
	{"LAN", "Los Ángeles", "ARB 11", -37.47, "CON", false},
	{"TEM", "Temuco", "ARB 11", -38.74, "CON", false},
	{"VDV", "Valdivia", "ARB 11", -39.81, "CON", false},
	{"PMO", "Puerto Montt", "ARB 11", -41.47, "CON", false},
	{"PUQ", "Punta Arenas", "ARB 11", -53.16, "SCL", true},
}

// Abonados fijos documentados en el archivo.
const (
	TelexCORFO = "40750" // central de tráfico de CORFO, Santiago
	TelexECOM  = "40506" // Empresa Nacional de Computación e Informática
)

func init() {
	vistos := map[string]bool{}
	for _, f := range CatalogoFabricas {
		for _, d := range f.Indices {
			if len(d.Codigo) != 22 {
				panic(fmt.Sprintf("código %s tiene %d letras, deben ser 22", d.Codigo, len(d.Codigo)))
			}
			if d.Codigo[:3] != f.Sector {
				panic("código con sector equivocado: " + d.Codigo)
			}
			if vistos[d.Codigo] {
				panic("código repetido: " + d.Codigo)
			}
			vistos[d.Codigo] = true
		}
	}
}

func SectorPorID(id string) *Sector {
	for _, s := range Sectores {
		if s.ID == id {
			return s
		}
	}
	return nil
}
