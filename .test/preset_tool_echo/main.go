package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		log.Printf("preset_tool_echo received: %s", body)

		var payload map[string]any
		_ = json.Unmarshal(body, &payload)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"result": fmt.Sprintf("echo: %s", string(body)),
			"received": payload,
		})
	})
	log.Println("preset_tool_echo listening on :9090")
	log.Fatal(http.ListenAndServe(":9090", nil))
}
