// SYNCO — emulador del Proyecto Cybersyn (Chile, 1971-1973).
//
// Un solo ejecutable, sin instalación ni internet: levanta un servidor en la
// red local para que un curso completo opere a la vez las fábricas, la red de
// télex, el computador de ECOM, la Sala de Operaciones, el simulador CHECO y el
// medidor algedónico del pueblo.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"synco/motor"
)

//go:embed web
var webFS embed.FS

const version = "1.0"

func main() {
	puerto := flag.Int("puerto", 8073, "puerto HTTP (8073 por 1973)")
	escenario := flag.String("escenario", "abril1973", "abril1973 | marzo1972 | octubre1972")
	sinNavegador := flag.Bool("sin-navegador", false, "no abrir el navegador al partir")
	velocidad := flag.Float64("velocidad", 1, "multiplicador del reloj de la simulación")
	dirWeb := flag.String("web", "", "servir la interfaz desde esta carpeta (para desarrollo)")
	flag.Parse()

	web, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	if *dirWeb != "" {
		web = os.DirFS(*dirWeb)
	}
	cfg := motor.ConfigPorDefecto()
	cfg.Velocidad = *velocidad
	m := motor.Nuevo(cfg, *escenario, 0)

	ln, p, err := escuchar(*puerto)
	if err != nil {
		fmt.Println("No pude abrir ningún puerto entre", *puerto, "y", *puerto+20, ":", err)
		esperarEnter()
		os.Exit(1)
	}
	s := nuevoServidor(m, web, p)
	s.urls = direccionesLAN(p)

	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		ultimo := time.Now()
		for ahora := range t.C {
			m.Avanzar(ahora.Sub(ultimo))
			ultimo = ahora
		}
	}()

	local := fmt.Sprintf("http://localhost:%d", p)
	fmt.Println()
	fmt.Println("  ███████ ██    ██ ███    ██  ██████  ██████ ")
	fmt.Println("  ██       ██  ██  ████   ██ ██      ██    ██")
	fmt.Println("  ███████   ████   ██ ██  ██ ██      ██    ██")
	fmt.Println("       ██    ██    ██  ██ ██ ██      ██    ██")
	fmt.Println("  ███████    ██    ██   ████  ██████  ██████ ")
	fmt.Println()
	fmt.Println("  Emulador del Proyecto Cybersyn · Chile 1971-1973 · versión", version)
	fmt.Println("  Desarrollado por Francisco Aldunate · github.com/franciscoaldun/synco")
	fmt.Println()
	fmt.Println("  En este computador:   ", local)
	if len(s.urls) > 0 {
		fmt.Println("  Para los estudiantes (misma red Wi-Fi):")
		for _, u := range s.urls {
			fmt.Println("                        ", u)
		}
	} else {
		fmt.Println("  No encontré una red local: los estudiantes no podrán conectarse.")
	}
	fmt.Println("  Clave del docente:    ", s.clave, "(para controlar desde otro equipo)")
	fmt.Println()
	fmt.Println("  Si Windows pregunta por el firewall, permita el acceso en redes privadas.")
	fmt.Println("  Para terminar, cierre esta ventana.")
	fmt.Println()

	if !*sinNavegador {
		go func() {
			time.Sleep(400 * time.Millisecond)
			abrirNavegador(local + "/")
		}()
	}
	srv := &http.Server{Handler: s.rutas(), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(ln); err != nil {
		fmt.Println("El servidor se detuvo:", err)
		esperarEnter()
	}
}

func escuchar(desde int) (net.Listener, int, error) {
	var ultimo error
	for p := desde; p <= desde+20; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			return ln, p, nil
		}
		ultimo = err
	}
	return nil, 0, ultimo
}

func abrirNavegador(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func esperarEnter() {
	fmt.Println("Presione Enter para cerrar.")
	var s string
	fmt.Scanln(&s)
}
