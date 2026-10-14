package qr

import (
	"os"
	"testing"
)

// Escribe matrices para verificarlas con un lector independiente.
func TestEscribirMatrices(t *testing.T) {
	textos := []string{
		"http://192.168.1.34:8073/",
		"http://192.168.100.123:8073/fabrica?id=FABRILAN",
		"http://10.0.0.5:8073/silla?n=7&nombre=Equipo%20de%20la%20sala%20de%20operaciones%20del%20Liceo",
		"HTTP://SYNCO.LOCAL:8073/PUEBLO?LOCAL=12&CLAVE=ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ",
	}
	dir := os.Getenv("QR_SALIDA")
	for i, tx := range textos {
		q, err := Codificar(tx)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("versión %d (%dx%d) para %d bytes", q.Version, q.N, q.N, len(tx))
		if dir != "" {
			os.WriteFile(dir+"/qr"+string(rune('0'+i))+".txt", []byte(tx+"\n"+q.Texto()), 0o644)
		}
	}
}
