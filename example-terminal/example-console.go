package main

import (
	"github.com/luisbebop/goajax"
	"net/http"
	"fmt"
	"errors"
)

func main() {
	s := goajax.NewServer()
	s.Register(new(Terminal))

	http.Handle("/", http.FileServer(http.Dir("./html")))
	http.Handle("/json", s)

	fmt.Println()
	fmt.Println("Starting server: http://localhost:9001")
	http.ListenAndServe(":9001", nil)
}

type Terminal int

func (s *Terminal) Login(username, password string) (string, error) {
	if username == "demo" && password == "demo" {
		return "token-1234", nil
	} else {
		return "", errors.New("Wrong password")
	}
}

func (s *Terminal) Ls(token string) ([]string, error) {
	return []string{"a", "b", "c"}, nil
}

func (s *Terminal) Help() (string, error) {
	return "Help\nHelp\nHelp\n", nil
}