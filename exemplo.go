package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// GenerateMockFiles cria arquivos de log JSON para serem processados. Essa função deve ser utilizada para testar o processamento de logs.
func GenerateMockFiles(dir string, numFiles, eventsPerFile int) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	eventTypes := []string{"click", "view", "purchase", "login"}
	regions := []string{"us-east-1", "eu-west-1", "ap-southeast-2", "sa-east-1"}

	for i := 0; i < numFiles; i++ {
		filePath := filepath.Join(dir, fmt.Sprintf("log_%03d.json", i))
		file, err := os.Create(filePath)
		if err != nil {
			return err
		}

		for j := 0; j < eventsPerFile; j++ {
			var line string
			if j%50 == 0 && j > 0 {
				line = "this is not valid json\n"
			} else {
				event := Event{
					EventType: eventTypes[(i+j)%len(eventTypes)],
					Region:    regions[j%len(regions)],
				}
				data, _ := json.Marshal(event)
				line = string(data) + "\n"
			}

			if _, err := file.WriteString(line); err != nil {
				log.Printf("Erro ao escrever linha no arquivo %s: %v", filePath, err)
			}
		}
		file.Close()
	}
	return nil
}
