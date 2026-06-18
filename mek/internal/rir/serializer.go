package rir

import (
	"encoding/json"
	"os"
)

func WriteRIR(path string, rir RIR) error {
	data, err := json.MarshalIndent(rir, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
