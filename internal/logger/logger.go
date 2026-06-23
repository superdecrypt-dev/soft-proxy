package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

type RotatorWriter struct {
	filename string
	maxSize  int64
	file     *os.File
	mu       sync.Mutex
}

func NewRotatorWriter(filename string, maxSize int64) (*RotatorWriter, error) {
	rw := &RotatorWriter{filename: filename, maxSize: maxSize}
	if err := rw.openFile(); err != nil {
		return nil, err
	}
	return rw, nil
}

func (rw *RotatorWriter) openFile() error {
	var err error
	rw.file, err = os.OpenFile(rw.filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	return err
}

func (rw *RotatorWriter) Write(p []byte) (n int, err error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	stat, err := rw.file.Stat()
	if err == nil && stat.Size()+int64(len(p)) > rw.maxSize {
		_ = rw.file.Close()
		_ = os.Rename(rw.filename, rw.filename+".1")
		if openErr := rw.openFile(); openErr != nil {
			return 0, fmt.Errorf("failed to open rotated log file: %w", openErr)
		}
	}

	if rw.file == nil {
		return 0, fmt.Errorf("log file is not open")
	}

	return rw.file.Write(p)
}

func LogJSON(level, msg string, args ...interface{}) {
	formattedMsg := fmt.Sprintf(msg, args...)
	timestamp := time.Now().Format(time.RFC3339)
	logEntry := fmt.Sprintf(`{"time":%q,"level":%q,"message":%q}`, timestamp, level, formattedMsg)
	log.Println(logEntry)
}

func Info(msg string, args ...interface{})  { LogJSON("info", msg, args...) }
func Warn(msg string, args ...interface{})  { LogJSON("warn", msg, args...) }
func Error(msg string, args ...interface{}) { LogJSON("error", msg, args...) }
