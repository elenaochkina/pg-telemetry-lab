package writer

import (
	"encoding/json"
	"io"

	"github.com/elenaochkina/pg-telemetry-lab/internal/telemetry"
)

// JSONWriter writes metrics to stdout or file in JSON/JSONL format.
type JSONWriter struct {
	output io.Writer
	pretty bool
}

// NewJSONWriter creates a new JSON writer.
// If pretty is true, output will be indented for readability.
// If pretty is false, output will be compact JSONL format (one line per metric).
func NewJSONWriter(output io.Writer, pretty bool) *JSONWriter {
	return &JSONWriter{
		output: output,
		pretty: pretty,
	}
}

// Write writes a single ReplicationMetrics to the output.
func (w *JSONWriter) Write(metrics *telemetry.ReplicationMetrics) error {
	var data []byte
	var err error

	if w.pretty {
		data, err = json.MarshalIndent(metrics, "", "  ")
	} else {
		data, err = json.Marshal(metrics)
	}

	if err != nil {
		return err
	}

	// Write the JSON data
	if _, err := w.output.Write(data); err != nil {
		return err
	}

	// Add newline
	if _, err := w.output.Write([]byte("\n")); err != nil {
		return err
	}

	return nil
}

// Close closes the writer (no-op for JSON writer).
func (w *JSONWriter) Close() error {
	return nil
}
