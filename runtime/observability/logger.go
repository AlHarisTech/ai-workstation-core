package observability

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

type StructuredLogger struct {
	writer io.Writer
	mu     sync.Mutex
	file   *os.File
}

func NewStructuredLogger(logPath string) (*StructuredLogger, error) {
	if err := os.MkdirAll(logPath[:len(logPath)-len("/gateway.log")], 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &StructuredLogger{writer: f, file: f}, nil
}

func (sl *StructuredLogger) Log(entry types.LogEntry) {
	entry.Timestamp = time.Now()
	sl.mu.Lock()
	defer sl.mu.Unlock()
	data, _ := json.Marshal(entry)
	sl.writer.Write(append(data, '\n'))
}

func (sl *StructuredLogger) LogRaw(data []byte) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.writer.Write(append(data, '\n'))
}

func (sl *StructuredLogger) Close() error {
	if sl.file != nil {
		return sl.file.Close()
	}
	return nil
}

func LogFn(logger *StructuredLogger) func(types.LogEntry) {
	if logger == nil {
		return func(types.LogEntry) {}
	}
	return logger.Log
}
