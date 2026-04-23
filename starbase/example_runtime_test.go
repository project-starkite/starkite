package starbase_test

import (
	"context"
	"fmt"

	"github.com/vladimirvivien/starkite/starbase"
)

func ExampleRuntime_Call() {
	rt, _ := starbase.NewTrusted(nil)
	defer rt.Close()

	_ = rt.ExecuteRepl(context.Background(), `
def greet(name):
    return "hello, " + name
`)

	v, _ := rt.Call(context.Background(), "greet", nil, map[string]any{"name": "world"})
	fmt.Println(v)
	// Output: "hello, world"
}
