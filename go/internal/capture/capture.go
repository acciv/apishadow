package capture

/*
#cgo CFLAGS: -I${SRCDIR}/../../../rust/include
#cgo LDFLAGS: -L${SRCDIR}/../../../rust/target/release -lapishadow_capture -Wl,-rpath,${SRCDIR}/../../../rust/target/release
#include <stdlib.h>
#include "apishadow_capture.h"
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"unsafe"
)

type Record struct {
	ID         string            `json:"id"`
	Role       string            `json:"role"`
	TS         float64           `json:"ts"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Query      string            `json:"query"`
	Status     int               `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	DurationMS float64           `json:"duration_ms"`
}

func Version() string {
	return C.GoString(C.apishadow_capture_version())
}

// NormalizeJSON прогоняет запись через Rust-ядро: убирает нестабильные заголовки.
func NormalizeJSON(record Record) (string, error) {
	raw, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	ptr := C.CString(string(raw))
	defer C.free(unsafe.Pointer(ptr))

	out := C.apishadow_capture_normalize(ptr)
	if out == nil {
		return "", fmt.Errorf("capture engine returned null")
	}
	defer C.apishadow_capture_free(out)

	result := C.GoString(out)
	var check map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result), &check); err != nil {
		return "", fmt.Errorf("decode capture result: %w", err)
	}
	if _, ok := check["error"]; ok {
		return "", fmt.Errorf("capture engine: %s", result)
	}
	return result, nil
}

type Writer struct {
	mu     sync.Mutex
	writer io.Writer
	file   *os.File
}

func NewFileWriter(path string) (*Writer, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &Writer{writer: file, file: file}, nil
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{writer: w}
}

func (w *Writer) Write(record Record) error {
	line, err := NormalizeJSON(record)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = fmt.Fprintln(w.writer, line)
	return err
}

func (w *Writer) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}
