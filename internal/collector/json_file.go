package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const maxCollectorJSONFileBytes = 64 << 20

func decodeJSONFile(path string, dest any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	limited := io.LimitReader(file, maxCollectorJSONFileBytes+1)
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	if decoder.InputOffset() > maxCollectorJSONFileBytes {
		return fmt.Errorf("collector JSON file exceeds %d MiB limit", maxCollectorJSONFileBytes>>20)
	}
	return nil
}
