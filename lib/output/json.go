package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// JSON marshals v as indented JSON and prints to stdout.
func JSON(v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "json marshal error: %v\n", err)
		return
	}
	fmt.Println(string(data))
}
