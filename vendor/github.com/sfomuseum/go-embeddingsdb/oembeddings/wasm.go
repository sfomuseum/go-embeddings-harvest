//go:build wasmjs

package oembeddings

import (
	"fmt"
	"syscall/js"
)

// ValidateFunc returns a [js.Func] instance that accepts a string and validates it using the [Validate] function.
func ValidateFunc() js.Func {

	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {

		oe_str := args[0].String()

		handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {

			resolve := args[0]
			reject := args[1]

			valid, err := Validate([]byte(oe_str))

			if err != nil {
				reject.Invoke(fmt.Sprintf("Failed to validate input, %v\n", err))
				return nil
			}

			if !valid {
				reject.Invoke(fmt.Sprintf("Input failed validation, %v\n", err))
				return nil
			}

			resolve.Invoke("")
			return nil
		})

		promiseConstructor := js.Global().Get("Promise")
		return promiseConstructor.New(handler)
	})
}
