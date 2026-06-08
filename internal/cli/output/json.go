package output

import "encoding/json"

// JSONFormatter renders JSON to stdout.
type JSONFormatter struct {
	Indent string
}

func (f JSONFormatter) Format(v any) ([]byte, error) {
	if f.Indent == "" {
		return json.Marshal(v)
	}
	return json.MarshalIndent(v, "", f.Indent)
}
