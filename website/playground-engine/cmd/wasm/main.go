//go:build js && wasm

package main

import (
	"encoding/json"
	"speedle-playground/playground"
	"syscall/js"
)

func main() {
	js.Global().Set("__speedleEvaluateJSON", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) != 1 {
			return `{"ok":false,"error":{"stage":"input","message":"Expected one JSON input."}}`
		}
		result := playground.EvaluateJSON([]byte(args[0].String()))
		data, err := json.Marshal(result)
		if err != nil {
			return `{"ok":false,"error":{"stage":"evaluation","message":"Could not serialize result."}}`
		}
		return string(data)
	}))
	js.Global().Set("__speedleReady", true)
	select {}
}
