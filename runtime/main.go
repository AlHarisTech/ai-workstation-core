package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/AlHarisTech/ai-workstation-core/runtime/kernel"
)

func main() {
	cfg := kernel.DefaultConfig()

	if root := os.Getenv("AI_WORKSTATION_ROOT"); root != "" {
		cfg.WorkspaceRoot = root
	} else {
		cfg.WorkspaceRoot, _ = os.Getwd()
	}

	if w := os.Getenv("AI_GATEWAY_WORKERS"); w != "" {
		fmt.Sscanf(w, "%d", &cfg.WorkerCount)
	}
	if q := os.Getenv("AI_QUEUE_SIZE"); q != "" {
		fmt.Sscanf(q, "%d", &cfg.QueueSize)
	}

	engine, err := kernel.NewKernelEngine(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, `{"status":"fatal","error":"%s"}`+"\n", err.Error())
		os.Exit(1)
	}

	engine.Start()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if err := engine.Ingest(json.RawMessage(line)); err != nil {
			response := map[string]interface{}{
				"status": "error",
				"error":  map[string]string{"code": "INGEST_FAILED", "message": err.Error()},
			}
			data, _ := json.Marshal(response)
			fmt.Println(string(data))
		}
	}

	engine.Shutdown()
}
