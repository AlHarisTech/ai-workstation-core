package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anomalyco/mek/internal/rir"
	"github.com/anomalyco/mek/internal/runtime"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: mek <command | -rir <path>>")
		return
	}

	arg := os.Args[1]

	if strings.HasPrefix(arg, "-") {
		switch arg {
		case "-rir":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Usage: mek -rir <path-to-rir.json>")
				os.Exit(1)
			}
			rirPath := os.Args[2]
			if err := runtime.Run(rirPath, nil); err != nil {
				fmt.Fprintf(os.Stderr, "MEK execution failed: %v\n", err)
				os.Exit(1)
			}
		default:
			fmt.Fprintln(os.Stderr, "Unknown flag:", arg)
			os.Exit(1)
		}
		return
	}

	switch arg {
	case "rir":
		handleRIR(os.Args[2:])
	default:
		fmt.Println("Unknown command:", arg)
	}
}

func handleRIR(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: mek rir generate <path>")
		return
	}

	switch args[0] {

	case "generate":
		project := args[1]

		files, err := rir.ScanProject(project)
		if err != nil {
			panic(err)
		}

		out := rir.Compile(files, project)

		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			panic(err)
		}

		err = os.WriteFile("rir.generated.json", data, 0644)
		if err != nil {
			panic(err)
		}

		fmt.Println("RIR generated: rir.generated.json")

	default:
		fmt.Println("Unknown rir command:", args[0])
	}
}
