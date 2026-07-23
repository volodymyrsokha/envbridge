// Command demo-server runs a throwaway SSH+SFTP server so the README demo
// (demo/demo.tape) can be recorded against a fake "prod-1" without touching
// real infrastructure. Not part of the envbridge product.
package main

import (
	"flag"
	"log"
	"os"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:2222", "listen address")
	dir := flag.String("dir", ".", "directory served as the fake server's home")
	flag.Parse()

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		log.Fatal(err)
	}
	srv := &gliderssh.Server{
		Addr:    *addr,
		Handler: func(s gliderssh.Session) {},
		SubsystemHandlers: map[string]gliderssh.SubsystemHandler{
			"sftp": func(sess gliderssh.Session) {
				s, err := sftp.NewServer(sess, sftp.WithServerWorkingDirectory(*dir))
				if err != nil {
					return
				}
				_ = s.Serve()
			},
		},
	}
	log.Printf("demo SSH server on %s serving %s", *addr, *dir)
	log.Fatal(srv.ListenAndServe())
}
