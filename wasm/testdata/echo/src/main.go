// Echo plugin for starkite WASM integration tests.
// Compiled with: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o ../echo.wasm .
package main

import (
	"encoding/json"

	"github.com/extism/go-pdk"
)

// echo reads a JSON object with an "input" field and returns it as a quoted JSON string.
//
//go:wasmexport echo
func echo() int32 {
	var params struct {
		Input string `json:"input"`
	}
	if err := pdk.InputJSON(&params); err != nil {
		pdk.SetError(err)
		return 1
	}

	out, err := json.Marshal(params.Input)
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(out)
	return 0
}

// add reads a JSON object with "a" and "b" int fields and returns their sum as a JSON number.
//
//go:wasmexport add
func add() int32 {
	var params struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	if err := pdk.InputJSON(&params); err != nil {
		pdk.SetError(err)
		return 1
	}

	sum := params.A + params.B
	out, err := json.Marshal(sum)
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(out)
	return 0
}

func main() {}
