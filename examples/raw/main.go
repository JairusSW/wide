package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"

	"github.com/JairusSW/wide"
	wago "github.com/wago-org/wago"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("usage: %s module.wasm", os.Args[0])
	}
	wasmBytes, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	rt := wago.NewRuntime()
	if err := rt.Use(wide.New()); err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	mod, err := rt.Compile(wasmBytes)
	if err != nil {
		log.Fatal(err)
	}
	instance, err := rt.Instantiate(context.Background(), mod)
	if err != nil {
		log.Fatal(err)
	}
	defer instance.Close()

	if _, err := instance.Invoke("run"); err != nil {
		log.Fatal(err)
	}

	memory := instance.Memory().Bytes()
	lanes := make([]uint32, 8)
	for i := range lanes {
		lanes[i] = binary.LittleEndian.Uint32(memory[i*4:])
	}
	fmt.Println(lanes)
}
