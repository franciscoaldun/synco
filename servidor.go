package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"synco/dynamo"
	"synco/motor"
	"synco/qr"
)

// ---------- difusión en tiempo real (Server-Sent Events) ----------

type cliente struct {
	ch    chan []byte
	rol   string
	ref   string
	id    string
	desde time.Time
}

type Hub struct {
	mu       sync.Mutex
	clientes map[*cliente]bool
}

func nuevoHub() *Hub { return &Hub{clientes: map[*cliente]bool{}} }

func (h *Hub) difundir(evts []motor.Evt) {
	var datos [][]byte
	for _, e := range evts {
		b, err := json.Marshal(e)
		if err != nil {
			continue
		}
		datos = append(datos, append(append([]byte("data: "), b...), '\n', '\n'))
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clientes {
		for _, d := range datos {
			select {
			case c.ch <- d:
			default:
				// cliente lento: se le pide que recargue el estado completo
			}
		}
	}
}

func (h *Hub) conectados() []map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []map[string]any
	for c := range h.clientes {
		out = append(out, map[string]any{"rol": c.rol, "ref": c.ref, "id": c.id, "desde": c.desde.UnixMilli()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["desde"].(int64) < out[j]["desde"].(int64) })
	return out
}

// ---------- servidor ----------

type Servidor struct {
	m      *motor.Motor
	hub    *Hub
	web    fs.FS
	puerto int
	clave  string
	urls   []string
}

func nuevoServidor(m *motor.Motor, web fs.FS, puerto int) *Servidor {
	b := make([]byte, 4)
	rand.Read(b)
	s := &Servidor{m: m, hub: nuevoHub(), web: web, puerto: puerto, clave: strings.ToUpper(hex.EncodeToString(b))}
	m.Emitir = s.hub.difundir
	return s
}

func (s *Servidor) rutas() http.Handler {
	mux := http.NewServeMux()
	paginas := map[string]string{
		"/": "index.html", "/sala": "sala.html", "/fabrica": "fabrica.html", "/silla": "silla.html",
		"/pueblo": "pueblo.html", "/red": "red.html", "/ecom": "ecom.html", "/futuro": "futuro.html",
		"/control": "control.html", "/historia": "historia.html",
	}
	estaticos := http.FileServer(http.FS(s.web))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if p, ok := paginas[r.URL.Path]; ok {
			b, err := fs.ReadFile(s.web, p)
			if err != nil {
				http.Error(w, "falta "+p, 500)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(b)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		estaticos.ServeHTTP(w, r)
	})

	mux.HandleFunc("GET /api/eventos", s.eventos)
	mux.HandleFunc("GET /api/estado", func(w http.ResponseWriter, r *http.Request) {
		e := s.m.Estado()
		responder(w, map[string]any{"estado": e, "red": s.infoRed(r), "conectados": s.hub.conectados()})
	})
	mux.HandleFunc("GET /api/indice/{codigo}", func(w http.ResponseWriter, r *http.Request) {
		res, hist, hs, evs, err := s.m.Historia(r.PathValue("codigo"))
		if err != nil {
			fallo(w, err, 404)
			return
		}
		responder(w, map[string]any{"indice": res, "historia": hist, "hs": hs, "eventos": evs})
	})
	mux.HandleFunc("GET /api/parte/{fabrica}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.m.Parte(r.PathValue("fabrica"))
		if err != nil {
			fallo(w, err, 404)
			return
		}
		responder(w, p)
	})
	mux.HandleFunc("GET /api/ecom/maestro", func(w http.ResponseWriter, r *http.Request) { responder(w, s.m.ArchivoMaestro()) })
	mux.HandleFunc("GET /api/ita2", func(w http.ResponseWriter, r *http.Request) {
		responder(w, motor.CodificarITA2(r.URL.Query().Get("texto")))
	})
	mux.HandleFunc("GET /api/qr", func(w http.ResponseWriter, r *http.Request) {
		q, err := qr.Codificar(r.URL.Query().Get("texto"))
		if err != nil {
			fallo(w, err, 400)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "max-age=3600")
		io.WriteString(w, q.SVG("#111", "#fff"))
	})

	// fábricas
	mux.HandleFunc("POST /api/fabrica/tomar", func(w http.ResponseWriter, r *http.Request) {
		var p struct{ Fabrica, Participante, Equipo string }
		if !leer(w, r, &p) {
			return
		}
		ok(w, s.m.TomarFabrica(p.Fabrica, p.Participante, p.Equipo))
	})
	mux.HandleFunc("POST /api/fabrica/soltar", func(w http.ResponseWriter, r *http.Request) {
		var p struct{ Fabrica string }
		if !leer(w, r, &p) {
			return
		}
		ok(w, s.m.TomarFabrica(p.Fabrica, "", ""))
	})
	mux.HandleFunc("POST /api/telex", func(w http.ResponseWriter, r *http.Request) {
		var p struct{ Fabrica, Destino, Texto string }
		if !leer(w, r, &p) {
			return
		}
		msg, err := s.m.EnviarTelexHumano(p.Fabrica, p.Destino, p.Texto)
		if err != nil {
			fallo(w, err, 400)
			return
		}
		responder(w, msg)
	})
	mux.HandleFunc("POST /api/decidir", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Senal          int
			Accion, Quien  string
			Fabrica        string
		}
		if !leer(w, r, &p) {
			return
		}
		quien := p.Quien
		if quien == "" {
			quien = "SALA DE OPERACIONES"
		}
		ok(w, s.m.Decidir(p.Senal, p.Accion, quien, p.Fabrica))
	})

	// sala de operaciones
	mux.HandleFunc("POST /api/sala/sentarse", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			N                    int
			Participante, Nombre string
		}
		if !leer(w, r, &p) {
			return
		}
		ok(w, s.m.Sentarse(p.N, p.Participante, p.Nombre))
	})
	mux.HandleFunc("POST /api/sala/levantarse", func(w http.ResponseWriter, r *http.Request) {
		var p struct{ Participante string }
		if !leer(w, r, &p) {
			return
		}
		s.m.Levantarse(p.Participante)
		ok(w, nil)
	})
	mux.HandleFunc("POST /api/sala/latido", func(w http.ResponseWriter, r *http.Request) {
		var p struct{ Participante string }
		if !leer(w, r, &p) {
			return
		}
		s.m.LatidoSilla(p.Participante)
		ok(w, nil)
	})
	mux.HandleFunc("POST /api/sala/hold", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			N            int
			Participante string
			Activo       bool
		}
		if !leer(w, r, &p) {
			return
		}
		ok(w, s.m.BotonHold(p.N, p.Participante, p.Activo))
	})
	mux.HandleFunc("POST /api/sala/combo", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			N            int
			Participante string
			Combo        []int
		}
		if !leer(w, r, &p) {
			return
		}
		if p.N == 0 && !s.esDocente(r) {
			fallo(w, fmt.Errorf("siéntese en un sillón para usar los botones"), 403)
			return
		}
		ok(w, s.m.Combinacion(p.N, p.Participante, p.Combo))
	})
	mux.HandleFunc("POST /api/sala/seleccionar", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Pared string
			Sel   motor.Seleccion
			Senal *int
		}
		if !leer(w, r, &p) {
			return
		}
		if !s.esDocente(r) {
			fallo(w, fmt.Errorf("sólo el proyector del docente navega directo"), 403)
			return
		}
		sen := -1
		if p.Senal != nil {
			sen = *p.Senal
		}
		s.m.Seleccionar(p.Pared, p.Sel, sen)
		ok(w, nil)
	})

	// pueblo
	mux.HandleFunc("POST /api/pueblo/medidor", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			ID    string
			Valor float64
		}
		if !leer(w, r, &p) {
			return
		}
		s.m.MoverMedidor(p.ID, p.Valor)
		ok(w, nil)
	})
	mux.HandleFunc("GET /api/pueblo/serie", func(w http.ResponseWriter, r *http.Request) { responder(w, s.m.SeriePueblo()) })

	// DYNAMO
	mux.HandleFunc("GET /api/dynamo/modelos", func(w http.ResponseWriter, r *http.Request) { responder(w, dynamo.Modelos()) })
	mux.HandleFunc("POST /api/dynamo/correr", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Fuente  string
			Ajustes map[string]float64
			Cyber   bool
		}
		if !leer(w, r, &p) {
			return
		}
		mod, err := dynamo.Compilar(p.Fuente)
		if err != nil {
			fallo(w, err, 400)
			return
		}
		aj := p.Ajustes
		if aj == nil {
			aj = map[string]float64{}
		}
		if p.Cyber {
			// el sueño de Beer: inyectar en CHECO datos en tiempo real de Cyberstride
			e := s.m.Estado()
			if e.Total.N > 0 {
				aj["CYBER"] = e.Total.PERF
			}
		}
		res, err := mod.Correr(aj, 1972)
		if err != nil {
			fallo(w, err, 400)
			return
		}
		responder(w, res)
	})

	// control docente (sólo desde este computador o con la clave)
	control := func(f http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !s.esDocente(r) {
				fallo(w, fmt.Errorf("el control es sólo para el docente: abra /control desde el computador que ejecuta SYNCO o use la clave"), 403)
				return
			}
			f(w, r)
		}
	}
	mux.HandleFunc("POST /api/control/pausa", control(func(w http.ResponseWriter, r *http.Request) {
		var p struct{ Pausa bool }
		if !leer(w, r, &p) {
			return
		}
		s.m.Pausar(p.Pausa)
		ok(w, nil)
	}))
	mux.HandleFunc("POST /api/control/config", control(func(w http.ResponseWriter, r *http.Request) {
		var c motor.Config
		if !leer(w, r, &c) {
			return
		}
		s.m.Configurar(c)
		ok(w, nil)
	}))
	mux.HandleFunc("POST /api/control/saltar", control(func(w http.ResponseWriter, r *http.Request) {
		s.m.SaltarFase()
		ok(w, nil)
	}))
	mux.HandleFunc("POST /api/control/reiniciar", control(func(w http.ResponseWriter, r *http.Request) {
		var p struct{ Escenario string }
		if !leer(w, r, &p) {
			return
		}
		s.m.ReiniciarSesion(p.Escenario)
		ok(w, nil)
	}))
	mux.HandleFunc("POST /api/control/evento", control(func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Codigo, Tipo, Causa string
			Magnitud            float64
			Duracion            int
		}
		if !leer(w, r, &p) {
			return
		}
		ev, err := s.m.InyectarEvento(p.Codigo, p.Tipo, p.Causa, p.Magnitud, p.Duracion)
		if err != nil {
			fallo(w, err, 400)
			return
		}
		responder(w, ev)
	}))
	mux.HandleFunc("POST /api/control/llevar-sala", control(func(w http.ResponseWriter, r *http.Request) {
		var p struct{ Senal int }
		if !leer(w, r, &p) {
			return
		}
		ok(w, s.m.LlevarALaSala(p.Senal))
	}))
	mux.HandleFunc("POST /api/control/politica", control(func(w http.ResponseWriter, r *http.Request) {
		var p struct{ Texto string }
		if !leer(w, r, &p) {
			return
		}
		s.m.AnunciarPolitica(p.Texto)
		ok(w, nil)
	}))
	mux.HandleFunc("GET /api/control/clave", func(w http.ResponseWriter, r *http.Request) {
		if !esLocal(r) {
			fallo(w, fmt.Errorf("sólo desde este computador"), 403)
			return
		}
		responder(w, map[string]string{"clave": s.clave})
	})

	// exportaciones para analizar en clase
	mux.HandleFunc("GET /api/export/historico.csv", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="cyberstride-historico.csv"`)
		io.WriteString(w, s.m.HistoricoCSV())
	})
	mux.HandleFunc("GET /api/export/telex.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="cybernet-telex.txt"`)
		e := s.m.Estado()
		for _, t := range e.Telex {
			fmt.Fprintf(w, "==== TELEX %d · DIA %d %s · %s -> %s · %s · %d caracteres ====\n%s\n\n", t.ID, t.Dia, t.Hora, t.De, t.A, t.Tipo, t.Caracteres, t.Texto)
		}
	})
	mux.HandleFunc("GET /api/export/bitacora.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="synco-bitacora.txt"`)
		io.WriteString(w, strings.Join(s.m.Estado().Bitacora, "\n"))
	})
	return mux
}

func (s *Servidor) eventos(w http.ResponseWriter, r *http.Request) {
	fl, okF := w.(http.Flusher)
	if !okF {
		http.Error(w, "sin streaming", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	q := r.URL.Query()
	c := &cliente{ch: make(chan []byte, 512), rol: q.Get("rol"), ref: q.Get("ref"), id: q.Get("id"), desde: time.Now()}
	s.hub.mu.Lock()
	s.hub.clientes[c] = true
	s.hub.mu.Unlock()
	s.avisarConectados()
	defer func() {
		s.hub.mu.Lock()
		delete(s.hub.clientes, c)
		s.hub.mu.Unlock()
		s.avisarConectados()
	}()
	fmt.Fprintf(w, "retry: 2000\ndata: {\"t\":\"hola\",\"d\":{}}\n\n")
	fl.Flush()
	latido := time.NewTicker(15 * time.Second)
	defer latido.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case d := <-c.ch:
			w.Write(d)
			// vaciar lo acumulado antes de enviar
			for n := len(c.ch); n > 0; n-- {
				w.Write(<-c.ch)
			}
			fl.Flush()
		case <-latido.C:
			io.WriteString(w, ": latido\n\n")
			fl.Flush()
		}
	}
}

func (s *Servidor) avisarConectados() {
	go s.hub.difundir([]motor.Evt{{T: "conectados", D: s.hub.conectados()}})
}

func (s *Servidor) esDocente(r *http.Request) bool {
	if esLocal(r) {
		return true
	}
	c := r.Header.Get("X-Clave")
	if c == "" {
		c = r.URL.Query().Get("clave")
	}
	return c != "" && strings.EqualFold(c, s.clave)
}

func esLocal(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Servidor) infoRed(r *http.Request) map[string]any {
	return map[string]any{"urls": s.urls, "puerto": s.puerto, "docente": s.esDocente(r), "local": esLocal(r)}
}

// direccionesLAN lista las IPv4 privadas del equipo para compartir con la clase.
// Primero va la de la interfaz con la ruta por defecto (normalmente el Wi-Fi de
// la sala); se descartan adaptadores virtuales (WSL, Hyper-V, VirtualBox, VPN).
func direccionesLAN(puerto int) []string {
	principal := ""
	if c, err := net.Dial("udp", "192.0.2.1:9"); err == nil { // no envía nada: sólo elige la ruta
		if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
			principal = a.IP.String()
		}
		c.Close()
	}
	virtuales := []string{"vethernet", "wsl", "hyper-v", "virtualbox", "vmware", "vbox", "tailscale", "zerotier", "docker", "loopback", "bluetooth"}
	type cand struct {
		ip    string
		orden int
	}
	var cs []cand
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		nombre := strings.ToLower(i.Name)
		virtual := false
		for _, v := range virtuales {
			if strings.Contains(nombre, v) {
				virtual = true
			}
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ip := ipn.IP.To4()
			if !ip.IsPrivate() {
				continue
			}
			o := 3
			switch {
			case ip.String() == principal:
				o = 0
			case virtual:
				continue
			case ip[0] == 192 && ip[1] == 168:
				o = 1
			case ip[0] == 10:
				o = 2
			}
			cs = append(cs, cand{ip.String(), o})
		}
	}
	sort.SliceStable(cs, func(a, b int) bool { return cs[a].orden < cs[b].orden })
	var out []string
	for _, c := range cs {
		out = append(out, fmt.Sprintf("http://%s:%d", c.ip, puerto))
	}
	return out
}

// ---------- utilidades HTTP ----------

func responder(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func fallo(w http.ResponseWriter, err error, codigo int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func ok(w http.ResponseWriter, err error) {
	if err != nil {
		fallo(w, err, 400)
		return
	}
	responder(w, map[string]bool{"ok": true})
}

func leer(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil && err != io.EOF {
		fallo(w, fmt.Errorf("datos inválidos: %v", err), 400)
		return false
	}
	return true
}

var _ = strconv.Itoa
