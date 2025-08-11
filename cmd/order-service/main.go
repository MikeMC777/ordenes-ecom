package main

import (
    "log"
    "net/http"
)

func main() {
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
    })
    addr := ":8082"
    log.Printf("order-service listening on %s", addr)
    log.Fatal(http.ListenAndServe(addr, mux))
}
