package cfgreader

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Read struct {
	file []byte
	err  error
}

func NewAppConfigReader() *Read { return &Read{} }

func (r *Read) ReadFrom(filePath string) *Read {
	b, err := os.ReadFile(filePath)
	if err != nil {
		r.err = err
	}

	r.file = b
	return r
}

func (r *Read) Decode(v any) error {
	if r.err != nil {
		return r.err
	}

	return yaml.Unmarshal(r.file, v)
}
