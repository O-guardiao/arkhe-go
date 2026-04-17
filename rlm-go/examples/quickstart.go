package main

import (
	"fmt"
	"log"

	rlm "github.com/O-guardiao/arkhe-go/rlm-go"
)

func main() {
	engine, err := rlm.New(rlm.Config{
		Backend: "mock",
		BackendKwargs: map[string]any{
			"model_name": "mock-model",
			"responses":  []string{"FINAL(hello from rlm-go)"},
		},
		MaxDepth: 1,
	})
	if err != nil {
		log.Fatal(err)
	}

	result, err := engine.Completion("say hello", "")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Response)
}
