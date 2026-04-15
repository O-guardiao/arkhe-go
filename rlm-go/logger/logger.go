package logger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/alexzhang13/rlm-go/types"
)

type RLMLogger struct {
	logFilePath    string
	saveToDisk     bool
	runMetadata    map[string]any
	iterations     []map[string]any
	iterationCount int
	metadataLogged bool
}

func New(logDir string, fileName string) (*RLMLogger, error) {
	logger := &RLMLogger{}
	if logDir != "" {
		if fileName == "" {
			fileName = "rlm"
		}
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return nil, err
		}
		logger.saveToDisk = true
		logger.logFilePath = filepath.Join(
			logDir,
			fileName+"_"+time.Now().Format("2006-01-02_15-04-05")+".jsonl",
		)
	}
	return logger, nil
}

func (l *RLMLogger) LogMetadata(metadata types.RLMMetadata) {
	if l == nil || l.metadataLogged {
		return
	}
	raw, _ := json.Marshal(metadata)
	_ = json.Unmarshal(raw, &l.runMetadata)
	l.metadataLogged = true
	if l.saveToDisk {
		l.writeJSONLine(map[string]any{
			"type":      "metadata",
			"timestamp": time.Now().Format(time.RFC3339Nano),
			"payload":   l.runMetadata,
		})
	}
}

func (l *RLMLogger) Log(iteration types.RLMIteration) {
	if l == nil {
		return
	}
	l.iterationCount++
	raw, _ := json.Marshal(iteration)
	payload := map[string]any{}
	_ = json.Unmarshal(raw, &payload)
	entry := map[string]any{
		"type":      "iteration",
		"iteration": l.iterationCount,
		"timestamp": time.Now().Format(time.RFC3339Nano),
	}
	for key, value := range payload {
		entry[key] = value
	}
	l.iterations = append(l.iterations, entry)
	if l.saveToDisk {
		l.writeJSONLine(entry)
	}
}

func (l *RLMLogger) ClearIterations() {
	if l == nil {
		return
	}
	l.iterations = nil
	l.iterationCount = 0
}

func (l *RLMLogger) GetTrajectory() map[string]any {
	if l == nil || l.runMetadata == nil {
		return nil
	}
	return map[string]any{
		"run_metadata": l.runMetadata,
		"iterations":   append([]map[string]any(nil), l.iterations...),
	}
}

func (l *RLMLogger) writeJSONLine(entry map[string]any) {
	file, err := os.OpenFile(l.logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()
	encoded, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = writer.Write(encoded)
	_, _ = writer.WriteString("\n")
}
