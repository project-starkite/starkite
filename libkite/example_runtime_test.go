package libkite_test

import (
	"context"
	"fmt"

	"github.com/project-starkite/starkite/libkite"
)

func ExampleRuntime_Call() {
	rt, _ := libkite.NewTrusted(nil)
	defer rt.Close()

	_ = rt.ExecuteRepl(context.Background(), `
def greet(name):
    return "hello, " + name
`)

	v, _ := rt.Call(context.Background(), "greet", nil, map[string]any{"name": "world"})
	fmt.Println(v)
	// Output: "hello, world"
}
