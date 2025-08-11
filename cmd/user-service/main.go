package main

import (
    "log"
    "net"
)

func main() {
    l, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("user-service listening on :50051 (stub)")
    select {} // bloquea el main
}
